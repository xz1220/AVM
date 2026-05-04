# Activation Model

> 最后更新：2026-05-04（runtime boundary 隔离）

Activation is the process of turning an Agent Profile or Environment into
runtime-managed files.

```text
avm use <profile-or-env>
  -> update ~/.avm/config.yaml active ref
  -> resolve active object
  -> resolve per Agent/runtime boundary
  -> build render input for each target runtime
  -> adapter plans managed path writes
  -> sync applies plans with conflict checks and backups
  -> write sync-state.json
```

Activation does not read, import, export, reset, or edit memory content. It only
selects runtime-native isolation boundaries such as `CODEX_HOME`,
`CLAUDE_CONFIG_DIR`, and OpenCode's process env envelope.
