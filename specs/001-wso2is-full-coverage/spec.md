# Feature Specification: Full WSO2 Identity Server 7.3 Coverage

**Feature Branch**: `001-wso2is-full-coverage`

**Created**: 2026-11-25

**Status**: Draft

**Input**: User description: "Full WSO2 Identity Server 7.3 coverage for the MCP servers in this repository. Administrators and developers must be able to manage everything the IS 7.3 REST APIs expose (116 official OpenAPI specs: 81 root-organization specs with 643 operations and 35 organization (B2B, /o/) specs with 278 operations, 921 operations total) through an MCP client in natural language: applications, API resources and scopes, SCIM 2.0 users, groups, roles v2, bulk and AI agents, identity providers and authenticators, claims, user stores, governance connectors and validation rules, branding, email and notification templates and senders, actions and webhooks, organizations and user sharing, workflows and approvals, consents, tenants, keystores, CORS, DCR, identity verification and verifiable credential templates. Frequent tasks get curated task-shaped tools, including the 21 SCIM tools of the wso2is-scim2-mcp-ai-gateway project with the same names (list_users, search_users, get_user, create_user, update_user_attribute, update_user_profile, delete_user, list_groups, create_group, add_user_to_group, remove_user_from_group, delete_group, list_roles, create_role, assign_role_to_user, remove_role_from_user, delete_role, list_agents, create_agent, delete_agent, bulk_create_users) but without the WSO2 AI Gateway. Every other operation stays reachable through generic catalog tools (search, describe, read call, write call) and, opt-in, through one tool per operation grouped by toolset. Organization APIs work for any sub-organization by organization id. The administration server runs over stdio with client credentials or over HTTP where it validates IS-issued JWTs and forwards them to IS. The self-service server gains the missing end-user APIs (approval tasks, my organizations, authorized apps v2, personal data export, user consents). Flow and credential APIs (app-native login, /authn, public self sign-up, account recovery, own password change, self account deletion, verification codes) are off unless ALLOW_FLOW_APIS=true. Secrets never reach the model; write and destructive actions need explicit user confirmation; READ_ONLY mode exists. Existing tools and environment variables keep working."

## Clarifications

### Session 2026-11-25

- Q: Which products are targets, and does the fork keep the upstream module path? → A: IS only;
  a new `wso2is-mcp` administration binary replaces `asgardeo-mcp`; module path
  `github.com/asgardeo/mcp` is kept for upstream cherry-picks; Asgardeo is not tested.
- Q: Without the WSO2 AI Gateway, how does HTTP mode authorize requests? → A: The server
  validates IS-issued JWTs (signature via JWKS, issuer, audience, validity), publishes protected
  resource metadata, and forwards the validated token to IS; token exchange is deferred.
- Q: Should flow and credential APIs become tools? → A: Yes, but disabled by default and enabled
  only with `ALLOW_FLOW_APIS=true`.
- Q: What language do the artifacts use? → A: English.
- Q: How do the servers make sure the user approves a write or destructive call? → A: When the
  connected client supports MCP elicitation, the server asks the user to approve the exact
  change before calling IS; otherwise it falls back to read/destructive markings plus tool
  descriptions that demand the user's explicit yes.

## User Scenarios & Testing *(mandatory)*

Actors:

- **Administrator**: an IS administrator or developer who manages an IS deployment through an
  AI agent in an MCP client (for example Claude Desktop, Claude Code, Gemini CLI, VS Code).
- **Operator**: the person who installs, configures and hosts the MCP servers.
- **End user**: a person with an account in IS who manages their own account through an agent.

### User Story 1 - Reach any IS 7.3 operation through a small set of tools (Priority: P1)

The administrator asks the agent to do something that no dedicated tool covers (for example
"list the CORS origins", "show the keystore certificates", "create a verifiable credential
template"). The agent searches the operation catalog by keyword, reads the description of the
matching operation (parameters, request body shape, required scopes, whether it changes data),
and calls it. Reads run directly; changes run only after the administrator confirms.

**Why this priority**: it delivers coverage of every IS 7.3 operation on day one with only four
tools, and every later story builds on the same catalog.

**Independent Test**: start the administration server against a test IS, ask the agent to
find and call three operations from three different API areas (one read, one create, one
delete), and verify each result in the IS Console.

**Acceptance Scenarios**:

1. **Given** the server is running, **When** the agent searches for "cors", **Then** it receives
   the matching operations with their identifiers, a one-line summary, the area they belong to
   and whether each one reads or changes data.
2. **Given** an operation identifier, **When** the agent asks for its description, **Then** it
   receives the parameters, the request body shape, the required scopes and its kind (read,
   write or destructive).
3. **Given** a read operation, **When** the agent calls it through the read tool, **Then** the
   IS response is returned with secrets removed.
4. **Given** a write or destructive operation, **When** the agent tries to call it through the
   read tool, **Then** the call is refused and the agent is told to use the write tool.
5. **Given** a request whose parameters or body do not match the operation's definition,
   **When** the agent calls it, **Then** the call is rejected before reaching IS with a message
   that names the invalid fields.
6. **Given** the credentials lack a scope, **When** the agent calls an operation, **Then** the
   error states the HTTP status, the IS message and the scopes the operation requires.

---

### User Story 2 - Manage users, groups, roles and agents with task-shaped tools (Priority: P1)

The administrator manages identities in natural language: "find users whose email ends with
example.com", "create the user emma.clarke", "add Emma to the Finance group", "give her the
auditor role", "create 20 users from this list", "register an AI agent for the billing
service". The tools have the same names and behavior as the 21 SCIM tools of the
wso2is-scim2-mcp-ai-gateway project, so existing prompts and habits keep working, and they work
without any gateway.

**Why this priority**: identity lifecycle is the most frequent administrative task and the
explicit integration goal of the fork.

**Independent Test**: run the 21 tools in sequence against a test IS (create, search, update,
group membership, role assignment, agent creation, bulk create, then deletes) and verify each
step through the IS Console or a read tool.

**Acceptance Scenarios**:

1. **Given** users exist, **When** the agent calls `search_users` with a filter such as
   `emails co example.com`, **Then** only matching users are returned with id, user name, name,
   emails and profile attributes.
2. **Given** a user and a group, **When** the agent calls `add_user_to_group` after the
   administrator confirms, **Then** the user becomes a member and the result says so.
3. **Given** a role in the root organization, **When** the agent calls `assign_role_to_user`,
   **Then** the role lists the user among its assigned users.
4. **Given** a list of users, **When** the agent calls `bulk_create_users`, **Then** each user is
   created or reported individually with the reason it failed.
5. **Given** the server runs with `READ_ONLY=true`, **When** the agent lists its tools, **Then**
   the only SCIM tools available are the read tools (`list_users`, `search_users`, `get_user`,
   `list_groups`, `get_group`, `list_roles`, `get_role`, `list_agents`, `get_agent`).

---

### User Story 3 - Administer sub-organizations (Priority: P2)

The administrator of a B2B deployment works inside a sub-organization: "list the users of the
Acme organization", "share the Portal application with Acme", "create a role in Acme". Any
operation that exists for organizations accepts an organization identifier, and the server
obtains the organization access on the administrator's behalf.

**Why this priority**: 278 of the 921 operations are organization-scoped; B2B is a core IS use
case but depends on the catalog (Story 1) being in place.

**Independent Test**: create a sub-organization in a test IS, then list users, create a group
and read an application inside it by passing its identifier, and verify in the Console.

**Acceptance Scenarios**:

1. **Given** a sub-organization identifier, **When** the agent calls an organization operation
   with it, **Then** the operation runs inside that organization and not the root.
2. **Given** an identifier the credentials cannot access, **When** the agent uses it, **Then** the
   error says that access to that organization was refused.
3. **Given** an operation that exists only at the root, **When** the agent passes an
   organization identifier, **Then** the call is rejected with a message saying the operation is
   not available for organizations.

---

### User Story 4 - Host the administration server for a team (Priority: P2)

The operator runs one administration server over HTTP for several administrators. Each
administrator's MCP client signs in to IS and sends its access token. The server accepts only
valid tokens issued by the configured IS for this server, tells clients where to sign in when
they have no token, and calls IS with each administrator's own token, so every change is
attributed to the person who made it and limited by their permissions.

**Why this priority**: removes the need for the WSO2 AI Gateway while keeping per-user
attribution; depends on Stories 1 and 2 for useful tools.

**Independent Test**: start the server in HTTP mode, connect an MCP client without a token, with
a token for another application, with an expired token and with a valid token, and check that
only the valid token reaches the tools and that IS audit logs show the signed-in user.

**Acceptance Scenarios**:

1. **Given** a request without a token, **When** it reaches the server, **Then** it is rejected
   as unauthenticated and the response points to where the client can discover how to sign in.
2. **Given** a token with a wrong issuer, wrong audience, bad signature or past expiry, **When** it
   reaches the server, **Then** it is rejected as unauthenticated and no call is made to IS.
3. **Given** a valid token, **When** a tool runs, **Then** IS receives that same administrator's
   token and applies that administrator's permissions.

---

### User Story 5 - Curated tools for common administration areas (Priority: P2)

The administrator performs common configuration in one step: register applications of every
type, update their protocols, claims and login flows, define API resources and scopes, add
social and enterprise identity providers, set password and lockout policies, enable self
registration, connect user stores, review and end user sessions, configure email and SMS
senders, branding and templates, webhooks and actions, approval workflows and organizations.
The 19 existing administration tools keep their names and behavior on IS.

**Why this priority**: generic tools cover everything but need several agent turns; curated
tools make the frequent tasks reliable.

**Independent Test**: for each area, run its curated tools against a test IS and verify the
resulting configuration in the Console.

**Acceptance Scenarios**:

1. **Given** an existing MCP client configuration from the upstream project, **When** it is
   pointed at the new server, **Then** the 19 existing tools keep their names, inputs and outputs.
2. **Given** an application is created, **When** the result is returned, **Then** it includes the
   client identifier and endpoints but never the client secret.
3. **Given** a curated tool fails with a missing scope, **When** the error is shown, **Then** it
   lists the scopes the tool needs.

---

### User Story 6 - Expose one tool per operation for chosen areas (Priority: P3)

An operator who wants explicit tools (for fine-grained client-side approval or for agents that
work better with specific tools) enables per-operation tools for chosen areas, for example
roles and user stores, without enabling all 921.

**Why this priority**: a convenience on top of full coverage; most deployments use Stories 1, 2
and 5.

**Independent Test**: enable two areas, list the tools, and check that exactly the operations of
those areas appear, with correct read or change markings and unique names.

**Acceptance Scenarios**:

1. **Given** two areas are enabled, **When** the client lists tools, **Then** one tool per
   operation of those areas appears in addition to the default tools.
2. **Given** `READ_ONLY=true` and an enabled area, **When** the client lists tools, **Then** only
   that area's read operations appear.

---

### User Story 7 - End users manage more of their own account (Priority: P3)

The end user asks their agent to "show the approval requests waiting for me and approve the
first one", "which organizations do I belong to", "download my personal data", "which apps can
access my account", "show and revoke my consents". The self-service server offers these on top
of its 28 existing tools.

**Why this priority**: valuable but independent from the administration work.

**Independent Test**: sign in as a test user, run each new self-service tool, and verify the
result in the My Account portal.

**Acceptance Scenarios**:

1. **Given** a pending approval task assigned to the user, **When** the user confirms approval,
   **Then** the task state changes to approved.
2. **Given** the user belongs to two organizations, **When** they ask for their organizations,
   **Then** both are listed.

---

### User Story 8 - Opt in to flow and credential APIs (Priority: P3)

An operator building a test or migration harness explicitly enables the flow and credential
APIs (interactive login, public self sign-up, account recovery, own password change, self
account deletion, verification codes). Without that explicit setting these operations are
listed as disabled and cannot be called.

**Why this priority**: required for literal 100% coverage, but risky and rarely needed.

**Independent Test**: call a flow operation with the setting off (refused, with the reason) and
on (executed).

**Acceptance Scenarios**:

1. **Given** the setting is off, **When** the agent describes a flow operation, **Then** it is
   marked disabled with the reason and how to enable it.
2. **Given** the setting is off, **When** the agent calls it, **Then** the call is refused and
   nothing is sent to IS.
3. **Given** the setting is on, **When** the agent calls it after confirmation, **Then** it runs.

### Edge Cases

- An IS response is very large (for example, all applications with full details): the result
  is cut at a configured size and the agent is told how to page or filter.
- A response contains a secret in an unexpected field (for example a custom notification sender
  property named `apiKey`): it is removed because the field name matches a secret pattern.
- Two vendored definitions describe the same operation: it appears once in the catalog.
- A vendored definition lacks an operation identifier or has an invalid one: a stable
  identifier is derived so the operation remains reachable.
- A deprecated API has a successor (roles v1, notification senders v1, user sharing v1,
  account recovery v1): it is hidden unless deprecated APIs are enabled.
- The super-tenant-only tenant management APIs are called with credentials of another tenant:
  IS refuses and the error explains that the operation needs the super tenant.
- The IS certificate is self-signed: the operator provides a CA file; verification is never
  turned off.
- An organization access token expires during a long session: a new one is obtained without
  user action.
- IS returns `404` for an operation that exists in 7.3 docs but is disabled on the deployment:
  the error says the feature may be unavailable on this server.
- The agent calls a write tool without the administrator's confirmation: if the client supports
  elicitation the server itself asks the administrator and stops on decline; otherwise the tool
  description forbids it and clients that honor the change markings ask for approval.
- The user declines or ignores an approval request: nothing is sent to IS and the agent is told
  the change was not approved.

## Requirements *(mandatory)*

### Functional Requirements

Catalog and coverage

- **FR-001**: The system MUST include every operation of the 116 official IS 7.3 API definitions
  (643 root and 278 organization operations as published) in a searchable catalog; operations
  published identically in two definitions appear once.
- **FR-002**: Each catalog entry MUST state its identifier, area, summary, whether it is
  organization-scoped, its kind (read, write or destructive), its required scopes, its parameters
  and request body shape, and whether it is deprecated, a flow API, or disabled.
- **FR-003**: The system MUST provide exactly four always-available catalog tools: search
  operations, describe an operation, call a read operation, and call a write or destructive
  operation.
- **FR-004**: The read call tool MUST refuse operations whose kind is not read.
- **FR-005**: Calls MUST be validated against the operation definition before being sent to IS;
  invalid calls MUST be rejected with the names of the invalid fields.
- **FR-006**: An automated check MUST fail if any catalog operation is neither callable nor
  explicitly classified as disabled with a reason.

Curated tools

- **FR-007**: The system MUST offer the 21 SCIM tools with the names listed in the input, with
  equivalent inputs and outputs to the wso2is-scim2-mcp-ai-gateway project, and without
  requiring any gateway.
- **FR-008**: The system MUST offer curated tools for applications, API authorization, identity
  providers, governance policies, user stores, user sessions, organizations, notification
  senders, branding and templates, webhooks and actions, and approval workflows.
- **FR-009**: The 19 existing administration tools MUST keep their names, inputs and outputs.
- **FR-010**: Curated SCIM tools MUST let the operator configure which extra profile attributes
  `update_user_profile` accepts; by default it accepts the enterprise extension attributes.

Organizations

- **FR-011**: Every organization-scoped operation and every curated tool that supports
  organizations MUST accept an organization identifier and run inside that organization.
- **FR-012**: Organization access MUST be obtained automatically from the caller's credentials
  and reused until it expires.

Per-operation tools

- **FR-013**: Operators MUST be able to enable one tool per operation for chosen areas; with no
  areas enabled, no per-operation tools are registered.
- **FR-014**: Tool names MUST be unique, stable across restarts and at most 64 characters.

Security

- **FR-015**: Secrets (client secrets, passwords, private keys, keystore material, one-time and
  backup codes, webhook and identity provider secrets, tokens) MUST never appear in tool
  results.
- **FR-016**: Every tool MUST declare whether it is read-only and whether it is destructive.
- **FR-017**: Every write or destructive tool MUST instruct the agent to describe the exact change
  and obtain the user's explicit confirmation before calling it.
- **FR-017a**: When the connected client supports MCP elicitation, every write or destructive
  call MUST first ask the user to approve a summary of the exact change (tool, target, operation
  and organization); a declined or cancelled request MUST end the call without contacting IS.
  When the client does not support elicitation, FR-016 and FR-017 are the safeguards.
- **FR-018**: When read-only mode is enabled, no tool that changes data MUST be registered.
- **FR-019**: Flow and credential operations MUST be refused unless the operator enables them
  explicitly with `ALLOW_FLOW_APIS=true`.
- **FR-020**: Certificate verification of IS connections MUST never be disabled; a custom
  certificate authority file MUST be supported.
- **FR-021**: Tokens and secrets MUST never be written to logs.

Hosting

- **FR-022**: The administration server MUST run over stdio with application credentials and
  over HTTP with per-request user tokens.
- **FR-023**: In HTTP mode the server MUST accept only tokens issued by the configured IS whose
  audience matches the configured value, with a valid signature and within their validity
  period; all others MUST be rejected before any tool runs.
- **FR-024**: In HTTP mode the server MUST publish metadata that tells MCP clients which
  authorization server to use, and unauthenticated responses MUST point to it.
- **FR-025**: In HTTP mode the server MUST call IS with the caller's own validated token.
- **FR-026**: The self-service server MUST apply the same token validation when the operator
  configures an expected issuer and audience.

Self-service

- **FR-027**: The self-service server MUST add tools to list, view, approve and reject the user's
  approval tasks; list the user's organizations; list and revoke authorized apps (v2); export the
  user's personal data; and list, view and revoke the user's consents.

Errors and responses

- **FR-028**: Errors MUST include the HTTP status, the IS error message, and for authorization
  failures the scopes the operation requires.
- **FR-029**: Results larger than a configured size MUST be cut, with guidance on paging or
  filtering.

Compatibility and configuration

- **FR-030**: All existing configuration variables MUST keep working, including the `BASE_URL`
  form `https://<host>/t/<tenant>` and the `ASGARDEO_*` fallbacks.
- **FR-031**: The operator MUST be able to choose the IS version; 7.3.0 is the default and the
  only version delivered by this feature.
- **FR-032**: Deprecated APIs that have a successor MUST be hidden unless the operator enables
  them.
- **FR-033**: User-facing documentation of tools and their required scopes MUST be generated
  from the same source as the tools, and a check MUST fail when it drifts.

### Key Entities

- **API definition**: one official IS 7.3 OpenAPI document; belongs to the root or the
  organization family; contains operations.
- **Operation**: one HTTP method and path; has identifier, area, kind, required scopes,
  parameters, body shape and flags (organization-scoped, deprecated, flow, disabled).
- **Area (toolset)**: a named group of operations, such as applications, users, roles,
  identity providers or organizations; used for search and per-operation tool selection.
- **Tool**: what the agent sees; curated, catalog or per-operation; has name, description,
  inputs, read-only and destructive markings and required scopes.
- **Credential context**: how a call authenticates to IS: application credentials, a fixed user
  token, or a validated per-request token, plus an optional organization.
- **Organization**: an IS sub-organization identified by an id; target of organization-scoped
  operations.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: 100% of the IS 7.3 operations (921 as published, before removing exact
  duplicates) are either callable through the catalog tools or listed as disabled with a reason,
  verified automatically on every change.
- **SC-002**: With default settings the administration server registers no more than 100 tools.
- **SC-003**: The 21 SCIM tools and the 19 existing administration tools pass the same
  acceptance scenarios as before the change.
- **SC-004**: An administrator can complete each of these tasks in a single request to the
  agent: create a user and add them to a group; register a single-page application; create a
  role with API scopes and assign it; list users of a sub-organization.
- **SC-005**: In a test with responses that contain secrets from every area listed in FR-015,
  zero secret values appear in tool results.
- **SC-006**: In HTTP mode, 100% of requests with missing, expired, wrongly-issued or
  wrong-audience tokens are rejected without any call to IS.
- **SC-007**: A user who knows the area but not the operation can find the right operation with
  at most two catalog searches in at least 90% of trials over a fixed set of 30 task prompts.
- **SC-008**: Upgrading to a new IS version requires adding its API definitions and
  normalization rules only, with no change to tool code, verified by a test with a second
  synthetic version.
- **SC-009**: With a client that supports elicitation, 100% of write and destructive calls ask
  the user first, and 0 requests reach IS after a decline.

## Assumptions

- The official IS 7.3 API definitions published in the WSO2 documentation repository are the
  authoritative description of the API surface; operations not described there are out of
  scope.
- Administrators use credentials (an application or a user token) whose permissions IS already
  enforces; the server does not add its own permission model on top of IS.
- For stdio administration, the application is authorized for the scopes it needs in the IS
  Console; the server requests all scopes the application is authorized for.
- Organization access for stdio administration uses an application that is shared with the
  sub-organizations and allowed to switch organizations; this is validated before Story 3 work.
- The WSO2 AI Gateway is not used; all authentication and authorization handling described here
  happens in the servers and in IS.
- Asgardeo is not a supported target; existing behavior against Asgardeo may keep working but is
  not tested. The administration binary is renamed to `wso2is-mcp`.
- Token exchange to a separate IS-API token is out of scope for this feature; HTTP mode forwards
  the validated caller token.
- Only IS 7.3.0 is delivered; other versions are future work enabled by FR-031.
