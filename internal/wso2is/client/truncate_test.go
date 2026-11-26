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

package client

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/asgardeo/mcp/internal/wso2is/catalog"
)

func TestTruncateUnderLimitUnchanged(t *testing.T) {
	input := map[string]any{
		"totalResults": 2,
		"Resources": []any{
			map[string]any{"id": "1"},
			map[string]any{"id": "2"},
		},
	}
	got := Truncate(input, 10000, nil)
	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", got)
	}
	if _, truncated := m["truncated"]; truncated {
		t.Errorf("did not expect truncated key for small payload: %v", m)
	}
}

func TestTruncateTopLevelArray(t *testing.T) {
	var arr []any
	for i := 0; i < 50; i++ {
		arr = append(arr, map[string]any{
			"id":   i,
			"name": strings.Repeat("x", 30),
		})
	}
	op := &catalog.Operation{
		Params: []catalog.Param{
			{Name: "limit", In: "query"},
			{Name: "offset", In: "query"},
		},
	}

	const maxBytes = 600
	got := Truncate(arr, maxBytes, op)
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	if len(raw) > maxBytes {
		t.Errorf("truncated size = %d, want <= %d", len(raw), maxBytes)
	}

	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected map wrapper, got %T", got)
	}
	if m["truncated"] != true {
		t.Errorf("truncated = %v, want true", m["truncated"])
	}
	if m["total"] != 50 {
		t.Errorf("total = %v, want 50", m["total"])
	}
	returned, ok := m["returned"].(int)
	if !ok || returned <= 0 || returned >= 50 {
		t.Errorf("returned = %v, want 0 < returned < 50", m["returned"])
	}
	items, ok := m["items"].([]any)
	if !ok || len(items) != returned {
		t.Errorf("len(items) = %d, returned = %d", len(items), returned)
	}
	hint, _ := m["hint"].(string)
	if !strings.Contains(hint, "limit") || !strings.Contains(hint, "offset") {
		t.Errorf("hint = %q, want limit and offset", hint)
	}
}

func TestTruncateSCIMResources(t *testing.T) {
	var resources []any
	for i := 0; i < 40; i++ {
		resources = append(resources, map[string]any{
			"id":       i,
			"userName": "user-" + strings.Repeat("a", 25),
		})
	}
	scimResp := map[string]any{
		"schemas":      []any{"urn:ietf:params:scim:api:messages:2.0:ListResponse"},
		"totalResults": float64(120),
		"startIndex":   float64(1),
		"itemsPerPage": float64(40),
		"Resources":    resources,
	}
	op := &catalog.Operation{
		Params: []catalog.Param{
			{Name: "count", In: "query"},
			{Name: "startIndex", In: "query"},
		},
	}

	const maxBytes = 700
	got := Truncate(scimResp, maxBytes, op)
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	if len(raw) > maxBytes {
		t.Errorf("truncated size = %d, want <= %d", len(raw), maxBytes)
	}

	m, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected map, got %T", got)
	}
	if m["truncated"] != true {
		t.Errorf("truncated = %v, want true", m["truncated"])
	}
	if m["total"] != 120 {
		t.Errorf("total = %v, want 120", m["total"])
	}
	returned, ok := m["returned"].(int)
	if !ok || returned <= 0 || returned >= 40 {
		t.Errorf("returned = %v, want 0 < returned < 40", m["returned"])
	}
	kept, ok := m["Resources"].([]any)
	if !ok || len(kept) != returned {
		t.Errorf("len(Resources) = %d, returned = %d", len(kept), returned)
	}
	hint, _ := m["hint"].(string)
	if !strings.Contains(hint, "count") || !strings.Contains(hint, "startIndex") {
		t.Errorf("hint = %q, want count and startIndex", hint)
	}
}

func TestTruncateCursorHintAndNonList(t *testing.T) {
	op := &catalog.Operation{
		Params: []catalog.Param{
			{Name: "before", In: "query"},
			{Name: "after", In: "query"},
		},
	}
	bigObj := map[string]any{
		"payload": strings.Repeat("z", 1000),
	}
	const maxBytes = 300
	got := Truncate(bigObj, maxBytes, op)
	raw, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	if len(raw) > maxBytes {
		t.Errorf("truncated size = %d, want <= %d", len(raw), maxBytes)
	}
	m := got.(map[string]any)
	if m["truncated"] != true {
		t.Errorf("expected truncated=true")
	}
	if _, hasTotal := m["total"]; hasTotal {
		t.Errorf("did not expect total when unknown, got %v", m["total"])
	}
	hint, _ := m["hint"].(string)
	if !strings.Contains(hint, "before") || !strings.Contains(hint, "after") {
		t.Errorf("hint = %q, want before and after", hint)
	}
}
