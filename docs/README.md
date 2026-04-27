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
│   ├── activation-model.md
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
├── test/
│   └── phase1-blackbox-test-plan.md
└── reviews/
    └── pre-coding-review.md
```

## 快速入口

1. [product/prd.md](./product/prd.md) — 产品需求、范围和 Phase 1 目标。
2. [design/tech-design.md](./design/tech-design.md) — 技术设计总入口。
3. [engineering/implementation-plan.md](./engineering/implementation-plan.md) — coding 路径、并发 lane 和文件所有权。
4. [engineering/acceptance.md](./engineering/acceptance.md) — Phase 1 验收标准。
5. [engineering/stage5-acceptance-gap-report.md](./engineering/stage5-acceptance-gap-report.md) — Stage 5 当前可执行验收结果和缺口清单。
6. [engineering/activation-model.md](./engineering/activation-model.md) — AVM 激活模型、GVM-like shell-local 设计和持久渲染边界。
7. [research/runtime-mapping.md](./research/runtime-mapping.md) — runtime 配置映射调研。
8. [engineering/fixture-conventions.md](./engineering/fixture-conventions.md) — fixture layout、路径占位符和 runtime convention。
9. [test/phase1-blackbox-test-plan.md](./test/phase1-blackbox-test-plan.md) — Phase 1 黑盒测试方案，按真实用户路径验证安装、切换和 runtime 生效。
10. [test/phase1-blackbox-test-report-2026-04-26.md](./test/phase1-blackbox-test-report-2026-04-26.md) — Phase 1 黑盒测试执行结果和问题清单。
11. [test/phase1-blackbox-retest-report-2026-04-26.md](./test/phase1-blackbox-retest-report-2026-04-26.md) — Phase 1 修复后的黑盒复测结果。
12. [test/phase1-skills-blackbox-report-2026-04-26.md](./test/phase1-skills-blackbox-report-2026-04-26.md) — Phase 1 skills 真实内容加载黑盒测试结果。
13. [test/phase1-skills-blackbox-retest-report-2026-04-26.md](./test/phase1-skills-blackbox-retest-report-2026-04-26.md) — Phase 1 skills 修复后的黑盒复测结果。
14. [test/phase1-skills-cleanup-retest-report-2026-04-26.md](./test/phase1-skills-cleanup-retest-report-2026-04-26.md) — Phase 1 skills stale 清理修复后的黑盒复测结果。

## 当前文档口径

- Stage 6 main 已覆盖 `init`、`agent create/list/show`、`agent show --runtime`、`env create`、`env create --local`、`memory import --dry-run`、`use/status/deactivate`、`sync`、`shell init`、`export/import` 的 smoke flow。
- `avm init` 会写 `state/import-report.json`，作为 read-only runtime scan 的报告；不会自动写入 imported agent/env，也不会修改 runtime 配置。
- Cursor Phase 1 成功写入时 runtime status 保持 `synced`；partial support 通过 warnings 和 mapping status 暴露。
