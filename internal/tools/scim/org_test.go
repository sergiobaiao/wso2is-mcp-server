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
	"net/http"
	"strings"
	"testing"
)

func TestOrgCapableSCIMTools(t *testing.T) {
	env := setupTestEnv(t)

	// Register /o/scim2/... routes on fake IS (root routes are not registered so root hits 404).
	env.srv.HandleJSON(http.MethodGet, "/o/scim2/Users", http.StatusOK, map[string]any{
		"totalResults": 1,
		"Resources":    []any{map[string]any{"id": "ou-1", "userName": "PRIMARY/orguser"}},
	})
	env.srv.HandleJSON(http.MethodPost, "/o/scim2/Users/.search", http.StatusOK, map[string]any{
		"totalResults": 1,
		"Resources":    []any{map[string]any{"id": "ou-1", "userName": "PRIMARY/orguser"}},
	})
	env.srv.HandleJSON(http.MethodGet, "/o/scim2/Users/ou-1", http.StatusOK, map[string]any{
		"id":       "ou-1",
		"userName": "PRIMARY/orguser",
	})
	env.srv.HandleJSON(http.MethodPost, "/o/scim2/Users", http.StatusCreated, map[string]any{
		"id":       "ou-2",
		"userName": "PRIMARY/neworguser",
	})
	env.srv.HandleJSON(http.MethodPatch, "/o/scim2/Users/ou-1", http.StatusOK, map[string]any{
		"id":       "ou-1",
		"userName": "PRIMARY/orguser",
	})
	env.srv.HandleJSON(http.MethodDelete, "/o/scim2/Users/ou-1", http.StatusNoContent, nil)

	env.srv.HandleJSON(http.MethodGet, "/o/scim2/Groups", http.StatusOK, map[string]any{
		"totalResults": 1,
		"Resources":    []any{map[string]any{"id": "og-1", "displayName": "PRIMARY/OrgGroup"}},
	})
	env.srv.HandleJSON(http.MethodPost, "/o/scim2/Groups", http.StatusCreated, map[string]any{
		"id":          "og-2",
		"displayName": "PRIMARY/OrgGroup2",
	})
	env.srv.HandleJSON(http.MethodGet, "/o/scim2/Groups/og-1", http.StatusOK, map[string]any{
		"id":          "og-1",
		"displayName": "PRIMARY/OrgGroup",
	})
	env.srv.HandleJSON(http.MethodPatch, "/o/scim2/Groups/og-1", http.StatusOK, map[string]any{
		"id":          "og-1",
		"displayName": "PRIMARY/OrgGroup",
	})
	env.srv.HandleJSON(http.MethodDelete, "/o/scim2/Groups/og-1", http.StatusNoContent, nil)

	env.srv.HandleJSON(http.MethodGet, "/o/scim2/v2/Roles", http.StatusOK, map[string]any{
		"totalResults": 1,
		"Resources":    []any{map[string]any{"id": "or-1", "displayName": "OrgRole"}},
	})
	env.srv.HandleJSON(http.MethodPost, "/o/scim2/v2/Roles", http.StatusCreated, map[string]any{
		"id":          "or-2",
		"displayName": "OrgRole2",
	})
	env.srv.HandleJSON(http.MethodGet, "/o/scim2/v2/Roles/or-1", http.StatusOK, map[string]any{
		"id":          "or-1",
		"displayName": "OrgRole",
	})
	env.srv.HandleJSON(http.MethodPatch, "/o/scim2/v2/Roles/or-1", http.StatusOK, map[string]any{
		"id":          "or-1",
		"displayName": "OrgRole",
	})
	env.srv.HandleJSON(http.MethodDelete, "/o/scim2/v2/Roles/or-1", http.StatusNoContent, nil)

	env.srv.HandleJSON(http.MethodPost, "/o/scim2/Bulk", http.StatusOK, map[string]any{
		"Operations": []any{
			map[string]any{"bulkId": "user-1", "location": "/o/scim2/Users/ou-3", "status": map[string]any{"code": "201"}},
		},
	})

	const orgID = "org-acme-123"
	cases := []struct {
		tool     string
		args     map[string]any
		wantPath string
	}{
		{"list_users", map[string]any{"organization_id": orgID}, "/o/scim2/Users"},
		{"search_users", map[string]any{"filter": "userName eq orguser", "organization_id": orgID}, "/o/scim2/Users/.search"},
		{"get_user", map[string]any{"userId": "ou-1", "organization_id": orgID}, "/o/scim2/Users/ou-1"},
		{"create_user", map[string]any{"userName": "neworguser", "password": "P1!", "givenName": "A", "familyName": "B", "email": "a@b.com", "organization_id": orgID}, "/o/scim2/Users"},
		{"update_user_attribute", map[string]any{"userId": "ou-1", "attribute": "nickName", "value": "n", "organization_id": orgID}, "/o/scim2/Users/ou-1"},
		{"update_user_profile", map[string]any{"userId": "ou-1", "department": "Eng", "organization_id": orgID}, "/o/scim2/Users/ou-1"},
		{"delete_user", map[string]any{"userId": "ou-1", "organization_id": orgID}, "/o/scim2/Users/ou-1"},
		{"patch_user", map[string]any{"userId": "ou-1", "operations": []any{map[string]any{"op": "replace", "path": "nickName", "value": "n"}}, "organization_id": orgID}, "/o/scim2/Users/ou-1"},
		{"list_groups", map[string]any{"organization_id": orgID}, "/o/scim2/Groups"},
		{"create_group", map[string]any{"displayName": "OrgGroup2", "organization_id": orgID}, "/o/scim2/Groups"},
		{"add_user_to_group", map[string]any{"groupId": "og-1", "userId": "ou-1", "organization_id": orgID}, "/o/scim2/Groups/og-1"},
		{"remove_user_from_group", map[string]any{"groupId": "og-1", "userId": "ou-1", "organization_id": orgID}, "/o/scim2/Groups/og-1"},
		{"delete_group", map[string]any{"groupId": "og-1", "organization_id": orgID}, "/o/scim2/Groups/og-1"},
		{"get_group", map[string]any{"groupId": "og-1", "organization_id": orgID}, "/o/scim2/Groups/og-1"},
		{"update_group", map[string]any{"groupId": "og-1", "displayName": "Renamed", "organization_id": orgID}, "/o/scim2/Groups/og-1"},
		{"list_roles", map[string]any{"organization_id": orgID}, "/o/scim2/v2/Roles"},
		{"create_role", map[string]any{"displayName": "OrgRole2", "organization_id": orgID}, "/o/scim2/v2/Roles"},
		{"assign_role_to_user", map[string]any{"roleId": "or-1", "userId": "ou-1", "organization_id": orgID}, "/o/scim2/v2/Roles/or-1"},
		{"remove_role_from_user", map[string]any{"roleId": "or-1", "userId": "ou-1", "organization_id": orgID}, "/o/scim2/v2/Roles/or-1"},
		{"delete_role", map[string]any{"roleId": "or-1", "organization_id": orgID}, "/o/scim2/v2/Roles/or-1"},
		{"get_role", map[string]any{"roleId": "or-1", "organization_id": orgID}, "/o/scim2/v2/Roles/or-1"},
		{"assign_role_to_group", map[string]any{"roleId": "or-1", "groupId": "og-1", "organization_id": orgID}, "/o/scim2/v2/Roles/or-1"},
		{"remove_role_from_group", map[string]any{"roleId": "or-1", "groupId": "og-1", "organization_id": orgID}, "/o/scim2/v2/Roles/or-1"},
		{"update_role_permissions", map[string]any{"roleId": "or-1", "addPermissions": []string{"s1"}, "organization_id": orgID}, "/o/scim2/v2/Roles/or-1"},
		{"bulk_create_users", map[string]any{"users": []any{map[string]any{"userName": "ou3", "password": "P1!", "givenName": "A", "familyName": "B"}}, "organization_id": orgID}, "/o/scim2/Bulk"},
	}

	for _, tc := range cases {
		t.Run(tc.tool, func(t *testing.T) {
			env.srv.ResetRequests()
			res := env.callTool(t, tc.tool, tc.args)
			_ = decodeResultJSON(t, res)

			req := lastAPIRequest(t, env.srv)
			if req.Path != tc.wantPath {
				t.Fatalf("path = %q, want %q", req.Path, tc.wantPath)
			}
			authz := req.Headers.Get("Authorization")
			if !strings.HasPrefix(authz, "Bearer org-token-"+orgID+"-") {
				t.Fatalf("expected organization-switched bearer token, got %q", authz)
			}
		})
	}
}

func TestAgentToolsRejectOrganizationID(t *testing.T) {
	env := setupTestEnv(t)

	agentCalls := []struct {
		name string
		args map[string]any
	}{
		{"list_agents", map[string]any{"organization_id": "org-1"}},
		{"create_agent", map[string]any{"displayName": "Bot", "organization_id": "org-1"}},
		{"delete_agent", map[string]any{"agentId": "ag-1", "organization_id": "org-1"}},
		{"get_agent", map[string]any{"agentId": "ag-1", "organization_id": "org-1"}},
	}

	for _, tc := range agentCalls {
		t.Run(tc.name, func(t *testing.T) {
			env.srv.ResetRequests()
			res := env.callTool(t, tc.name, tc.args)
			if !res.IsError {
				t.Fatalf("%s expected error when organization_id is passed", tc.name)
			}
			text := resultText(t, res)
			if !strings.Contains(text, "not available for organizations") {
				t.Fatalf("expected 'not available for organizations' error, got %q", text)
			}
			if got := len(env.srv.Requests()); got != 0 {
				t.Fatalf("expected 0 IS/token requests when rejected, got %d", got)
			}
		})
	}
}
