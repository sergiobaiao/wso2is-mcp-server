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

package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"

	apispecs "github.com/asgardeo/mcp/api-specs"
	"github.com/asgardeo/mcp/internal/config"
	"github.com/asgardeo/mcp/internal/wso2is/catalog"
	"github.com/asgardeo/mcp/internal/wso2is/client"
	"github.com/mark3labs/mcp-go/mcp"
)

// listedTool is the subset of a tools/list entry the compatibility checks read.
type listedTool struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	InputSchema struct {
		Type       string                    `json:"type"`
		Required   []string                  `json:"required"`
		Properties map[string]map[string]any `json:"properties"`
	} `json:"inputSchema"`
	Annotations mcp.ToolAnnotation `json:"annotations"`
}

type goldenTool struct {
	Name        string `json:"name"`
	InputSchema struct {
		Type       string            `json:"type,omitempty"`
		Required   []string          `json:"required,omitempty"`
		Properties map[string]string `json:"properties,omitempty"`
	} `json:"inputSchema"`
}

func newTestServer(t *testing.T, readOnly bool) []listedTool {
	t.Helper()
	t.Setenv("PRODUCT_MODE", "")

	cat, err := catalog.Load(apispecs.FS, "7.3.0")
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}
	cfg := &config.Config{ServerURL: "https://localhost:9443", TenantDomain: "carbon.super", ReadOnly: readOnly}
	cl, err := client.New(cfg)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	s, _, err := setupServer(cfg, cat, cl)
	if err != nil {
		t.Fatalf("setupServer: %v", err)
	}

	respMsg := s.HandleMessage(context.Background(), json.RawMessage(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	rawResp, err := json.Marshal(respMsg)
	if err != nil {
		t.Fatalf("marshal tools/list response: %v", err)
	}
	var rpc struct {
		Result struct {
			Tools []listedTool `json:"tools"`
		} `json:"result"`
		Error any `json:"error"`
	}
	if err := json.Unmarshal(rawResp, &rpc); err != nil {
		t.Fatalf("unmarshal tools/list response: %v", err)
	}
	if rpc.Error != nil {
		t.Fatalf("unexpected JSON-RPC error: %v", rpc.Error)
	}
	return rpc.Result.Tools
}

// TestAdminToolsCompat checks that every tool captured in the T001 golden file (before any change)
// still exists and still accepts the same inputs: same property types, no new required inputs.
// Added optional inputs, new annotations and new descriptions are allowed (FR-009).
// The golden file is never regenerated.
func TestAdminToolsCompat(t *testing.T) {
	listed := newTestServer(t, false)
	byName := make(map[string]listedTool, len(listed))
	for _, tl := range listed {
		byName[tl.Name] = tl
	}

	rawGolden, err := os.ReadFile(filepath.Join("..", "..", "internal", "tools", "testdata", "admin_tools.golden.json"))
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	var want []goldenTool
	if err := json.Unmarshal(rawGolden, &want); err != nil {
		t.Fatalf("unmarshal golden: %v", err)
	}
	if len(want) != 19 {
		t.Fatalf("golden must hold the 19 original admin tools, has %d", len(want))
	}

	for _, w := range want {
		got, ok := byName[w.Name]
		if !ok {
			t.Errorf("tool %q is missing", w.Name)
			continue
		}
		for prop, typ := range w.InputSchema.Properties {
			gp, ok := got.InputSchema.Properties[prop]
			if !ok {
				t.Errorf("tool %q lost input %q", w.Name, prop)
				continue
			}
			if gt, _ := gp["type"].(string); gt != typ {
				t.Errorf("tool %q input %q type changed from %q to %q", w.Name, prop, typ, gt)
			}
		}
		oldRequired := map[string]bool{}
		for _, r := range w.InputSchema.Required {
			oldRequired[r] = true
		}
		for _, r := range got.InputSchema.Required {
			if !oldRequired[r] {
				t.Errorf("tool %q added required input %q", w.Name, r)
			}
		}
	}
}

// TestReadOnlyRegistersOnlyReadTools covers READ_ONLY=true (FR-019, T049).
func TestReadOnlyRegistersOnlyReadTools(t *testing.T) {
	listed := newTestServer(t, true)
	if len(listed) == 0 {
		t.Fatal("no tools registered in read-only mode")
	}
	names := make([]string, 0, len(listed))
	for _, tl := range listed {
		names = append(names, tl.Name)
		if tl.Annotations.ReadOnlyHint == nil || !*tl.Annotations.ReadOnlyHint {
			t.Errorf("read-only mode registered non-read tool %q", tl.Name)
		}
	}
	sort.Strings(names)
	for _, forbidden := range []string{"call_write_operation", "create_user", "delete_user"} {
		if i := sort.SearchStrings(names, forbidden); i < len(names) && names[i] == forbidden {
			t.Errorf("read-only mode registered %q", forbidden)
		}
	}
	for _, required := range []string{"search_operations", "describe_operation", "call_read_operation", "list_users"} {
		if i := sort.SearchStrings(names, required); i >= len(names) || names[i] != required {
			t.Errorf("read-only mode is missing %q", required)
		}
	}
}
