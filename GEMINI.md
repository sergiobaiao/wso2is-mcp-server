# GEMINI.md

Guidance for Gemini CLI in this repository.

@./AGENTS.md

## Gemini CLI specifics

- Spec Kit commands are installed in `.gemini/commands/speckit.*.toml`. Invoke them as
  `/speckit.implement`, `/speckit.converge`, `/speckit.analyze` and so on.
- The active feature is `specs/001-wso2is-full-coverage`. Spec Kit reads it from
  `.specify/feature.json`, which is not committed; create it as described in `AGENTS.md`
  section 3.
- Antigravity and Gemini CLI follow the same rules. Read `AGENTS.md` section 8 (mistakes earlier
  agents made) before the first task of a session.
- Follow `AGENTS.md` section 3 for the workflow and section 4 for the rules. The constitution in
  `.specify/memory/constitution.md` overrides everything else.
