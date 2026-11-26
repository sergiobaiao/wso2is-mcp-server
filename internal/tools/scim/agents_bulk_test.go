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
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/asgardeo/mcp/internal/wso2is/redact"
)

func TestListAgents(t *testing.T) {
	env := setupTestEnv(t)
	env.srv.HandleJSON(http.MethodGet, "/scim2/Agents", http.StatusOK, map[string]any{
		"totalResults": 1,
		"Resources": []any{
			map[string]any{"id": "ag-1", "userName": "AGENT/ag-1"},
		},
	})

	res := env.callTool(t, "list_agents", map[string]any{"count": 10})
	out := decodeResultJSON(t, res)
	if out["totalResults"] != float64(1) {
		t.Fatalf("totalResults = %v", out["totalResults"])
	}
	agents, ok := out["agents"].([]any)
	if !ok || len(agents) != 1 {
		t.Fatalf("agents = %#v", out["agents"])
	}
}

func TestCreateAgentRedactsCredentials(t *testing.T) {
	env := setupTestEnv(t)
	env.srv.HandleJSON(http.MethodPost, "/scim2/Agents", http.StatusCreated, map[string]any{
		"id":       "ag-new",
		"userName": "AGENT/ag-new",
		"password": "AgentSecretPassword!99",
	})

	res := env.callTool(t, "create_agent", map[string]any{
		"displayName": "Billing Service",
		"description": "Autonomous billing agent",
	})
	text := resultText(t, res)
	if strings.Contains(text, "AgentSecretPassword!99") {
		t.Fatalf("agent password leaked in output: %s", text)
	}
	out := decodeResultJSON(t, res)
	if out["id"] != "ag-new" || out["userName"] != "AGENT/ag-new" {
		t.Fatalf("unexpected output: %#v", out)
	}
	if pwd, ok := out["password"]; ok && pwd != redact.RedactedPlaceholder {
		t.Fatalf("expected password to be %q when present, got %v", redact.RedactedPlaceholder, pwd)
	}
}

func TestGetAndDeleteAgent(t *testing.T) {
	env := setupTestEnv(t)
	env.srv.HandleJSON(http.MethodGet, "/scim2/Agents/ag-1", http.StatusOK, map[string]any{
		"id":       "ag-1",
		"userName": "AGENT/ag-1",
		"password": "leaked-if-not-redacted",
	})
	env.srv.HandleJSON(http.MethodDelete, "/scim2/Agents/ag-1", http.StatusNoContent, nil)

	getRes := env.callTool(t, "get_agent", map[string]any{"agentId": "ag-1"})
	if strings.Contains(resultText(t, getRes), "leaked-if-not-redacted") {
		t.Fatal("get_agent leaked password")
	}
	getOut := decodeResultJSON(t, getRes)
	ag := getOut["agent"].(map[string]any)
	if ag["id"] != "ag-1" || ag["password"] != redact.RedactedPlaceholder {
		t.Fatalf("unexpected agent output: %#v", ag)
	}

	delRes := env.callTool(t, "delete_agent", map[string]any{"agentId": "ag-1"})
	delOut := decodeResultJSON(t, delRes)
	if delOut["status"] != "deleted" {
		t.Fatalf("status = %v", delOut["status"])
	}
}

func TestBulkCreateUsers(t *testing.T) {
	t.Run("array input", func(t *testing.T) {
		env := setupTestEnv(t)
		env.srv.HandleJSON(http.MethodPost, "/scim2/Bulk", http.StatusOK, map[string]any{
			"schemas": []string{"urn:ietf:params:scim:api:messages:2.0:BulkResponse"},
			"Operations": []any{
				map[string]any{
					"bulkId":   "user-1",
					"method":   "POST",
					"location": "https://localhost:9443/scim2/Users/u-1",
					"status":   map[string]any{"code": "201"},
				},
				map[string]any{
					"bulkId":   "user-2",
					"method":   "POST",
					"location": "",
					"status":   map[string]any{"code": "409"},
				},
			},
		})

		res := env.callTool(t, "bulk_create_users", map[string]any{
			"users": []any{
				map[string]any{
					"userName":   "u1",
					"password":   "BulkPass1!",
					"givenName":  "One",
					"familyName": "User",
					"email":      "u1@example.com",
				},
				map[string]any{
					"userName":   "u2",
					"password":   "BulkPass2!",
					"givenName":  "Two",
					"familyName": "User",
				},
			},
		})
		text := resultText(t, res)
		if strings.Contains(text, "BulkPass1!") || strings.Contains(text, "BulkPass2!") {
			t.Fatal("passwords leaked in bulk_create_users output")
		}
		out := decodeResultJSON(t, res)
		results, ok := out["results"].([]any)
		if !ok || len(results) != 2 {
			t.Fatalf("results = %#v", out["results"])
		}
		r0 := results[0].(map[string]any)
		if r0["location"] != "https://localhost:9443/scim2/Users/u-1" || r0["status"] == nil {
			t.Fatalf("unexpected first result: %#v", r0)
		}
	})

	t.Run("legacy user1/user2 inputs", func(t *testing.T) {
		env := setupTestEnv(t)
		env.srv.HandleJSON(http.MethodPost, "/scim2/Bulk", http.StatusOK, map[string]any{
			"Operations": []any{
				map[string]any{"bulkId": "user-1", "location": "/Users/1", "status": map[string]any{"code": "201"}},
				map[string]any{"bulkId": "user-2", "location": "/Users/2", "status": map[string]any{"code": "201"}},
			},
		})

		res := env.callTool(t, "bulk_create_users", map[string]any{
			"user1Username":   "u1",
			"user1Password":   "Pass1!",
			"user1GivenName":  "One",
			"user1FamilyName": "User",
			"user1Email":      "u1@example.com",
			"user2Username":   "u2",
			"user2Password":   "Pass2!",
			"user2GivenName":  "Two",
			"user2FamilyName": "User",
		})
		out := decodeResultJSON(t, res)
		results := out["results"].([]any)
		if len(results) != 2 {
			t.Fatalf("expected 2 results, got %d", len(results))
		}
		req := lastAPIRequest(t, env.srv)
		var bulkBody map[string]any
		if err := json.Unmarshal(req.Body, &bulkBody); err != nil {
			t.Fatalf("unmarshal bulk body: %v", err)
		}
		ops := bulkBody["Operations"].([]any)
		if len(ops) != 2 {
			t.Fatalf("expected 2 bulk operations, got %d", len(ops))
		}
	})

	t.Run("rejects empty or >100 users without calling IS", func(t *testing.T) {
		env := setupTestEnv(t)

		res0 := env.callTool(t, "bulk_create_users", map[string]any{"users": []any{}})
		if !res0.IsError {
			t.Fatal("expected error for 0 users")
		}

		many := make([]any, 101)
		for i := range many {
			many[i] = map[string]any{
				"userName":   fmt.Sprintf("u%d", i),
				"password":   "Pass1234!",
				"givenName":  "First",
				"familyName": "Last",
			}
		}
		res101 := env.callTool(t, "bulk_create_users", map[string]any{"users": many})
		if !res101.IsError {
			t.Fatal("expected error for 101 users")
		}
		if got := len(apiRequests(env.srv)); got != 0 {
			t.Fatalf("expected 0 IS requests on invalid bulk counts, got %d", got)
		}
	})
}
