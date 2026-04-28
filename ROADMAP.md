# Roadmap

Agent VM is being built in small vertical slices. The priority is to make each
slice honest, testable, and useful before broadening runtime support.

## Phase 1: Local Profile Activation

Goal: make `avm use <profile>` a trustworthy local workflow.

- [x] Go CLI scaffold
- [x] config model for Agent Profile, Environment, capabilities, and memory refs
- [x] `avm init`
- [x] `avm agent create/list/show`
- [x] `avm env create` (including `--local` project override)
- [x] `avm memory import --from <file> --dry-run`
- [x] adapter contract and fake adapter
- [x] active manifest rebuild under `~/.avm/active`
- [x] `avm use <profile-or-env>`
- [x] `avm status`
- [x] `avm deactivate`
- [x] backup and conflict detection for managed runtime paths
- [x] first concrete adapter write path
- [x] `avm sync`
- [x] `avm shell init` (bash/zsh/fish)
- [x] `avm agent show --runtime` mapping preview

## Phase 2: Runtime Coverage

Goal: support the runtimes that AI coding power users actually combine.

- [x] Codex profile rendering
- [x] Claude Code agent and MCP rendering
- [x] OpenCode config, agent, permission, MCP, and skill rendering
- [x] Cline rules and MCP rendering
- [x] Cursor PoC rendering
- [x] support matrix in `avm status`
- [x] export/import of portable profiles

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
