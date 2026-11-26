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

package scim_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestListGroups(t *testing.T) {
	env := setupTestEnv(t)
	env.srv.HandleJSON(http.MethodGet, "/scim2/Groups", http.StatusOK, map[string]any{
		"totalResults": 1,
		"Resources": []any{
			map[string]any{"id": "g-1", "displayName": "PRIMARY/Finance"},
		},
	})

	res := env.callTool(t, "list_groups", map[string]any{"count": 20})
	out := decodeResultJSON(t, res)
	if out["totalResults"] != float64(1) {
		t.Fatalf("totalResults = %v", out["totalResults"])
	}
	groups, ok := out["groups"].([]any)
	if !ok || len(groups) != 1 {
		t.Fatalf("groups = %#v", out["groups"])
	}
	req := lastAPIRequest(t, env.srv)
	if !strings.Contains(req.RawQuery, "attributes=displayName%2Cmembers") && !strings.Contains(req.RawQuery, "attributes=displayName,members") {
		t.Fatalf("expected attributes=displayName,members in query, got %q", req.RawQuery)
	}
}

func TestCreateGroup(t *testing.T) {
	env := setupTestEnv(t)
	env.srv.HandleJSON(http.MethodPost, "/scim2/Groups", http.StatusCreated, map[string]any{
		"id":          "g-new",
		"displayName": "PRIMARY/Finance",
	})

	res := env.callTool(t, "create_group", map[string]any{"displayName": "Finance"})
	out := decodeResultJSON(t, res)
	if out["id"] != "g-new" || out["displayName"] != "PRIMARY/Finance" {
		t.Fatalf("unexpected output: %#v", out)
	}
}

func TestAddUserToGroup(t *testing.T) {
	env := setupTestEnv(t)
	env.srv.HandleJSON(http.MethodGet, "/scim2/Users/u-1", http.StatusOK, map[string]any{
		"id":       "u-1",
		"userName": "PRIMARY/emma",
	})
	env.srv.HandleJSON(http.MethodPatch, "/scim2/Groups/g-1", http.StatusOK, map[string]any{
		"id":          "g-1",
		"displayName": "PRIMARY/Finance",
		"members": []any{
			map[string]any{"value": "u-1", "display": "PRIMARY/emma"},
		},
	})

	res := env.callTool(t, "add_user_to_group", map[string]any{
		"groupId": "g-1",
		"userId":  "u-1",
	})
	out := decodeResultJSON(t, res)
	if out["group"] == nil || out["addedUser"] == nil {
		t.Fatalf("expected group and addedUser in output: %#v", out)
	}

	reqs := apiRequests(env.srv)
	if len(reqs) != 2 {
		t.Fatalf("expected 2 API requests (user lookup + group patch), got %d", len(reqs))
	}
	if reqs[0].Method != http.MethodGet || reqs[0].Path != "/scim2/Users/u-1" {
		t.Fatalf("unexpected first request: %s %s", reqs[0].Method, reqs[0].Path)
	}
	if !strings.Contains(reqs[0].RawQuery, "attributes=userName") {
		t.Fatalf("expected attributes=userName on lookup, got %q", reqs[0].RawQuery)
	}
	if reqs[1].Method != http.MethodPatch || reqs[1].Path != "/scim2/Groups/g-1" {
		t.Fatalf("unexpected second request: %s %s", reqs[1].Method, reqs[1].Path)
	}
	var patch map[string]any
	if err := json.Unmarshal(reqs[1].Body, &patch); err != nil {
		t.Fatalf("unmarshal group patch: %v", err)
	}
	ops, _ := patch["Operations"].([]any)
	if len(ops) != 1 {
		t.Fatalf("expected 1 operation, got %#v", patch["Operations"])
	}
}

func TestRemoveUserFromGroup(t *testing.T) {
	env := setupTestEnv(t)
	env.srv.HandleJSON(http.MethodPatch, "/scim2/Groups/g-1", http.StatusOK, map[string]any{
		"id":          "g-1",
		"displayName": "PRIMARY/Finance",
	})

	res := env.callTool(t, "remove_user_from_group", map[string]any{
		"groupId": "g-1",
		"userId":  "u-1",
	})
	out := decodeResultJSON(t, res)
	if out["group"] == nil {
		t.Fatalf("expected group in output: %#v", out)
	}

	req := lastAPIRequest(t, env.srv)
	var patch map[string]any
	if err := json.Unmarshal(req.Body, &patch); err != nil {
		t.Fatalf("unmarshal patch: %v", err)
	}
	ops, _ := patch["Operations"].([]any)
	op0, _ := ops[0].(map[string]any)
	if op0["op"] != "remove" || op0["path"] != `members[value eq "u-1"]` {
		t.Fatalf("unexpected remove patch op: %#v", op0)
	}
}

func TestDeleteGroup(t *testing.T) {
	env := setupTestEnv(t)
	env.srv.HandleJSON(http.MethodDelete, "/scim2/Groups/g-1", http.StatusNoContent, nil)

	res := env.callTool(t, "delete_group", map[string]any{"groupId": "g-1"})
	out := decodeResultJSON(t, res)
	if out["status"] != "deleted" {
		t.Fatalf("status = %v", out["status"])
	}
}

func TestGetGroup(t *testing.T) {
	env := setupTestEnv(t)
	env.srv.HandleJSON(http.MethodGet, "/scim2/Groups/g-1", http.StatusOK, map[string]any{
		"id":          "g-1",
		"displayName": "PRIMARY/Finance",
		"members":     []any{map[string]any{"value": "u-1", "display": "PRIMARY/emma"}},
	})

	res := env.callTool(t, "get_group", map[string]any{"groupId": "g-1"})
	out := decodeResultJSON(t, res)
	grp, _ := out["group"].(map[string]any)
	if grp["id"] != "g-1" {
		t.Fatalf("group = %#v", out["group"])
	}
}

func TestUpdateGroup(t *testing.T) {
	env := setupTestEnv(t)
	env.srv.HandleJSON(http.MethodPatch, "/scim2/Groups/g-1", http.StatusOK, map[string]any{
		"id":          "g-1",
		"displayName": "PRIMARY/Accounting",
	})

	res := env.callTool(t, "update_group", map[string]any{
		"groupId":     "g-1",
		"displayName": "Accounting",
	})
	out := decodeResultJSON(t, res)
	grp, _ := out["group"].(map[string]any)
	if grp["displayName"] != "PRIMARY/Accounting" {
		t.Fatalf("group = %#v", grp)
	}
}
