# Minimal Phase 1 Fixture

This fixture models one AVM agent, one AVM environment, one MCP server, one
skill. It is deliberately small so tests can
focus on deterministic config resolution and per-runtime render planning.

The fixture is not a runtime config directory. Any path that would normally
point at a user's machine must use the tokens defined in `fixtures/README.md`.
