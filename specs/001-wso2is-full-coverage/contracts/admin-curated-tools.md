# Contract: Curated Administration Tools (non-SCIM)

Budget (SC-002, R13): existing 19 + SCIM 21 − 1 (merged `create_user`) + SCIM gaps 8 + new 43 =
90 curated, + 4 catalog = 94 default tools.

## Existing tools (frozen names, FR-009)

`list_applications`, `create_single_page_app`, `create_webapp_with_ssr`, `create_mobile_app`,
`create_m2m_app`, `get_application_by_name`, `get_application_by_client_id`,
`update_application_basic_info`, `update_application_oauth_config`,
`update_application_claim_config`, `authorize_api`, `list_authorized_api`, `update_login_flow`,
`list_api_resources`, `search_api_resources_by_name`, `get_api_resource_by_identifier`,
`create_api_resource`, `create_user` (now in scim-tools.md), `list_claims`.

Inputs and outputs stay as documented in `docs/asgardeo-mcp.md` (to be renamed
`docs/wso2is-mcp.md`). Changes allowed: add optional `organization_id`; add annotations; the
OAuth endpoints in outputs use `SERVER_URL` and `TENANT_DOMAIN`.

## New tools by area

Each row: name, kind, IS operation(s) by catalog id prefix (exact ids fixed at implementation and
asserted by a test that every referenced id exists).

### applications (2)

| Tool | Kind | Operation |
|---|---|---|
| `delete_application` | destructive | `applications.deleteApplication` |
| `share_application` | write | `applications.shareOrgApplication` (share with all or listed orgs; root organization only) |

### api_authorization (5)

| Tool | Kind | Operation |
|---|---|---|
| `update_api_resource` | write | `api_authorization.patchApiResourcesByApiresourceid` |
| `add_api_resource_scopes` | write | `api_authorization.getApiResourcesByApiresourceidScopes` then `putApiResourcesByApiresourceidScopes` (merge with existing by name) |
| `delete_api_resource` | destructive | `api_authorization.deleteApiResourcesByApiresourceid` |
| `list_oidc_scopes` | read | `api_authorization.getApiServerV1OidcScopes` (OIDC) |
| `create_oidc_scope` | write | `api_authorization.addScope` (OIDC) |

### identity_providers (5)

| Tool | Kind | Operation |
|---|---|---|
| `list_identity_providers` | read | `identity_providers.getIDPs` |
| `get_identity_provider` | read | `identity_providers.getIDP` |
| `create_identity_provider` | write | `identity_providers.addIDP` with `template` ∈ `google`, `github`, `facebook`, `microsoft`, `apple`, `oidc`, `saml` and template-specific inputs (client id; client secret accepted as input, never returned) |
| `delete_identity_provider` | destructive | `identity_providers.deleteIDP` |
| `list_authenticators` | read | `identity_providers.getAuthenticators` |

### governance_security (4)

| Tool | Kind | Operation |
|---|---|---|
| `list_governance_connectors` | read | `governance_security.getCategories`, or `getConnectorsOfCategory` with `category_id` |
| `get_governance_connector` | read | `governance_security.getConnector` |
| `update_governance_connector` | write | `governance_security.patchConnector` (password policy, account lock, self registration, recovery are connectors) |
| `update_validation_rules` | write | `governance_security.updateValidationRules` |

### user_stores (4)

| Tool | Kind | Operation |
|---|---|---|
| `list_user_stores` | read | `user_stores.getSecondaryUserStores` |
| `get_user_store` | read | `user_stores.getUserStoreByDomainId` |
| `create_user_store` | write | `user_stores.addUserStore` (`type_name` resolved through `getAvailableUserStoreTypes`) |
| `test_user_store_connection` | read | `user_stores.testRDBMSConnection` (POST, overridden to read) |

### user_lifecycle (2)

| Tool | Kind | Operation |
|---|---|---|
| `list_user_sessions` | read | `user_lifecycle.getSessionsByUserId` |
| `terminate_user_sessions` | destructive | `user_lifecycle.terminateSessionsByUserId` |

### organizations (5)

| Tool | Kind | Operation |
|---|---|---|
| `list_organizations` | read | `organizations.organizationsGet` |
| `get_organization` | read | `organizations.getOrganizationsByOrganizationId` (input `id`) |
| `create_organization` | write | `organizations.organizationPost` |
| `delete_organization` | destructive | `organizations.deleteOrganizationsByOrganizationId` (input `id`) |
| `share_users` | write | `organizations.shareUsersWithSelected` with `organizations`, or `shareUsersWithAll` with `policy` (v2) |

### branding_notifications (5)

| Tool | Kind | Operation |
|---|---|---|
| `get_branding` | read | `branding_notifications.getBrandingPreference` (root organization only; type ORG and the tenant domain by default) |
| `update_branding` | write | `branding_notifications.updateBrandingPreference` |
| `configure_email_sender` | write | `branding_notifications.getEmailSender`, then `updateEmailSender`, or `createEmailSender` on 404 (v2; default name `EmailPublisher`) |
| `configure_sms_sender` | write | `branding_notifications.getSMSSender`, then `updateSMSSender`, or `postNotificationSendersSms` on 404 (v2; default name `SMSPublisher`; see research.md R2 for `createSMSSender`) |
| `update_email_template` | write | `branding_notifications.updateOrgEmailTemplate` (v2, by template type id and locale) |

### extensions (6)

| Tool | Kind | Operation |
|---|---|---|
| `list_webhooks` | read | `extensions.getWebhooks` |
| `create_webhook` | write | `extensions.createWebhook` (events from webhook metadata) |
| `delete_webhook` | destructive | `extensions.deleteWebhook` |
| `list_actions` | read | `extensions.getActionsByActionType` for `action_type`; without it, `getActionTypes` then the actions of every type in use, as a list of `{action_type, actions}` |
| `create_action` | write | `extensions.createAction` (action types of IS 7.3: `preIssueAccessToken`, `preUpdatePassword`, `preUpdateProfile`, `preRegistration`) |
| `delete_action` | destructive | `extensions.deleteAction` |

### workflows_approvals (4)

| Tool | Kind | Operation |
|---|---|---|
| `list_workflows` | read | `workflows_approvals.getWorkflows` |
| `create_workflow` | write | `workflows_approvals.addWorkflow` (engine `WorkflowEngine` unless given) |
| `associate_workflow` | write | `workflows_approvals.addWorkflowAssociation` |
| `list_workflow_requests` | read | `workflows_approvals.getWorkflowInstances` |

### claims (1)

| Tool | Kind | Operation |
|---|---|---|
| `create_local_claim` | write | `claims.addLocalClaim` |

Organization variants: every root id above has an `<area>.org.<operationId>` twin used when
the call passes `organization_id`, except the API resource writes (`addAPIResource`,
`patchApiResourcesByApiresourceid`, `putApiResourcesByApiresourceidScopes`,
`deleteApiResourcesByApiresourceid`), the OIDC scope tools and the session tools, which IS
exposes only for the root organization. The migrated read tools `list_api_resources`,
`search_api_resources_by_name` and `get_api_resource_by_identifier` use
`api_authorization.getAPIResources` with a `filter`; `list_claims` uses `claims.getLocalClaims`
(`exclude-hidden-claims=true` on the root variant, which is the only one that declares it).

The migrated application tools use `applications.getAllApplications`, `createApplication`,
`getApplication`, `patchApplication`, `getInboundOAuthConfiguration`,
`updateInboundOAuthConfiguration`, `addAuthorizedAPI` and `getAuthorizedAPIs` (all with
organization variants). After a create, the new application is read back by name. The
`get_application_by_*` scope list adds OIDC scopes through
`api_authorization.getApiServerV1OidcScopes` and `claims.getExternalClaims` (OIDC dialect), in the
root organization only. `update_login_flow` (root only) sends the local authenticators
(`identity_providers.getAuthenticators`), the identity providers (`getIDPs`) and the visible local
claims to `applications.generateLoginFlow`, polls `getLoginFlowStatus` every 2 s for up to
3 minutes, and patches the application with the `getLoginFlowResult` sequence.

Operation ids in this file are expected names. The implementation task for each area first
runs `search_operations` logic over the catalog to fix the exact ids and records them in the
tool's code; a test fails if any referenced id is missing from the catalog.
