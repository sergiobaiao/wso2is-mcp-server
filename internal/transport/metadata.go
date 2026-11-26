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
	"slices"
	"strings"

	"github.com/asgardeo/mcp/internal/config"
)

const (
	protectedResourceMetadataPath    = "/.well-known/oauth-protected-resource"
	protectedResourceMetadataMCPPath = "/.well-known/oauth-protected-resource/mcp"
	defaultResourceName              = "WSO2 IS MCP Server"
)

// ProtectedResourceMetadata represents RFC 9728 OAuth 2.0 Protected Resource Metadata.
type ProtectedResourceMetadata struct {
	Resource               string   `json:"resource"`
	AuthorizationServers   []string `json:"authorization_servers"`
	BearerMethodsSupported []string `json:"bearer_methods_supported"`
	ScopesSupported        []string `json:"scopes_supported"`
	ResourceName           string   `json:"resource_name"`
}

// BuildProtectedResourceMetadata constructs the RFC 9728 metadata document from cfg and registered tool scopes.
func BuildProtectedResourceMetadata(cfg *config.Config, scopes []string) ProtectedResourceMetadata {
	var resource, issuerBase string
	if cfg != nil {
		resource = cfg.Resource
		issuerBase = strings.TrimRight(strings.TrimSuffix(strings.TrimRight(cfg.Issuer, "/"), "/oauth2/token"), "/")
		if issuerBase == "" {
			issuerBase = strings.TrimRight(cfg.TenantBaseURL(), "/")
		}
	}

	authServers := []string{}
	if issuerBase != "" {
		authServers = []string{issuerBase}
	}

	return ProtectedResourceMetadata{
		Resource:               resource,
		AuthorizationServers:   authServers,
		BearerMethodsSupported: []string{"header"},
		ScopesSupported:        normalizeScopes(scopes),
		ResourceName:           defaultResourceName,
	}
}

// MetadataHandler returns an http.Handler serving the RFC 9728 protected resource metadata document.
func MetadataHandler(cfg *config.Config, scopes []string) http.Handler {
	doc := BuildProtectedResourceMetadata(cfg, scopes)
	payload, err := json.Marshal(doc)
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if err != nil {
			http.Error(w, "failed to encode protected resource metadata", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(payload)
	})
}

// RegisterMetadataRoutes mounts both RFC 9728 well-known endpoints on mux.
func RegisterMetadataRoutes(mux *http.ServeMux, cfg *config.Config, scopes []string) {
	if mux == nil {
		return
	}
	h := MetadataHandler(cfg, scopes)
	mux.Handle("GET "+protectedResourceMetadataPath, h)
	mux.Handle("GET "+protectedResourceMetadataMCPPath, h)
}

func normalizeScopes(scopes []string) []string {
	if len(scopes) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(scopes))
	out := make([]string, 0, len(scopes))
	for _, s := range scopes {
		trimmed := strings.TrimSpace(s)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	slices.Sort(out)
	return out
}
