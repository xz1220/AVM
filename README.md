<p align="center">
  <img src="assets/avm-hero.svg" alt="Agent VM: one profile, every coding agent runtime" width="100%">
</p>

<h1 align="center">Agent VM</h1>

<p align="center">
  <strong>nvm for AI coding agents.</strong>
  <br>
  One portable profile for tools, permissions, model settings, and memory refs.
</p>

<p align="center">
  <a href="https://github.com/xz1220/Agent-VM/actions/workflows/ci.yml"><img src="https://github.com/xz1220/Agent-VM/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <img src="https://img.shields.io/badge/status-early_preview-0f766e" alt="Status: early preview">
  <img src="https://img.shields.io/badge/runtime-Codex%20%7C%20Claude%20Code%20%7C%20OpenCode-1d4ed8" alt="Supported runtime targets">
  <img src="https://img.shields.io/badge/language-Go-00ADD8" alt="Go">
</p>

<p align="center">
  English | <a href="README.zh-CN.md">简体中文</a>
</p>

Agent VM, or `avm`, is a local control plane for AI coding agent profiles. It
keeps an agent's role, tools, permissions, model preferences, and memory refs in
one portable profile, then lets adapters render that profile into runtimes such
as Codex, Claude Code, and OpenCode. Cline and Cursor remain available as
compatibility adapters; Cursor support is a conservative Phase 1 rules/MCP PoC.

<p align="center">
  <img src="assets/avm-before-after.svg" alt="Before AVM config is scattered; after AVM one profile activates an agent" width="100%">
</p>

## The Move

```bash
avm create backend-coder
eval "$(avm activate backend-coder)"
```

Create an agent from a package, activate it in the current shell, then start the
runtime. Instead of rebuilding the same role across prompt files, MCP config,
rules directories, and memory notes, AVM makes the agent profile the source of
truth. `avm use` remains available for explicit profile/env activation and sync.

```text
backend-coder package
  -> avm create backend-coder
    -> backend-coder.yaml
      -> eval "$(avm activate backend-coder)"
        -> Codex profile
        -> Claude Code agent
        -> OpenCode config/agent/skills
        -> Cline rules/MCP settings
        -> Cursor rules/MCP PoC
```

## Why This Is Different

| Approach | What it manages | What it misses |
| --- | --- | --- |
| Dotfiles | Files and symlinks | No agent object, no mapping status |
| MCP config managers | Tool server config | Usually no role, memory, model, or permission model |
| Runtime-native profiles | One ecosystem | Hard to carry across tools |
| Agent VM | Agent Profile + capabilities + memory refs + adapters | Early; Phase 1 adapters are conservative and report mapping status |

AVM is not trying to flatten every runtime into the same interface. Each adapter
must report how fields map: `native`, `rendered_as_instructions`, `ignored`, or
`unsupported`.

## What A Profile Carries

| Layer | Example |
| --- | --- |
| Identity | `backend-coder`, `pr-reviewer`, `incident-runner` |
| Runtime | `codex`, `claude-code`, `opencode`, `cline`, `cursor` |
| Model run | model name, reasoning effort, verbosity |
| Capabilities | skills, commands, hooks, MCP servers, toolsets |
| Permissions | approval mode, sandbox intent, allow/deny policy |
| Memory refs | project architecture, team conventions, user preferences |

## Recipes

<details open>
<summary><strong>backend-coder</strong></summary>

```yaml
name: backend-coder
runtime:
  preferred: codex
model_run:
  model: gpt-5.4
  reasoning_effort: high
capabilities:
  skills: [git, test, migration]
  mcps: [github, postgres-readonly]
permissions:
  approval: on-risky-actions
  sandbox: workspace-write
memory_refs:
  - id: backend-standards
    scope: project
    mode: read
```

</details>

<details>
<summary><strong>pr-reviewer</strong></summary>

```yaml
name: pr-reviewer
runtime:
  preferred: claude-code
capabilities:
  skills: [review, security, test-analysis]
  mcps: [github]
permissions:
  approval: never
  sandbox: read-only
memory_refs:
  - id: review-policy
    scope: team
    mode: read
```

</details>

<details>
<summary><strong>incident-runner</strong></summary>

```yaml
name: incident-runner
runtime:
  preferred: codex
capabilities:
  skills: [diagnose, summarize, runbook]
  mcps: [logs-readonly, github]
permissions:
  approval: prompt
  sandbox: read-only
memory_refs:
  - id: incident-runbooks
    scope: team
    mode: read
```

</details>

## Status

This repository is an early preview. The core model, Stage 5 CLI hardening, and
managed activation path are in place.

Working today:

- `avm init`
- `avm create <package>` for first-run profile creation
- `avm package list/show` for built-in create packages
- `avm agent create/list/show`, including `avm agent show --runtime <runtime>`
- `avm env create`, including `avm env create --local`
- `avm memory import --from <file> --dry-run`
- `avm use`, `avm status`, and `avm deactivate`
- `avm sync`
- `avm shell init bash|zsh|fish`
- `avm export`, `avm import`, and `avm install <file.avm.zip>`
- `avm init` runtime import/report scan with `state/import-report.json`
- managed Codex, Claude Code, OpenCode, Cline, and Cursor render outputs
- config validation and resolution tests
- adapter contract, fake adapter, and Phase 1 fixtures

Cursor Phase 1 writes successfully as `synced`; partial support is exposed
through warnings and mapping status, not a separate Cursor-only sync state.

Still post-MVP or policy follow-up:

- broader package policy for config/defaults/active state, project overrides,
  runtime outputs, and interactive overwrite/rename

## Quickstart

Install a tagged preview release:

```bash
curl -fsSL https://raw.githubusercontent.com/xz1220/Agent-VM/main/scripts/install.sh | sh
```

Create your first agent from a package:

```bash
avm create backend-coder
```

Use it in the current shell, then start your runtime:

```bash
eval "$(avm activate backend-coder)"
codex
```

Use OpenCode instead by selecting it during creation or passing a flag:

```bash
avm create backend-coder --runtime opencode
eval "$(avm activate backend-coder)"
opencode
```

Inspect available packages:

```bash
avm package list
avm package show reviewer
```

Run from source for development or local testing:

```bash
git clone https://github.com/xz1220/Agent-VM.git
cd Agent-VM
go run ./cmd/avm create backend-coder --yes
```

For a local smoke run without installed runtime CLIs, create the runtime config
directories before activation:

```bash
mkdir -p "$HOME/.codex" "$HOME/.claude" "$HOME/.config/opencode" "$HOME/.cline/data" .cursor
```

Create and inspect profiles:

```bash
avm agent create backend-coder \
  --runtime codex \
  --model gpt-5.4 \
  --reasoning high \
  --skills git,test \
  --mcps github \
  --memory backend-standards:project

avm agent create reviewer --runtime claude-code --skills review
avm agent create opencode-coder --runtime opencode --skills git,test
avm agent create cline-helper --runtime cline --skills test
avm agent create cursor-helper --runtime cursor --skills rules

avm agent list
avm agent show backend-coder
```

Create environments:

```bash
avm env create all-runtimes \
  --codex backend-coder \
  --claude-code reviewer \
  --opencode opencode-coder \
  --cline cline-helper \
  --cursor cursor-helper

avm env create all-runtimes --local --codex backend-coder
```

Preview a portable memory import:

```bash
avm memory import \
  --from testdata/memory/backend-standards.md \
  --dry-run \
  --format json
```

Activate, inspect, resync, and deactivate:

```bash
avm use --kind env all-runtimes
avm status
avm sync
avm deactivate
```

Shell prompt integration prints eval-safe snippets:

```bash
eval "$(avm shell init zsh)"
```

Export and install packages:

```bash
avm export backend-coder --kind agent --output backend-coder.avm.zip
avm install backend-coder.avm.zip
```

Build locally:

```bash
make build
./bin/avm --help
```

## Current Status Shape

The current activation loop is:

```bash
avm create backend-coder
eval "$(avm activate backend-coder)"
avm status
```

Expected status shape:

```text
active: profile:backend-coder
runtime status:
  codex: synced (agent backend-coder)
managed paths:
  codex:
    - ~/.avm/runtime-homes/profile-backend-coder/codex/config.toml owner=avm merge=whole-file
    - ~/.avm/runtime-homes/profile-backend-coder/codex/agents/backend-coder.toml owner=avm merge=whole-file
mapping status:
  codex:
    - capabilities.skills -> .../agents/backend-coder.toml#developer_instructions: rendered_as_instructions
warnings:
  none
```

## Safety Model

AVM is designed to be conservative by default:

- installer initialization and `avm init` only write under `~/.avm`.
- Runtime-native memory is imported only through explicit commands.
- Memory import supports dry-run reporting before writes.
- Adapters own explicit managed paths.
- Runtime fields that cannot be represented must be reported, not dropped.
- Secrets should be referenced through environment variables, not exported as
  plaintext profile data.

## Roadmap

| Phase | Theme | Headline |
| --- | --- | --- |
| 1 | Local profile activation | `avm use <profile>` |
| 2 | Runtime coverage | Codex, Claude Code, OpenCode, plus Cline/Cursor compatibility adapters |
| 3 | Portable memory | explicit import/export/diff/push/pull |
| 4 | Team registry | shareable agent profiles with policy and audit |

See [ROADMAP.md](ROADMAP.md).

## Project Docs

- [Design system](DESIGN.md)
- [Product requirements](docs/product/prd.md)
- [Technical design](docs/design/tech-design.md)
- [Architecture](docs/engineering/architecture.md)
- [Data model](docs/engineering/data-model.md)
- [Implementation plan](docs/engineering/implementation-plan.md)
- [Acceptance criteria](docs/engineering/acceptance.md)
- [GitHub launch checklist](docs/marketing/github-launch-checklist.md)

## Development

```bash
make test
make vet
make fmt
make build
```

The main package is `cmd/avm`. Core packages live under `internal/config`,
`internal/adapter`, `internal/memory`, `internal/sync`, and `internal/state`.

## Contributing

AVM is early. The most useful contributions right now are narrow and concrete:

- runtime mapping research for Codex, Claude Code, OpenCode, Cline, Cursor, and
  GitHub Copilot custom agents
- adapter fixtures
- CLI behavior tests
- docs that explain real workflows
- bug reports from people managing multiple AI coding tools

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

No open-source license has been selected yet. Until a license is added, the code
is source-available but not broadly reusable under an open-source license.
