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

package sessions_test

import (
	"context"
	"net/http"
	"slices"
	"testing"

	apispecs "github.com/asgardeo/mcp/api-specs"
	"github.com/asgardeo/mcp/internal/config"
	"github.com/asgardeo/mcp/internal/tools/admin/sessions"
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

	defs, err := sessions.Tools(cat, cl, cfg)
	if err != nil {
		t.Fatalf("sessions.Tools failed: %v", err)
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

	want := []string{"list_user_sessions", "terminate_user_sessions"}
	got := env.reg.Names()
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		t.Fatalf("registered tools = %v, want %v", got, want)
	}

	for _, st := range env.reg.ServerTools() {
		ann := st.Tool.Annotations
		switch st.Tool.Name {
		case "list_user_sessions":
			if ann.ReadOnlyHint == nil || !*ann.ReadOnlyHint {
				t.Error("list_user_sessions expected ReadOnlyHint=true")
			}
			if ann.DestructiveHint == nil || *ann.DestructiveHint {
				t.Error("list_user_sessions expected DestructiveHint=false")
			}
		case "terminate_user_sessions":
			if ann.ReadOnlyHint == nil || *ann.ReadOnlyHint {
				t.Error("terminate_user_sessions expected ReadOnlyHint=false")
			}
			if ann.DestructiveHint == nil || !*ann.DestructiveHint {
				t.Error("terminate_user_sessions expected DestructiveHint=true")
			}
		}
	}

	if _, err := sessions.Tools(&catalog.Catalog{}, env.cl, env.cfg); err == nil {
		t.Fatal("expected error for empty catalog")
	}

	roEnv := setupTestEnv(t, func(cfg *config.Config) { cfg.ReadOnly = true })
	if roEnv.reg.Count() != 1 {
		t.Fatalf("READ_ONLY=true count = %d, want 1", roEnv.reg.Count())
	}
}

func TestListAndTerminateUserSessions(t *testing.T) {
	env := setupTestEnv(t)

	env.srv.HandleJSON(http.MethodGet, "/api/users/v1/u-123/sessions", http.StatusOK, map[string]any{
		"userId": "u-123",
		"sessions": []any{
			map[string]any{"id": "sess-1"},
		},
	})
	env.srv.HandleJSON(http.MethodDelete, "/api/users/v1/u-123/sessions", http.StatusNoContent, nil)

	listRes := env.callTool(t, "list_user_sessions", map[string]any{"user_id": "u-123", "limit": 5})
	if listRes.IsError {
		t.Fatalf("list_user_sessions failed: %s", resultText(t, listRes))
	}
	req := lastAPIRequest(t, env.srv)
	if req.Method != http.MethodGet || req.Path != "/api/users/v1/u-123/sessions" {
		t.Fatalf("unexpected request: %s %s", req.Method, req.Path)
	}

	termRes := env.callTool(t, "terminate_user_sessions", map[string]any{"user_id": "u-123"})
	if termRes.IsError {
		t.Fatalf("terminate_user_sessions failed: %s", resultText(t, termRes))
	}
	termReq := lastAPIRequest(t, env.srv)
	if termReq.Method != http.MethodDelete || termReq.Path != "/api/users/v1/u-123/sessions" {
		t.Fatalf("unexpected request: %s %s", termReq.Method, termReq.Path)
	}
}
