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

package applications_test

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"

	apispecs "github.com/asgardeo/mcp/api-specs"
	"github.com/asgardeo/mcp/internal/config"
	"github.com/asgardeo/mcp/internal/tools/admin/applications"
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

	defs, err := applications.Tools(cat, cl, cfg)
	if err != nil {
		t.Fatalf("applications.Tools failed: %v", err)
	}

	reg := toolkit.NewRegistry(cfg.ReadOnly, cfg.MaxResponseBytes)
	reg.MustRegister(defs...)

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

	return &testEnv{
		srv:    srv,
		cat:    cat,
		cl:     cl,
		cfg:    cfg,
		reg:    reg,
		mcpCli: mcpCli,
	}
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
		t.Fatal("empty tool result content")
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
		t.Fatalf("unmarshal result JSON: %v\nraw: %s", err, resultText(t, res))
	}
	return out
}

func decodeResultArray(t *testing.T, res *mcp.CallToolResult) []any {
	t.Helper()
	if res.IsError {
		t.Fatalf("unexpected tool error: %s", resultText(t, res))
	}
	var out []any
	if err := json.Unmarshal([]byte(resultText(t, res)), &out); err != nil {
		t.Fatalf("unmarshal result JSON array: %v\nraw: %s", err, resultText(t, res))
	}
	return out
}

func apiRequests(srv *istest.Server) []istest.RecordedRequest {
	var out []istest.RecordedRequest
	for _, r := range srv.Requests() {
		if strings.HasSuffix(r.Path, "/oauth2/token") || strings.HasSuffix(r.Path, "/oauth2/jwks") {
			continue
		}
		out = append(out, r)
	}
	return out
}

func lastAPIRequest(t *testing.T, srv *istest.Server) istest.RecordedRequest {
	t.Helper()
	reqs := apiRequests(srv)
	if len(reqs) == 0 {
		t.Fatal("expected at least one IS API request")
	}
	return reqs[len(reqs)-1]
}

func TestToolsCatalogLookupAndConventions(t *testing.T) {
	t.Run("missing catalog operation fails", func(t *testing.T) {
		emptyFS := fstest.MapFS{
			"wso2is/7.3.0/SOURCE":     &fstest.MapFile{Data: []byte("repository: test\ncommit: abc\n")},
			"wso2is/7.3.0/root/.keep": &fstest.MapFile{Data: []byte("")},
			"wso2is/7.3.0/org/.keep":  &fstest.MapFile{Data: []byte("")},
		}
		emptyCat, err := catalog.Load(fs.FS(emptyFS), "7.3.0")
		if err != nil {
			t.Fatalf("catalog.Load empty failed: %v", err)
		}
		srv := istest.New(t)
		cfg := &config.Config{ServerURL: srv.URL(), TenantDomain: config.DefaultTenantDomain}
		cl, err := client.New(cfg)
		if err != nil {
			t.Fatalf("client.New failed: %v", err)
		}
		if _, err := applications.Tools(emptyCat, cl, cfg); err == nil {
			t.Fatal("expected error when catalog operations are missing")
		}
	})

	t.Run("registers all 15 application tools with proper annotations", func(t *testing.T) {
		env := setupTestEnv(t)
		want := []string{
			"list_applications",
			"create_single_page_app",
			"create_webapp_with_ssr",
			"create_mobile_app",
			"create_m2m_app",
			"get_application_by_name",
			"get_application_by_client_id",
			"update_application_basic_info",
			"update_application_oauth_config",
			"update_application_claim_config",
			"authorize_api",
			"list_authorized_api",
			"update_login_flow",
			"delete_application",
			"share_application",
		}
		got := env.reg.Names()
		slices.Sort(got)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Fatalf("registered tools = %v, want %v", got, want)
		}

		readOnly := map[string]bool{
			"list_applications":            true,
			"get_application_by_name":      true,
			"get_application_by_client_id": true,
			"list_authorized_api":          true,
		}
		destructive := map[string]bool{
			"delete_application": true,
		}

		for _, st := range env.reg.ServerTools() {
			ann := st.Tool.Annotations
			isRead := ann.ReadOnlyHint != nil && *ann.ReadOnlyHint
			isDestr := ann.DestructiveHint != nil && *ann.DestructiveHint
			if isRead != readOnly[st.Tool.Name] {
				t.Errorf("%s ReadOnlyHint = %v, want %v", st.Tool.Name, isRead, readOnly[st.Tool.Name])
			}
			if isDestr != destructive[st.Tool.Name] {
				t.Errorf("%s DestructiveHint = %v, want %v", st.Tool.Name, isDestr, destructive[st.Tool.Name])
			}
		}
	})

	t.Run("read_only mode registers only 4 read tools", func(t *testing.T) {
		env := setupTestEnv(t, func(cfg *config.Config) {
			cfg.ReadOnly = true
		})
		got := env.reg.Names()
		slices.Sort(got)
		want := []string{
			"get_application_by_client_id",
			"get_application_by_name",
			"list_applications",
			"list_authorized_api",
		}
		if !slices.Equal(got, want) {
			t.Fatalf("READ_ONLY tools = %v, want %v", got, want)
		}
	})
}

func TestListApplications(t *testing.T) {
	env := setupTestEnv(t)
	env.srv.HandleJSON(http.MethodGet, "/api/server/v1/applications", http.StatusOK, map[string]any{
		"totalResults": 2,
		"applications": []any{
			map[string]any{"id": "app-1", "name": "Demo SPA"},
			map[string]any{"id": "app-2", "name": "Backend M2M"},
		},
	})

	res := env.callTool(t, "list_applications", nil)
	arr := decodeResultArray(t, res)
	if len(arr) != 2 {
		t.Fatalf("expected 2 apps, got %d", len(arr))
	}
	first := arr[0].(map[string]any)
	if first["id"] != "app-1" || first["name"] != "Demo SPA" {
		t.Fatalf("unexpected first item: %#v", first)
	}
}

func TestCreateApplicationsAndTenantOAuthEndpoints(t *testing.T) {
	t.Run("create_single_page_app with tenant domain", func(t *testing.T) {
		env := setupTestEnv(t, func(cfg *config.Config) {
			cfg.TenantDomain = "wso2.com"
		})
		env.srv.Handle(http.MethodPost, "/t/wso2.com/api/server/v1/applications", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Location", env.srv.URL()+"/t/wso2.com/api/server/v1/applications/spa-1")
			w.WriteHeader(http.StatusCreated)
		})
		env.srv.HandleJSON(http.MethodGet, "/t/wso2.com/api/server/v1/applications", http.StatusOK, map[string]any{
			"applications": []any{
				map[string]any{"id": "spa-1", "name": "MySPA", "clientId": "cid-spa-1"},
			},
		})
		env.srv.HandleJSON(http.MethodGet, "/t/wso2.com/api/server/v1/applications/spa-1", http.StatusOK, map[string]any{
			"id":       "spa-1",
			"name":     "MySPA",
			"clientId": "cid-spa-1",
		})

		res := env.callTool(t, "create_single_page_app", map[string]any{
			"application_name": "MySPA",
			"redirect_url":     "https://app.example.com/cb",
		})
		out := decodeResultJSON(t, res)
		appCfg := out["application_configurations"].(map[string]any)
		if appCfg["id"] != "spa-1" || appCfg["client_id"] != "cid-spa-1" || appCfg["application_type"] != "spa" || appCfg["redirect_url"] != "https://app.example.com/cb" {
			t.Fatalf("unexpected application_configurations: %#v", appCfg)
		}
		endpoints := out["oauth_endpoints"].(map[string]any)
		wantBase := env.srv.URL() + "/t/wso2.com"
		if endpoints["base_url"] != wantBase || endpoints["authorize_url"] != wantBase+"/oauth2/authorize" || endpoints["token_url"] != wantBase+"/oauth2/token" {
			t.Fatalf("unexpected oauth_endpoints: %#v", endpoints)
		}
	})

	t.Run("create_webapp_with_ssr redacts client secret", func(t *testing.T) {
		env := setupTestEnv(t)
		env.srv.Handle(http.MethodPost, "/api/server/v1/applications", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
		})
		env.srv.HandleJSON(http.MethodGet, "/api/server/v1/applications", http.StatusOK, map[string]any{
			"applications": []any{
				map[string]any{"id": "web-1", "name": "MyWeb"},
			},
		})
		env.srv.HandleJSON(http.MethodGet, "/api/server/v1/applications/web-1/inbound-protocols/oidc", http.StatusOK, map[string]any{
			"clientId":     "cid-web-1",
			"clientSecret": "super-secret-web-value",
			"grantTypes":   []string{"authorization_code"},
		})

		res := env.callTool(t, "create_webapp_with_ssr", map[string]any{
			"application_name": "MyWeb",
			"redirect_url":     "https://web.example.com/cb",
		})
		raw := resultText(t, res)
		if strings.Contains(raw, "super-secret-web-value") {
			t.Fatalf("client secret leaked in tool output: %s", raw)
		}
		out := decodeResultJSON(t, res)
		appCfg := out["application_configurations"].(map[string]any)
		if appCfg["id"] != "web-1" || appCfg["client_id"] != "cid-web-1" || appCfg["application_type"] != "ssr_web" {
			t.Fatalf("unexpected application_configurations: %#v", appCfg)
		}
	})

	t.Run("create_mobile_app", func(t *testing.T) {
		env := setupTestEnv(t)
		env.srv.Handle(http.MethodPost, "/api/server/v1/applications", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
		})
		env.srv.HandleJSON(http.MethodGet, "/api/server/v1/applications", http.StatusOK, map[string]any{
			"applications": []any{
				map[string]any{"id": "mob-1", "name": "MyMobile"},
			},
		})
		env.srv.HandleJSON(http.MethodGet, "/api/server/v1/applications/mob-1", http.StatusOK, map[string]any{
			"id":       "mob-1",
			"name":     "MyMobile",
			"clientId": "cid-mob-1",
		})

		res := env.callTool(t, "create_mobile_app", map[string]any{
			"application_name": "MyMobile",
			"redirect_url":     "myapp://callback",
		})
		out := decodeResultJSON(t, res)
		appCfg := out["application_configurations"].(map[string]any)
		if appCfg["id"] != "mob-1" || appCfg["client_id"] != "cid-mob-1" || appCfg["application_type"] != "mobile" {
			t.Fatalf("unexpected application_configurations: %#v", appCfg)
		}
	})

	t.Run("create_m2m_app omits authorize_url and redacts secret", func(t *testing.T) {
		env := setupTestEnv(t)
		env.srv.Handle(http.MethodPost, "/api/server/v1/applications", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
		})
		env.srv.HandleJSON(http.MethodGet, "/api/server/v1/applications", http.StatusOK, map[string]any{
			"applications": []any{
				map[string]any{"id": "m2m-1", "name": "MyM2M"},
			},
		})
		env.srv.HandleJSON(http.MethodGet, "/api/server/v1/applications/m2m-1/inbound-protocols/oidc", http.StatusOK, map[string]any{
			"clientId":     "cid-m2m-1",
			"clientSecret": "super-secret-m2m",
			"grantTypes":   []string{"client_credentials"},
		})

		res := env.callTool(t, "create_m2m_app", map[string]any{
			"application_name": "MyM2M",
		})
		raw := resultText(t, res)
		if strings.Contains(raw, "super-secret-m2m") {
			t.Fatalf("client secret leaked in tool output: %s", raw)
		}
		out := decodeResultJSON(t, res)
		appCfg := out["application_configurations"].(map[string]any)
		if appCfg["id"] != "m2m-1" || appCfg["client_id"] != "cid-m2m-1" || appCfg["application_type"] != "m2m" {
			t.Fatalf("unexpected application_configurations: %#v", appCfg)
		}
		endpoints := out["oauth_endpoints"].(map[string]any)
		if _, hasAuth := endpoints["authorize_url"]; hasAuth {
			t.Fatalf("m2m oauth_endpoints should not contain authorize_url: %#v", endpoints)
		}
		if endpoints["token_url"] == "" || endpoints["jwks_url"] == "" || endpoints["userinfo_url"] == "" {
			t.Fatalf("missing m2m oauth_endpoints: %#v", endpoints)
		}
	})
}

func TestGetApplicationByNameAndClientID(t *testing.T) {
	env := setupTestEnv(t)
	env.srv.HandleJSON(http.MethodGet, "/api/server/v1/applications", http.StatusOK, map[string]any{
		"applications": []any{
			map[string]any{"id": "app-1", "name": "DemoApp", "clientId": "cid-1", "templateId": "6a90e4b0-fbff-42d7-bfde-1efd98f07cd7"},
		},
	})
	env.srv.HandleJSON(http.MethodGet, "/api/server/v1/applications/app-1", http.StatusOK, map[string]any{
		"id":         "app-1",
		"name":       "DemoApp",
		"clientId":   "cid-1",
		"templateId": "6a90e4b0-fbff-42d7-bfde-1efd98f07cd7",
		"claimConfiguration": map[string]any{
			"requestedClaims": []any{
				map[string]any{"claim": map[string]any{"uri": "http://wso2.org/claims/emailaddress"}},
			},
		},
	})
	env.srv.HandleJSON(http.MethodGet, "/api/server/v1/applications/app-1/inbound-protocols/oidc", http.StatusOK, map[string]any{
		"clientId":     "cid-1",
		"clientSecret": "secret-should-never-appear",
		"callbackURLs": []string{"regexp=(https://a.example.com/cb|https://b.example.com/cb)"},
		"grantTypes":   []string{"authorization_code"},
	})
	env.srv.HandleJSON(http.MethodGet, "/api/server/v1/applications/app-1/authorized-apis", http.StatusOK, []any{
		map[string]any{
			"id":               "api-1",
			"identifier":       "greetings_api",
			"displayName":      "Greetings",
			"policyIdentifier": "RBAC",
			"type":             "BUSINESS",
			"authorizedScopes": []any{
				map[string]any{"id": "s-1", "name": "greetings:read", "displayName": "Read"},
			},
		},
	})
	env.srv.HandleJSON(http.MethodGet, "/api/server/v1/oidc/scopes", http.StatusOK, []any{
		map[string]any{"name": "email", "claims": []string{"email"}},
		map[string]any{"name": "openid", "claims": []string{"sub"}},
	})
	env.srv.HandleJSON(http.MethodGet, "/api/server/v1/claim-dialects/aHR0cDovL3dzbzIub3JnL29pZGMvY2xhaW0/claims", http.StatusOK, []any{
		map[string]any{"claimURI": "email", "mappedLocalClaimURI": "http://wso2.org/claims/emailaddress"},
	})

	for _, tc := range []struct {
		tool string
		args map[string]any
	}{
		{"get_application_by_name", map[string]any{"application_name": "DemoApp"}},
		{"get_application_by_client_id", map[string]any{"client_id": "cid-1"}},
	} {
		t.Run(tc.tool, func(t *testing.T) {
			res := env.callTool(t, tc.tool, tc.args)
			raw := resultText(t, res)
			if strings.Contains(raw, "secret-should-never-appear") {
				t.Fatalf("secret leaked: %s", raw)
			}
			out := decodeResultJSON(t, res)
			appCfg := out["application_configurations"].(map[string]any)
			if appCfg["id"] != "app-1" || appCfg["client_id"] != "cid-1" || appCfg["application_type"] != "spa" {
				t.Fatalf("unexpected application_configurations: %#v", appCfg)
			}
			if appCfg["redirect_url"] != "https://a.example.com/cb,https://b.example.com/cb" {
				t.Fatalf("unexpected redirect_url: %v", appCfg["redirect_url"])
			}
			scopeStr := appCfg["scope"].(string)
			if !strings.Contains(scopeStr, "openid") || !strings.Contains(scopeStr, "email") || !strings.Contains(scopeStr, "greetings:read") {
				t.Fatalf("unexpected scope: %q", scopeStr)
			}
		})
	}
}

func TestUpdateApplicationTools(t *testing.T) {
	env := setupTestEnv(t)

	t.Run("update_application_basic_info", func(t *testing.T) {
		env.srv.HandleJSON(http.MethodPatch, "/api/server/v1/applications/app-1", http.StatusOK, nil)
		res := env.callTool(t, "update_application_basic_info", map[string]any{
			"id":                "app-1",
			"name":              "Renamed",
			"description":       "New desc",
			"image_url":         "https://img.example.com/logo.png",
			"access_url":        "https://app.example.com",
			"logout_return_url": "https://app.example.com/logout",
		})
		if res.IsError {
			t.Fatalf("unexpected error: %s", resultText(t, res))
		}
		req := lastAPIRequest(t, env.srv)
		var body map[string]any
		if err := json.Unmarshal(req.Body, &body); err != nil {
			t.Fatalf("unmarshal patch body: %v", err)
		}
		if body["name"] != "Renamed" || body["imageUrl"] != "https://img.example.com/logo.png" || body["logoutReturnUrl"] != "https://app.example.com/logout" {
			t.Fatalf("unexpected patch body: %#v", body)
		}
	})

	t.Run("update_application_oauth_config", func(t *testing.T) {
		env.srv.HandleJSON(http.MethodGet, "/api/server/v1/applications/app-1/inbound-protocols/oidc", http.StatusOK, map[string]any{
			"clientId":   "cid-1",
			"grantTypes": []string{"authorization_code"},
		})
		env.srv.HandleJSON(http.MethodPut, "/api/server/v1/applications/app-1/inbound-protocols/oidc", http.StatusOK, nil)

		res := env.callTool(t, "update_application_oauth_config", map[string]any{
			"id":                                        "app-1",
			"redirect_urls":                             []any{"https://a.example.com/cb", "https://b.example.com/cb"},
			"allowed_origins":                           []any{"https://a.example.com"},
			"user_access_token_expiry_time":             float64(7200),
			"application_access_token_expiry_time":      float64(1800),
			"refresh_token_expiry_time":                 float64(86400),
			"revoke_tokens_when_idp_session_terminated": true,
			"access_token_attributes":                   []any{"email", "given_name"},
		})
		if res.IsError {
			t.Fatalf("unexpected error: %s", resultText(t, res))
		}
		req := lastAPIRequest(t, env.srv)
		var body map[string]any
		if err := json.Unmarshal(req.Body, &body); err != nil {
			t.Fatalf("unmarshal put body: %v", err)
		}
		cbs := body["callbackURLs"].([]any)
		if len(cbs) != 1 || cbs[0] != "regexp=(https://a.example.com/cb|https://b.example.com/cb)" {
			t.Fatalf("unexpected callbackURLs: %#v", cbs)
		}
		at := body["accessToken"].(map[string]any)
		if at["userAccessTokenExpiryInSeconds"] != float64(7200) || at["revokeTokensWhenIDPSessionTerminated"] != true {
			t.Fatalf("unexpected accessToken: %#v", at)
		}
	})

	t.Run("update_application_claim_config", func(t *testing.T) {
		env.srv.HandleJSON(http.MethodPatch, "/api/server/v1/applications/app-1", http.StatusOK, nil)
		res := env.callTool(t, "update_application_claim_config", map[string]any{
			"id":     "app-1",
			"claims": []any{"http://wso2.org/claims/username", "http://wso2.org/claims/emailaddress"},
		})
		if res.IsError {
			t.Fatalf("unexpected error: %s", resultText(t, res))
		}
		req := lastAPIRequest(t, env.srv)
		var body map[string]any
		if err := json.Unmarshal(req.Body, &body); err != nil {
			t.Fatalf("unmarshal patch body: %v", err)
		}
		cc := body["claimConfiguration"].(map[string]any)
		reqClaims := cc["requestedClaims"].([]any)
		if len(reqClaims) != 2 {
			t.Fatalf("expected 2 requestedClaims, got %d", len(reqClaims))
		}
	})
}

func TestAuthorizeAndListAuthorizedAPI(t *testing.T) {
	env := setupTestEnv(t)
	env.srv.HandleJSON(http.MethodPost, "/api/server/v1/applications/app-1/authorized-apis", http.StatusCreated, nil)
	env.srv.HandleJSON(http.MethodGet, "/api/server/v1/applications/app-1/authorized-apis", http.StatusOK, []any{
		map[string]any{
			"id":          "api-1",
			"identifier":  "greetings_api",
			"displayName": "Greetings",
			"policyId":    "RBAC",
			"type":        "BUSINESS",
			"authorizedScopes": []any{
				map[string]any{"id": "s1", "name": "greetings:read", "displayName": "Read"},
			},
		},
	})

	authRes := env.callTool(t, "authorize_api", map[string]any{
		"appId":            "app-1",
		"id":               "api-1",
		"policyIdentifier": "RBAC",
		"scopes":           []any{"greetings:read"},
	})
	if authRes.IsError {
		t.Fatalf("authorize_api failed: %s", resultText(t, authRes))
	}

	listRes := env.callTool(t, "list_authorized_api", map[string]any{
		"app_id": "app-1",
	})
	arr := decodeResultArray(t, listRes)
	if len(arr) != 1 {
		t.Fatalf("expected 1 authorized API, got %d", len(arr))
	}
	item := arr[0].(map[string]any)
	if item["id"] != "api-1" || item["policy_id"] != "RBAC" {
		t.Fatalf("unexpected authorized API item: %#v", item)
	}
}

func TestUpdateLoginFlow(t *testing.T) {
	env := setupTestEnv(t)

	var statusCalls atomic.Int32
	// Context the tool sends to the login flow AI: authenticators, IdPs and user claims.
	env.srv.HandleJSON(http.MethodGet, "/api/server/v1/authenticators", http.StatusOK, []any{
		map[string]any{"id": "QmFzaWNBdXRoZW50aWNhdG9y", "name": "BasicAuthenticator", "type": "LOCAL", "description": "Username and password"},
		map[string]any{"id": "dG90cA", "name": "totp", "type": "LOCAL", "description": "TOTP"},
	})
	env.srv.HandleJSON(http.MethodGet, "/api/server/v1/identity-providers", http.StatusOK, map[string]any{
		"identityProviders": []any{map[string]any{
			"name": "Google",
			"federatedAuthenticators": map[string]any{
				"defaultAuthenticatorId": "R29vZ2xlT0lEQ0F1dGhlbnRpY2F0b3I",
				"authenticators":         []any{map[string]any{"authenticatorId": "R29vZ2xlT0lEQ0F1dGhlbnRpY2F0b3I", "name": "GoogleOIDCAuthenticator"}},
			},
		}},
	})
	env.srv.HandleJSON(http.MethodGet, "/api/server/v1/claim-dialects/local/claims", http.StatusOK, []any{
		map[string]any{"claimURI": "http://wso2.org/claims/emailaddress", "description": "Email address"},
	})
	env.srv.HandleJSON(http.MethodPost, "/api/server/v1/applications/loginflow/generate", http.StatusOK, map[string]any{
		"operation_id": "op-flow-1",
	})
	env.srv.Handle(http.MethodGet, "/api/server/v1/applications/loginflow/status/op-flow-1", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if statusCalls.Add(1) == 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"operation_id": "op-flow-1",
				"status":       map[string]any{"generating": false},
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"operation_id": "op-flow-1",
			"status":       map[string]any{"generating": true},
		})
	})
	env.srv.HandleJSON(http.MethodGet, "/api/server/v1/applications/loginflow/result/op-flow-1", http.StatusOK, map[string]any{
		"status": "COMPLETED",
		"data": map[string]any{
			"type":            "USER_DEFINED",
			"subjectStepId":   1,
			"attributeStepId": 1,
			"steps": []any{
				map[string]any{
					"id":      1,
					"options": []any{map[string]any{"idp": "LOCAL", "authenticator": "basic"}},
				},
			},
		},
	})
	env.srv.HandleJSON(http.MethodPatch, "/api/server/v1/applications/app-1", http.StatusOK, nil)

	res := env.callTool(t, "update_login_flow", map[string]any{
		"app_id":      "app-1",
		"user_prompt": "Username and password",
	})
	if res.IsError {
		t.Fatalf("update_login_flow failed: %s", resultText(t, res))
	}
	if statusCalls.Load() < 2 {
		t.Fatalf("expected at least 2 status poll calls, got %d", statusCalls.Load())
	}
	req := lastAPIRequest(t, env.srv)
	if req.Method != http.MethodPatch || req.Path != "/api/server/v1/applications/app-1" {
		t.Fatalf("expected final PATCH /applications/app-1, got %s %s", req.Method, req.Path)
	}
	var patch map[string]any
	if err := json.Unmarshal(req.Body, &patch); err != nil {
		t.Fatalf("unmarshal patch: %v", err)
	}
	if patch["authenticationSequence"] == nil {
		t.Fatalf("expected authenticationSequence in patch, got %#v", patch)
	}
}

func TestDeleteAndShareApplication(t *testing.T) {
	env := setupTestEnv(t)

	t.Run("delete_application", func(t *testing.T) {
		env.srv.HandleJSON(http.MethodDelete, "/api/server/v1/applications/app-del", http.StatusNoContent, nil)
		res := env.callTool(t, "delete_application", map[string]any{"id": "app-del"})
		if res.IsError {
			t.Fatalf("delete_application failed: %s", resultText(t, res))
		}
		req := lastAPIRequest(t, env.srv)
		if req.Method != http.MethodDelete || req.Path != "/api/server/v1/applications/app-del" {
			t.Fatalf("unexpected request: %s %s", req.Method, req.Path)
		}
	})

	t.Run("share_application", func(t *testing.T) {
		env.srv.HandleJSON(http.MethodPost, "/api/server/v1/applications/app-share/share", http.StatusOK, nil)
		res := env.callTool(t, "share_application", map[string]any{
			"id":                      "app-share",
			"share_with_all_children": false,
			"shared_organizations":    []any{"org-1", "org-2"},
		})
		if res.IsError {
			t.Fatalf("share_application failed: %s", resultText(t, res))
		}
		req := lastAPIRequest(t, env.srv)
		if req.Method != http.MethodPost || req.Path != "/api/server/v1/applications/app-share/share" {
			t.Fatalf("unexpected request: %s %s", req.Method, req.Path)
		}
		var body map[string]any
		if err := json.Unmarshal(req.Body, &body); err != nil {
			t.Fatalf("unmarshal share body: %v", err)
		}
		orgs := body["sharedOrganizations"].([]any)
		if body["shareWithAllChildren"] != false || len(orgs) != 2 {
			t.Fatalf("unexpected share body: %#v", body)
		}
	})
}

func TestOrgCapableApplicationTools(t *testing.T) {
	env := setupTestEnv(t)
	env.srv.HandleJSON(http.MethodGet, "/o/api/server/v1/applications", http.StatusOK, map[string]any{
		"applications": []any{
			map[string]any{"id": "org-app-1", "name": "OrgApp"},
		},
	})
	env.srv.HandleJSON(http.MethodDelete, "/o/api/server/v1/applications/org-app-1", http.StatusNoContent, nil)

	res := env.callTool(t, "list_applications", map[string]any{"organization_id": "org-acme"})
	arr := decodeResultArray(t, res)
	if len(arr) != 1 || arr[0].(map[string]any)["id"] != "org-app-1" {
		t.Fatalf("unexpected org list_applications result: %#v", arr)
	}
	req := lastAPIRequest(t, env.srv)
	if req.Path != "/o/api/server/v1/applications" {
		t.Fatalf("expected /o/ path, got %s", req.Path)
	}

	delRes := env.callTool(t, "delete_application", map[string]any{"id": "org-app-1", "organization_id": "org-acme"})
	if delRes.IsError {
		t.Fatalf("org delete_application failed: %s", resultText(t, delRes))
	}
	delReq := lastAPIRequest(t, env.srv)
	if delReq.Path != "/o/api/server/v1/applications/org-app-1" {
		t.Fatalf("expected /o/ path on delete, got %s", delReq.Path)
	}
}
