# Acceptance Criteria

> 最后更新：2026-05-04（runtime memory isolation）

## CLI Smoke

The current preview must support:

- `avm init`
- `avm agent create/list/show/edit/delete/rename/clone`
- `avm agent show --runtime <runtime>`
- `avm env create`
- `avm env create --local`
- `avm use`
- `avm status`
- `avm sync`
- `avm run <runtime>`
- `avm deactivate`
- `avm shell init`
- `avm package inspect/export/install`
- `avm skill list`

There is no `avm memory` acceptance path.

## Safety

- `avm init` writes only under `~/.avm`.
- Runtime-native config is written only through adapter managed paths.
- Runtime homes are keyed by stable Agent ID, not active/env name.
- OpenCode full data/state isolation is provided through `avm run opencode`.
- Unsupported mappings are surfaced in status.
- Package install does not activate the installed package.
- Secrets remain references.

## Verification

CI should run:

- `go build ./...`
- `go vet ./...`
- `test -z "$(gofmt -l .)"`
- `go test ./...`
