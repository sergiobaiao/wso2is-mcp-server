# Contract: HTTP Transport (both binaries, `TRANSPORT=http`)

## Endpoints

| Method | Path | Auth | Response |
|---|---|---|---|
| `POST`, `GET`, `DELETE` | `/mcp` | Bearer JWT | MCP Streamable HTTP |
| `GET` | `/.well-known/oauth-protected-resource` | none | RFC 9728 metadata |
| `GET` | `/.well-known/oauth-protected-resource/mcp` | none | Same document |
| `GET` | `/healthz` | none | `200 ok` (no IS call) |

## Protected resource metadata

```json
{
  "resource": "<MCP_RESOURCE>",
  "authorization_servers": ["<issuer base, e.g. https://is.example.com or https://is.example.com/t/acme>"],
  "bearer_methods_supported": ["header"],
  "scopes_supported": ["<union of scopes of registered tools>"],
  "resource_name": "WSO2 IS MCP Server"
}
```

## Authentication

1. No `Authorization: Bearer` header → `401`,
   `WWW-Authenticate: Bearer resource_metadata="<PUBLIC_URL>/.well-known/oauth-protected-resource"`.
2. Token not a JWT, bad signature (JWKS), `iss` ≠ `OAUTH_ISSUER`, `aud` not containing
   `MCP_RESOURCE`, expired, or not yet valid (skew `JWT_CLOCK_SKEW`) → `401`,
   `WWW-Authenticate: Bearer error="invalid_token", error_description="<reason>",
   resource_metadata="..."`. Reasons never include the token.
3. Valid → request context gets `CredentialContext{Mode: bearer, BearerToken, Subject}`; IS calls
   use `Authorization: Bearer <same token>`.
4. JWKS is cached and refreshed in the background (minimum interval 5 min); an unknown `kid`
   triggers one refresh, rate-limited to once per minute.

Self-service without `MCP_RESOURCE`: step 2 is skipped (legacy behavior) and a startup warning
is logged.

## Server behavior

- `ReadHeaderTimeout` 10 s; graceful shutdown on SIGINT/SIGTERM with 10 s timeout.
- Stateful sessions by default (elicitation); `MCP_STATELESS=true` switches to stateless.
- Access log line per request: method, path, status, duration, `sub` (never token or body).
