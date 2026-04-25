# Stage 5 Acceptance Status Report

> Date: 2026-04-26
> Base: `main` after Stage 5 integration
> Scope: Phase 1 acceptance hardening

This report captures the executable acceptance state after the Stage 5
hardening branches were integrated. It keeps the passing smoke path, records
which Stage 5 gaps were closed, and leaves only post-MVP or still-undecided
items as follow-up work.

## Passing Smoke

The following smoke path passes in a temporary `HOME`, project directory,
`CODEX_HOME`, `CLAUDE_CONFIG_DIR`, and `CLINE_DATA_HOME`:

```text
avm init
avm agent create codex-agent --runtime codex --model gpt-5.4 --reasoning medium --skills test --mcps github --memory standards:project:/memory/standards.md:read
avm agent create claude-agent --runtime claude-code --model claude-sonnet --reasoning medium --skills test
avm agent create cline-agent --runtime cline --model cline-model --reasoning medium --skills test
avm agent create cursor-agent --runtime cursor --model cursor-model --reasoning medium --skills test
avm agent list
avm agent show codex-agent
avm env create all-runtimes --codex codex-agent --claude-code claude-agent --cline cline-agent --cursor cursor-agent
avm memory import --from <file> --dry-run --format json
avm use --kind env all-runtimes
avm status
avm deactivate
```

Observed pass criteria:

- `~/.avm/config.yaml`, default agent, default env, custom agents, custom env, active manifest, and sync state are created by the full flow.
- `memory import --dry-run` does not write formal `~/.avm/memory/**` files.
- `use --kind env all-runtimes` reports `codex`, `claude-code`, `cline`, and `cursor` as synced.
- Runtime managed outputs are written only after `use`:
  - Codex: `config.toml`, `agents/<agent>.toml`
  - Claude Code: `.claude/agents/<agent>.md`
  - Cline: `.clinerules/avm/<agent>.md`
  - Cursor: `.cursor/rules/avm-<agent>.md`
- Cursor partial behavior is visible through warnings and mapping status.
- `status` shows active env, runtime states, managed paths, and mapping status.

## Closed Stage 5 Gaps

These gaps were directly verified before Stage 5 and are now implemented or
covered by tests:

| Area | Previous behavior | Current behavior |
|------|------------------|-------------------|
| `avm init` idempotency | Second `avm init` rewrote defaults | Existing config fails unless `--force` is passed |
| `avm init` state files | Initial `init` did not create `state/sync-state.json` | `state/sync-state.json` exists after init |
| `avm init` cache dir | Initial `init` did not create `~/.avm/cache` | `cache/` exists after init |
| `avm env create --local` | Returned not implemented | Writes project `.avm/env.yaml` override |
| Env reference validation | Missing profile references could be written | Missing referenced profiles fail before write |
| `avm shell init` | Returned not implemented | Prints eval-safe bash/zsh/fish shell hooks |
| `avm sync` | Command was absent | Re-syncs current active without changing selection |
| `avm export` / `avm import` | Commands were absent | Portable `.avm.zip` export/import for Phase 1 agents/envs |

## Remaining Follow-up

These are still outside the closed Stage 5 scope:

| Area | Current behavior | Follow-up target |
|------|------------------|------------------|
| Runtime import during init | Runtime import/report is not implemented | Read-only scan of runtime configs with `state/import-report.json` |
| `avm agent show --runtime` | Shows YAML, not mapping preview | Show native/rendered/unsupported mapping preview |
| Cursor status | Runtime status is `synced` with warnings and mapping status | Keep `synced` for successful writes; expose partial support through warnings and mapping status |
| Package scope | Export/import excludes config/global defaults/active, project env overrides, state/backup/cache, runtime outputs, runtime-native memory, and interactive overwrite/rename | Decide which belong in post-MVP packaging |

## Stage 5 Integration Result

Integrated branches:

- `origin/feat/acceptance-harness`
- `origin/feat/cli-hardening`
- `origin/feat/env-hardening`
- `origin/feat/package-io`

## Automated Coverage Added

The Stage 5 acceptance harness now adds Go tests for the passing smoke flow and
the hardened CLI behavior:

- `cmd/avm/stage5_acceptance_test.go` runs `init`, `agent`, `env`, `memory
  import --dry-run`, `use`, `status`, and `deactivate` under temporary `HOME`,
  project, `CODEX_HOME`, `CLAUDE_CONFIG_DIR`, and `CLINE_DATA_HOME` paths.
- The harness asserts `init` does not create project `.avm` or adapter managed
  runtime files, and that those managed paths appear only after `use`.
- The harness asserts `codex`, `claude-code`, `cline`, and `cursor` all reach
  `synced` entries in `state/sync-state.json` for the multi-runtime env.
- The harness asserts `memory import --dry-run` does not add formal files under
  `~/.avm/memory/**`.
- The harness covers `init --force`, initial cache/sync-state artifacts, project
  local env overrides, missing profile validation, shell hook output, `sync`,
  and package `export`/`import`.

## Non-goals Still In Force

- No `sync --watch` in Phase 1.
- No `workspace_isolation` field on `AgentProfile`.
- No silent writes to runtime-native memory.
- No default overwrite of user-owned instruction files such as `AGENTS.md`, `CLAUDE.md`, and `.cursorrules`.
