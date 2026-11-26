# Implementation Plan: Full WSO2 Identity Server 7.3 Coverage

**Branch**: `001-wso2is-full-coverage` | **Date**: 2026-11-25 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/001-wso2is-full-coverage/spec.md`

**Note**: This template is filled in by the `/speckit-plan` command; its definition describes the execution workflow.

## Summary

Make both MCP servers cover the whole WSO2 IS 7.3 REST surface (116 OpenAPI files, 921
operations) without the WSO2 AI Gateway. The official specs are vendored and embedded, then
normalized at startup into an operation catalog. One IS client handles client credentials, a
static user token or a validated bearer token, organization switch, CA trust, validation, error
mapping, redaction and truncation. Three tool layers sit on the catalog:

- **Curated** (90): the 19 existing admin tools, the 21 SCIM tools ported from the Arazzo
  workflows, and 51 new task tools.
- **Catalog** (4): search, describe, read call, write call.
- **Per operation**: generated at runtime for the areas listed in `TOOLSETS`.

HTTP mode validates IS JWTs (JWKS, `iss`, `aud`, `exp`), publishes RFC 9728 metadata and forwards
the bearer. Write calls ask for approval through MCP elicitation when the client supports it.
Details: [research.md](research.md), [data-model.md](data-model.md), [contracts/](contracts/).

## Technical Context

**Language/Version**: Go 1.25.5 (raised from 1.23 because mcp-go v1.6.0 requires it)

**Primary Dependencies**:

- `github.com/mark3labs/mcp-go` v1.6.0 (upgrade from v0.34.0, R4)
- `github.com/getkin/kin-openapi` v0.153.0 (new, R3)
- `github.com/lestrrat-go/jwx/v3` v3.4.0 (new, R5)
- `github.com/asgardeo/go` v0.0.18 (kept until migration, then removed, R17)

**Storage**: none. The specs are embedded with `go:embed`. Token caches are in memory.

**Testing**:

- `go test -race`, with an `httptest` fake IS (`internal/wso2is/istest`).
- A golden catalog stats file.
- An `integration` build tag against `wso2/wso2is:7.3.0`.
- `golangci-lint`.

**Target Platform**: Linux, macOS and Windows binaries (`CGO_ENABLED=0`); container-friendly.

**Project Type**: two CLI/server binaries (MCP over stdio and Streamable HTTP) sharing internal
packages.

**Performance Goals**:

- Catalog load under 1.5 s at startup (116 files), measured by a benchmark.
- Catalog search under 20 ms.
- Server-side overhead per tool call under 50 ms, excluding IS time.

**Constraints**:

- At most 100 tools by default (SC-002; the plan has 94).
- Results capped at `MAX_RESPONSE_BYTES`.
- No secrets in results or logs.
- TLS verification always on.

**Scale/Scope**:

- 921 operations, 20 areas, 2 binaries.
- About 98 curated tools (90 admin, 8 new self-service).
- HTTP mode serves tens of concurrent administrators.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Gate | Pre-research | Post-design |
|---|---|---|---|
| I. IS 7.3 first | IS-only target; module path kept; no gateway; version-keyed behavior | PASS | PASS: `IS_VERSION` selects the catalog; `PRODUCT_MODE` only changes the product name |
| II. Spec catalog source of truth | Vendored at a pinned commit; no hand edits; normalization tested; coverage test | PASS | PASS (R1, R2, R16). The three Login Flow AI entries live in code (`catalog/extra.go`), not in edited specs; see Complexity Tracking |
| III. Secure by default | Redaction, annotations, confirmation, `READ_ONLY`, `ALLOW_FLOW_APIS`, TLS, no token logs, JWT validation + RFC 9728 | PASS | PASS (R5, R10, R12, contracts/tool-conventions.md, contracts/http-transport.md), with elicitation as well |
| IV. Test-first | Tests before code; fake IS; integration tag; security tests | PASS | PASS: tasks put tests first in every phase |
| V. Simplicity | One client, framework, redaction, error mapper and transport; runtime tool generation; dependencies justified | PASS | PASS: three dependencies justified in R3, R4 and R5; the SDK is removed at the end |
| VI. LLM-grade tools | Curated + catalog + opt-in toolsets; budget; names ≤ 64; actionable errors | PASS | PASS (R8, R13) |
| VII. Compatibility | Env vars and tool names kept; per-binary semver; catalog per version | PASS | PASS: the binary is renamed `asgardeo-mcp` → `wso2is-mcp` (clarification); the migration is noted in the README and release notes |

## Project Structure

### Documentation (this feature)

```text
specs/001-wso2is-full-coverage/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   ├── configuration.md
│   ├── tool-conventions.md
│   ├── catalog-tools.md
│   ├── scim-tools.md
│   ├── admin-curated-tools.md
│   ├── self-service-tools.md
│   └── http-transport.md
├── checklists/requirements.md
└── tasks.md
```

### Source Code (repository root)

```text
api-specs/wso2is/7.3.0/
├── SOURCE                         # repo, commit, paths (R1)
├── root/*.yaml|yml                # 81 files, unmodified
└── org/*.yaml                     # 35 files, unmodified
api-specs/embed.go                 # package apispecs; //go:embed wso2is
scripts/sync-specs.sh              # refresh specs from wso2/docs-is at a commit

cmd/
├── wso2is-mcp/main.go             # admin server (replaces cmd/asgardeo-mcp)
└── self-service-mcp/main.go       # end-user server

internal/
├── config/                        # env parsing: Config (data-model.md), BASE_URL compat
├── wso2is/
│   ├── catalog/                   # load, normalize (R2, R9, R10, R11), area table, search, extra.go
│   │   └── testdata/stats.golden.json
│   ├── client/                    # HTTP, auth modes, org switch cache, validation, errors
│   ├── redact/                    # R12
│   └── istest/                    # httptest fake IS + JWKS + token endpoint (tests only)
├── transport/                     # stdio/HTTP serve, JWT middleware, RFC 9728, health
├── tools/
│   ├── toolkit/                   # Tool builder: annotations, confirmation, elicitation, result shaping
│   ├── catalog/                   # 4 catalog tools + per-operation generation (TOOLSETS)
│   ├── scim/                      # 21 ported + 8 gap tools
│   ├── admin/<area>/              # curated admin tools (existing 19 migrated + new 43)
│   └── enduser/                   # self-service (existing 28 + 8 new)
└── docsgen/                       # generates docs/*.md tool and scope tables

test/integration/                  # //go:build integration
docs/
├── wso2is-mcp.md                  # generated tables + hand-written guide
└── self-service-mcp.md
```

**Structure Decision**:

- Internal packages are shared by both binaries.
- `internal/asgardeo`, `internal/selfservice` and `internal/utils` are folded into
  `internal/wso2is/client`, `internal/wso2is/redact` and `internal/tools/toolkit`.
- `internal/tools/*.go` moves to `internal/tools/admin/<area>`.
- The old packages are deleted once nothing imports them (T106).

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| Three catalog entries declared in code (`catalog/extra.go`) for the Login Flow AI paths | `update_login_flow` (a frozen existing tool) uses endpoints that the 7.3 OpenAPI files do not describe | Removing the tool breaks Principle VII. Editing the vendored specs breaks Principle II |
| Stateful Streamable HTTP by default | Elicitation needs server-to-client requests on a session | Stateless HTTP would make FR-017a impossible in HTTP mode. `MCP_STATELESS=true` remains available |
