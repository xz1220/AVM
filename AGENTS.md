# Repository Guidelines

## Project Structure & Module Organization

Agent VM is a Go CLI project. `cmd/avm/main.go` is the only composition root: it wires concrete infra + runtime drivers into a `service.Container` and hands the CLI a single `Deps`. Shared packages under `internal/` follow the four-layer split documented in `docs/rewrite-architecture-proposal.md`:

- `internal/app/{model,service}` — AVM-stable product model and use-case orchestration (AgentService, RunService, PackageService, CapabilityService, DiagnosticsService).
- `internal/runtime/{driver,registry,types}` plus `internal/runtime/{codex,claudecode,opencode}` — RuntimeDriver per runtime, owning facts/adapter/boundary/launcher.
- `internal/infra/{home,agentstore,capstore,packageio,runlog,process,managedfile,fsutil}` — filesystem and process side effects.
- `internal/presentation/{cli,render}` — cobra commands, huh-powered interactive flows, and rendering helpers.

Long-form documentation lives in `docs/`, with engineering details in `docs/engineering/`. The CLI/UI contract is in `docs/api/cli-protocol.md`. `docs/legacy-architecture.md` is historical and not maintained. Visual README assets are in `assets/`; developer scripts are in `scripts/`. Tests construct their own fixtures with `t.TempDir()` and inline payloads — there is no shared `fixtures/` or `testdata/` directory.

## Build, Test, and Development Commands

- `make build`: builds the CLI to `bin/avm`.
- `make test`: runs `go test ./...`.
- `make fmt`: runs `gofmt -w ./cmd ./internal`.
- `make vet`: runs `go vet ./...`.
- `go run ./cmd/avm --help`: runs the CLI locally without installing it.
- `make clean`: removes generated build and coverage artifacts.

CI runs `go build ./...`, `go vet ./...`, `test -z "$(gofmt -l .)"`, and `go test ./...`.

## Coding Style & Naming Conventions

Target Go 1.23. Keep Go code `gofmt`-formatted and organized around small packages with clear ownership. Use idiomatic Go names: exported identifiers use `PascalCase`, unexported identifiers use `camelCase`, and test helpers stay unexported unless needed across files. CLI behavior belongs in `cmd/avm/`; reusable logic belongs in `internal/`. Avoid serializing secrets or machine-local paths.

## Engineering Approach

Prefer the correct long-term abstraction over the smallest local patch. When a change touches product semantics, config/state models, adapter contracts, activation, isolation boundaries, package IO, or runtime behavior, design the durable architecture first and implement toward it.

Do not use "minimum change", "short-term workaround", or "temporary compatibility path" as the primary solution for architectural work. A compatibility bridge is acceptable only when it preserves existing user data or staged migrations, and it must be explicitly documented as compatibility rather than the target design.

Implementation plans should name the owning abstraction, the data boundary, and the long-term behavior before listing code edits. If the existing code shape makes the correct design harder, adjust the abstraction instead of spreading special cases across the presentation, application service, runtime driver, and infrastructure layers.

## Communication Discipline

When the user is exploring an idea or asking for judgment, use Socratic
clarifying questions before forcing a conclusion. Prefer 1-3 targeted questions
that expose assumptions, constraints, tradeoffs, or decision criteria.

## Testing Guidelines

Tests use the standard Go `testing` package and live beside implementation files as `*_test.go`. Prefer table-driven tests for validation, parsing, rendering, and CLI behavior. Construct fixtures inline with `t.TempDir()` and `t.Setenv` — keep tests self-contained rather than depending on shared on-disk fixtures. Add tests for behavior changes, especially runtime driver mapping (Plan/Boundary/LaunchSpec), capability discovery and import, agent CRUD, package install/export, and the JSON error envelope.

## Commit & Pull Request Guidelines

Recent history uses short imperative subjects, often with prefixes such as `fix:`, `feat:`, and `revert:`. Keep commits focused on one behavior or docs change.

PRs should include a summary, testing results, and any notes about docs, secrets, or machine-local paths. Follow `.github/pull_request_template.md`: check `go test ./...`, `go vet ./...`, update docs when behavior changes, and separate implemented behavior from planned behavior. Avoid broad rewrites or unrelated refactors unless already scoped.

## Security & Configuration Notes

Treat `~/.avm` as the source of truth. `avm init` must not modify runtime config files, and runtime writes go through driver-owned managed paths under `~/.avm/boundaries/<runtime>/<agent>/`. Reference secrets rather than copying them into portable profiles.
