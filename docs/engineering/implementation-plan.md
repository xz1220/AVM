# Agent VM — Coding Implementation Plan

> 日期：2026-04-24
> 范围：Phase 1 MVP
> 目标：支持多个 coding Agent 并发实现，同时避免文件冲突和架构漂移

---

## 总结

当前 `main` 已完成 Round 0/1/2/3：Go scaffold、config model、adapter contract、CLI skeleton、Phase 1 fixtures、first vertical slice、activation pipeline、Codex first path 都已合并。下一阶段进入 **Stage 4 / Runtime Adapter Completion**，目标是补齐 Claude Code、Cline、Cursor PoC，并继续硬化 Codex adapter：

```text
avm use <profile|env>
  -> rebuild ~/.avm/active/**
  -> sync managed paths
  -> runtime adapters render owned config
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
| Round 3 P1-P3 | `DONE` | `origin/feat/sync-activation`、`origin/feat/cli-activation`、`origin/feat/codex-adapter` | `go test ./...`、`go vet ./...` |
| Round 3 P4 | `DONE` | CLI 接入真实 sync、Codex adapter 注册、临时 HOME/CODEX_HOME activation smoke | `go test ./...`、`go vet ./...`、CLI smoke |
| Stage 4 P0 | `DONE` | Runtime adapter completion 分工、prompts、registry ownership 边界 | `git diff --check` |
| Stage 4 P1-P4 | `DONE` | `origin/feat/claude-adapter`、`origin/feat/cline-adapter`、`origin/feat/cursor-adapter`、`origin/feat/codex-hardening` | `go test ./...`、`go vet ./...` |
| Stage 4 P5 | `DONE` | Runtime registry 接入 Claude Code / Cline / Cursor，multi-runtime activation smoke，sync warnings 去重 | `go test ./...`、`go vet ./...`、multi-runtime smoke |

Round 1 合并后的能力基线：

- Config 层已有 Phase 1 model、YAML read/write/list/validate、round-trip tests。
- Adapter 层已有 contract、mapping status、fake adapter、render plan normalization。
- CLI 层已有命令 skeleton，命令可稳定返回 `not implemented`。
- Fixtures 已有 Phase 1 minimal layout。
- First vertical slice 已可用：`avm init`、`avm agent create/list/show`、`avm env create`、`avm memory import --from <file> --dry-run`。

### 当前阶段：Acceptance Hardening

状态：Stage 4 runtime adapter completion 已合入 `main`。下一步进入 Stage 5，跑 acceptance、补 release-facing docs/examples，并修剩余边界问题。

当前 `avm use` 会重建 active、调用 sync、写 sync-state，并可通过 Codex / Claude Code / Cline / Cursor adapters 写入 AVM-managed config。Cursor 仍是 Phase 1 partial adapter，必须通过 warnings 和 mapping status 明确说明能力边界。

已锁定的 Round 3 边界：

- `adapter.RenderInputFromResolved` / `RenderInputsFromResolved` 负责把 `config.ResolvedActivation` 投影为 adapter 输入。
- `internal/sync` 已有 `Syncer.SyncActivation`、`Options`、`AdapterRegistry`、`Result`、`TargetResult`。
- `internal/state` 已有 `SyncState`、`RuntimeState`、managed path/mapping state contract 和 JSON store。
- fake adapter `Render` 会实际 apply `ensure_dir`、`write_file`、`remove_file`，并回填真实 `changed`。
- Codex adapter 已支持 structured managed block 和 role TOML whole-file render。
- Claude Code adapter 已支持 `.claude/agents/<agent>.md` 和 `.mcp.json` AVM-managed structured merge。
- Cline adapter 已支持 `.clinerules/avm/<agent>.md` 和 Cline MCP settings AVM-owned entries。
- Cursor adapter 已支持 `.cursor/rules/avm-<agent>.md` 和 `.cursor/mcp.json` partial PoC。

执行顺序：

1. `R3-P0 Lead prep` 已完成：adapter/config/sync/state 边界已落地。
2. `R3-P1 Sync Agent` 已完成：active rebuild、state、backup、conflict detection、fake adapter render。
3. `R3-P2 CLI Activation Agent` 已完成：`avm use`、`avm status`、`avm deactivate`。
4. `R3-P3 Codex Adapter Agent` 已完成：Codex first path。
5. `R3-P4 Lead integration` 已完成：合并分支、CLI 接真实 sync、注册 Codex adapter、跑 smoke。
6. `S4-P1/P2/P3/P4 Runtime Adapter Agents` 已完成：Claude Code / Cline / Cursor / Codex hardening。
7. `S4-P5 Lead integration` 已完成：合并 adapters、更新 runtime registry、跑 multi-runtime smoke。

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
2. Round 3 已完成，不再启动对应 Agent。
3. Stage 4 的每个子 Agent 必须从最新 `origin/main` 创建独立 worktree 和 branch。
4. Runtime adapters 只能在 `Adapter` + `config.ResolvedActivation` + `sync.Syncer` 边界内开发，不改公共 contract。
5. 每个子 Agent 必须使用独立 `git worktree` 和独立 branch，不允许多个 Agent 直接在同一个 worktree 并发写。
6. 任何 Agent 需要改 owner 之外的文件时，必须停止并在交付说明里声明，不要自行跨边界修改。
7. Stage 4 adapter agents 不修改 `internal/runtime/registry.go`；runtime registration 由 Lead 在 integration 时串行完成。

Stage 4 推荐 worktree 形态：

```bash
git fetch origin main
git worktree add ../agent-vm-claude-adapter -b feat/claude-adapter origin/main
git worktree add ../agent-vm-cline-adapter -b feat/cline-adapter origin/main
git worktree add ../agent-vm-cursor-adapter -b feat/cursor-adapter origin/main
git worktree add ../agent-vm-codex-hardening -b feat/codex-hardening origin/main
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

标记：`DONE`

可同时启动：

- `P4 Sync/state/backup`
- `P5 use/status/deactivate`
- `P7/P8/P9/P10 adapter implementations`

退出条件：

- `avm use backend-coder` 能重建 active。
- fake adapter 能写 managed path 并记录 mapping status。
- concrete adapter 至少 Codex 或 Claude Code 打通一个。

### Stage 4：Runtime Adapter Completion

标记：`DONE`

任务：

- Claude Code adapter
- Codex adapter
- Cline adapter
- Cursor PoC

退出条件：

- 每个 adapter 有 fixture。
- 每个 adapter 有 `Plan` 和 `Render` 单测。
- `avm status` 能显示 partial/ignored/unsupported。
- adapter agents 不改 `internal/runtime/registry.go`，Lead integration 时统一注册。

### Stage 5：Acceptance Hardening

标记：`NEXT`

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
| `internal/runtime/registry.go` | Lead Agent | Stage 4 adapters 合并后统一注册，避免并发冲突 |
| `cmd/avm/memory*.go` | Memory Agent | 不和 Agent CLI 混改；registration 由 Lead 集成 |
| `cmd/avm/init*.go`, `agent*.go`, `env*.go` | Agent CLI | init/agent/env create 已实现 |
| `cmd/avm/use*.go`, `status*.go`, `shell*.go`, `deactivate*.go` | CLI Agent | activation pipeline 已实现；Stage 4 adapters 不改 |
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
- `internal/runtime/registry.go`

如果必须改，先在任务说明里声明，等 owner 合并后再继续。

---

## Round 3 任务队列

```text
R3-P0 Lead prep (DONE)
  - Align adapter.RenderInput with config.ResolvedActivation / AgentProfile.
  - Lock sync input/output structs and fake adapter call path.

R3-P1 Sync Agent (DONE)
  - internal/sync/**, internal/state/**, internal/backup/**
  - active rebuild, state/current-active, sync-state.json, backups, conflict detection

R3-P2 CLI Activation Agent (DONE)
  - cmd/avm/use*.go, status*.go, deactivate*.go, tests
  - avm use, avm status, avm deactivate

R3-P3 Codex Adapter Agent (DONE)
  - internal/adapter/codex/**

Stage 4 Runtime Adapter Agents (DONE)
  - internal/adapter/claude/**
  - internal/adapter/cline/**
  - internal/adapter/cursor/**
  - internal/adapter/codex/** hardening only

R3-P4 Lead integration (DONE)
  - merge branches
  - run sync fake tests and Codex activation smoke
  - update this plan with Round 3 result

S4-P5 Lead integration (DONE)
  - merge Stage 4 adapter branches
  - update internal/runtime/registry.go
  - run multi-runtime activation smoke with temp HOME and runtime homes
```

### Round 3 archived prompts

Round 3 分工 prompt 已执行完毕，对应分支和结果记录在上方状态表。后续不要再启动这些旧 prompt。

### Stage 4 archived prompts

Stage 4 分工 prompt 已执行完毕，对应分支和结果记录在上方状态表。后续不要再启动这些旧 prompt。

### Stage 5 prep

下一阶段由 Lead 串行准备 acceptance checklist，再按缺口拆小任务。优先顺序：

- acceptance e2e 覆盖 `init -> agent/env/memory -> use/status/deactivate`。
- conflict detection 和 backup 行为验证。
- README/examples 与实际 CLI 输出对齐。
- fixture drift check：fixture render plan 与 adapter Plan 输出一致。
- runtime native memory 写入仍保持禁止。

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
