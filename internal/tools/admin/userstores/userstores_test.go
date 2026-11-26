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

package userstores_test

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	apispecs "github.com/asgardeo/mcp/api-specs"
	"github.com/asgardeo/mcp/internal/config"
	"github.com/asgardeo/mcp/internal/tools/admin/userstores"
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

	defs, err := userstores.Tools(cat, cl, cfg)
	if err != nil {
		t.Fatalf("userstores.Tools failed: %v", err)
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

func TestCatalogValidationAndReadOverride(t *testing.T) {
	env := setupTestEnv(t)

	want := []string{
		"list_user_stores",
		"get_user_store",
		"create_user_store",
		"test_user_store_connection",
	}
	got := env.reg.Names()
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("registered tools = %v, want %v", got, want)
	}

	kinds := map[string]bool{
		"list_user_stores":           true,
		"get_user_store":             true,
		"create_user_store":          false,
		"test_user_store_connection": true, // POST in spec, overridden to read!
	}
	for _, st := range env.reg.ServerTools() {
		wantRO := kinds[st.Tool.Name]
		if st.Tool.Annotations.ReadOnlyHint == nil || *st.Tool.Annotations.ReadOnlyHint != wantRO {
			t.Errorf("%s ReadOnlyHint = %v, want %v", st.Tool.Name, st.Tool.Annotations.ReadOnlyHint, wantRO)
		}
		if st.Tool.Annotations.DestructiveHint == nil || *st.Tool.Annotations.DestructiveHint {
			t.Errorf("%s DestructiveHint expected false", st.Tool.Name)
		}
	}

	if _, err := userstores.Tools(&catalog.Catalog{}, env.cl, env.cfg); err == nil {
		t.Fatal("expected error for empty catalog")
	}

	// READ_ONLY=true MUST register test_user_store_connection too (3 read tools).
	roEnv := setupTestEnv(t, func(cfg *config.Config) { cfg.ReadOnly = true })
	if roEnv.reg.Count() != 3 {
		t.Fatalf("READ_ONLY=true count = %d, want 3 (%v)", roEnv.reg.Count(), roEnv.reg.Names())
	}
}

func TestUserStoresCRUDAndTestConnection(t *testing.T) {
	env := setupTestEnv(t)

	env.srv.HandleJSON(http.MethodGet, "/api/server/v1/userstores", http.StatusOK, []any{
		map[string]any{"id": "U0VD", "name": "SEC"},
	})
	env.srv.HandleJSON(http.MethodGet, "/api/server/v1/userstores/U0VD", http.StatusOK, map[string]any{
		"name": "SEC",
		"properties": []any{
			map[string]any{"name": "url", "value": "jdbc:mysql://db:3306/is"},
			map[string]any{"name": "password", "value": "db-secret-pw"},
		},
	})
	env.srv.HandleJSON(http.MethodGet, "/api/server/v1/userstores/meta/types", http.StatusOK, []any{
		map[string]any{
			"typeId":    "VW5pcXVlSURKREJDVXNlclN0b3JlTWFuYWdlcg",
			"typeName":  "UniqueIDJDBCUserStoreManager",
			"className": "org.wso2.carbon.user.core.jdbc.UniqueIDJDBCUserStoreManager",
		},
	})
	env.srv.Handle(http.MethodPost, "/api/server/v1/userstores", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		body["id"] = "U0VD"
		_ = json.NewEncoder(w).Encode(body)
	})
	env.srv.HandleJSON(http.MethodPost, "/api/server/v1/userstores/test-connection", http.StatusOK, map[string]any{
		"connection": true,
	})
	env.srv.HandleJSON(http.MethodGet, "/o/api/server/v1/userstores", http.StatusOK, []any{})

	// 1. list_user_stores
	listRes := env.callTool(t, "list_user_stores", map[string]any{})
	if listRes.IsError {
		t.Fatalf("list_user_stores failed: %s", resultText(t, listRes))
	}

	// 2. get_user_store redacts password
	getRes := env.callTool(t, "get_user_store", map[string]any{"domain_id": "U0VD"})
	getText := resultText(t, getRes)
	if getRes.IsError {
		t.Fatalf("get_user_store failed: %s", getText)
	}
	if strings.Contains(getText, "db-secret-pw") {
		t.Fatal("password leaked in get_user_store response")
	}

	// 3. create_user_store resolves type_name via getAvailableUserStoreTypes and redacts password
	createRes := env.callTool(t, "create_user_store", map[string]any{
		"name":        "SEC",
		"description": "Secondary DB",
		"type_name":   "UniqueIDJDBCUserStoreManager",
		"properties": []any{
			map[string]any{"name": "url", "value": "jdbc:mysql://db:3306/is"},
			map[string]any{"name": "password", "value": "create-secret-pw"},
		},
	})
	createText := resultText(t, createRes)
	if createRes.IsError {
		t.Fatalf("create_user_store failed: %s", createText)
	}
	if strings.Contains(createText, "create-secret-pw") {
		t.Fatal("password leaked in create_user_store response")
	}

	// 4. test_user_store_connection executes POST /userstores/test-connection despite KindRead
	testRes := env.callTool(t, "test_user_store_connection", map[string]any{
		"driver_name":         "com.mysql.cj.jdbc.Driver",
		"connection_url":      "jdbc:mysql://db:3306/is",
		"username":            "admin",
		"connection_password": "conn-secret-pw",
	})
	if testRes.IsError {
		t.Fatalf("test_user_store_connection failed: %s", resultText(t, testRes))
	}
	req := lastAPIRequest(t, env.srv)
	if req.Method != http.MethodPost || req.Path != "/api/server/v1/userstores/test-connection" {
		t.Fatalf("unexpected test-connection request: %s %s", req.Method, req.Path)
	}
	if strings.Contains(resultText(t, testRes), "conn-secret-pw") {
		t.Fatal("connectionPassword leaked in response")
	}

	// 5. org_id support
	const orgID = "org-us-1"
	orgRes := env.callTool(t, "list_user_stores", map[string]any{"organization_id": orgID})
	if orgRes.IsError {
		t.Fatalf("org list_user_stores failed: %s", resultText(t, orgRes))
	}
	orgReq := lastAPIRequest(t, env.srv)
	if orgReq.Path != "/o/api/server/v1/userstores" {
		t.Fatalf("org path = %q", orgReq.Path)
	}
}
