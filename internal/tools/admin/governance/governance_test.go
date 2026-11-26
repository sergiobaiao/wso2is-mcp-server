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

package governance_test

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	apispecs "github.com/asgardeo/mcp/api-specs"
	"github.com/asgardeo/mcp/internal/config"
	"github.com/asgardeo/mcp/internal/tools/admin/governance"
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

	defs, err := governance.Tools(cat, cl, cfg)
	if err != nil {
		t.Fatalf("governance.Tools failed: %v", err)
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

func TestCatalogValidationAndAnnotations(t *testing.T) {
	env := setupTestEnv(t)

	want := []string{
		"list_governance_connectors",
		"get_governance_connector",
		"update_governance_connector",
		"update_validation_rules",
	}
	got := env.reg.Names()
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("registered tools = %v, want %v", got, want)
	}

	kinds := map[string]bool{
		"list_governance_connectors":  true,
		"get_governance_connector":    true,
		"update_governance_connector": false,
		"update_validation_rules":     false,
	}
	for _, st := range env.reg.ServerTools() {
		wantRO := kinds[st.Tool.Name]
		if st.Tool.Annotations.ReadOnlyHint == nil || *st.Tool.Annotations.ReadOnlyHint != wantRO {
			t.Errorf("%s ReadOnlyHint = %v, want %v", st.Tool.Name, st.Tool.Annotations.ReadOnlyHint, wantRO)
		}
	}

	if _, err := governance.Tools(&catalog.Catalog{}, env.cl, env.cfg); err == nil {
		t.Fatal("expected error for empty catalog")
	}

	roEnv := setupTestEnv(t, func(cfg *config.Config) { cfg.ReadOnly = true })
	if roEnv.reg.Count() != 2 {
		t.Fatalf("READ_ONLY=true count = %d, want 2", roEnv.reg.Count())
	}
}

func TestGovernanceConnectorsAndValidationRules(t *testing.T) {
	env := setupTestEnv(t)

	env.srv.HandleJSON(http.MethodGet, "/api/server/v1/identity-governance/", http.StatusOK, []any{
		map[string]any{
			"id":   "cat-1",
			"name": "Account Management Policies",
			"connectors": []any{
				map[string]any{"id": "conn-1", "name": "account.lock.handler"},
			},
		},
	})
	env.srv.HandleJSON(http.MethodGet, "/api/server/v1/identity-governance/cat-1/connectors", http.StatusOK, map[string]any{
		"name": "Account Management Policies",
		"connectors": []any{
			map[string]any{"id": "conn-1", "name": "account.lock.handler"},
		},
	})
	env.srv.HandleJSON(http.MethodGet, "/api/server/v1/identity-governance/cat-1/connectors/conn-1", http.StatusOK, map[string]any{
		"id":   "conn-1",
		"name": "account.lock.handler",
		"properties": []any{
			map[string]any{"name": "account.lock.handler.enable", "value": "true"},
			map[string]any{"name": "secretKey", "value": "secret-gov-val"},
		},
	})
	env.srv.HandleJSON(http.MethodPatch, "/api/server/v1/identity-governance/cat-1/connectors/conn-1", http.StatusOK, nil)
	env.srv.HandleJSON(http.MethodPut, "/api/server/v1/validation-rules", http.StatusOK, nil)
	env.srv.HandleJSON(http.MethodGet, "/o/api/server/v1/identity-governance/", http.StatusOK, []any{})

	// 1. list_governance_connectors (all categories)
	res := env.callTool(t, "list_governance_connectors", map[string]any{})
	if res.IsError {
		t.Fatalf("list_governance_connectors failed: %s", resultText(t, res))
	}
	// 1b. list_governance_connectors (specific category)
	resCat := env.callTool(t, "list_governance_connectors", map[string]any{"category_id": "cat-1"})
	if resCat.IsError {
		t.Fatalf("list_governance_connectors(category_id) failed: %s", resultText(t, resCat))
	}

	// 2. get_governance_connector (redaction check)
	getRes := env.callTool(t, "get_governance_connector", map[string]any{"category_id": "cat-1", "connector_id": "conn-1"})
	getText := resultText(t, getRes)
	if getRes.IsError {
		t.Fatalf("get_governance_connector failed: %s", getText)
	}
	if strings.Contains(getText, "secret-gov-val") {
		t.Fatal("secret leaked in get_governance_connector response")
	}

	// 3. update_governance_connector
	updRes := env.callTool(t, "update_governance_connector", map[string]any{
		"category_id":  "cat-1",
		"connector_id": "conn-1",
		"properties": []any{
			map[string]any{"name": "account.lock.handler.enable", "value": "true"},
		},
	})
	if updRes.IsError {
		t.Fatalf("update_governance_connector failed: %s", resultText(t, updRes))
	}
	req := lastAPIRequest(t, env.srv)
	var patchBody map[string]any
	if err := json.Unmarshal(req.Body, &patchBody); err != nil {
		t.Fatalf("unmarshal patchBody: %v", err)
	}
	if patchBody["operation"] != "UPDATE" {
		t.Fatalf("operation = %v, want UPDATE", patchBody["operation"])
	}

	// 4. update_validation_rules
	valRes := env.callTool(t, "update_validation_rules", map[string]any{
		"rules": []any{
			map[string]any{
				"field": "password",
				"rules": []any{
					map[string]any{
						"validator": "LengthValidator",
						"properties": []any{
							map[string]any{"key": "min.length", "value": "8"},
						},
					},
				},
			},
		},
	})
	if valRes.IsError {
		t.Fatalf("update_validation_rules failed: %s", resultText(t, valRes))
	}
	valReq := lastAPIRequest(t, env.srv)
	if valReq.Method != http.MethodPut || valReq.Path != "/api/server/v1/validation-rules" {
		t.Fatalf("unexpected request: %s %s", valReq.Method, valReq.Path)
	}

	// 5. org_id support
	const orgID = "org-gov-1"
	orgRes := env.callTool(t, "list_governance_connectors", map[string]any{"organization_id": orgID})
	if orgRes.IsError {
		t.Fatalf("org list_governance_connectors failed: %s", resultText(t, orgRes))
	}
	orgReq := lastAPIRequest(t, env.srv)
	if orgReq.Path != "/o/api/server/v1/identity-governance/" {
		t.Fatalf("org path = %q", orgReq.Path)
	}
}
