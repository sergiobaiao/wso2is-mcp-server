# Data Model: Full WSO2 Identity Server 7.3 Coverage

The feature stores nothing persistently. These are the in-memory models built at startup and
per request. Field names are the Go field names to use; types are Go types.

## SpecFile

One vendored OpenAPI document.

| Field | Type | Rules |
|---|---|---|
| `Family` | `Family` (`root` \| `org`) | From the directory (`api-specs/wso2is/<v>/root` or `/org`) |
| `Name` | `string` | File name without extension, e.g. `scim2-users` |
| `Area` | `string` | From the area table (below); every file MUST map to exactly one area |
| `PathPrefix` | `string` | e.g. `/api/server/v1`, `/scim2`, `/o/scim2`, `/api/users/v1/me/push`; derived per R2 |
| `Deprecated` | `bool` | Per R11 |
| `Flow` | `bool` | Per R10 (whole-file flow specs; operation-level exceptions such as `pi-info` are set on Operation.Flow) |
| `Doc` | `*openapi3.T` | Loaded; Swagger 2.0 converted |

## Operation

One HTTP method on one path. Built from `SpecFile` paths plus `catalog/extra.go`.

| Field | Type | Rules |
|---|---|---|
| `ID` | `string` | `<area>.<operationId>` for root, `<area>.org.<operationId>` for org; unique (test) |
| `OperationID` | `string` | Normalized per R2; lowerCamelCase ASCII |
| `Area` | `string` | From `SpecFile.Area` |
| `Family` | `Family` | From `SpecFile` |
| `Method` | `string` | Upper case |
| `Path` | `string` | Template relative to tenant root, including `PathPrefix`, e.g. `/scim2/Users/{id}` |
| `Summary` | `string` | `summary`, else first sentence of `description`, max 200 chars |
| `Description` | `string` | HTML tags stripped |
| `Kind` | `Kind` (`read` \| `write` \| `destructive`) | Per R9 |
| `Scopes` | `[]string` | Sorted, unique, per R2 |
| `Params` | `[]Param` | Path, query and header parameters (cookie parameters ignored) |
| `Body` | `*BodySpec` | nil when the operation has no request body |
| `Deprecated` | `bool` | File-level or operation-level flag |
| `Flow` | `bool` | Per R10 |
| `AIFeature` | `bool` | Only for the Login Flow AI extras |
| `Source` | `string` | `root/scim2-users.yaml#GET /Users/{id}` for traceability |

Derived:

- `Availability()` → `available` \| `flow_disabled` \| `deprecated_hidden` \| `read_only_hidden`
  given the runtime `Config`.
- `OrgScoped()` → `Family == org`.

## Param

| Field | Type | Rules |
|---|---|---|
| `Name` | `string` | As in the spec (`session-id`, `startIndex`) |
| `In` | `string` | `path` \| `query` \| `header` |
| `Required` | `bool` | Path params always true |
| `Schema` | `*openapi3.SchemaRef` | Used for validation and JSON-schema output |

Headers named `Authorization`, `Content-Type`, `Accept` are never accepted from the agent.

## BodySpec

| Field | Type | Rules |
|---|---|---|
| `ContentType` | `string` | First of `application/scim+json`, `application/json`, `application/x-www-form-urlencoded`, `multipart/form-data` present in the spec |
| `Schema` | `*openapi3.SchemaRef` | |
| `Required` | `bool` | |

`multipart/form-data` bodies (keystore and branding uploads) accept base64 strings for binary
fields; the client encodes them.

## Area table

Every `SpecFile.Name` maps to one area. Counts are operations as published (root/org).

| Area | Files | Root | Org |
|---|---|---|---|
| `applications` | application, org-application-mgt, authorized-apps, authorized-apps-v2, oauth-dcr, script-library, discoverable-application | 87 | 19 |
| `api_authorization` | api-resources, oauth2-scope-endpoint, oidc-scope-management, permission-management | 23 | 4 |
| `users` | scim2-users, user-management, scim2-bulk, scim2-me, scim2-resource-types, scim2-schemas, scim2-sp-configs | 18 | 8 |
| `groups` | scim2-groups, group-management | 7 | 7 |
| `roles` | roles-v2, roles-v1, role-management | 14 | 7 |
| `agents` | scim2-agents | 6 | 0 |
| `user_lifecycle` | offline-user-onboard-mgt-api, org-offline-user-onboard-mgt, end-user-credential-management, user-password, org-user-password, functionality, idle-accounts-identification, association-management-by-admin, association-management-by-user, session-mgt-by-admin, session-mgt-by-user | 31 | 12 |
| `identity_providers` | idp, authenticators, identity-verification-providers, admin-identity-verification, user-identity-verification, certificate-validation-management | 67 | 40 |
| `claims` | claim-management, claim-mgt | 18 | 9 |
| `user_stores` | user-store | 15 | 15 |
| `governance_security` | identity-governance, validation-rules, configs, cors, keystore, admin-advisory-management, rule-metadata | 53 | 20 |
| `branding_notifications` | branding-preference, branding-management, email-templates, email-templates-v2, notification-templates, notification-senders, notification-senders-v2 | 97 | 95 |
| `extensions` | actions, webhook-management, webhook-metadata, extension-mgt | 22 | 8 |
| `organizations` | organization-management, organization-discovery, organization-discovery-config-mgt, organization-user-share, organization-user-share-v2, organization-user-invitation, user-organization | 36 | 29 |
| `workflows_approvals` | workflow, workflow-engine, approvals | 19 | 0 |
| `consents` | consent-management, consent-management-v2, user-consent | 41 | 0 |
| `tenants` | tenant-management | 10 | 0 |
| `verifiable_credentials` | vc-templates | 7 | 0 |
| `self_service_me` | fido, totp, push-notification-devices | 16 | 0 |
| `flows` | app-native-authentication-api, authentication-api, authentication, self-service, self-sign-up, account-recovery, user-account-recovery-v1, user-account-recovery-v2, verification-code-management, verification-code-management-me | 56 | 5 |
| **Total** | 116 files | **643** | **278** |

## Catalog

| Field | Type | Rules |
|---|---|---|
| `Version` | `string` | `IS_VERSION`, default `7.3.0` |
| `Source` | `SourceInfo` | From `SOURCE` (repository, commit, paths) |
| `Operations` | `[]*Operation` | Sorted by `ID` |
| `byID` | `map[string]*Operation` | |

Methods: `Get(id)`, `Search(query, filter) []*Operation`, `Areas()`, `Stats()`.

## Config

Loaded once from environment (see contracts/configuration.md).

| Field | Type |
|---|---|
| `ServerURL`, `TenantDomain` | `string` |
| `AuthMode` | `client_credentials` \| `static_token` \| `bearer` (derived from transport and binary) |
| `ClientID`, `ClientSecret`, `AccessToken` | `string` (never logged) |
| `CertPath` | `string` |
| `Transport` | `stdio` \| `http`; `Port`; `Stateless` |
| `Issuer`, `JWKSURL`, `Resource` | `string` (HTTP mode) |
| `ReadOnly`, `AllowFlowAPIs`, `IncludeDeprecated` | `bool` |
| `Toolsets` | `[]string` (validated against areas; `all` allowed) |
| `MaxResponseBytes` | `int` |
| `SCIMProfileAttributes` | `[]string` |
| `PortalName` | `string` (self-service) |
| `ISVersion` | `string` |

## CredentialContext

Per call, carried on `context.Context`.

| Field | Type | Rules |
|---|---|---|
| `Mode` | as `Config.AuthMode` | |
| `BearerToken` | `string` | Only in `bearer` mode, already validated |
| `Subject` | `string` | `sub` claim, for logs (never the token) |
| `OrganizationID` | `string` | Empty for root |

## OrgTokenCache entry

| Field | Type | Rules |
|---|---|---|
| Key | `sha256(root token) + ":" + orgID` | Root token never stored in clear as key |
| `AccessToken` | `string` | |
| `ExpiresAt` | `time.Time` | Evict 30 s before |

## Tool

What `mcp-go` registers.

| Field | Type | Rules |
|---|---|---|
| `Name` | `string` | snake_case, ≤ 64, unique across all layers |
| `Layer` | `curated` \| `catalog` \| `operation` | |
| `Kind` | `Kind` | Drives annotations and elicitation |
| `Scopes` | `[]string` | Curated: declared; operation layer: from `Operation.Scopes` |
| `OrgCapable` | `bool` | Adds `organization_id` input |
| `Description` | `string` | Write/destructive get the confirmation sentence appended |
| `Handler` | `func(ctx, args) (any, error)` | Result passes through redact and truncate |

## State transitions

### Write call

```
received → validated (schema) → [elicitation supported?]
   yes → elicit(summary) → accept → sent → result | decline/cancel → not_sent ("not approved")
   no  → sent → result
validated fails → rejected (fields)      flow & !AllowFlowAPIs → refused (reason)
```

### HTTP request

```
received → bearer? no → 401 (resource_metadata)
         → JWT valid (sig, iss, aud, exp, nbf)? no → 401 invalid_token
         → MCP handled with CredentialContext{bearer}
```
