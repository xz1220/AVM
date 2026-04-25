# Stage 5 Acceptance Gap Report

> Date: 2026-04-25
> Base: `3f776d4`
> Scope: Phase 1 acceptance hardening

This report captures the current executable acceptance state before Stage 5
implementation work. It separates passing paths from known gaps so the next
tasks can target concrete behavior instead of re-reading the full acceptance
document.

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

## Verified Gaps

These are directly verified gaps against `docs/engineering/acceptance.md`:

| Area | Current behavior | Acceptance target |
|------|------------------|-------------------|
| `avm init` idempotency | Second `avm init` exits 0 and rewrites defaults | Existing config should fail unless `--force` is passed |
| `avm init` state files | Initial `init` does not create `state/sync-state.json` | `state/sync-state.json` exists after init |
| `avm init` cache dir | Initial `init` does not create `~/.avm/cache` | `cache/` exists after init |
| Runtime import during init | Runtime import/report is not implemented | Read-only scan of runtime configs with `state/import-report.json` |
| `avm env create --local` | Returns `avm env create --local is not supported yet` | Writes project `.avm/env.yaml` override |
| Env reference validation | `env create` can write mappings to missing profiles | Missing referenced profiles should fail |
| `avm shell init` | Returns `avm shell init: not implemented` | Prints eval-safe shell hook |
| `avm sync` | Command is absent | Re-sync current active without changing selection |
| `avm export` / `avm import` | Commands are absent | Portable package export/import |
| `avm agent show --runtime` | Shows YAML, not mapping preview | Shows native/rendered/unsupported mapping preview |
| Cursor status | Runtime status is `synced` with partial warnings | Acceptance text says Cursor should display `partial` |

## Stage 5 Work Slices

Recommended next work slices:

1. Acceptance harness: convert the passing smoke and selected negative cases into automated tests.
2. Init/shell/sync CLI gaps: implement `init --force`, initial state/cache artifacts, shell hook output, and `avm sync`.
3. Env hardening: implement project-local env overrides and validate referenced agent profiles.
4. Export/import packaging: implement portable `.avm.zip` export/import for agents, envs, referenced capabilities, and memory refs.
5. Mapping preview/status polish: add `agent show --runtime` preview and decide whether Cursor should surface as `partial` status or remain `synced` with explicit warnings.

## Automated Coverage Added

The Stage 5 acceptance harness now adds Go tests for the passing smoke flow and
selected negative gap tracking:

- `cmd/avm/stage5_acceptance_test.go` runs `init`, `agent`, `env`, `memory
  import --dry-run`, `use`, `status`, and `deactivate` under temporary `HOME`,
  project, `CODEX_HOME`, `CLAUDE_CONFIG_DIR`, and `CLINE_DATA_HOME` paths.
- The harness asserts `init` does not create project `.avm` or adapter managed
  runtime files, and that those managed paths appear only after `use`.
- The harness asserts `codex`, `claude-code`, `cline`, and `cursor` all reach
  `synced` entries in `state/sync-state.json` for the multi-runtime env.
- The harness asserts `memory import --dry-run` does not add formal files under
  `~/.avm/memory/**`.
- The harness tracks current negative behavior for `env create --local`, `shell
  init`, and absent top-level `sync`, `export`, and `import` commands.

## Non-goals Still In Force

- No `sync --watch` in Phase 1.
- No `workspace_isolation` field on `AgentProfile`.
- No silent writes to runtime-native memory.
- No default overwrite of user-owned instruction files such as `AGENTS.md`, `CLAUDE.md`, and `.cursorrules`.
