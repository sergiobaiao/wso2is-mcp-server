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

package redact

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRedactFixtures(t *testing.T) {
	tests := []struct {
		fixture string
		secrets []string
		kept    []string
	}{
		{
			fixture: "application.json",
			secrets: []string{"super-secret-oauth-client-secret"},
			kept:    []string{"my-spa-app", "oauth-client-id-123"},
		},
		{
			fixture: "idp.json",
			secrets: []string{"google-oauth-secret-xyz", "saml-signing-key-pem"},
			kept:    []string{"Google", "google-client-id-abc"},
		},
		{
			fixture: "userstore.json",
			secrets: []string{"ldap-bind-password-999", "db-conn-pass-888"},
			kept:    []string{"PRIMARY-LDAP", "cn=admin,dc=example,dc=com"},
		},
		{
			fixture: "notification_sender.json",
			secrets: []string{"smtp-pass-secret-111", "twilio-auth-token-222"},
			kept:    []string{"EmailPublisher", "smtp.example.com"},
		},
		{
			fixture: "webhook.json",
			secrets: []string{"whsec_top_secret_signing_key"},
			kept:    []string{"user-events", "https://hooks.example.com/is"},
		},
		{
			fixture: "action.json",
			secrets: []string{"bearer-api-token-333", "basic-auth-passwd-444"},
			kept:    []string{"pre-issue-token", "https://ext.example.com/action"},
		},
		{
			fixture: "keystore.json",
			secrets: []string{"-----BEGIN PRIVATE KEY-----secret-key-material", "keystore-pass-555"},
			kept:    []string{"wso2carbon.jks", "-----BEGIN CERTIFICATE-----public-cert"},
		},
		{
			fixture: "agent.json",
			secrets: []string{"agent-secret-666", "agent-token-777"},
			kept:    []string{"billing-agent", "agent-id-1"},
		},
		{
			fixture: "totp.json",
			secrets: []string{"otpauth://totp/alice?secret=JBSWY3DPEHPK3PXP", "JBSWY3DPEHPK3PXP", "backup-code-1111", "backup-code-2222"},
			kept:    []string{"alice"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.fixture, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("testdata", tc.fixture))
			if err != nil {
				t.Fatalf("read fixture %s: %v", tc.fixture, err)
			}
			var input any
			if err := json.Unmarshal(raw, &input); err != nil {
				t.Fatalf("unmarshal fixture %s: %v", tc.fixture, err)
			}

			out := Value(input)
			outBytes, err := json.Marshal(out)
			if err != nil {
				t.Fatalf("marshal output: %v", err)
			}
			outStr := string(outBytes)

			for _, s := range tc.secrets {
				if strings.Contains(outStr, s) {
					t.Errorf("fixture %s leaked secret %q in %s", tc.fixture, s, outStr)
				}
			}
			if len(tc.secrets) > 0 && !strings.Contains(outStr, RedactedPlaceholder) {
				t.Errorf("fixture %s expected %q marker in %s", tc.fixture, RedactedPlaceholder, outStr)
			}
			for _, k := range tc.kept {
				if !strings.Contains(outStr, k) {
					t.Errorf("fixture %s lost non-secret value %q in %s", tc.fixture, k, outStr)
				}
			}
		})
	}
}

func TestRedactBytesAndStrings(t *testing.T) {
	raw := []byte(`{"clientSecret":"s1","nested":{"apiKey":"k2"},"ok":"v"}`)
	redacted := JSONBytes(raw)
	s := string(redacted)
	if strings.Contains(s, "s1") || strings.Contains(s, "k2") {
		t.Fatalf("secret leaked: %s", s)
	}
	if !strings.Contains(s, RedactedPlaceholder) || !strings.Contains(s, `"ok":"v"`) {
		t.Fatalf("unexpected redacted JSON: %s", s)
	}
}
