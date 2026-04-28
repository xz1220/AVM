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
~/.avm/runtime-homes/<active>/opencode/
  opencode.json
  agents/<agent>.md
  skills/<skill>/SKILL.md
```

Activation exports:

```bash
export OPENCODE_CONFIG=".../opencode.json"
export OPENCODE_CONFIG_DIR=".../opencode"
```

This avoids the earlier soft-link/overwrite class of problems and keeps the user's native OpenCode config untouched.

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
