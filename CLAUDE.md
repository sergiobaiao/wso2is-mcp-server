# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

@AGENTS.md

## Claude Code specifics

- Spec Kit skills are installed in `.claude/skills/speckit-*`. Invoke them as `/speckit-implement`,
  `/speckit-converge`, `/speckit-analyze` and so on. The Spec Kit scripts run with
  `python3 .specify/scripts/python/<script>.py`.
- The active feature is `specs/001-wso2is-full-coverage`. Spec Kit reads it from
  `.specify/feature.json`, which is not committed; create it as described in `AGENTS.md`
  section 3.
- Follow `AGENTS.md` section 3 for the workflow and section 4 for the rules. The constitution in
  `.specify/memory/constitution.md` overrides everything else.
