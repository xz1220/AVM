# Config Module

> 最后更新：2026-05-04（Agent ID）

`internal/config` owns AVM schema, YAML read/write helpers, validation, and
activation resolution.

## Responsibilities

- Read/write global config.
- Read/write/list Agent Profiles.
- Read/write/list Environments and project overrides.
- Validate known schema fields and reject unknown YAML fields.
- Resolve an active profile or environment into runtime agents and capabilities.

## Current Model

- `GlobalConfig`
- `AgentProfile`
- `Environment`
- `ActiveRef`
- capability registry entries
- resolved capability structs

`AgentProfile` includes a stable `id` used for runtime boundary isolation;
rename preserves it, clone/create generate a new one, and legacy profiles are
backfilled on read.

There is no config-level memory model. `PortableMemory`, `MemoryRef`,
`memory_refs`, and memory path helpers are intentionally absent.

## Resolution Contract

`ResolveActivation(ref, cwd)` returns:

- `Active`
- `Env`
- `RuntimeAgents`
- `Capabilities`
- `Targets`
- `SourceFiles`
- `Warnings`
