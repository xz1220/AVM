# Fixtures

This directory contains repository-level fixtures for Agent VM behavior that
crosses package boundaries. Package-local test fixtures should live under
`testdata/` and may reference these examples instead of copying them.

## Layout

```
fixtures/
+-- phase1/
    `-- minimal/
        |-- config/
        |-- adapter-render-plan/
        `-- runtimes/
```

## Path Tokens

Fixtures must not include real user runtime paths. Use stable tokens:

| Token | Meaning |
|-------|---------|
| `<AVM_HOME>` | The synthetic AVM home used by the fixture. |
| `<PROJECT_ROOT>` | The synthetic project root used by the fixture. |
| `<CODEX_HOME>` | The synthetic Codex config home. |
| `<CLAUDE_CODE_HOME>` | The synthetic Claude Code config home. |
| `<CLINE_DATA_HOME>` | The synthetic Cline data directory. |

Runtime-specific examples should describe intent and expected writes without
assuming a developer's local machine layout.
