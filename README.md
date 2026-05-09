<p align="center">
  <img src="assets/avm-hero.svg" alt="Agent VM: one profile, every agent runtime" width="100%">
</p>

<h1 align="center">Agent VM</h1>

<p align="center">
  <strong>Manage AI coding-agent configs across runtimes.</strong>
  <br>
  Define an Agent once, then run it through Codex, Claude Code, OpenCode, Cline, or Cursor.
</p>

<p align="center">
  <a href="https://github.com/xz1220/Agent-VM/actions/workflows/ci.yml"><img src="https://github.com/xz1220/Agent-VM/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <img src="https://img.shields.io/badge/status-early_preview-0f766e" alt="Status: early preview">
  <img src="https://img.shields.io/badge/runtime-Codex%20%7C%20Claude%20Code%20%7C%20OpenCode-1d4ed8" alt="Supported runtime targets">
  <img src="https://img.shields.io/badge/language-Go%20%2B%20TypeScript-00ADD8" alt="Go + TypeScript">
</p>

<p align="center">
  English | <a href="README.zh-CN.md">简体中文</a>
</p>

Agent VM (`avm`) is a local config manager for AI coding agents. You build a
reusable **Agent** — instructions, skills, MCP servers, runtime preferences —
and AVM applies it to whichever target runtime you launch. AVM owns the
managed config, reports what each runtime can and cannot natively express, and
keeps your hand-edited runtime files out of its way.

The core objects you'll see:

- **Agent** — your reusable working profile. The only object you create or
  edit directly.
- **Capability** — a skill or MCP server the Agent references. AVM can
  discover the ones already installed in your runtimes and import them so
  Agents can reuse them.
- **Package** — a `.avm.zip` bundle that exports an Agent (and the
  capabilities it points at) for sharing or reinstalling on another machine.
- **Runtime** — the target tool that actually runs the Agent: Codex,
  Claude Code, OpenCode, Cline, or Cursor.

> Environment is internal-only. You will never need to create, switch, or
> manage one — Agents own their runtime configuration directly.

## Two Surfaces

AVM ships as two binaries that pair together:

| Binary | What it is | When to reach for it |
| --- | --- | --- |
| `avm` (Go CLI) | Non-interactive plumbing. Every command takes flags or stdin and emits human text or `--json`. | Scripts, CI, power users who like flags, anything programmatic. |
| `avm-ui` (TS TUI) | Full-screen interactive UI. Lives in [`ui/`](ui/) and shells out to `avm`. | Day-to-day editing, browsing the agent/capability list, the create/edit wizard. |

The Go CLI **never prompts**. If you want a wizard, run `avm-ui`.

## Quick Start

Install the CLI:

```bash
curl -fsSL https://raw.githubusercontent.com/xz1220/Agent-VM/main/scripts/install.sh | sh
avm init
avm shell install            # optional: shell completion for bash/zsh/fish
```

The installer drops `avm` into `$HOME/.local/bin` and initializes `~/.avm`
unless you pass `AVM_SKIP_INIT=1`.

Pull capabilities you already have installed in your runtimes into AVM so
Agents can reference them:

```bash
avm capability bootstrap --runtime claude-code
avm capability bootstrap --runtime codex
```

Create your first Agent (flag-driven; or use `avm-ui` for a wizard):

```bash
avm agent create \
  --name backend-coder \
  --runtime codex \
  --description "API + DB work on the order service" \
  --skill git --skill test
```

Run it:

```bash
avm run backend-coder
```

## Daily Commands

### Agent

```bash
avm agent list
avm agent show backend-coder
avm agent show backend-coder --runtime codex   # see how each field maps to that runtime
avm agent edit backend-coder --skill git --skill test --skill review
avm agent clone backend-coder --name backend-reviewer
avm agent rename backend-reviewer reviewer
avm agent delete reviewer --yes
```

`agent edit` is non-interactive: any list flag (`--skill`, `--mcp`,
`--runtime`) replaces the existing list. To preserve current values, read them
first with `avm agent show <name> --json`.

### Run

```bash
avm run backend-coder
avm run backend-coder --runtime codex      # required when the Agent has multiple runtimes
avm run backend-coder --preview            # show the plan; do not launch
avm run backend-coder --drift merge        # acknowledge drift between AVM and existing managed config
```

`avm run` propagates the runtime's exit code so shell scripts can branch on
it.

### Capabilities

Capabilities are AVM's internal record of skills/MCP servers. You usually
won't touch them once bootstrapped, but the surface is there:

```bash
avm capability discover                    # what AVM and runtimes can see right now
avm capability list                        # what's already in the AVM store
avm capability show <id>
avm capability import --runtime codex --kind skill --name my-skill
avm capability bootstrap --runtime claude-code
```

### Package

```bash
avm package list
avm package show <name>
avm package export backend-coder -o backend-coder.avm.zip
avm package install ./backend-coder.avm.zip --on-conflict rename
avm package inspect ./backend-coder.avm.zip
avm package uninstall backend-coder --yes
```

### System / diagnostics

```bash
avm doctor                  # AVM home, runtimes, recent runs
avm status [agent]
avm runtime list            # runtimes registered with AVM, with availability
avm shell install           # install completion for the current shell
avm uninstall --yes         # remove ~/.avm and the binary
```

Every command above accepts `--json` and emits a model from
[`internal/app/model/`](internal/app/model/). The exact JSON shape, error
codes, and exit-code semantics live in [`docs/api/cli-protocol.md`](docs/api/cli-protocol.md)
— that file is the source of truth for anything calling `avm` programmatically.

## Runtime Support

AVM renders the selected Agent into runtime-specific managed files.

| Runtime | Status | Notes |
| --- | --- | --- |
| Codex | Supported | Native profile/model/reasoning mapping; isolated `CODEX_HOME` per run |
| Claude Code | Supported | Agent frontmatter, MCP, skills; pruned auth state carried into the boundary |
| OpenCode | Supported | Config, agent, skills, and MCP mapping |
| Cline | Compatibility | Mostly rendered as rules/MCP settings |
| Cursor | Compatibility | Conservative rules/MCP proof of concept |

Each runtime driver reports every Agent field as `native`,
`rendered_as_instructions`, `ignored`, or `unsupported`. AVM never silently
drops something a runtime can't represent — `avm agent show <name> --runtime <rt>`
shows the full mapping.

## What's Not Yet Stable

This is an early preview. Things you may notice:

- The TS UI is on its first integration pass; some Agent-edit screens still
  call into mocked capability data. See [`ui/INTEGRATION-GAPS.md`](ui/INTEGRATION-GAPS.md).
- `avm package show` returns "registry not yet implemented" until the
  installed-package registry lands; use `avm package inspect` against the
  `.avm.zip` directly.
- `avm init` and `avm uninstall` are human-mode-only today (no `--json`).
- Cline and Cursor are best-effort; check `avm agent show --runtime` before
  trusting the mapping.

## Safety Model

AVM stays out of files it doesn't own:

- `avm init` writes only under `~/.avm`.
- Agents are an explicit CRUD resource — no implicit overwrites.
- Runtime-native files are written only through driver-declared managed
  paths. Anything else you've hand-edited is left alone.
- Unsupported runtime fields are reported in the mapping, not silently
  dropped.
- Secrets are referenced through environment variables, never exported as
  plaintext into a portable Package.

## Development

```bash
make build        # build bin/avm
make build-ui     # install ui deps, typecheck, build dist/avm-ui.js
make build-all    # both
make test
make vet
make fmt
```

Layout in one glance:

- `cmd/avm/` — Go CLI entry point (composition root only).
- `internal/presentation/cli/` — cobra commands, flag parsing, JSON/human
  rendering. Where `avm <subcommand>` lives.
- `internal/app/{model,service}/` — product model and service orchestration
  (`AgentService`, `RunService`, `PackageService`, `CapabilityService`,
  `DiagnosticsService`, `SystemService`).
- `internal/runtime/{codex,claudecode,opencode}/` — one driver per runtime,
  each owning its own boundary, plan, and launch spec.
- `internal/infra/` — side effects: `home`, `agentstore`, `capstore`,
  `managedfile`, `process`, `runlog`, `packageio`, `fsutil`.
- `ui/` — TypeScript + Ink interactive UI; consumes the Go CLI's `--json`
  contract only.

Useful project docs:

- [Product requirements (PRD)](docs/product/prd.md)
- [Architecture overview](docs/engineering/architecture-overview.md)
- [Rewrite architecture proposal](docs/rewrite-architecture-proposal.md)
- [CLI protocol contract](docs/api/cli-protocol.md)
- [Runtime research](docs/engineering/runtime-research/)

## License

No open-source license has been selected yet. Until a license is added, the
code is source-available but not broadly reusable under an open-source
license.
