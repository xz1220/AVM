# Agent VM — Coding Implementation Plan

> 日期：2026-04-24
> 范围：Phase 1 MVP
> 目标：支持多个 coding Agent 并发实现，同时避免文件冲突和架构漂移

---

## 总结

当前 `main` 已完成 Round 0/1/2：Go scaffold、config model、adapter contract、CLI skeleton、Phase 1 fixtures、first vertical slice 都已合并。下一阶段进入 **Round 3 / Activation Pipeline**，目标是在 fake adapter 上打通：

```text
avm use <profile>
  -> rebuild ~/.avm/active/**
  -> fake adapter render managed paths
  -> avm status / deactivate
```

重要约束：**“推荐 Agent 分工”不是启动顺序**。启动顺序以 Stage/Round 为准；分工表只定义文件所有权和任务边界。任何公共接口、根命令、module 依赖、核心 struct 变更，都必须由对应 owner 串行落地后，其他 Agent 才能基于它继续。

Phase 1 的实现主线：

```text
Scaffold
  -> config/data model
  -> agent create/list/show
  -> memory import --dry-run
  -> use/sync render plan
  -> concrete runtime adapters
  -> acceptance fixtures
```

PRD 已明确：Phase 1 不做 `sync --watch`，不做 `workspace_isolation` 主字段，不静默同步 runtime native memory。

---

## 当前执行状态

### 已完成

| Round | 状态 | 合并内容 | 验收 |
|-------|------|----------|------|
| Round 0 | `DONE` | Go module、Makefile、CI 占位、root command、version 包、公共 package 占位目录 | `go test ./...`、`go run ./cmd/avm --help` |
| Round 1 | `DONE` | `origin/feat/config`、`origin/feat/adapter-contract`、`origin/feat/cli-skeleton`、`origin/feat/fixtures` | `go test ./...`、`go vet ./...`、关键 CLI help、fixture JSON/YAML 校验、路径扫描 |
| Round 2 | `DONE` | `origin/feat/config-resolve`、`origin/feat/agent-cli`、`origin/feat/memory-import` | `go test ./...`、`go vet ./...`、临时 HOME first vertical slice smoke |
| Round 3 P0 | `DONE` | adapter/config projection、sync/state contract、fake adapter apply render operations | `go test ./...`、`go vet ./...` |

Round 1 合并后的能力基线：

- Config 层已有 Phase 1 model、YAML read/write/list/validate、round-trip tests。
- Adapter 层已有 contract、mapping status、fake adapter、render plan normalization。
- CLI 层已有命令 skeleton，命令可稳定返回 `not implemented`。
- Fixtures 已有 Phase 1 minimal layout。
- First vertical slice 已可用：`avm init`、`avm agent create/list/show`、`avm env create`、`avm memory import --from <file> --dry-run`。

### 下一阶段：Round 3 Activation Pipeline

状态：`R3-P0 DONE`，可以启动 `R3-P1/R3-P2/R3-P3`。

Round 3 才开始写 runtime managed paths，但仍不做 `sync --watch`。先用 fake adapter 打通 active rebuild、state、backup、conflict detection，再并发 concrete runtime adapters。

Lead 已锁定的 R3-P0 边界：

- `adapter.RenderInputFromResolved` / `RenderInputsFromResolved` 负责把 `config.ResolvedActivation` 投影为 adapter 输入。
- `internal/sync` 已有 `Options`、`AdapterRegistry`、`Result`、`TargetResult` 最小 contract。
- `internal/state` 已有 `SyncState`、`RuntimeState`、managed path/mapping state contract。
- fake adapter `Render` 会实际 apply `ensure_dir`、`write_file`、`remove_file`，并回填真实 `changed`。
- `merge_section` / `structured_set` 暂未 apply，fake adapter 会显式报错，后续由 concrete adapter 或 sync 层定义更细语义。

执行顺序：

1. `R3-P0 Lead prep` 已完成：adapter/config/sync/state 边界已落地。
2. `R3-P1 Sync Agent` 并发：实现 active rebuild、state、backup、conflict detection、fake adapter render。
3. `R3-P2 CLI Activation Agent` 并发：实现 `avm use`、`avm status`、`avm deactivate`。
4. `R3-P3 Runtime Adapter Agents` 并发：先启动 Codex adapter，Claude/Cline/Cursor 等 fake e2e 稳定后再扩。
5. `R3-P4 Lead integration` 串行：合并后跑 fake adapter e2e，再决定是否进入 runtime completion。

Round 3 退出条件：

- `go test ./...` 和 `go vet ./...` 通过。
- `avm use backend-coder` 能重建 `~/.avm/active/**`。
- fake adapter 能写 managed path，并记录 mapping status。
- `avm status` 能显示 active、runtime result、ignored/unsupported/rendered mapping。
- `avm deactivate` 能回到 default active。
- concrete adapter 至少 Codex 或 Claude Code 打通一个 first path。

---

## 并发规则

### 启动规则

1. Round 0、Round 1、Round 2 已完成，不再启动对应 Agent。
2. Round 3 先由 Lead 做 `R3-P0` 串行准备，完成后再并发启动 Sync、CLI Activation、Runtime Adapter agents。
3. Round 3 的 Codex 必须从最新 `origin/main` 创建独立 worktree 和 branch。
4. Runtime adapters 只能在 `Adapter` + `config.ResolvedActivation` 边界锁定后启动。
5. 每个 Codex 必须使用独立 `git worktree` 和独立 branch，不允许多个 Codex 直接在同一个 worktree 并发写。
6. 任何 Agent 需要改 owner 之外的文件时，必须停止并在交付说明里声明，不要自行跨边界修改。

Round 3 推荐 worktree 形态：

```bash
git fetch origin main
git worktree add ../agent-vm-sync -b feat/sync-activation origin/main
git worktree add ../agent-vm-cli-activation -b feat/cli-activation origin/main
git worktree add ../agent-vm-codex-adapter -b feat/codex-adapter origin/main
git worktree add ../agent-vm-claude-adapter -b feat/claude-adapter origin/main
```

### 必须串行

这些任务会定义公共接口或全局结构，不能多人同时改：

| 标记 | 任务 | 原因 | Owner |
|------|------|------|-------|
| `S0` | Go module、目录结构、测试命令、CI 占位 | 所有后续任务依赖 | Lead Agent |
| `S1` | `internal/config` 核心 struct 命名 | adapter、sync、memory 都依赖 | Config Agent |
| `S2` | `internal/adapter` interface | concrete adapter 和 sync 都依赖 | Adapter Contract Agent |
| `S3` | CLI root command 和错误输出格式 | 所有命令共享 | CLI Agent |
| `S4` | integration 合并和最终验收 | 需要统一行为 | Lead Agent |

### 可以并发

这些任务有清晰文件边界，可以并行：

| 标记 | 任务 | 写入范围 | 依赖 |
|------|------|----------|------|
| `P1` | Config YAML 读写 + validation | `internal/config/**` | `DONE` |
| `P2` | Portable Memory import dry-run | `internal/memory/**`, `cmd/avm/memory*.go` | `DONE` |
| `P3` | Adapter contract + fake adapter | `internal/adapter/**` | `DONE` |
| `P4` | Sync/state/backup | `internal/sync/**`, `internal/state/**` | `S1`, `S2` |
| `P5` | CLI agent/env/status commands | `cmd/avm/**` | partial: init/agent/env done; use/status/deactivate next |
| `P6` | Runtime fixtures | `testdata/**`, `fixtures/**` | `DONE` |
| `P11` | Config ResolveActivation | `internal/config/resolve*.go`, `merge*.go`, tests | `DONE` |
| `P12` | Agent CLI implementation | `cmd/avm/init*.go`, `agent*.go`, `env*.go`, tests | `DONE` |
| `P7` | Claude Code adapter | `internal/adapter/claude/**` | `S2` |
| `P8` | Codex adapter | `internal/adapter/codex/**` | `S2` |
| `P9` | Cline adapter | `internal/adapter/cline/**` | `S2` |
| `P10` | Cursor PoC adapter | `internal/adapter/cursor/**` | `S2` |

---

## 推荐 Agent 分工

### Lead Agent

Owner:

- `go.mod`, `go.sum`
- `cmd/avm/root.go`
- `internal/version/**`
- `Makefile` 或 `justfile`
- integration merge

交付：

- `go test ./...` 可运行。
- `avm --help` 可运行。
- 所有公共 package 有空实现或接口占位。

不要做：

- 不实现具体 runtime adapter。
- 不把业务逻辑塞进 `cmd/avm`。

### Config Agent

Owner:

- `internal/config/**`
- `testdata/config/**`

交付：

- `GlobalConfig`
- `AgentProfile`
- `Environment`
- `PortableMemory`
- `Read/Write/List/Validate`
- `ResolveActivation`

验收：

- YAML round-trip 字段稳定。
- `workspace_isolation` 不存在于 `AgentProfile`。
- Environment 不接受 `capabilities` 或 `memory_layers`。

### Memory Agent

Owner:

- `internal/memory/**`
- `cmd/avm/memory*.go`
- `testdata/memory/**`

交付：

- `avm memory import --from <path|runtime> --dry-run`
- `MemoryImportPlan`
- `MemoryDiff`
- `memory-import-report.json`

验收：

- dry-run 不写 runtime 文件。
- dry-run 不写正式 `~/.avm/memory/` 文件。
- 输出 `new | changed | conflict | skipped`。

### Adapter Contract Agent

Owner:

- `internal/adapter/adapter.go`
- `internal/adapter/fake/**`
- `internal/adapter/renderplan/**`

交付：

- `Adapter` interface
- `RenderPlan`
- `FieldMapping`
- `ManagedPath`
- optional `MemoryImportCapable`
- fake adapter for sync tests

验收：

- mapping status 只允许 `native | rendered_as_instructions | ignored | unsupported`。
- fake adapter 能生成 deterministic render plan。

### Sync Agent

Owner:

- `internal/sync/**`
- `internal/state/**`
- `internal/backup/**`

交付：

- `RebuildActive`
- `SyncActivation`
- conflict detection
- backup
- `sync-state.json`

验收：

- active 重建失败不破坏旧 active。
- 只检测 `ManagedPaths`。
- 不实现 watch。

### CLI Agent

Owner:

- `cmd/avm/agent*.go`
- `cmd/avm/env*.go`
- `cmd/avm/use*.go`
- `cmd/avm/status*.go`
- `cmd/avm/shell*.go`

交付：

- `avm init`
- `avm agent create/list/show`
- `avm env create`
- `avm use`
- `avm status`
- `avm shell init`
- `avm deactivate`

验收：

- 命令输出稳定，适合 golden tests。
- 命令层只编排，不直接写 runtime config。

### Runtime Adapter Agents

并发启动条件：`Adapter` interface 和 `RenderInput` 已锁定。

| Agent | Owner | 交付 |
|------|-------|------|
| Claude Agent | `internal/adapter/claude/**` | `.claude/agents/*.md`, `.mcp.json`, memory refs 渲染 |
| Codex Agent | `internal/adapter/codex/**` | `config.toml`, role TOML, skills/memory instructions |
| Cline Agent | `internal/adapter/cline/**` | MCP settings, `.clinerules/avm/*.md` |
| Cursor Agent | `internal/adapter/cursor/**` | `.cursor/mcp.json`, rules PoC, partial status |

共同验收：

- 不覆盖用户 instruction 文件。
- 不展开 `${ENV_VAR}`。
- unsupported/ignored/rendered 都写入 mapping status。

---

## 执行阶段

### Stage 0：Scaffold 串行

标记：`DONE`

任务：

1. 初始化 Go module。
2. 建目录：

```text
cmd/avm/
internal/config/
internal/adapter/
internal/memory/
internal/sync/
internal/state/
internal/backup/
internal/runtime/
testdata/
```

3. 建 `README` 里的本地开发命令。
4. 建最小 `go test ./...`。

退出条件：

- `go test ./...` 通过。
- `go run ./cmd/avm --help` 有输出。

### Stage 1：Core Contract 并发

标记：`DONE`

可同时启动：

- `P1 Config`
- `P3 Adapter Contract`
- `P6 Runtime Fixtures`
- `P5 CLI skeleton`

退出条件：

- config structs 可编译。
- adapter fake 可编译。
- CLI commands 可注册但允许返回 `not implemented`。

### Stage 2：First Vertical Slice

标记：`DONE`

目标：先打通一个非 runtime 写入路径。

任务：

1. `avm init`
2. `avm agent create/list/show`
3. `avm memory import --from <file> --dry-run`

退出条件：

- 能在临时 HOME 下跑 e2e。
- 不写任何 runtime 配置文件。
- `memory import --dry-run` 有 diff 输出。

### Stage 3：Activation Pipeline

标记：`IN_PROGRESS`

可同时启动：

- `P4 Sync/state/backup`
- `P5 use/status/deactivate`
- `P7/P8/P9/P10 adapter implementations`

退出条件：

- `avm use backend-coder` 能重建 active。
- fake adapter 能写 managed path 并记录 mapping status。
- concrete adapter 至少 Codex 或 Claude Code 打通一个。

### Stage 4：Runtime Adapter Completion

标记：`PARALLEL`

任务：

- Claude Code adapter
- Codex adapter
- Cline adapter
- Cursor PoC

退出条件：

- 每个 adapter 有 fixture。
- 每个 adapter 有 `Plan` 和 `Render` 单测。
- `avm status` 能显示 partial/ignored/unsupported。

### Stage 5：Acceptance Hardening

标记：`SERIAL_JOIN`

任务：

1. 按 [acceptance.md](./acceptance.md) 跑全量 e2e。
2. 修冲突检测、路径权限、export/import 边界。
3. 补 README 和示例。

退出条件：

- `go test ./...` 通过。
- Phase 1 acceptance 核心路径通过。
- 没有 silent field drop。

---

## 文件所有权

并发时按 owner 控制写入：

| 路径 | Owner | 备注 |
|------|-------|------|
| `go.mod`, `go.sum` | Lead Agent | 其他 Agent 需要依赖时先记录，合并时统一添加 |
| `cmd/avm/root.go` | Lead Agent | 不并发改 |
| `cmd/avm/commands.go` | Lead Agent | 用于集中注册，避免 Agent 冲突 |
| `cmd/avm/memory*.go` | Memory Agent | 不和 Agent CLI 混改；registration 由 Lead 集成 |
| `cmd/avm/init*.go`, `agent*.go`, `env*.go` | Agent CLI | init/agent/env create 已实现 |
| `cmd/avm/use*.go`, `status*.go`, `shell*.go`, `deactivate*.go` | CLI Agent | Round 3 再实现 use/status/deactivate 行为 |
| `internal/config/**` | Config Agent | 其他 Agent 不改 config struct |
| `internal/adapter/adapter.go` | Adapter Contract Agent | concrete adapter 不改 interface |
| `internal/adapter/<runtime>/**` | 对应 Runtime Agent | 每个 runtime 独立 |
| `internal/memory/**` | Memory Agent | 包含 import dry-run 和 diff |
| `internal/sync/**` | Sync Agent | 不改 adapter |
| `internal/state/**` | Sync Agent | memory report 可由 Memory Agent 加独立文件 |
| `testdata/<area>/**` | 对应 Agent | fixture 按模块拆分 |

---

## 禁止并发修改

以下文件需要单 owner 串行合并：

- `go.mod`
- `go.sum`
- `cmd/avm/root.go`
- `cmd/avm/commands.go`
- `internal/config/models.go`
- `internal/adapter/adapter.go`
- `internal/config/resolve.go`
- `internal/sync/syncer.go`

如果必须改，先在任务说明里声明，等 owner 合并后再继续。

---

## Round 3 任务队列

```text
R3-P0 Lead prep (DONE)
  - Align adapter.RenderInput with config.ResolvedActivation / AgentProfile.
  - Lock sync input/output structs and fake adapter call path.

R3-P1 Sync Agent (PARALLEL after R3-P0)
  - internal/sync/**, internal/state/**, internal/backup/**
  - active rebuild, state/current-active, sync-state.json, backups, conflict detection

R3-P2 CLI Activation Agent (PARALLEL after R3-P0)
  - cmd/avm/use*.go, status*.go, deactivate*.go, tests
  - avm use, avm status, avm deactivate

R3-P3 Runtime Adapter Agents (PARALLEL after R3-P0)
  - internal/adapter/codex/**
  - internal/adapter/claude/**
  - internal/adapter/cline/**
  - internal/adapter/cursor/**

R3-P4 Lead integration (SERIAL)
  - merge branches
  - run fake adapter activation e2e
  - update this plan with Round 3 result
```

### Round 3 Agent prompts

#### R3-P1 Sync Agent prompt

```text
你是 Agent VM 的 R3-P1 Sync Agent。请从最新 origin/main 创建独立 worktree 后开发、提交、推送。

准备：
cd /Users/danielxing/code/agent-vm
git fetch origin main
git worktree add ../agent-vm-sync -b feat/sync-activation origin/main
cd ../agent-vm-sync

Owner：
- internal/sync/**
- internal/state/**
- internal/backup/**
- 可新增对应 tests/testdata

不要修改：
- go.mod/go.sum
- cmd/avm/**
- internal/config/models.go、internal/config/resolve.go
- internal/adapter/adapter.go
- concrete runtime adapter 目录

任务：
- 实现 sync activation 主流程，使用 adapter.RenderInputsFromResolved(resolved, opts) 作为 config 到 adapter 的唯一入口。
- 通过 AdapterRegistry 获取 adapter；missing runtime 要返回 skipped/failed target result，不要 panic。
- 对每个可用 runtime 调用 Plan，再按 managed path 做冲突检测/backup，最后 Render。
- 实现 active rebuild 到 ~/.avm/active/**，失败时不能破坏旧 active。
- 实现 sync-state.json 的读写，沿用 internal/state 的 SyncState/RuntimeState contract。
- 支持 dry-run：生成计划和状态结果，但不写 active、不写 runtime managed paths。
- 只处理 ManagedPaths，不做 sync --watch，不做 runtime native memory 写入。

验收：
- go test ./...
- go vet ./...
- 单测覆盖 fake adapter render 成功、dry-run 不写文件、冲突检测、active rebuild 失败回滚或保留旧 active、sync-state round-trip。
- 提交前 git status --short 只能包含 Owner 范围文件。

完成后：
git push -u origin feat/sync-activation
git status --short
cd /Users/danielxing/code/agent-vm
git worktree remove ../agent-vm-sync
回复修改文件、测试结果、commit hash、远程分支，以及仍需 Lead/CLI 接入的点。
```

#### R3-P2 CLI Activation Agent prompt

```text
你是 Agent VM 的 R3-P2 CLI Activation Agent。请从最新 origin/main 创建独立 worktree 后开发、提交、推送。

准备：
cd /Users/danielxing/code/agent-vm
git fetch origin main
git worktree add ../agent-vm-cli-activation -b feat/cli-activation origin/main
cd ../agent-vm-cli-activation

Owner：
- cmd/avm/use*.go
- cmd/avm/status*.go
- cmd/avm/deactivate*.go
- cmd/avm/shell*.go（只在必须接 activation 状态时改）
- cmd/avm/*activation*_test.go 或相关 command tests

不要修改：
- go.mod/go.sum
- cmd/avm/root.go、cmd/avm/commands.go
- internal/config 核心模型
- internal/adapter contract
- internal/sync/internal/state 实现文件；如果 sync API 不够，先记录需求，不要跨 owner 造接口

任务：
- 实现 avm use <profile-or-env>，支持 --kind profile|env；未指定 kind 时先尝试 profile，再尝试 env，错误信息要稳定。
- use 调用 config.ResolveActivation，再调用 sync 层 API；如果 sync API 暂未合入，保留最小编排 helper 和清晰 TODO，保证可编译。
- 实现 avm status，展示 active、runtime status、managed paths、mapping status、warnings。
- 实现 avm deactivate，回到 default active，并调用 sync 或至少更新 config active，行为需和 sync 接口兼容。
- 命令层只编排，不直接写 runtime config。

验收：
- go test ./...
- go vet ./...
- 临时 HOME 下覆盖 avm use/status/deactivate 的 command tests。
- 输出稳定，适合 golden tests。
- 提交前 git status --short 只能包含 Owner 范围文件。

完成后：
git push -u origin feat/cli-activation
git status --short
cd /Users/danielxing/code/agent-vm
git worktree remove ../agent-vm-cli-activation
回复修改文件、测试结果、commit hash、远程分支，以及等待 sync API 合并的点。
```

#### R3-P3 Codex Adapter Agent prompt

```text
你是 Agent VM 的 R3-P3 Codex Adapter Agent。请从最新 origin/main 创建独立 worktree 后开发、提交、推送。本轮只做 Codex adapter，不做 Claude/Cline/Cursor。

准备：
cd /Users/danielxing/code/agent-vm
git fetch origin main
git worktree add ../agent-vm-codex-adapter -b feat/codex-adapter origin/main
cd ../agent-vm-codex-adapter

Owner：
- internal/adapter/codex/**
- fixtures/phase1/codex/** 或 testdata/adapter/codex/**
- 可新增 codex adapter tests

不要修改：
- go.mod/go.sum
- internal/adapter/adapter.go
- internal/config/**
- cmd/avm/**
- sync/state/backup

任务：
- 实现 Codex adapter 的 Plan/Render/ManagedPaths/Detect 最小可用路径。
- Plan 从 adapter.RenderInput 生成 deterministic RenderPlan，至少覆盖 agent instructions、model/reasoning、permissions、skills/memory refs 的渲染策略。
- Render 只写 adapter 声明的 ManagedPaths，不能覆盖用户未声明路径；不能展开 ${ENV_VAR}。
- 对不能原生表达的字段写 FieldMapping：rendered_as_instructions / ignored / unsupported，不能 silent drop。
- 保持 Phase 1 保守：先打通一个 Codex first path，不引入额外依赖。

验收：
- go test ./...
- go vet ./...
- Plan/Render 单测覆盖 deterministic plan、managed path 写入、mapping status、不会触碰未声明路径。
- fixture 与 fixtures/phase1 convention 对齐。
- 提交前 git status --short 只能包含 Owner 范围文件。

完成后：
git push -u origin feat/codex-adapter
git status --short
cd /Users/danielxing/code/agent-vm
git worktree remove ../agent-vm-codex-adapter
回复修改文件、测试结果、commit hash、远程分支，以及 partial/unsupported mapping 清单。
```

---

## 成功标准

Phase 1 coding 完成时至少满足：

1. `avm init` 只写 `~/.avm/**`。
2. `avm agent create/list/show` 可用。
3. `avm memory import --dry-run` 可用，且不写 runtime native memory。
4. `avm use <profile>` 可重建 active 并调用 adapter render。
5. `avm env create/use` 可按 runtime 选择不同 Agent Profile。
6. Codex、Claude Code、Cline 至少有 fixture 覆盖。
7. Cursor 明确标记 partial。
8. `workspace_isolation` 不存在于 Agent Profile 主模型。
9. mapping status 无 silent drop。
10. `go test ./...` 通过。
