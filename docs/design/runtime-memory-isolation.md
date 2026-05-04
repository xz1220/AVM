# Runtime Memory Isolation 技术方案

> 最后更新：2026-05-04
> 当前范围：只讨论隔离，不讨论 memory 内容管理、导入导出、同步、重置或开关。

## 目标

AVM 要解决的问题是：同一台机器上存在多个 AVM Agent 配置时，它们使用同一个
runtime 产生的全局/用户级 memory state 不能互相串。

隔离单位是：

```text
AVM Agent Profile + Runtime
```

例如：

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

## 改造前 AVM 状态

AVM 原本已经有一套 runtime home 机制：

- `config.RuntimeHomesDir()` 返回 `~/.avm/runtime-homes`
- `config.RuntimeHomeDir(active, runtime)` 返回
  `~/.avm/runtime-homes/<active>/<runtime>`
- `internal/sync` 在 activation 时为 `codex`、`claude-code`、`opencode`
  创建 runtime home
- adapter 的 `RenderInput.RuntimeHome` 会传入对应 adapter
- `avm activate` 会导出：
  - `CODEX_HOME`
  - `CLAUDE_CONFIG_DIR`
  - `OPENCODE_CONFIG`
  - `OPENCODE_CONFIG_DIR`

这套机制已经能把 runtime 写入从用户真实 home 里隔开，但当前 key 是 active：

```text
~/.avm/runtime-homes/profile-backend-coder/codex
~/.avm/runtime-homes/env-backend-dev/codex
```

这不满足 memory isolation 的目标。

问题有两个：

1. 同一个 Agent 出现在多个 Environment 里，会被分成多个 runtime home，memory
   不连续。
2. 同一个 Environment 后续换了 runtime -> Agent 映射，旧 Agent 和新 Agent 可能
   复用同一个 env-scoped runtime home，memory 会串。

因此隔离边界不能按 active/env key。更进一步，最优方案也不应该只是新增一个
`AgentRuntimeHomeDir` helper，而应该把“runtime 隔离边界”建成一等对象。

## 推荐架构

### 1. 稳定 Agent Identity

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

如果当前阶段暂时不想改 schema，可以先用 `name` 作为 provisional identity，
但技术方案必须明确：最优边界是 stable Agent ID。

### 2. RuntimeBoundary 一等对象

新增一个独立 resolver，而不是让 sync、adapter、activate 各自拼路径和 env。

建议包：

```text
internal/boundary
```

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

`Root` 是这个 Agent/runtime 的私有边界根目录。`Env` 是可以由 `avm activate`
长期导出到 shell 的安全环境变量。`RunEnv` 是由 `avm run <runtime>` 注入给
runtime 进程的完整 envelope。`Paths` 存 runtime-specific 子路径，例如
`config_dir`、`data_home`、`state_home`、`db_path`。

### 3. RuntimeBoundaryResolver

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

### 4. 目录模型

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

Environment 只决定“当前 runtime 使用哪个 Agent”，不决定 memory boundary。

### 5. Adapter 输入

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

### 6. Activation Env Envelope

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

## Runtime 方案

### Codex

隔离能力：强。

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

### Claude Code

隔离能力：用户级部分隔离。

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

### OpenCode

隔离能力：通过 `avm run opencode` 完整隔离。

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

但 `XDG_*` 不适合直接导出到用户 shell，因为会影响同一个 shell 里启动的其他
程序。因此 OpenCode resolver 和 `avm run opencode` 一起交付，只对 OpenCode
进程注入 `XDG_*` 和 `OPENCODE_DB`。如果没有进程级 env envelope，OpenCode
必须标为 `unsupported`，不能把 config dir agent-scoped 说成完整 isolation。

### OpenClaw

隔离能力：global state 可隔离。

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

并且默认不注入共享：

```text
memory.qmd.paths
agents.defaults.memorySearch.extraPaths
extraCollections
```

只要用户显式配置共享路径，AVM 应把 isolation 标为 `shared` 或给出 warning。

### Hermes

隔离能力：built-in memory 强隔离。

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

## Isolation Status

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

## Backward Compatibility

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
4. 不把旧目录内容自动搬迁到新目录，因为这属于 memory/state 迁移，超出当前“只关注隔离”的范围。

## 测试计划

需要覆盖：

1. Agent 创建时生成稳定 ID，rename 保留 ID，clone 生成新 ID。
2. Boundary resolver 用 `AgentID + Runtime` 生成稳定 root。
3. `avm use <agent>` 为每个 runtime 生成 agent-scoped boundary。
4. `avm use <env>` 按 env 中 resolved Agent 生成 boundary，而不是 env 名称。
5. 同一个 Agent 出现在两个 env 中，boundary 相同。
6. 同一个 env 修改 runtime -> Agent 映射后，新 Agent 使用自己的 boundary。
7. `avm activate` 输出 env 时使用 `target.Boundary.Env`，不在 CLI switch runtime。
8. OpenCode 的 `activate` 不导出 `XDG_*`，`avm run opencode` 注入完整
   `OPENCODE_DB` 和 `XDG_*`。
9. managed paths 不包含项目级 memory 文件。

## 一次性交付范围

本方案不按“先最小改造、再补完整能力”的方式落地。对当前 AVM 已支持或即将
声明支持的 runtime，memory isolation 必须作为一个完整 feature 一次性交付。
不能先把 runtime home 换成 agent-scoped，然后把 stable ID、boundary status、
OpenCode 进程级隔离、activation env envelope 留作后续。

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

## 非目标

- 不做 `avm memory`。
- 不做 memory import/export。
- 不做 memory reset。
- 不做 runtime memory sync。
- 不修改项目级 memory 文件。
- 不把 memory 放进 package。
- 不把 runtime native memory 自动提升成 AVM Agent 字段。
