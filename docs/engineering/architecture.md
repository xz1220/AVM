# Architecture

> 最后更新：2026-05-03（移除显式 Memory 设计）

AVM 是本地 agent 配置管理器。`~/.avm` 是 source of truth，runtime 配置文件是
adapter 渲染结果。

## 核心对象

| 对象 | 说明 |
| --- | --- |
| Agent Profile | 用户管理的主对象，包含 instructions、capabilities、permissions、model/runtime preferences |
| Environment | 多 runtime 工作场景，保存 runtime 到 Agent Profile 的映射 |
| Capability Registry | skills、MCP、commands、hooks、toolsets 的元数据来源 |
| Package | Agent/Environment 及 capability metadata 的分发单元 |
| Sync State | 最近一次 render/apply 的状态、managed paths、mapping 结果 |

Memory 不在当前架构对象中。AVM 不声明、不导入、不导出、不渲染 portable
memory；runtime-native memory 暂时只作为未来原则讨论的研究对象。

## 数据流

```text
ActiveRef
  -> config.ResolveActivation
  -> adapter.RenderInput
  -> adapter.RenderPlan
  -> sync apply managed paths
  -> state.SyncState
```

## 不变量

- `~/.avm` 保存 AVM 管理的 Agent、Environment、registry、state。
- `avm init` 不写 runtime-native 配置。
- runtime 写入只能发生在 adapter 声明的 managed paths。
- adapter 不能静默丢字段；必须报告 mapping status。
- Package 安装不改变 active 对象。
- Secrets 只能引用，不复制明文。
