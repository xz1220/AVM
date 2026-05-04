# Workflows

> 最后更新：2026-05-03（移除显式 Memory 设计）

## Initialize

```bash
avm init
```

Creates `~/.avm`, default config, default Agent, default Environment, state,
backup, cache, and registry directories. It does not write runtime-native
configuration.

## Agent CRUD

```bash
avm agent create backend-coder --runtime codex --skills test --mcps github
avm agent list
avm agent show backend-coder
avm agent show backend-coder --runtime codex
avm agent edit backend-coder
avm agent clone backend-coder --name api-coder
avm agent rename backend-coder api-coder --update-refs
avm agent delete api-coder --force
```

Agent create/edit manage identity, instructions, runtime preferences, model
preferences, capabilities, and permissions. They do not manage memory.

## Environment

```bash
avm env create coding --codex backend-coder --claude-code reviewer
avm env create default --local --codex backend-coder
```

Environment maps runtimes to Agent Profiles.

## Activation

```bash
avm use backend-coder
avm use --kind env coding
avm status
avm sync
avm deactivate
```

`use` updates active config and applies runtime render plans. `sync` reapplies
the current active object for repair/debugging.

## Package

```bash
avm package inspect backend-coder.avm.zip
avm package export backend-coder --output backend-coder.avm.zip
avm package install backend-coder.avm.zip
```

Packages include Agent/Environment YAML and referenced capability metadata.
