# Contributing

Thanks for taking the time to look at Agent VM.

AVM is still early, so the best contributions are small, test-backed, and tied
to one runtime or one CLI behavior. Avoid broad rewrites unless an issue or
maintainer discussion has already narrowed the scope.

## Development Setup

Prerequisites:

- Go 1.23+

```bash
git clone https://github.com/xz1220/Agent-VM.git
cd Agent-VM

go test ./...
go vet ./...
go run ./cmd/avm --help
```

Useful commands:

```bash
make test
make vet
make fmt
make build
```

## Good First Contributions

- Add or improve runtime mapping notes.
- Add fixtures for real Codex, Claude Code, OpenCode, Cline, or Cursor configs.
- Add tests around CLI output and validation errors.
- Improve docs for a real multi-agent workflow.
- Report confusing behavior with exact commands and outputs.

## Design Constraints

- `~/.avm` is the AVM source of truth.
- `avm init` must not modify runtime config files.
- Runtime writes must go through adapter-owned managed paths.
- Unsupported runtime fields must be reported, not silently dropped.
- Runtime-native memory import/export is not part of the current AVM model.
  Do not add it without a separate design review.
- Secrets should be referenced, not serialized into portable profiles.

## Pull Request Checklist

- Run `go test ./...`.
- Run `go vet ./...`.
- Run `gofmt` through `make fmt` when editing Go files.
- Add tests for behavior changes.
- Keep docs honest about implemented versus planned behavior.
- Do not mix unrelated refactors into feature PRs.

## Issue Reports

Include:

- OS and shell
- Go version
- exact `avm` command
- expected behavior
- actual behavior
- relevant config snippets with secrets removed

For security issues, see [SECURITY.md](SECURITY.md).
