# Agent VM — Pre-Coding Review Report

> 日期：2026-04-24
> 范围：PRD v12 + tech-design v8 + docs/ 设计文档
> 目的：coding 前确认产品心智、数据模型和验收标准一致

---

## 结论

可以进入 coding。

本轮已确认并同步核心设计：Phase 1 以 Agent Profile 为主对象，capabilities 和 memory refs 跟随 Agent Profile；Environment 只做多 runtime 到 Agent Profile 的激活映射。

---

## 已锁定决策

1. **主激活对象是 Agent Profile。**
   `avm use backend-coder` 是 Phase 1 的最小核心体验。

2. **能力引用写在 Agent Profile。**
   Capability Registry 存能力本体，Agent Profile 引用 skills、MCP、commands、hooks 和 memory refs。

3. **Environment 是 runtime 映射。**
   `avm use backend-dev` 的含义是让 Codex、Claude Code、Cline 各自切到预设的 Agent Profile，而不是把能力重新组合一次。

4. **状态使用 active ref。**
   `config.yaml.active` 和 `state/current-active` 记录当前 active kind/name，kind 为 `profile` 或 `env`。

---

## Coding 前验收重点

- `avm use <agent-profile>` 能完成单 profile 激活、prompt 展示、runtime 写入和 status 展示。
- `avm env create <name> --codex <agent> --claude-code <agent> --cline <agent>` 只写 runtime mapping。
- Environment YAML 不接受 `capabilities` 或 `memory_layers`。
- export/import 单 Agent Profile 时能带上 referenced capabilities 和 memory refs。
- adapter mapping 不能静默丢字段，unsupported/rendered/ignored 都必须进入 status。

---

## 不再阻塞的旧问题

- env capabilities 与 agent capabilities 的合并语义已取消。
- env memory_layers 与 agent memory_refs 的优先级问题已取消。
- 旧的单字符串 active 字段已被 `active.kind/name` 取代。
