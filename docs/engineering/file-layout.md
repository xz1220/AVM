# File Layout

> 最后更新：2026-05-04（Agent-scoped runtime homes）

## AVM Home

```text
~/.avm/
├── config.yaml
├── agents/
├── envs/
├── registry/
│   ├── skills/
│   ├── mcps/
│   ├── commands/
│   ├── hooks/
│   └── toolsets/
├── active/
├── runtime-homes/
├── state/
├── backup/
└── cache/
```

There is no AVM-managed memory directory in the current design.

## Active Directory

`~/.avm/active/` is rebuilt by sync and may contain runtime-independent
projections for agents and capabilities:

```text
active/
├── agents/
├── skills/
├── mcps/
├── commands/
├── hooks/
└── render/
```

## Runtime Homes

AVM writes isolated runtime homes under:

```text
~/.avm/runtime-homes/agents/<agent-id>/<runtime>/
```

Adapters can also write project-managed files such as Cursor rules or Cline
rules when those paths are declared in the render plan.

## Permissions

- AVM home directories should be created with restrictive local permissions.
- Secrets should stay as environment variable references.
- Runtime-native user files are not overwritten unless an adapter declares an
  explicit managed path and conflict checks pass.
