# Claude Code Fixture Convention

Claude Code fixtures use `<CLAUDE_CODE_HOME>` for global runtime config and
`<PROJECT_ROOT>` for project-local files. Render-plan fixtures should cover:

- `<PROJECT_ROOT>/.claude/agents/<agent>.md` for the rendered agent.
- `<PROJECT_ROOT>/.mcp.json` for project MCP entries.
- runtime-native agent files as adapter inputs.

Runtime-native files must be treated as user-owned unless a test explicitly
exercises an adapter managed path.
