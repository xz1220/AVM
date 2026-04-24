<p align="center">
  <img src="assets/avm-hero.svg" alt="Agent VM: one profile, every coding agent runtime" width="100%">
</p>

<h1 align="center">Agent VM</h1>

<p align="center">
  <strong>nvm for AI coding agents.</strong>
  <br>
  Un profil portable pour les outils, permissions, réglages de modèle et memory refs.
</p>

<p align="center">
  <a href="https://github.com/xz1220/Agent-VM/actions/workflows/ci.yml"><img src="https://github.com/xz1220/Agent-VM/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <img src="https://img.shields.io/badge/status-early_preview-0f766e" alt="Status: early preview">
  <img src="https://img.shields.io/badge/runtime-Codex%20%7C%20Claude%20Code%20%7C%20Cline%20%7C%20Cursor-1d4ed8" alt="Runtime targets">
  <img src="https://img.shields.io/badge/language-Go-00ADD8" alt="Go">
</p>

<p align="center">
  <a href="README.md">English</a> | <a href="README.zh-CN.md">简体中文</a> | <a href="README.ja.md">日本語</a> | <a href="README.ko.md">한국어</a> | <a href="README.es.md">Español</a> | <a href="README.pt-BR.md">Português</a> | Français
</p>

Agent VM, ou `avm`, est un control plane local pour les profils d'agents IA de programmation. Vous définissez un agent une seule fois, puis vous rendez ce profil vers des runtimes comme Codex, Claude Code, Cline et Cursor.

Le pari : les développeurs ne vont pas se standardiser sur un seul coding agent. Il manque un objet portable qui décrit qui est l'agent, ce qu'il peut utiliser, ses réglages de modèle, ses permissions et la mémoire long terme qu'il doit porter.

<p align="center">
  <img src="assets/avm-before-after.svg" alt="Before AVM config is scattered; after AVM one profile activates an agent" width="100%">
</p>

## Le geste clé

```bash
avm use backend-coder
```

Cette commande doit devenir le réflexe pour changer votre environnement local d'AI coding. Au lieu de reconstruire le même rôle dans des prompt files, MCP config, rules directories et notes de mémoire, AVM fait de l'Agent Profile la source of truth.

```text
backend-coder.yaml
  -> avm use backend-coder
    -> Codex profile
    -> Claude Code agent
    -> Cline rules
    -> Cursor rules
```

## Ce qui change

| Approche | Ce qui est géré | Ce qui manque |
| --- | --- | --- |
| Dotfiles | Fichiers et symlinks | Pas d'objet Agent, pas de mapping status |
| MCP config managers | Configuration des tool servers | Souvent pas de role, memory, model ou permissions |
| Runtime-native profiles | Un seul écosystème | Difficile à porter vers d'autres runtimes |
| Agent VM | Agent Profile + capabilities + memory refs + adapters | Projet jeune; concrete adapters en construction |

Chaque adapter doit indiquer comment les champs sont mappés : `native`, `rendered_as_instructions`, `ignored` ou `unsupported`.

## Ce qu'un Profile contient

| Couche | Exemple |
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

## Statut

Ce dépôt est une early preview. Le modèle central et les premiers morceaux de CLI sont en place; le prochain jalon majeur est profile activation.

Disponible aujourd'hui :

- `avm init`
- `avm agent create/list/show`
- `avm env create`
- `avm memory import --from <file> --dry-run`
- config validation and resolution tests
- adapter contract, fake adapter, and Phase 1 fixtures

En cours :

- `avm use <profile-or-env>`
- `avm status`
- `avm deactivate`
- concrete Codex and Claude Code adapter writes
- release packaging

## Quickstart

Prérequis :

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

- `avm init` écrit uniquement sous `~/.avm`.
- La runtime-native memory n'est importée que par des commandes explicites.
- `memory import` prend en charge un dry-run avant écriture.
- Les adapters ne possèdent que les managed paths déclarés.
- Les champs non supportés sont reportés, pas supprimés silencieusement.
- Les secrets doivent être référencés via des variables d'environnement, pas exportés en plaintext.

## Docs

- [Design system](DESIGN.md)
- [Product requirements](docs/product/prd.md)
- [Technical design](docs/design/tech-design.md)
- [Roadmap](ROADMAP.md)

## License

No open-source license has been selected yet. Until a license is added, the code is source-available but not broadly reusable under an open-source license.
