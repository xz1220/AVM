<p align="center">
  <img src="assets/avm-hero.svg" alt="Agent VM: one profile, every coding agent runtime" width="100%">
</p>

<h1 align="center">Agent VM</h1>

<p align="center">
  <strong>nvm for AI coding agents.</strong>
  <br>
  Un perfil portable para herramientas, permisos, modelos y referencias de memoria.
</p>

<p align="center">
  <a href="https://github.com/xz1220/Agent-VM/actions/workflows/ci.yml"><img src="https://github.com/xz1220/Agent-VM/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <img src="https://img.shields.io/badge/status-early_preview-0f766e" alt="Status: early preview">
  <img src="https://img.shields.io/badge/runtime-Codex%20%7C%20Claude%20Code%20%7C%20OpenClaw%20%7C%20Hermes%20Agent-1d4ed8" alt="Runtime targets">
  <img src="https://img.shields.io/badge/language-Go-00ADD8" alt="Go">
</p>

<p align="center">
  <a href="README.md">English</a> | <a href="README.zh-CN.md">简体中文</a> | <a href="README.ja.md">日本語</a> | <a href="README.ko.md">한국어</a> | Español | <a href="README.pt-BR.md">Português</a> | <a href="README.fr.md">Français</a>
</p>

Agent VM, o `avm`, es un control plane local para perfiles de agentes de IA de programación. Mantiene el rol, las herramientas, los permisos, las preferencias de modelo y las memory refs en un perfil portable, y luego lo renderiza hacia runtimes como Codex, Claude Code, OpenClaw y Hermes Agent.

<p align="center">
  <img src="assets/avm-before-after.svg" alt="Before AVM config is scattered; after AVM one profile activates an agent" width="100%">
</p>

## El movimiento

```bash
avm use backend-coder
```

Ese comando debería convertirse en el hábito para cambiar tu entorno local de AI coding. En vez de reconstruir el mismo rol en prompt files, MCP config, rules directories y notas de memoria, AVM convierte el Agent Profile en la source of truth.

```text
backend-coder.yaml
  -> avm use backend-coder
    -> Codex profile
    -> Claude Code agent
    -> OpenClaw workspace
    -> Hermes Agent profile
```

## Diferencias

| Enfoque | Qué gestiona | Qué falta |
| --- | --- | --- |
| Dotfiles | Archivos y symlinks | Sin objeto Agent ni mapping status |
| MCP config managers | Configuración de tool servers | Normalmente sin role, memory, model ni permissions |
| Runtime-native profiles | Un solo ecosistema | Difícil de portar a otros runtimes |
| Agent VM | Agent Profile + capabilities + memory refs + adapters | Proyecto temprano; concrete adapters en construcción |

Cada adapter debe reportar cómo se mapea cada campo: `native`, `rendered_as_instructions`, `ignored` o `unsupported`.

## Qué lleva un Profile

| Capa | Ejemplo |
| --- | --- |
| Identity | `backend-coder`, `pr-reviewer`, `incident-runner` |
| Runtime | `codex`, `claude-code`, `openclaw`, `hermes-agent` |
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

## Estado

Este repositorio es una preview temprana. El modelo central y los primeros comandos de CLI ya existen; el próximo hito importante es profile activation.

Funciona hoy:

- `avm init`
- `avm agent create/list/show`
- `avm env create`
- `avm memory import --from <file> --dry-run`
- config validation and resolution tests
- adapter contract, fake adapter, and Phase 1 fixtures

En progreso:

- `avm use <profile-or-env>`
- `avm status`
- `avm deactivate`
- concrete Codex and Claude Code adapter writes
- release packaging

## Quickstart

Prerequisitos:

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

- `avm init` solo escribe dentro de `~/.avm`.
- La memoria nativa de runtime solo se importa con comandos explícitos.
- `memory import` soporta dry-run antes de escribir.
- Los adapters solo poseen managed paths declarados.
- Los campos no soportados se reportan; no se descartan en silencio.
- Los secrets deben referenciarse con variables de entorno, no exportarse en plaintext.

## Docs

- [Design system](DESIGN.md)
- [Product requirements](docs/product/prd.md)
- [Technical design](docs/design/tech-design.md)
- [Roadmap](ROADMAP.md)

## License

No open-source license has been selected yet. Until a license is added, the code is source-available but not broadly reusable under an open-source license.
