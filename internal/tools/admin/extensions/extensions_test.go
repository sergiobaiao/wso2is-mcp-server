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

package extensions_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/asgardeo/mcp/internal/tools/admin/admintest"
	"github.com/asgardeo/mcp/internal/tools/admin/extensions"
)

func TestExtensionTools(t *testing.T) {
	e := admintest.New(t, extensions.Tools)
	e.Kinds(t, map[string][2]bool{
		"list_webhooks": {true, false}, "create_webhook": {false, false}, "delete_webhook": {false, true},
		"list_actions": {true, false}, "create_action": {false, false}, "delete_action": {false, true},
	})

	e.IS.HandleJSON(http.MethodGet, "/api/server/v1/webhooks", http.StatusOK, map[string]any{"webhooks": []any{}})
	e.MustCall(t, "list_webhooks", nil)

	e.IS.HandleJSON(http.MethodPost, "/api/server/v1/webhooks", http.StatusCreated, map[string]any{"id": "w1", "secret": "hook-secret-1"})
	text := e.MustCall(t, "create_webhook", map[string]any{
		"name": "audit", "endpoint": "https://hooks.example.com/is", "secret": "hook-secret-1",
		"channels_subscribed": []any{"https://schemas.identity.wso2.org/events/login"},
	})
	b := e.LastBody(t)
	if b["endpoint"] != "https://hooks.example.com/is" || b["eventProfile"].(map[string]any)["name"] != "WSO2" {
		t.Fatalf("webhook body = %v", b)
	}
	if strings.Contains(text, "hook-secret-1") {
		t.Fatalf("webhook secret leaked: %s", text)
	}

	e.IS.HandleJSON(http.MethodDelete, "/api/server/v1/webhooks/w1", http.StatusNoContent, nil)
	e.MustCall(t, "delete_webhook", map[string]any{"webhook_id": "w1"})
	e.Expect(t, http.MethodDelete, "/api/server/v1/webhooks/w1")

	e.IS.HandleJSON(http.MethodGet, "/api/server/v1/actions/types", http.StatusOK, []any{
		map[string]any{"type": "PRE_ISSUE_ACCESS_TOKEN", "count": 1, "self": "/api/server/v1/actions/preIssueAccessToken"},
	})
	e.IS.HandleJSON(http.MethodGet, "/api/server/v1/actions/preIssueAccessToken", http.StatusOK, []any{map[string]any{"id": "a1", "name": "claims"}})
	if out := e.MustCall(t, "list_actions", nil); !strings.Contains(out, "a1") {
		t.Fatalf("list_actions output = %s", out)
	}
	e.MustCall(t, "list_actions", map[string]any{"action_type": "preIssueAccessToken"})
	e.Expect(t, http.MethodGet, "/api/server/v1/actions/preIssueAccessToken")

	e.IS.HandleJSON(http.MethodPost, "/api/server/v1/actions/preIssueAccessToken", http.StatusCreated, map[string]any{"id": "a2"})
	e.MustCall(t, "create_action", map[string]any{
		"action_type": "preIssueAccessToken", "name": "add claims", "endpoint_uri": "https://ext.example.com/token",
		"auth_type": "BEARER", "auth_properties": map[string]any{"accessToken": "ext-token-1"},
	})
	b = e.LastBody(t)
	ep := b["endpoint"].(map[string]any)
	if ep["uri"] != "https://ext.example.com/token" || ep["authentication"].(map[string]any)["type"] != "BEARER" {
		t.Fatalf("action body = %v", b)
	}

	e.IS.HandleJSON(http.MethodDelete, "/api/server/v1/actions/preIssueAccessToken/a2", http.StatusNoContent, nil)
	e.MustCall(t, "delete_action", map[string]any{"action_type": "preIssueAccessToken", "action_id": "a2"})
	e.Expect(t, http.MethodDelete, "/api/server/v1/actions/preIssueAccessToken/a2")
}
