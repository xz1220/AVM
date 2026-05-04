# Agent VM — 技术设计入口

> 最后更新：2026-05-03（移除显式 Memory 设计）

当前技术设计以 **Agent Profile + Environment + Package + Adapter** 为主线。
Memory 不再是 AVM 的显式配置对象：没有 `memory_refs` schema、没有
`avm memory` 命令、没有 portable memory 文件布局，也不会在 adapter 中渲染成
runtime 指令。

后续即使重新讨论 memory，当前也只关注全局/用户级 runtime state 的隔离。
不讨论 memory 内容管理、导入导出、重置、同步或 runtime memory 开关。项目级
memory、repo 内规则文件、workspace memory 文件属于项目资产，不由 AVM 管理、
迁移或隔离。

## 文档结构

```
docs/
├── product/prd.md
├── design/tech-design.md
├── design/runtime-memory-isolation.md
├── engineering/
│   ├── architecture.md
│   ├── data-model.md
│   ├── file-layout.md
│   ├── workflows.md
│   ├── acceptance.md
│   └── modules/
│       ├── config.md
│       ├── adapter.md
│       └── sync.md
└── research/runtime-mapping.md
```

## 核心决策

1. Agent Profile 是用户创建、编辑、激活和分享的主对象。
2. Environment 只保存 runtime 到 Agent Profile 的映射。
3. Package 只携带 Agent、Environment 和 capability metadata。
4. Adapter 必须报告字段映射状态：`native`、`rendered_as_instructions`、
   `ignored`、`unsupported`。
5. Sync 是 `use` 的实现细节，同时保留为高级修复命令。
6. Memory 原则需要重新讨论；当前范围仅限全局/用户级 runtime state 隔离，
   项目级 memory 不进入 AVM schema 或 CLI。

## 当前模块

- `internal/config`：配置模型、YAML 读写、active/env/agent resolution。
- `internal/adapter`：runtime-independent render input 与各 runtime adapter。
- `internal/sync`：生成 render plan、写 managed paths、记录状态。
- `internal/packageio`：package inspect/export/install。
- `internal/runtime`：runtime registry。
- `internal/state`：sync state。
