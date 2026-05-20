# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this repo is

AVM (`avm`) is a local config manager for AI coding agents. You define a reusable **Agent** (instructions, skills, MCP servers, runtime prefs) once and AVM renders it into runtime-specific managed config for Codex, Claude Code, OpenCode, etc. Two binaries that pair together:

- **`avm`** — Go CLI, **plumbing only**. Never prompts. Every command takes flags or stdin; emits human text or `--json`.
- **`avm-ui`** — TypeScript Ink TUI in `ui/` that shells out to `avm` over the JSON contract. All interactive UX lives here, not in Go.

The "Go CLI is plumbing, UI does interactivity" split is load-bearing — do not add prompts/wizards/TUI libraries to the Go side.

## Build, test, dev

```bash
make build           # bin/avm (Go 1.23, ldflags inject version/commit/date)
make build-ui        # ui/dist/avm-ui.js (pnpm via corepack, Node 22+)
make build-all
make test            # go test ./...
make fmt             # gofmt -w ./cmd ./internal
make vet             # go vet ./...
go run ./cmd/avm --help        # run CLI without installing

# Single test:
go test ./internal/app/service -run TestRunner_Plan
go test -run TestRunner_Plan ./...

# UI dev (after make build to get a fresh bin/avm):
cd ui && pnpm install && pnpm dev -- --avm ../bin/avm
cd ui && pnpm typecheck
```

CI gate (`.github/workflows/ci.yml`): `go build ./...`, `go vet ./...`, `gofmt -l .` must be empty, `go test ./...`, `make build-ui`, plus a real `scripts/install.sh` install smoke test. Match these locally before pushing.

## Architecture — the four layers

Inside `internal/`, dependency direction is strictly top-to-bottom. Reverse imports are forbidden; sibling crossings (runtime ↔ infra, infra ↔ infra) are also forbidden.

```
internal/presentation/{cli,render}          ← cobra commands, rendering. Only sees service.Container + model.
internal/app/{model,service}                ← stable product model + use-case services (Agent/Run/Package/Capability/Diagnostics/System).
internal/runtime/{driver,registry,types}    ← Driver interface + per-runtime drivers (codex, claudecode, opencode).
internal/infra/*                            ← side effects (agentstore, capstore, managedfile, process, runlog, packageio, home, fsutil).
```

`cmd/avm/main.go` is the **only** composition root: it builds the registry, instantiates each infra component, assembles a `service.Container`, and hands the CLI a `cli.Deps`. No package below it imports anything above it. When adding a new service or driver, wire it here — don't reach across layers.

The detailed map is in [`docs/engineering/architecture-overview.md`](docs/engineering/architecture-overview.md). The proposal-level "why" is in [`docs/rewrite-architecture-proposal.md`](docs/rewrite-architecture-proposal.md). `docs/legacy-architecture.md` is historical — ignore it.

## Runtime driver contract

Every runtime implements `runtime.Driver` (`internal/runtime/driver.go`) with `Facts / DiscoverGlobal / ExportGlobal / Plan / Boundary / LaunchSpec`. Each driver is self-contained under `internal/runtime/<name>/`.

Two invariants the application layer relies on:

- **App never writes runtime-managed paths.** Services call `Driver.Plan`, get back `[]runtime.ManagedFile`, and hand them to `infra/managedfile`, which is the only thing that touches those files on disk.
- **Per-(Agent, runtime) isolation** lives under `$AVM_HOME/boundaries/<runtime>/<agent-name>/`. The driver owns which env vars (e.g. `CODEX_HOME`) point the runtime at that boundary.

When adding/changing a runtime, also report every Agent field as `native`, `rendered_as_instructions`, `ignored`, or `unsupported` in the field-mapping output — `avm agent show <name> --runtime <rt>` surfaces it.

## CLI ↔ UI JSON contract

The UI depends on **`docs/api/cli-protocol.md`**, not on Go source. Treat that file as the versioned interface:

- Renaming a JSON field, removing an error code, or changing exit-code semantics is a **breaking change** — bump the contract and update the doc in the same PR.
- Adding new fields/codes/commands is non-breaking.
- `avm run <agent>` propagates the runtime's own exit code so shell scripts can branch on it. Non-`run` commands return `0` or `1`.
- JSON tags on `internal/app/model/*` structs are the source of truth for field names. Empty optional fields are `omitempty`.

## Conventions that aren't obvious from the code

- **`agent edit` list flags replace, not append.** `--skill`, `--mcp`, `--runtime` overwrite the existing list. To preserve current values, read with `avm agent show <name> --json` first. This is intentional non-interactivity — the UI handles merge UX.
- **Capability identity is content-addressed:** `sha256(kind + "\n" + name + "\n" + content_sha256)`. `ImportFrom` is audit-only — it never influences which Agent points at which capability. Don't add code that resolves capabilities by source path/runtime origin.
- **`~/.avm` is the source of truth.** `avm init` must not modify runtime config files. Runtime writes only go through driver-owned managed paths under `~/.avm/boundaries/...`. Reference secrets; never copy them into portable profiles.
- **Tests construct fixtures inline** with `t.TempDir()` and `t.Setenv` — there's no shared `fixtures/` or `testdata/` directory, and `testdata/` at the root is not a fixture dump. Keep tests self-contained.
- **Prefer table-driven tests** for validation, parsing, rendering, and CLI behavior. Add tests for runtime driver Plan/Boundary/LaunchSpec mapping, capability discovery + import, agent CRUD, package install/export, and the JSON error envelope.

## Engineering approach (from AGENTS.md)

Prefer the correct long-term abstraction over the smallest local patch. When a change touches product semantics, config/state models, adapter contracts, activation, isolation boundaries, package IO, or runtime behavior: design the durable abstraction first, then implement toward it. Don't spread special cases across presentation, service, driver, and infra layers — push the policy up into the right layer instead. A compatibility shim is only acceptable to preserve existing user data or a staged migration, and must be labeled as such.
