# Phase 1 Fixtures

Phase 1 fixtures cover the source-of-truth config model, dry-run portable
adapter render plans and runtime-specific conventions for:

- Codex
- Claude Code
- Cline
- Cursor PoC

The `minimal/` fixture is intentionally small. It is a single backend agent
bound through a multi-runtime environment so future tests can exercise config
resolution and adapter planning without depending on real runtime files.
