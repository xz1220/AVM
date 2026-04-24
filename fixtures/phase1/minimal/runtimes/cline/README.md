# Cline Fixture Convention

Cline fixtures use `<CLINE_DATA_HOME>` for runtime data and `<PROJECT_ROOT>` for
workspace rules. Render-plan fixtures should cover:

- `<CLINE_DATA_HOME>/settings/cline_mcp_settings.json` for MCP server entries.
- `<PROJECT_ROOT>/.clinerules/avm/<agent>.md` for rendered instructions.
- `autoApprovalSettings` only for conservative, explicitly modeled fields.

Cline subagents are not AVM Agent Profiles; fixtures may expose their toggle
state but should not model them as primary agents.
