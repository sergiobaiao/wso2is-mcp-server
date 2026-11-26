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

package client

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/asgardeo/mcp/internal/config"
	"github.com/asgardeo/mcp/internal/wso2is/catalog"
	"github.com/asgardeo/mcp/internal/wso2is/istest"
	"github.com/getkin/kin-openapi/openapi3"
)

func newTestConfig(srv *istest.Server, t *testing.T) *config.Config {
	t.Helper()
	return &config.Config{
		ServerURL:    srv.URL(),
		TenantDomain: config.DefaultTenantDomain,
		AuthMode:     config.AuthModeClientCredentials,
		ClientID:     "test-client-id",
		ClientSecret: "test-client-secret-value",
		CertPath:     srv.WriteCAPEM(t),
	}
}

func testSCIMListOp() *catalog.Operation {
	return &catalog.Operation{
		ID:     "users.listUsers",
		Area:   "users",
		Family: catalog.FamilyRoot,
		Method: http.MethodGet,
		Path:   "/scim2/Users",
		Kind:   catalog.KindRead,
		Scopes: []string{"internal_user_mgt_list"},
		Params: []catalog.Param{
			{Name: "filter", In: "query", Schema: openapi3.NewSchemaRef("", openapi3.NewStringSchema())},
			{Name: "count", In: "query", Schema: openapi3.NewSchemaRef("", openapi3.NewIntegerSchema())},
		},
	}
}

func TestClientCredentialsFetchAndCache(t *testing.T) {
	srv := istest.New(t)
	srv.HandleJSON(http.MethodGet, "/scim2/Users", http.StatusOK, map[string]any{
		"totalResults": 1,
		"Resources":    []any{map[string]any{"id": "u1", "userName": "alice"}},
	})

	c, err := New(newTestConfig(srv, t))
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	op := testSCIMListOp()
	res1, err := c.Do(context.Background(), op, nil, nil, nil)
	if err != nil {
		t.Fatalf("first Do() failed: %v", err)
	}
	if res1 == nil {
		t.Fatal("expected non-nil result")
	}

	_, err = c.Do(context.Background(), op, nil, nil, nil)
	if err != nil {
		t.Fatalf("second Do() failed: %v", err)
	}

	reqs := srv.Requests()
	var tokenReqs, apiReqs int
	for _, r := range reqs {
		switch r.Path {
		case "/oauth2/token":
			tokenReqs++
			if !strings.Contains(string(r.Body), "grant_type=client_credentials") {
				t.Errorf("expected grant_type=client_credentials, got %q", string(r.Body))
			}
			if !strings.Contains(string(r.Body), "scope=SYSTEM") {
				t.Errorf("expected scope=SYSTEM, got %q", string(r.Body))
			}
		case "/scim2/Users":
			apiReqs++
			if got := r.Headers.Get("Authorization"); got != "Bearer cc-token-1" {
				t.Errorf("Authorization = %q, want Bearer cc-token-1", got)
			}
		}
	}
	if tokenReqs != 1 {
		t.Errorf("token requests = %d, want 1 (cached)", tokenReqs)
	}
	if apiReqs != 2 {
		t.Errorf("API requests = %d, want 2", apiReqs)
	}
}

func TestClientCredentialsExpiryMargin(t *testing.T) {
	srv := istest.New(t)
	srv.HandleJSON(http.MethodGet, "/scim2/Users", http.StatusOK, map[string]any{"Resources": []any{}})

	c, err := New(newTestConfig(srv, t))
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	c.setNow(func() time.Time { return now })

	op := testSCIMListOp()
	if _, err := c.Do(context.Background(), op, nil, nil, nil); err != nil {
		t.Fatalf("Do 1 failed: %v", err)
	}

	// Still valid at 31s before expiry (expires_in=3600s -> 3569s elapsed).
	now = now.Add(3569 * time.Second)
	if _, err := c.Do(context.Background(), op, nil, nil, nil); err != nil {
		t.Fatalf("Do 2 failed: %v", err)
	}

	// Within 30s margin (3571s elapsed) -> must refresh.
	now = now.Add(2 * time.Second)
	if _, err := c.Do(context.Background(), op, nil, nil, nil); err != nil {
		t.Fatalf("Do 3 failed: %v", err)
	}

	var tokenReqs int
	var lastAuth string
	for _, r := range srv.Requests() {
		if r.Path == "/oauth2/token" {
			tokenReqs++
		}
		if r.Path == "/scim2/Users" {
			lastAuth = r.Headers.Get("Authorization")
		}
	}
	if tokenReqs != 2 {
		t.Errorf("token requests = %d, want 2 after 30s expiry margin", tokenReqs)
	}
	if lastAuth != "Bearer cc-token-2" {
		t.Errorf("last Authorization = %q, want Bearer cc-token-2", lastAuth)
	}
}

func TestStaticTokenAuth(t *testing.T) {
	srv := istest.New(t)
	srv.HandleJSON(http.MethodGet, "/scim2/Users", http.StatusOK, map[string]any{"Resources": []any{}})

	cfg := &config.Config{
		ServerURL:    srv.URL(),
		TenantDomain: config.DefaultTenantDomain,
		AuthMode:     config.AuthModeStaticToken,
		AccessToken:  "static-enduser-token",
		CertPath:     srv.WriteCAPEM(t),
	}
	c, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	if _, err := c.Do(context.Background(), testSCIMListOp(), nil, nil, nil); err != nil {
		t.Fatalf("Do() failed: %v", err)
	}

	reqs := srv.Requests()
	if len(reqs) != 1 {
		t.Fatalf("expected 1 request (no token call), got %d", len(reqs))
	}
	if got := reqs[0].Headers.Get("Authorization"); got != "Bearer static-enduser-token" {
		t.Errorf("Authorization = %q, want Bearer static-enduser-token", got)
	}
}

func TestBearerFromCredentialContext(t *testing.T) {
	srv := istest.New(t)
	srv.HandleJSON(http.MethodGet, "/scim2/Users", http.StatusOK, map[string]any{"Resources": []any{}})

	cfg := &config.Config{
		ServerURL:    srv.URL(),
		TenantDomain: config.DefaultTenantDomain,
		AuthMode:     config.AuthModeBearer,
		CertPath:     srv.WriteCAPEM(t),
	}
	c, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Missing bearer token must fail without hitting IS.
	if _, err := c.Do(context.Background(), testSCIMListOp(), nil, nil, nil); err == nil {
		t.Fatal("expected error when bearer token missing")
	}
	if len(srv.Requests()) != 0 {
		t.Fatalf("expected 0 requests when bearer token missing, got %d", len(srv.Requests()))
	}

	ctx := WithCredentialContext(context.Background(), CredentialContext{
		Mode:        config.AuthModeBearer,
		BearerToken: "incoming-user-jwt",
		Subject:     "alice",
	})
	if _, err := c.Do(ctx, testSCIMListOp(), nil, nil, nil); err != nil {
		t.Fatalf("Do() with bearer failed: %v", err)
	}
	reqs := srv.Requests()
	if len(reqs) != 1 || reqs[0].Headers.Get("Authorization") != "Bearer incoming-user-jwt" {
		t.Errorf("unexpected requests: %+v", reqs)
	}
}

func TestOrgSwitchTokenAndCache(t *testing.T) {
	srv := istest.New(t)
	srv.HandleJSON(http.MethodGet, "/o/scim2/Users", http.StatusOK, map[string]any{"Resources": []any{}})

	c, err := New(newTestConfig(srv, t))
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	op := &catalog.Operation{
		ID:     "users.org.listUsers",
		Area:   "users",
		Family: catalog.FamilyOrg,
		Method: http.MethodGet,
		Path:   "/o/scim2/Users",
		Kind:   catalog.KindRead,
		Scopes: []string{"internal_org_user_mgt_list"},
	}

	ctx := WithOrganizationID(context.Background(), "org-abc")
	if _, err := c.Do(ctx, op, nil, nil, nil); err != nil {
		t.Fatalf("org Do 1 failed: %v", err)
	}
	if _, err := c.Do(ctx, op, nil, nil, nil); err != nil {
		t.Fatalf("org Do 2 failed: %v", err)
	}

	var ccReqs, orgSwitchReqs, apiReqs int
	for _, r := range srv.Requests() {
		if r.Path == "/oauth2/token" {
			body := string(r.Body)
			if strings.Contains(body, "grant_type=client_credentials") {
				ccReqs++
			}
			if strings.Contains(body, "grant_type=organization_switch") {
				orgSwitchReqs++
				if !strings.Contains(body, "switching_organization=org-abc") {
					t.Errorf("missing switching_organization in %q", body)
				}
				if !strings.Contains(body, "scope=internal_org_user_mgt_list") {
					t.Errorf("missing scope in %q", body)
				}
			}
		}
		if r.Path == "/o/scim2/Users" {
			apiReqs++
			if !strings.HasPrefix(r.Headers.Get("Authorization"), "Bearer org-token-org-abc-") {
				t.Errorf("expected org token, got %q", r.Headers.Get("Authorization"))
			}
		}
	}
	if ccReqs != 1 || orgSwitchReqs != 1 || apiReqs != 2 {
		t.Errorf("counts cc=%d org=%d api=%d, want 1/1/2", ccReqs, orgSwitchReqs, apiReqs)
	}
}

func TestTenantURLBuildingAndParamEncoding(t *testing.T) {
	srv := istest.New(t)
	srv.HandleJSON(http.MethodGet, "/t/acme.com/scim2/Users/alice%2Fbob", http.StatusOK, map[string]any{"id": "1"})
	srv.SetFallback(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"path":     r.URL.EscapedPath(),
			"rawQuery": r.URL.RawQuery,
		})
	})

	cfg := newTestConfig(srv, t)
	cfg.TenantDomain = "acme.com"
	c, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	op := &catalog.Operation{
		ID:     "users.getUser",
		Method: http.MethodGet,
		Path:   "/scim2/Users/{id}",
		Kind:   catalog.KindRead,
		Params: []catalog.Param{
			{Name: "id", In: "path", Required: true, Schema: openapi3.NewSchemaRef("", openapi3.NewStringSchema())},
			{Name: "attributes", In: "query", Schema: openapi3.NewSchemaRef("", openapi3.NewStringSchema())},
		},
	}

	res, err := c.Do(context.Background(), op, map[string]string{"id": "alice/bob"}, map[string]any{"attributes": "userName,emails"}, nil)
	if err != nil {
		t.Fatalf("Do() failed: %v", err)
	}
	m, ok := res.(map[string]any)
	if !ok {
		t.Fatalf("unexpected result type %T", res)
	}
	if m["path"] != "/t/acme.com/scim2/Users/alice%2Fbob" {
		t.Errorf("path = %v, want /t/acme.com/scim2/Users/alice%%2Fbob", m["path"])
	}
	if m["rawQuery"] != "attributes=userName%2Cemails" {
		t.Errorf("rawQuery = %v", m["rawQuery"])
	}
}

func TestSCIMContentTypeAndBodyFormats(t *testing.T) {
	srv := istest.New(t)
	var gotCT string
	var gotBody []byte
	srv.Handle(http.MethodPost, "/scim2/Users", func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/scim+json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"u-new","userName":"bob"}`))
	})

	srv.Handle(http.MethodPost, "/api/server/v1/applications/import", func(w http.ResponseWriter, r *http.Request) {
		ct := r.Header.Get("Content-Type")
		mediaType, params, err := mime.ParseMediaType(ct)
		if err != nil || mediaType != "multipart/form-data" {
			t.Errorf("unexpected multipart Content-Type: %q (%v)", ct, err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		mr := multipart.NewReader(r.Body, params["boundary"])
		part, err := mr.NextPart()
		if err != nil {
			t.Errorf("NextPart failed: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		data, _ := io.ReadAll(part)
		if part.FormName() != "file" || string(data) != "<app/>" {
			t.Errorf("part=%q data=%q, want file/<app/>", part.FormName(), string(data))
		}
		w.WriteHeader(http.StatusCreated)
	})

	c, err := New(newTestConfig(srv, t))
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	scimOp := &catalog.Operation{
		ID:     "users.createUser",
		Method: http.MethodPost,
		Path:   "/scim2/Users",
		Kind:   catalog.KindWrite,
		Body: &catalog.BodySpec{
			ContentType: "application/scim+json",
			Required:    true,
			Schema: openapi3.NewSchemaRef("", openapi3.NewObjectSchema().
				WithProperty("userName", openapi3.NewStringSchema())),
		},
	}

	res, err := c.Do(context.Background(), scimOp, nil, nil, map[string]any{"userName": "bob"})
	if err != nil {
		t.Fatalf("SCIM Do() failed: %v", err)
	}
	if gotCT != "application/scim+json" {
		t.Errorf("Content-Type = %q, want application/scim+json", gotCT)
	}
	if !strings.Contains(string(gotBody), `"userName":"bob"`) {
		t.Errorf("unexpected SCIM body: %s", string(gotBody))
	}
	if res == nil {
		t.Fatal("expected parsed JSON response")
	}

	// Multipart upload with base64-encoded binary field -> 201 with empty body returns nil, nil.
	fileSchema := openapi3.NewStringSchema()
	fileSchema.Format = "binary"
	multipartOp := &catalog.Operation{
		ID:     "applications.importApplication",
		Method: http.MethodPost,
		Path:   "/api/server/v1/applications/import",
		Kind:   catalog.KindWrite,
		Body: &catalog.BodySpec{
			ContentType: "multipart/form-data",
			Required:    true,
			Schema: openapi3.NewSchemaRef("", openapi3.NewObjectSchema().
				WithProperty("file", fileSchema)),
		},
	}
	encoded := base64.StdEncoding.EncodeToString([]byte("<app/>"))
	res2, err := c.Do(context.Background(), multipartOp, nil, nil, map[string]any{"file": encoded})
	if err != nil {
		t.Fatalf("multipart Do() failed: %v", err)
	}
	if res2 != nil {
		t.Errorf("expected nil on empty 201 body, got %v", res2)
	}
}

func TestCertPathEnforcedAndNoTokenInLogs(t *testing.T) {
	srv := istest.New(t)
	srv.HandleJSON(http.MethodGet, "/scim2/Users", http.StatusOK, map[string]any{"Resources": []any{}})

	// Without CERT_PATH, connecting to self-signed istest MUST fail (TLS verification is never disabled).
	cfgNoCert := &config.Config{
		ServerURL:    srv.URL(),
		TenantDomain: config.DefaultTenantDomain,
		AuthMode:     config.AuthModeStaticToken,
		AccessToken:  "super-secret-static-token",
	}
	cNoCert, err := New(cfgNoCert)
	if err != nil {
		t.Fatalf("New() without CertPath failed: %v", err)
	}
	if _, err := cNoCert.Do(context.Background(), testSCIMListOp(), nil, nil, nil); err == nil {
		t.Fatal("expected TLS verification failure when CERT_PATH is not configured for self-signed server")
	}

	// Capture slog output while running requests and error responses.
	var logBuf bytes.Buffer
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(oldLogger) })

	cfg := newTestConfig(srv, t)
	c, err := New(cfg)
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}
	if _, err := c.Do(context.Background(), testSCIMListOp(), nil, nil, nil); err != nil {
		t.Fatalf("Do() failed: %v", err)
	}

	// Also verify DoRaw fails cleanly with APIError on 404.
	_, err = c.DoRaw(context.Background(), http.MethodGet, "/nonexistent", "", nil)
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 APIError from DoRaw, got %v", err)
	}

	logs := logBuf.String()
	for _, secret := range []string{cfg.ClientSecret, "cc-token-1", "super-secret-static-token"} {
		if strings.Contains(logs, secret) {
			t.Errorf("secret %q leaked in logs: %s", secret, logs)
		}
	}
}

func TestValidationBlocksHTTPRequestAndFormURLEncoded(t *testing.T) {
	srv := istest.New(t)
	var gotFormCT, gotFormBody string
	srv.Handle(http.MethodPost, "/api/users/v2/me/webauthn/start-usernameless-registration", func(w http.ResponseWriter, r *http.Request) {
		gotFormCT = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotFormBody = string(b)
		w.WriteHeader(http.StatusCreated)
	})

	c, err := New(newTestConfig(srv, t))
	if err != nil {
		t.Fatalf("New() failed: %v", err)
	}

	// Invalid input must fail before any HTTP request (not even a token request) is sent.
	getOp := &catalog.Operation{
		ID:     "users.getUser",
		Method: http.MethodGet,
		Path:   "/scim2/Users/{id}",
		Kind:   catalog.KindRead,
		Params: []catalog.Param{
			{Name: "id", In: "path", Required: true, Schema: openapi3.NewSchemaRef("", openapi3.NewStringSchema())},
		},
	}
	if _, err := c.Do(context.Background(), getOp, nil, nil, nil); err == nil {
		t.Fatal("expected validation error")
	}
	if len(srv.Requests()) != 0 {
		t.Fatalf("expected 0 HTTP requests on validation error, got %d", len(srv.Requests()))
	}

	// Form-urlencoded body support.
	formSchema := openapi3.NewObjectSchema().WithProperty("appId", openapi3.NewStringSchema())
	formSchema.Required = []string{"appId"}
	formOp := &catalog.Operation{
		ID:     "self_service_me.startFIDO",
		Method: http.MethodPost,
		Path:   "/api/users/v2/me/webauthn/start-usernameless-registration",
		Kind:   catalog.KindWrite,
		Body: &catalog.BodySpec{
			ContentType: "application/x-www-form-urlencoded",
			Required:    true,
			Schema:      openapi3.NewSchemaRef("", formSchema),
		},
	}
	if _, err := c.Do(context.Background(), formOp, nil, nil, map[string]any{"appId": "https://localhost:9443"}); err != nil {
		t.Fatalf("form Do() failed: %v", err)
	}
	if gotFormCT != "application/x-www-form-urlencoded" {
		t.Errorf("form Content-Type = %q", gotFormCT)
	}
	if gotFormBody != "appId=https%3A%2F%2Flocalhost%3A9443" {
		t.Errorf("form body = %q", gotFormBody)
	}
}
