# Agent VM — Coding Implementation Plan

> 日期：2026-04-26
> 范围：Phase 1 MVP
> 目标：支持多个 coding Agent 并发实现，同时避免文件冲突和架构漂移

---

## 总结

当前 `main` 已完成 Round 0/1/2/3、Stage 4、Stage 5：Go scaffold、config model、adapter contract、CLI skeleton、Phase 1 fixtures、first vertical slice、activation pipeline、runtime adapters、acceptance hardening、env local override、sync CLI、shell hook、portable package export/import 都已合并。下一阶段进入 **Stage 6 / Acceptance Polish**，重点是补齐仍未关闭的验收边界，而不是继续扩展 Phase 1 数据模型：

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
| Stage 5 P0 | `DONE` | acceptance smoke、缺口报告、Stage 5 任务拆分 | `go run ./cmd/avm --help`、临时 HOME multi-runtime smoke |
| Stage 5 P1-P4 | `DONE` | `origin/feat/acceptance-harness`、`origin/feat/cli-hardening`、`origin/feat/env-hardening`、`origin/feat/package-io` | 各分支 `go test ./...`、`go vet ./...` |
| Stage 5 P5 | `DONE` | Stage 5 branch integration、acceptance harness 更新、gap report 更新 | `go test ./...`、`go vet ./...`、`git diff --check` |
| Stage 6 P0 | `DONE` | Acceptance polish 边界决策、prompts、Cursor 状态语义收敛 | `git diff --check` |
| Stage 6 P1-P3 | `DONE` | `origin/feat/mapping-preview`、`origin/feat/init-import-report`、`origin/feat/docs-polish` | 各分支 `go test ./...`、`go vet ./...` 或 `git diff --check` |
| Stage 6 P4 | `DONE` | Stage 6 branch integration、README/acceptance 状态修正 | `go test ./...`、`go vet ./...`、CLI smoke |
| Stage 7 P0 | `DONE` | OpenCode adapter、OpenCode activation env、README/docs 对齐 | `go test ./...`、`go vet ./...`、OpenCode first-run smoke |

Round 1 合并后的能力基线：

- Config 层已有 Phase 1 model、YAML read/write/list/validate、round-trip tests。
- Adapter 层已有 contract、mapping status、fake adapter、render plan normalization。
- CLI 层已有命令 skeleton，命令可稳定返回 `not implemented`。
- Fixtures 已有 Phase 1 minimal layout。
- First vertical slice 已可用：`avm init`、`avm agent create/list/show`、`avm env create`、`avm memory import --from <file> --dry-run`。

### 当前阶段：Post Stage 6 Follow-up

状态：`S6-P4 DONE`。`agent show --runtime` mapping preview、`init` read-only runtime import report、README/examples/docs 对齐都已合并。Cursor Phase 1 状态语义已定：成功写入时保持 `synced`，partial 能力边界必须通过 warnings 和 mapping status 明确展示。

当前 `avm use` 会重建 active、调用 sync、写 sync-state，并可通过 Codex / Claude Code / OpenCode / Cline / Cursor adapters 写入 AVM-managed config。Cursor 仍是 Phase 1 partial adapter，必须通过 warnings 和 mapping status 明确说明能力边界。

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
8. `S5-P0 Lead acceptance scan` 已完成：跑 smoke、确认 gaps、更新 Stage 5 分工。
9. `S5-P1/P2/P3/P4 Stage 5 Agents` 已完成：acceptance harness、CLI hardening、env hardening、package IO。
10. `S5-P5 Lead integration` 已完成：合并 Stage 5 分支、更新 acceptance harness 和 gap report、跑整体验证。
11. `S6-P0 Lead prep` 已完成：锁定 Cursor 状态语义、拆分 Stage 6 owner 和 prompt。
12. `S6-P1/P2/P3 Stage 6 Agents` 已完成：mapping preview、init import report、docs polish。
13. `S6-P4 Lead integration` 已完成：合并 Stage 6 分支、更新 README/acceptance 状态。

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
3. Stage 4 已完成，不再启动对应 Agent。
4. Stage 5 已完成，不再启动对应 Agent。
5. Stage 6 已完成，不再启动对应 Agent。
6. 每个子 Agent 必须使用独立 `git worktree` 和独立 branch，不允许多个 Agent 直接在同一个 worktree 并发写。
7. 任何 Agent 需要改 owner 之外的文件时，必须停止并在交付说明里声明，不要自行跨边界修改。
8. 后续任务优先修复 [stage5-acceptance-gap-report.md](./stage5-acceptance-gap-report.md) 中的 remaining follow-up，不扩大到 `sync --watch`、workspace isolation 或 runtime-native memory 写入。

Stage 5 已执行 worktree 形态（归档，不再启动）：

```bash
git fetch origin main
git worktree add ../agent-vm-acceptance -b feat/acceptance-harness origin/main
git worktree add ../agent-vm-cli-hardening -b feat/cli-hardening origin/main
git worktree add ../agent-vm-env-hardening -b feat/env-hardening origin/main
git worktree add ../agent-vm-package-io -b feat/package-io origin/main
```

Stage 6 已执行 worktree 形态（归档，不再启动）：

```bash
git fetch origin main
git worktree add ../agent-vm-mapping-preview -b feat/mapping-preview origin/main
git worktree add ../agent-vm-init-report -b feat/init-import-report origin/main
git worktree add ../agent-vm-docs-polish -b feat/docs-polish origin/main
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
| `P5` | CLI agent/env/status commands | `cmd/avm/**` | `DONE` |
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
| Cursor Agent | `internal/adapter/cursor/**` | `.cursor/mcp.json`, rules PoC, partial support warnings |

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
- `avm status` 能显示 warnings 和 ignored/unsupported/rendered mapping status。
- adapter agents 不改 `internal/runtime/registry.go`，Lead integration 时统一注册。

### Stage 5：Acceptance Hardening

标记：`DONE`

任务：

1. 按 [acceptance.md](./acceptance.md) 和 [stage5-acceptance-gap-report.md](./stage5-acceptance-gap-report.md) 跑全量 e2e。
2. 修 `init --force`、initial state/cache、shell init、sync command、env local、reference validation、export/import 等 verified gaps。
3. 补 automated acceptance tests，再更新 README/examples。

退出条件：

- `go test ./...` 通过。
- Phase 1 acceptance 核心路径通过。
- 没有 silent field drop。
- acceptance gap report 中的 Phase 1 blocking gaps 已关闭或明确降级到 post-MVP。

### Stage 6：Acceptance Polish

标记：`DONE`

任务：

1. `agent show --runtime` 使用 adapter `Plan` 输出 mapping preview，展示 native、rendered_as_instructions、ignored、unsupported。
2. `avm init` 增加 read-only runtime scan report，写 `~/.avm/state/import-report.json`，不写 runtime 配置、不自动激活导入对象。
3. README/examples/acceptance docs 与当前 CLI 对齐，明确 Cursor Phase 1 是 `synced` + warnings/mapping status，而不是独立 `partial` sync 状态。

退出条件：

- `go test ./...` 和 `go vet ./...` 通过。
- `avm agent show <name> --runtime <runtime>` 可在临时 HOME/project 下稳定输出 mapping preview。
- `avm init` 可在存在 runtime fixture 时生成 import report，且 runtime 文件 hash 不变。
- README/examples 不再引用未实现或已变更的 Stage 5 行为。

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

S5-P0 Lead acceptance scan (DONE)
  - run current executable smoke
  - record verified gaps in stage5-acceptance-gap-report.md
  - prepare Stage 5 work slices

S5-P1 Acceptance Harness Agent (DONE)
  - cmd/avm/*acceptance*_test.go, testdata/acceptance/**
  - encode passing smoke and selected negative cases

S5-P2 CLI Hardening Agent (DONE)
  - cmd/avm/init*.go, shell*.go, sync*.go, tests
  - init --force, initial state/cache, shell init, avm sync

S5-P3 Env Hardening Agent (DONE)
  - cmd/avm/env*.go, internal/config env helpers if needed, tests
  - env reference validation and project-local env override

S5-P4 Package IO Agent (DONE)
  - cmd/avm/export*.go, import*.go, internal/package/**, tests
  - portable .avm.zip export/import for Phase 1 objects

S5-P5 Lead integration (DONE)
  - merge Stage 5 branches
  - rerun acceptance smoke
  - decide remaining post-MVP gaps
```

### Round 3 archived prompts

Round 3 分工 prompt 已执行完毕，对应分支和结果记录在上方状态表。后续不要再启动这些旧 prompt。

### Stage 4 archived prompts

Stage 4 分工 prompt 已执行完毕，对应分支和结果记录在上方状态表。后续不要再启动这些旧 prompt。

### Stage 5 result

Stage 5 已完成合并。已关闭：

- acceptance e2e 覆盖 `init -> agent/env/memory -> use/status/deactivate`。
- `init --force`、初始 `cache/` 和 `state/sync-state.json`。
- `shell init`、`sync`、`env create --local`、env profile reference validation。
- Phase 1 portable package `export` / `import`。
- runtime native memory 写入仍保持禁止。

仍保留为后续任务：

- package scope 是否扩展到 config/active/project override/state/backup/cache/runtime-native memory。

### Stage 6 archived prompts

以下 prompt 是历史记录，不要重新执行。对应分支和结果记录在上方状态表。

#### S6-P1 Mapping Preview prompt

```text
你是 Agent VM 的 S6-P1 Mapping Preview Agent。请从最新 origin/main 创建独立 worktree 后开发、提交、推送，完成后删除自己的 worktree。

准备：
cd /Users/danielxing/code/agent-vm
git fetch origin main
git worktree add ../agent-vm-mapping-preview -b feat/mapping-preview origin/main
cd ../agent-vm-mapping-preview

Owner：
- cmd/avm/agent*.go
- cmd/avm/*agent*_test.go
- cmd/avm/*mapping*_test.go only if needed

不要修改：
- go.mod/go.sum
- internal/adapter/** contract 或 concrete adapters
- internal/config core model
- internal/sync/**
- internal/runtime/registry.go
- README / localized README / docs

任务：
- 实现 `avm agent show <name> --runtime <runtime>` 的 mapping preview；未传 `--runtime` 时继续输出现有 YAML。
- 读取 profile 后构造只包含该 runtime 的 resolved activation/input，调用对应 runtime adapter `Plan`，不要调用 `Render`，不要写 runtime 文件。
- 输出必须稳定、可测，包含 runtime、agent、managed paths、warnings、以及 mapping groups：native、rendered_as_instructions、ignored、unsupported。
- Cursor Phase 1 不引入独立 partial sync status；preview 中必须展示 unsupported/rendered/ignored 边界。
- invalid runtime、missing profile、adapter plan error 要返回稳定错误。

验收：
- go test ./...
- go vet ./...
- 临时 HOME/project 测试覆盖：未传 --runtime 仍输出 YAML；codex preview 有 native/rendered/unsupported；cursor preview 有 unsupported；preview 不创建 runtime managed files。
- 提交前 git status --short --untracked-files=all 只能包含 Owner 范围文件。

完成后：
git push -u origin feat/mapping-preview
git status --short
cd /Users/danielxing/code/agent-vm
git worktree remove ../agent-vm-mapping-preview

回复修改文件、测试结果、commit hash、远程分支，以及仍未覆盖的 mapping preview 项。
```

#### S6-P2 Init Import Report prompt

```text
你是 Agent VM 的 S6-P2 Init Import Report Agent。请从最新 origin/main 创建独立 worktree 后开发、提交、推送，完成后删除自己的 worktree。

准备：
cd /Users/danielxing/code/agent-vm
git fetch origin main
git worktree add ../agent-vm-init-report -b feat/init-import-report origin/main
cd ../agent-vm-init-report

Owner：
- cmd/avm/init*.go
- cmd/avm/*init*_test.go
- internal/state/*import* only if needed
- testdata/init/** only if needed

不要修改：
- go.mod/go.sum
- internal/adapter/** implementation
- internal/config core model
- internal/runtime/registry.go
- README / localized README / docs

任务：
- `avm init` 完成默认 config/agent/env/sync-state 后，read-only 扫描已注册 runtime adapters：调用 Detect 和 Import。
- 写 `~/.avm/state/import-report.json`，结构稳定，至少包含 version、generated_at、runtimes[]、found/config_dir/version、agents candidates、warnings/errors。
- 扫描只能读 runtime 文件，不得创建/修改/删除 Codex/Claude/OpenCode/Cline/Cursor runtime 配置；不得自动 `avm use`；不得把 candidate 直接写入 agents/envs。
- adapter Import 返回错误时记录到 report，不让整个 init 失败，除非 report 文件本身写入失败。
- `init --force` 也应刷新 report；已有用户额外 `~/.avm/**` 文件不得删除。

验收：
- go test ./...
- go vet ./...
- 临时 HOME/project/runtime fixture 测试覆盖：report 生成、runtime 文件 hash 不变、Import error 被记录、init --force 刷新 report。
- 提交前 git status --short --untracked-files=all 只能包含 Owner 范围文件。

完成后：
git push -u origin feat/init-import-report
git status --short
cd /Users/danielxing/code/agent-vm
git worktree remove ../agent-vm-init-report

回复修改文件、测试结果、commit hash、远程分支，以及 init import/report 仍未覆盖的 runtime。
```

#### S6-P3 Docs Polish prompt

```text
你是 Agent VM 的 S6-P3 Docs Polish Agent。请从最新 origin/main 创建独立 worktree 后开发、提交、推送，完成后删除自己的 worktree。

准备：
cd /Users/danielxing/code/agent-vm
git fetch origin main
git worktree add ../agent-vm-docs-polish -b feat/docs-polish origin/main
cd ../agent-vm-docs-polish

Owner：
- README.md
- README.zh-CN.md
- docs/README.md
- docs/engineering/acceptance.md
- docs/engineering/stage5-acceptance-gap-report.md
- docs/engineering/fixture-conventions.md only if needed

不要修改：
- go.mod/go.sum
- cmd/**
- internal/**
- fixtures/**
- testdata/**
- docs/engineering/implementation-plan.md

任务：
- README/examples 对齐当前 CLI：init、agent create/list/show、env create/use、env create --local、memory import --dry-run、use/status/deactivate、sync、shell init、export/import。
- 明确 Cursor Phase 1 语义：成功写入时 status 保持 `synced`；partial support 通过 warnings 和 mapping status 暴露。
- 标注 runtime import-report 和 `agent show --runtime` 如果相关分支尚未合入，则属于 Stage 6 in-progress；不要把未合入能力写成已发布。
- 删除或改写 Stage 5 之前的过时 not implemented 文案。
- 保持中英文 README 信息一致，避免引入营销型内容。

验收：
- git diff --check
- go test ./...（文档改动也跑一遍，防止示例相关测试漂移）
- 搜索确认 docs/README 中没有把 Cursor 要求为 standalone partial status 的旧说法。
- 提交前 git status --short --untracked-files=all 只能包含 Owner 范围文件。

完成后：
git push -u origin feat/docs-polish
git status --short
cd /Users/danielxing/code/agent-vm
git worktree remove ../agent-vm-docs-polish

回复修改文件、测试结果、commit hash、远程分支，以及仍需 Lead 在合并后更新的 docs 项。
```

### Stage 5 archived prompts

以下 prompt 是历史记录，不要重新执行。S5-P1 中原本用于 gap tracking 的 negative tests 已在 Lead integration 中按最终实现转成正向 acceptance 覆盖。

#### S5-P1 Acceptance Harness prompt

```text
你是 Agent VM 的 S5-P1 Acceptance Harness Agent。请从最新 origin/main 创建独立 worktree 后开发、提交、推送，完成后删除自己的 worktree。

准备：
cd /Users/danielxing/code/agent-vm
git fetch origin main
git worktree add ../agent-vm-acceptance -b feat/acceptance-harness origin/main
cd ../agent-vm-acceptance

Owner：
- cmd/avm/*acceptance*_test.go
- testdata/acceptance/**
- docs/engineering/stage5-acceptance-gap-report.md（只允许补充测试覆盖结果，不重写结论）

不要修改：
- go.mod/go.sum
- runtime adapter implementation
- internal/config core model
- internal/sync behavior
- README / localized README

任务：
- 把当前通过的 smoke 固化成 Go acceptance tests：临时 HOME、项目目录、CODEX_HOME、CLAUDE_CONFIG_DIR、CLINE_DATA_HOME 下跑 init/agent/env/memory/use/status/deactivate。
- 覆盖 multi-runtime env：codex、claude-code、cline、cursor 都进入 sync result。
- 断言 init 不写项目 .avm，不写 runtime config；use 后才写 adapter managed paths。
- 断言 memory import --dry-run 不写正式 ~/.avm/memory/** 文件。
- 增加 selected negative tests：env --local 未实现、shell init 未实现、sync/export/import absent 或未实现，作为 Stage 5 gap tracking；如果其他分支已实现，对应测试应按新行为调整。

验收：
- go test ./...
- go vet ./...
- 测试必须不依赖本机真实 runtime 配置或真实 HOME。
- 提交前 git status --short --untracked-files=all 只能包含 Owner 范围文件。

完成后：
git push -u origin feat/acceptance-harness
git status --short
cd /Users/danielxing/code/agent-vm
git worktree remove ../agent-vm-acceptance

回复修改文件、测试结果、commit hash、远程分支，以及仍缺少自动覆盖的 acceptance 项。
```

#### S5-P2 CLI Hardening prompt

```text
你是 Agent VM 的 S5-P2 CLI Hardening Agent。请从最新 origin/main 创建独立 worktree 后开发、提交、推送，完成后删除自己的 worktree。

准备：
cd /Users/danielxing/code/agent-vm
git fetch origin main
git worktree add ../agent-vm-cli-hardening -b feat/cli-hardening origin/main
cd ../agent-vm-cli-hardening

Owner：
- cmd/avm/init*.go
- cmd/avm/shell*.go
- cmd/avm/sync*.go
- cmd/avm/commands.go（只允许注册 sync command）
- cmd/avm/*init*_test.go, *shell*_test.go, *sync*_test.go
- internal/state/** only if needed for init state creation

不要修改：
- go.mod/go.sum
- internal/adapter/**
- internal/config core model
- internal/runtime/registry.go
- README / localized README

任务：
- 实现 avm init --force：已有 ~/.avm/config.yaml 时默认报错，--force 才允许重建默认 config/agent/env；不得删除用户额外文件。
- init 创建 acceptance 要求的基础目录，包括 cache/，并创建初始 state/sync-state.json。
- 实现 avm shell init zsh/bash/fish，输出 eval-safe shell snippet，只读取 ~/.avm/state/current-active 或等价状态，不写 runtime 文件。
- 增加 avm sync：读取当前 config active，ResolveActivation 后调用 sync.Syncer；不改变 active selection；遇到 conflict 返回非 0，并保留 status 可见。
- 保持命令输出稳定，适合 golden tests。

验收：
- go test ./...
- go vet ./...
- 临时 HOME 测试覆盖 init repeat without --force、init --force、shell init zsh/bash/fish、sync current active。
- 提交前 git status --short --untracked-files=all 只能包含 Owner 范围文件。

完成后：
git push -u origin feat/cli-hardening
git status --short
cd /Users/danielxing/code/agent-vm
git worktree remove ../agent-vm-cli-hardening

回复修改文件、测试结果、commit hash、远程分支，以及仍未关闭的 CLI acceptance gaps。
```

#### S5-P3 Env Hardening prompt

```text
你是 Agent VM 的 S5-P3 Env Hardening Agent。请从最新 origin/main 创建独立 worktree 后开发、提交、推送，完成后删除自己的 worktree。

准备：
cd /Users/danielxing/code/agent-vm
git fetch origin main
git worktree add ../agent-vm-env-hardening -b feat/env-hardening origin/main
cd ../agent-vm-env-hardening

Owner：
- cmd/avm/env*.go
- cmd/avm/*env*_test.go
- internal/config/env*.go, internal/config/paths.go, internal/config/merge.go only if needed for project-local env write helpers
- testdata/config/** only if needed

不要修改：
- go.mod/go.sum
- internal/adapter/**
- internal/sync/**
- internal/runtime/registry.go
- README / localized README

任务：
- 实现 avm env create --local：写当前项目 .avm/env.yaml，extends 指向当前 active env 或指定全局 env；不影响 ~/.avm/envs/**。
- env create 必须校验每个 runtime 绑定的 agent profile 存在，优先项目 .avm/agents，再全局 ~/.avm/agents。
- Environment YAML 仍不得接受 capabilities 或 memory_layers。
- 保持 ResolveActivation 对 project override 的既有优先级。

验收：
- go test ./...
- go vet ./...
- 测试覆盖 global env create 引用缺失失败、project-local env override 写入、ResolveActivation 应用 local override、env 禁止 capabilities。
- 提交前 git status --short --untracked-files=all 只能包含 Owner 范围文件。

完成后：
git push -u origin feat/env-hardening
git status --short
cd /Users/danielxing/code/agent-vm
git worktree remove ../agent-vm-env-hardening

回复修改文件、测试结果、commit hash、远程分支，以及仍未关闭的 env acceptance gaps。
```

#### S5-P4 Package IO prompt

```text
你是 Agent VM 的 S5-P4 Package IO Agent。请从最新 origin/main 创建独立 worktree 后开发、提交、推送，完成后删除自己的 worktree。

准备：
cd /Users/danielxing/code/agent-vm
git fetch origin main
git worktree add ../agent-vm-package-io -b feat/package-io origin/main
cd ../agent-vm-package-io

Owner：
- cmd/avm/export*.go
- cmd/avm/import*.go
- cmd/avm/commands.go（只允许注册 export/import commands）
- cmd/avm/*package*_test.go 或 export/import tests
- internal/packageio/**
- testdata/packageio/**

不要修改：
- go.mod/go.sum
- internal/adapter/**
- internal/sync/**
- internal/runtime/registry.go
- README / localized README

任务：
- 实现 avm export <agent-or-env> --output <file.avm.zip>。
- 实现 avm import <file.avm.zip>。
- 使用标准库 archive/zip，不新增依赖。
- 包含 manifest.yaml、agent YAML 或 env YAML 及引用 agent YAML、referenced memory/capability metadata when present。
- 默认不包含 runtime 输出文件、backup、明文 secrets。
- import 校验 manifest version；同名同内容 skip；同名不同内容返回稳定错误，先不做交互式 overwrite。
- import 后不自动 avm use。

验收：
- go test ./...
- go vet ./...
- 临时 HOME 测试覆盖 agent export/import、env export/import、same-content skip、different-content conflict。
- 提交前 git status --short --untracked-files=all 只能包含 Owner 范围文件。

完成后：
git push -u origin feat/package-io
git status --short
cd /Users/danielxing/code/agent-vm
git worktree remove ../agent-vm-package-io

回复修改文件、测试结果、commit hash、远程分支，以及 export/import 仍未覆盖的对象类型。
```

---

## 成功标准

Phase 1 coding 完成时至少满足：

1. `avm init` 只写 `~/.avm/**`。
2. `avm agent create/list/show` 可用。
3. `avm memory import --dry-run` 可用，且不写 runtime native memory。
4. `avm use <profile>` 可重建 active 并调用 adapter render。
5. `avm env create/use` 可按 runtime 选择不同 Agent Profile。
6. Codex、Claude Code、OpenCode、Cline 至少有 fixture 覆盖。
7. Cursor Phase 1 partial support 通过 warnings 和 mapping status 明确展示。
8. `workspace_isolation` 不存在于 Agent Profile 主模型。
9. mapping status 无 silent drop。
10. `go test ./...` 通过。
