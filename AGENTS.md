# Repository Guidelines

## Project Structure & Module Organization

Agent VM is a Go CLI project. The executable entrypoint and Cobra commands live in `cmd/avm/`. Shared packages are under `internal/`, grouped by concern: `config`, `adapter`, `sync`, `runtime`, `state`, and `packageio`. Long-form documentation lives in `docs/`, with engineering details in `docs/engineering/`. Use `fixtures/` for realistic sample AVM homes and runtime layouts, and `testdata/` for stable inputs and expected outputs. Visual README assets are in `assets/`; developer scripts are in `scripts/`.

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

Implementation plans should name the owning abstraction, the data boundary, and the long-term behavior before listing code edits. If the existing code shape makes the correct design harder, adjust the abstraction instead of spreading special cases across CLI, sync, adapter, and config layers.

## Communication Discipline

When the user is exploring an idea or asking for judgment, use Socratic
clarifying questions before forcing a conclusion. Prefer 1-3 targeted questions
that expose assumptions, constraints, tradeoffs, or decision criteria.

Every final response at the end of a turn must be logically auditable. This is
a collaboration rule, not a tool-use rule. Even if the agent used tools during
the turn, the final response should explain the outcome with visible reasoning
structure:

- `Question`: restate the user's actual question or decision.
- `Premises`: list the key facts, assumptions, or constraints being used.
- `Reasoning`: explain the inference in clear steps, without exposing private chain-of-thought.
- `Conclusion`: state the answer or recommendation directly.
- `Uncertainty`: name what is unknown, what could change the conclusion, or what needs confirmation.

For very small answers, this structure may be compressed into a short paragraph
or a compact subset of headings, but the answer should still distinguish
evidence, inference, conclusion, and uncertainty.

## Testing Guidelines

Tests use the standard Go `testing` package and live beside implementation files as `*_test.go`. Prefer table-driven tests for validation, parsing, rendering, and CLI behavior. Put reusable golden inputs in `testdata/`; put human-readable scenario fixtures in `fixtures/`. Add tests for behavior changes, especially adapter mapping, config resolution, activation, sync, import/export, and error handling.

## Commit & Pull Request Guidelines

Recent history uses short imperative subjects, often with prefixes such as `fix:`, `feat:`, and `revert:`. Keep commits focused on one behavior or docs change.

PRs should include a summary, testing results, and any notes about docs, secrets, or machine-local paths. Follow `.github/pull_request_template.md`: check `go test ./...`, `go vet ./...`, update docs when behavior changes, and separate implemented behavior from planned behavior. Avoid broad rewrites or unrelated refactors unless already scoped.

## Security & Configuration Notes

Treat `~/.avm` as the source of truth. `avm init` must not modify runtime config files, and runtime writes should go through adapter-owned managed paths. Reference secrets rather than copying them into portable profiles.
