<p align="center">
  <img src="assets/avm-hero.svg" alt="Agent VM: one profile, every coding agent runtime" width="100%">
</p>

<h1 align="center">Agent VM</h1>

<p align="center">
  <strong>AI coding agent のための nvm。</strong>
  <br>
  ツール、権限、モデル設定、memory refs を 1 つの portable profile で管理します。
</p>

<p align="center">
  <a href="https://github.com/xz1220/Agent-VM/actions/workflows/ci.yml"><img src="https://github.com/xz1220/Agent-VM/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <img src="https://img.shields.io/badge/status-early_preview-0f766e" alt="Status: early preview">
  <img src="https://img.shields.io/badge/runtime-Codex%20%7C%20Claude%20Code%20%7C%20OpenClaw%20%7C%20Hermes%20Agent-1d4ed8" alt="Runtime targets">
  <img src="https://img.shields.io/badge/language-Go-00ADD8" alt="Go">
</p>

<p align="center">
  <a href="README.md">English</a> | <a href="README.zh-CN.md">简体中文</a> | 日本語 | <a href="README.ko.md">한국어</a> | <a href="README.es.md">Español</a> | <a href="README.pt-BR.md">Português</a> | <a href="README.fr.md">Français</a>
</p>

Agent VM、または `avm` は、AI coding agent profile のためのローカル control plane です。Agent の role、tools、permissions、model preferences、memory refs を 1 つの portable profile にまとめ、Codex、Claude Code、OpenClaw、Hermes Agent などの runtime に adapter でレンダリングします。

<p align="center">
  <img src="assets/avm-before-after.svg" alt="Before AVM config is scattered; after AVM one profile activates an agent" width="100%">
</p>

## Core Move

```bash
avm use backend-coder
```

このコマンドを、ローカル AI coding 環境を切り替えるための基本動作にしたいと考えています。prompt file、MCP config、rules directory、memory notes に同じ役割を何度も作る代わりに、AVM は Agent Profile を source of truth にします。

```text
backend-coder.yaml
  -> avm use backend-coder
    -> Codex profile
    -> Claude Code agent
    -> OpenClaw workspace
    -> Hermes Agent profile
```

## 違い

| Approach | 管理するもの | 足りないもの |
| --- | --- | --- |
| Dotfiles | ファイルと symlink | Agent object と mapping status がない |
| MCP config managers | tool server config | role、memory、model、permission model が弱い |
| Runtime-native profiles | 1 つの ecosystem | 他の runtime に持ち運びにくい |
| Agent VM | Agent Profile + capabilities + memory refs + adapters | まだ初期段階で concrete adapters を構築中 |

AVM はすべての runtime を無理に同じ interface にしません。各 adapter は field の mapping を `native`、`rendered_as_instructions`、`ignored`、`unsupported` として報告します。

## Profile が持つもの

| Layer | Example |
| --- | --- |
| Identity | `backend-coder`, `pr-reviewer`, `incident-runner` |
| Runtime | `codex`, `claude-code`, `openclaw`, `hermes-agent` |
| Model run | model name, reasoning effort, verbosity |
| Capabilities | skills, commands, hooks, MCP servers, toolsets |
| Permissions | approval mode, sandbox intent, allow/deny policy |
| Memory refs | project architecture, team conventions, user preferences |

## Recipes

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

## Status

この repository は early preview です。core model と最初の CLI slice は入っています。次の大きな milestone は profile activation です。

Working today:

- `avm init`
- `avm agent create/list/show`
- `avm env create`
- `avm memory import --from <file> --dry-run`
- config validation and resolution tests
- adapter contract, fake adapter, and Phase 1 fixtures

In progress:

- `avm use <profile-or-env>`
- `avm status`
- `avm deactivate`
- concrete Codex and Claude Code adapter writes
- release packaging

## Quickstart

Prerequisites:

- Go 1.22+

```bash
git clone https://github.com/xz1220/Agent-VM.git
cd Agent-VM

go run ./cmd/avm --help
go run ./cmd/avm init
```

```bash
go run ./cmd/avm agent create backend-coder \
  --runtime codex \
  --model gpt-5.4 \
  --reasoning high \
  --skills git,test \
  --mcps github \
  --memory backend-standards:project
```

```bash
go run ./cmd/avm agent list
go run ./cmd/avm agent show backend-coder
```

```bash
go run ./cmd/avm memory import \
  --from testdata/memory/backend-standards.md \
  --dry-run
```

## Target CLI Experience

```bash
avm init
avm agent create backend-coder --runtime codex --skills git,test
avm use backend-coder
avm status
```

```text
active   profile:backend-coder
runtime  codex          native: model, permissions
runtime  claude-code    rendered: skills, memory_refs
runtime  openclaw       rendered: workspace, memory_refs
```

## Safety Model

- `avm init` は `~/.avm` だけを書き込みます。
- runtime-native memory の import は明示的な command のみで行います。
- memory import は書き込み前の dry-run report をサポートします。
- adapter は宣言した managed paths だけを所有します。
- 表現できない runtime field は silent drop せず報告します。
- secrets は plaintext export ではなく environment variables で参照します。

## Docs

- [Design system](DESIGN.md)
- [Product requirements](docs/product/prd.md)
- [Technical design](docs/design/tech-design.md)
- [Roadmap](ROADMAP.md)

## License

No open-source license has been selected yet. Until a license is added, the code is source-available but not broadly reusable under an open-source license.
