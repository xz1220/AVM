# Roadmap

Agent VM is being built in small vertical slices. The priority is to make each
slice honest, testable, and useful before broadening runtime support.

## Phase 1: Local Profile Activation

Goal: make `avm use <profile>` a trustworthy local workflow.

- [x] Go CLI scaffold
- [x] config model for Agent Profile, Environment, capabilities, and memory refs
- [x] `avm init`
- [x] `avm agent create/list/show`
- [x] `avm env create`
- [x] `avm memory import --from <file> --dry-run`
- [x] adapter contract and fake adapter
- [ ] active manifest rebuild under `~/.avm/active`
- [ ] `avm use <profile-or-env>`
- [ ] `avm status`
- [ ] `avm deactivate`
- [ ] backup and conflict detection for managed runtime paths
- [ ] first concrete adapter write path

## Phase 2: Runtime Coverage

Goal: support the runtimes that AI coding power users actually combine.

- [ ] Codex profile rendering
- [ ] Claude Code agent and MCP rendering
- [ ] Cline rules and MCP rendering
- [ ] Cursor PoC rendering
- [ ] support matrix in `avm status`
- [ ] export/import of portable profiles

## Phase 3: Portable Memory

Goal: make long-lived agent knowledge auditable and portable.

- [ ] explicit memory export
- [ ] memory diff before runtime writes
- [ ] scoped user, project, and team memory
- [ ] conflict handling for native runtime memory
- [ ] portable memory bundles for teams

## Phase 4: Team Registry

Goal: let teams share agent profiles without sharing secrets or unsafe local
state.

- [ ] signed profile bundles
- [ ] team registry layout
- [ ] policy checks
- [ ] profile review workflow
- [ ] release packaging
