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

package config

import (
	"slices"
	"strings"
	"testing"
	"time"
)

func clearConfigEnv(t *testing.T) {
	t.Helper()
	keys := []string{
		"SERVER_URL", "TENANT_DOMAIN", "BASE_URL", "ASGARDEO_BASE_URL",
		"CERT_PATH", "TRANSPORT", "PORT", "MCP_STATELESS", "IS_VERSION",
		"MAX_RESPONSE_BYTES", "LOG_LEVEL", "CLIENT_ID", "CLIENT_SECRET",
		"ASGARDEO_CLIENT_ID", "ASGARDEO_CLIENT_SECRET", "PRODUCT_MODE",
		"READ_ONLY", "ALLOW_FLOW_APIS", "INCLUDE_DEPRECATED", "TOOLSETS",
		"SCIM_PROFILE_ATTRIBUTES", "ACCESS_TOKEN", "PORTAL_NAME",
		"MCP_RESOURCE", "OAUTH_ISSUER", "OAUTH_JWKS_URL", "PUBLIC_URL",
		"JWT_CLOCK_SKEW",
	}
	for _, k := range keys {
		t.Setenv(k, "")
	}
}

func TestLoadAdminStdioDefaultsAndFallbacks(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("ASGARDEO_BASE_URL", "https://localhost:9443/t/acme/")
	t.Setenv("ASGARDEO_CLIENT_ID", "cid-1")
	t.Setenv("ASGARDEO_CLIENT_SECRET", "super-secret-value")

	cfg, err := LoadFor(BinaryAdmin)
	if err != nil {
		t.Fatalf("LoadFor(BinaryAdmin) failed: %v", err)
	}

	if cfg.ServerURL != "https://localhost:9443" {
		t.Errorf("ServerURL = %q, want https://localhost:9443", cfg.ServerURL)
	}
	if cfg.TenantDomain != "acme" {
		t.Errorf("TenantDomain = %q, want acme", cfg.TenantDomain)
	}
	if cfg.Transport != TransportStdio {
		t.Errorf("Transport = %q, want stdio", cfg.Transport)
	}
	if cfg.AuthMode != AuthModeClientCredentials {
		t.Errorf("AuthMode = %q, want client_credentials", cfg.AuthMode)
	}
	if cfg.Port != "8000" {
		t.Errorf("Port = %q, want 8000", cfg.Port)
	}
	if cfg.ISVersion != "7.3.0" {
		t.Errorf("ISVersion = %q, want 7.3.0", cfg.ISVersion)
	}
	if cfg.MaxResponseBytes != 100000 {
		t.Errorf("MaxResponseBytes = %d, want 100000", cfg.MaxResponseBytes)
	}
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want info", cfg.LogLevel)
	}
	if cfg.ClientID != "cid-1" || cfg.ClientSecret != "super-secret-value" {
		t.Errorf("unexpected client credentials loaded")
	}
	if cfg.Issuer != "https://localhost:9443/t/acme/oauth2/token" {
		t.Errorf("Issuer = %q, want https://localhost:9443/t/acme/oauth2/token", cfg.Issuer)
	}
	if cfg.JWKSURL != "https://localhost:9443/t/acme/oauth2/jwks" {
		t.Errorf("JWKSURL = %q, want https://localhost:9443/t/acme/oauth2/jwks", cfg.JWKSURL)
	}
	if cfg.JWTClockSkew != 60*time.Second {
		t.Errorf("JWTClockSkew = %v, want 60s", cfg.JWTClockSkew)
	}
	if len(cfg.SCIMProfileAttributes) != 4 {
		t.Errorf("SCIMProfileAttributes len = %d, want 4", len(cfg.SCIMProfileAttributes))
	}
}

func TestLoadCarbonSuperDerivedIssuerAndJWKS(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("SERVER_URL", "https://is.example.com:9443/")
	t.Setenv("CLIENT_ID", "cid")
	t.Setenv("CLIENT_SECRET", "csec")

	cfg, err := LoadFor(BinaryAdmin)
	if err != nil {
		t.Fatalf("LoadFor failed: %v", err)
	}
	if cfg.ServerURL != "https://is.example.com:9443" {
		t.Errorf("ServerURL = %q", cfg.ServerURL)
	}
	if cfg.TenantDomain != "carbon.super" {
		t.Errorf("TenantDomain = %q, want carbon.super", cfg.TenantDomain)
	}
	if cfg.Issuer != "https://is.example.com:9443/oauth2/token" {
		t.Errorf("Issuer = %q", cfg.Issuer)
	}
	if cfg.JWKSURL != "https://is.example.com:9443/oauth2/jwks" {
		t.Errorf("JWKSURL = %q", cfg.JWKSURL)
	}
}

func TestLoadBaseURLConflicts(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
	}{
		{
			name: "server url disagrees with base url",
			env: map[string]string{
				"SERVER_URL":    "https://a.example.com",
				"BASE_URL":      "https://b.example.com/t/acme",
				"CLIENT_ID":     "c",
				"CLIENT_SECRET": "s",
			},
		},
		{
			name: "tenant domain disagrees with base url",
			env: map[string]string{
				"BASE_URL":      "https://a.example.com/t/acme",
				"TENANT_DOMAIN": "other",
				"CLIENT_ID":     "c",
				"CLIENT_SECRET": "s",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clearConfigEnv(t)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			if _, err := LoadFor(BinaryAdmin); err == nil {
				t.Fatalf("expected conflict error, got nil")
			}
		})
	}
}

func TestLoadEnumAndToolsetValidation(t *testing.T) {
	tests := []struct {
		name    string
		binary  Binary
		env     map[string]string
		wantErr string
	}{
		{
			name:   "invalid transport",
			binary: BinaryAdmin,
			env: map[string]string{
				"SERVER_URL":    "https://is.example.com",
				"CLIENT_ID":     "c",
				"CLIENT_SECRET": "s",
				"TRANSPORT":     "grpc",
			},
			wantErr: "TRANSPORT",
		},
		{
			name:   "invalid log level",
			binary: BinaryAdmin,
			env: map[string]string{
				"SERVER_URL":    "https://is.example.com",
				"CLIENT_ID":     "c",
				"CLIENT_SECRET": "s",
				"LOG_LEVEL":     "trace",
			},
			wantErr: "LOG_LEVEL",
		},
		{
			name:   "invalid toolset",
			binary: BinaryAdmin,
			env: map[string]string{
				"SERVER_URL":    "https://is.example.com",
				"CLIENT_ID":     "c",
				"CLIENT_SECRET": "s",
				"TOOLSETS":      "applications,nonexistent_area",
			},
			wantErr: "TOOLSETS",
		},
		{
			name:   "admin http missing mcp resource",
			binary: BinaryAdmin,
			env: map[string]string{
				"SERVER_URL": "https://is.example.com",
				"TRANSPORT":  "http",
			},
			wantErr: "MCP_RESOURCE",
		},
		{
			name:   "self-service stdio missing access token",
			binary: BinarySelfService,
			env: map[string]string{
				"SERVER_URL": "https://is.example.com",
				"TRANSPORT":  "stdio",
			},
			wantErr: "ACCESS_TOKEN",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			clearConfigEnv(t)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			_, err := LoadFor(tc.binary)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error %q does not mention %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestLoadToolsetsValid(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("SERVER_URL", "https://is.example.com")
	t.Setenv("TRANSPORT", "http")
	t.Setenv("MCP_RESOURCE", "http://localhost:8000/mcp")
	t.Setenv("TOOLSETS", "applications, scim2_bad_no, users")
	// Test valid list first:
	t.Setenv("TOOLSETS", "applications, users, roles")

	cfg, err := LoadFor(BinaryAdmin)
	if err != nil {
		t.Fatalf("LoadFor failed: %v", err)
	}
	want := []string{"applications", "users", "roles"}
	if !slices.Equal(cfg.Toolsets, want) {
		t.Errorf("Toolsets = %v, want %v", cfg.Toolsets, want)
	}

	t.Setenv("TOOLSETS", "all")
	cfg, err = LoadFor(BinaryAdmin)
	if err != nil {
		t.Fatalf("LoadFor(all) failed: %v", err)
	}
	if !slices.Equal(cfg.Toolsets, []string{"all"}) {
		t.Errorf("Toolsets = %v, want [all]", cfg.Toolsets)
	}
}

func TestErrorsNeverLeakSecrets(t *testing.T) {
	clearConfigEnv(t)
	secret := "very-sensitive-client-secret-999"
	token := "very-sensitive-access-token-888"
	t.Setenv("SERVER_URL", "https://is.example.com")
	t.Setenv("CLIENT_SECRET", secret)
	t.Setenv("ACCESS_TOKEN", token)
	t.Setenv("TRANSPORT", "invalid")

	_, err := LoadFor(BinaryAdmin)
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), token) {
		t.Fatalf("error leaked secret: %v", err)
	}
}

func TestSelfServiceHTTPAllowsEmptyMCPResource(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("SERVER_URL", "https://is.example.com")
	t.Setenv("PORT", "9000")

	cfg, err := LoadFor(BinarySelfService)
	if err != nil {
		t.Fatalf("LoadFor(BinarySelfService) failed: %v", err)
	}
	if cfg.Transport != TransportHTTP {
		t.Errorf("Transport = %q, want http", cfg.Transport)
	}
	if cfg.AuthMode != AuthModeBearer {
		t.Errorf("AuthMode = %q, want bearer", cfg.AuthMode)
	}
	if cfg.PublicURL != "http://localhost:9000" {
		t.Errorf("PublicURL = %q, want http://localhost:9000", cfg.PublicURL)
	}
	if cfg.PortalName != "self-service portal" {
		t.Errorf("PortalName = %q", cfg.PortalName)
	}
}
