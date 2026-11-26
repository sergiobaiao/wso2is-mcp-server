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

package config

const (
	SERVER_URL_PARAM              = "SERVER_URL"
	TENANT_DOMAIN_PARAM           = "TENANT_DOMAIN"
	BASE_URL_PARAM                = "BASE_URL"
	CLIENT_ID_PARAM               = "CLIENT_ID"
	CLIENT_SECRET_PARAM           = "CLIENT_SECRET"
	CERTIFICATE_PATH_PARAM        = "CERT_PATH"
	PRODUCT_MODE_PARAM            = "PRODUCT_MODE"
	ACCESS_TOKEN_PARAM            = "ACCESS_TOKEN"
	TRANSPORT_PARAM               = "TRANSPORT"
	PORT_PARAM                    = "PORT"
	MCP_STATELESS_PARAM           = "MCP_STATELESS"
	IS_VERSION_PARAM              = "IS_VERSION"
	MAX_RESPONSE_BYTES_PARAM      = "MAX_RESPONSE_BYTES"
	LOG_LEVEL_PARAM               = "LOG_LEVEL"
	READ_ONLY_PARAM               = "READ_ONLY"
	ALLOW_FLOW_APIS_PARAM         = "ALLOW_FLOW_APIS"
	INCLUDE_DEPRECATED_PARAM      = "INCLUDE_DEPRECATED"
	TOOLSETS_PARAM                = "TOOLSETS"
	SCIM_PROFILE_ATTRIBUTES_PARAM = "SCIM_PROFILE_ATTRIBUTES"
	PORTAL_NAME_PARAM             = "PORTAL_NAME"
	MCP_RESOURCE_PARAM            = "MCP_RESOURCE"
	OAUTH_ISSUER_PARAM            = "OAUTH_ISSUER"
	OAUTH_JWKS_URL_PARAM          = "OAUTH_JWKS_URL"
	PUBLIC_URL_PARAM              = "PUBLIC_URL"
	JWT_CLOCK_SKEW_PARAM          = "JWT_CLOCK_SKEW"
)

// Deprecated constants for backward compatibility
const (
	ASGARDEO_BASE_URL_PARAM      = "ASGARDEO_BASE_URL"
	ASGARDEO_CLIENT_ID_PARAM     = "ASGARDEO_CLIENT_ID"
	ASGARDEO_CLIENT_SECRET_PARAM = "ASGARDEO_CLIENT_SECRET"
)

const (
	DefaultTenantDomain     = "carbon.super"
	DefaultPort             = "8000"
	DefaultISVersion        = "7.3.0"
	DefaultMaxResponseBytes = 100000
	DefaultLogLevel         = "info"
	DefaultPortalName       = "self-service portal"
)

const DefaultSCIMProfileAttributes = "urn:ietf:params:scim:schemas:extension:enterprise:2.0:User:department," +
	"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User:employeeNumber," +
	"urn:ietf:params:scim:schemas:extension:enterprise:2.0:User:costCenter," +
	"addresses.work"

var ProductModes = struct {
	WSO2IS   string
	Asgardeo string
}{
	WSO2IS:   "wso2is",
	Asgardeo: "asgardeo",
}

var ProductNames = struct {
	WSO2IS   string
	Asgardeo string
}{
	WSO2IS:   "WSO2 Identity Server",
	Asgardeo: "Asgardeo",
}

// KnownAreas lists every API area defined in the catalog area table.
var KnownAreas = []string{
	"applications",
	"api_authorization",
	"users",
	"groups",
	"roles",
	"agents",
	"user_lifecycle",
	"identity_providers",
	"claims",
	"user_stores",
	"governance_security",
	"branding_notifications",
	"extensions",
	"organizations",
	"workflows_approvals",
	"consents",
	"tenants",
	"verifiable_credentials",
	"self_service_me",
	"flows",
}
