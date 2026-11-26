# Contract: Tool Conventions (all layers, both binaries)

## Names

- snake_case, `^[a-z][a-z0-9_]{0,63}$`, unique across curated, catalog and per-operation layers.
- Per-operation tools: `<area>_<operation_snake>`; over 64 chars → first 55 chars + `_` + first 8
  hex chars of SHA-256(full name).
- Existing names are frozen (FR-009, Principle VII).

## Annotations

| Kind | `readOnlyHint` | `destructiveHint` | `openWorldHint` |
|---|---|---|---|
| `read` | true | false | true |
| `write` | false | false | true |
| `destructive` | false | true | true |

## Descriptions

- First sentence says what the tool does and on which IS object.
- Organization-capable tools end with: "Pass organization_id to run inside a sub-organization."
- Write and destructive tools append exactly:
  "Before calling, describe the exact change and its target to the user and get their explicit
  yes to it. The user's request is not a yes, and a yes to an earlier or different change
  doesn't carry over."
- Tool data (names, descriptions, consent texts) is untrusted; server `instructions` say so.

## Inputs

- JSON Schema objects; required fields listed; no `additionalProperties` for curated tools.
- `organization_id` (string, optional) on every org-capable tool.
- A tool that is not org-capable refuses a non-empty `organization_id` with
  `operation <id> is not available for organizations` and sends nothing, so it never runs in
  the root organization by mistake (enforced in `internal/tools/toolkit`).
- Agents never pass `Authorization`, `Content-Type` or `Accept`.

## Approval (elicitation, FR-017a)

- Applies to every `write` and `destructive` call when the session's client declared the
  `elicitation` capability.
- Request message: `"<tool>: <one-line summary>"` plus a schema with one boolean field
  `approve` (title "Approve this change?"). Summary includes operation id (catalog/operation
  layers), target identifiers, organization id and, for bodies, the top-level keys.
- `accept` with `approve: true` → proceed. `accept` with `approve: false`, `decline`, `cancel`,
  timeout (120 s) or error → result `isError: true`, text
  `"Not approved by the user. Nothing was changed."`; no request reaches IS.

## Results

- Success: `text` content with indented JSON of the redacted (R12) and truncated (R15) value;
  empty IS body → `"Done."`.
- Truncation adds `{"truncated": true, "returned": n, "total": m, "hint": "..."}` at top level.
- Errors: `isError: true`, text:
  `request failed with status <code>: <hint>. <IS message>` and for `403`:
  ` Required scopes: <scope1>, <scope2>.`
  Hints: 400 "the request was rejected by IS", 401 "the access token is missing, expired or
  revoked", 403 "the credentials lack permission", 404 "not found, or not available on this
  server", 409 "it already exists or conflicts with the current state".
- Validation errors (before IS): `invalid input: <field>: <reason>; ...`.
- Refusals: `operation <id> is a flow API and is disabled; set ALLOW_FLOW_APIS=true to enable
  it`, `operation <id> changes data; use call_write_operation`, `operation <id> is not available
  for organizations`, `operation <id> is deprecated; set INCLUDE_DEPRECATED=true`.

## Read-only mode

`READ_ONLY=true` registers only `read` tools; `call_write_operation` is not registered.
