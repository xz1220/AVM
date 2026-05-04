# Implementation Plan

> 最后更新：2026-05-04（runtime memory isolation）

## Current Done Scope

- CLI scaffold and root command.
- `init`, `use`, `status`, `sync`, `deactivate`, `shell init`.
- Agent create/list/show/edit/delete/rename/clone.
- Environment create and local project override.
- Runtime adapters for Codex, Claude Code, OpenCode, Cline, Cursor.
- Package inspect/export/install.
- Capability registry lookup for skills/MCP and adapter projection.
- Runtime memory isolation by stable Agent ID, including Codex/Claude runtime
  homes and OpenCode `avm run` process envelope.

## Removed Scope

The previous explicit memory implementation has been removed:

- `avm memory`
- `internal/memory`
- `memory_refs`
- portable memory config CRUD
- memory package export/import
- adapter memory rendering

## Next Work

- Complete Environment CRUD.
- Unify package command naming and lifecycle.
- Improve first-run and interactive create/edit flows.
- Add doctor/uninstall.
- Keep future memory content management out of scope until a separate product
  model exists.
