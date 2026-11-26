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
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestSCIMSequence runs quickstart.md section 3 (Story 2) against a real IS 7.3 and deletes
// everything it creates.
func TestSCIMSequence(t *testing.T) {
	ts := newToolSet(t)
	suffix := fmt.Sprintf("%d", time.Now().UnixNano()%1_000_000_000)
	userName := "mcpit" + suffix
	const password = "Mcp-It-Pass-123!" // throwaway value for the local test container

	user := ts.mustCall(t, "create_user", map[string]any{
		"userName": userName, "password": password, "givenName": "Mcp", "familyName": "Test",
		"email": userName + "@example.com",
	})
	userID, _ := user["id"].(string)
	if userID == "" {
		t.Fatalf("create_user returned no id: %v", user)
	}
	t.Cleanup(func() { ts.call(t, "delete_user", map[string]any{"userId": userID}) })

	found := ts.mustCall(t, "search_users", map[string]any{"filter": "userName eq " + userName})
	if !strings.Contains(mustJSON(t, found), userID) {
		t.Fatalf("search_users did not find %s: %v", userName, found)
	}

	group := ts.mustCall(t, "create_group", map[string]any{"displayName": "mcpit-group-" + suffix})
	groupID, _ := group["id"].(string)
	if groupID == "" {
		t.Fatalf("create_group returned no id: %v", group)
	}
	t.Cleanup(func() { ts.call(t, "delete_group", map[string]any{"groupId": groupID}) })
	ts.mustCall(t, "add_user_to_group", map[string]any{"groupId": groupID, "userId": userID})

	role := ts.mustCall(t, "create_role", map[string]any{"displayName": "mcpit-role-" + suffix})
	roleID, _ := role["id"].(string)
	if roleID == "" {
		t.Fatalf("create_role returned no id: %v", role)
	}
	t.Cleanup(func() { ts.call(t, "delete_role", map[string]any{"roleId": roleID}) })
	ts.mustCall(t, "assign_role_to_user", map[string]any{"roleId": roleID, "userId": userID})

	got := mustJSON(t, ts.mustCall(t, "get_user", map[string]any{"userId": userID}))
	if !strings.Contains(got, groupID) || !strings.Contains(got, roleID) {
		t.Fatalf("get_user does not show group %s and role %s: %s", groupID, roleID, got)
	}

	bulkNames := make([]string, 0, 3)
	users := make([]any, 0, 3)
	for i := 1; i <= 3; i++ {
		name := fmt.Sprintf("mcpitbulk%d%s", i, suffix)
		bulkNames = append(bulkNames, name)
		users = append(users, map[string]any{
			"userName": name, "password": password, "givenName": "Bulk", "familyName": fmt.Sprintf("User%d", i),
		})
	}
	t.Cleanup(func() {
		for _, name := range bulkNames {
			res, isErr := ts.call(t, "search_users", map[string]any{"filter": "userName eq " + name})
			if isErr {
				continue
			}
			var r map[string]any
			_ = json.Unmarshal([]byte(res), &r)
			for _, id := range collectIDs(r) {
				ts.call(t, "delete_user", map[string]any{"userId": id})
			}
		}
	})
	bulk := mustJSON(t, ts.mustCall(t, "bulk_create_users", map[string]any{"users": users}))
	if strings.Contains(bulk, `"error`) {
		t.Fatalf("bulk_create_users reported errors: %s", bulk)
	}
}
