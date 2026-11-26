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
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/asgardeo/mcp/internal/config"
	iscatalog "github.com/asgardeo/mcp/internal/wso2is/catalog"
	"github.com/getkin/kin-openapi/openapi3"
)

func TestDescribeOperation_FieldsAndExamples(t *testing.T) {
	cat := load730Catalog(t)
	h := newTestHarness(t, cat, &config.Config{}, nil)

	t.Run("read path-param operation", func(t *testing.T) {
		res, text := callTool(t, h.mcpClient, "describe_operation", map[string]any{
			"operation_id": "users.getUserById",
		})
		if res.IsError {
			t.Fatalf("describe_operation failed: %s", text)
		}

		var out map[string]any
		if err := json.Unmarshal([]byte(text), &out); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}

		requiredKeys := []string{
			"id", "summary", "description", "method", "path", "kind",
			"organization", "scopes", "availability", "deprecated",
			"parameters", "call_with", "example",
		}
		for _, k := range requiredKeys {
			if _, ok := out[k]; !ok {
				t.Errorf("missing key %q in describe output", k)
			}
		}

		if out["id"] != "users.getUserById" {
			t.Errorf("id = %v", out["id"])
		}
		if out["method"] != http.MethodGet {
			t.Errorf("method = %v, want GET", out["method"])
		}
		if out["kind"] != "read" {
			t.Errorf("kind = %v, want read", out["kind"])
		}
		if out["organization"] != false {
			t.Errorf("organization = %v, want false", out["organization"])
		}
		if out["call_with"] != "call_read_operation" {
			t.Errorf("call_with = %v, want call_read_operation", out["call_with"])
		}
		if out["availability"] != "available" {
			t.Errorf("availability = %v, want available", out["availability"])
		}

		params, ok := out["parameters"].([]any)
		if !ok || len(params) == 0 {
			t.Fatalf("expected non-empty parameters, got %T %v", out["parameters"], out["parameters"])
		}
		foundID := false
		for _, p := range params {
			pm := p.(map[string]any)
			if pm["name"] == "id" && pm["in"] == "path" && pm["required"] == true {
				foundID = true
			}
		}
		if !foundID {
			t.Errorf("expected required path parameter 'id' in %v", params)
		}

		ex, ok := out["example"].(map[string]any)
		if !ok {
			t.Fatalf("expected example object, got %T", out["example"])
		}
		if ex["operation_id"] != "users.getUserById" {
			t.Errorf("example.operation_id = %v", ex["operation_id"])
		}
		pp, ok := ex["path_params"].(map[string]any)
		if !ok || pp["id"] == nil {
			t.Errorf("expected example.path_params.id, got %v", ex["path_params"])
		}
	})

	t.Run("write org-scoped operation includes body and organization_id example", func(t *testing.T) {
		res, text := callTool(t, h.mcpClient, "describe_operation", map[string]any{
			"operation_id": "applications.org.patchApplication",
		})
		if res.IsError {
			t.Fatalf("describe_operation failed: %s", text)
		}

		var out map[string]any
		if err := json.Unmarshal([]byte(text), &out); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if out["organization"] != true {
			t.Errorf("organization = %v, want true", out["organization"])
		}
		if out["call_with"] != "call_write_operation" {
			t.Errorf("call_with = %v, want call_write_operation", out["call_with"])
		}
		body, ok := out["body"].(map[string]any)
		if !ok {
			t.Fatalf("expected body object, got %T", out["body"])
		}
		if body["content_type"] == "" || body["schema"] == nil {
			t.Errorf("expected body content_type and schema, got %v", body)
		}
		ex := out["example"].(map[string]any)
		if ex["organization_id"] == nil || ex["organization_id"] == "" {
			t.Errorf("expected organization_id in example, got %v", ex)
		}
		if _, hasBody := ex["body"]; !hasBody {
			t.Errorf("expected body skeleton in example, got %v", ex)
		}
	})

	t.Run("unknown operation returns error", func(t *testing.T) {
		res, _ := callTool(t, h.mcpClient, "describe_operation", map[string]any{
			"operation_id": "does.not.exist",
		})
		if !res.IsError {
			t.Fatal("expected error for unknown operation_id")
		}
	})
}

func TestDescribeOperation_Depth4SchemaLimitAndRefs(t *testing.T) {
	// Build a 6-level nested schema with $ref names at each level plus a cycle.
	lvl6 := &openapi3.SchemaRef{
		Ref:   "#/components/schemas/Level6",
		Value: openapi3.NewStringSchema(),
	}
	lvl5 := &openapi3.SchemaRef{
		Ref: "#/components/schemas/Level5",
		Value: openapi3.NewObjectSchema().WithProperties(map[string]*openapi3.Schema{
			"leaf": openapi3.NewStringSchema(),
		}),
	}
	lvl5.Value.Properties["next"] = lvl6

	lvl4 := &openapi3.SchemaRef{
		Ref:   "#/components/schemas/Level4",
		Value: openapi3.NewObjectSchema(),
	}
	lvl4.Value.Properties = openapi3.Schemas{"lvl5": lvl5}

	lvl3 := &openapi3.SchemaRef{
		Ref:   "#/components/schemas/Level3",
		Value: openapi3.NewObjectSchema(),
	}
	lvl3.Value.Properties = openapi3.Schemas{"lvl4": lvl4}

	lvl2 := &openapi3.SchemaRef{
		Ref:   "#/components/schemas/Level2",
		Value: openapi3.NewObjectSchema(),
	}
	lvl2.Value.Properties = openapi3.Schemas{"lvl3": lvl3}

	rootSchema := &openapi3.SchemaRef{
		Ref:   "#/components/schemas/Level1",
		Value: openapi3.NewObjectSchema(),
	}
	rootSchema.Value.Properties = openapi3.Schemas{
		"lvl2": lvl2,
		"self": rootSchema, // cycle
	}

	op := &iscatalog.Operation{
		ID:          "applications.deepOp",
		OperationID: "deepOp",
		Area:        "applications",
		Family:      iscatalog.FamilyRoot,
		Method:      http.MethodPost,
		Path:        "/api/server/v1/deep",
		Summary:     "Deep schema op",
		Kind:        iscatalog.KindWrite,
		Body: &iscatalog.BodySpec{
			ContentType: "application/json",
			Required:    true,
			Schema:      rootSchema,
		},
	}

	defs := Tools(nil, nil, nil)
	_ = defs

	// Build a minimal catalog using Search/Get via load730 or synthetic helper in package.
	// Using summarizeSchema directly and through describeOperation:
	got := summarizeSchema(rootSchema, 1, map[string]bool{})
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("marshal summarized schema: %v", err)
	}
	text := string(raw)

	for _, wantRef := range []string{
		"#/components/schemas/Level1",
		"#/components/schemas/Level2",
		"#/components/schemas/Level3",
		"#/components/schemas/Level4",
	} {
		if !strings.Contains(text, wantRef) {
			t.Errorf("expected %s to be preserved within 4 levels; got %s", wantRef, text)
		}
	}
	if strings.Contains(text, "Level5") || strings.Contains(text, "Level6") {
		t.Errorf("depth > 4 schema leaked Level5/Level6: %s", text)
	}
	_ = op
}

func TestDescribeOperation_AvailabilityReasons(t *testing.T) {
	cat := load730Catalog(t)

	t.Run("deprecated operation hidden by default and available when enabled", func(t *testing.T) {
		hOff := newTestHarness(t, cat, &config.Config{IncludeDeprecated: false}, nil)
		res, text := callTool(t, hOff.mcpClient, "describe_operation", map[string]any{
			"operation_id": "roles.getRole",
		})
		if res.IsError {
			t.Fatalf("unexpected error: %s", text)
		}
		var out map[string]any
		if err := json.Unmarshal([]byte(text), &out); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if out["availability"] != "deprecated_hidden" {
			t.Errorf("availability = %v, want deprecated_hidden", out["availability"])
		}
		if out["deprecated"] != true {
			t.Errorf("deprecated = %v, want true", out["deprecated"])
		}
		reason, _ := out["availability_reason"].(string)
		if !strings.Contains(reason, "INCLUDE_DEPRECATED=true") {
			t.Errorf("availability_reason = %q, want mention of INCLUDE_DEPRECATED=true", reason)
		}

		hOn := newTestHarness(t, cat, &config.Config{IncludeDeprecated: true}, nil)
		_, textOn := callTool(t, hOn.mcpClient, "describe_operation", map[string]any{
			"operation_id": "roles.getRole",
		})
		var outOn map[string]any
		if err := json.Unmarshal([]byte(textOn), &outOn); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if outOn["availability"] != "available" {
			t.Errorf("availability = %v, want available", outOn["availability"])
		}
	})

	t.Run("read_only_hidden when READ_ONLY=true", func(t *testing.T) {
		hRO := newTestHarness(t, cat, &config.Config{ReadOnly: true}, nil)
		res, text := callTool(t, hRO.mcpClient, "describe_operation", map[string]any{
			"operation_id": "users.createUser",
		})
		if res.IsError {
			t.Fatalf("unexpected error: %s", text)
		}
		var out map[string]any
		if err := json.Unmarshal([]byte(text), &out); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if out["availability"] != "read_only_hidden" {
			t.Errorf("availability = %v, want read_only_hidden", out["availability"])
		}
		reason, _ := out["availability_reason"].(string)
		if !strings.Contains(reason, "READ_ONLY=true") {
			t.Errorf("availability_reason = %q, want mention of READ_ONLY=true", reason)
		}
	})
}
