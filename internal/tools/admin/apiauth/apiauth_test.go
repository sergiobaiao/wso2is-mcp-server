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

package apiauth_test

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	apispecs "github.com/asgardeo/mcp/api-specs"
	"github.com/asgardeo/mcp/internal/config"
	"github.com/asgardeo/mcp/internal/tools/admin/apiauth"
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

	defs, err := apiauth.Tools(cat, cl, cfg)
	if err != nil {
		t.Fatalf("apiauth.Tools failed: %v", err)
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

func TestAPIAuthToolsRegistrationAndCatalogLookup(t *testing.T) {
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
		if _, err := apiauth.Tools(emptyCat, cl, cfg); err == nil {
			t.Fatal("expected error when catalog operations are missing")
		}
	})

	t.Run("registers all 9 tools with proper kinds", func(t *testing.T) {
		env := setupTestEnv(t)
		want := []string{
			"list_api_resources",
			"search_api_resources_by_name",
			"get_api_resource_by_identifier",
			"create_api_resource",
			"update_api_resource",
			"add_api_resource_scopes",
			"delete_api_resource",
			"list_oidc_scopes",
			"create_oidc_scope",
		}
		got := env.reg.Names()
		slices.Sort(got)
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Fatalf("registered tools = %v, want %v", got, want)
		}

		readOnly := map[string]bool{
			"list_api_resources":             true,
			"search_api_resources_by_name":   true,
			"get_api_resource_by_identifier": true,
			"list_oidc_scopes":               true,
		}
		destructive := map[string]bool{
			"delete_api_resource": true,
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
			"get_api_resource_by_identifier",
			"list_api_resources",
			"list_oidc_scopes",
			"search_api_resources_by_name",
		}
		if !slices.Equal(got, want) {
			t.Fatalf("READ_ONLY tools = %v, want %v", got, want)
		}
	})
}

func TestListSearchGetAPIResources(t *testing.T) {
	env := setupTestEnv(t)

	env.srv.HandleJSON(http.MethodGet, "/api/server/v1/api-resources", http.StatusOK, map[string]any{
		"totalResults": 1,
		"APIResources": []any{
			map[string]any{
				"id":                    "api-1",
				"name":                  "Payments API",
				"identifier":            "payments_api",
				"type":                  "BUSINESS",
				"requiresAuthorization": true,
			},
		},
	})

	t.Run("list_api_resources", func(t *testing.T) {
		res := env.callTool(t, "list_api_resources", map[string]any{"limit": float64(25)})
		arr := decodeResultArray(t, res)
		if len(arr) != 1 {
			t.Fatalf("expected 1 item, got %d", len(arr))
		}
		item := arr[0].(map[string]any)
		if item["id"] != "api-1" || item["name"] != "Payments API" || item["requiresAuthorization"] != true {
			t.Fatalf("unexpected item: %#v", item)
		}
		req := lastAPIRequest(t, env.srv)
		if !strings.Contains(req.RawQuery, "limit=25") {
			t.Fatalf("expected limit=25 in query, got %q", req.RawQuery)
		}
	})

	t.Run("search_api_resources_by_name", func(t *testing.T) {
		res := env.callTool(t, "search_api_resources_by_name", map[string]any{"name": "Payments API"})
		arr := decodeResultArray(t, res)
		if len(arr) != 1 {
			t.Fatalf("expected 1 item, got %d", len(arr))
		}
		req := lastAPIRequest(t, env.srv)
		if !strings.Contains(req.RawQuery, "filter=name+eq+Payments+API") && !strings.Contains(req.RawQuery, "filter=name%20eq%20Payments%20API") {
			t.Fatalf("expected filter in query, got %q", req.RawQuery)
		}
	})

	t.Run("get_api_resource_by_identifier", func(t *testing.T) {
		res := env.callTool(t, "get_api_resource_by_identifier", map[string]any{"identifier": "payments_api"})
		out := decodeResultJSON(t, res)
		if out["id"] != "api-1" || out["name"] != "Payments API" {
			t.Fatalf("unexpected item: %#v", out)
		}
	})

	t.Run("org-capable list_api_resources", func(t *testing.T) {
		env.srv.HandleJSON(http.MethodGet, "/o/api/server/v1/api-resources", http.StatusOK, map[string]any{
			"APIResources": []any{
				map[string]any{"id": "org-api-1", "name": "Org API"},
			},
		})
		res := env.callTool(t, "list_api_resources", map[string]any{"organization_id": "org-1"})
		arr := decodeResultArray(t, res)
		if len(arr) != 1 || arr[0].(map[string]any)["id"] != "org-api-1" {
			t.Fatalf("unexpected org result: %#v", arr)
		}
	})
}

func TestCreateUpdateDeleteAPIResourceAndScopes(t *testing.T) {
	env := setupTestEnv(t)

	t.Run("create_api_resource accepts string and object scopes", func(t *testing.T) {
		env.srv.HandleJSON(http.MethodPost, "/api/server/v1/api-resources", http.StatusCreated, map[string]any{
			"id":         "api-new",
			"name":       "Greetings API",
			"identifier": "greetings_api",
		})

		res := env.callTool(t, "create_api_resource", map[string]any{
			"identifier":            "greetings_api",
			"name":                  "Greetings API",
			"requiresAuthorization": true,
			"scopes": []any{
				"greetings:read",
				map[string]any{"name": "greetings:write", "displayName": "Write", "description": "Write greetings"},
			},
		})
		out := decodeResultJSON(t, res)
		if out["id"] != "api-new" {
			t.Fatalf("unexpected create response: %#v", out)
		}
		req := lastAPIRequest(t, env.srv)
		var body map[string]any
		if err := json.Unmarshal(req.Body, &body); err != nil {
			t.Fatalf("unmarshal create body: %v", err)
		}
		scopes := body["scopes"].([]any)
		if len(scopes) != 2 {
			t.Fatalf("expected 2 scopes in body, got %d", len(scopes))
		}
	})

	t.Run("update_api_resource", func(t *testing.T) {
		env.srv.HandleJSON(http.MethodPatch, "/api/server/v1/api-resources/api-new", http.StatusNoContent, nil)
		res := env.callTool(t, "update_api_resource", map[string]any{
			"id":          "api-new",
			"name":        "Greetings v2",
			"description": "Updated",
			"added_scopes": []any{
				map[string]any{"name": "greetings:admin", "displayName": "Admin"},
			},
		})
		if res.IsError {
			t.Fatalf("update_api_resource failed: %s", resultText(t, res))
		}
		req := lastAPIRequest(t, env.srv)
		if req.Method != http.MethodPatch || req.Path != "/api/server/v1/api-resources/api-new" {
			t.Fatalf("unexpected request: %s %s", req.Method, req.Path)
		}
		var body map[string]any
		if err := json.Unmarshal(req.Body, &body); err != nil {
			t.Fatalf("unmarshal patch body: %v", err)
		}
		if body["name"] != "Greetings v2" || len(body["addedScopes"].([]any)) != 1 {
			t.Fatalf("unexpected patch body: %#v", body)
		}
	})

	t.Run("add_api_resource_scopes merges with existing scopes", func(t *testing.T) {
		env.srv.HandleJSON(http.MethodGet, "/api/server/v1/api-resources/api-new/scopes", http.StatusOK, []any{
			map[string]any{"id": "s-1", "name": "greetings:read", "displayName": "Read", "description": "Read desc"},
		})
		env.srv.HandleJSON(http.MethodPut, "/api/server/v1/api-resources/api-new/scopes", http.StatusNoContent, nil)

		res := env.callTool(t, "add_api_resource_scopes", map[string]any{
			"id": "api-new",
			"scopes": []any{
				map[string]any{"name": "greetings:write", "displayName": "Write", "description": "Write desc"},
			},
		})
		if res.IsError {
			t.Fatalf("add_api_resource_scopes failed: %s", resultText(t, res))
		}
		req := lastAPIRequest(t, env.srv)
		if req.Method != http.MethodPut || req.Path != "/api/server/v1/api-resources/api-new/scopes" {
			t.Fatalf("unexpected request: %s %s", req.Method, req.Path)
		}
		var putScopes []map[string]any
		if err := json.Unmarshal(req.Body, &putScopes); err != nil {
			t.Fatalf("unmarshal PUT scopes: %v", err)
		}
		if len(putScopes) != 2 || putScopes[0]["name"] != "greetings:read" || putScopes[1]["name"] != "greetings:write" {
			t.Fatalf("expected merged scopes [greetings:read, greetings:write], got %#v", putScopes)
		}
	})

	t.Run("delete_api_resource", func(t *testing.T) {
		env.srv.HandleJSON(http.MethodDelete, "/api/server/v1/api-resources/api-new", http.StatusNoContent, nil)
		res := env.callTool(t, "delete_api_resource", map[string]any{"id": "api-new"})
		if res.IsError {
			t.Fatalf("delete_api_resource failed: %s", resultText(t, res))
		}
		req := lastAPIRequest(t, env.srv)
		if req.Method != http.MethodDelete || req.Path != "/api/server/v1/api-resources/api-new" {
			t.Fatalf("unexpected request: %s %s", req.Method, req.Path)
		}
	})
}

func TestOIDCScopes(t *testing.T) {
	env := setupTestEnv(t)

	env.srv.HandleJSON(http.MethodGet, "/api/server/v1/oidc/scopes", http.StatusOK, []any{
		map[string]any{"name": "openid", "displayName": "OpenID", "claims": []string{"sub"}},
	})
	env.srv.HandleJSON(http.MethodPost, "/api/server/v1/oidc/scopes", http.StatusCreated, nil)

	listRes := env.callTool(t, "list_oidc_scopes", nil)
	arr := decodeResultArray(t, listRes)
	if len(arr) != 1 || arr[0].(map[string]any)["name"] != "openid" {
		t.Fatalf("unexpected list_oidc_scopes output: %#v", arr)
	}

	createRes := env.callTool(t, "create_oidc_scope", map[string]any{
		"name":         "custom_scope",
		"display_name": "Custom Scope",
		"description":  "Custom OIDC scope",
		"claims":       []any{"email", "profile"},
	})
	if createRes.IsError {
		t.Fatalf("create_oidc_scope failed: %s", resultText(t, createRes))
	}
	req := lastAPIRequest(t, env.srv)
	if req.Method != http.MethodPost || req.Path != "/api/server/v1/oidc/scopes" {
		t.Fatalf("unexpected request: %s %s", req.Method, req.Path)
	}
	var body map[string]any
	if err := json.Unmarshal(req.Body, &body); err != nil {
		t.Fatalf("unmarshal create_oidc_scope body: %v", err)
	}
	if body["name"] != "custom_scope" || body["displayName"] != "Custom Scope" || len(body["claims"].([]any)) != 2 {
		t.Fatalf("unexpected create_oidc_scope body: %#v", body)
	}
}
