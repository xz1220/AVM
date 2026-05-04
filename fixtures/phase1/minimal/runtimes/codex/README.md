# Codex Fixture Convention

Codex fixtures use `<CODEX_HOME>` for runtime config paths. Render-plan
fixtures should cover:

- `<CODEX_HOME>/config.toml` for the active AVM profile, MCP servers, and role
  registration.
- `<CODEX_HOME>/agents/<agent>.toml` for the rendered role config.
- Skills rendered into `developer_instructions` unless a
  future native mapping is added.

Do not use a developer's real `config.toml` or agents directory in fixtures.
