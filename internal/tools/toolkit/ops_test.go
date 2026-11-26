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

package toolkit

import (
	"context"
	"strings"
	"testing"

	"github.com/asgardeo/mcp/internal/wso2is/catalog"
	"github.com/asgardeo/mcp/internal/wso2is/client"
	"github.com/mark3labs/mcp-go/mcp"
)

func TestOpPairSelect(t *testing.T) {
	root := &catalog.Operation{ID: "a.getX", Scopes: []string{"s2", "s1"}}
	org := &catalog.Operation{ID: "a.org.getX", Scopes: []string{"s1", "s3"}}
	orgCtx := client.WithOrganizationID(context.Background(), "org-1")

	if op, err := (OpPair{Root: root, Org: org}).Select(context.Background()); err != nil || op != root {
		t.Fatalf("root ctx: op=%v err=%v", op, err)
	}
	if op, err := (OpPair{Root: root, Org: org}).Select(orgCtx); err != nil || op != org {
		t.Fatalf("org ctx: op=%v err=%v", op, err)
	}
	_, err := (OpPair{Root: root}).Select(orgCtx)
	if err == nil || err.Error() != "operation a.getX is not available for organizations" {
		t.Fatalf("root-only pair in org ctx: err=%v", err)
	}
	if got := strings.Join((OpPair{Root: root, Org: org}).Scopes(), ","); got != "s1,s2,s3" {
		t.Fatalf("Scopes = %s", got)
	}
	if _, err := Lookup(&catalog.Catalog{}, "a.getX"); err == nil {
		t.Fatal("Lookup on empty catalog must fail")
	}
}

func TestRootOnlyToolRefusesOrganizationID(t *testing.T) {
	def := ToolDef{
		Name: "root_only", Layer: LayerCurated, Kind: catalog.KindRead,
		Operation: &catalog.Operation{ID: "a.getX"},
		Handler:   func(context.Context, map[string]any) (any, error) { return "ran", nil },
	}
	st, err := BuildTool(def, 0)
	if err != nil {
		t.Fatal(err)
	}
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{"organization_id": "org-1"}
	res, err := st.Handler(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	text := res.Content[0].(mcp.TextContent).Text
	if !res.IsError || text != "operation a.getX is not available for organizations" {
		t.Fatalf("result = %v %q", res.IsError, text)
	}
}

func TestArgs(t *testing.T) {
	args := map[string]any{"s": " x ", "n": float64(3), "f": 1.5, "b": true, "l": []any{"a", "b"}, "bad": 1}
	if s, err := RequireString(args, "s"); err != nil || s != "x" {
		t.Fatalf("RequireString = %q, %v", s, err)
	}
	if _, err := RequireString(args, "missing"); err == nil || err.Error() != "invalid input: missing: is required" {
		t.Fatalf("missing required: %v", err)
	}
	if _, err := OptString(args, "bad"); err == nil {
		t.Fatal("non-string must fail")
	}
	if n, ok, err := OptInt(args, "n"); err != nil || !ok || n != 3 {
		t.Fatalf("OptInt = %d %v %v", n, ok, err)
	}
	if _, _, err := OptInt(args, "f"); err == nil {
		t.Fatal("fractional int must fail")
	}
	if b, ok, err := OptBool(args, "b"); err != nil || !ok || !b {
		t.Fatalf("OptBool = %v %v %v", b, ok, err)
	}
	if l, err := OptStrings(args, "l"); err != nil || len(l) != 2 {
		t.Fatalf("OptStrings = %v %v", l, err)
	}
	if _, err := Paging(map[string]any{"limit": float64(-1)}); err == nil {
		t.Fatal("negative limit must fail")
	}
	body := map[string]any{}
	PutIf(body, map[string]any{"display_name": "D", "other": nil}, map[string]string{"display_name": "displayName", "other": "o"})
	if body["displayName"] != "D" || len(body) != 1 {
		t.Fatalf("PutIf = %v", body)
	}
}
