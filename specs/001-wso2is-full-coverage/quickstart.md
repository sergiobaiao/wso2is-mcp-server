# Quickstart: Validate Full WSO2 IS 7.3 Coverage

Runnable checks that prove the feature end to end. Contracts are in `contracts/`, models in
`data-model.md`.

## Prerequisites

- Go per `go.mod` (1.25.5+), Docker, `golangci-lint`.
- A local IS 7.3.0:

```bash
docker run -d --name wso2is -p 9443:9443 wso2/wso2is:7.3.0
```

- IS Console (`https://localhost:9443/console`, default admin for local tests only):
  1. Create an **M2M application** `mcp-admin`. Authorize the Management APIs the tests use
     (SCIM2 Users, Groups, Roles v2, Bulk, Agents; Application, API Resource, IdP, Claim,
     Identity Governance, User Store, Organization Management) with all scopes. Note client id and
     secret.
  2. For HTTP mode: under **Resources > MCP Servers** register `http://localhost:8000/mcp`
     (Identifier) and create an **MCP Client Application** `mcp-client` authorized for that MCP
     server and for the same management APIs; set its access token type to **JWT**.
  3. For organization checks: create sub-organization `acme`, share `mcp-admin` with it and
     enable the Organization Switch grant.
- Export the IS CA: `openssl s_client -connect localhost:9443 -showcerts </dev/null | openssl x509 > is-ca.pem`.

## 1. Unit, golden and coverage tests (no IS needed)

```bash
go test -race ./...
golangci-lint run
```

Expected: all pass; `internal/wso2is/catalog` reports the golden stats (116 files, 921
published operations, duplicates removed, per-area counts as in data-model.md); the coverage
test reports 0 unreachable operations.

## 2. Catalog tools over stdio (Story 1)

```bash
go build -o wso2is-mcp ./cmd/wso2is-mcp
SERVER_URL=https://localhost:9443 CLIENT_ID=<id> CLIENT_SECRET=<secret> CERT_PATH=is-ca.pem ./wso2is-mcp
```

With an MCP client (or `npx @modelcontextprotocol/inspector ./wso2is-mcp`):

| Step | Expected |
|---|---|
| `search_operations` `{"query":"cors"}` | CORS operations, kind `read` |
| `call_read_operation` on the CORS list operation | origins list |
| `call_read_operation` on a `write` operation | refused: use `call_write_operation` |
| `call_write_operation` with an invalid body | `invalid input: ...` and no IS request |

## 3. SCIM tools (Story 2)

Run the sequence: `create_user` → `search_users` (`userName eq <name>`) → `create_group` →
`add_user_to_group` → `create_role` → `assign_role_to_user` → `get_user` (shows group and role)
→ `bulk_create_users` (3 users) → deletes. Expected: each step succeeds and the Console shows
the result. Then restart with `READ_ONLY=true` and list tools: only read tools are present.

## 4. Organizations (Story 3)

`list_users` with `organization_id=<acme id>` returns acme users only; a root-only operation with
`organization_id` is refused with "not available for organizations".

## 5. HTTP mode (Story 4)

```bash
TRANSPORT=http PORT=8000 SERVER_URL=https://localhost:9443 CERT_PATH=is-ca.pem \
MCP_RESOURCE=http://localhost:8000/mcp ./wso2is-mcp
curl -i http://localhost:8000/mcp -X POST                      # 401 + resource_metadata
curl -s http://localhost:8000/.well-known/oauth-protected-resource
curl -i http://localhost:8000/mcp -X POST -H "Authorization: Bearer <token of mcp-admin>"  # 401 invalid_token (aud)
```

Then connect an MCP client that supports OAuth to `http://localhost:8000/mcp`; it discovers IS,
signs in through `mcp-client`, and `list_users` works. IS audit logs show the signed-in user.

## 6. Elicitation

With a client that supports elicitation, call `delete_user`: the client shows an approval
prompt; decline → result "Not approved by the user. Nothing was changed." and the user still
exists.

## 7. Integration suite

```bash
IS_URL=https://localhost:9443 IS_ADMIN_USER=admin IS_ADMIN_PASSWORD=<local admin password> \
go test -tags integration -race ./test/integration/...
```

Expected: all pass against the container.
