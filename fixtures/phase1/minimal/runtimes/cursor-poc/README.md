# Cursor PoC Fixture Convention

Cursor fixtures use `<PROJECT_ROOT>` because Phase 1 support is file-level PoC
only. Render-plan fixtures should cover:

- `<PROJECT_ROOT>/.cursor/mcp.json` for MCP server entries.
- `<PROJECT_ROOT>/.cursor/rules/avm-<agent>.md` for rendered instructions.

Cursor plans must declare `status: partial` or an equivalent field-level
mapping status for unsupported Agent Profile and permission behavior.
