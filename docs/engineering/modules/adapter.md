# Agent VM — Adapter 模块设计

> 最后更新：2026-04-24（v6 — Portable Memory 对齐）

Adapter 模块负责把 AVM 的 Agent Profile 渲染到各 runtime 的原生配置。Environment 只在进入 adapter 前解析为“当前 runtime 应该使用哪个 Agent Profile”；adapter 不从 Environment 读取 capabilities 或 memory。adapter 必须显式说明每个字段是 native、生成为 instructions、ignored 还是 unsupported，不能静默丢字段。

---

## 职责

1. 检测 runtime 是否安装、配置目录是否存在。
2. 导入已有 runtime 配置，生成 AVM 候选对象。
3. 根据 resolved active 和当前 runtime 的 Agent Profile 生成 render plan。
4. 写入 runtime managed paths。
5. 返回字段级 mapping status。
6. 提供冲突检测和备份所需的 managed paths。

## 不做

- 不解析全局/项目级覆盖。
- 不管理 registry 安装。
- 不决定 conflict strategy。
- 不直接覆盖用户非 AVM 管理的 instruction 文件。

---

## 接口定义

```go
package adapter

type Adapter interface {
    Name() string
    Detect(ctx Context) Detection
    Import(ctx Context) (*ImportResult, error)
    Plan(ctx Context, input RenderInput) (*RenderPlan, error)
    Render(ctx Context, plan *RenderPlan) (*RenderResult, error)
    ManagedPaths(ctx Context, plan *RenderPlan) []ManagedPath
}

type RenderInput struct {
    Active       *config.ResolvedActivation
    Runtime      string
    Agent        config.AgentProfile
    Capabilities config.ResolvedCapabilities
    Memory       []config.PortableMemory
    ProjectRoot  string
}

type RenderPlan struct {
    Runtime      string
    Active       config.ActiveRef
    AgentName    string
    ManagedPaths []ManagedPath
    Operations   []RenderOperation
    Mappings     []FieldMapping
}

type ManagedPath struct {
    Path        string
    Owner       string // avm | shared-section
    Description string
    Required    bool
    MergeMode   string // whole-file | marked-block | structured-section
}

type FieldMapping struct {
    SourcePath string
    TargetPath string
    Status     MappingStatus
    Reason     string
}

type MappingStatus string

const (
    MappingNative                 MappingStatus = "native"
    MappingRenderedAsInstructions MappingStatus = "rendered_as_instructions"
    MappingIgnored                MappingStatus = "ignored"
    MappingUnsupported            MappingStatus = "unsupported"
)
```

Portable Memory 的显式迁移能力通过可选接口暴露。Phase 1 至少实现 dry-run；Phase 2 再允许写回。

```go
type MemoryImportCapable interface {
    ImportMemory(ctx Context, opts MemoryImportOptions) (*MemoryImportPlan, error)
}

type MemoryImportOptions struct {
    Runtime string
    Source  string
    DryRun  bool
}

type MemoryImportPlan struct {
    Runtime    string
    Source     string
    Candidates []config.PortableMemory
    Diffs      []MemoryDiff
    Warnings   []string
}

type MemoryDiff struct {
    MemoryID   string
    Status     string // new | changed | conflict | skipped
    SourcePath string
    TargetPath string
    Preview    string
}
```

---

## 写入原则

1. **保留用户内容**：读取目标文件后只改 AVM 管理 section 或 adapter 声明的字段。
2. **写前备份**：`ManagedPaths` 中所有会覆盖的文件先备份。
3. **原子写入**：写临时文件后 rename。
4. **不展开 secrets**：`${ENV_VAR}` 保持引用，除非用户显式允许。
5. **状态可见**：所有 ignored/unsupported 字段写入 `sync-state.json` 并在 `avm status` 展示。

---

## Claude Code Adapter

### 写入目标

| AVM 字段 | Claude Code 目标 | 状态 |
|----------|------------------|------|
| agent name/description/instructions | `.claude/agents/<name>.md` frontmatter + body | native |
| model/reasoning | agent frontmatter `model`/`effort` 或 settings | native |
| tools allow/deny | agent frontmatter `tools`/`disallowedTools` 或 settings `permissions` | native |
| skills | `~/.claude/skills -> ~/.avm/active/skills` 或 `.claude/skills` | native |
| MCP | `<project>/.mcp.json` 或 settings MCP fields | native |
| hooks | settings `hooks` 或 agent frontmatter `hooks` | native |
| memory_refs | agent frontmatter memory scope + AVM memory references | native/rendered |

### Agent 文件

```markdown
---
name: backend-coder
description: "Backend implementation agent"
tools: Bash, Read, Edit
disallowedTools: WebFetch
skills: git, test
mcpServers: github, postgres
model: inherit
effort: medium
permissionMode: default
memory: project
---

You implement backend changes with tests.
```

### MCP 文件

```json
{
  "mcpServers": {
    "github": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-github"],
      "env": {
        "GITHUB_TOKEN": "${GITHUB_TOKEN}"
      }
    }
  }
}
```

### 注意事项

- `CLAUDE_CONFIG_DIR` 可覆盖 `~/.claude`。
- `.claude/settings.local.json` 属于本地机器配置，只有 `source_scope=local` 时才写。
- `CLAUDE.md` 不默认覆盖。
- `avm use` 不静默写入 Claude native memory 内容；native memory 的读写通过显式 `avm memory import/pull/push` 完成。
- memory scope 只接受 `user|project|local`；其他 scope 渲染到 instructions。

---

## Codex Adapter

### 写入目标

| AVM 字段 | Codex 目标 | 状态 |
|----------|------------|------|
| active-level model/reasoning/approval/sandbox | `[profiles.avm-<active>]` | native |
| MCP | `[mcp_servers.<name>]` | native |
| agent role | `[agents.<role>]` + `.codex/agents/<role>.toml` | native |
| developer instructions | role TOML `developer_instructions` | native |
| skills | `developer_instructions` 引用 active skills | rendered_as_instructions |
| memory_refs | `developer_instructions` 引用 | rendered_as_instructions |
| project `AGENTS.md` | 只读导入，不默认覆盖 | ignored |

### `config.toml`

```toml
profile = "avm-backend-coder"

[profiles.avm-backend-coder]
model = "gpt-5.4"
model_reasoning_effort = "medium"
approval_policy = "on-request"
sandbox_mode = "workspace-write"

[mcp_servers.github]
command = "npx"
args = ["-y", "@modelcontextprotocol/server-github"]
env = { GITHUB_TOKEN = "${GITHUB_TOKEN}" }
supports_parallel_tool_calls = true

[agents.backend-coder]
description = "Backend implementation agent"
config_file = "./agents/backend-coder.toml"
nickname_candidates = ["Coder"]
```

### role TOML

```toml
name = "backend-coder"
description = "Backend implementation agent"
nickname_candidates = ["Coder"]
developer_instructions = """
You implement backend changes with tests.

Active AVM skills:
- ~/.avm/active/skills/git/SKILL.md
- ~/.avm/active/skills/test/SKILL.md
"""
model = "gpt-5.4"
approval_policy = "on-request"
sandbox_mode = "workspace-write"
```

### 注意事项

- Codex profile 是 active-level 运行配置；`avm use backend-coder` 生成 `avm-backend-coder`，`avm use backend-dev` 生成 `avm-backend-dev`。
- 在 Environment 激活时，每个 runtime 只渲染该 runtime 的 primary Agent Profile；一个 active 中多个 AVM agents 可写成多个 `[agents.<role>]` 和 role TOML，但默认启动 role 必须来自 `runtime_agents.<runtime>.primary`。
- `sandbox_mode`: `read-only | workspace-write | danger-full-access`。
- `approval_policy`: `untrusted | on-request | never` 等。
- role 需要 `description`；直接发现的 role TOML 需要 `name` 和 `developer_instructions`。
- TOML 必须用结构化 parser 合并，不能用字符串拼接。

---

## OpenCode Adapter

### 写入目标

| AVM 字段 | OpenCode 目标 | 状态 |
|----------|---------------|------|
| active default agent | `opencode.json#default_agent` | native |
| agent definition | `agents/<agent>.md` | native |
| permissions | agent/config `permission` | native |
| MCP | `opencode.json#mcp` | native |
| skills with `SKILL.md` source | `skills/<skill>/SKILL.md` | native |
| unresolved skill names | agent body | rendered_as_instructions |
| reasoning/verbosity | agent body | rendered_as_instructions |
| project `AGENTS.md` / `.opencode/` | user-owned | ignored |

### Isolated Runtime Home

OpenCode supports environment-variable isolation:

```bash
export OPENCODE_CONFIG="$AVM_RUNTIME_HOME/opencode.json"
export OPENCODE_CONFIG_DIR="$AVM_RUNTIME_HOME"
```

AVM writes:

```text
$AVM_RUNTIME_HOME/
  opencode.json
  agents/<agent>.md
  skills/<skill>/SKILL.md
```

The adapter must not overwrite `~/.config/opencode` or project `.opencode/`.

---

## Cline Adapter

### 写入目标

| AVM 字段 | Cline 目标 | 状态 |
|----------|------------|------|
| MCP | `<cline-data>/settings/cline_mcp_settings.json` | native |
| permissions | `globalState.json` 的 `autoApprovalSettings` 安全子集 | native/partial |
| agent instructions | `.clinerules/avm/<agent>.md` | rendered_as_instructions |
| skills/hooks/workflows toggles | global/workspace state | native/partial |
| subagents | `subagentsEnabled` 状态 | native for toggle, not Agent Profile |
| memory_refs | `.clinerules/avm/<agent>.md` 引用 | rendered_as_instructions |

### MCP 文件

```json
{
  "mcpServers": {
    "github": {
      "command": "node",
      "args": ["/path/to/server.js"],
      "env": {
        "GITHUB_TOKEN": "${GITHUB_TOKEN}"
      },
      "alwaysAllow": [],
      "disabled": false
    }
  }
}
```

### Rules 文件

```markdown
---
paths:
  - "src/**"
---

# AVM Agent: backend-coder

You implement backend changes with tests.

Memory references:
- ~/.avm/memory/project/backend-standards.md
```

### 注意事项

- `CLINE_DIR` 可覆盖 `~/.cline`；CLI/standalone 数据目录是 `<CLINE_DIR>/data`。
- VS Code/Cursor 扩展形态使用 IDE `globalStorage` 下的 Cline 扩展目录；adapter detect 需要同时查找 CLI 数据目录和扩展数据目录。
- workspace rules 推荐写 `<project>/.clinerules/avm/`，避免污染用户已有 rules。
- Cline subagents 是实验性只读研究代理，不等价于 AVM Agent Profile。
- `autoApprovalSettings` 字段必须保守映射，不能打开 `executeAllCommands` 这类高风险能力，除非用户显式配置。

---

## Cursor PoC Adapter

Phase 1 只做文件级 PoC：

| AVM 字段 | Cursor 目标 | 状态 |
|----------|-------------|------|
| MCP | `<project>/.cursor/mcp.json` | native |
| instructions | `.cursor/rules/avm-<agent>.md` 或 `.cursorrules` AVM block | rendered_as_instructions |
| Agent Profile | 无完整映射 | unsupported |
| memory/permissions | rules 文本或 ignored | rendered/ignored |

Cursor adapter 的 `Detection.Level` 必须返回 `partial`，`avm status` 要显示“PoC only”。

---

## OpenClaw Future Adapter 约束

不在 Phase 1 实现，但字段设计要兼容：

| AVM 字段 | OpenClaw 未来目标 |
|----------|-------------------|
| agents | `agents.list[]` |
| workspace | `agents.defaults.workspace` 或 per-agent `workspace` |
| sandbox | `agents.defaults.sandbox` 或 per-agent `sandbox` |
| skills | `~/.openclaw/workspace/skills/<skill>/SKILL.md` |
| MCP | `mcp.servers` |
| channel routing | `bindings[]` |
| gateway/channel policy | `gateway`、`channels.<provider>` |

Phase 1 统一模型不保留 `workspace_isolation` 主干字段。OpenClaw 的 workspace、routing、channel binding、gateway 原生字段先保存在 `runtime_extensions.openclaw`，由 future adapter 显示 mapping status；这些字段未来可再提升为一等模型。

---

## Import 规范

`Import()` 返回候选对象，不直接写 source of truth：

```go
type ImportResult struct {
    Confirmed         ImportBundle
    Candidates        []ImportCandidate
    Ignored           []ImportIgnored
    RuntimeExtensions map[string]map[string]any
    Warnings          []string
}

type ImportBundle struct {
    Agents       []config.AgentProfile
    Capabilities config.CapabilityBundle
    Memory       []config.PortableMemory
    Environments []config.Environment
}

type ImportCandidate struct {
    Kind       string // agent | capability | memory | environment
    Name       string
    SourcePath string
    Confidence string // high | medium | low
    Reason     string
    Value      any
}

type ImportIgnored struct {
    SourcePath string
    Reason     string
}
```

导入策略：

- 原生 agent/profile 且语义明确时转成 `Confirmed.Agents`。
- rules、项目指导文件或多义 profile 先进入 `Candidates`，由 CLI/import 报告让用户确认。
- 无法识别的 runtime 字段放入 `runtime_extensions.<runtime>`。
- 用户 instruction 文件只引用路径或创建 imported instruction ref，不覆盖。
- 同名对象冲突时由 CLI 交互决定 rename/skip/overwrite。
