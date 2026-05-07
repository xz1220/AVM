# AVM Legacy Architecture (pre-rewrite)

> 说明：这份文档是当前代码库重构前的「老架构」汇总。仅作为历史参考，不再维护。
> 当前 source of truth 是：
>
> 1. `docs/product/prd.md` — 产品需求和范围。
> 2. `docs/engineering/runtime-research/` — 底层 runtime 的实际能力与约束。
>
> 重构后代码和新架构设计会另行落地；本文件保留是为了避免丢失旧设计的决策轨迹。

本文件汇总了以下历史文档：

- `design/tech-design.md`
- `design/runtime-memory-isolation.md`（与 `research/runtime-memory-isolation.md` 基本一致）
- `engineering/architecture.md`
- `engineering/activation-model.md`
- `engineering/data-model.md`
- `engineering/file-layout.md`
- `engineering/workflows.md`
- `engineering/acceptance.md`
- `engineering/implementation-plan.md`
- `engineering/codex-home-isolation.md`
- `engineering/fixture-conventions.md`
- `engineering/modules/{config,sync,adapter}.md`

---

## 1. 技术设计总入口（tech-design）

当前技术设计以 **Agent Profile + Environment + Package + Adapter** 为主线。
Memory 不是 AVM 的显式配置对象：没有 `memory_refs` schema、没有 `avm memory`
命令、没有 portable memory 文件布局，也不会在 adapter 中渲染成 runtime 指令。

后续即使重新讨论 memory，当前也只关注全局/用户级 runtime state 的隔离。
不讨论 memory 内容管理、导入导出、重置、同步或 runtime memory 开关。项目级
memory、repo 内规则文件、workspace memory 文件属于项目资产，不由 AVM 管理、
迁移或隔离。

### 核心决策

1. Agent Profile 是用户创建、编辑、激活和分享的主对象。
2. Environment 只保存 runtime 到 Agent Profile 的映射。
3. Package 只携带 Agent、Environment 和 capability metadata。
4. Adapter 必须报告字段映射状态：`native`、`rendered_as_instructions`、
   `ignored`、`unsupported`。
5. Sync 是 `use` 的实现细节，同时保留为高级修复命令。
6. Memory 原则需要重新讨论；当前范围仅限全局/用户级 runtime state 隔离，
   项目级 memory 不进入 AVM schema 或 CLI。

### 模块划分

- `internal/config`：配置模型、YAML 读写、active/env/agent resolution。
- `internal/adapter`：runtime-independent render input 与各 runtime adapter。
- `internal/sync`：生成 render plan、写 managed paths、记录状态。
- `internal/packageio`：package inspect/export/install。
- `internal/runtime`：runtime registry。
- `internal/state`：sync state。

---

## 2. Architecture（engineering/architecture）

AVM 是本地 agent 配置管理器。`~/.avm` 是 source of truth，runtime 配置文件是
adapter 渲染结果。

### 核心对象

| 对象 | 说明 |
| --- | --- |
| Agent Profile | 用户管理的主对象，包含 instructions、capabilities、permissions、model/runtime preferences |
| Environment | 多 runtime 工作场景，保存 runtime 到 Agent Profile 的映射 |
| Capability Registry | skills、MCP、commands、hooks、toolsets 的元数据来源 |
| Package | Agent/Environment 及 capability metadata 的分发单元 |
| Sync State | 最近一次 render/apply 的状态、managed paths、mapping 结果 |

Memory 不在当前架构对象中。AVM 不声明、不导入、不导出、不渲染 portable
memory；runtime-native memory 暂时只作为未来原则讨论的研究对象。

### 数据流

```text
ActiveRef
  -> config.ResolveActivation
  -> adapter.RenderInput
  -> adapter.RenderPlan
  -> sync apply managed paths
  -> state.SyncState
```

### 不变量

- `~/.avm` 保存 AVM 管理的 Agent、Environment、registry、state。
- `avm init` 不写 runtime-native 配置。
- runtime 写入只能发生在 adapter 声明的 managed paths。
- adapter 不能静默丢字段；必须报告 mapping status。
- Package 安装不改变 active 对象。
- Secrets 只能引用，不复制明文。

---

## 3. Activation Model

Activation is the process of turning an Agent Profile or Environment into
runtime-managed files.

```text
avm use <profile-or-env>
  -> update ~/.avm/config.yaml active ref
  -> resolve active object
  -> resolve per Agent/runtime boundary
  -> build render input for each target runtime
  -> adapter plans managed path writes
  -> sync applies plans with conflict checks and backups
  -> write sync-state.json
```

Activation does not read, import, export, reset, or edit memory content. It only
selects runtime-native isolation boundaries such as `CODEX_HOME`,
`CLAUDE_CONFIG_DIR`, and OpenCode's process env envelope.

---

## 4. Data Model

### Global Config

`~/.avm/config.yaml` 保存默认 scope、目标 runtime、冲突策略、写入模式和当前
active ref。

### Agent Profile

`~/.avm/agents/<name>.yaml` 或 `<project>/.avm/agents/<name>.yaml`：

```yaml
name: backend-coder
id: agt_11111111111111111111111111111111
version: 1.0.0
source_scope: global
runtime:
  preferred: codex
  kind: local
  mode: primary
  fallback:
    - claude-code
instructions:
  system: |
    You implement backend changes with tests.
  developer: |
    Prefer small, reviewable changes.
capabilities:
  skills:
    - test
  mcps:
    - github
permissions:
  approval: on-request
  sandbox: workspace-write
model_run:
  model: gpt-5.4
  reasoning_effort: medium
```

`id` 是稳定 Agent identity：rename 保留，clone/create 生成新值。runtime
memory isolation 使用 `id + runtime` 决定边界目录。

Agent Profile 当前没有 memory 字段。任何 `memory_refs`、portable memory metadata
或 memory layers 都不属于当前 schema。

### Environment

`~/.avm/envs/<name>.yaml`：

```yaml
name: coding
version: 1.0.0
runtime_agents:
  codex:
    primary: backend-coder
  claude-code:
    primary: reviewer
targets:
  - codex
  - claude-code
```

Environment 只做 runtime 到 Agent Profile 的映射，不重复声明 capabilities。

### Resolved Activation

`config.ResolveActivation` 输出：

- active ref
- resolved runtime agents
- resolved capabilities
- targets
- source files
- warnings

### Package Manifest

Package manifest 当前包含：

- version/exported_at/kind/name
- agents
- envs
- capabilities
- include_files

---

## 5. File Layout

### AVM Home

```text
~/.avm/
├── config.yaml
├── agents/
├── envs/
├── registry/
│   ├── skills/
│   ├── mcps/
│   ├── commands/
│   ├── hooks/
│   └── toolsets/
├── active/
├── runtime-homes/
├── state/
├── backup/
└── cache/
```

There is no AVM-managed memory directory in the current design.

### Active Directory

`~/.avm/active/` is rebuilt by sync and may contain runtime-independent
projections for agents and capabilities:

```text
active/
├── agents/
├── skills/
├── mcps/
├── commands/
├── hooks/
└── render/
```

### Runtime Homes

AVM writes isolated runtime homes under:

```text
~/.avm/runtime-homes/agents/<agent-id>/<runtime>/
```

Adapters can also write project-managed files such as Cursor rules or Cline
rules when those paths are declared in the render plan.

### Permissions

- AVM home directories should be created with restrictive local permissions.
- Secrets should stay as environment variable references.
- Runtime-native user files are not overwritten unless an adapter declares an
  explicit managed path and conflict checks pass.

---

## 6. Workflows

### Initialize

```bash
avm init
```

Creates `~/.avm`, default config, default Agent, default Environment, state,
backup, cache, and registry directories. It does not write runtime-native
configuration.

### Agent CRUD

```bash
avm agent create backend-coder --runtime codex --skills test --mcps github
avm agent list
avm agent show backend-coder
avm agent show backend-coder --runtime codex
avm agent edit backend-coder
avm agent clone backend-coder --name api-coder
avm agent rename backend-coder api-coder --update-refs
avm agent delete api-coder --force
```

Agent create/edit manage identity, instructions, runtime preferences, model
preferences, capabilities, and permissions. They do not manage memory.

### Environment

```bash
avm env create coding --codex backend-coder --claude-code reviewer
avm env create default --local --codex backend-coder
```

Environment maps runtimes to Agent Profiles.

### Activation

```bash
avm use backend-coder
avm use --kind env coding
avm status
avm sync
avm deactivate
```

`use` updates active config and applies runtime render plans. `sync` reapplies
the current active object for repair/debugging.

### Package

```bash
avm package inspect backend-coder.avm.zip
avm package export backend-coder --output backend-coder.avm.zip
avm package install backend-coder.avm.zip
```

Packages include Agent/Environment YAML and referenced capability metadata.

---

## 7. Acceptance Criteria

### CLI Smoke

- `avm init`
- `avm agent create/list/show/edit/delete/rename/clone`
- `avm agent show --runtime <runtime>`
- `avm env create`
- `avm env create --local`
- `avm use`
- `avm status`
- `avm sync`
- `avm run <runtime>`
- `avm deactivate`
- `avm shell init`
- `avm package inspect/export/install`
- `avm skill list`

There is no `avm memory` acceptance path.

### Safety

- `avm init` writes only under `~/.avm`.
- Runtime-native config is written only through adapter managed paths.
- Runtime homes are keyed by stable Agent ID, not active/env name.
- OpenCode full data/state isolation is provided through `avm run opencode`.
- Unsupported mappings are surfaced in status.
- Package install does not activate the installed package.
- Secrets remain references.

### Verification

CI should run:

- `go build ./...`
- `go vet ./...`
- `test -z "$(gofmt -l .)"`
- `go test ./...`

---

## 8. Implementation Plan Snapshot

### Current Done Scope

- CLI scaffold and root command.
- `init`, `use`, `status`, `sync`, `deactivate`, `shell init`.
- Agent create/list/show/edit/delete/rename/clone.
- Environment create and local project override.
- Runtime adapters for Codex, Claude Code, OpenCode, Cline, Cursor.
- Package inspect/export/install.
- Capability registry lookup for skills/MCP and adapter projection.
- Runtime memory isolation by stable Agent ID, including Codex/Claude runtime
  homes and OpenCode `avm run` process envelope.

### Removed Scope

The previous explicit memory implementation has been removed:

- `avm memory`
- `internal/memory`
- `memory_refs`
- portable memory config CRUD
- memory package export/import
- adapter memory rendering

### Next Work (historical)

- Complete Environment CRUD.
- Unify package command naming and lifecycle.
- Improve first-run and interactive create/edit flows.
- Add doctor/uninstall.
- Keep future memory content management out of scope until a separate product
  model exists.

---

## 9. Module Responsibilities

### Config (`internal/config`)

`internal/config` owns AVM schema, YAML read/write helpers, validation, and
activation resolution.

Responsibilities:

- Read/write global config.
- Read/write/list Agent Profiles.
- Read/write/list Environments and project overrides.
- Validate known schema fields and reject unknown YAML fields.
- Resolve an active profile or environment into runtime agents and capabilities.

Current model:

- `GlobalConfig`
- `AgentProfile`
- `Environment`
- `ActiveRef`
- capability registry entries
- resolved capability structs

`AgentProfile` includes a stable `id` used for runtime boundary isolation;
rename preserves it, clone/create generate a new one, and legacy profiles are
backfilled on read.

There is no config-level memory model. `PortableMemory`, `MemoryRef`,
`memory_refs`, and memory path helpers are intentionally absent.

Resolution contract — `ResolveActivation(ref, cwd)` returns:

- `Active`
- `Env`
- `RuntimeAgents`
- `Capabilities`
- `Targets`
- `SourceFiles`
- `Warnings`

### Sync (`internal/sync`)

`internal/sync` applies resolved activation to runtime-managed files.

Responsibilities:

- Resolve current active object.
- Resolve per Agent/runtime boundary.
- Build adapter render inputs.
- Ask adapters for render plans.
- Check managed path conflicts.
- Create backups when configured.
- Apply render operations.
- Persist sync state.

`~/.avm/active/` may contain:

- agents
- skills
- MCP metadata
- commands
- hooks
- render metadata

It does not contain memory projections.

Sync does not manage memory content. It only writes adapter-managed runtime
config under the Agent's runtime boundary and records isolation status in
sync-state.

Conflict rules: AVM only writes adapter-declared managed paths. If an existing
file has conflicting unmanaged content, sync reports the conflict instead of
overwriting silently.

### Adapter (`internal/adapter`)

`internal/adapter` translates resolved AVM activation input into
runtime-specific render plans.

Contract:

```go
type Adapter interface {
    Name() string
    Detect(ctx Context) Detection
    Plan(ctx Context, input RenderInput) (*RenderPlan, error)
    Render(ctx Context, plan *RenderPlan) (*RenderResult, error)
    ManagedPaths(ctx Context, plan *RenderPlan) []ManagedPath
}
```

`RenderInput` contains active ref, runtime, Agent projection, resolved
capabilities, project root, active dir, and runtime boundary. `RuntimeHome`
remains as a compatibility field; adapters should prefer `Boundary`.

Every adapter must report how fields map:

- `native`
- `rendered_as_instructions`
- `ignored`
- `unsupported`

Adapters no longer expose memory import capabilities and do not render AVM
memory refs.

Supported runtime scope:

- Codex: isolated config + role files.
- Claude Code: agent markdown + MCP file.
- OpenCode: config, agent, skills, MCP.
- Cline: rules + MCP settings.
- Cursor: conservative rules/MCP proof of concept.

### Codex Home Isolation (detail)

The Codex adapter writes an AVM-owned runtime home instead of mutating the
user's native Codex config directly.

```text
~/.avm/runtime-homes/agents/<agent-id>/codex/
├── config.toml
└── agents/
    └── <agent>.toml
```

The render plan maps Agent fields to Codex profile/agent fields where possible
and renders unsupported guidance into developer instructions. It does not render
memory refs or portable memory content.

---

## 10. Runtime Memory Isolation 方案

> 范围：只讨论隔离，不讨论 memory 内容管理、导入导出、同步、重置或开关。

### 目标

AVM 要解决的问题是：同一台机器上存在多个 AVM Agent 配置时，它们使用同一个
runtime 产生的全局/用户级 memory state 不能互相串。

隔离单位是 `AVM Agent Profile + Runtime`，例如：

```text
backend-coder + codex
backend-coder + claude-code
reviewer + codex
reviewer + claude-code
```

每个组合都应该有自己的 runtime memory boundary。

项目级 memory 不进入 AVM 管理面。`CLAUDE.md`、repo 内 rules、workspace
`MEMORY.md`、`.github/instructions/memory.instruction.md`、project-scoped
subagent memory 都是项目资产，AVM 不导入、不导出、不重置、不迁移、不隔离。

### 改造前 AVM 状态

AVM 原本已经有一套 runtime home 机制：

- `config.RuntimeHomesDir()` 返回 `~/.avm/runtime-homes`
- `config.RuntimeHomeDir(active, runtime)` 返回
  `~/.avm/runtime-homes/<active>/<runtime>`
- `internal/sync` 在 activation 时为 `codex`、`claude-code`、`opencode`
  创建 runtime home
- adapter 的 `RenderInput.RuntimeHome` 会传入对应 adapter
- `avm activate` 会导出 `CODEX_HOME`、`CLAUDE_CONFIG_DIR`、`OPENCODE_CONFIG`、
  `OPENCODE_CONFIG_DIR`

这套机制已经能把 runtime 写入从用户真实 home 里隔开，但当前 key 是 active：

```text
~/.avm/runtime-homes/profile-backend-coder/codex
~/.avm/runtime-homes/env-backend-dev/codex
```

这不满足 memory isolation 的目标，两个问题：

1. 同一个 Agent 出现在多个 Environment 里，会被分成多个 runtime home，memory
   不连续。
2. 同一个 Environment 后续换了 runtime → Agent 映射，旧 Agent 和新 Agent 可能
   复用同一个 env-scoped runtime home，memory 会串。

因此隔离边界不能按 active/env key。最优方案也不应该只是新增一个
`AgentRuntimeHomeDir` helper，而应该把「runtime 隔离边界」建成一等对象。

### 推荐架构

#### 1. 稳定 Agent Identity

隔离边界的 key 应该是稳定 Agent identity，而不是可重命名的 display/name。

当前 `AgentProfile.Name` 同时承担了文件名、用户可见名称和引用 ID。只用 name 做
runtime home key 会有一个结构性问题：`avm agent rename` 后，逻辑上还是同一个
Agent，但 runtime home 路径会变化，memory/state 连续性会丢失。

推荐在 Agent Profile 中引入稳定 ID：

```yaml
id: agt_01h...
name: backend-coder
```

规则：

- `agent create` 生成新 ID。
- `agent rename` 保留 ID。
- `agent clone` 生成新 ID。
- 旧配置缺少 ID 时，首次读写或 migration 时补齐。
- Package import 如遇 ID 冲突，按导入冲突策略生成新 ID 或要求用户确认。

#### 2. RuntimeBoundary 一等对象

新增一个独立 resolver，而不是让 sync、adapter、activate 各自拼路径和 env。

建议包：`internal/boundary`

核心结构：

```go
type IsolationStatus string

const (
    IsolationIsolated   IsolationStatus = "isolated"
    IsolationShared     IsolationStatus = "shared"
    IsolationUnsupported IsolationStatus = "unsupported"
)

type BoundaryKey struct {
    AgentID   string
    AgentName string
    Runtime   string
}

type RuntimeBoundary struct {
    Key          BoundaryKey
    Root         string
    Env          map[string]string
    RunEnv       map[string]string
    Paths        map[string]string
    Isolation    IsolationStatus
    BoundaryType string // runtime_home | process_env | none
    Warnings     []string
}
```

- `Root` 是这个 Agent/runtime 的私有边界根目录。
- `Env` 是可以由 `avm activate` 长期导出到 shell 的安全环境变量。
- `RunEnv` 是由 `avm run <runtime>` 注入给 runtime 进程的完整 envelope。
- `Paths` 存 runtime-specific 子路径，例如 `config_dir`、`data_home`、
  `state_home`、`db_path`。

#### 3. RuntimeBoundaryResolver

每个 runtime 有自己的 boundary resolver：

```go
type RuntimeBoundaryResolver interface {
    ResolveBoundary(input BoundaryInput) (RuntimeBoundary, error)
}

type BoundaryInput struct {
    Runtime   string
    AgentID   string
    AgentName string
    Overrides BoundaryOverrides
}
```

sync 不再自己决定 runtime home。sync 只做：

```text
ResolvedActivation
  -> for each runtime target, find resolved Agent
  -> boundary.Resolve(runtime, agent)
  -> adapter.RenderInput{RuntimeHome/RuntimeBoundary}
  -> adapter.Plan/Render
  -> result.Target.Boundary
```

activate 也不再 switch runtime 拼 env，而是直接输出 `target.Boundary.Env`。

#### 4. 目录模型

私有边界根目录：

```text
~/.avm/runtime-homes/agents/<agent-id>/<runtime>/
```

runtime 名称继续规范化：

```text
claude-code -> claude
codex       -> codex
opencode    -> opencode
openclaw    -> openclaw
hermes      -> hermes
```

示例：

```text
~/.avm/runtime-homes/agents/agt_backend/codex/
~/.avm/runtime-homes/agents/agt_backend/claude/
~/.avm/runtime-homes/agents/agt_reviewer/codex/
~/.avm/runtime-homes/agents/agt_reviewer/claude/
```

Environment 只决定「当前 runtime 使用哪个 Agent」，不决定 memory boundary。

#### 5. Adapter 输入

目标 contract 是把 boundary 明确传入 adapter：

```go
type RenderInput struct {
    Active       ActiveRef
    Runtime      string
    Agent        Agent
    Capabilities CapabilitySet
    ProjectRoot  string
    ActiveDir    string
    Boundary     RuntimeBoundary
}
```

这样 OpenCode 这类 runtime 可以拿到 `config_dir`、`data_home`、`state_home`、
`db_path`，不必把所有东西压缩成一个 `RuntimeHome` 字符串。

旧 adapter 仍可通过 `RuntimeHome = boundary.Root` 做兼容桥，但新的实现不能把
单字符串 `RuntimeHome` 当成目标抽象。

#### 6. Activation Env Envelope

`avm activate` 不应该维护 runtime-specific switch：

```go
case "codex": CODEX_HOME=...
case "claude-code": CLAUDE_CONFIG_DIR=...
case "opencode": OPENCODE_CONFIG=...
```

这些应该由 `RuntimeBoundary.Env` 提供。

对于 Codex/Claude/Hermes 这种单进程 home env，shell export 可以直接生效。
对于 OpenCode 这种需要 `XDG_*` 的 runtime，最优方案是进程级 env envelope：

```bash
avm run opencode
```

或 shell wrapper 只在启动 OpenCode 时注入 `XDG_DATA_HOME`、`XDG_STATE_HOME`、
`XDG_CACHE_HOME`、`OPENCODE_DB`。不要把 `XDG_*` 长期 export 到用户 shell，
否则会影响同一个 shell 中的其他程序。

### Runtime 方案

#### Codex（隔离能力：强）

Codex memory 和 state 都在 `CODEX_HOME` 下：

```text
<CODEX_HOME>/memories/
<CODEX_HOME>/memories_extensions/
<CODEX_HOME>/state_*.sqlite
```

AVM 方案：

```bash
CODEX_HOME=~/.avm/runtime-homes/agents/<agent-id>/codex
```

只要每个 Agent/runtime 组合使用独立 `CODEX_HOME`，Codex 的 memory 与 state
就能隔离。

#### Claude Code（用户级部分隔离）

AVM 只隔离用户级 `.claude`：

```bash
CLAUDE_CONFIG_DIR=~/.avm/runtime-homes/agents/<agent-id>/claude
```

这可以隔离 user-level Claude state 和 user-scope subagent memory。

AVM 不管理：

```text
CLAUDE.md
.claude/rules/*.md
.claude/agent-memory/<agent>/
.claude/agent-memory-local/<agent>/
```

这些是项目资产。AVM 不为它们声明隔离。

#### OpenCode（通过 `avm run opencode` 完整隔离）

`avm activate` 只导出配置路径：

```bash
OPENCODE_CONFIG_DIR=~/.avm/runtime-homes/agents/<agent-id>/opencode/config
OPENCODE_CONFIG=~/.avm/runtime-homes/agents/<agent-id>/opencode/config/opencode.json
```

这只能隔离配置，不能把 `XDG_*` 长期导出到用户 shell。

`avm run opencode` 注入完整进程级隔离环境：

```bash
OPENCODE_DB=~/.avm/runtime-homes/agents/<agent-id>/opencode/data/opencode.db
XDG_DATA_HOME=~/.avm/runtime-homes/agents/<agent-id>/opencode/xdg-data
XDG_STATE_HOME=~/.avm/runtime-homes/agents/<agent-id>/opencode/xdg-state
XDG_CACHE_HOME=~/.avm/runtime-homes/agents/<agent-id>/opencode/xdg-cache
```

因此 OpenCode resolver 和 `avm run opencode` 一起交付，只对 OpenCode
进程注入 `XDG_*` 和 `OPENCODE_DB`。如果没有进程级 env envelope，OpenCode
必须标为 `unsupported`，不能把 config dir agent-scoped 说成完整 isolation。

#### OpenClaw（global state 可隔离）

OpenClaw global memory state：

```text
<state>/memory/<agentId>.sqlite
<state>/agents/<agentId>/qmd/
```

项目/workspace memory：

```text
<workspace>/MEMORY.md
<workspace>/memory/**/*.md
```

AVM 只管前者。

未来 OpenClaw adapter 应使用：

```bash
OPENCLAW_STATE_DIR=~/.avm/runtime-homes/agents/<agent-id>/openclaw/state
agentId=<agent-id>
```

并且默认不注入共享（`memory.qmd.paths`、
`agents.defaults.memorySearch.extraPaths`、`extraCollections`）。
只要用户显式配置共享路径，AVM 应把 isolation 标为 `shared` 或给出 warning。

#### Hermes（built-in memory 强隔离）

Hermes built-in memory 和 state 都在 `HERMES_HOME`：

```text
<HERMES_HOME>/memories/MEMORY.md
<HERMES_HOME>/memories/USER.md
<HERMES_HOME>/state.db
```

未来 Hermes adapter 应使用：

```bash
HERMES_HOME=~/.avm/runtime-homes/agents/<agent-id>/hermes
```

或者把 Hermes native profile 名称绑定到 AVM Agent name。

外部 memory provider 需要 provider-specific namespace：

- Honcho：workspace/peer 共享会导致 memory 共享。
- Hindsight：可以用 profile/workspace/user/session 模板生成 bank id。

AVM 只能在 provider namespace 可控时声明隔离。

### Isolation Status

当前只需要表达隔离状态，不表达 memory 内容。

建议内部结构：

```go
type RuntimeMemoryIsolation struct {
    Runtime      string   `json:"runtime"`
    AgentID      string   `json:"agent_id"`
    AgentName    string   `json:"agent_name"`
    Status       string   `json:"status"` // isolated | shared | unsupported
    BoundaryType string   `json:"boundary_type,omitempty"` // runtime_home | process_env | none
    Boundary     string   `json:"boundary,omitempty"`
    Warnings     []string `json:"warnings,omitempty"`
}
```

初期不一定要暴露为独立 CLI。可以先进入 sync result / status 输出，用于解释：

```text
codex        backend-coder  isolated    CODEX_HOME=...
claude-code  reviewer       isolated    CLAUDE_CONFIG_DIR=...
opencode     backend-coder  isolated    process envelope via avm run opencode
```

如果用户 override boundary：

```yaml
runtime_boundaries:
  agents:
    backend-coder:
      codex:
        root: /shared/codex
```

则对应 runtime isolation 不能再标 `isolated`，应标为 `shared` 或 warning。

### Backward Compatibility

旧目录：

```text
~/.avm/runtime-homes/profile-<name>/<runtime>
~/.avm/runtime-homes/env-<name>/<runtime>
```

新目录：

```text
~/.avm/runtime-homes/agents/<agent-id>/<runtime>
```

迁移原则：

1. 不自动删除旧目录，避免误删 runtime state 或用户数据。
2. 新 activation 开始使用新目录。
3. 可以通过 `avm doctor` 或文档提示旧 active-scoped runtime homes 可能已不再使用。
4. 不把旧目录内容自动搬迁到新目录，因为这属于 memory/state 迁移，超出当前「只关注隔离」的范围。

### 一次性交付范围

本方案不按「先最小改造、再补完整能力」的方式落地。对当前 AVM 已支持或即将
声明支持的 runtime，memory isolation 必须作为一个完整 feature 一次性交付。

完整交付范围：

1. 为 Agent Profile 增加稳定 ID，处理 create、rename、clone、旧配置补齐和
   package import 冲突。
2. 新增 `internal/boundary`，实现 `RuntimeBoundary`、resolver registry、
   runtime-specific paths/env/status/warnings。
3. `internal/sync` 改为按 resolved Agent boundary 渲染，而不是按 active 计算
   runtime home。
4. adapter input 显式接收 boundary。兼容 `RuntimeHome` 只能作为迁移桥，不作为
   目标 contract。
5. `cmd/avm/activate.go` 输出 `target.Boundary.Env`，移除 runtime-specific env
   拼接。
6. 对 Codex、Claude Code、OpenCode 实现完整 resolver。
7. OpenCode 同时实现进程级 env envelope，例如 `avm run opencode` 或 shell
   wrapper，注入 `OPENCODE_DB` 和 `XDG_*`。在没有进程级 envelope 前，不能声明
   OpenCode isolation 完成。
8. 在 sync result 或 status 中展示 runtime memory isolation status。
9. 对 boundary override 输出 `shared` 或 warning。
10. 更新测试、fixtures、README/PRD/design 文档。

OpenClaw 和 Hermes 当前不是 AVM 已实现 adapter。它们的 resolver 不需要在本次
feature 中交付，但对应 adapter 一旦进入支持范围，必须同时交付 memory isolation
boundary，不能先落 adapter 再补隔离。

### 非目标

- 不做 `avm memory`。
- 不做 memory import/export。
- 不做 memory reset。
- 不做 runtime memory sync。
- 不修改项目级 memory 文件。
- 不把 memory 放进 package。
- 不把 runtime native memory 自动提升成 AVM Agent 字段。

---

## 11. Fixture Conventions（保留原则）

`fixtures/` contains human-readable scenario fixtures. `testdata/` contains
stable inputs and golden outputs for automated tests.

Fixtures should model current AVM behavior only:

- Agent/Profile YAML
- Environment YAML
- registry capability metadata
- adapter render plans
- runtime layout samples

Do not add memory fixtures unless a new memory design is accepted and
implemented.
