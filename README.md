# AVM

> Agent VM — define an AI coding agent once, run it on any runtime.

<p>
  <a href="https://github.com/xz1220/Agent-VM/actions/workflows/ci.yml"><img src="https://github.com/xz1220/Agent-VM/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <img src="https://img.shields.io/badge/status-early_preview-0f766e" alt="Status: early preview">
  <img src="https://img.shields.io/badge/runtime-Codex%20%7C%20Claude%20Code%20%7C%20OpenCode-1d4ed8" alt="Supported runtime targets">
  <img src="https://img.shields.io/badge/lang-Go%20%2B%20TypeScript-00ADD8" alt="Languages">
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-PolyForm--NC%201.0.0-6b21a8" alt="License: PolyForm Noncommercial 1.0.0"></a>
</p>

English | [简体中文](README.zh-CN.md)

## Introduction

AVM (`avm`) is a local config manager for AI coding agents. You build a
reusable **Agent** — instructions, skills, MCP servers, runtime preferences —
and AVM applies it to whichever target runtime you launch. AVM owns the
managed config, reports what each runtime can natively express, and keeps the
files you hand-edit out of its way.

The objects you'll see day-to-day:

- **Agent** — your reusable working profile; the only object you create or
  edit directly.
- **Capability** — a skill or MCP server an Agent references. AVM discovers
  the ones already installed in your runtimes and imports them.
- **Package** — a `.avm.zip` bundle exporting an Agent (and its
  capabilities) for sharing or reinstalling.
- **Runtime** — the target tool that actually runs the Agent.

AVM ships as two binaries that pair together:

- **`avm`** — the Go CLI, non-interactive plumbing. Every command takes
  flags or stdin, emits human text or `--json`. Use it from scripts and CI.
- **`avm-ui`** — the TypeScript full-screen TUI in [`ui/`](ui/) that shells
  out to `avm` over the JSON contract. Use it for interactive editing and
  browsing.

## Architecture

```mermaid
flowchart TD
    UI["<b>avm-ui</b><br/>TypeScript · Ink TUI<br/><sub>interactive editing &amp; browsing</sub>"]
    CLI["<b>avm</b><br/>Go · --json plumbing<br/><sub>scripts · CI · programmatic use</sub>"]
    APP["<b>Application Services</b><br/>Agent · Run · Package<br/>Capability · System"]
    RT["<b>Runtime Drivers</b><br/>codex · claude-code · opencode"]
    INFRA["<b>Infrastructure</b><br/>home · agentstore · capstore<br/>managedfile · runlog · packageio"]
    EXT["Codex · Claude Code · OpenCode · …<br/><sub>managed config + launch</sub>"]

    UI -->|spawn + JSON contract| CLI
    CLI --> APP
    APP --> RT
    APP --> INFRA
    RT --> EXT

    style UI fill:#dbeafe,stroke:#1d4ed8,color:#1e3a8a
    style CLI fill:#dbeafe,stroke:#1d4ed8,color:#1e3a8a
    style APP fill:#ccfbf1,stroke:#0d9488,color:#134e4a
    style RT fill:#ede9fe,stroke:#7c3aed,color:#3b0764
    style INFRA fill:#fed7aa,stroke:#ea580c,color:#7c2d12
    style EXT fill:#e5e7eb,stroke:#374151,color:#111827
```

The detailed mapping between this picture and the source tree lives in
[`docs/engineering/architecture-overview.md`](docs/engineering/architecture-overview.md).

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/xz1220/Agent-VM/main/scripts/install.sh | sh
avm init
avm shell install            # optional: bash/zsh/fish completion
```

The installer drops `avm` into `$HOME/.local/bin` and initializes `~/.avm`
unless you pass `AVM_SKIP_INIT=1`.

To build from source:

```bash
make build         # bin/avm
make build-ui      # dist/avm-ui.js (interactive TUI)
make build-all
```

## User Manual

### 1. Pull existing skills/MCP into AVM

The first time you use AVM on a machine, import whatever your runtimes
already have so Agents can reference them:

```bash
avm capability bootstrap --runtime claude-code
avm capability bootstrap --runtime codex
avm capability list
```

### 2. Create an Agent

Flag-driven (the CLI never prompts; use `avm-ui` if you want a wizard):

```bash
avm agent create \
  --name backend-coder \
  --runtime codex \
  --description "API + DB work on the order service" \
  --skill git --skill test
```

### 3. Inspect and edit

```bash
avm agent list
avm agent show backend-coder
avm agent show backend-coder --runtime codex   # how each field maps to the runtime
avm agent edit backend-coder --skill git --skill test --skill review
avm agent clone backend-coder --name backend-reviewer
avm agent rename backend-reviewer reviewer
avm agent delete reviewer --yes
```

`agent edit` is non-interactive: any list flag (`--skill`, `--mcp`,
`--runtime`) **replaces** the current list. To preserve current values, read
them first with `avm agent show <name> --json`.

### 4. Run

```bash
avm run backend-coder
avm run backend-coder --runtime codex      # required when the Agent has multiple runtimes
avm run backend-coder --preview            # show the plan; do not launch
avm run backend-coder --drift merge        # acknowledge drift between AVM and existing managed config
```

`avm run` propagates the runtime's exit code so shell scripts can branch on
it.

### 5. Share via Packages

```bash
avm package export backend-coder -o backend-coder.avm.zip
avm package install ./backend-coder.avm.zip --on-conflict rename
avm package inspect ./backend-coder.avm.zip
avm package list
avm package uninstall backend-coder --yes
```

### 6. Diagnose

```bash
avm doctor                  # AVM home, runtimes, recent runs
avm status [agent]
avm runtime list            # registered runtimes with availability
```

Every command above accepts `--json` and emits a model from
[`internal/app/model/`](internal/app/model/). The exact JSON shape, error
codes, and exit-code semantics live in [`docs/api/cli-protocol.md`](docs/api/cli-protocol.md).

## Runtime Support

AVM renders the selected Agent into runtime-specific managed files. Each
driver reports every Agent field as `native`, `rendered_as_instructions`,
`ignored`, or `unsupported` — `avm agent show <name> --runtime <rt>` shows
the full mapping.

| Runtime | Status | Notes |
| --- | --- | --- |
| Codex | Supported | Native profile/model/reasoning mapping; isolated `CODEX_HOME` per run |
| Claude Code | Supported | Agent frontmatter, MCP, skills; pruned auth state carried into the boundary |
| OpenCode | Supported | Config, agent, skills, and MCP mapping |
| OpenClaw (龙虾) | In progress | Runtime research complete ([`docs/.../openclaw-runtime.md`](docs/engineering/runtime-research/openclaw-runtime.md)); driver not yet implemented |
| Hermes Agent (爱马仕) | In progress | Listed in [`DESIGN.md`](DESIGN.md) as a target runtime; driver not yet implemented |

## License

AVM is released under the [PolyForm Noncommercial License 1.0.0](LICENSE).

In plain language:

- You may copy, modify, distribute, and learn from this code for any
  **non-commercial** purpose — personal projects, research, education,
  evaluation, hobby use, and use by non-profit organizations.
- **Commercial use requires a separate license** from the project owner.
- The software is provided "as is", without warranty.

If you'd like to use AVM commercially, open an issue or contact the
maintainer.

## Contributors

Thanks to everyone who has helped build AVM.

<a href="https://github.com/xz1220/Agent-VM/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=xz1220/Agent-VM" alt="Contributors" />
</a>

## Star History

<a href="https://star-history.com/#xz1220/Agent-VM&Date">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/svg?repos=xz1220/Agent-VM&type=Date&theme=dark" />
    <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/svg?repos=xz1220/Agent-VM&type=Date" />
    <img alt="Star History Chart" src="https://api.star-history.com/svg?repos=xz1220/Agent-VM&type=Date" />
  </picture>
</a>
