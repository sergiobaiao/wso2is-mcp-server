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
	"strings"
	"testing"
)

// TestCatalogTools runs quickstart.md section 2 (Story 1) against a real IS 7.3.
func TestCatalogTools(t *testing.T) {
	ts := newToolSet(t)

	found := ts.mustCall(t, "search_operations", map[string]any{"query": "cors"})
	if !strings.Contains(mustJSON(t, found), "governance_security.getCORSOrigins") {
		t.Fatalf("search_operations cors did not return getCORSOrigins: %v", found)
	}

	ts.mustCall(t, "call_read_operation", map[string]any{"operation_id": "governance_security.getCORSOrigins"})

	text, isErr := ts.call(t, "call_read_operation", map[string]any{"operation_id": "users.createUser"})
	if !isErr || !strings.Contains(text, "use call_write_operation") {
		t.Fatalf("call_read_operation on a write operation: want refusal, got %q", text)
	}

	text, isErr = ts.call(t, "call_write_operation", map[string]any{
		"operation_id": "users.createUser",
		"body":         map[string]any{"userName": 42},
	})
	if !isErr || !strings.HasPrefix(text, "invalid input:") {
		t.Fatalf("call_write_operation with invalid body: want invalid input, got %q", text)
	}
}
