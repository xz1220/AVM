# Agent VM 文档

> 日期：2026-04-26
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
│   ├── data-model.md
│   ├── file-layout.md
│   ├── fixture-conventions.md
│   ├── workflows.md
│   ├── acceptance.md
│   ├── stage5-acceptance-gap-report.md
│   ├── implementation-plan.md
│   └── modules/
│       ├── adapter.md
│       ├── config.md
│       └── sync.md
├── research/
│   └── runtime-mapping.md
└── reviews/
    └── pre-coding-review.md
```

## 快速入口

1. [product/prd.md](./product/prd.md) — 产品需求、范围和 Phase 1 目标。
2. [design/tech-design.md](./design/tech-design.md) — 技术设计总入口。
3. [engineering/implementation-plan.md](./engineering/implementation-plan.md) — coding 路径、并发 lane 和文件所有权。
4. [engineering/acceptance.md](./engineering/acceptance.md) — Phase 1 验收标准。
5. [engineering/stage5-acceptance-gap-report.md](./engineering/stage5-acceptance-gap-report.md) — Stage 5 当前可执行验收结果和缺口清单。
6. [research/runtime-mapping.md](./research/runtime-mapping.md) — runtime 配置映射调研。
7. [engineering/fixture-conventions.md](./engineering/fixture-conventions.md) — fixture layout、路径占位符和 runtime convention。

## 当前文档口径

- Stage 5 main 已覆盖 `init`、`agent create/list/show`、`env create`、`env create --local`、`memory import --dry-run`、`use/status/deactivate`、`sync`、`shell init`、`export/import` 的 smoke flow。
- Cursor Phase 1 成功写入时 runtime status 保持 `synced`；partial support 通过 warnings 和 mapping status 暴露。
- `avm init` runtime import-report 和 `avm agent show --runtime` mapping preview 属于 Stage 6 in-progress，除非对应分支已经合入 main，不应写成已发布能力。
