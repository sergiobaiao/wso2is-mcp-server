---

description: "Task list for Full WSO2 Identity Server 7.3 Coverage"
---

# Tasks: Full WSO2 Identity Server 7.3 Coverage

**Input**: Design documents from `/specs/001-wso2is-full-coverage/`

**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/, quickstart.md

**Tests**: REQUIRED. Constitution Principle IV (Test-First, NON-NEGOTIABLE) applies: in every
phase, write the test tasks first and confirm they fail before the implementation tasks.

**Organization**: Tasks are grouped by user story so each story can be implemented and tested on
its own.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies on incomplete tasks)
- **[Story]**: User story from spec.md (US1–US8)
- Paths are relative to the repository root (plan.md, Project Structure)

## Rules for the implementing agent

- Read `AGENTS.md` and `.specify/memory/constitution.md` before the first task.
- Every new `.go` file starts with the WSO2 Apache 2.0 header used in `cmd/self-service-mcp/main.go`.
- A task is done only when `go build ./...`, `go vet ./...` and the tests named or touched by
  the task pass with `-race`.
- Mark a task `[X]` in this file when it is done.

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Safety net for existing tools, toolchain, dependencies, vendored specs, CI.

- [X] T001 [P] Write `cmd/asgardeo-mcp/compat_test.go` (package `main`): build the server with `setupServer()`, send an in-process `tools/list` JSON-RPC request through `MCPServer.HandleMessage`, normalize each tool (name, description, input schema with sorted `required` and property names/types, annotations) and compare with `internal/tools/testdata/admin_tools.golden.json` (`-update` flag writes it); generate the golden for the 19 existing tools from the unchanged code
- [X] T002 [P] Write `cmd/self-service-mcp/compat_test.go` the same way for the 28 existing tools (`setupServer(client)` with a client built from a dummy `BASE_URL` and token), golden `internal/tools/testdata/enduser_tools.golden.json`
- [X] T003 [P] Add the WSO2 Apache 2.0 license header (copied from `cmd/self-service-mcp/main.go`, year 2025) to `internal/config/config.go`, `internal/asgardeo/client.go` and `internal/utils/utils.go`, the only tracked `.go` files without it
- [X] T004 Upgrade `go.mod` to `go 1.25.5` and `github.com/mark3labs/mcp-go` v1.6.0 (research.md R4); fix compile errors in `cmd/asgardeo-mcp/main.go`, `cmd/self-service-mcp/main.go`, `internal/tools/**/*.go` without changing tool names, inputs or outputs; `go build ./...` and the T001/T002 compat tests pass unchanged (if mcp-go serializes defaults differently, adjust only the normalizer, never the golden)
- [X] T005 Vendor the specs: copy the 81 root files and 35 org files from `wso2/docs-is` commit `832c86be91f9753ecc67054f5737a5f0e389f395` into `api-specs/wso2is/7.3.0/root/` and `api-specs/wso2is/7.3.0/org/`, and write `api-specs/wso2is/7.3.0/SOURCE` (repository, commit, both source paths) per research.md R1
- [X] T006 Create `scripts/sync-specs.sh` (args: commit, IS version; downloads both directories through `https://raw.githubusercontent.com/wso2/docs-is/<commit>/...` using the GitHub contents listing; rewrites `SOURCE`; fails on any download error)
- [X] T007 Create `api-specs/embed.go` (package `apispecs`, `//go:embed wso2is`, exported `FS embed.FS`)
- [X] T008 [P] Add `github.com/getkin/kin-openapi` v0.153.0 to `go.mod` (research.md R3) with `go get` and `go mod tidy`
- [X] T009 [P] Add `.golangci.yml` enabling the default linters plus `gosec`, `errorlint`, `revive` (research.md R16), and add the missing `lint` target (`golangci-lint run`) to `Makefile`
- [X] T010 [P] Create `.github/workflows/ci.yml` running on pull requests and pushes to `main`: `gofmt -s -l` (fail if output), `go vet ./...`, `golangci-lint run`, `go test -race ./...`

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Spike validation, config, fake IS, catalog, redaction, IS client, tool toolkit, basic
transport, new admin binary. No user story starts before this phase is complete.

### Spike (validate research assumptions against a real IS 7.3)

- [X] T011 Create `test/integration/spike_test.go` (`//go:build integration`) with one test per assumption: (a) client_credentials `scope=SYSTEM` token works on `/api/server/v1/applications` (R7); (b) organization_switch from an M2M token for a shared app (R6); (c) DCR and OAuth2 scope endpoints work under `/t/carbon.super` (R14); (d) a JWT from an MCP Client app authorized for an MCP server has `aud` containing the MCP resource and is accepted by `/scim2/Users` (R5); (e) Login Flow AI endpoints respond or fail with a recognizable error (R17)
- [ ] T012 Run T011 against `wso2/wso2is:7.3.0` and record each outcome (pass, or the fallback chosen) under a new "Spike results" section at the end of `specs/001-wso2is-full-coverage/research.md`; if (b) or (d) fails, update `contracts/` and this file before Phase 5 or 6 starts

### Configuration

- [X] T013 [P] Write `internal/config/config_test.go`: every variable in `contracts/configuration.md` (defaults, enums, `BASE_URL` parsing into `data-model.md` Config.ServerURL and Config.TenantDomain, conflict error, `ASGARDEO_*` fallbacks, `TOOLSETS` validation against areas, derived Config.Issuer and Config.JWKSURL), and that errors never contain secret values
- [X] T014 Implement `internal/config/config.go` per `contracts/configuration.md`, setting `data-model.md` Config.ServerURL, Config.TenantDomain, Config.AuthMode (enum), Config.Transport (enum), Config.Stateless, Config.Issuer, Config.JWKSURL, Config.Resource, Config.ReadOnly, Config.AllowFlowAPIs, Config.IncludeDeprecated, Config.Toolsets (validated against areas), Config.MaxResponseBytes, Config.SCIMProfileAttributes, Config.ISVersion (replace the current `Load()`; keep exported constants in `internal/config/constants.go`, adding new names)

### Fake IS for tests

- [X] T015 Create `internal/wso2is/istest/fakeis.go`: `httptest.NewTLSServer` with route registration by method and path, request recording, `/oauth2/token` (client_credentials, organization_switch), `/oauth2/jwks` backed by a generated RSA key, and a `SignJWT(claims)` helper; plus `internal/wso2is/istest/fakeis_test.go`

### Catalog

- [X] T016 [P] Write `internal/wso2is/catalog/normalize_test.go`: table tests for operationId derivation, sanitizing (`getUser by id` → `getUserById`), duplicate suffixing (`createSMSSender`), path-prefix extraction and the per-file prefix table (research.md R2), and the kind rules and overrides (research.md R9, `data-model.md` Operation.Kind)
- [X] T017 [P] Write `internal/wso2is/catalog/catalog_test.go`: load 7.3.0 and assert (1) all 116 files load; (2) every file maps to exactly one area (`data-model.md` SpecFile.Area); (3) `data-model.md` Operation.ID is unique and matches `<area>.<operationId>` or `<area>.org.<operationId>`; (4) every operation has a kind, a path starting with its SpecFile.PathPrefix, and sorted unique scopes (`data-model.md` Operation.Scopes); (5) flow and deprecated flags follow research.md R10 and R11; (6) the stats equal `testdata/stats.golden.json` (`-update` flag rewrites it)
- [X] T018 Implement `internal/wso2is/catalog/load.go`: `Load(fsys fs.FS, version string)` walks the given FS (production passes `apispecs.FS`) for the version, loads each file with kin-openapi, converts Swagger 2.0 files with `openapi2conv` (research.md R2) and sets `data-model.md` SpecFile.Family (from the directory), SpecFile.Name and SpecFile.Doc
- [X] T019 Implement `internal/wso2is/catalog/areas.go` with the area table from `data-model.md` (Area table), which sets `data-model.md` SpecFile.Area (exactly one per file), the per-file prefix overrides that set `data-model.md` SpecFile.PathPrefix (research.md R2), and `data-model.md` SpecFile.Deprecated and SpecFile.Flow (research.md R10, R11)
- [X] T020 Implement `internal/wso2is/catalog/normalize.go`: `data-model.md` Operation.OperationID rules (research.md R2), Operation.Method (upper case), deduplication by family+method+full path, `data-model.md` Operation.Scopes (regex extraction, sorted, unique), `data-model.md` Operation.Summary (max 200 chars) and Operation.Description cleanup, parameter and body extraction (`data-model.md` Param.In, Param.Required, BodySpec.ContentType), `data-model.md` Operation.Kind (research.md R9), `data-model.md` Operation.Flow (research.md R10, including the `pi-info` exception), `data-model.md` Operation.Deprecated (research.md R11), Operation.Source
- [X] T021 Implement `internal/wso2is/catalog/catalog.go`: `Catalog` (`data-model.md` Catalog.Version, Catalog.Source, Catalog.Operations sorted by ID), `data-model.md` Operation.ID (`<area>.<operationId>` or `<area>.org.<operationId>`, uniqueness enforced at load), `Get`, `Areas`, `Stats`, `Availability(op, cfg)` (`data-model.md` Operation derived `Availability()`)
- [X] T022 Implement `internal/wso2is/catalog/extra.go` with the three Login Flow AI operations (research.md R17, flagged `data-model.md` Operation.AIFeature) and generate `internal/wso2is/catalog/testdata/stats.golden.json`; T016 and T017 pass
- [X] T023 [P] Add `internal/wso2is/catalog/bench_test.go` benchmarking full load (target under 1.5 s, plan.md Performance Goals)
- [X] T024 [P] Write `internal/wso2is/catalog/version_test.go` with a synthetic version in `internal/wso2is/catalog/testdata/versions/9.9.9/{root,org}/` (two minimal files whose names exist in the area table, plus `SOURCE`): `Load(os.DirFS(...), "9.9.9")` succeeds, operations get areas, kinds and ids, and a second test builds per-operation tool definitions for it through the same catalog API with no code change (SC-008); assert an unknown `IS_VERSION` fails startup with the list of available versions

### Redaction

- [X] T025 [P] Write `internal/wso2is/redact/redact_test.go` with fixtures in `internal/wso2is/redact/testdata/` for application, IdP, user store, notification sender, webhook, action, keystore, agent creation and TOTP responses (research.md R12); assert zero secret values remain and that `[REDACTED]` marks each removed value
- [X] T026 Implement `internal/wso2is/redact/redact.go` (keys, substrings, property arrays) per research.md R12, superseding `redact()` in `internal/tools/enduser/enduser.go`

### IS client

- [X] T027 [P] Write `internal/wso2is/client/client_test.go` against `istest`: client_credentials token fetch and caching (R7), static token, bearer from context (`data-model.md` CredentialContext.Mode, CredentialContext.BearerToken), tenant URL building (R14), `CERT_PATH` trust, SCIM content type, query/path parameter encoding, and that no log output contains tokens
- [X] T028 [P] Write `internal/wso2is/client/validate_test.go`: request validation with openapi3filter for missing path params, wrong types, unknown query params, invalid bodies; the error lists field names (FR-005)
- [X] T029 [P] Write `internal/wso2is/client/errors_test.go`: messages per `contracts/tool-conventions.md` (Results), including required scopes on 403 (FR-028)
- [X] T030 [P] Write `internal/wso2is/client/truncate_test.go`: cutting at the last complete element of a top-level array and of SCIM `Resources`, with `truncated`, `returned`, `total` and paging hints (research.md R15)
- [X] T031 Implement `internal/wso2is/client/client.go` (`Do(ctx, op, PathParams, Query, Body) (any, error)`), `internal/wso2is/client/auth.go` (three modes selected by `data-model.md` CredentialContext.Mode, token cache with 30 s margin) and `internal/wso2is/client/context.go` (`data-model.md` CredentialContext.Mode, CredentialContext.BearerToken, CredentialContext.Subject, CredentialContext.OrganizationID; headers listed under `data-model.md` Param are never taken from agent input)
- [X] T032 Implement `internal/wso2is/client/validate.go`, `internal/wso2is/client/errors.go` and `internal/wso2is/client/truncate.go` so T028–T030 pass

### Tool toolkit

- [X] T033 [P] Write `internal/tools/toolkit/toolkit_test.go`: annotations per kind, confirmation sentence appended to write and destructive descriptions, results passed through redact and truncate, `"Done."` for empty bodies, error rendering, name validation (`data-model.md` Tool.Name), registry rejecting duplicate names, `READ_ONLY` filtering (FR-018) and a tool counter for budget tests
- [X] T034 [P] Write `internal/tools/toolkit/elicit_test.go` using an in-process mcp-go client with and without the elicitation capability: accept proceeds; decline, cancel, `approve=false` and timeout return "Not approved by the user. Nothing was changed." with zero requests to `istest` (FR-017a, SC-009)
- [X] T035 Implement `internal/tools/toolkit/tool.go` (builder for `data-model.md` Tool: Name, Layer, Kind, Scopes, OrgCapable, Description, Handler) and `internal/tools/toolkit/registry.go`
- [X] T036 Implement `internal/tools/toolkit/elicit.go` (summary message and boolean `approve` schema per `contracts/tool-conventions.md` Approval; 120 s timeout)
- [X] T037 [P] Add `internal/tools/toolkit/bench_test.go` benchmarking one read and one write tool call end to end against `istest` with zero-latency routes; per-call server overhead under 50 ms (plan.md Performance Goals)

### Transport and binary

- [X] T038 [P] Write `internal/transport/transport_test.go`: stdio serve, HTTP `/mcp` and `/healthz`, stateful default, `MCP_STATELESS`, `ReadHeaderTimeout`, graceful shutdown (`contracts/http-transport.md`, Server behavior)
- [X] T039 Implement `internal/transport/transport.go` by moving `serveHTTP` and `requireAuthorization` out of `cmd/self-service-mcp/main.go`; `requireAuthorization` stays as the legacy no-validation middleware for self-service without `MCP_RESOURCE`
- [X] T040 Create `cmd/wso2is-mcp/main.go` (config, catalog, client, toolkit registry, transport; instructions text warning that tool data is untrusted) and move the existing tool registration and `cmd/asgardeo-mcp/compat_test.go` (T001) from `cmd/asgardeo-mcp/` to `cmd/wso2is-mcp/`; delete `cmd/asgardeo-mcp/`; update `Makefile` (`SERVER ?= wso2is-mcp self-service-mcp`, `BINARY_NAME`), `.gitignore`, `.github/workflows/release.yml` (server option `wso2is-mcp`) and `.github/scripts/release.sh` (tag prefix `wso2is-mcp-v`, package `wso2is_mcp_server`, legacy `asgardeo-mcp-v` and `v` tags read for the current version)

**Checkpoint**: `go test -race ./...` passes; `wso2is-mcp` starts and serves the existing 19 tools.

---

## Phase 3: User Story 1 - Reach any IS 7.3 operation through a small set of tools (Priority: P1) 🎯 MVP

**Goal**: Four catalog tools give access to every operation (FR-001–FR-006).

**Independent Test**: quickstart.md §2: search, describe, read call and write call against a test IS for three areas.

### Tests for User Story 1

- [X] T041 [P] [US1] Write `internal/tools/catalog/search_test.go`: ranking, filters `area`, `kind`, `organization`, `limit` bounds, output shape per `contracts/catalog-tools.md`
- [X] T042 [P] [US1] Write `internal/tools/catalog/describe_test.go`: fields per `contracts/catalog-tools.md`, body schema depth limit 4, `call_with`, `example`, availability text for flow and deprecated operations
- [X] T043 [P] [US1] Write `internal/tools/catalog/call_test.go`: read tool refuses non-read kinds; write tool refuses read kinds; refusals per `contracts/tool-conventions.md` (flow, deprecated, organization mismatch); validation failures send no request; write calls go through elicitation
- [X] T044 [US1] Write `internal/tools/catalog/coverage_test.go`: for every catalog operation, build a minimal valid input from its schema, call through the matching tool against `istest`, and assert method and path reached the fake IS, or the expected refusal for flow and deprecated operations (FR-006, SC-001)

- [X] T045 [P] [US1] Write `internal/tools/catalog/search_eval_test.go` with `internal/tools/catalog/testdata/search_eval.json`: 30 administrator task phrases across at least 10 areas, each with the expected operation id and two queries (first and refined); pass when the expected id is in the top 10 results of the first or second query for at least 27 of 30 entries (SC-007)
- [X] T046 [P] [US1] Add `internal/wso2is/catalog/search_bench_test.go`: search over the full 7.3.0 catalog under 20 ms (plan.md Performance Goals)

### Implementation for User Story 1

- [X] T047 [US1] Implement `internal/wso2is/catalog/search.go` (token match over `data-model.md` Operation.ID, Operation.Summary, Operation.Path, Operation.Area; research.md R8)
- [X] T048 [US1] Implement `internal/tools/catalog/tools.go` with `search_operations`, `describe_operation`, `call_read_operation`, `call_write_operation` per `contracts/catalog-tools.md` using `toolkit` and `client`
- [X] T049 [US1] Register the catalog tools in `cmd/wso2is-mcp/main.go` (`call_write_operation` omitted when `READ_ONLY=true`)
- [X] T050 [US1] Add `test/integration/catalog_test.go` (`//go:build integration`): quickstart.md §2 steps

**Checkpoint**: US1 complete; every operation is reachable or refused with a reason.

---

## Phase 4: User Story 2 - Manage users, groups, roles and agents with task-shaped tools (Priority: P1)

**Goal**: The 21 SCIM tools plus 8 gap tools (`contracts/scim-tools.md`, FR-007, FR-010).

**Independent Test**: quickstart.md §3.

### Tests for User Story 2

- [X] T051 [P] [US2] Write `internal/tools/scim/users_test.go`: `list_users`, `search_users`, `get_user`, `create_user` (both input spellings, conflict error, password never echoed), `update_user_attribute`, `update_user_profile` (`SCIM_PROFILE_ATTRIBUTES`, legacy names), `delete_user`, `patch_user`; request bodies and `error`/`errorStatus` outputs per `contracts/scim-tools.md`
- [X] T052 [P] [US2] Write `internal/tools/scim/groups_test.go`: `list_groups`, `create_group`, `add_user_to_group` (user lookup then PATCH), `remove_user_from_group`, `delete_group`, `get_group`, `update_group`
- [X] T053 [P] [US2] Write `internal/tools/scim/roles_test.go`: `list_roles`, `create_role`, `assign_role_to_user`, `remove_role_from_user` (PATCH `/v2/Roles/{id}` with paths `users`), `delete_role`, `get_role`, `assign_role_to_group`, `remove_role_from_group`, `update_role_permissions`
- [X] T054 [P] [US2] Write `internal/tools/scim/agents_bulk_test.go`: `list_agents`, `create_agent` (credentials redacted), `delete_agent`, `get_agent`, `bulk_create_users` (array 1–100, legacy `user1*`/`user2*`, per-user results)
- [X] T055 [P] [US2] Write `internal/tools/scim/readonly_test.go`: with `READ_ONLY=true` only the read SCIM tools are registered (US2 scenario 5)

### Implementation for User Story 2

- [X] T056 [US2] Implement `internal/tools/scim/common.go`: compact attribute list, SCIM error mapping to `error`/`errorStatus`, catalog operation lookups by id (fail at startup if missing)
- [X] T057 [P] [US2] Implement `internal/tools/scim/users.go` (replaces `internal/tools/users.go`; delete it)
- [X] T058 [P] [US2] Implement `internal/tools/scim/groups.go`
- [X] T059 [P] [US2] Implement `internal/tools/scim/roles.go`
- [X] T060 [P] [US2] Implement `internal/tools/scim/agents.go` and `internal/tools/scim/bulk.go`
- [X] T061 [US2] Register SCIM tools in `cmd/wso2is-mcp/main.go`; descriptions copied from the Arazzo `summary` text adapted to MCP errors
- [X] T062 [US2] Add `test/integration/scim_test.go` (`//go:build integration`): quickstart.md §3 sequence

**Checkpoint**: US1 and US2 work independently.

---

## Phase 5: User Story 3 - Administer sub-organizations (Priority: P2)

**Goal**: `organization_id` on org operations and org-capable tools (FR-011, FR-012).

**Independent Test**: quickstart.md §4.

### Tests for User Story 3

- [ ] T063 [P] [US3] Write `internal/wso2is/client/orgswitch_test.go`: organization_switch request fields (research.md R6), cache key per `data-model.md` OrgTokenCache entry (Key, ExpiresAt), refresh before expiry, refusal message on IS rejection, HTTP mode without `CLIENT_ID` returns a configuration error, `/t/<tenant>/o/` path building
- [X] T064 [P] [US3] Write `internal/tools/scim/org_test.go`: SCIM tools with `organization_id` call `/o/scim2/...`; agent tools reject `organization_id`

### Implementation for User Story 3

- [ ] T065 [US3] Implement `internal/wso2is/client/orgswitch.go` with the cache keyed by `data-model.md` OrgTokenCache entry Key (SHA-256 of the root token plus organization id; the root token is never stored as a key) and evicted by `data-model.md` OrgTokenCache entry ExpiresAt (30 s margin), and route org operations (`data-model.md` CredentialContext.OrganizationID) in `internal/wso2is/client/client.go`
- [ ] T066 [US3] Add `organization_id` handling to catalog tools in `internal/tools/catalog/tools.go` and to org-capable SCIM tools in `internal/tools/scim/common.go` (`data-model.md` Tool.OrgCapable)
- [ ] T067 [US3] Add `test/integration/org_test.go` (`//go:build integration`): quickstart.md §4

**Checkpoint**: B2B operations work by organization id.

---

## Phase 6: User Story 4 - Host the administration server for a team (Priority: P2)

**Goal**: JWT validation, RFC 9728 metadata, bearer forwarding (FR-022–FR-026).

**Independent Test**: quickstart.md §5.

### Tests for User Story 4

- [X] T068 [P] [US4] Write `internal/transport/jwt_test.go` with `istest` JWKS: missing token, non-JWT, bad signature, wrong `iss`, wrong `aud`, expired, not yet valid, skew, unknown `kid` refresh rate limit; `WWW-Authenticate` values per `contracts/http-transport.md`; zero IS calls on rejection (SC-006); reasons never include the token
- [X] T069 [P] [US4] Write `internal/transport/metadata_test.go`: both well-known paths, fields per `contracts/http-transport.md`, `scopes_supported` equals the union of registered tool scopes
- [ ] T070 [P] [US4] Write `cmd/wso2is-mcp/main_test.go`: HTTP mode requires `MCP_RESOURCE`; a valid token reaches a tool and `istest` sees the same bearer (`data-model.md` CredentialContext.BearerToken); `CredentialContext.Subject` appears in the access log and the token does not

### Implementation for User Story 4

- [X] T071 [US4] Add `github.com/lestrrat-go/jwx/v3` v3.4.0 (research.md R5) and implement `internal/transport/jwt.go` (JWKS cache, validation, context injection)
- [X] T072 [US4] Implement `internal/transport/metadata.go` (RFC 9728) and wire both into `internal/transport/transport.go`
- [ ] T073 [US4] Enable HTTP mode in `cmd/wso2is-mcp/main.go`; in `cmd/self-service-mcp/main.go` use JWT validation when `MCP_RESOURCE` is set, else keep legacy behavior with a startup warning (FR-026)
- [ ] T074 [US4] Add `test/integration/http_test.go` (`//go:build integration`): quickstart.md §5 curl checks and a tool call with an MCP Client app token

**Checkpoint**: HTTP hosting works without any gateway.

---

## Phase 7: User Story 5 - Curated tools for common administration areas (Priority: P2)

**Goal**: Migrate the 19 existing tools and add 43 new curated tools (`contracts/admin-curated-tools.md`, FR-008, FR-009).

**Independent Test**: run each area's tools against a test IS and check the Console.

### Tests for User Story 5

- [ ] T075 [US5] Write `internal/tools/admin/compat_test.go`: the registered admin tools still match `internal/tools/testdata/admin_tools.golden.json` (captured by T001 before any change), allowing only added optional `organization_id` and annotations; plus output-shape assertions for the 19 existing tools against `istest`
- [ ] T076 [P] [US5] Write `internal/tools/admin/operations_test.go`: every catalog operation id referenced by a curated tool exists in the 7.3.0 catalog
- [ ] T077 [P] [US5] Write `internal/tools/admin/budget_test.go`: default config registers ≤ 100 tools (SC-002) and exactly the names listed in `contracts/admin-curated-tools.md`, `contracts/scim-tools.md` and `contracts/catalog-tools.md`
- [ ] T078 [P] [US5] Write tests per area in `internal/tools/admin/applications/applications_test.go`, `internal/tools/admin/apiauth/apiauth_test.go`, `internal/tools/admin/idp/idp_test.go`, `internal/tools/admin/governance/governance_test.go`, `internal/tools/admin/userstores/userstores_test.go`, `internal/tools/admin/sessions/sessions_test.go`, `internal/tools/admin/organizations/organizations_test.go`, `internal/tools/admin/branding/branding_test.go`, `internal/tools/admin/extensions/extensions_test.go`, `internal/tools/admin/workflows/workflows_test.go`, `internal/tools/admin/claims/claims_test.go` covering each tool in `contracts/admin-curated-tools.md` (request shape, kind, redaction of secrets such as client secrets and IdP secrets)

### Implementation for User Story 5

- [ ] T079 [US5] Migrate `internal/tools/applications.go` to `internal/tools/admin/applications/applications.go` on `toolkit` and `client` (catalog ids for application operations, `catalog/extra.go` for Login Flow AI); outputs use `SERVER_URL` and `TENANT_DOMAIN`; delete the old file
- [X] T080 [US5] Migrate `internal/tools/api_resources.go` to `internal/tools/admin/apiauth/apiauth.go` and `internal/tools/claims.go` to `internal/tools/admin/claims/claims.go`; delete the old files
- [ ] T081 [P] [US5] Add `delete_application` and `share_application` in `internal/tools/admin/applications/applications.go`
- [X] T082 [P] [US5] Add the 5 api_authorization tools in `internal/tools/admin/apiauth/apiauth.go`
- [X] T083 [P] [US5] Implement the 5 identity_providers tools (templates `google`, `github`, `facebook`, `microsoft`, `apple`, `oidc`, `saml`) in `internal/tools/admin/idp/idp.go`
- [X] T084 [P] [US5] Implement the 4 governance_security tools in `internal/tools/admin/governance/governance.go`
- [X] T085 [P] [US5] Implement the 4 user_stores tools in `internal/tools/admin/userstores/userstores.go`
- [X] T086 [P] [US5] Implement the 2 session tools in `internal/tools/admin/sessions/sessions.go`
- [ ] T087 [P] [US5] Implement the 5 organizations tools in `internal/tools/admin/organizations/organizations.go`
- [ ] T088 [P] [US5] Implement the 5 branding_notifications tools in `internal/tools/admin/branding/branding.go`
- [ ] T089 [P] [US5] Implement the 6 extensions tools in `internal/tools/admin/extensions/extensions.go`
- [ ] T090 [P] [US5] Implement the 4 workflows_approvals tools in `internal/tools/admin/workflows/workflows.go`
- [X] T091 [P] [US5] Add `create_local_claim` in `internal/tools/admin/claims/claims.go`
- [ ] T092 [US5] Create `internal/tools/admin/register.go` (registers all admin areas) and call it from `cmd/wso2is-mcp/main.go`; T075–T078 pass
- [ ] T093 [US5] Add `test/integration/admin_test.go` (`//go:build integration`) covering SC-004: `create_single_page_app`; `create_api_resource` with two scopes, `create_role`, `update_role_permissions` with those scopes and `assign_role_to_user`; `create_user` then `add_user_to_group`; and `list_users` with `organization_id` of a sub-organization

**Checkpoint**: 90 curated tools + 4 catalog tools by default.

---

## Phase 8: User Story 6 - Expose one tool per operation for chosen areas (Priority: P3)

**Goal**: Runtime per-operation tools selected by `TOOLSETS` (FR-013, FR-014).

**Independent Test**: enable two areas and list tools.

### Tests for User Story 6

- [ ] T094 [P] [US6] Write `internal/tools/catalog/operations_test.go`: names per `contracts/tool-conventions.md` (Names) and `data-model.md` Tool.Name, unique over all operations with `TOOLSETS=all`; input schema from `data-model.md` Param and BodySpec; kinds and annotations; `READ_ONLY` filtering; flow and deprecated operations not generated unless enabled

### Implementation for User Story 6

- [ ] T095 [US6] Implement `internal/tools/catalog/operations.go` (generation from `data-model.md` Operation into `data-model.md` Tool with Layer `operation`) and register it from `cmd/wso2is-mcp/main.go` when `TOOLSETS` is set

**Checkpoint**: per-operation toolsets work.

---

## Phase 9: User Story 7 - End users manage more of their own account (Priority: P3)

**Goal**: Self-service additions (`contracts/self-service-tools.md`, FR-027).

**Independent Test**: sign in as a test user and run each new tool.

### Tests for User Story 7

- [ ] T096 [US7] Write `internal/tools/enduser/compat_test.go`: the 28 existing tools still match `internal/tools/testdata/enduser_tools.golden.json` (captured by T002 before any change), allowing only added annotations; plus output-shape assertions against `istest`
- [ ] T097 [P] [US7] Write `internal/tools/enduser/approvals_test.go`, `internal/tools/enduser/organizations_test.go`, `internal/tools/enduser/export_test.go`, `internal/tools/enduser/consent_records_test.go` and extend `internal/tools/enduser/applications_test.go` for the v2-then-v1 fallback

### Implementation for User Story 7

- [ ] T098 [US7] Migrate `internal/tools/enduser/*.go` to `toolkit`, `client` and `redact` (remove `newTool`, `redact`, `sensitiveKeys` from `internal/tools/enduser/enduser.go`; keep `Tools(c, clientID)` as the entry point); delete `internal/selfservice/client.go` once unused
- [ ] T099 [P] [US7] Implement `internal/tools/enduser/approvals.go` (3 tools)
- [ ] T100 [P] [US7] Implement `internal/tools/enduser/organizations.go` (`list_my_organizations`) and `internal/tools/enduser/export.go` (`export_my_data`; `GET /pi-info` is not a flow operation per research.md R10, so no exception is needed)
- [ ] T101 [P] [US7] Implement `internal/tools/enduser/consent_records.go` (3 tools) and the authorized apps v2 fallback in `internal/tools/enduser/applications.go`

**Checkpoint**: self-service server has 36 tools.

---

## Phase 10: User Story 8 - Opt in to flow and credential APIs (Priority: P3)

**Goal**: `ALLOW_FLOW_APIS=true` enables flow operations (FR-019).

**Independent Test**: call a flow operation with the setting off and on.

- [X] T102 [P] [US8] Write `internal/tools/catalog/flow_test.go`: with the flag off, describe marks flow operations disabled with the reason and how to enable; calls are refused with zero requests; with the flag on, calls go through `call_write_operation` with elicitation; covers all 51 unique operations classified `flow` and asserts the 2 unique `pi-info` operations are not flow (research.md R10)
- [X] T103 [US8] Make `internal/wso2is/catalog/catalog.go` Availability and `internal/tools/catalog/tools.go` honor `data-model.md` Config.AllowFlowAPIs; T102 passes

**Checkpoint**: all user stories complete.

---

## Phase 11: Polish & Cross-Cutting Concerns

- [ ] T104 [P] Write `internal/docsgen/docsgen_test.go` and implement `internal/docsgen/docsgen.go` plus `cmd/docsgen/main.go`: generate the tool and scope tables between `<!-- BEGIN GENERATED -->` markers in `docs/wso2is-mcp.md` and `docs/self-service-mcp.md`; add `make docs` and a CI step that fails on drift (FR-033)
- [ ] T105 Rename `docs/asgardeo-mcp.md` to `docs/wso2is-mcp.md` and rewrite it for IS 7.3 (setup with M2M app, HTTP mode with MCP Client app and MCP server resource, `organization_id`, `TOOLSETS`, `READ_ONLY`, `ALLOW_FLOW_APIS`, elicitation, migration from `asgardeo-mcp`); update `README.md` and `docs/self-service-mcp.md` (new tools, `MCP_RESOURCE`)
- [ ] T106 Remove `github.com/asgardeo/go`: delete `internal/asgardeo/`, `internal/selfservice/`, `internal/utils/` and any remaining imports; `go mod tidy` (research.md R17)
- [ ] T107 [P] Write `test/integration/redaction_test.go` (`//go:build integration`): create an app, an IdP with a secret, an email sender with a password and an agent, then read them through tools and assert no secret appears (SC-005)
- [ ] T108 Run every quickstart.md section against `wso2/wso2is:7.3.0` and record results in `specs/001-wso2is-full-coverage/quickstart.md` under "Validation log"
- [ ] T109 Update `AGENTS.md` sections "Build and test" and "Architecture" to match the delivered code (package names, commands)

---

## Dependencies & Execution Order

### Phase Dependencies

- Phase 1 (Setup) → Phase 2 (Foundational) → user story phases → Phase 11 (Polish).
- T001 and T002 (compat goldens) MUST be done before T004 and before any other change to existing tools (Principle IV).
- T012 results can change Phase 5 (US3) and Phase 6 (US4) contracts; those phases start only after T012.

### User Story Dependencies

- **US1 (P1)**: after Phase 2. MVP.
- **US2 (P1)**: after Phase 2; independent of US1 (uses catalog lookups, not catalog tools).
- **US3 (P2)**: after Phase 2 and T012; adds `organization_id` to US1 and US2 tools when present (T066 touches files from T048 and T056).
- **US4 (P2)**: after Phase 2 and T012; independent of other stories.
- **US5 (P2)**: after Phase 2; T092 shares `cmd/wso2is-mcp/main.go` with T049, T061; T093 also needs US2 and US3.
- **US6 (P3)**: after US1 (reuses `internal/tools/catalog`).
- **US7 (P3)**: after Phase 2; independent.
- **US8 (P3)**: after US1.
- **Polish**: T106 after US5 and US7 (all SDK users migrated).

### Within Each User Story

- Tests first, confirmed failing; then implementation; then the integration test.
- Tasks touching `cmd/wso2is-mcp/main.go` (T040, T049, T061, T073, T092, T095) run sequentially.

### Parallel Opportunities

- Phase 1: T001, T002, T003 first (in parallel); then T008, T009, T010.
- Phase 2: T013, T016, T017, T023, T024, T025, T027–T030, T033, T034, T037, T038 (test files are independent).
- US2: T051–T055 together; T057–T060 together after T056.
- US5: T076–T078 together; T081–T091 together after T079–T080.
- US1, US2, US4 and US7 can proceed in parallel after Phase 2.

---

## Parallel Example: User Story 5

```bash
Task: "Implement the 5 identity_providers tools in internal/tools/admin/idp/idp.go"
Task: "Implement the 4 governance_security tools in internal/tools/admin/governance/governance.go"
Task: "Implement the 4 user_stores tools in internal/tools/admin/userstores/userstores.go"
Task: "Implement the 6 extensions tools in internal/tools/admin/extensions/extensions.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Phase 1 Setup.
2. Phase 2 Foundational, including the spike (T011–T012).
3. Phase 3 US1.
4. Stop and validate with quickstart.md §1–§2.

### Incremental Delivery

1. Setup + Foundational.
2. US1 → validate (full coverage MVP).
3. US2 → validate (SCIM parity with the Arazzo tools).
4. US3, US4 → validate (B2B, hosted HTTP).
5. US5 → validate (curated admin).
6. US6, US7, US8 → validate.
7. Polish, then `/speckit-converge`.

---

## Notes

- [P] tasks touch different files and do not depend on incomplete tasks.
- Commit after each task or logical group with a message naming the task id.
- Never hand-edit files under `api-specs/`; use `scripts/sync-specs.sh` (Principle II).
