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

package organizations_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/asgardeo/mcp/internal/config"
	"github.com/asgardeo/mcp/internal/tools/admin/admintest"
	"github.com/asgardeo/mcp/internal/tools/admin/organizations"
)

func TestOrganizationTools(t *testing.T) {
	e := admintest.New(t, organizations.Tools)
	e.Kinds(t, map[string][2]bool{
		"list_organizations": {true, false}, "get_organization": {true, false},
		"create_organization": {false, false}, "delete_organization": {false, true},
		"share_users": {false, false},
	})
	if ro := admintest.New(t, organizations.Tools, func(c *config.Config) { c.ReadOnly = true }); ro.Reg.Count() != 2 {
		t.Fatalf("READ_ONLY count = %d, want 2", ro.Reg.Count())
	}

	e.IS.HandleJSON(http.MethodGet, "/api/server/v1/organizations", http.StatusOK, map[string]any{"organizations": []any{map[string]any{"id": "o1", "name": "acme"}}})
	e.MustCall(t, "list_organizations", map[string]any{"filter": "name co ac", "limit": 5, "recursive": true})
	if q := e.Last(t).RawQuery; !strings.Contains(q, "limit=5") || !strings.Contains(q, "recursive=true") {
		t.Fatalf("list query = %q", q)
	}

	e.IS.HandleJSON(http.MethodGet, "/api/server/v1/organizations/o1", http.StatusOK, map[string]any{"id": "o1", "name": "acme"})
	e.MustCall(t, "get_organization", map[string]any{"id": "o1"})
	e.Expect(t, http.MethodGet, "/api/server/v1/organizations/o1")

	e.IS.HandleJSON(http.MethodPost, "/api/server/v1/organizations", http.StatusCreated, map[string]any{"id": "o2", "name": "beta"})
	e.MustCall(t, "create_organization", map[string]any{"name": "beta", "description": "Beta corp", "parent_id": "o1"})
	if b := e.LastBody(t); b["name"] != "beta" || b["parentId"] != "o1" {
		t.Fatalf("create body = %v", b)
	}

	e.IS.HandleJSON(http.MethodDelete, "/api/server/v1/organizations/o2", http.StatusNoContent, nil)
	e.MustCall(t, "delete_organization", map[string]any{"id": "o2"})
	e.Expect(t, http.MethodDelete, "/api/server/v1/organizations/o2")

	e.IS.HandleJSON(http.MethodPost, "/api/server/v2/users/share", http.StatusAccepted, nil)
	e.MustCall(t, "share_users", map[string]any{
		"user_ids":      []any{"u1"},
		"organizations": []any{map[string]any{"orgId": "o1", "policy": "SELECTED_ORG_ONLY"}},
	})
	b := e.LastBody(t)
	if ids := b["userCriteria"].(map[string]any)["userIds"].([]any); len(ids) != 1 || len(b["organizations"].([]any)) != 1 {
		t.Fatalf("share body = %v", b)
	}

	e.IS.HandleJSON(http.MethodPost, "/api/server/v2/users/share-with-all", http.StatusAccepted, nil)
	e.MustCall(t, "share_users", map[string]any{"user_ids": []any{"u1"}, "policy": "ALL_EXISTING_ORGS_ONLY"})
	if b := e.LastBody(t); b["policy"] != "ALL_EXISTING_ORGS_ONLY" {
		t.Fatalf("share-with-all body = %v", b)
	}
	if text, isErr := e.Call(t, "share_users", map[string]any{"user_ids": []any{"u1"}}); !isErr || !strings.Contains(text, "invalid input") {
		t.Fatalf("share_users without target: %q", text)
	}

	e.IS.HandleJSON(http.MethodGet, "/o/api/server/v1/organizations", http.StatusOK, map[string]any{"organizations": []any{}})
	e.MustCall(t, "list_organizations", map[string]any{"organization_id": "o1"})
	e.Expect(t, http.MethodGet, "/o/api/server/v1/organizations")
}
