# Sync Module

> 最后更新：2026-05-04（runtime boundary）

`internal/sync` applies resolved activation to runtime-managed files.

## Responsibilities

- Resolve current active object.
- Resolve per Agent/runtime boundary.
- Build adapter render inputs.
- Ask adapters for render plans.
- Check managed path conflicts.
- Create backups when configured.
- Apply render operations.
- Persist sync state.

## Active Rebuild

`~/.avm/active/` may contain:

- agents
- skills
- MCP metadata
- commands
- hooks
- render metadata

It does not contain memory projections.

Sync does not manage memory content. It only writes adapter-managed runtime
config under the Agent's runtime boundary and records isolation status in
sync-state.

## Conflict Rules

AVM only writes adapter-declared managed paths. If an existing file has
conflicting unmanaged content, sync reports the conflict instead of overwriting
silently.
