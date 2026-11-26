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
	"testing"
)

func TestListRoles(t *testing.T) {
	env := setupTestEnv(t)
	env.srv.HandleJSON(http.MethodGet, "/scim2/v2/Roles", http.StatusOK, map[string]any{
		"totalResults": 1,
		"Resources": []any{
			map[string]any{"id": "r-1", "displayName": "auditor"},
		},
	})

	res := env.callTool(t, "list_roles", map[string]any{"count": 10})
	out := decodeResultJSON(t, res)
	if out["totalResults"] != float64(1) {
		t.Fatalf("totalResults = %v", out["totalResults"])
	}
	roles, ok := out["roles"].([]any)
	if !ok || len(roles) != 1 {
		t.Fatalf("roles = %#v", out["roles"])
	}
}

func TestCreateRole(t *testing.T) {
	env := setupTestEnv(t)
	env.srv.HandleJSON(http.MethodPost, "/scim2/v2/Roles", http.StatusCreated, map[string]any{
		"id":          "r-new",
		"displayName": "auditor",
	})

	res := env.callTool(t, "create_role", map[string]any{"displayName": "auditor"})
	out := decodeResultJSON(t, res)
	if out["id"] != "r-new" || out["displayName"] != "auditor" {
		t.Fatalf("unexpected output: %#v", out)
	}
}

func TestAssignAndRemoveRoleToUser(t *testing.T) {
	env := setupTestEnv(t)
	env.srv.HandleJSON(http.MethodPatch, "/scim2/v2/Roles/r-1", http.StatusOK, map[string]any{
		"id":          "r-1",
		"displayName": "auditor",
	})

	t.Run("assign_role_to_user", func(t *testing.T) {
		env.srv.ResetRequests()
		res := env.callTool(t, "assign_role_to_user", map[string]any{
			"roleId": "r-1",
			"userId": "u-1",
		})
		out := decodeResultJSON(t, res)
		if out["role"] == nil {
			t.Fatalf("expected role in output: %#v", out)
		}

		req := lastAPIRequest(t, env.srv)
		var patch map[string]any
		if err := json.Unmarshal(req.Body, &patch); err != nil {
			t.Fatalf("unmarshal patch: %v", err)
		}
		ops, _ := patch["Operations"].([]any)
		op0, _ := ops[0].(map[string]any)
		if op0["op"] != "add" || op0["path"] != "users" {
			t.Fatalf("unexpected op: %#v", op0)
		}
		vals, _ := op0["value"].([]any)
		if len(vals) != 1 || vals[0].(map[string]any)["value"] != "u-1" {
			t.Fatalf("unexpected value: %#v", op0["value"])
		}
	})

	t.Run("remove_role_from_user", func(t *testing.T) {
		env.srv.ResetRequests()
		res := env.callTool(t, "remove_role_from_user", map[string]any{
			"roleId": "r-1",
			"userId": "u-1",
		})
		out := decodeResultJSON(t, res)
		if out["role"] == nil {
			t.Fatalf("expected role in output: %#v", out)
		}

		req := lastAPIRequest(t, env.srv)
		var patch map[string]any
		if err := json.Unmarshal(req.Body, &patch); err != nil {
			t.Fatalf("unmarshal patch: %v", err)
		}
		ops, _ := patch["Operations"].([]any)
		op0, _ := ops[0].(map[string]any)
		if op0["op"] != "remove" || op0["path"] != `users[value eq "u-1"]` {
			t.Fatalf("unexpected remove op: %#v", op0)
		}
	})
}

func TestDeleteRoleAndGetRole(t *testing.T) {
	env := setupTestEnv(t)
	env.srv.HandleJSON(http.MethodGet, "/scim2/v2/Roles/r-1", http.StatusOK, map[string]any{
		"id":          "r-1",
		"displayName": "auditor",
		"audience":    map[string]any{"type": "organization", "value": "org-root"},
		"permissions": []any{map[string]any{"value": "internal_user_mgt_list"}},
	})
	env.srv.HandleJSON(http.MethodDelete, "/scim2/v2/Roles/r-1", http.StatusNoContent, nil)

	getRes := env.callTool(t, "get_role", map[string]any{"roleId": "r-1"})
	getOut := decodeResultJSON(t, getRes)
	role, _ := getOut["role"].(map[string]any)
	if role["id"] != "r-1" {
		t.Fatalf("role = %#v", role)
	}

	delRes := env.callTool(t, "delete_role", map[string]any{"roleId": "r-1"})
	delOut := decodeResultJSON(t, delRes)
	if delOut["status"] != "deleted" {
		t.Fatalf("status = %v", delOut["status"])
	}
}

func TestAssignAndRemoveRoleToGroup(t *testing.T) {
	env := setupTestEnv(t)
	env.srv.HandleJSON(http.MethodPatch, "/scim2/v2/Roles/r-1", http.StatusOK, map[string]any{
		"id": "r-1",
	})

	res := env.callTool(t, "assign_role_to_group", map[string]any{
		"roleId":  "r-1",
		"groupId": "g-1",
	})
	_ = decodeResultJSON(t, res)
	req := lastAPIRequest(t, env.srv)
	var patch map[string]any
	_ = json.Unmarshal(req.Body, &patch)
	op0 := patch["Operations"].([]any)[0].(map[string]any)
	if op0["op"] != "add" || op0["path"] != "groups" {
		t.Fatalf("unexpected assign_role_to_group op: %#v", op0)
	}

	env.srv.ResetRequests()
	res2 := env.callTool(t, "remove_role_from_group", map[string]any{
		"roleId":  "r-1",
		"groupId": "g-1",
	})
	_ = decodeResultJSON(t, res2)
	req2 := lastAPIRequest(t, env.srv)
	var patch2 map[string]any
	_ = json.Unmarshal(req2.Body, &patch2)
	op02 := patch2["Operations"].([]any)[0].(map[string]any)
	if op02["op"] != "remove" || op02["path"] != `groups[value eq "g-1"]` {
		t.Fatalf("unexpected remove_role_from_group op: %#v", op02)
	}
}

func TestUpdateRolePermissions(t *testing.T) {
	env := setupTestEnv(t)
	env.srv.HandleJSON(http.MethodPatch, "/scim2/v2/Roles/r-1", http.StatusOK, map[string]any{
		"id":          "r-1",
		"displayName": "auditor",
	})

	res := env.callTool(t, "update_role_permissions", map[string]any{
		"roleId":            "r-1",
		"addPermissions":    []string{"scope_a", "scope_b"},
		"removePermissions": []string{"scope_old"},
	})
	out := decodeResultJSON(t, res)
	if out["role"] == nil {
		t.Fatalf("expected role in output: %#v", out)
	}

	req := lastAPIRequest(t, env.srv)
	var patch map[string]any
	if err := json.Unmarshal(req.Body, &patch); err != nil {
		t.Fatalf("unmarshal patch: %v", err)
	}
	ops, _ := patch["Operations"].([]any)
	if len(ops) != 2 {
		t.Fatalf("expected 2 operations (add + remove), got %#v", ops)
	}
	addOp := ops[0].(map[string]any)
	if addOp["op"] != "add" || addOp["path"] != "permissions" {
		t.Fatalf("unexpected add permissions op: %#v", addOp)
	}
	remOp := ops[1].(map[string]any)
	if remOp["op"] != "remove" || remOp["path"] != `permissions[value eq "scope_old"]` {
		t.Fatalf("unexpected remove permissions op: %#v", remOp)
	}
}
