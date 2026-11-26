# Contract: Catalog Tools (administration server, always registered)

## `search_operations` (read)

Input:

| Field | Type | Required | Notes |
|---|---|---|---|
| `query` | string | yes | Words matched against id, summary, path, area, tags |
| `area` | string | no | One area from data-model.md |
| `kind` | string | no | `read` \| `write` \| `destructive` |
| `organization` | boolean | no | `true` only org operations, `false` only root |
| `limit` | integer | no | 1–50, default 20 |

Output:

```json
{"total": 3, "operations": [
  {"id": "governance_security.listAllowedCORSOrigins", "summary": "...", "method": "GET",
   "path": "/api/server/v1/cors/origins", "kind": "read", "organization": false,
   "availability": "available"}
]}
```

## `describe_operation` (read)

Input: `operation_id` (string, required).

Output: `id`, `summary`, `description`, `method`, `path`, `kind`, `organization`, `scopes`,
`availability`, `deprecated`, `parameters` (`name`, `in`, `required`, `schema`), `body`
(`content_type`, `required`, `schema` depth-limited to 4), `call_with`
(`call_read_operation` \| `call_write_operation`), `example` (input object skeleton).

## `call_read_operation` (read)

| Field | Type | Required |
|---|---|---|
| `operation_id` | string | yes |
| `path_params` | object (string values) | when the path has params |
| `query` | object | no |
| `organization_id` | string | org operations only |

Refuses non-`read` operations, unavailable operations and org operations without
`organization_id` (and root operations with it).

## `call_write_operation` (write, `destructiveHint: true`)

Same inputs as `call_read_operation` plus `body` (object, array or string). Not registered when
`READ_ONLY=true`. Elicitation applies. Refuses `read` operations ("use call_read_operation").
Declared as destructive because it can run destructive operations.
