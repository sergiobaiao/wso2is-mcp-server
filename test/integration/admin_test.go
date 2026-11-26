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
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// TestAdminTasks covers SC-004 against a real IS 7.3: each task is one tool call, and the test
// deletes what it creates.
func TestAdminTasks(t *testing.T) {
	ts := newToolSet(t)
	suffix := fmt.Sprintf("%d", time.Now().UnixNano()%1_000_000_000)

	spa := ts.mustCall(t, "create_single_page_app", map[string]any{
		"application_name": "mcpit-spa-" + suffix, "redirect_url": "https://localhost:3000/cb",
	})
	appID, _ := spa["application_configurations"].(map[string]any)["id"].(string)
	if appID == "" {
		t.Fatalf("create_single_page_app returned no id: %v", spa)
	}
	t.Cleanup(func() { ts.call(t, "delete_application", map[string]any{"id": appID}) })

	api := ts.mustCall(t, "create_api_resource", map[string]any{
		"identifier": "mcpit_api_" + suffix, "name": "mcpit api " + suffix, "requiresAuthorization": true,
		"scopes": []any{"mcpit_" + suffix + ":read", "mcpit_" + suffix + ":write"},
	})
	apiID, _ := api["id"].(string)
	if apiID == "" {
		t.Fatalf("create_api_resource returned no id: %v", api)
	}
	t.Cleanup(func() { ts.call(t, "delete_api_resource", map[string]any{"id": apiID}) })

	ts.mustCall(t, "authorize_api", map[string]any{
		"appId": appID, "id": apiID, "policyIdentifier": "RBAC",
		"scopes": []any{"mcpit_" + suffix + ":read", "mcpit_" + suffix + ":write"},
	})

	role := ts.mustCall(t, "create_role", map[string]any{"displayName": "mcpit-admin-role-" + suffix})
	roleID, _ := role["id"].(string)
	if roleID == "" {
		t.Fatalf("create_role returned no id: %v", role)
	}
	t.Cleanup(func() { ts.call(t, "delete_role", map[string]any{"roleId": roleID}) })
	ts.mustCall(t, "update_role_permissions", map[string]any{
		"roleId": roleID, "addPermissions": []any{"mcpit_" + suffix + ":read", "mcpit_" + suffix + ":write"},
	})

	user := ts.mustCall(t, "create_user", map[string]any{
		"username": "mcpitadm" + suffix, "password": "Mcp-It-Pass-123!", "first_name": "Mcp",
		"last_name": "Admin", "email": "mcpitadm" + suffix + "@example.com",
	})
	userID, _ := user["id"].(string)
	if userID == "" {
		t.Fatalf("create_user returned no id: %v", user)
	}
	t.Cleanup(func() { ts.call(t, "delete_user", map[string]any{"userId": userID}) })
	ts.mustCall(t, "assign_role_to_user", map[string]any{"roleId": roleID, "userId": userID})

	group := ts.mustCall(t, "create_group", map[string]any{"displayName": "mcpit-admin-group-" + suffix})
	groupID, _ := group["id"].(string)
	t.Cleanup(func() { ts.call(t, "delete_group", map[string]any{"groupId": groupID}) })
	ts.mustCall(t, "add_user_to_group", map[string]any{"groupId": groupID, "userId": userID})

	orgID := os.Getenv("IS_ORG_ID")
	if orgID == "" {
		t.Log("IS_ORG_ID not set; skipping the sub-organization list_users step")
		return
	}
	text, isErr := ts.call(t, "list_users", map[string]any{"organization_id": orgID})
	if isErr {
		t.Fatalf("list_users in organization %s: %s", orgID, text)
	}
	if strings.Contains(text, userID) {
		t.Fatalf("root user %s appears in sub-organization %s", userID, orgID)
	}
}
