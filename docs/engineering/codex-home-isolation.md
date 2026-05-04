# Codex Home Isolation

> 最后更新：2026-05-04（Agent-scoped runtime home）

The Codex adapter writes an AVM-owned runtime home instead of mutating the
user's native Codex config directly.

```text
~/.avm/runtime-homes/agents/<agent-id>/codex/
├── config.toml
└── agents/
    └── <agent>.toml
```

The render plan maps Agent fields to Codex profile/agent fields where possible
and renders unsupported guidance into developer instructions. It does not render
memory refs or portable memory content.
