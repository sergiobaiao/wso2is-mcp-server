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

package catalog

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"

	apispecs "github.com/asgardeo/mcp/api-specs"
	"github.com/asgardeo/mcp/internal/config"
	"github.com/asgardeo/mcp/internal/tools/toolkit"
	iscatalog "github.com/asgardeo/mcp/internal/wso2is/catalog"
	"github.com/asgardeo/mcp/internal/wso2is/client"
	"github.com/asgardeo/mcp/internal/wso2is/istest"
	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

var (
	cached730Once sync.Once
	cached730Cat  *iscatalog.Catalog
	cached730Err  error
)

func load730Catalog(t *testing.T) *iscatalog.Catalog {
	t.Helper()
	cached730Once.Do(func() {
		cached730Cat, cached730Err = iscatalog.Load(apispecs.FS, "7.3.0")
	})
	if cached730Err != nil {
		t.Fatalf("load 7.3.0 catalog: %v", cached730Err)
	}
	return cached730Cat
}

type funcElicitHandler struct {
	fn func(ctx context.Context, req mcp.ElicitationRequest) (*mcp.ElicitationResult, error)
}

func (h *funcElicitHandler) Elicit(ctx context.Context, req mcp.ElicitationRequest) (*mcp.ElicitationResult, error) {
	return h.fn(ctx, req)
}

type testHarness struct {
	srv       *istest.Server
	isClient  *client.Client
	reg       *toolkit.Registry
	mcpSrv    *server.MCPServer
	mcpClient *mcpclient.Client
}

func newTestHarness(t *testing.T, cat *iscatalog.Catalog, cfg *config.Config, elicitFn func(ctx context.Context, req mcp.ElicitationRequest) (*mcp.ElicitationResult, error)) *testHarness {
	t.Helper()
	srv := istest.New(t)
	if cfg == nil {
		cfg = &config.Config{}
	}
	cfgCopy := *cfg
	cfgCopy.ServerURL = srv.URL()
	if cfgCopy.TenantDomain == "" {
		cfgCopy.TenantDomain = config.DefaultTenantDomain
	}
	if cfgCopy.AuthMode == "" {
		cfgCopy.AuthMode = config.AuthModeClientCredentials
	}
	if cfgCopy.ClientID == "" {
		cfgCopy.ClientID = "test-client-id"
	}
	if cfgCopy.ClientSecret == "" {
		cfgCopy.ClientSecret = "test-client-secret"
	}
	cfgCopy.CertPath = srv.WriteCAPEM(t)

	cl, err := client.New(&cfgCopy)
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}

	reg := toolkit.NewRegistry(cfgCopy.ReadOnly, cfgCopy.MaxResponseBytes)
	if err := Register(reg, cat, cl, &cfgCopy); err != nil {
		t.Fatalf("Register: %v", err)
	}

	var srvOpts []server.ServerOption
	if elicitFn != nil {
		srvOpts = append(srvOpts, server.WithElicitation())
	}
	mcpSrv := server.NewMCPServer("test-wso2is-mcp", "1.0.0", srvOpts...)
	reg.AddToServer(mcpSrv)

	var clientOpts []mcpclient.ClientOption
	if elicitFn != nil {
		clientOpts = append(clientOpts, mcpclient.WithElicitationHandler(&funcElicitHandler{fn: elicitFn}))
	}
	mc, err := mcpclient.NewInProcessClientWithOptions(mcpSrv, clientOpts...)
	if err != nil {
		t.Fatalf("NewInProcessClient: %v", err)
	}
	t.Cleanup(func() { _ = mc.Close() })

	if err := mc.Start(t.Context()); err != nil {
		t.Fatalf("mcp client Start: %v", err)
	}

	initReq := mcp.InitializeRequest{
		Params: mcp.InitializeParams{
			ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
			ClientInfo:      mcp.Implementation{Name: "test-client", Version: "1.0.0"},
		},
	}
	if elicitFn != nil {
		initReq.Params.Capabilities.Elicitation = &mcp.ElicitationCapability{}
	}
	if _, err := mc.Initialize(t.Context(), initReq); err != nil {
		t.Fatalf("mcp client Initialize: %v", err)
	}

	return &testHarness{
		srv:       srv,
		isClient:  cl,
		reg:       reg,
		mcpSrv:    mcpSrv,
		mcpClient: mc,
	}
}

func callTool(t *testing.T, mc *mcpclient.Client, name string, args map[string]any) (*mcp.CallToolResult, string) {
	t.Helper()
	res, err := mc.CallTool(t.Context(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{
			Name:      name,
			Arguments: args,
		},
	})
	if err != nil {
		t.Fatalf("CallTool(%q) protocol error: %v", name, err)
	}
	if res == nil || len(res.Content) == 0 {
		t.Fatalf("CallTool(%q) returned empty content", name)
	}
	tc, ok := res.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("CallTool(%q) content[0] is %T, want TextContent", name, res.Content[0])
	}
	return res, tc.Text
}

func TestRegisterAndToolMetadata(t *testing.T) {
	cat := load730Catalog(t)

	t.Run("default config registers 4 catalog tools with expected layers and annotations", func(t *testing.T) {
		defs := Tools(cat, nil, &config.Config{})
		if len(defs) != 4 {
			t.Fatalf("len(Tools) = %d, want 4", len(defs))
		}
		byName := make(map[string]toolkit.ToolDef, len(defs))
		for _, d := range defs {
			byName[d.Name] = d
			if d.Layer != toolkit.LayerCatalog {
				t.Errorf("tool %s Layer = %q, want %q", d.Name, d.Layer, toolkit.LayerCatalog)
			}
		}

		for _, name := range []string{"search_operations", "describe_operation", "call_read_operation", "call_write_operation"} {
			if _, ok := byName[name]; !ok {
				t.Fatalf("missing catalog tool %q", name)
			}
		}
		if byName["search_operations"].Kind != iscatalog.KindRead {
			t.Errorf("search_operations kind = %s, want read", byName["search_operations"].Kind)
		}
		if byName["describe_operation"].Kind != iscatalog.KindRead {
			t.Errorf("describe_operation kind = %s, want read", byName["describe_operation"].Kind)
		}
		if byName["call_read_operation"].Kind != iscatalog.KindRead {
			t.Errorf("call_read_operation kind = %s, want read", byName["call_read_operation"].Kind)
		}
		if byName["call_write_operation"].Kind != iscatalog.KindDestructive {
			t.Errorf("call_write_operation kind = %s, want destructive", byName["call_write_operation"].Kind)
		}

		h := newTestHarness(t, cat, &config.Config{}, nil)
		toolsRes, err := h.mcpClient.ListTools(t.Context(), mcp.ListToolsRequest{})
		if err != nil {
			t.Fatalf("ListTools: %v", err)
		}
		if len(toolsRes.Tools) != 4 {
			t.Fatalf("ListTools count = %d, want 4", len(toolsRes.Tools))
		}
		for _, tool := range toolsRes.Tools {
			switch tool.Name {
			case "call_write_operation":
				if tool.Annotations.ReadOnlyHint == nil || *tool.Annotations.ReadOnlyHint {
					t.Errorf("call_write_operation ReadOnlyHint = %v, want false", tool.Annotations.ReadOnlyHint)
				}
				if tool.Annotations.DestructiveHint == nil || !*tool.Annotations.DestructiveHint {
					t.Errorf("call_write_operation DestructiveHint = %v, want true", tool.Annotations.DestructiveHint)
				}
				if !strings.Contains(tool.Description, "Before calling, describe the exact change and its target to the user") {
					t.Errorf("call_write_operation missing confirmation sentence: %q", tool.Description)
				}
			default:
				if tool.Annotations.ReadOnlyHint == nil || !*tool.Annotations.ReadOnlyHint {
					t.Errorf("%s ReadOnlyHint = %v, want true", tool.Name, tool.Annotations.ReadOnlyHint)
				}
				if tool.Annotations.DestructiveHint == nil || *tool.Annotations.DestructiveHint {
					t.Errorf("%s DestructiveHint = %v, want false", tool.Name, tool.Annotations.DestructiveHint)
				}
			}
		}
	})

	t.Run("READ_ONLY omits call_write_operation", func(t *testing.T) {
		h := newTestHarness(t, cat, &config.Config{ReadOnly: true}, nil)
		if h.reg.Count() != 3 {
			t.Fatalf("Registry.Count = %d, want 3", h.reg.Count())
		}
		for _, name := range h.reg.Names() {
			if name == "call_write_operation" {
				t.Fatal("call_write_operation should not be registered when READ_ONLY=true")
			}
		}
	})
}

func TestSearchOperations_RankingFiltersLimitsAndShape(t *testing.T) {
	cat := load730Catalog(t)
	h := newTestHarness(t, cat, &config.Config{}, nil)

	t.Run("ranking and output shape", func(t *testing.T) {
		res, text := callTool(t, h.mcpClient, "search_operations", map[string]any{
			"query": "cors origins",
		})
		if res.IsError {
			t.Fatalf("unexpected error: %s", text)
		}

		var out struct {
			Total      int              `json:"total"`
			Operations []map[string]any `json:"operations"`
		}
		if err := json.Unmarshal([]byte(text), &out); err != nil {
			t.Fatalf("unmarshal search output: %v\n%s", err, text)
		}
		if out.Total == 0 || len(out.Operations) == 0 {
			t.Fatalf("expected matches, got total=%d len=%d", out.Total, len(out.Operations))
		}
		top := out.Operations[0]
		if top["id"] != "governance_security.getCORSOrigins" {
			t.Errorf("top id = %v, want governance_security.getCORSOrigins", top["id"])
		}
		for _, key := range []string{"id", "summary", "method", "path", "kind", "organization", "availability"} {
			if _, ok := top[key]; !ok {
				t.Errorf("missing key %q in search operation item: %v", key, top)
			}
		}
		if top["method"] != http.MethodGet {
			t.Errorf("method = %v, want GET", top["method"])
		}
		if top["kind"] != "read" {
			t.Errorf("kind = %v, want read", top["kind"])
		}
		if top["organization"] != false {
			t.Errorf("organization = %v, want false", top["organization"])
		}
		if top["availability"] != "available" {
			t.Errorf("availability = %v, want available", top["availability"])
		}
	})

	t.Run("filters area, kind, organization", func(t *testing.T) {
		res, text := callTool(t, h.mcpClient, "search_operations", map[string]any{
			"query":        "application",
			"area":         "applications",
			"kind":         "write",
			"organization": true,
			"limit":        50,
		})
		if res.IsError {
			t.Fatalf("unexpected error: %s", text)
		}
		var out struct {
			Total      int              `json:"total"`
			Operations []map[string]any `json:"operations"`
		}
		if err := json.Unmarshal([]byte(text), &out); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if out.Total == 0 {
			t.Fatal("expected org write application operations")
		}
		for _, op := range out.Operations {
			id, _ := op["id"].(string)
			if !strings.HasPrefix(id, "applications.org.") {
				t.Errorf("unexpected id %q for area=applications organization=true", id)
			}
			if op["kind"] != "write" {
				t.Errorf("op %s kind = %v, want write", id, op["kind"])
			}
			if op["organization"] != true {
				t.Errorf("op %s organization = %v, want true", id, op["organization"])
			}
		}
	})

	t.Run("limit bounds default 20 and max 50", func(t *testing.T) {
		// Default limit 20
		_, text := callTool(t, h.mcpClient, "search_operations", map[string]any{
			"query": "user",
		})
		var defOut struct {
			Total      int              `json:"total"`
			Operations []map[string]any `json:"operations"`
		}
		if err := json.Unmarshal([]byte(text), &defOut); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if defOut.Total <= 20 || len(defOut.Operations) != 20 {
			t.Fatalf("default limit: total=%d len=%d, want total>20 len=20", defOut.Total, len(defOut.Operations))
		}

		// Limit 5
		_, text5 := callTool(t, h.mcpClient, "search_operations", map[string]any{
			"query": "user",
			"limit": 5,
		})
		var out5 struct {
			Operations []map[string]any `json:"operations"`
		}
		if err := json.Unmarshal([]byte(text5), &out5); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(out5.Operations) != 5 {
			t.Fatalf("limit=5 returned %d operations, want 5", len(out5.Operations))
		}

		// Limit > 50 clamped to 50
		_, text100 := callTool(t, h.mcpClient, "search_operations", map[string]any{
			"query": "user",
			"limit": 100,
		})
		var out100 struct {
			Operations []map[string]any `json:"operations"`
		}
		if err := json.Unmarshal([]byte(text100), &out100); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(out100.Operations) != 50 {
			t.Fatalf("limit=100 returned %d operations, want 50", len(out100.Operations))
		}

		// Limit <= 0 defaults to 20
		_, text0 := callTool(t, h.mcpClient, "search_operations", map[string]any{
			"query": "user",
			"limit": 0,
		})
		var out0 struct {
			Operations []map[string]any `json:"operations"`
		}
		if err := json.Unmarshal([]byte(text0), &out0); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(out0.Operations) != 20 {
			t.Fatalf("limit=0 returned %d operations, want 20", len(out0.Operations))
		}
	})

	t.Run("no matches returns empty operations slice", func(t *testing.T) {
		res, text := callTool(t, h.mcpClient, "search_operations", map[string]any{
			"query": "zzzzqqqqxxxx9999",
		})
		if res.IsError {
			t.Fatalf("unexpected error: %s", text)
		}
		var out struct {
			Total      int              `json:"total"`
			Operations []map[string]any `json:"operations"`
		}
		if err := json.Unmarshal([]byte(text), &out); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if out.Total != 0 || out.Operations == nil || len(out.Operations) != 0 {
			t.Fatalf("got total=%d ops=%v, want total=0 empty non-nil slice", out.Total, out.Operations)
		}
	})

	t.Run("invalid search inputs return error", func(t *testing.T) {
		cases := []map[string]any{
			{},
			{"query": 123},
			{"query": "users", "kind": "invalid_kind"},
			{"query": "users", "organization": "not-a-bool"},
			{"query": "users", "limit": "ten"},
		}
		for _, args := range cases {
			res, text := callTool(t, h.mcpClient, "search_operations", args)
			if !res.IsError {
				t.Errorf("expected error for args %v, got %s", args, text)
			}
		}
	})

	t.Run("availability reflects flow and deprecated flags", func(t *testing.T) {
		_, text := callTool(t, h.mcpClient, "search_operations", map[string]any{
			"query": "roles.getRole",
		})
		if !strings.Contains(text, `"deprecated_hidden"`) {
			t.Errorf("expected deprecated_hidden availability in %s", text)
		}
	})
}
