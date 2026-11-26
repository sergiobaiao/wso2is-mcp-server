# Contract: Configuration (environment variables)

Both binaries read only environment variables. Unknown values for enumerated variables are a
startup error naming the variable and the allowed values. Secrets are never echoed in errors or
logs.

## Shared

| Variable | Default | Binaries | Description |
|---|---|---|---|
| `SERVER_URL` | — | both | `https://<host>[:port]`. Required unless `BASE_URL` is set |
| `TENANT_DOMAIN` | `carbon.super` | both | Root organization (tenant) domain |
| `BASE_URL` | — | both | Legacy form `https://<host>[:port]/t/<tenant>` or `https://<host>`; parsed into `SERVER_URL` and `TENANT_DOMAIN`. If both forms are set and disagree, startup fails |
| `ASGARDEO_BASE_URL` | — | admin | Legacy fallback for `BASE_URL` |
| `CERT_PATH` | — | both | PEM CA bundle added to the system pool. No option disables verification |
| `TRANSPORT` | admin `stdio`, self-service `http` | both | `stdio` \| `http` |
| `PORT` | `8000` | both | HTTP listen port |
| `MCP_STATELESS` | `false` | both | `true` runs Streamable HTTP stateless (no elicitation) |
| `IS_VERSION` | `7.3.0` | both | Catalog version directory |
| `MAX_RESPONSE_BYTES` | `100000` | both | Result size cap (R15) |
| `LOG_LEVEL` | `info` | both | `debug` \| `info` \| `warn` \| `error`; logs go to stderr |

## Administration server (`wso2is-mcp`)

| Variable | Default | Description |
|---|---|---|
| `CLIENT_ID`, `CLIENT_SECRET` | — | stdio: required (client credentials). HTTP: optional, needed only for organization switch |
| `ASGARDEO_CLIENT_ID`, `ASGARDEO_CLIENT_SECRET` | — | Legacy fallbacks |
| `PRODUCT_MODE` | `wso2is` | Accepted for compatibility; only affects the product name in tool descriptions |
| `READ_ONLY` | `false` | Register only read tools |
| `ALLOW_FLOW_APIS` | `false` | Allow `flow` operations |
| `INCLUDE_DEPRECATED` | `false` | Show deprecated operations |
| `TOOLSETS` | empty | Comma list of areas (data-model.md area table) or `all`, for per-operation tools |
| `SCIM_PROFILE_ATTRIBUTES` | `urn:ietf:params:scim:schemas:extension:enterprise:2.0:User:department,urn:ietf:params:scim:schemas:extension:enterprise:2.0:User:employeeNumber,urn:ietf:params:scim:schemas:extension:enterprise:2.0:User:costCenter,addresses.work` | Attributes accepted by `update_user_profile` (full SCIM paths) |

## Self-service server (`self-service-mcp`)

| Variable | Default | Description |
|---|---|---|
| `ACCESS_TOKEN` | — | stdio: required |
| `CLIENT_ID` | — | Client id of the app the user signed in to (kept by `revoke_app_access`) |
| `PORTAL_NAME` | `self-service portal` | Name used when sending users to the portal |
| `ALLOW_FLOW_APIS` | `false` | Allows `change_my_password` and `delete_my_account` style flow tools if added later; no flow tool is registered by default |

## HTTP mode authorization (both binaries)

| Variable | Default | Description |
|---|---|---|
| `MCP_RESOURCE` | — | Required in HTTP mode for the admin server; optional for self-service (when unset, self-service keeps today's behavior: bearer required, not validated, logged as a startup warning). Must equal the Identifier of the MCP server registered in IS (token `aud`) |
| `OAUTH_ISSUER` | derived | `<SERVER_URL>/oauth2/token` for `carbon.super`, else `<SERVER_URL>/t/<tenant>/oauth2/token` |
| `OAUTH_JWKS_URL` | derived | Issuer base + `/oauth2/jwks` |
| `PUBLIC_URL` | `http://localhost:<PORT>` | Public base URL used in protected resource metadata |
| `JWT_CLOCK_SKEW` | `60s` | Accepted skew for `exp`/`nbf` |
