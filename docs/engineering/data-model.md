# Data Model

> 最后更新：2026-05-04（Agent ID 与 runtime boundary）

## Global Config

`~/.avm/config.yaml` 保存默认 scope、目标 runtime、冲突策略、写入模式和当前
active ref。

## Agent Profile

`~/.avm/agents/<name>.yaml` 或 `<project>/.avm/agents/<name>.yaml`：

```yaml
name: backend-coder
id: agt_11111111111111111111111111111111
version: 1.0.0
source_scope: global
runtime:
  preferred: codex
  kind: local
  mode: primary
  fallback:
    - claude-code
instructions:
  system: |
    You implement backend changes with tests.
  developer: |
    Prefer small, reviewable changes.
capabilities:
  skills:
    - test
  mcps:
    - github
permissions:
  approval: on-request
  sandbox: workspace-write
model_run:
  model: gpt-5.4
  reasoning_effort: medium
```

`id` 是稳定 Agent identity：rename 保留，clone/create 生成新值。runtime
memory isolation 使用 `id + runtime` 决定边界目录。

Agent Profile 当前没有 memory 字段。任何 `memory_refs`、portable memory metadata
或 memory layers 都不属于当前 schema。

## Environment

`~/.avm/envs/<name>.yaml`：

```yaml
name: coding
version: 1.0.0
runtime_agents:
  codex:
    primary: backend-coder
  claude-code:
    primary: reviewer
targets:
  - codex
  - claude-code
```

Environment 只做 runtime 到 Agent Profile 的映射，不重复声明 capabilities。

## Resolved Activation

`config.ResolveActivation` 输出：

- active ref
- resolved runtime agents
- resolved capabilities
- targets
- source files
- warnings

## Package Manifest

Package manifest 当前包含：

- version/exported_at/kind/name
- agents
- envs
- capabilities
- include_files
