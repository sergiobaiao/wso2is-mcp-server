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

// Package catalog loads, normalizes, and searches vendored WSO2 IS OpenAPI specifications.
package catalog

import "github.com/getkin/kin-openapi/openapi3"

// Family identifies whether a specification targets the root tenant or sub-organizations.
type Family string

const (
	FamilyRoot Family = "root"
	FamilyOrg  Family = "org"
)

// SpecFile represents one loaded OpenAPI document.
type SpecFile struct {
	Family     Family
	Name       string
	FileName   string
	Area       string
	PathPrefix string
	Deprecated bool
	Flow       bool
	Doc        *openapi3.T
}

var fileToArea = map[string]string{
	// applications
	"application":              "applications",
	"org-application-mgt":      "applications",
	"authorized-apps":          "applications",
	"authorized-apps-v2":       "applications",
	"oauth-dcr":                "applications",
	"script-library":           "applications",
	"discoverable-application": "applications",

	// api_authorization
	"api-resources":         "api_authorization",
	"oauth2-scope-endpoint": "api_authorization",
	"oidc-scope-management": "api_authorization",
	"permission-management": "api_authorization",

	// users
	"scim2-users":          "users",
	"user-management":      "users",
	"scim2-bulk":           "users",
	"scim2-me":             "users",
	"scim2-resource-types": "users",
	"scim2-schemas":        "users",
	"scim2-sp-configs":     "users",

	// groups
	"scim2-groups":     "groups",
	"group-management": "groups",

	// roles
	"roles-v2":        "roles",
	"roles-v1":        "roles",
	"role-management": "roles",

	// agents
	"scim2-agents": "agents",

	// user_lifecycle
	"offline-user-onboard-mgt-api":    "user_lifecycle",
	"org-offline-user-onboard-mgt":    "user_lifecycle",
	"end-user-credential-management":  "user_lifecycle",
	"user-password":                   "user_lifecycle",
	"org-user-password":               "user_lifecycle",
	"functionality":                   "user_lifecycle",
	"idle-accounts-identification":    "user_lifecycle",
	"association-management-by-admin": "user_lifecycle",
	"association-management-by-user":  "user_lifecycle",
	"session-mgt-by-admin":            "user_lifecycle",
	"session-mgt-by-user":             "user_lifecycle",

	// identity_providers
	"idp":                               "identity_providers",
	"authenticators":                    "identity_providers",
	"identity-verification-providers":   "identity_providers",
	"admin-identity-verification":       "identity_providers",
	"user-identity-verification":        "identity_providers",
	"certificate-validation-management": "identity_providers",

	// claims
	"claim-management": "claims",
	"claim-mgt":        "claims",

	// user_stores
	"user-store": "user_stores",

	// governance_security
	"identity-governance":       "governance_security",
	"validation-rules":          "governance_security",
	"configs":                   "governance_security",
	"cors":                      "governance_security",
	"keystore":                  "governance_security",
	"admin-advisory-management": "governance_security",
	"rule-metadata":             "governance_security",

	// branding_notifications
	"branding-preference":     "branding_notifications",
	"branding-management":     "branding_notifications",
	"email-templates":         "branding_notifications",
	"email-templates-v2":      "branding_notifications",
	"notification-templates":  "branding_notifications",
	"notification-senders":    "branding_notifications",
	"notification-senders-v2": "branding_notifications",

	// extensions
	"actions":            "extensions",
	"webhook-management": "extensions",
	"webhook-metadata":   "extensions",
	"extension-mgt":      "extensions",

	// organizations
	"organization-management":           "organizations",
	"organization-discovery":            "organizations",
	"organization-discovery-config-mgt": "organizations",
	"organization-user-share":           "organizations",
	"organization-user-share-v2":        "organizations",
	"organization-user-invitation":      "organizations",
	"user-organization":                 "organizations",

	// workflows_approvals
	"workflow":        "workflows_approvals",
	"workflow-engine": "workflows_approvals",
	"approvals":       "workflows_approvals",

	// consents
	"consent-management":    "consents",
	"consent-management-v2": "consents",
	"user-consent":          "consents",

	// tenants
	"tenant-management": "tenants",

	// verifiable_credentials
	"vc-templates": "verifiable_credentials",

	// self_service_me
	"fido":                      "self_service_me",
	"totp":                      "self_service_me",
	"push-notification-devices": "self_service_me",

	// flows
	"app-native-authentication-api":   "flows",
	"authentication-api":              "flows",
	"authentication":                  "flows",
	"self-service":                    "flows",
	"self-sign-up":                    "flows",
	"account-recovery":                "flows",
	"user-account-recovery-v1":        "flows",
	"user-account-recovery-v2":        "flows",
	"verification-code-management":    "flows",
	"verification-code-management-me": "flows",
}

// prefixOverrides corrects specs with missing, malformed, or misleading servers entries (research.md R2).
var prefixOverrides = map[string]string{
	"root/scim2-schemas":                 "/scim2",
	"root/authentication-api":            "/oauth2",
	"root/admin-identity-verification":   "/api/users/v1",
	"root/user-identity-verification":    "/api/users/v1",
	"root/oauth-dcr":                     "/api/identity/oauth2/dcr/v1.1",
	"org/end-user-credential-management": "/o/api/server/v1",
	"org/rule-metadata":                  "/o/api/server/v1",
}

var flowSpecs = map[string]bool{
	"app-native-authentication-api":   true,
	"authentication-api":              true,
	"authentication":                  true,
	"self-service":                    true,
	"self-sign-up":                    true,
	"account-recovery":                true,
	"user-account-recovery-v1":        true,
	"user-account-recovery-v2":        true,
	"verification-code-management":    true,
	"verification-code-management-me": true,
}

var deprecatedSpecs = map[string]bool{
	"roles-v1":                 true,
	"notification-senders":     true,
	"organization-user-share":  true,
	"user-account-recovery-v1": true,
}
