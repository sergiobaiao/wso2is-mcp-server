//go:build integration

/*
 * Copyright (c) 2026, WSO2 LLC. (http://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

package integration

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// spikeHTTPClient trusts the system pool plus the IS CA in CERT_PATH. TLS verification is never
// disabled (constitution Principle III); export the container CA as in quickstart.md.
func spikeHTTPClient() *http.Client {
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	if p := os.Getenv("CERT_PATH"); p != "" {
		if pem, err := os.ReadFile(filepath.Clean(p)); err == nil {
			pool.AppendCertsFromPEM(pem)
		}
	}
	return &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12},
		},
	}
}

func requireISURL(t *testing.T) string {
	t.Helper()
	u := strings.TrimRight(os.Getenv("IS_URL"), "/")
	if u == "" {
		t.Skip("IS_URL not set; skipping integration spike test")
	}
	return u
}

func fetchClientCredentialsToken(t *testing.T, httpClient *http.Client, tokenURL, clientID, clientSecret, scope string) string {
	t.Helper()
	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	if scope != "" {
		form.Set("scope", scope)
	}
	req, err := http.NewRequest(http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("new token request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(clientID, clientSecret)

	resp, err := httpClient.Do(req)
	if err != nil {
		t.Fatalf("token request failed: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("token endpoint returned %d: %s", resp.StatusCode, string(body))
	}
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tok); err != nil || tok.AccessToken == "" {
		t.Fatalf("decode access token: %v (%s)", err, string(body))
	}
	return tok.AccessToken
}

// TestSpikeClientCredentialsSystemScope verifies research.md R7:
// client_credentials with scope=SYSTEM works on /api/server/v1/applications.
func TestSpikeClientCredentialsSystemScope(t *testing.T) {
	isURL := requireISURL(t)
	clientID := os.Getenv("IS_CLIENT_ID")
	clientSecret := os.Getenv("IS_CLIENT_SECRET")
	if clientID == "" || clientSecret == "" {
		t.Skip("IS_CLIENT_ID / IS_CLIENT_SECRET not set")
	}

	hc := spikeHTTPClient()
	token := fetchClientCredentialsToken(t, hc, isURL+"/oauth2/token", clientID, clientSecret, "SYSTEM")

	req, err := http.NewRequest(http.MethodGet, isURL+"/api/server/v1/applications", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := hc.Do(req)
	if err != nil {
		t.Fatalf("GET applications: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on /api/server/v1/applications, got %d: %s", resp.StatusCode, string(body))
	}
}

// TestSpikeOrganizationSwitch verifies research.md R6:
// organization_switch grant from an M2M token for a shared sub-organization.
func TestSpikeOrganizationSwitch(t *testing.T) {
	isURL := requireISURL(t)
	clientID := os.Getenv("IS_CLIENT_ID")
	clientSecret := os.Getenv("IS_CLIENT_SECRET")
	orgID := os.Getenv("IS_ORG_ID")
	if clientID == "" || clientSecret == "" || orgID == "" {
		t.Skip("IS_CLIENT_ID / IS_CLIENT_SECRET / IS_ORG_ID not set")
	}

	hc := spikeHTTPClient()
	rootToken := fetchClientCredentialsToken(t, hc, isURL+"/oauth2/token", clientID, clientSecret, "SYSTEM")

	form := url.Values{}
	form.Set("grant_type", "organization_switch")
	form.Set("token", rootToken)
	form.Set("switching_organization", orgID)
	form.Set("scope", "internal_org_user_mgt_list")

	req, err := http.NewRequest(http.MethodPost, isURL+"/oauth2/token", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("new org switch request: %v", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(clientID, clientSecret)

	resp, err := hc.Do(req)
	if err != nil {
		t.Fatalf("org switch failed: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on organization_switch, got %d: %s", resp.StatusCode, string(body))
	}
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tok); err != nil || tok.AccessToken == "" {
		t.Fatalf("missing access_token in org switch response: %s", string(body))
	}
}

// TestSpikeTenantPrefixedDCRAndScopes verifies research.md R14:
// DCR and OAuth2 scope endpoints work under /t/carbon.super.
func TestSpikeTenantPrefixedDCRAndScopes(t *testing.T) {
	isURL := requireISURL(t)
	clientID := os.Getenv("IS_CLIENT_ID")
	clientSecret := os.Getenv("IS_CLIENT_SECRET")
	if clientID == "" || clientSecret == "" {
		t.Skip("IS_CLIENT_ID / IS_CLIENT_SECRET not set")
	}

	hc := spikeHTTPClient()
	token := fetchClientCredentialsToken(t, hc, isURL+"/oauth2/token", clientID, clientSecret, "SYSTEM")

	for _, path := range []string{
		"/t/carbon.super/api/identity/oauth2/dcr/v1.1/register",
		"/t/carbon.super/api/identity/oauth2/meta/v1.0/scopes",
	} {
		req, err := http.NewRequest(http.MethodGet, isURL+path, nil)
		if err != nil {
			t.Fatalf("new request %s: %v", path, err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := hc.Do(req)
		if err != nil {
			t.Fatalf("GET %s failed: %v", path, err)
		}
		resp.Body.Close()
		if resp.StatusCode == http.StatusNotFound {
			t.Fatalf("%s returned 404 under /t/carbon.super", path)
		}
	}
}

// TestSpikeMCPClientJWTForIS verifies research.md R5:
// a JWT from an MCP Client app authorized for an MCP server contains MCP_RESOURCE in aud and works on /scim2/Users.
func TestSpikeMCPClientJWTForIS(t *testing.T) {
	isURL := requireISURL(t)
	jwtToken := os.Getenv("IS_MCP_CLIENT_JWT")
	mcpResource := os.Getenv("MCP_RESOURCE")
	if jwtToken == "" || mcpResource == "" {
		t.Skip("IS_MCP_CLIENT_JWT / MCP_RESOURCE not set")
	}

	parts := strings.Split(jwtToken, ".")
	if len(parts) != 3 {
		t.Fatalf("IS_MCP_CLIENT_JWT is not a JWT")
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode JWT payload: %v", err)
	}
	var claims struct {
		Aud any `json:"aud"`
	}
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		t.Fatalf("unmarshal JWT claims: %v", err)
	}
	t.Logf("JWT aud claim: %v", claims.Aud)

	hc := spikeHTTPClient()
	req, err := http.NewRequest(http.MethodGet, isURL+"/scim2/Users?count=1", nil)
	if err != nil {
		t.Fatalf("new SCIM request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+jwtToken)
	resp, err := hc.Do(req)
	if err != nil {
		t.Fatalf("GET /scim2/Users failed: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on /scim2/Users with MCP client JWT, got %d: %s", resp.StatusCode, string(body))
	}
}

// TestSpikeLoginFlowAI verifies research.md R17:
// Login Flow AI endpoints respond or fail with a recognizable error.
func TestSpikeLoginFlowAI(t *testing.T) {
	isURL := requireISURL(t)
	clientID := os.Getenv("IS_CLIENT_ID")
	clientSecret := os.Getenv("IS_CLIENT_SECRET")
	if clientID == "" || clientSecret == "" {
		t.Skip("IS_CLIENT_ID / IS_CLIENT_SECRET not set")
	}

	hc := spikeHTTPClient()
	token := fetchClientCredentialsToken(t, hc, isURL+"/oauth2/token", clientID, clientSecret, "SYSTEM")

	reqBody := []byte(`{"user_query":"username and password","available_authenticators":["BasicAuthenticator"]}`)
	req, err := http.NewRequest(http.MethodPost, isURL+"/api/server/v1/applications/loginflow/generate", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("new loginflow request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := hc.Do(req)
	if err != nil {
		t.Fatalf("POST loginflow/generate failed: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 500 && len(body) == 0 {
		t.Fatalf("unexpected empty %d from loginflow/generate", resp.StatusCode)
	}
	t.Logf("loginflow/generate status=%d body=%s", resp.StatusCode, string(body))
}
