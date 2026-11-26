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

// Package admintest runs curated admin area tools against the istest fake IS through an
// in-process MCP client. It is used only by tests.
package admintest

import (
	"context"
	"encoding/json"
	"slices"
	"testing"

	apispecs "github.com/asgardeo/mcp/api-specs"
	"github.com/asgardeo/mcp/internal/config"
	"github.com/asgardeo/mcp/internal/tools/toolkit"
	"github.com/asgardeo/mcp/internal/wso2is/catalog"
	"github.com/asgardeo/mcp/internal/wso2is/client"
	"github.com/asgardeo/mcp/internal/wso2is/istest"
	mcpclient "github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// ToolsFunc is the Tools constructor every admin area exports.
type ToolsFunc func(*catalog.Catalog, *client.Client, *config.Config) ([]toolkit.ToolDef, error)

// Env is one fake IS plus the area tools registered on an MCP server.
type Env struct {
	IS  *istest.Server
	Reg *toolkit.Registry
	Cat *catalog.Catalog
	Cl  *client.Client
	Cfg *config.Config
	cli *mcpclient.Client
}

// New builds the area tools with the 7.3.0 catalog. mods adjust the config (for example
// READ_ONLY).
func New(t *testing.T, tools ToolsFunc, mods ...func(*config.Config)) *Env {
	t.Helper()
	srv := istest.New(t)
	cat, err := catalog.Load(apispecs.FS, "7.3.0")
	if err != nil {
		t.Fatalf("catalog.Load: %v", err)
	}
	cfg := &config.Config{
		ServerURL: srv.URL(), TenantDomain: config.DefaultTenantDomain,
		AuthMode: config.AuthModeClientCredentials, ClientID: "test-client-id", ClientSecret: "test-client-secret",
		CertPath: srv.WriteCAPEM(t), MaxResponseBytes: config.DefaultMaxResponseBytes,
	}
	for _, m := range mods {
		m(cfg)
	}
	cl, err := client.New(cfg)
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}
	defs, err := tools(cat, cl, cfg)
	if err != nil {
		t.Fatalf("Tools: %v", err)
	}
	if _, err := tools(&catalog.Catalog{}, cl, cfg); err == nil {
		t.Fatal("Tools must fail when catalog operations are missing")
	}
	reg := toolkit.NewRegistry(cfg.ReadOnly, cfg.MaxResponseBytes)
	for _, d := range defs {
		if err := reg.Register(d); err != nil {
			t.Fatalf("Register(%s): %v", d.Name, err)
		}
	}
	s := server.NewMCPServer("admintest", "1.0.0")
	reg.AddToServer(s)
	cli, err := mcpclient.NewInProcessClient(s)
	if err != nil {
		t.Fatalf("NewInProcessClient: %v", err)
	}
	t.Cleanup(func() { _ = cli.Close() })
	if err := cli.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	init := mcp.InitializeRequest{}
	init.Params.ProtocolVersion = mcp.LATEST_PROTOCOL_VERSION
	init.Params.ClientInfo = mcp.Implementation{Name: "admintest", Version: "1.0.0"}
	if _, err := cli.Initialize(context.Background(), init); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	return &Env{IS: srv, Reg: reg, Cat: cat, Cl: cl, Cfg: cfg, cli: cli}
}

// Kinds asserts the registered names and, per name, readOnly and destructive annotations.
func (e *Env) Kinds(t *testing.T, want map[string][2]bool) {
	t.Helper()
	got := e.Reg.Names()
	names := make([]string, 0, len(want))
	for n := range want {
		names = append(names, n)
	}
	slices.Sort(got)
	slices.Sort(names)
	if !slices.Equal(got, names) {
		t.Fatalf("registered tools = %v, want %v", got, names)
	}
	for _, st := range e.Reg.ServerTools() {
		w := want[st.Tool.Name]
		ann := st.Tool.Annotations
		if ro := ann.ReadOnlyHint != nil && *ann.ReadOnlyHint; ro != w[0] {
			t.Errorf("%s readOnlyHint = %v, want %v", st.Tool.Name, ro, w[0])
		}
		if de := ann.DestructiveHint != nil && *ann.DestructiveHint; de != w[1] {
			t.Errorf("%s destructiveHint = %v, want %v", st.Tool.Name, de, w[1])
		}
	}
}

// Call runs a tool and returns its text and isError flag.
func (e *Env) Call(t *testing.T, name string, args map[string]any) (string, bool) {
	t.Helper()
	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args
	res, err := e.cli.CallTool(context.Background(), req)
	if err != nil {
		t.Fatalf("CallTool(%s): %v", name, err)
	}
	text := ""
	for _, c := range res.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			text += tc.Text
		}
	}
	return text, res.IsError
}

// MustCall runs a tool and fails the test on an error result.
func (e *Env) MustCall(t *testing.T, name string, args map[string]any) string {
	t.Helper()
	text, isErr := e.Call(t, name, args)
	if isErr {
		t.Fatalf("%s failed: %s", name, text)
	}
	return text
}

// Last returns the last request that was not a token or JWKS request.
func (e *Env) Last(t *testing.T) istest.RecordedRequest {
	t.Helper()
	reqs := e.IS.Requests()
	for i := len(reqs) - 1; i >= 0; i-- {
		if reqs[i].Path != "/oauth2/token" && reqs[i].Path != "/oauth2/jwks" {
			return reqs[i]
		}
	}
	t.Fatal("no IS API request was recorded")
	return istest.RecordedRequest{}
}

// LastBody decodes the body of the last API request.
func (e *Env) LastBody(t *testing.T) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(e.Last(t).Body, &m); err != nil {
		t.Fatalf("decode request body: %v (%s)", err, e.Last(t).Body)
	}
	return m
}

// Expect fails unless the last API request has method and path.
func (e *Env) Expect(t *testing.T, method, path string) {
	t.Helper()
	if r := e.Last(t); r.Method != method || r.Path != path {
		t.Fatalf("last request = %s %s, want %s %s", r.Method, r.Path, method, path)
	}
}
