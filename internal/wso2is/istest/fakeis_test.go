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

package istest

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestFakeISRoutesAndRecording(t *testing.T) {
	srv := New(t)
	srv.HandleJSON(http.MethodGet, "/api/server/v1/cors/origins", http.StatusOK, map[string]any{
		"origins": []string{"https://example.com"},
	})

	resp, err := srv.Client().Get(srv.URL() + "/api/server/v1/cors/origins")
	if err != nil {
		t.Fatalf("GET failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	reqs := srv.Requests()
	if len(reqs) != 1 || reqs[0].Method != http.MethodGet || reqs[0].Path != "/api/server/v1/cors/origins" {
		t.Fatalf("unexpected recorded requests: %+v", reqs)
	}
}

func TestFakeISTokenEndpoints(t *testing.T) {
	srv := New(t)
	srv.RejectOrg("denied-org")

	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("scope", "SYSTEM")
	resp, err := srv.Client().PostForm(srv.URL()+"/oauth2/token", form)
	if err != nil {
		t.Fatalf("token POST failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("client_credentials status = %d", resp.StatusCode)
	}
	var cc struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&cc); err != nil || cc.AccessToken == "" {
		t.Fatalf("decode client_credentials: %v", err)
	}

	orgForm := url.Values{}
	orgForm.Set("grant_type", "organization_switch")
	orgForm.Set("token", cc.AccessToken)
	orgForm.Set("switching_organization", "acme-org")
	resp2, err := srv.Client().PostForm(srv.URL()+"/oauth2/token", orgForm)
	if err != nil {
		t.Fatalf("org switch POST failed: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("organization_switch status = %d", resp2.StatusCode)
	}

	orgForm.Set("switching_organization", "denied-org")
	resp3, err := srv.Client().PostForm(srv.URL()+"/oauth2/token", orgForm)
	if err != nil {
		t.Fatalf("denied org switch POST failed: %v", err)
	}
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusForbidden {
		t.Fatalf("denied org switch status = %d, want 403", resp3.StatusCode)
	}
}

func TestFakeISJWKSAndSignJWT(t *testing.T) {
	srv := New(t)

	resp, err := srv.Client().Get(srv.URL() + "/oauth2/jwks")
	if err != nil {
		t.Fatalf("GET jwks failed: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	var jwks struct {
		Keys []map[string]any `json:"keys"`
	}
	if err := json.Unmarshal(body, &jwks); err != nil || len(jwks.Keys) != 1 {
		t.Fatalf("invalid JWKS response: %s", string(body))
	}
	if srv.JWKSFetches() != 1 {
		t.Fatalf("JWKSFetches = %d, want 1", srv.JWKSFetches())
	}

	tok := srv.SignJWT(map[string]any{
		"sub": "alice",
		"iss": srv.URL() + "/oauth2/token",
		"aud": "http://localhost:8000/mcp",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("expected 3 JWT segments, got %d", len(parts))
	}

	pubAny, err := x509.ParsePKIXPublicKey(srv.PublicKeyDER())
	if err != nil {
		t.Fatalf("parse pub key: %v", err)
	}
	pub := pubAny.(*rsa.PublicKey)
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatalf("decode sig: %v", err)
	}
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, digest[:], sig); err != nil {
		t.Fatalf("JWT signature verification failed: %v", err)
	}
}
