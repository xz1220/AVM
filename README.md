# Agent VM

> Local control plane for portable AI coding agent profiles.

[![CI](https://github.com/xz1220/Agent-VM/actions/workflows/ci.yml/badge.svg)](https://github.com/xz1220/Agent-VM/actions/workflows/ci.yml)

Agent VM, or `avm`, lets you define an AI coding agent once, then project that
profile into different agent runtimes such as Codex, Claude Code, Cline, and
Cursor.

The goal is simple: stop treating agent setup as scattered dotfiles. Make the
agent itself a named, versionable, portable object.

## Why This Exists

Modern AI coding work is no longer tied to one assistant. A typical developer
may use Codex for repo edits, Claude Code for local workflows, Cursor in the
IDE, and Cline for task automation. Each runtime has its own prompt files, MCP
config, tool policy, model settings, and memory surface.

Agent VM gives those pieces a common local model:

- **Agent Profile**: role, runtime preference, model settings, permissions,
  capabilities, and memory refs.
- **Capability Registry**: reusable skills, commands, hooks, toolsets, and MCP
  server references.
- **Portable Memory**: auditable project knowledge and user preferences that can
  be attached to a profile without silently writing into runtime-native memory.
- **Runtime Adapters**: render plans that map a profile into each target runtime
  while reporting unsupported or degraded fields.

AVM is not a generic dotfiles sync tool. It is a local-first profile manager for
AI agents.

## Status

This repository is an early preview. The data model, CLI scaffold, config
commands, dry-run memory import, fixtures, and adapter contract are in place.

Working today:

- `avm init`
- `avm agent create/list/show`
- `avm env create`
- `avm memory import --from <file> --dry-run`
- config validation and resolution tests
- fake adapter and Phase 1 fixtures

In progress:

- `avm use <profile-or-env>`
- `avm status`
- `avm deactivate`
- concrete Codex and Claude Code adapter writes
- release packaging

## Quickstart

Prerequisites:

- Go 1.22+

Run from source:

```bash
git clone https://github.com/xz1220/Agent-VM.git
cd Agent-VM

go run ./cmd/avm --help
go run ./cmd/avm init
```

Create a profile:

```bash
go run ./cmd/avm agent create backend-coder \
  --runtime codex \
  --model gpt-5.4 \
  --reasoning high \
  --skills git,test \
  --mcps github \
  --memory backend-standards:project
```

Inspect it:

```bash
go run ./cmd/avm agent list
go run ./cmd/avm agent show backend-coder
```

Preview a portable memory import:

```bash
go run ./cmd/avm memory import \
  --from testdata/memory/backend-standards.md \
  --dry-run
```

Build locally:

```bash
make build
./bin/avm --help
```

## Example Profile

```yaml
name: backend-coder
version: 1.0.0
source_scope: global
runtime:
  preferred: codex
  kind: local
  mode: primary
model_run:
  model: gpt-5.4
  reasoning_effort: high
capabilities:
  skills:
    - git
    - test
  mcps:
    - github
permissions:
  approval: on-risky-actions
  sandbox: workspace-write
memory_refs:
  - id: backend-standards
    scope: project
    path: ~/.avm/memory/project/backend-standards.md
    mode: read
```

## Runtime Roadmap

| Runtime | Current state | Target |
| --- | --- | --- |
| Codex | Config model and fixtures | Active profile rendering |
| Claude Code | Mapping research and fixtures | Agent file and MCP rendering |
| Cline | Mapping research and fixtures | Rules and MCP rendering |
| Cursor | Partial PoC fixture | Rules and MCP rendering |

Adapters must report every mapping as `native`, `rendered_as_instructions`,
`ignored`, or `unsupported`. AVM should never pretend that all runtimes support
the same agent surface.

## Safety Model

AVM is designed to be conservative by default:

- `avm init` only writes under `~/.avm`.
- Runtime-native memory is imported only through explicit commands.
- Memory import supports dry-run reporting before writes.
- Adapters own explicit managed paths.
- Runtime fields that cannot be represented must be reported, not dropped.
- Secrets should be referenced through environment variables, not exported as
  plaintext profile data.

## Project Docs

- [Product requirements](docs/product/prd.md)
- [Technical design](docs/design/tech-design.md)
- [Architecture](docs/engineering/architecture.md)
- [Data model](docs/engineering/data-model.md)
- [Implementation plan](docs/engineering/implementation-plan.md)
- [Acceptance criteria](docs/engineering/acceptance.md)

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

- runtime mapping research for Codex, Claude Code, Cline, Cursor, and GitHub
  Copilot custom agents
- adapter fixtures
- CLI behavior tests
- docs that explain real workflows
- bug reports from people managing multiple AI coding tools

See [CONTRIBUTING.md](CONTRIBUTING.md).

## License

No open-source license has been selected yet. Until a license is added, the code
is source-available but not broadly reusable under an open-source license.
