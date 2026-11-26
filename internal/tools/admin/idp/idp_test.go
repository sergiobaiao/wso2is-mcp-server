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

package idp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	apispecs "github.com/asgardeo/mcp/api-specs"
	"github.com/asgardeo/mcp/internal/config"
	"github.com/asgardeo/mcp/internal/tools/admin/idp"
	"github.com/asgardeo/mcp/internal/tools/toolkit"
	"github.com/asgardeo/mcp/internal/wso2is/catalog"
	"github.com/asgardeo/mcp/internal/wso2is/client"
	"github.com/asgardeo/mcp/internal/wso2is/istest"
	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type testEnv struct {
	srv    *istest.Server
	cat    *catalog.Catalog
	cl     *client.Client
	cfg    *config.Config
	reg    *toolkit.Registry
	mcpCli *mcpclient.Client
}

func setupTestEnv(t *testing.T, customCfg ...func(*config.Config)) *testEnv {
	t.Helper()

	srv := istest.New(t)
	cat, err := catalog.Load(apispecs.FS, "7.3.0")
	if err != nil {
		t.Fatalf("catalog.Load failed: %v", err)
	}

	cfg := &config.Config{
		ServerURL:        srv.URL(),
		TenantDomain:     config.DefaultTenantDomain,
		AuthMode:         config.AuthModeClientCredentials,
		ClientID:         "test-client-id",
		ClientSecret:     "test-client-secret",
		CertPath:         srv.WriteCAPEM(t),
		MaxResponseBytes: config.DefaultMaxResponseBytes,
	}
	for _, fn := range customCfg {
		fn(cfg)
	}

	cl, err := client.New(cfg)
	if err != nil {
		t.Fatalf("client.New failed: %v", err)
	}

	defs, err := idp.Tools(cat, cl, cfg)
	if err != nil {
		t.Fatalf("idp.Tools failed: %v", err)
	}

	reg := toolkit.NewRegistry(cfg.ReadOnly, cfg.MaxResponseBytes)
	for _, def := range defs {
		if err := reg.Register(def); err != nil {
			t.Fatalf("Register(%s) failed: %v", def.Name, err)
		}
	}

	mcpSrv := server.NewMCPServer("wso2is-mcp-test", "1.0.0")
	reg.AddToServer(mcpSrv)

	mcpCli, err := mcpclient.NewInProcessClient(mcpSrv)
	if err != nil {
		t.Fatalf("NewInProcessClient failed: %v", err)
	}
	t.Cleanup(func() { _ = mcpCli.Close() })

	if err := mcpCli.Start(context.Background()); err != nil {
		t.Fatalf("mcpCli.Start failed: %v", err)
	}
	initReq := mcp.InitializeRequest{}
	initReq.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	initReq.Params.ClientInfo = mcp.Implementation{Name: "test-client", Version: "1.0.0"}
	if _, err := mcpCli.Initialize(context.Background(), initReq); err != nil {
		t.Fatalf("mcpCli.Initialize failed: %v", err)
	}

	return &testEnv{srv: srv, cat: cat, cl: cl, cfg: cfg, reg: reg, mcpCli: mcpCli}
}

func (e *testEnv) callTool(t *testing.T, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args
	res, err := e.mcpCli.CallTool(context.Background(), req)
	if err != nil {
		t.Fatalf("CallTool(%q) protocol error: %v", name, err)
	}
	return res
}

func resultText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if len(res.Content) == 0 {
		t.Fatal("expected non-empty content")
	}
	tc, ok := res.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", res.Content[0])
	}
	return tc.Text
}

func decodeResultJSON(t *testing.T, res *mcp.CallToolResult) map[string]any {
	t.Helper()
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", resultText(t, res))
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(resultText(t, res)), &out); err != nil {
		t.Fatalf("unmarshal tool output JSON: %v\nraw:\n%s", err, resultText(t, res))
	}
	return out
}

func lastAPIRequest(t *testing.T, srv *istest.Server) istest.RecordedRequest {
	t.Helper()
	reqs := srv.Requests()
	for i := len(reqs) - 1; i >= 0; i-- {
		if reqs[i].Path != "/oauth2/token" && reqs[i].Path != "/oauth2/jwks" {
			return reqs[i]
		}
	}
	t.Fatal("expected at least one non-token IS request")
	return istest.RecordedRequest{}
}

func TestCatalogValidationAndAnnotations(t *testing.T) {
	env := setupTestEnv(t)

	want := []string{
		"list_identity_providers",
		"get_identity_provider",
		"create_identity_provider",
		"delete_identity_provider",
		"list_authenticators",
	}
	got := env.reg.Names()
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("registered tools = %v, want %v", got, want)
	}

	kinds := map[string]struct {
		readOnly    bool
		destructive bool
	}{
		"list_identity_providers":  {readOnly: true, destructive: false},
		"get_identity_provider":    {readOnly: true, destructive: false},
		"create_identity_provider": {readOnly: false, destructive: false},
		"delete_identity_provider": {readOnly: false, destructive: true},
		"list_authenticators":      {readOnly: true, destructive: false},
	}

	for _, st := range env.reg.ServerTools() {
		exp, ok := kinds[st.Tool.Name]
		if !ok {
			t.Fatalf("unexpected tool %q", st.Tool.Name)
		}
		ann := st.Tool.Annotations
		if ann.ReadOnlyHint == nil || *ann.ReadOnlyHint != exp.readOnly {
			t.Errorf("%s ReadOnlyHint = %v, want %v", st.Tool.Name, ann.ReadOnlyHint, exp.readOnly)
		}
		if ann.DestructiveHint == nil || *ann.DestructiveHint != exp.destructive {
			t.Errorf("%s DestructiveHint = %v, want %v", st.Tool.Name, ann.DestructiveHint, exp.destructive)
		}
	}

	// Empty catalog must fail.
	if _, err := idp.Tools(&catalog.Catalog{}, env.cl, env.cfg); err == nil {
		t.Fatal("expected error for empty catalog")
	}

	// Read-only mode registers only 3 read tools.
	roEnv := setupTestEnv(t, func(cfg *config.Config) { cfg.ReadOnly = true })
	if roEnv.reg.Count() != 3 {
		t.Fatalf("READ_ONLY=true count = %d, want 3 (%v)", roEnv.reg.Count(), roEnv.reg.Names())
	}
}

func TestListAndGetIdentityProviders(t *testing.T) {
	env := setupTestEnv(t)

	env.srv.HandleJSON(http.MethodGet, "/api/server/v1/identity-providers", http.StatusOK, map[string]any{
		"totalResults": 1,
		"identityProviders": []any{
			map[string]any{"id": "idp-1", "name": "Google"},
		},
	})
	env.srv.HandleJSON(http.MethodGet, "/api/server/v1/identity-providers/idp-1", http.StatusOK, map[string]any{
		"id":   "idp-1",
		"name": "Google",
		"federatedAuthenticators": map[string]any{
			"defaultAuthenticatorId": "R29vZ2xlT0lEQ0F1dGhlbnRpY2F0b3I",
			"authenticators": []any{
				map[string]any{
					"authenticatorId": "R29vZ2xlT0lEQ0F1dGhlbnRpY2F0b3I",
					"properties": []any{
						map[string]any{"key": "ClientId", "value": "cid-1"},
						map[string]any{"key": "ClientSecret", "value": "super-secret-value"},
					},
				},
			},
		},
	})

	listRes := env.callTool(t, "list_identity_providers", map[string]any{"limit": 10, "filter": "name co Google"})
	listOut := decodeResultJSON(t, listRes)
	if listOut["totalResults"] != float64(1) {
		t.Fatalf("totalResults = %v", listOut["totalResults"])
	}
	req := lastAPIRequest(t, env.srv)
	if !strings.Contains(req.RawQuery, "limit=10") {
		t.Fatalf("expected limit=10 in query, got %q", req.RawQuery)
	}

	getRes := env.callTool(t, "get_identity_provider", map[string]any{"identity_provider_id": "idp-1"})
	text := resultText(t, getRes)
	if strings.Contains(text, "super-secret-value") {
		t.Fatal("ClientSecret leaked in get_identity_provider response")
	}
	if !strings.Contains(text, "[REDACTED]") {
		t.Fatalf("expected [REDACTED] placeholder in response: %s", text)
	}
}

func TestCreateIdentityProviderTemplatesAndRedaction(t *testing.T) {
	env := setupTestEnv(t)

	templates := []struct {
		name string
		args map[string]any
	}{
		{
			name: "google",
			args: map[string]any{"name": "Google IdP", "template": "google", "client_id": "g-cid", "client_secret": "g-secret-123"},
		},
		{
			name: "github",
			args: map[string]any{"name": "GitHub IdP", "template": "github", "client_id": "gh-cid", "client_secret": "gh-secret-123"},
		},
		{
			name: "facebook",
			args: map[string]any{"name": "Facebook IdP", "template": "facebook", "client_id": "fb-cid", "client_secret": "fb-secret-123"},
		},
		{
			name: "microsoft",
			args: map[string]any{"name": "Microsoft IdP", "template": "microsoft", "client_id": "ms-cid", "client_secret": "ms-secret-123"},
		},
		{
			name: "apple",
			args: map[string]any{"name": "Apple IdP", "template": "apple", "client_id": "ap-cid", "team_id": "T1", "key_id": "K1", "private_key": "ap-priv-123"},
		},
		{
			name: "oidc",
			args: map[string]any{
				"name":                   "Custom OIDC",
				"template":               "oidc",
				"client_id":              "oidc-cid",
				"client_secret":          "oidc-secret-123",
				"authorization_endpoint": "https://idp.example.com/authorize",
				"token_endpoint":         "https://idp.example.com/token",
			},
		},
		{
			name: "saml",
			args: map[string]any{
				"name":          "Corp SAML",
				"template":      "saml",
				"idp_entity_id": "https://idp.example.com/entity",
				"sso_url":       "https://idp.example.com/sso",
			},
		},
	}

	for _, tc := range templates {
		t.Run(tc.name, func(t *testing.T) {
			env.srv.ResetRequests()
			env.srv.Handle(http.MethodPost, "/api/server/v1/identity-providers", func(w http.ResponseWriter, r *http.Request) {
				var body map[string]any
				_ = json.NewDecoder(r.Body).Decode(&body)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				body["id"] = "created-idp-1"
				_ = json.NewEncoder(w).Encode(body)
			})

			res := env.callTool(t, "create_identity_provider", tc.args)
			text := resultText(t, res)
			if res.IsError {
				t.Fatalf("create_identity_provider(%s) failed: %s", tc.name, text)
			}
			for _, secret := range []string{"g-secret-123", "gh-secret-123", "fb-secret-123", "ms-secret-123", "ap-priv-123", "oidc-secret-123"} {
				if strings.Contains(text, secret) {
					t.Fatalf("secret %q leaked in response for %s: %s", secret, tc.name, text)
				}
			}

			req := lastAPIRequest(t, env.srv)
			var sent map[string]any
			if err := json.Unmarshal(req.Body, &sent); err != nil {
				t.Fatalf("unmarshal sent body: %v", err)
			}
			if sent["name"] != tc.args["name"] {
				t.Fatalf("sent name = %v, want %v", sent["name"], tc.args["name"])
			}
			fa, _ := sent["federatedAuthenticators"].(map[string]any)
			if fa == nil || fa["defaultAuthenticatorId"] == "" {
				t.Fatalf("missing federatedAuthenticators.defaultAuthenticatorId: %#v", sent)
			}
		})
	}
}

func TestDeleteIdentityProviderAndListAuthenticatorsAndOrg(t *testing.T) {
	env := setupTestEnv(t)

	env.srv.HandleJSON(http.MethodDelete, "/api/server/v1/identity-providers/idp-1", http.StatusNoContent, nil)
	env.srv.HandleJSON(http.MethodGet, "/api/server/v1/authenticators", http.StatusOK, []any{
		map[string]any{"id": "QmFzaWNBdXRoZW50aWNhdG9y", "name": "BasicAuthenticator"},
	})
	env.srv.HandleJSON(http.MethodGet, "/o/api/server/v1/identity-providers", http.StatusOK, map[string]any{
		"totalResults":      0,
		"identityProviders": []any{},
	})

	delRes := env.callTool(t, "delete_identity_provider", map[string]any{"identity_provider_id": "idp-1"})
	if delRes.IsError {
		t.Fatalf("delete_identity_provider failed: %s", resultText(t, delRes))
	}

	authRes := env.callTool(t, "list_authenticators", map[string]any{"limit": 20})
	if authRes.IsError {
		t.Fatalf("list_authenticators failed: %s", resultText(t, authRes))
	}

	const orgID = "org-123"
	orgRes := env.callTool(t, "list_identity_providers", map[string]any{"organization_id": orgID})
	if orgRes.IsError {
		t.Fatalf("org list_identity_providers failed: %s", resultText(t, orgRes))
	}
	req := lastAPIRequest(t, env.srv)
	if req.Path != "/o/api/server/v1/identity-providers" {
		t.Fatalf("org path = %q, want /o/api/server/v1/identity-providers", req.Path)
	}
	if !strings.HasPrefix(req.Headers.Get("Authorization"), "Bearer org-token-"+orgID+"-") {
		t.Fatalf("expected org token, got %q", req.Headers.Get("Authorization"))
	}
}
