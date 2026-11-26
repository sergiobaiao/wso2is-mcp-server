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
	"net/http"
	"testing"
)

func TestDeriveOperationID(t *testing.T) {
	tests := []struct {
		method string
		path   string
		want   string
	}{
		{http.MethodGet, "/me/sessions/{session-id}", "getMeSessionsBySessionId"},
		{http.MethodPost, "/Users/.search", "postUsersSearch"},
		{http.MethodDelete, "/scim2/Users/{id}", "deleteScim2UsersById"},
		{http.MethodPatch, "/v2/Roles/{id}", "patchV2RolesById"},
		{http.MethodGet, "/", "getRoot"},
	}

	for _, tc := range tests {
		got := deriveOperationID(tc.method, tc.path)
		if got != tc.want {
			t.Errorf("deriveOperationID(%q, %q) = %q, want %q", tc.method, tc.path, got, tc.want)
		}
	}
}

func TestSanitizeOperationID(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"getUser by id", "getUserById"},
		{"getGroup by id", "getGroupById"},
		{"Initiate Authentication*", "initiateAuthentication"},
		{"Handle Authentication", "handleAuthentication"},
		{"listAllowedCORSOrigins", "listAllowedCORSOrigins"},
		{"organizations_organization_id_get", "organizationsOrganizationIdGet"},
	}

	for _, tc := range tests {
		got := sanitizeOperationID(tc.in)
		if got != tc.want {
			t.Errorf("sanitizeOperationID(%q) = %q, want %q", tc.in, tc.want, tc.want)
		}
	}
}

func TestExtractPathPrefix(t *testing.T) {
	tests := []struct {
		name       string
		family     Family
		fileName   string
		servers    []string
		basePath   string
		wantPrefix string
	}{
		{
			name:       "standard root tenant template",
			family:     FamilyRoot,
			fileName:   "cors",
			servers:    []string{"https://{serverUrl}/t/{tenantDomain}/api/server/v1"},
			wantPrefix: "/api/server/v1",
		},
		{
			name:       "standard org tenant template",
			family:     FamilyOrg,
			fileName:   "application",
			servers:    []string{"https://{serverUrl}/t/{tenantDomain}/o/api/server/v1"},
			wantPrefix: "/o/api/server/v1",
		},
		{
			name:       "scim2-schemas swagger without servers",
			family:     FamilyRoot,
			fileName:   "scim2-schemas",
			wantPrefix: "/scim2",
		},
		{
			name:       "authentication-api swaggerhub override",
			family:     FamilyRoot,
			fileName:   "authentication-api",
			servers:    []string{"https://virtserver.swaggerhub.com/JOHND_1/Authenticate/v1", "https://{server-url}/t/{tenant-domain}/oauth2/"},
			wantPrefix: "/oauth2",
		},
		{
			name:       "admin-identity-verification asgardeo.io override",
			family:     FamilyRoot,
			fileName:   "admin-identity-verification",
			servers:    []string{"https://api.asgardeo.io/t/{organization-name}/api/users/v1"},
			wantPrefix: "/api/users/v1",
		},
		{
			name:       "org end-user-credential-management missing /o/ override",
			family:     FamilyOrg,
			fileName:   "end-user-credential-management",
			servers:    []string{"https://{server-url}/t/{tenant-domain}/api/server/v1"},
			wantPrefix: "/o/api/server/v1",
		},
		{
			name:       "org rule-metadata missing /o/ override",
			family:     FamilyOrg,
			fileName:   "rule-metadata",
			servers:    []string{"https://{server-url}/t/{tenant-domain}/api/server/v1"},
			wantPrefix: "/o/api/server/v1",
		},
		{
			name:       "oauth-dcr malformed scheme slash",
			family:     FamilyRoot,
			fileName:   "oauth-dcr",
			servers:    []string{"https:/{serverUrl}/api/identity/oauth2/dcr/v1.1"},
			wantPrefix: "/api/identity/oauth2/dcr/v1.1",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := resolvePathPrefix(tc.family, tc.fileName, tc.servers, tc.basePath)
			if got != tc.wantPrefix {
				t.Errorf("resolvePathPrefix = %q, want %q", got, tc.wantPrefix)
			}
		})
	}
}

func TestClassifyKind(t *testing.T) {
	tests := []struct {
		method string
		path   string
		want   Kind
	}{
		{http.MethodGet, "/api/server/v1/applications", KindRead},
		{http.MethodHead, "/api/server/v1/applications/{id}", KindRead},
		{http.MethodDelete, "/scim2/Users/{id}", KindDestructive},
		{http.MethodPost, "/api/server/v1/applications", KindWrite},
		{http.MethodPatch, "/scim2/Users/{id}", KindWrite},
		{http.MethodPut, "/api/server/v1/cors/origins", KindWrite},
		// Overrides per research.md R9:
		{http.MethodPost, "/scim2/Users/.search", KindRead},
		{http.MethodPost, "/o/scim2/Users/.search", KindRead},
		{http.MethodPost, "/scim2/Bulk", KindWrite},
		{http.MethodPost, "/api/server/v1/applications/{id}/regenerate-secret", KindDestructive},
		{http.MethodPost, "/api/users/v1/{user-id}/sessions/terminate", KindDestructive},
		{http.MethodPost, "/api/server/v1/applications/{id}/revoke", KindDestructive},
		{http.MethodPost, "/api/server/v1/organizations/unshare", KindDestructive},
		{http.MethodPut, "/scim2/Groups/{id}", KindDestructive},
		{http.MethodPut, "/scim2/v2/Roles/{id}", KindDestructive},
	}

	for _, tc := range tests {
		got := classifyKind(tc.method, tc.path)
		if got != tc.want {
			t.Errorf("classifyKind(%q, %q) = %q, want %q", tc.method, tc.path, got, tc.want)
		}
	}
}

func TestNormalizeOpenAPI3Raw(t *testing.T) {
	raw := []byte(`openapi: 3.0.0
info:
  title: test
  version: 1.0.0
paths:
  /test:
    post:
      parameters:
        - $ref: '#/components/parameters/fileTypeHeaderParam'
        - $ref: '#/components/parameters/preserveClaimsParam'
      requestBody:
        content:
          multipart/form-data:
            schema:
              $ref: '#/components/schemas/FileUpload'
      responses:
        '200':
          description: ok
          content:
            application/json:
              schema:
                $ref: '#/definitions/MyObj'
components:
  schemas:
    MyObj:
      type: object
`)
	out, err := normalizeOpenAPI3Raw(raw)
	if err != nil {
		t.Fatalf("normalizeOpenAPI3Raw failed: %v", err)
	}
	sf, err := parseSpecFile(FamilyRoot, "claim-management.yaml", out)
	if err != nil {
		t.Fatalf("parseSpecFile after normalization failed: %v", err)
	}
	if sf.Doc == nil || sf.Doc.Components == nil || sf.Doc.Components.Schemas["FileUpload"] == nil {
		t.Fatalf("expected synthesized FileUpload schema")
	}
}
