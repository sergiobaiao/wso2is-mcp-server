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

package transport

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/asgardeo/mcp/internal/config"
)

func TestProtectedResourceMetadata(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name            string
		issuer          string
		resource        string
		scopes          []string
		wantAuthServers []string
		wantScopes      []string
	}{
		{
			name:            "super tenant issuer with duplicates and unsorted scopes",
			issuer:          "https://is.example.com/oauth2/token",
			resource:        "https://mcp.example.com/mcp",
			scopes:          []string{"internal_user_mgt_view", "internal_app_mgt_view", "internal_user_mgt_view", "", "  "},
			wantAuthServers: []string{"https://is.example.com"},
			wantScopes:      []string{"internal_app_mgt_view", "internal_user_mgt_view"},
		},
		{
			name:            "tenanted issuer",
			issuer:          "https://is.example.com/t/acme/oauth2/token",
			resource:        "https://mcp.example.com/mcp",
			scopes:          []string{"openid", "internal_login"},
			wantAuthServers: []string{"https://is.example.com/t/acme"},
			wantScopes:      []string{"internal_login", "openid"},
		},
		{
			name:            "custom issuer without oauth2 token suffix and empty scopes",
			issuer:          "https://is.example.com/t/acme/",
			resource:        "https://mcp.example.com/mcp",
			scopes:          nil,
			wantAuthServers: []string{"https://is.example.com/t/acme"},
			wantScopes:      []string{},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			cfg := &config.Config{
				Issuer:   tc.issuer,
				Resource: tc.resource,
			}

			mux := http.NewServeMux()
			RegisterMetadataRoutes(mux, cfg, tc.scopes)
			srv := httptest.NewServer(mux)
			defer srv.Close()

			for _, path := range []string{
				"/.well-known/oauth-protected-resource",
				"/.well-known/oauth-protected-resource/mcp",
			} {
				resp, err := srv.Client().Get(srv.URL + path)
				if err != nil {
					t.Fatalf("GET %s: %v", path, err)
				}
				defer func() { _ = resp.Body.Close() }()

				if resp.StatusCode != http.StatusOK {
					t.Fatalf("GET %s: status=%d, want 200", path, resp.StatusCode)
				}
				if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
					t.Fatalf("GET %s: Content-Type=%q, want application/json", path, ct)
				}

				var got ProtectedResourceMetadata
				if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
					t.Fatalf("GET %s: decode JSON: %v", path, err)
				}

				if got.Resource != tc.resource {
					t.Errorf("GET %s: resource=%q, want %q", path, got.Resource, tc.resource)
				}
				if !reflect.DeepEqual(got.AuthorizationServers, tc.wantAuthServers) {
					t.Errorf("GET %s: authorization_servers=%v, want %v", path, got.AuthorizationServers, tc.wantAuthServers)
				}
				if !reflect.DeepEqual(got.BearerMethodsSupported, []string{"header"}) {
					t.Errorf("GET %s: bearer_methods_supported=%v, want [header]", path, got.BearerMethodsSupported)
				}
				if !reflect.DeepEqual(got.ScopesSupported, tc.wantScopes) {
					t.Errorf("GET %s: scopes_supported=%v, want %v", path, got.ScopesSupported, tc.wantScopes)
				}
				if got.ResourceName != "WSO2 IS MCP Server" {
					t.Errorf("GET %s: resource_name=%q, want %q", path, got.ResourceName, "WSO2 IS MCP Server")
				}
			}
		})
	}
}
