<p align="center">
  <img src="assets/avm-hero.svg" alt="Agent VM: one profile, every coding agent runtime" width="100%">
</p>

<h1 align="center">Agent VM</h1>

<p align="center">
  <strong>AI 코딩 에이전트를 위한 nvm.</strong>
  <br>
  도구, 권한, 모델 설정, memory refs를 하나의 portable profile로 관리합니다.
</p>

<p align="center">
  <a href="https://github.com/xz1220/Agent-VM/actions/workflows/ci.yml"><img src="https://github.com/xz1220/Agent-VM/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <img src="https://img.shields.io/badge/status-early_preview-0f766e" alt="Status: early preview">
  <img src="https://img.shields.io/badge/runtime-Codex%20%7C%20Claude%20Code%20%7C%20Cline%20%7C%20Cursor-1d4ed8" alt="Runtime targets">
  <img src="https://img.shields.io/badge/language-Go-00ADD8" alt="Go">
</p>

<p align="center">
  <a href="README.md">English</a> | <a href="README.zh-CN.md">简体中文</a> | <a href="README.ja.md">日本語</a> | 한국어 | <a href="README.es.md">Español</a> | <a href="README.pt-BR.md">Português</a> | <a href="README.fr.md">Français</a>
</p>

Agent VM, 줄여서 `avm`, 은 AI coding agent profile을 위한 로컬 control plane입니다. Agent를 한 번 정의하고 그 profile을 Codex, Claude Code, Cline, Cursor 같은 runtime으로 렌더링합니다.

핵심 가정은 명확합니다. 개발자는 하나의 coding agent로 수렴하지 않습니다. 필요한 것은 agent의 정체성, 도구, 모델 설정, 권한, 장기 memory를 표현하는 portable object입니다.

<p align="center">
  <img src="assets/avm-before-after.svg" alt="Before AVM config is scattered; after AVM one profile activates an agent" width="100%">
</p>

## Core Move

```bash
avm use backend-coder
```

이 명령은 로컬 AI coding 환경을 전환하는 기본 동작이 되어야 합니다. prompt file, MCP config, rules directory, memory notes에 같은 역할을 반복해서 만들지 않고, AVM은 Agent Profile을 source of truth로 둡니다.

```text
backend-coder.yaml
  -> avm use backend-coder
    -> Codex profile
    -> Claude Code agent
    -> Cline rules
    -> Cursor rules
```

## 차이점

| Approach | 관리 대상 | 부족한 점 |
| --- | --- | --- |
| Dotfiles | 파일과 symlink | Agent object와 mapping status가 없음 |
| MCP config managers | tool server config | role, memory, model, permission model이 약함 |
| Runtime-native profiles | 하나의 ecosystem | 다른 runtime으로 옮기기 어려움 |
| Agent VM | Agent Profile + capabilities + memory refs + adapters | 초기 단계이며 concrete adapters를 구축 중 |

각 adapter는 field mapping을 `native`, `rendered_as_instructions`, `ignored`, `unsupported`로 보고해야 합니다.

## Profile 내용

| Layer | Example |
| --- | --- |
| Identity | `backend-coder`, `pr-reviewer`, `incident-runner` |
| Runtime | `codex`, `claude-code`, `cline`, `cursor` |
| Model run | model name, reasoning effort, verbosity |
| Capabilities | skills, commands, hooks, MCP servers, toolsets |
| Permissions | approval mode, sandbox intent, allow/deny policy |
| Memory refs | project architecture, team conventions, user preferences |

## Recipe

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

이 repository는 early preview입니다. core model과 첫 CLI slice는 준비되어 있으며, 다음 주요 milestone은 profile activation입니다.

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

## Target CLI Experience

```bash
avm init
avm agent create backend-coder --runtime codex --skills git,test
avm use backend-coder
avm status
```

## Safety Model

- `avm init`은 `~/.avm` 아래만 씁니다.
- runtime-native memory import는 명시적 command로만 수행합니다.
- memory import는 dry-run report를 지원합니다.
- adapter는 선언된 managed paths만 소유합니다.
- 표현할 수 없는 runtime field는 조용히 버리지 않고 보고합니다.
- secrets는 plaintext export 대신 environment variables로 참조합니다.

## Docs

- [Design system](DESIGN.md)
- [Product requirements](docs/product/prd.md)
- [Technical design](docs/design/tech-design.md)
- [Roadmap](ROADMAP.md)

## License

No open-source license has been selected yet. Until a license is added, the code is source-available but not broadly reusable under an open-source license.
