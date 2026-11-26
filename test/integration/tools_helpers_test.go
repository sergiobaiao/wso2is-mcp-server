//go:build integration

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

package integration

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	apispecs "github.com/asgardeo/mcp/api-specs"
	"github.com/asgardeo/mcp/internal/config"
	"github.com/asgardeo/mcp/internal/tools/admin"
	catalogtools "github.com/asgardeo/mcp/internal/tools/catalog"
	"github.com/asgardeo/mcp/internal/tools/scim"
	"github.com/asgardeo/mcp/internal/tools/toolkit"
	"github.com/asgardeo/mcp/internal/wso2is/catalog"
	"github.com/asgardeo/mcp/internal/wso2is/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// toolSet holds the admin tools built the way cmd/wso2is-mcp builds them (catalog, SCIM and
// admin layers), configured from the environment in contracts/configuration.md.
type toolSet struct {
	byName map[string]server.ServerTool
}

// newToolSet skips the test unless SERVER_URL, CLIENT_ID and CLIENT_SECRET point at an IS 7.3
// (quickstart.md Prerequisites). CERT_PATH supplies the IS CA; TLS verification stays on.
func newToolSet(t *testing.T) *toolSet {
	t.Helper()
	for _, k := range []string{"SERVER_URL", "CLIENT_ID", "CLIENT_SECRET"} {
		if os.Getenv(k) == "" {
			t.Skipf("%s not set; skipping integration test", k)
		}
	}
	cfg, err := config.LoadFor(config.BinaryAdmin)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	cat, err := catalog.Load(apispecs.FS, cfg.ISVersion)
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	cl, err := client.New(cfg)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	reg := toolkit.NewRegistry(cfg.ReadOnly, cfg.MaxResponseBytes)
	if err := catalogtools.Register(reg, cat, cl, cfg); err != nil {
		t.Fatalf("register catalog tools: %v", err)
	}
	if err := scim.Register(reg, cat, cl, cfg); err != nil {
		t.Fatalf("register SCIM tools: %v", err)
	}
	if err := admin.Register(reg, cat, cl, cfg); err != nil {
		t.Fatalf("register admin tools: %v", err)
	}
	ts := &toolSet{byName: map[string]server.ServerTool{}}
	for _, st := range reg.ServerTools() {
		ts.byName[st.Tool.Name] = st
	}
	return ts
}

// call runs a tool and returns its text result and isError flag.
func (ts *toolSet) call(t *testing.T, name string, args map[string]any) (string, bool) {
	t.Helper()
	st, ok := ts.byName[name]
	if !ok {
		t.Fatalf("tool %q is not registered", name)
	}
	req := mcp.CallToolRequest{}
	req.Params.Name = name
	req.Params.Arguments = args
	res, err := st.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("%s: handler error: %v", name, err)
	}
	var sb strings.Builder
	for _, c := range res.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			sb.WriteString(tc.Text)
		}
	}
	return sb.String(), res.IsError
}

// mustCall runs a tool, fails on an error result, and decodes the JSON text into a map.
func (ts *toolSet) mustCall(t *testing.T, name string, args map[string]any) map[string]any {
	t.Helper()
	text, isErr := ts.call(t, name, args)
	if isErr {
		t.Fatalf("%s failed: %s", name, text)
	}
	out := map[string]any{}
	if text != "Done." {
		if err := json.Unmarshal([]byte(text), &out); err != nil {
			t.Fatalf("%s: decode result: %v (%s)", name, err, text)
		}
	}
	return out
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

// collectIDs returns every "id" string found in a decoded JSON value.
func collectIDs(v any) []string {
	var ids []string
	switch x := v.(type) {
	case map[string]any:
		if id, ok := x["id"].(string); ok {
			ids = append(ids, id)
		}
		for _, e := range x {
			ids = append(ids, collectIDs(e)...)
		}
	case []any:
		for _, e := range x {
			ids = append(ids, collectIDs(e)...)
		}
	}
	return ids
}
