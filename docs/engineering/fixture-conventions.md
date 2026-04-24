# Agent VM Fixture Conventions

Phase 1 fixtures define stable inputs and expected outputs for config
resolution, memory import dry-runs, adapter render plans, and runtime-specific
mapping behavior.

## Repository Layout

```
fixtures/
+-- phase1/
    `-- minimal/
        |-- config/
        |-- adapter-render-plan/
        |-- memory-import-dry-run/
        `-- runtimes/

testdata/
`-- phase1/
```

Use `fixtures/` for cross-package golden scenarios. Use `testdata/` for
package-local fixtures that should not become shared contracts.

## Path Tokens

Fixtures must not contain real user runtime paths. Use placeholders instead:

| Token | Meaning |
|-------|---------|
| `<AVM_HOME>` | Synthetic AVM home. |
| `<PROJECT_ROOT>` | Synthetic project root. |
| `<CODEX_HOME>` | Synthetic Codex config home. |
| `<CLAUDE_CODE_HOME>` | Synthetic Claude Code config home. |
| `<CLINE_DATA_HOME>` | Synthetic Cline data directory. |

## Fixture Families

Config fixtures contain AVM source-of-truth files such as `config.yaml`,
`agents/*.yaml`, `envs/*.yaml`, registry capabilities, portable memory, and
active manifests.

Memory import dry-run fixtures contain runtime-native memory input and expected
dry-run reports. They must not model writes to native runtime memory.

Adapter render-plan fixtures contain resolved activation input and per-runtime
expected plans. Plans should include managed paths, operations, and field-level
mapping status.

Runtime convention fixtures document how Codex, Claude Code, Cline, and Cursor
PoC paths are represented with tokens. Cursor fixtures must keep PoC status
visible because Phase 1 only promises MCP and rules file rendering.
