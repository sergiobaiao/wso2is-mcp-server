# Copilot instructions

Read and follow `AGENTS.md` at the repository root before any change. It is the canonical guide
for this repository.

- Precedence: `.specify/memory/constitution.md` > `specs/001-wso2is-full-coverage/spec.md` >
  `plan.md`, `research.md`, `data-model.md`, `contracts/` > `tasks.md` > `AGENTS.md`.
- Spec Kit has no Copilot integration installed here. To run a Spec Kit step, follow the
  matching `.agents/skills/speckit-<step>/SKILL.md` (for example `speckit-implement`).
- Implement only tasks listed in `specs/001-wso2is-full-coverage/tasks.md`, in order, with tests
  first. Mark each finished task `[X]`.
- Never hand-edit `api-specs/`. Never put secrets in code, logs or tool results. Never disable
  TLS verification.
