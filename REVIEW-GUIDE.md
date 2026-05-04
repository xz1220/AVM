# AVM Review Guide

AVM is a local configuration manager for AI coding runtimes. Review changes by
checking whether they preserve the current product boundary:

- Agent Profile is the main user object.
- Environment maps runtimes to Agent Profiles.
- Package moves Agent/Environment config plus capability metadata.
- Adapter writes only declared managed paths.
- Sync records status and conflicts without hiding unsupported fields.

## Current Package Map

```text
cmd/avm          CLI commands
internal/config  schema, YAML IO, validation, resolution
internal/adapter runtime render contracts and adapters
internal/sync    activation apply, conflict detection, state updates
internal/state   sync state storage
internal/runtime adapter registry
internal/packageio package inspect/export/install
```

There is no current memory module. Do not review against `memory_refs`,
portable memory import, or `avm memory` behavior; that design was removed.

## Review Priorities

1. User-visible behavior regressions.
2. Unsafe writes outside adapter managed paths.
3. Schema drift between config models, tests, fixtures, and docs.
4. Missing conflict checks before overwriting runtime/user files.
5. Secret handling in package export/import.
6. Mapping statuses that silently drop unsupported runtime fields.

## Commands To Run

```bash
go test ./...
go vet ./...
gofmt -l .
go build ./...
```
