/*
 * Copyright (c) 2025, WSO2 LLC. (http://www.wso2.com).
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

// Package config loads and validates runtime configuration from environment variables.
package config

import (
	"fmt"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Binary identifies which MCP server binary is loading configuration.
type Binary string

const (
	BinaryAdmin       Binary = "wso2is-mcp"
	BinarySelfService Binary = "self-service-mcp"
)

// AuthMode identifies how the server authenticates requests to IS.
type AuthMode string

const (
	AuthModeClientCredentials AuthMode = "client_credentials"
	AuthModeStaticToken       AuthMode = "static_token"
	AuthModeBearer            AuthMode = "bearer"
)

// Transport identifies the MCP server transport.
type Transport string

const (
	TransportStdio Transport = "stdio"
	TransportHTTP  Transport = "http"
)

// Config holds the validated configuration for either MCP server binary.
type Config struct {
	ServerURL             string
	TenantDomain          string
	AuthMode              AuthMode
	ClientID              string
	ClientSecret          string
	AccessToken           string
	CertPath              string
	Transport             Transport
	Port                  string
	Stateless             bool
	Issuer                string
	JWKSURL               string
	Resource              string
	PublicURL             string
	JWTClockSkew          time.Duration
	ProductMode           string
	ReadOnly              bool
	AllowFlowAPIs         bool
	IncludeDeprecated     bool
	Toolsets              []string
	MaxResponseBytes      int
	SCIMProfileAttributes []string
	PortalName            string
	ISVersion             string
	LogLevel              string
}

// LoadFor reads and validates environment configuration for the given binary.
func LoadFor(bin Binary) (*Config, error) {
	serverURL, tenantDomain, err := loadURLAndTenant(bin)
	if err != nil {
		return nil, err
	}

	transport, err := loadTransport(bin)
	if err != nil {
		return nil, err
	}

	authMode, clientID, clientSecret, accessToken, err := loadAuth(bin, transport)
	if err != nil {
		return nil, err
	}

	port := getEnvDefault(PORT_PARAM, DefaultPort)

	stateless, err := parseBoolEnv(MCP_STATELESS_PARAM, false)
	if err != nil {
		return nil, err
	}
	readOnly, err := parseBoolEnv(READ_ONLY_PARAM, false)
	if err != nil {
		return nil, err
	}
	allowFlow, err := parseBoolEnv(ALLOW_FLOW_APIS_PARAM, false)
	if err != nil {
		return nil, err
	}
	includeDeprecated, err := parseBoolEnv(INCLUDE_DEPRECATED_PARAM, false)
	if err != nil {
		return nil, err
	}

	maxBytes, err := parsePositiveIntEnv(MAX_RESPONSE_BYTES_PARAM, DefaultMaxResponseBytes)
	if err != nil {
		return nil, err
	}

	logLevel, err := loadLogLevel()
	if err != nil {
		return nil, err
	}

	productMode, err := loadProductMode()
	if err != nil {
		return nil, err
	}

	toolsets, err := loadToolsets()
	if err != nil {
		return nil, err
	}

	skew, err := parseDurationEnv(JWT_CLOCK_SKEW_PARAM, 60*time.Second)
	if err != nil {
		return nil, err
	}

	resource := strings.TrimSpace(os.Getenv(MCP_RESOURCE_PARAM))
	if bin == BinaryAdmin && transport == TransportHTTP && resource == "" {
		return nil, fmt.Errorf("missing required environment variable %s for HTTP transport", MCP_RESOURCE_PARAM)
	}

	issuerBase := serverURL
	if tenantDomain != DefaultTenantDomain {
		issuerBase = serverURL + "/t/" + tenantDomain
	}

	issuer := strings.TrimRight(strings.TrimSpace(os.Getenv(OAUTH_ISSUER_PARAM)), "/")
	if issuer == "" {
		issuer = issuerBase + "/oauth2/token"
	}

	jwksURL := strings.TrimRight(strings.TrimSpace(os.Getenv(OAUTH_JWKS_URL_PARAM)), "/")
	if jwksURL == "" {
		jwksURL = issuerBase + "/oauth2/jwks"
	}

	publicURL := strings.TrimRight(strings.TrimSpace(os.Getenv(PUBLIC_URL_PARAM)), "/")
	if publicURL == "" {
		publicURL = "http://localhost:" + port
	}

	scimAttrsRaw := strings.TrimSpace(os.Getenv(SCIM_PROFILE_ATTRIBUTES_PARAM))
	if scimAttrsRaw == "" {
		scimAttrsRaw = DefaultSCIMProfileAttributes
	}
	scimAttrs := splitCSV(scimAttrsRaw)

	return &Config{
		ServerURL:             serverURL,
		TenantDomain:          tenantDomain,
		AuthMode:              authMode,
		ClientID:              clientID,
		ClientSecret:          clientSecret,
		AccessToken:           accessToken,
		CertPath:              strings.TrimSpace(os.Getenv(CERTIFICATE_PATH_PARAM)),
		Transport:             transport,
		Port:                  port,
		Stateless:             stateless,
		Issuer:                issuer,
		JWKSURL:               jwksURL,
		Resource:              resource,
		PublicURL:             publicURL,
		JWTClockSkew:          skew,
		ProductMode:           productMode,
		ReadOnly:              readOnly,
		AllowFlowAPIs:         allowFlow,
		IncludeDeprecated:     includeDeprecated,
		Toolsets:              toolsets,
		MaxResponseBytes:      maxBytes,
		SCIMProfileAttributes: scimAttrs,
		PortalName:            getEnvDefault(PORTAL_NAME_PARAM, DefaultPortalName),
		ISVersion:             getEnvDefault(IS_VERSION_PARAM, DefaultISVersion),
		LogLevel:              logLevel,
	}, nil
}

// GetProductName returns the display name of the configured identity server product.
func GetProductName() string {
	productMode := os.Getenv(PRODUCT_MODE_PARAM)
	if productMode == ProductModes.WSO2IS {
		return ProductNames.WSO2IS
	}
	return ProductNames.Asgardeo
}

// TenantBaseURL returns the base URL including /t/<tenant> when tenant is non-default.
func (c *Config) TenantBaseURL() string {
	if c.TenantDomain == "" || c.TenantDomain == DefaultTenantDomain {
		return c.ServerURL
	}
	return c.ServerURL + "/t/" + c.TenantDomain
}

func loadURLAndTenant(bin Binary) (string, string, error) {
	rawServerURL := strings.TrimRight(strings.TrimSpace(os.Getenv(SERVER_URL_PARAM)), "/")
	rawTenant := strings.TrimSpace(os.Getenv(TENANT_DOMAIN_PARAM))

	rawBaseURL := strings.TrimRight(strings.TrimSpace(os.Getenv(BASE_URL_PARAM)), "/")
	if rawBaseURL == "" && bin == BinaryAdmin {
		rawBaseURL = strings.TrimRight(strings.TrimSpace(os.Getenv(ASGARDEO_BASE_URL_PARAM)), "/")
	}

	var baseServerURL, baseTenant string
	if rawBaseURL != "" {
		u, err := url.Parse(rawBaseURL)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return "", "", fmt.Errorf("invalid %s: must be a valid URL", BASE_URL_PARAM)
		}
		baseServerURL = u.Scheme + "://" + u.Host
		path := strings.TrimRight(u.Path, "/")
		if path != "" {
			if !strings.HasPrefix(path, "/t/") {
				return "", "", fmt.Errorf("invalid %s: path must be empty or /t/<tenant>", BASE_URL_PARAM)
			}
			t := strings.TrimPrefix(path, "/t/")
			if t == "" || strings.Contains(t, "/") {
				return "", "", fmt.Errorf("invalid %s: invalid tenant path", BASE_URL_PARAM)
			}
			baseTenant = t
		}
	}

	if rawServerURL != "" {
		u, err := url.Parse(rawServerURL)
		if err != nil || u.Scheme == "" || u.Host == "" || (u.Path != "" && u.Path != "/") {
			return "", "", fmt.Errorf("invalid %s: must be https://<host>[:port]", SERVER_URL_PARAM)
		}
		rawServerURL = u.Scheme + "://" + u.Host
		if baseServerURL != "" && rawServerURL != baseServerURL {
			return "", "", fmt.Errorf("%s (%q) and %s (%q) disagree", SERVER_URL_PARAM, rawServerURL, BASE_URL_PARAM, baseServerURL)
		}
	} else {
		rawServerURL = baseServerURL
	}

	if rawServerURL == "" {
		return "", "", fmt.Errorf("missing required environment variable %s or %s", SERVER_URL_PARAM, BASE_URL_PARAM)
	}

	if rawTenant != "" && baseTenant != "" && rawTenant != baseTenant {
		return "", "", fmt.Errorf("%s (%q) and %s tenant (%q) disagree", TENANT_DOMAIN_PARAM, rawTenant, BASE_URL_PARAM, baseTenant)
	}

	tenant := rawTenant
	if tenant == "" {
		tenant = baseTenant
	}
	if tenant == "" {
		tenant = DefaultTenantDomain
	}

	return rawServerURL, tenant, nil
}

func loadTransport(bin Binary) (Transport, error) {
	def := TransportStdio
	if bin == BinarySelfService {
		def = TransportHTTP
	}
	raw := strings.TrimSpace(os.Getenv(TRANSPORT_PARAM))
	if raw == "" {
		return def, nil
	}
	switch Transport(raw) {
	case TransportStdio, TransportHTTP:
		return Transport(raw), nil
	default:
		return "", fmt.Errorf("invalid %s=%q: allowed values are stdio, http", TRANSPORT_PARAM, raw)
	}
}

func loadAuth(bin Binary, transport Transport) (AuthMode, string, string, string, error) {
	clientID := strings.TrimSpace(os.Getenv(CLIENT_ID_PARAM))
	if clientID == "" && bin == BinaryAdmin {
		clientID = strings.TrimSpace(os.Getenv(ASGARDEO_CLIENT_ID_PARAM))
	}
	clientSecret := strings.TrimSpace(os.Getenv(CLIENT_SECRET_PARAM))
	if clientSecret == "" && bin == BinaryAdmin {
		clientSecret = strings.TrimSpace(os.Getenv(ASGARDEO_CLIENT_SECRET_PARAM))
	}
	accessToken := strings.TrimSpace(os.Getenv(ACCESS_TOKEN_PARAM))

	if transport == TransportHTTP {
		return AuthModeBearer, clientID, clientSecret, "", nil
	}
	if bin == BinarySelfService {
		if accessToken == "" {
			return "", "", "", "", fmt.Errorf("missing required environment variable %s", ACCESS_TOKEN_PARAM)
		}
		return AuthModeStaticToken, clientID, "", accessToken, nil
	}
	if clientID == "" || clientSecret == "" {
		return "", "", "", "", fmt.Errorf("missing required environment variables %s and %s", CLIENT_ID_PARAM, CLIENT_SECRET_PARAM)
	}
	return AuthModeClientCredentials, clientID, clientSecret, "", nil
}

func loadLogLevel() (string, error) {
	raw := strings.TrimSpace(os.Getenv(LOG_LEVEL_PARAM))
	if raw == "" {
		return DefaultLogLevel, nil
	}
	switch raw {
	case "debug", "info", "warn", "error":
		return raw, nil
	default:
		return "", fmt.Errorf("invalid %s=%q: allowed values are debug, info, warn, error", LOG_LEVEL_PARAM, raw)
	}
}

func loadProductMode() (string, error) {
	raw := strings.TrimSpace(os.Getenv(PRODUCT_MODE_PARAM))
	if raw == "" {
		return ProductModes.WSO2IS, nil
	}
	switch raw {
	case ProductModes.WSO2IS, ProductModes.Asgardeo:
		return raw, nil
	default:
		return "", fmt.Errorf("invalid %s=%q: allowed values are wso2is, asgardeo", PRODUCT_MODE_PARAM, raw)
	}
}

func loadToolsets() ([]string, error) {
	raw := strings.TrimSpace(os.Getenv(TOOLSETS_PARAM))
	if raw == "" {
		return nil, nil
	}
	items := splitCSV(raw)
	if len(items) == 1 && items[0] == "all" {
		return items, nil
	}
	for _, item := range items {
		if !slices.Contains(KnownAreas, item) {
			return nil, fmt.Errorf("invalid %s entry %q: allowed values are %s or all", TOOLSETS_PARAM, item, strings.Join(KnownAreas, ", "))
		}
	}
	return items, nil
}

func getEnvDefault(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func parseBoolEnv(key string, def bool) (bool, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def, nil
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return false, fmt.Errorf("invalid %s=%q: expected boolean", key, raw)
	}
	return v, nil
}

func parsePositiveIntEnv(key string, def int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return 0, fmt.Errorf("invalid %s=%q: expected positive integer", key, raw)
	}
	return v, nil
}

func parseDurationEnv(key string, def time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return def, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d < 0 {
		return 0, fmt.Errorf("invalid %s=%q: expected non-negative duration", key, raw)
	}
	return d, nil
}

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}
