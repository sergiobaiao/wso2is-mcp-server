# Research: Full WSO2 Identity Server 7.3 Coverage

All Technical Context unknowns are resolved below. Each entry records the decision, why it was
chosen, and what was rejected. Facts marked **(verify in T011–T012)** are confirmed against a
running IS 7.3 container by the spike tasks in `tasks.md` (T011–T012) before dependent stories start.

## R1. Source of the API surface

- **Decision**: Vendor the official OpenAPI files from `wso2/docs-is` at commit
  `832c86be91f9753ecc67054f5737a5f0e389f395` (last change to
  `en/identity-server/7.3.0/docs/apis`, 2026-09-27):
  - `en/identity-server/7.3.0/docs/apis/restapis/*` (81 files, 643 operations) into
    `api-specs/wso2is/7.3.0/root/`
  - `en/identity-server/7.3.0/docs/apis/organization-apis/restapis/*` (35 files, 278
    operations) into `api-specs/wso2is/7.3.0/org/`
  - A `SOURCE` file records repository, commit and paths. `scripts/sync-specs.sh <commit>`
    refreshes the directory.
- **Rationale**: these are the files the IS documentation renders; they carry the scopes and
  paths per operation. The product source (`wso2/identity-api-server`) splits specs across many
  modules and versions and is harder to pin.
- **Alternatives**: hand-written tool list (rejected: 921 operations); Asgardeo Go SDK (rejected:
  covers 9 APIs, Asgardeo-oriented, codegen-based).

## R2. Spec defects found (measured on the pinned commit)

| Defect | Count | Normalization rule |
|---|---|---|
| Missing `operationId` | 135 operations in 23 files | Derive `<method><PathInPascalCase>` with path params as `By<Name>`, e.g. `GET /me/sessions/{session-id}` → `getMeSessionsBySessionId` |
| Invalid `operationId` (spaces, `*`) | 6 (`getUser by id`, `getGroup by id` ×2 each, `Initiate Authentication*`, `Handle Authentication`) | Sanitize to lowerCamelCase ASCII (`getUserById`) |
| Duplicate `operationId` in one file | `createSMSSender` in 4 notification-sender files | Second occurrence gets the derived name. In `notification-senders-v2` the first occurrence is the push sender, so `branding_notifications.createSMSSender` is `POST /v2/notification-senders/push` and the SMS create is `branding_notifications.postNotificationSendersSms` |
| Swagger 2.0 files | `root/scim2-schemas.yaml`, `root/user-consent.yaml` | Convert with `openapi2conv.ToV3` at load time |
| Missing or misleading `servers` | `scim2-schemas` (none), `authentication-api` (swaggerhub), `admin-identity-verification` and `user-identity-verification` (asgardeo.io), `org/end-user-credential-management` and `org/rule-metadata` (root URL in org spec) | Path prefix comes from a per-file table in code when `servers` is absent or wrong; otherwise from the part of `servers[0].url` after `/t/{tenant}` |
| Duplicated specs | `self-service.yaml` = `self-sign-up.yaml` (11 operations) | Deduplicate by `(family, method, full path)`; first file in sorted order wins |
| Scopes only in description text | 263 distinct `internal_*` scopes; 10 specs declare none | Regex `internal_[a-z0-9_]+` over operation description; empty set allowed |
| Request body schemas no valid body satisfies (found during US5) | `notification-senders-v2` email sender create and update (`oneOf` SMTP/HTTP variants without required fields, so every body matches both); `email-templates-v2` `EmailTemplateWithID` (`allOf` an object and an array) | Curated tools for these operations skip client-side body validation with `toolkit.CallUnvalidatedBody`; IS still validates the body. SCIM tools do the same for their malformed schemas. Catalog tools keep validating |

Operation identifiers exposed to agents are `<area>.<operationId>` with the family encoded in
the area for org specs (see data-model.md, Operation.ID) to stay unique across 116 files.

## R3. OpenAPI library

- **Decision**: `github.com/getkin/kin-openapi` v0.153.0.
- **Rationale**: one dependency owns three needs: OpenAPI 3 loading with `$ref` resolution,
  Swagger 2.0 conversion (`openapi2conv`) and request validation (`openapi3filter`), with
  JSON-schema output usable as MCP input schemas.
- **Alternatives**: `pb33f/libopenapi` (no request validator equivalent); hand-written parser
  (violates Principle V); `oapi-codegen` typed clients (prohibited by Principle V).

## R4. MCP library and elicitation

- **Decision**: upgrade `github.com/mark3labs/mcp-go` v0.34.0 → v1.6.0; Go directive to `1.25.5`
  (required by mcp-go v1.6.0).
- **Rationale**: v1.6.0 provides `MCPServer.RequestElicitation`, `server.WithElicitation()`,
  Streamable HTTP with `WithStateLess` and `WithHTTPContextFunc` (already used). Elicitation
  needs a session that can send server-to-client requests: stdio and stateful Streamable HTTP
  qualify, stateless HTTP does not.
- **Consequence**: HTTP mode runs **stateful** by default so elicitation works
  (`MCP_STATELESS=true` opts out and falls back to FR-016/FR-017 safeguards). The client
  capability check is per session: if the client did not declare `elicitation`, the fallback
  applies.
- **Migration risk**: API changes between v0.34 and v1.6 in tool construction and handler
  signatures; the upgrade is its own task with the existing tools compiling and their tests
  passing before any new work (verified by T004 against the compat goldens from T001 and T002).

## R5. HTTP authorization without a gateway

- **Decision**: validate IS-issued JWTs in the server and forward the same bearer to IS.
  - Library: `github.com/lestrrat-go/jwx/v3` v3.4.0 (`jwk.Cache` with background refresh,
    `jwt.Parse` with `WithKeySet`, `WithIssuer`, `WithAudience`, `WithAcceptableSkew`).
  - Issuer: `https://<host>/oauth2/token` for `carbon.super`, `https://<host>/t/<tenant>/oauth2/token`
    otherwise; override `OAUTH_ISSUER`.
  - JWKS: `<issuer base>/oauth2/jwks`; override `OAUTH_JWKS_URL`.
  - Audience: `MCP_RESOURCE` (required in HTTP mode). In IS 7.3 an MCP server registered under
    **Resources > MCP Servers** has an Identifier that IS puts in the token `aud`; only apps
    created from the **MCP Client Application** template can be authorized for it. The same app
    is also authorized for the IS management API scopes the tools need, so the forwarded token
    is accepted by IS **(verify in T011–T012: `aud` contains both client id and MCP resource; IS
    management APIs accept the token)**.
  - The app's access token type must be JWT (Protocol tab), otherwise validation fails with a
    clear startup/401 message.
  - Protected resource metadata (RFC 9728) at `/.well-known/oauth-protected-resource` and
    `/.well-known/oauth-protected-resource/mcp`: `resource` = `MCP_RESOURCE`,
    `authorization_servers` = [issuer base], `scopes_supported` = scopes of registered tools,
    `bearer_methods_supported` = `["header"]`.
  - `401` carries `WWW-Authenticate: Bearer resource_metadata="<url>"` and, for invalid tokens,
    `error="invalid_token"`.
- **Rationale**: removes the WSO2 AI Gateway (user decision) and keeps per-user attribution in
  IS audit logs; IS remains the scope enforcer.
- **Alternatives**: `golang-jwt/jwt/v5` + `MicahParks/keyfunc/v3` (two dependencies for the same
  job); token exchange RFC 8693 (deferred by clarification; IS 7.3 token exchange targets
  external-IdP tokens and needs separate configuration); no validation (rejected: Principle III).

## R6. Organization (B2B) access

- **Decision**: per call, when `organization_id` is set, obtain a token with
  `grant_type=organization_switch&token=<root token>&switching_organization=<id>&scope=<scopes>`
  at the **root** tenant token endpoint, authenticated with the configured client credentials,
  then call `/t/<tenant>/o/<path>`. Cache by `(root token hash, organization id)` until 30 s
  before expiry. Scopes requested: the operation's catalog scopes (org scopes are
  `internal_org_*`).
- **HTTP mode**: organization switch needs client authentication. `CLIENT_ID`/`CLIENT_SECRET`
  of the MCP Client app are configured on the server for this purpose only; without them,
  organization calls in HTTP mode return a configuration error.
- **Prerequisites (documented)**: the app is shared with the organizations and has the
  Organization Switch grant enabled **(verify in T011–T012 for an M2M app; fallback: a
  standard-based app with client_credentials and organization_switch grants)**.
- **Alternatives**: separate credentials per organization (rejected: does not scale); basic auth
  (rejected: passwords).

## R7. Token for stdio administration

- **Decision**: keep `client_credentials` with `scope=SYSTEM` (the current SDK behavior), which
  returns all scopes the app is authorized for. Cache until 30 s before expiry.
- **Alternatives**: requesting per-operation scopes per call (more round trips; no benefit since
  IS enforces scopes either way).

## R8. Catalog tool design

- **Decision**: four tools.
  - `search_operations(query, area?, kind?, organization?, limit?)`: case-insensitive token
    match over id, summary, path, area and tags; ranked; default limit 20.
  - `describe_operation(operation_id)`: parameters, body schema (depth-limited to 4 levels,
    `$ref` names kept), scopes, kind, flags and an example call shape.
  - `call_read_operation(operation_id, path_params?, query?, organization_id?)`.
  - `call_write_operation(operation_id, path_params?, query?, body?, organization_id?)`.
- **Rationale**: four tools give full coverage within any tool budget, and splitting read and
  write gives correct MCP annotations.
- **Alternatives**: one generic tool (rejected: annotations cannot vary per call); only
  per-operation tools (rejected: 921 tools exceed every client budget).

## R9. Kind classification

- **Decision**: GET/HEAD → `read`; DELETE → `destructive`; POST/PUT/PATCH → `write`; overrides
  in code:
  - `read`: `POST */.search` (SCIM search), `POST /scim2/Bulk` is **not** read.
  - `destructive`: any path segment `revoke`, `regenerate-secret`, `terminate`, `unshare`,
    `remove`, `delete`, `disable`, `purge`; `PUT` that replaces collections
    (`PUT /scim2/Groups/{id}`, `PUT /scim2/v2/Roles/{id}`); `POST .../sessions/terminate`.
- **Test**: golden table of overrides; coverage test asserts every operation has a kind.

## R10. Flow APIs

- **Decision**: classify as `flow` (refused unless `ALLOW_FLOW_APIS=true`): all operations of
  `app-native-authentication-api`, `authentication-api`, `authentication`, `self-service`,
  `self-sign-up`, `account-recovery` (root and org), `user-account-recovery-v1`,
  `user-account-recovery-v2`, `verification-code-management`, `verification-code-management-me`;
  plus `POST /me/change-password` (root and org user-password specs) and `DELETE /scim2/Me`.
- **Not flow**: `GET /pi-info` and `GET /pi-info/{userId}` in `self-service` and `self-sign-up`.
  They export personal data (own data, or a user's data for an administrator), carry no
  credentials or codes, and are `read`.
- Totals:

  | Count | Published | Unique (after removing the 11 `self-service`/`self-sign-up` duplicates) |
  |---|---|---|
  | Operations in flow specs (56 root, 5 org) | 61 | 50 |
  | minus `pi-info` operations | −4 | −2 |
  | plus single flow operations | +3 | +3 |
  | **Classified `flow`** | **60** | **51** |

## R11. Deprecated APIs

- **Decision**: hidden unless `INCLUDE_DEPRECATED=true`: `roles-v1`, `notification-senders`
  (v1, root and org), `organization-user-share` (v1, root and org), `user-account-recovery-v1`,
  and every operation whose `deprecated: true` flag is set.
- Consent v1 and email templates v1 are **not** deprecated in the 7.3 docs and stay visible.

## R12. Redaction

- **Decision**: one `redact` package, applied to every tool result and every error body:
  - Exact keys (case-insensitive): `clientSecret`, `client_secret`, `password`, `privateKey`,
    `secret`, `token`, `access_token`, `refresh_token`, `id_token`, `apiKey`, `key`,
    `qrCodeUrl`, `backupCodes`, `deviceToken`, `challenge`, `publicKey`, `publicKeyCose`,
    `attestationObject`, `clientDataJSON`, `signature`, `certificate` when under keystore
    private paths.
  - Substring keys: `secret`, `password`, `passwd`, `credential` (value only when string),
    `privatekey`, `apikey`.
  - IS property arrays `[{"key": "...", "value": "..."}]` and `[{"name": "...", "value": "..."}]`:
    redact `value` when `key`/`name` matches the rules (used by IdPs, notification senders,
    user stores, webhooks, actions).
  - Replacement value `"[REDACTED]"` so the agent knows the field exists.
- **Rationale**: extends the existing self-service `redact()` (keys only) to property arrays used
  across admin APIs.
- **Test**: fixtures per area (application, IdP, user store, notification sender, webhook,
  action, keystore, agent creation, TOTP) with zero secret leakage (SC-005).

## R13. Tool budget and naming

- **Decision**: default registration = curated (90) + catalog (4) = 94 < 100 (SC-002); a test
  counts registered tools with default config. Per-operation names: `<area>_<operation_snake>`;
  if > 64 chars, truncate to 55 and append `_` + 8 hex chars of SHA-256 of the full name.
  Uniqueness test over all 921 operations.

## R14. Base URL and tenancy

- **Decision**: `SERVER_URL` (e.g. `https://localhost:9443`) + `TENANT_DOMAIN` (default
  `carbon.super`). `BASE_URL` is parsed: `https://host[:port]/t/<tenant>` → both values;
  `https://host` → tenant `carbon.super`. All tenant-scoped requests use `/t/<tenant>`;
  operations whose spec has no `/t/` (`oauth-dcr`, `oauth2-scope-endpoint`,
  `user-account-recovery-*`, `authentication`) also work under `/t/<tenant>`, so one rule
  applies **(verify in T011–T012 for DCR and OAuth2 scopes)**.

## R15. Large responses

- **Decision**: `MAX_RESPONSE_BYTES` default 100000. Larger JSON results are cut at the last
  complete array element when the top level is a list or a SCIM `Resources` array, and the
  result carries `truncated: true`, `returned`, `total` (when known) and a hint naming the
  paging parameters of the operation (`limit`/`offset`, `count`/`startIndex`, `before`/`after`).

## R16. Testing strategy

- **Unit**: `go test -race ./...`; no network. `internal/wso2is/istest` provides an `httptest`
  fake IS with programmable routes, a token endpoint (client_credentials, organization_switch)
  and a JWKS endpoint backed by a generated RSA key.
- **Golden**: `internal/wso2is/catalog/testdata/stats.golden.json` (counts per family, area,
  kind, flow, deprecated, excluded) regenerated with `go test ./internal/wso2is/catalog -update`.
- **Coverage**: one test iterates every operation, builds a minimal valid call from the schema
  and asserts it reaches the fake IS with the right method and path (or is refused for flow
  and deprecated operations with the right reason).
- **Integration**: build tag `integration`; `docker run -p 9443:9443 wso2/wso2is:7.3.0`; a
  bootstrap helper uses the default admin to create the M2M app and authorize scopes; tests run
  the 21 SCIM tools, an org switch and a catalog call per area.
- **Lint**: `golangci-lint` with the default linters plus `gosec`, `errorlint`, `revive`.

## R17. Asgardeo Go SDK

- **Decision**: keep `github.com/asgardeo/go` until the 19 admin tools and 28 self-service tools
  run on the new client; then remove it in the same feature (T106). `update_login_flow`
  calls `POST /api/server/v1/applications/loginflow/generate`, `GET .../loginflow/status/{id}`
  and `GET .../loginflow/result/{id}`. These paths are **not** in the 7.3 OpenAPI files, so they
  are registered as three hand-declared catalog entries in `catalog/extra.go` (area
  `applications`, flagged `ai_feature`). They require the IS AI subscription and return a
  clear error otherwise **(verify in T011–T012)**.
