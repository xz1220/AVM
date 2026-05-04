# AVM Runtime Experience Iteration

> Date: 2026-04-29
> Branch: `proposal/install-onboarding-path`
> Goal: keep the first-run path simple while updating runtime coverage around the agent CLIs users are likely to try in 2026.

## User Path

The primary path stays package-first:

```bash
avm create backend-coder
eval "$(avm activate backend-coder)"
codex
```

For OpenCode:

```bash
avm create backend-coder --runtime opencode
eval "$(avm activate backend-coder)"
opencode
```

The user should not need to understand templates, symlinks, or runtime config file layouts before they can start.

## Create Experience Loop

The next friction point is not installation; it is turning a package or existing
AVM profile into a focused profile.

Implemented in this iteration:

- `avm create --from <profile>` copies an existing AVM profile, so users can start from `default` without editing YAML by hand.
- Interactive `avm create` now shows package and profile sources.
- Interactive `avm create` uses a terminal wizard with arrow-key navigation and Space-based multi-select for runtimes, installed skills, and MCP servers from the local registry.
- `avm skill list` gives users a standalone inventory with skill summaries before activation; inside an activated shell it defaults to the active profile's selected skills, with `--all` for the global registry.

Example:

```bash
avm skill list
avm create --from default --name api-coder
avm create backend-coder --runtimes codex,opencode
```

This keeps package-first onboarding while avoiding ambiguous conversion from
runtime-native subagents into AVM Agents.

## Runtime Priorities

Recommended first-class runtimes:

- `codex`: strong native profile/config mapping and existing AVM support.
- `claude-code`: strong native agent/skill/MCP mapping and existing AVM support.
- `opencode`: current open CLI target with documented env-var config isolation, agent files, permissions, skills, and MCP.

Compatibility runtimes:

- `cline`: keep support for existing users, but do not center the onboarding path around it.
- `cursor`: keep as rules/MCP PoC; continue reporting partial mapping status.

Not added in this iteration:

- `opencloud`: no stable public CLI/config surface was found under that name. The current implementable target is OpenCode, backed by official docs.
- Runtimes without stable local config, env-var isolation, or documented agent/capability model.

## OpenCode Mapping

OpenCode official docs define:

- `OPENCODE_CONFIG` for a custom config file.
- `OPENCODE_CONFIG_DIR` for custom agent/command/mode/plugin directories.
- `agents/<name>.md` for Markdown agents.
- `skills/<name>/SKILL.md` for agent skills.
- `permission` for native allow/ask/deny rules.
- `mcp` for local and remote MCP servers.

AVM renders OpenCode into an isolated runtime home:

```text
~/.avm/runtime-homes/agents/<agent-id>/opencode/
  config/opencode.json
  config/agents/<agent>.md
  config/skills/<skill>/SKILL.md
  data/opencode.db
  xdg-data/
  xdg-state/
  xdg-cache/
```

Activation exports:

```bash
export OPENCODE_CONFIG=".../opencode/config/opencode.json"
export OPENCODE_CONFIG_DIR=".../opencode/config"
```

For full data/state isolation, start OpenCode through `avm run opencode`; it
injects `OPENCODE_DB` and `XDG_*` only into that process.

## Support Rules

Support:

- Package creation with `--runtime opencode`.
- Environment mapping with `avm env create --opencode <agent>`.
- Shell-local activation exports for OpenCode.
- Native OpenCode `default_agent`, agent Markdown, permissions, MCP, and skills.
- Explicit mapping status for fields OpenCode cannot represent directly.

Do not support:

- Writing user-owned `~/.config/opencode` directly during activation.
- Overwriting project `.opencode/` or `AGENTS.md`.
- Installing package scripts or executing package hooks.
- Treating unresolved skill names as installed native OpenCode skills.
- Hiding partial mappings; unresolved fields must show in `avm status`.
