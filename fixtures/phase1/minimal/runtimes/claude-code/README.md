# Claude Code Fixture Convention

Claude Code fixtures use `<CLAUDE_CODE_HOME>` for global runtime config and
`<PROJECT_ROOT>` for project-local files. Render-plan fixtures should cover:

- `<PROJECT_ROOT>/.claude/agents/<agent>.md` for the rendered agent.
- `<PROJECT_ROOT>/.mcp.json` for project MCP entries.
- `<CLAUDE_CODE_HOME>/agent-memory/<agent>/MEMORY.md` only as dry-run import
  input.

Native memory files must be read-only inputs for dry-run fixtures unless a test
explicitly covers a confirmed push or pull workflow.
