# Agent VM 文档

> 说明：本目录是代码仓库内的产品、技术设计和工程执行文档入口。

## 文档结构

```
docs/
├── product/
│   └── prd.md
├── design/
│   └── tech-design.md
├── engineering/
│   ├── architecture.md
│   ├── activation-model.md
│   ├── data-model.md
│   ├── file-layout.md
│   ├── fixture-conventions.md
│   ├── workflows.md
│   ├── acceptance.md
│   ├── implementation-plan.md
│   └── modules/
│       ├── adapter.md
│       ├── config.md
│       └── sync.md
├── research/
│   └── runtime-mapping.md
├── review/
│   └── entropy-control.md
├── test/
│   └── phase1-blackbox-test-plan.md
└── marketing/
    └── github-launch-checklist.md
```

## 快速入口

1. [product/prd.md](./product/prd.md) — 产品需求、范围和 Phase 1 目标。
2. [design/tech-design.md](./design/tech-design.md) — 技术设计总入口。
3. [engineering/architecture.md](./engineering/architecture.md) — 模块架构和依赖关系。
4. [engineering/implementation-plan.md](./engineering/implementation-plan.md) — coding 路径、并发 lane 和文件所有权。
5. [engineering/acceptance.md](./engineering/acceptance.md) — Phase 1 验收标准。
6. [engineering/activation-model.md](./engineering/activation-model.md) — AVM 激活模型、GVM-like shell-local 设计和持久渲染边界。
7. [research/runtime-mapping.md](./research/runtime-mapping.md) — runtime 配置映射调研。
8. [test/phase1-blackbox-test-plan.md](./test/phase1-blackbox-test-plan.md) — Phase 1 黑盒测试方案。
9. [review/entropy-control.md](./review/entropy-control.md) — AI coding 迭代中的仓库熵控制规范。
