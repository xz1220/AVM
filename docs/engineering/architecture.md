# Agent VM — 顶层架构设计

> 最后更新：2026-04-29（v8 — OpenCode adapter）

## 设计目标

1. **Agent Profile 一等对象** — 用 `AgentProfile` 描述一个可迁移的 agent，而不是只同步某几个配置文件。
2. **能力和记忆跟随 Profile** — skills、MCP、hooks、commands、permissions 和 memory refs 由 Agent Profile 引用；Environment 不重复定义能力或 memory。
3. **运行时适配** — adapter 把统一模型渲染为 Claude Code、Codex、OpenCode、Cline 等原生配置，并输出字段级映射状态。
4. **多 runtime 映射** — Environment 只负责把 runtime 绑定到各自的默认 Agent Profile。
5. **安全写入** — 默认只写 AVM 管理区或结构化字段；写前冲突检测和备份。

## 管理边界

| 资源 | Phase 1 是否管理 | 说明 |
|------|:---------------:|------|
| Agent Profile | 是 | `~/.avm/agents/<name>.yaml`，产品核心对象，包含能力引用和 memory refs |
| Environment | 是 | `~/.avm/envs/<name>.yaml`，多 runtime 到 Agent Profile 的激活映射 |
| Skills | 是 | registry + active symlink；运行时支持不同则 adapter 转换 |
| MCP | 是 | registry 中保存统一 server 定义，adapter 写 runtime 原生配置 |
| Commands/Hooks | 部分 | 统一 registry 保留；仅运行时支持时 native 写入 |
| Portable Memory | 部分 | Phase 1 支持 memory refs 和 import dry-run；Phase 2 做 diff/push/pull 迁移 |
| Rules/Instructions | 部分 | 作为 Agent Profile 输入渲染；不默认覆盖用户项目文件 |
| Runtime Preferences | 部分 | model、reasoning、approval、sandbox 等最小字段 |

**关键边界：** AVM 不默认覆盖 `AGENTS.md`、`CLAUDE.md`、`.cursorrules`、`.github/copilot-instructions.md` 这类用户/团队手写文件。adapter 必须优先写 AVM 管理片段或 runtime 原生 agent/profile 文件。

## 架构不变量

1. **Source of Truth 不变量**：`~/.avm` 保存 Agent Profile、Environment、Capability、Memory refs 和 render state；runtime 配置文件只作为导入来源或渲染输出。
2. **只读初始化不变量**：`avm init` 只能扫描和导入，不得修改 runtime 配置文件；它最多写入 `~/.avm/**`。
3. **受控激活不变量**：`avm use` 只能写 adapter 返回的 `ManagedPaths` 或 AVM 管理片段；写入前必须完成冲突检测和备份。
4. **用户资产保护不变量**：手写 rules、instructions、project guidance 默认保留；adapter 不得为了适配方便整文件覆盖。
5. **映射透明不变量**：adapter 对每个关键字段都必须输出 mapping status；`ignored` 和 `unsupported` 必须有 reason。
6. **保守导入不变量**：无法确定语义的 runtime 字段进入 `runtime_extensions` 或 import candidate，不得被强行折叠进统一模型。
7. **Portable Memory 显式读写不变量**：`avm use` 只投影当前 profile 的 memory refs；runtime native memory 的 pull/push/import/export 必须通过显式命令、diff 和用户确认。

---

## 架构分层

```
┌──────────────────────────────────────────────────────────────┐
│ CLI 层 cmd/avm                                                │
│ init | agent | env | use | deactivate | shell | sync | status │
│ memory | export | import                                      │
├──────────────────────────────────────────────────────────────┤
│ 核心层 internal/                                              │
│ ┌──────────┐ ┌───────────┐ ┌──────────┐ ┌──────────┐         │
│ │ config   │ │ memory    │ │ sync     │ │ packageio│         │
│ │ YAML     │ │ import    │ │ activate │ │ export   │         │
│ │ validate │ │ standards │ │ conflict │ │ import   │         │
│ └────┬─────┘ └─────┬─────┘ └────┬─────┘ └────┬─────┘         │
│      │             │            │            │               │
│ ┌────┴─────────────┴────────────┴────────────┴────┐          │
│ │ state: hash、render plan、mapping status、import report │      │
│ └───────────────────────┬─────────────────────────┘          │
├─────────────────────────┼────────────────────────────────────┤
│ Adapter 层 internal/adapter                                  │
│ Claude Code | Codex | OpenCode | Cline | Cursor PoC | future  │
│ Detect / Import / Plan / Render / ManagedPaths                │
├──────────────────────────────────────────────────────────────┤
│ 文件系统层                                                    │
│ ~/.avm/**                       # AVM source of truth          │
│ runtime config files            # adapter 管理的输出目标       │
│ <project>/.avm/env.yaml         # 项目级 runtime 绑定覆盖        │
└──────────────────────────────────────────────────────────────┘
```

## Source of Truth

AVM 的 source of truth 是 `~/.avm/`：

- `agents/`：Agent Profile
- `envs/`：多 runtime 激活映射
- `registry/`：capability 全量池
- `memory/`：portable memory 文件、元数据和导入候选
- `active/`：当前 profile/env 的实例化结果，派生目录，不由用户直接编辑
- `state/`：render status 和冲突检测 hash

各运行时配置文件是派生产物。用户可以手改 runtime 配置，但 AVM 下次写入前必须检测冲突；如果字段不是 AVM 管理的 section，必须保留。

---

## 核心数据流

### 写入方向：AVM → Runtime

```
avm use backend-coder
  │
  ├─ 1. config 解析 active ref
  │      如果是 profile：读取 ~/.avm/agents/backend-coder.yaml
  │      如果是 env：读取 ~/.avm/envs/<name>.yaml 并展开 runtime_agents
  │
  ├─ 2. activation resolver 展开：
  │      runtime -> AgentProfile
  │      profile capabilities -> registry item
  │      profile memory_refs -> portable memory refs
  │      runtime_overrides -> target-specific fields
  │
  ├─ 3. sync 重建 ~/.avm/active/
  │      skills/ commands/ hooks/ mcps/ agents/ memory/
  │
  ├─ 4. 对每个 target runtime 并发：
  │      Detect runtime
  │      Plan(active + runtime agent) -> RenderPlan
  │      Detect conflicts on ManagedPaths
  │      Backup ManagedPaths
  │      Render native config
  │      Save mapping status + file/managed hash
  │
  └─ 5. 更新 config.yaml.active、state/current-active 和 state/sync-state.json
```

### 读取方向：Runtime → AVM

`avm init` 和 `avm import` 可以读取已存在 runtime 配置：

```
adapter.Import()
  -> imported AgentProfile candidates
  -> imported MCP/skills/rules references
  -> default profile or environment
```

导入流程必须只读：adapter 不得在 `Import()` 中写入 runtime 文件或修改用户手写配置。导入结果必须保守：无法确定语义的字段放入 `runtime_extensions.<runtime>` 或标记为 candidate，不能丢弃。

`avm memory import --dry-run` 复用 adapter 的只读能力，但输出对象是 memory diff 而不是完整 profile 导入。它可以从 runtime native memory、rules、`AGENTS.md`、`CLAUDE.md` 等来源生成候选 portable memory；未确认前不得写入 `~/.avm/memory/` 或 runtime 文件。

---

## Adapter 输出模型

adapter 不只“写文件”，还要产出 render plan：

```go
type RenderPlan struct {
    Runtime      string
    Agent        string
    ManagedPaths []ManagedPath
    Mappings     []FieldMapping
}

type FieldMapping struct {
    SourcePath string // agents.backend.model_run.model
    TargetPath string // ~/.codex/agents/backend.toml model
    Status     string // native | rendered_as_instructions | ignored | unsupported
    Reason     string
}
```

`avm status` 必须展示关键 unsupported/ignored 字段，避免用户误以为配置已经生效。

---

## Phase 1 Runtime 矩阵

| Runtime | Agent | Skills | MCP | Permissions | Memory | 说明 |
|---------|-------|--------|-----|-------------|--------|------|
| Claude Code | native `.claude/agents/*.md` | native/symlink | native `.mcp.json` / settings | native settings | native scope 或 instruction | 完整 adapter |
| Codex | native `[agents]` + role TOML | instruction 或 future | native `[mcp_servers]` | native profile | instruction refs | 完整 adapter |
| OpenCode | native `agents/*.md` | native `skills/*/SKILL.md` | native `mcp` | native `permission` | instruction refs | 完整 adapter |
| Cline | rendered rules | rules/skills toggles | native `cline_mcp_settings.json` | native global state 部分字段 | rules/instructions | 完整 adapter，Agent 非原生 |
| Cursor | rendered rules | unsupported | native `.cursor/mcp.json` | unsupported | rules/instructions | 文件级 PoC |
| OpenClaw | future `agents.list[]` | native workspace skills | native `mcp.servers` | native sandbox/gateway | native/future | 仅设计约束 |

---

## 模块职责边界

| 模块 | 包路径 | 职责 | 不做什么 |
|------|--------|------|---------|
| config | `internal/config` | YAML 读写、schema 校验、profile/env 解析、路径解析 | 不写 runtime 文件 |
| adapter | `internal/adapter` | runtime detect/import/plan/render、managed paths | 不做全局合并策略 |
| sync | `internal/sync` | active 重建、adapter 编排、冲突检测 | 不直接了解 runtime 格式 |
| backup | `internal/backup` | 写入前备份 managed paths | 不决定备份策略 |
| memory | `internal/memory` | portable memory metadata、import dry-run、standards 校验 | 不静默写 runtime native memory |
| state | `internal/state` | hash、render status、last sync、import report | 不承载业务配置 |
| runtime | `internal/runtime` | adapter 注册表，按名称查找 adapter 实例 | 不实现具体 adapter 逻辑 |
| packageio | `internal/packageio` | 整包 export/import（ZIP 格式） | 不做 profile 解析 |
| version | `internal/version` | 版本号管理 | 无外部依赖 |

## 依赖关系

```
cmd/avm
  ├── config
  ├── adapter
  ├── sync         -> config, adapter, backup, state
  ├── memory       -> config
  ├── packageio    -> config
  ├── runtime      -> adapter/claude, adapter/codex, adapter/cline, adapter/cursor
  ├── state        -> adapter, config
  └── version
```

adapter 层内部依赖：
```
adapter/*  -> config（引用 config models）
backup     -> adapter, config
```

约束：

- adapter 不依赖 sync。
- state 不依赖 env。
- registry 不写 runtime config。
- config struct 是跨模块合同，不能在 adapter 中临时拼 map 替代。

---

## 并发模型

`avm use` 分两阶段：

```
阶段 1 串行：重建 ~/.avm/active/
  - 原子替换 active/skills、active/mcps、active/agents、active/memory

阶段 2 并发：每个 target runtime 独立 plan/render
  - adapter detect
  - conflict detection
  - backup
  - render
  - state update
```

单个 runtime 失败不回滚已成功 runtime，但最终输出必须说明失败项；如果 source env 无法解析，则整个命令立即失败。

---

## 错误处理策略

| 场景 | 策略 |
|------|------|
| AgentProfile YAML 解析失败 | 立即失败，给出文件和行号 |
| profile/env 引用不存在的 agent/capability | 默认失败；`--allow-missing` 时 warning + ignored |
| runtime 未安装 | warning，跳过该 target，退出码 0 |
| managed file 被外部修改 | 按 conflict strategy：prompt / avm-wins / local-wins |
| adapter 字段 unsupported | 不失败，但记录 mapping status 并在 status 显示 |
| 备份失败 | 不允许覆盖，当前 runtime 失败 |

---

## 性能目标

| 操作 | 目标 | 实现手段 |
|------|------|---------|
| `avm agent list` | < 100ms | 读 `agents/*.yaml` |
| `avm env list` | < 100ms | 读 `envs/*.yaml` |
| `avm status` | < 500ms | 并发读 state/runtime path |
| `avm use <profile/env>` | < 500ms | active 原子替换 + adapter 并发 |
| `avm memory import --dry-run` | < 1 秒 | adapter 只读扫描 + diff 输出 |

---

## 扩展性设计

新增 runtime 只需实现 adapter：

```go
type Adapter interface {
    Name() string
    Detect(ctx Context) Detection
    Import(ctx Context) (*ImportResult, error)
    Plan(ctx Context, input RenderInput) (*RenderPlan, error)
    Render(ctx Context, plan *RenderPlan) (*RenderResult, error)
    ManagedPaths(ctx Context, plan *RenderPlan) []ManagedPath
}
```

新增 capability 类型时：

1. 在 `config/models.go` 增加字段定义。
2. 在 `CapabilitySet` 增加引用字段。
3. adapter 逐个声明支持状态。
4. acceptance 增加至少一个 native 或 ignored 测试。
