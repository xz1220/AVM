# Agent-VM 项目接口级认知指南

> 模块名：`github.com/xz1220/agent-vm` | Go 1.22 | ~10,000 行代码

## 一句话概括

AVM 是一个 **多 AI-runtime 的 agent 配置管理器**。它把 agent profile、environment、memory 等配置统一建模，然后通过 adapter 模式同步到不同的 AI coding runtime（Claude Code、Cline、Cursor、Codex）。

---

## 架构分层（自底向上）

```
┌─────────────────────────────────────────────────┐
│                  cmd/avm (CLI)                   │  ← 用户入口，16 个子命令
├─────────────────────────────────────────────────┤
│              internal/sync (编排层)               │  ← 把 resolved config 同步到各 runtime
├──────────┬──────────┬───────────┬───────────────┤
│  state   │  backup  │  memory   │  packageio    │  ← 状态追踪 / 备份 / 记忆导入 / 包导入导出
├──────────┴──────────┴───────────┴───────────────┤
│           internal/runtime (注册表)               │  ← 持有所有 adapter 实例
├─────────────────────────────────────────────────┤
│  adapter/claude  adapter/cline  adapter/codex   │
│  adapter/cursor  adapter/fake   adapter/shared  │  ← 各 runtime 的具体实现
├─────────────────────────────────────────────────┤
│           internal/adapter (接口层)               │  ← 核心抽象：Adapter interface
├─────────────────────────────────────────────────┤
│           internal/config (基础层)                │  ← 所有数据模型 + YAML 读写 + 校验
└─────────────────────────────────────────────────┘
```

---

## 核心接口清单（全项目仅 5 个 interface）

### 1. `adapter.Adapter` — 最核心的抽象

```go
// internal/adapter/adapter.go:11
type Adapter interface {
    Name() string                                                    // runtime 名称
    Detect(ctx Context) Detection                                    // 检测本机是否安装了该 runtime
    Import(ctx Context) (*ImportResult, error)                       // 从 runtime 导入已有配置
    Plan(ctx Context, input RenderInput) (*RenderPlan, error)        // 根据 resolved config 生成渲染计划
    Render(ctx Context, plan *RenderPlan) (*RenderResult, error)     // 执行渲染，写入 runtime 配置文件
    ManagedPaths(ctx Context, plan *RenderPlan) []ManagedPath        // 声明该 adapter 管理哪些文件路径
}
```

**这是整个项目的核心**。所有 runtime 适配都围绕这 6 个方法展开。

### 2. `adapter.MemoryImportCapable` — 可选能力接口

```go
// internal/adapter/adapter.go:22
type MemoryImportCapable interface {
    ImportMemory(ctx Context, opts MemoryImportOptions) (*MemoryImportPlan, error)
}
```

目前只有 `fake` adapter 实现了它（用于测试）。

### 3. `sync.AdapterRegistry` — adapter 注册表

```go
// internal/sync/types.go:18
type AdapterRegistry interface {
    Get(runtime string) (adapter.Adapter, bool)
}
```

sync 层通过这个接口获取 adapter，不直接依赖具体实现。

### 4-5. `initAdapterRegistry` / `agentMappingPreviewRegistry`

CLI 层定义的两个局部接口，签名与 `AdapterRegistry` 完全相同，用于 CLI 命令内部的依赖注入。

---

## 各包职责与关键函数

### `internal/config` — 基础层（1,670 行，12 文件）

**职责**：所有数据模型的 source of truth + YAML 持久化 + 校验

| 函数 | 作用 |
|------|------|
| `ResolveActivation(ref, cwd)` | **最关键的函数**。把 ActiveRef → 完整的 ResolvedActivation（合并 agent + env + override + memory + capabilities） |
| `ReadAgent / WriteAgent / ListAgents` | Agent profile 的 CRUD |
| `ReadEnvironment / WriteEnvironment / ListEnvironments` | Environment 的 CRUD |
| `ReadProjectOverride / WriteProjectOverride` | 项目级覆盖配置 |
| `ReadPortableMemory / WritePortableMemory / ListPortableMemory` | 可移植记忆的 CRUD |
| `Validate / ValidateAgentProfile / ValidateEnvironment` | 各类配置校验 |
| `AvmDir() / GlobalConfigPath() / AgentsDir() / EnvsDir()` | 路径解析（~20 个路径函数） |

**核心数据模型**：
- `AgentProfile` — agent 的完整定义（身份、指令、模型参数、IO 契约、能力、权限、生命周期钩子）
- `Environment` — 环境定义（包含哪些 runtime agent、target 列表、runtime 覆盖）
- `ResolvedActivation` — 解析后的激活状态（最终要同步到 runtime 的完整数据）
- `PortableMemory` — 可移植记忆（跨 runtime 共享的上下文信息）

### `internal/adapter` — 接口层（573 行，3 文件）

**职责**：定义 Adapter 接口 + 所有相关的输入输出类型

关键类型（在 `types.go` 和 `config_input.go` 中）：
- `RenderInput` — Plan 的输入
- `RenderPlan` / `RenderOperation` — 渲染计划（要对 runtime 配置做什么操作）
- `RenderResult` — 渲染结果
- `ManagedPath` — adapter 管理的文件路径声明
- `Detection` — runtime 检测结果
- `Context` — adapter 操作的上下文

### `internal/adapter/claude` — Claude Code 适配器（1,127 行）

**职责**：把 resolved config 渲染为 Claude Code 的配置格式

- 管理 `~/.claude/` 下的配置文件
- `Plan()` 生成针对 CLAUDE.md、settings.json 等文件的写入计划
- `Render()` 执行实际文件写入
- `Import()` 从已有 Claude Code 配置反向导入

### `internal/adapter/cline` — Cline 适配器（949 行）

同上模式，管理 Cline（VS Code 扩展）的配置目录。支持 VS Code / Cursor / VSCodium 多种宿主。

### `internal/adapter/codex` — Codex 适配器（1,184 行）

同上模式，管理 OpenAI Codex CLI 的配置。Import 目前是 placeholder。

### `internal/adapter/cursor` — Cursor 适配器（606 行）

同上模式，管理 Cursor 的 `.cursor/` 目录。Import 目前是 placeholder。

### `internal/adapter/fake` — 测试用 adapter（425 行）

可配置行为的 mock adapter，用于 sync 层的单元测试。

### `internal/adapter/shared` — 共享工具（225 行）

跨 adapter 复用的工具函数：原子写文件、slug 生成、JSON 格式化、MCP server 排序等。

### `internal/adapter/renderplan` — 渲染计划工具（83 行）

`Normalize()` 函数，标准化渲染计划。

### `internal/sync` — 编排层（1,134 行，5 文件）

**职责**：把 ResolvedActivation 同步到各 runtime target

| 函数 | 作用 |
|------|------|
| `NewSyncer(registry) → *Syncer` | 创建 syncer |
| `Syncer.SyncActivation(ctx, resolved, opts)` | **核心编排**：遍历 targets → 对每个 runtime 调用 adapter.Plan → adapter.Render → 记录状态 |
| `RebuildActive(resolved, activeDir)` | 重建 active 目录结构 |
| `DetectConflicts(runtime, paths, state)` | 检测文件冲突（其他 runtime 是否管理了同一文件） |

### `internal/state` — 状态追踪（187 行）

**职责**：持久化 sync 状态到 `sync-state.json`

- `SyncState` — 整体同步状态（版本、上次激活、各 runtime 状态）
- `RuntimeState` — 单个 runtime 的同步状态（managed paths、mappings、warnings）
- `LoadSyncState / SaveSyncState` — JSON 读写

### `internal/memory` — 记忆导入（699 行）

**职责**：从 runtime 导入记忆文件，生成 diff 和迁移计划

- `ImportDryRun(opts)` — 预览导入效果
- `WriteTextReport(w, plan)` — 生成文本报告

### `internal/packageio` — 包导入导出（1,054 行）

**职责**：把 agent/env 打包为 ZIP 归档，或从 ZIP 导入

- `ExportPackage(opts) → *ExportResult`
- `ImportPackage(opts) → *ImportResult`

### `internal/backup` — 备份（158 行）

**职责**：sync 写入前备份被管理的文件

- `BackupManagedPaths(runtime, paths, backupRoot, now) → *Snapshot`

### `internal/runtime` — 注册表（34 行）

**职责**：持有所有 adapter 实例，提供 `Get(runtime) → Adapter` 查询

---

## 数据流：从 `avm sync` 到文件写入

```
用户执行 avm sync
       │
       ▼
config.ResolveActivation(activeRef, cwd)
       │  合并 agent + env + override + memory + capabilities
       ▼
ResolvedActivation
       │
       ▼
sync.Syncer.SyncActivation(resolved, opts)
       │  遍历 resolved.Targets
       │
       ├─► adapter.Plan(input)     → RenderPlan（要写哪些文件、什么内容）
       │
       ├─► sync.DetectConflicts()  → 检查文件冲突
       │
       ├─► backup.BackupManagedPaths()  → 备份即将被覆盖的文件
       │
       ├─► adapter.Render(plan)    → 实际写入 runtime 配置文件
       │
       └─► state.SaveSyncState()  → 持久化同步状态
```

---

## CLI 命令一览（cmd/avm）

| 命令 | 作用 |
|------|------|
| `avm init` | 初始化 AVM 配置 |
| `avm agent` | 管理 agent profiles |
| `avm env` | 管理 environments |
| `avm use` | 切换 active agent/env |
| `avm sync` | 同步配置到 runtime |
| `avm status` | 查看当前状态 |
| `avm shell` | 进入 AVM shell |
| `avm memory` | 管理 portable memory |
| `avm export` | 导出 agent/env 为 ZIP 包 |
| `avm import` | 从 ZIP 包导入 |
| `avm deactivate` | 停用当前激活 |

---

## 设计模式总结

1. **Adapter 模式** — 核心。统一接口，多 runtime 实现。
2. **Functional Options** — 所有 adapter 的构造函数都用 `New(opts ...Option)` 模式。
3. **Plan-Render 两阶段** — 先生成计划（可 dry-run），再执行渲染。支持预览和冲突检测。
4. **分层依赖** — config 无依赖 → adapter 依赖 config → sync 依赖 adapter + state → CLI 依赖所有。
5. **接口隔离** — sync 层通过 `AdapterRegistry` 接口获取 adapter，不直接依赖具体实现。

---

## 需要关注的点

1. **adapter 实现的一致性** — 4 个 runtime adapter 是否都正确实现了 Plan/Render 逻辑？
2. **ResolveActivation 的合并逻辑** — 这是最复杂的函数，agent + env + override 的合并优先级是否正确？
3. **冲突检测** — 多个 runtime 管理同一文件时的处理是否完备？
4. **Cursor/Codex 的 Import** — 目前是 placeholder，是否有计划补全？
5. **测试覆盖** — fake adapter 存在，但集成测试是否覆盖了完整的 sync 流程？
