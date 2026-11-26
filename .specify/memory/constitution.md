# WSO2 IS MCP Server Constitution

## Core Principles

### I. WSO2 Identity Server 7.3 First

- The product target is WSO2 Identity Server (IS). The baseline version is 7.3.0. Asgardeo is
  not a supported target and carries no testing obligation.
- The Go module path MUST remain `github.com/asgardeo/mcp` so that upstream commits from
  `asgardeo/asgardeo-mcp-server` can be cherry-picked.
- The servers MUST run standalone. No feature may require the WSO2 AI Gateway or any other
  external proxy to be secure or functional.
- Product-specific behavior MUST be keyed on the IS version in the catalog, never on hostnames
  or string sniffing.

Rationale: the fork exists to cover IS completely; ambiguity about the target product is the
main source of wasted work in the upstream code.

### II. Spec Catalog Is the Source of Truth

- Official IS OpenAPI specs MUST be vendored under `api-specs/wso2is/<version>/{root,org}/`
  from `wso2/docs-is` at a pinned commit recorded in a `SOURCE` file.
- Vendored spec files MUST NOT be hand-edited. Every defect (missing, invalid or duplicate
  `operationId`, wrong `servers`, Swagger 2.0 files, duplicated specs, scopes in free text) MUST
  be fixed by normalization code that has tests.
- Every operation in the catalog MUST be reachable through the MCP surface or be explicitly
  classified (`excluded`, `flow`, `deprecated`) with a recorded reason. A coverage test MUST
  enforce this.
- Required scopes, operation kind (read, write, destructive) and toolset MUST be derived from
  the catalog, not duplicated by hand in tool code.

Rationale: 921 operations cannot be kept correct by hand; the official specs plus tested
normalization give one auditable source.

### III. Secure by Default (NON-NEGOTIABLE)

- Secrets MUST never reach the LLM context: client secrets, passwords, private keys, keystore
  material, OTP or backup codes, webhook and IdP secrets, and tokens are removed by one central
  redaction layer that every tool result passes through.
- Every tool MUST declare MCP annotations (`readOnlyHint`, `destructiveHint`). Write and
  destructive tools MUST instruct the agent to describe the exact change and obtain the user's
  explicit confirmation before calling.
- `READ_ONLY=true` MUST prevent registration of every non-read tool.
- Flow and credential APIs (interactive login, public self sign-up, account recovery, own
  password change, self account deletion, verification codes) MUST be disabled unless
  `ALLOW_FLOW_APIS=true`.
- TLS verification MUST never be disabled. Custom trust is configured only through `CERT_PATH`.
- Tokens, Authorization headers and secrets MUST never be logged.
- The HTTP transport MUST validate IS-issued JWTs (JWKS signature, `iss`, `aud`, `exp`, `nbf`)
  before any tool runs, MUST answer unauthenticated requests with `401` and a `WWW-Authenticate`
  header pointing at the RFC 9728 protected resource metadata it publishes, and then forwards the
  validated bearer token to IS, which enforces scopes on every call.

Rationale: an agent with identity-admin power is a high-value target; safety has to come from
the server's structure, not from the model's judgment.

### IV. Test-First (NON-NEGOTIABLE)

- A failing test MUST exist before the code that makes it pass (red, green, refactor).
- The catalog MUST have a coverage test over all vendored operations of the active version.
- HTTP client and tool behavior MUST be tested against an `httptest` fake IS. Unit tests MUST
  NOT require network access or a running IS.
- Integration tests against a real IS 7.3 container MUST live behind the `integration` build tag.
- Security behaviors in Principle III (redaction, annotations, `READ_ONLY`, `ALLOW_FLOW_APIS`,
  JWT validation) MUST each have dedicated tests.

Rationale: the repository started with zero tests; every later change depends on a safety net.

### V. Simplicity and Single Ownership

- There MUST be exactly one IS HTTP client, one tool-building framework, one redaction layer,
  one error-mapping function and one HTTP transport package, shared by both binaries.
- Per-operation tools MUST be generated at runtime from the catalog. Code generation of typed
  clients for every spec is prohibited.
- Every new third-party dependency MUST be justified in the feature's `plan.md` (what it owns,
  why the standard library or an existing dependency does not suffice).
- No speculative configuration, extension points or interfaces with a single implementation.

Rationale: the smallest system that covers all operations is a catalog plus one client; each
duplicate layer multiplies the cost of every IS version upgrade.

### VI. LLM-Grade Tool Design

- Frequent administrative tasks MUST have curated, task-shaped tools with precise descriptions
  and compact responses.
- Full API coverage MUST be available through a small fixed set of generic catalog tools
  (search, describe, read call, write call).
- Per-operation tools MUST be opt-in per toolset (`TOOLSETS`). The default registration MUST
  stay within a documented tool budget.
- Tool names MUST be unique, stable, snake_case and at most 64 characters.
- Errors MUST be actionable: HTTP status, IS error message, and for `403` the scopes the
  catalog says the operation requires.

Rationale: tool count and description quality determine whether an agent can use the server
at all.

### VII. Compatibility and Versioning

- Existing environment variables (`BASE_URL`, `CLIENT_ID`, `CLIENT_SECRET`, `CERT_PATH`,
  `PRODUCT_MODE`, `ASGARDEO_*` fallbacks, `TRANSPORT`, `PORT`, `ACCESS_TOKEN`, `PORTAL_NAME`) and
  existing tool names MUST keep working, or be removed only through a documented deprecation.
- Each binary is released with its own semantic version and tag prefix.
- The catalog is versioned per IS version. Supporting another IS version means adding a spec
  directory and normalization rules, not branching tool code.

Rationale: users configure MCP clients once; silent breakage of names or variables breaks them.

## Technology & Security Constraints

- Language: Go (the version in `go.mod`). MCP library: `github.com/mark3labs/mcp-go`.
- Binaries: `cmd/wso2is-mcp` (administration) and `cmd/self-service-mcp` (end user).
- Transports: stdio and MCP Streamable HTTP. HTTP servers MUST set header read timeouts and
  shut down gracefully.
- Authentication to IS: OAuth2 client credentials (stdio administration), static user token
  (stdio self-service), and validated bearer forwarding (HTTP).
- Organization APIs are reached through the IS `organization_switch` grant, per request.
- Every source file MUST carry the WSO2 Apache 2.0 license header used in existing files.

## Development Workflow & Quality Gates

- Every feature follows the Spec Kit flow in order: constitution, specify, clarify, plan,
  tasks, analyze, implement, converge. Implementation MUST NOT start while `analyze` reports
  CRITICAL issues.
- Tasks are executed in `tasks.md` order; a task is complete only when its tests pass.
- Merge gates: `gofmt -s` clean, `go vet ./...`, `golangci-lint run`, `go test -race ./...`.
- User-facing tool and scope tables in `docs/` MUST be generated from the tool registry and
  catalog, and checked in CI for drift.
- Agent guidance lives in `AGENTS.md`. `CLAUDE.md`, `GEMINI.md` and
  `.github/copilot-instructions.md` MUST defer to `AGENTS.md` and this constitution.

## Governance

- This constitution supersedes all other practices in the repository. Conflicts are resolved in
  favor of this document.
- Amendments are made through `/speckit-constitution`, recorded with a Sync Impact Report, and
  versioned semantically: MAJOR for removed or redefined principles, MINOR for added principles
  or materially expanded guidance, PATCH for clarifications.
- Every `plan.md` MUST pass the Constitution Check before research and again after design.
  Deviations MUST be recorded in its Complexity Tracking table with the rejected simpler option.
- Reviews and `/speckit-analyze` MUST treat any violation of a MUST statement as CRITICAL.

**Version**: 1.0.0 | **Ratified**: 2026-11-25 | **Last Amended**: 2026-11-25
