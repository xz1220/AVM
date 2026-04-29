# Agent VM — 技术设计文档

> 最后更新：2026-04-24（v8 — Portable Memory 对齐 PRD v12）

本文档是 Agent VM 技术设计的索引入口。PRD v12 已将产品核心收敛为“Agent Profile + Portable Memory”：Agent Profile 是用户创建、切换、导出和分享的主对象；Capability Registry 存能力本体；Portable Memory 提供可审计的记忆迁移层；Environment 只在多 runtime 场景下保存 runtime 到 Agent Profile 的激活映射。

---

## 文档结构

```
docs/
├── product/
│   └── prd.md                 # 产品需求文档
├── design/
│   └── tech-design.md         # 技术设计总入口
├── engineering/
│   ├── architecture.md        # 顶层架构设计
│   ├── data-model.md          # 持久化数据模型
│   ├── file-layout.md         # ~/.avm/ 和 runtime 写入目标
│   ├── workflows.md           # 核心 CLI 流程
│   ├── acceptance.md          # Phase 1 验收标准
│   ├── implementation-plan.md # coding 执行路径和并发分工
│   └── modules/
│       ├── config.md          # 配置读写、解析、合并
│       ├── adapter.md         # runtime adapter 和 render plan
│       └── sync.md            # active 重建、冲突检测、备份、状态
├── research/
│   └── runtime-mapping.md     # Claude Code / Codex / Cline / OpenClaw 配置调研
└── reviews/
    └── pre-coding-review.md   # coding 前设计评审
```

---

## 快速导航

1. [prd.md](../product/prd.md) — 产品范围和 Phase 1 目标。
2. [architecture.md](../engineering/architecture.md) — 先看整体边界、数据流和 Phase 1 runtime 范围。
3. [data-model.md](../engineering/data-model.md) — 实现时以这里的 YAML/JSON schema 为合同。
4. [runtime-mapping.md](../research/runtime-mapping.md) — adapter 写入路径和运行时字段映射依据。
5. [modules/adapter.md](../engineering/modules/adapter.md) — 每个运行时如何标记 native / rendered-as-instructions / ignored。
6. [implementation-plan.md](../engineering/implementation-plan.md) — coding 执行顺序、并发 lane 和文件所有权。
7. [workflows.md](../engineering/workflows.md) 与 [acceptance.md](../engineering/acceptance.md) — CLI 行为与验收。

---

## 核心设计决策

### 1. Agent Profile 是一等对象

Phase 1 不再把环境理解为 skill/MCP 子集，而是：

```
Agent Profile
  + capability refs
  + memory refs
  + optional Environment runtime mapping
  -> Adapter Render Plan
  -> Runtime Native Config
```

`avm agent create/list/show` 管理 Agent Profile；`avm use <agent-profile>` 激活单个 profile；`avm env create` 只绑定 `codex -> backend-coder`、`claude-code -> code-reviewer`、`opencode -> opencode-coder` 这类 runtime 到 profile 的映射；`avm use <env>` 把映射后的 profiles 渲染到 Claude Code、Codex、OpenCode、Cline 等运行时。

### 2. 最小统一抽象，不追求最低公倍数

统一模型只覆盖跨运行时稳定成立的主干：

- identity: `name`、`description`、`role`
- runtime: preferred adapter、local/remote kind、primary/subagent mode、fallback
- source_scope: global、project、local
- instructions: system/developer/user-facing instructions
- model_run: model、reasoning effort、verbosity、temperature
- capabilities: skills、commands、hooks、MCP、tool permissions
- permissions: approval、sandbox、allow/deny
- memory_refs: 可迁移 memory 层、文件或条目；Phase 1 支持手动引用和 import dry-run

`workspace_isolation` 不进入 Agent Profile 的共同主干。project、worktree、container、remote workspace 由 runtime adapter 或用户启动方式决定；adapter 只能通过 mapping status 展示实际映射、ignored 或 unsupported。OpenClaw 这类 gateway runtime 的 workspace/routing/channel 字段先保存在 `runtime_extensions.<runtime>`。

运行时不能原生表达的字段必须进入 render plan，并标记为：

| 状态 | 含义 |
|------|------|
| `native` | 运行时有原生字段，可直接写入 |
| `rendered_as_instructions` | 无原生字段，但可安全渲染进 instruction |
| `ignored` | Phase 1 不写入，必须给出原因 |
| `unsupported` | 运行时明确不支持，不能假装生效 |

### 3. 能力绑定跟随 Agent Profile

能力本体在 registry，能力引用写在 Agent Profile：

- `~/.avm/registry/skills/<name>/` 保存 skill 本体。
- `~/.avm/registry/mcps/<name>.yaml` 保存 MCP 本体。
- `~/.avm/agents/<profile>.yaml` 通过 `capabilities` 和 `memory_refs` 引用它们。

Environment 不重复声明 capabilities 或 memory，只能在未来通过显式 override 做临时禁用或替换。

### 4. Portable Memory 是显式读写迁移层

Portable Memory 是 AVM 管理的可审计 memory 中间层。它可以从 runtime native memory、规则文件或 markdown/yaml 中显式导入，也可以在用户确认 diff 后写回目标 runtime。

Phase 1 只承诺：

- `memory_refs` 跟随 Agent Profile。
- `avm use` 只投影当前 profile 引用的 memory，不静默双向同步 runtime native memory。
- `avm memory import --dry-run` 能读取至少一个 runtime 或规则文件，输出可迁移 memory diff。

Phase 2 再实现 `avm memory pull --from <runtime>`、`avm memory push --to <runtime>`、`avm memory diff` 和跨 runtime 迁移确认流。

### 5. 激活体验接近 conda，但主激活对象是 Agent Profile

Phase 1 的用户心智是：

```bash
avm use backend-coder
# shell prompt:
(avm:backend-coder) $ codex
```

`avm use <agent-profile>` 是 Phase 1 主路径：更新 `~/.avm/config.yaml.active`、重建 `~/.avm/active/`，并把当前 Agent Profile 渲染到目标 runtime 的原生配置。

多 runtime 场景使用：

```bash
avm use backend-dev
(avm:backend-dev) $ codex   # backend-coder
(avm:backend-dev) $ claude  # code-reviewer
(avm:backend-dev) $ cline   # backend-assistant
```

shell integration 由 `avm shell init <shell>` 提供：它展示当前 active profile/env，并用 shell function 包装 `avm use`，让 `CODEX_HOME`、`CLAUDE_CONFIG_DIR`、`OPENCODE_CONFIG` 等 runtime env 在当前 shell 立即生效。

### 6. 管理边界：只写 AVM 管理区或结构化字段

AVM 可以写：

- `~/.avm/**`
- 各运行时的 AVM 管理片段或结构化配置字段
- adapter 明确声明的 runtime config section

AVM 不默认覆盖用户的项目指导文件，例如 `AGENTS.md`、`CLAUDE.md`、`.cursorrules`。如果某个运行时只能靠 instruction 文件表达，adapter 需要优先写 AVM 自己的 fragment/agent file，并在 render plan 中标明来源。

### 7. Symlink 仍用于目录型 capability，但不是产品核心

skills 这类目录资源仍可通过 active symlink 快速切换：

```
~/.avm/registry/skills/
~/.avm/active/skills/            # 当前环境子集
~/.avm/runtime-homes/<active>/<runtime>/skills/
```

但 Agent Profile、memory refs、runtime profile、MCP 和 permissions 都需要 adapter 结构化渲染，不能只靠 symlink。

### 8. Phase 1 runtime 范围

| Runtime | Phase 1 范围 |
|---------|--------------|
| Claude Code | 完整 adapter：agents、skills、MCP、settings、memory 引用渲染；native memory 只在显式 memory 命令中读写 |
| Codex | 完整 adapter：profiles、agent roles、MCP、instructions |
| OpenCode | 完整 adapter：`OPENCODE_CONFIG`、`OPENCODE_CONFIG_DIR`、agent markdown、skills、MCP、permission |
| Cline | 完整 adapter：rules、MCP、auto-approval、subagents 开关只做能力状态 |
| Cursor | 文件级 PoC：rules/MCP 映射，暂不承诺完整 Agent adapter |
| OpenClaw | 不实现 adapter，只作为 gateway/channel/remote workspace 设计约束；workspace/routing 字段进入 `runtime_extensions.openclaw` |

---

## 顶层不变量

这些不变量是 Phase 1 的实现边界，优先级高于单个 adapter 的便利性。

1. **`~/.avm` 是 Agent Profile 的 Source of Truth。**
   Runtime 配置文件是派生产物；`AGENTS.md`、`CLAUDE.md`、`.clinerules`、`.mcp.json`、`config.toml` 等文件可以被导入或渲染，但不再是 AVM 内部的主数据。

2. **`avm init` 只读扫描，不接管 runtime 配置。**
   `init` 可以创建 `~/.avm`、导入候选 Agent Profile/Capability/Environment、记录来源和映射状态，但不得修改 Claude Code、Codex、OpenCode、Cline、Cursor 等 runtime 的原始配置文件。

3. **`avm use <profile/env>` 才进入受控写入。**
   激活 profile/env 时，AVM 只能写 adapter 声明的 managed paths、managed sections 或 AVM 自己的 fragment/agent/profile 文件；首次写入前必须备份并做冲突检测。

4. **用户手写配置默认保留。**
   AVM 不默认覆盖 `AGENTS.md`、`CLAUDE.md`、`.cursorrules`、`.github/copilot-instructions.md` 等用户/团队资产；需要覆盖或接管时必须有显式策略。

5. **字段映射必须可解释。**
   每个 adapter 必须把关键字段标记为 `native`、`rendered_as_instructions`、`ignored` 或 `unsupported`，并让 `avm status` 可见；不得静默丢字段。

6. **导入必须保守。**
   runtime 原生字段无法可靠归一化时，必须保留到 `runtime_extensions.<runtime>` 或标记为 candidate，而不是猜测语义后写入统一字段。

7. **Portable Memory 显式读写不变量。**
   AVM 可以引用现有 markdown/yaml memory，也可以做 import dry-run；但 `avm use` 不从 runtime native memory 静默 pull，也不向 runtime native memory 静默 push。任何 runtime memory 写回都必须通过显式命令、diff 和用户确认。

---

## 技术栈

| 组件 | 技术选型 | 理由 |
|------|---------|------|
| 语言 | Go 1.23+ | 单二进制、跨平台、并发 sync |
| CLI 框架 | spf13/cobra | 命令层成熟稳定 |
| YAML/TOML/JSON | go-yaml/yaml、BurntSushi/toml 或 pelletier/go-toml、encoding/json | 运行时配置格式不同，避免字符串拼接 |
| 路径处理 | `os.UserHomeDir` + `filepath` | 避免平台路径差异 |

---

## 开发计划（Phase 1）

### Week 1：统一模型和本地 Profile

- Day 1: 项目结构、CLI 框架、`avm init`
- Day 2: config/data model：AgentProfile、Environment runtime mapping、Capability、PortableMemory
- Day 3: `avm agent create/list/show`、`avm memory import --dry-run`
- Day 4: `avm env create/use/status` 的解析和 render plan
- Day 5: sync state、冲突检测、备份

### Week 2：关键 runtime adapter

- Day 6: Claude Code adapter
- Day 7: Codex adapter
- Day 8: Cline adapter
- Day 9: Cursor PoC、export/import、memory import fixture
- Day 10: 验收测试、文档收尾、fixture 覆盖

---

## 性能目标

| 操作 | 目标 | 实现手段 |
|------|------|---------|
| `avm agent list` | < 100ms | 直接读 `agents/` 索引 |
| `avm env list` | < 100ms | 直接读 `envs/` |
| `avm status` | < 500ms | 并发检测 runtime 文件和 hash |
| `avm use <profile/env>` | < 500ms（普通本地环境） | active 目录重建 + 并发 adapter 写入 |
| `avm memory import --dry-run` | < 1 秒（普通本地 memory/rules 文件） | adapter 只读扫描 + diff 输出 |

---

## 技术风险与应对

| 风险 | 应对 |
|------|------|
| 运行时配置格式频繁变化 | adapter 独立，render plan 中输出字段级映射状态 |
| 覆盖用户配置 | 只写 AVM 管理区域或结构化字段；写前 hash 检测和备份 |
| 统一抽象过宽 | Phase 1 只实现最小主干；OpenClaw/Cursor 等复杂场景只保留字段，不假装完整支持 |
| memory 语义不一致 | Portable Memory 只做显式 import/push/pull；每次写回 runtime native memory 必须 diff + 用户确认 |
| secrets 泄露 | MCP/profile 只保存 `${ENV_VAR}` 引用，备份和 export 默认脱敏 |

---

## 下一步

1. 按 [data-model.md](../engineering/data-model.md) 实现 config struct。
2. 按 [runtime-mapping.md](../research/runtime-mapping.md) 实现 Claude Code、Codex、Cline adapter。
3. 按 [workflows.md](../engineering/workflows.md) 串起 CLI。
4. 按 [acceptance.md](../engineering/acceptance.md) 写 fixture 和端到端验收。

**原则：文档是实现合同；adapter 不得静默丢字段，必须把映射状态写入状态和用户可见输出。**
