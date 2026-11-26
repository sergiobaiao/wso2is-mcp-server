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
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/asgardeo/mcp/internal/config"
	"github.com/asgardeo/mcp/internal/tools/toolkit"
	"github.com/asgardeo/mcp/internal/wso2is/redact"
	"github.com/mark3labs/mcp-go/mcp"
)

func TestCallOperation_RefusalsSendZeroRequests(t *testing.T) {
	cat := load730Catalog(t)
	h := newTestHarness(t, cat, &config.Config{
		AllowFlowAPIs:     false,
		IncludeDeprecated: false,
	}, nil)

	cases := []struct {
		name    string
		tool    string
		args    map[string]any
		wantSub string
	}{
		{
			name:    "call_read_operation refuses write operation",
			tool:    "call_read_operation",
			args:    map[string]any{"operation_id": "users.createUser"},
			wantSub: "changes data; use call_write_operation",
		},
		{
			name:    "call_read_operation refuses destructive operation",
			tool:    "call_read_operation",
			args:    map[string]any{"operation_id": "users.deleteUser", "path_params": map[string]any{"id": "u1"}},
			wantSub: "changes data; use call_write_operation",
		},
		{
			name:    "call_write_operation refuses read operation",
			tool:    "call_write_operation",
			args:    map[string]any{"operation_id": "users.getUser"},
			wantSub: "is read-only; use call_read_operation",
		},
		{
			name:    "call_write_operation refuses flow operation when disabled",
			tool:    "call_write_operation",
			args:    map[string]any{"operation_id": "user_lifecycle.changePassword"},
			wantSub: "ALLOW_FLOW_APIS=true",
		},
		{
			name:    "call_read_operation refuses deprecated operation when disabled",
			tool:    "call_read_operation",
			args:    map[string]any{"operation_id": "roles.getRole"},
			wantSub: "INCLUDE_DEPRECATED=true",
		},
		{
			name:    "org-scoped operation without organization_id",
			tool:    "call_read_operation",
			args:    map[string]any{"operation_id": "applications.org.getAllApplications"},
			wantSub: "requires organization_id",
		},
		{
			name:    "root operation with organization_id",
			tool:    "call_read_operation",
			args:    map[string]any{"operation_id": "applications.getAllApplications", "organization_id": "org-1"},
			wantSub: "is not available for organizations",
		},
		{
			name:    "missing required path param fails validation",
			tool:    "call_read_operation",
			args:    map[string]any{"operation_id": "users.getUserById"},
			wantSub: "invalid input:",
		},
		{
			name:    "unknown query param fails validation",
			tool:    "call_read_operation",
			args:    map[string]any{"operation_id": "users.getUser", "query": map[string]any{"bogusParam": "x"}},
			wantSub: "invalid input:",
		},
		{
			name:    "unknown operation_id",
			tool:    "call_read_operation",
			args:    map[string]any{"operation_id": "no.such.op"},
			wantSub: "not found",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h.srv.ResetRequests()
			res, text := callTool(t, h.mcpClient, tc.tool, tc.args)
			if !res.IsError {
				t.Fatalf("expected error, got success: %s", text)
			}
			if !strings.Contains(text, tc.wantSub) {
				t.Errorf("error = %q, want substring %q", text, tc.wantSub)
			}
			if reqs := h.srv.Requests(); len(reqs) != 0 {
				t.Fatalf("expected zero HTTP requests on refusal/validation error, got %d: %+v", len(reqs), reqs)
			}
		})
	}
}

func TestCallOperation_ReadWriteOrgAndElicitation(t *testing.T) {
	cat := load730Catalog(t)

	t.Run("read operation reaches IS and redacts response", func(t *testing.T) {
		h := newTestHarness(t, cat, &config.Config{}, nil)
		h.srv.HandleJSON(http.MethodGet, "/scim2/Users/u-42", http.StatusOK, map[string]any{
			"id":       "u-42",
			"userName": "alice",
			"password": "secret-password",
		})

		res, text := callTool(t, h.mcpClient, "call_read_operation", map[string]any{
			"operation_id": "users.getUserById",
			"path_params":  map[string]any{"id": "u-42"},
		})
		if res.IsError {
			t.Fatalf("unexpected error: %s", text)
		}
		var out map[string]any
		if err := json.Unmarshal([]byte(text), &out); err != nil {
			t.Fatalf("unmarshal response: %v", err)
		}
		if out["password"] != redact.RedactedPlaceholder {
			t.Errorf("expected password=%q, got %v", redact.RedactedPlaceholder, out["password"])
		}
		if strings.Contains(text, "secret-password") {
			t.Fatalf("secret leaked in response: %s", text)
		}
	})

	t.Run("org-scoped operation performs organization_switch and sends org token", func(t *testing.T) {
		h := newTestHarness(t, cat, &config.Config{}, nil)
		h.srv.HandleJSON(http.MethodGet, "/o/api/server/v1/applications", http.StatusOK, map[string]any{
			"totalResults": 1,
			"applications": []any{map[string]any{"id": "app-1", "name": "MyApp"}},
		})

		res, text := callTool(t, h.mcpClient, "call_read_operation", map[string]any{
			"operation_id":    "applications.org.getAllApplications",
			"organization_id": "org-abc",
		})
		if res.IsError {
			t.Fatalf("unexpected error: %s", text)
		}

		var sawAppReq bool
		for _, r := range h.srv.Requests() {
			if r.Method == http.MethodGet && r.Path == "/o/api/server/v1/applications" {
				sawAppReq = true
				auth := r.Headers.Get("Authorization")
				if !strings.HasPrefix(auth, "Bearer org-token-org-abc-") {
					t.Errorf("Authorization = %q, want org-switched token", auth)
				}
			}
		}
		if !sawAppReq {
			t.Fatal("expected GET /o/api/server/v1/applications request")
		}
	})

	t.Run("write operation elicitation reject sends zero requests", func(t *testing.T) {
		var elicited atomic.Bool
		h := newTestHarness(t, cat, &config.Config{}, func(_ context.Context, req mcp.ElicitationRequest) (*mcp.ElicitationResult, error) {
			elicited.Store(true)
			if !strings.Contains(req.Params.Message, "users.createUser") {
				t.Errorf("elicitation message %q should mention operation id users.createUser", req.Params.Message)
			}
			return &mcp.ElicitationResult{
				ElicitationResponse: mcp.ElicitationResponse{
					Action:  mcp.ElicitationResponseActionAccept,
					Content: map[string]any{"approve": false},
				},
			}, nil
		})

		h.srv.HandleJSON(http.MethodPost, "/scim2/Users", http.StatusCreated, map[string]any{"id": "u-new"})
		h.srv.ResetRequests()

		res, text := callTool(t, h.mcpClient, "call_write_operation", map[string]any{
			"operation_id": "users.createUser",
			"body": map[string]any{
				"userName": "bob",
			},
		})
		if !elicited.Load() {
			t.Fatal("expected elicitation to be invoked")
		}
		if !res.IsError || text != toolkit.NotApprovedMessage {
			t.Fatalf("got (isError=%v, text=%q), want (true, %q)", res.IsError, text, toolkit.NotApprovedMessage)
		}
		if reqs := h.srv.Requests(); len(reqs) != 0 {
			t.Fatalf("expected 0 HTTP requests when elicitation declined, got %d", len(reqs))
		}
	})

	t.Run("write operation elicitation approve executes request and supports array body", func(t *testing.T) {
		var elicitedMsg string
		h := newTestHarness(t, cat, &config.Config{}, func(_ context.Context, req mcp.ElicitationRequest) (*mcp.ElicitationResult, error) {
			elicitedMsg = req.Params.Message
			return &mcp.ElicitationResult{
				ElicitationResponse: mcp.ElicitationResponse{
					Action:  mcp.ElicitationResponseActionAccept,
					Content: map[string]any{"approve": true},
				},
			}, nil
		})

		h.srv.HandleJSON(http.MethodPut, "/api/server/v1/cors/origins", http.StatusOK, nil)
		// Use patchRole or updateClaim or similar write op with array or object body:
		h.srv.HandleJSON(http.MethodPost, "/scim2/Users", http.StatusCreated, map[string]any{
			"id":       "u-created",
			"userName": "bob",
		})

		res, text := callTool(t, h.mcpClient, "call_write_operation", map[string]any{
			"operation_id": "users.createUser",
			"body": map[string]any{
				"userName": "bob",
			},
		})
		if res.IsError {
			t.Fatalf("unexpected error: %s", text)
		}
		if !strings.Contains(elicitedMsg, "users.createUser") || !strings.Contains(elicitedMsg, "userName") {
			t.Errorf("elicitation message %q should contain operation id and top-level body keys", elicitedMsg)
		}
		if !strings.Contains(text, "u-created") {
			t.Errorf("expected u-created in %s", text)
		}

		// Also verify array body works through call_write_operation.
		h.srv.HandleJSON(http.MethodPut, "/api/server/v1/api-resources/api-1/scopes", http.StatusNoContent, nil)
		arrRes, arrText := callTool(t, h.mcpClient, "call_write_operation", map[string]any{
			"operation_id": "api_authorization.putApiResourcesByApiresourceidScopes",
			"path_params":  map[string]any{"apiResourceId": "api-1"},
			"body": []any{
				map[string]any{
					"name":        "greetings:read",
					"displayName": "Read Greetings",
				},
			},
		})
		if arrRes.IsError {
			t.Fatalf("call_write_operation with array body failed: %s", arrText)
		}
		if arrText != "Done." {
			t.Errorf("got %q for 204 No Content, want %q", arrText, "Done.")
		}
	})
}
