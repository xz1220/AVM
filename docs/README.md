# Agent VM 文档

## 核心文档

1. **PRD（要实现什么）**
   - [`product/prd.md`](./product/prd.md)

2. **架构（怎么实现的）**
   - [`rewrite-architecture-proposal.md`](./rewrite-architecture-proposal.md) — 四层分层和职责边界（presentation / application / runtime / infrastructure）。
   - [`engineering/architecture-overview.md`](./engineering/architecture-overview.md) — 上述文档与代码包的对应关系，以及阅读代码的入口。

3. **Runtime Research（底层依赖的 runtime 能力）**
   - [`engineering/runtime-research/claude-code-runtime.md`](./engineering/runtime-research/claude-code-runtime.md)
   - [`engineering/runtime-research/codex-runtime.md`](./engineering/runtime-research/codex-runtime.md)
   - [`engineering/runtime-research/openclaw-runtime.md`](./engineering/runtime-research/openclaw-runtime.md)

## 历史参考

- [`legacy-architecture.md`](./legacy-architecture.md) — 重写前旧架构（adapter/sync/state/config 多 registry）的单文件汇总，仅作历史参考，不再维护。
