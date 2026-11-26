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

package branding_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/asgardeo/mcp/internal/tools/admin/admintest"
	"github.com/asgardeo/mcp/internal/tools/admin/branding"
)

func TestBrandingTools(t *testing.T) {
	e := admintest.New(t, branding.Tools)
	e.Kinds(t, map[string][2]bool{
		"get_branding": {true, false}, "update_branding": {false, false},
		"configure_email_sender": {false, false}, "configure_sms_sender": {false, false},
		"update_email_template": {false, false},
	})

	e.IS.HandleJSON(http.MethodGet, "/api/server/v1/branding-preference", http.StatusOK, map[string]any{"type": "ORG", "preference": map[string]any{}})
	e.MustCall(t, "get_branding", nil)
	if q := e.Last(t).RawQuery; !strings.Contains(q, "type=ORG") {
		t.Fatalf("get_branding query = %q", q)
	}

	e.IS.HandleJSON(http.MethodPut, "/api/server/v1/branding-preference", http.StatusOK, map[string]any{"type": "ORG"})
	e.MustCall(t, "update_branding", map[string]any{"preference": map[string]any{"theme": map[string]any{"activeTheme": "DARK"}}})
	if b := e.LastBody(t); b["type"] != "ORG" || b["preference"] == nil {
		t.Fatalf("update_branding body = %v", b)
	}

	// Email sender: not found, so it is created with the name and the password is not echoed.
	e.IS.HandleJSON(http.MethodGet, "/api/server/v2/notification-senders/email/EmailPublisher", http.StatusNotFound, map[string]any{"code": "NSM-60002"})
	e.IS.HandleJSON(http.MethodPost, "/api/server/v2/notification-senders/email", http.StatusCreated, map[string]any{
		"name": "EmailPublisher", "properties": []any{map[string]any{"key": "password", "value": "smtp-secret-1"}},
	})
	text := e.MustCall(t, "configure_email_sender", map[string]any{
		"smtp_server_host": "smtp.example.com", "smtp_port": 587, "from_address": "iam@example.com",
		"auth_type": "BASIC", "properties": []any{map[string]any{"key": "password", "value": "smtp-secret-1"}},
	})
	e.Expect(t, http.MethodPost, "/api/server/v2/notification-senders/email")
	if b := e.LastBody(t); b["name"] != "EmailPublisher" || b["smtpServerHost"] != "smtp.example.com" {
		t.Fatalf("email sender body = %v", b)
	}
	if strings.Contains(text, "smtp-secret-1") {
		t.Fatalf("email sender secret leaked: %s", text)
	}

	// SMS sender: exists, so it is updated without the name.
	e.IS.HandleJSON(http.MethodGet, "/api/server/v2/notification-senders/sms/SMSPublisher", http.StatusOK, map[string]any{"name": "SMSPublisher"})
	e.IS.HandleJSON(http.MethodPut, "/api/server/v2/notification-senders/sms/SMSPublisher", http.StatusOK, map[string]any{"name": "SMSPublisher"})
	e.MustCall(t, "configure_sms_sender", map[string]any{"provider": "Twilio", "provider_url": "https://api.twilio.com", "key": "k", "secret": "sms-secret", "sender": "+100", "content_type": "FORM"})
	e.Expect(t, http.MethodPut, "/api/server/v2/notification-senders/sms/SMSPublisher")
	if b := e.LastBody(t); b["provider"] != "Twilio" || b["name"] != nil {
		t.Fatalf("sms sender body = %v", b)
	}

	e.IS.HandleJSON(http.MethodPut, "/api/server/v2/email/template-types/dHlwZQ/org-templates/en_US", http.StatusOK, nil)
	e.MustCall(t, "update_email_template", map[string]any{"template_type_id": "dHlwZQ", "locale": "en_US", "subject": "Hi", "body": "<p>Hi</p>"})
	if b := e.LastBody(t); b["contentType"] != "text/html" || b["subject"] != "Hi" {
		t.Fatalf("template body = %v", b)
	}

	// get_branding is root-only (the organization spec has no GET); organization_id is refused.
	if text, isErr := e.Call(t, "get_branding", map[string]any{"organization_id": "o1"}); !isErr || !strings.Contains(text, "not available for organizations") {
		t.Fatalf("get_branding in an organization: %q", text)
	}
}
