# Agent VM — Runtime 配置映射调研

> 最后更新：2026-04-29（v2 — added OpenCode from official docs）

本文档记录从 `agent-frameworks/` 读取到的关键 runtime 配置面。它是 `internal/adapter` 的实现依据，不是用户手册。

---

## 总览

| Runtime | 配置入口 | Agent/Role | Capability | Memory | Phase 1 状态 |
|---------|----------|------------|------------|--------|--------------|
| Claude Code | `~/.claude/settings.json`、`<project>/.claude/settings*.json`、`.mcp.json` | `.claude/agents/<name>.md` | skills、hooks、MCP、tools | `agent-memory/<agent>/MEMORY.md` | 完整 adapter |
| Codex | `~/.codex/config.toml`、`.codex/agents/*.toml`、`AGENTS.md` | `[agents.<role>]` + role TOML | MCP、profiles、tool/permission config | instruction 引用为主 | 完整 adapter |
| OpenCode | `~/.config/opencode/opencode.json`、`OPENCODE_CONFIG`、`OPENCODE_CONFIG_DIR`、`.opencode/` | `agent.<name>` 或 `agents/<name>.md` | MCP、permissions、skills、commands | instruction 引用为主 | 完整 adapter |
| Cline | `<cline-data>/globalState.json`、`<cline-data>/settings/cline_mcp_settings.json`、`.clinerules/` | 无稳定 Agent Profile；subagents 是实验研究工具 | rules、MCP、auto approval、skills/workflows/hooks toggles | rules/memory-bank instructions | 完整 adapter，但 agent 以 instructions 渲染 |
| Cursor | `.cursor/mcp.json`、`.cursor/rules/` / `.cursorrules` | 无统一本地 Agent Profile | MCP、rules | rules/instructions | 文件级 PoC |
| OpenClaw | `~/.openclaw/openclaw.json` | `agents.list[]`、`bindings[]` | gateway tools、channels、skills、MCP | `memory` config | Phase 1 不实现，仅保留设计约束 |

---

## Claude Code

### 路径和来源

- 用户配置目录：`CLAUDE_CONFIG_DIR` 或 `~/.claude`
- 用户 settings：`~/.claude/settings.json`
- 项目 settings：`<project>/.claude/settings.json`
- 项目 local settings：`<project>/.claude/settings.local.json`
- 项目 MCP：`<project>/.mcp.json`
- 用户 agents：`~/.claude/agents/<agent>.md`
- 项目 agents：`<project>/.claude/agents/<agent>.md`
- 用户 skills：`~/.claude/skills/<skill>/SKILL.md`
- 项目 skills：`<project>/.claude/skills/<skill>/SKILL.md`

### Agent 文件格式

Claude Code agent 是带 YAML frontmatter 的 Markdown：

```markdown
---
name: backend-coder
description: "Use for backend implementation"
tools: Bash, Read, Edit
disallowedTools: WebFetch
skills: git, test
mcpServers: github, postgres
hooks: pre-commit
model: inherit
effort: medium
permissionMode: default
memory: project
---

You are responsible for backend implementation...
```

已确认的 frontmatter 字段包括：

- required: `name`、`description`
- tools/capability: `tools`、`disallowedTools`、`skills`、`mcpServers`、`hooks`
- run config: `model`、`effort`、`permissionMode`、`maxTurns`、`initialPrompt`
- memory/isolation: `memory` (`user|project|local`)、`background`、`isolation`
- UI: `color`

### Settings 映射

`settings.json` 可表达：

- `permissions.allow/deny/ask/defaultMode/additionalDirectories`
- `model`、`availableModels`、`modelOverrides`
- `agent`
- `hooks`
- `enableAllProjectMcpServers`
- `enabledMcpjsonServers` / `disabledMcpjsonServers`
- `allowedMcpServers` / `deniedMcpServers`
- `autoMemoryEnabled`、`autoMemoryDirectory`
- `sandbox`

### MCP 映射

项目 `.mcp.json` 使用：

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

Claude Code 还会从 settings、enterprise/policy、dynamic 等来源读取 MCP。AVM Phase 1 默认写项目 `.mcp.json` 或 AVM 管理的 settings section；写入前必须保留非 AVM 管理 server。

### Memory 映射

Claude Code 支持 agent memory scope：

- user: `~/.claude/agent-memory/<agent>/MEMORY.md`
- project: `<project>/.claude/agent-memory/<agent>/MEMORY.md`
- local: `<project>/.claude/agent-memory-local/<agent>/MEMORY.md`

AVM Phase 1 在 `avm use` 中只投影 `memory_refs`：能安全表达 scope 时写 agent frontmatter 或 AVM 管理片段，不能匹配时渲染为 instructions，并标记 `rendered_as_instructions`。Claude native memory 内容的读取/写回只通过显式 `avm memory import/pull/push` 发生，并且必须输出 diff。

---

## Codex

### 路径和来源

- 主配置：`~/.codex/config.toml`
- role 配置：`~/.codex/agents/<role>.toml` 或相对 `config.toml` 的 `config_file`
- 项目指导：`AGENTS.md`

### Profile 映射

Codex 支持 `[profiles.<name>]`：

```toml
profile = "avm-coding"

[profiles.avm-coding]
model = "gpt-5.4"
model_reasoning_effort = "medium"
approval_policy = "on-request"
sandbox_mode = "workspace-write"
```

已确认枚举：

- `sandbox_mode`: `read-only`、`workspace-write`、`danger-full-access`
- `approval_policy`: `untrusted`、`on-request`、`never`，以及更细粒度配置

AVM Phase 1 应把 Codex profile 作为 active-level 运行配置，例如 `avm-<profile>` 或 `avm-<env>`。单 Agent Profile 激活时只渲染该 profile；Environment 激活时按 `runtime_agents.codex.primary` 选择默认 role。多个 AVM Agent 可通过 `[agents.<role>]` 和 role TOML 表达，但不能为每个 agent 互相抢写全局 `profile`。

### MCP 映射

Codex MCP server 在 `[mcp_servers.<name>]`：

```toml
[mcp_servers.github]
command = "npx"
args = ["-y", "@modelcontextprotocol/server-github"]
env = { GITHUB_TOKEN = "${GITHUB_TOKEN}" }
supports_parallel_tool_calls = true
default_tools_approval_mode = "on-request"
```

也支持 HTTP/SSE 形态、`enabled_tools`、`disabled_tools`、per-tool `approval_mode`、startup/tool timeout 等。

### Agent Role 映射

Codex 支持 `[agents.<role>]`，并可引用 role TOML：

```toml
[agents.backend-reviewer]
description = "Review backend changes"
config_file = "./agents/backend-reviewer.toml"
nickname_candidates = ["Reviewer"]
```

role TOML 示例：

```toml
name = "backend-reviewer"
description = "Review backend changes"
nickname_candidates = ["Reviewer"]
developer_instructions = """
Focus on correctness, tests, and security-sensitive changes.
"""
model = "gpt-5.4"
approval_policy = "on-request"
sandbox_mode = "read-only"
```

实现注意：

- role 需要 `description`。
- 直接发现的 role 文件需要非空 `name` 和 `developer_instructions`。
- `config_file` 可相对 `config.toml`。
- 项目 `AGENTS.md` 是用户/项目指导文件，AVM 不默认覆盖。

---

## OpenCode

资料来源：

- Config: <https://opencode.ai/docs/config/>
- Agents: <https://opencode.ai/docs/agents/>
- Permissions: <https://opencode.ai/docs/permissions/>
- MCP servers: <https://opencode.ai/docs/mcp-servers>
- Skills: <https://opencode.ai/docs/skills/>

### 路径和来源

OpenCode 的配置文件支持 JSON/JSONC，并会合并多个来源。关键路径：

- 全局 config：`~/.config/opencode/opencode.json`
- 项目 config：`opencode.json`
- 自定义 config：`OPENCODE_CONFIG=/path/to/opencode.json`
- 自定义 config directory：`OPENCODE_CONFIG_DIR=/path/to/dir`
- agent 目录：`~/.config/opencode/agents/`、`.opencode/agents/`
- skill 目录：`~/.config/opencode/skills/`、`.opencode/skills/`

AVM adapter 使用 isolated runtime home：

```bash
export OPENCODE_CONFIG="$AVM_HOME/runtime-homes/<active>/opencode/opencode.json"
export OPENCODE_CONFIG_DIR="$AVM_HOME/runtime-homes/<active>/opencode"
```

这样不需要软链接，不覆盖用户的 `~/.config/opencode`，也能让 OpenCode 读取 AVM 渲染的 agents 和 skills。

### Agent 映射

OpenCode 支持 `opencode.json` 里的 `agent` 对象，也支持 Markdown agent 文件。AVM Phase 1 采用 Markdown agent 文件，配合 config 里的 `default_agent`：

```json
{
  "$schema": "https://opencode.ai/config.json",
  "default_agent": "backend-coder"
}
```

```markdown
---
description: "Backend implementation agent"
mode: "primary"
model: "anthropic/claude-sonnet-4-5"
permission:
  edit: allow
  bash:
    "*": ask
    "go test ./...": allow
---

Developer instructions:
Prefer small, reviewable changes.
```

映射策略：

- `agent.name`：文件名和 `default_agent`
- `agent.description`：frontmatter `description`
- `agent.instructions.system/developer`：agent body
- `agent.model.model`：frontmatter `model` 和 config `model`
- `agent.model.temperature`：frontmatter `temperature`
- `agent.model.reasoning_effort`、`verbosity`：OpenCode 无 AVM 等价字段，写入 body 并标记 `rendered_as_instructions`

### Permissions 映射

OpenCode 的 `permission` 支持 `allow`、`ask`、`deny`，并可对 `bash`、`edit`、`read`、`external_directory` 等 key 做细粒度规则。AVM 映射：

- `permissions.approval = never` -> 默认 `allow`
- `permissions.approval = on-request|prompt|on-risky-actions|untrusted` -> 默认 `ask`
- `permissions.sandbox = read-only` -> `edit: deny`，`bash` 至少 `ask`
- `permissions.sandbox = workspace-write|danger-full-access` -> `edit: allow`
- `permissions.allow/deny` 中的 `Bash(<pattern>)` -> `permission.bash.<pattern>`
- `permissions.additional_directories` -> `permission.external_directory.<path>: allow`

### MCP 映射

OpenCode config 使用 `mcp`：

```json
{
  "mcp": {
    "github": {
      "type": "local",
      "command": ["npx", "-y", "@modelcontextprotocol/server-github"],
      "environment": {
        "GITHUB_TOKEN": "${GITHUB_TOKEN}"
      },
      "enabled": true
    }
  }
}
```

远程 MCP 使用：

```json
{
  "mcp": {
    "context7": {
      "type": "remote",
      "url": "https://mcp.context7.com/mcp",
      "headers": {
        "CONTEXT7_API_KEY": "{env:CONTEXT7_API_KEY}"
      },
      "enabled": true
    }
  }
}
```

AVM 会把 command 型 MCP 渲染为 local server；URL 型 MCP 渲染为 remote server。remote server 的 env 不能等价表达为 OpenCode 的 `headers` 时，会保留 warning。

### Skills 映射

OpenCode skill 使用 `<dir>/skills/<skill>/SKILL.md`，frontmatter 需要 `name` 和 `description`。AVM 从 active skill set 复制 `SKILL.md` 到 isolated runtime home，并添加 `avm_managed: true` 作为清理边界。没有 source path 的 skill name 只写入 agent instructions，并标记 `rendered_as_instructions`。

---

## Cline

### 路径和来源

- CLI/standalone 配置目录：`CLINE_DIR` 或 `~/.cline`
- CLI/standalone 数据目录：`~/.cline/data`
- VS Code/Cursor 扩展数据目录：IDE `globalStorage` 下的 Cline 扩展目录，例如 macOS VS Code 常见为 `~/Library/Application Support/Code/User/globalStorage/saoudrizwan.claude-dev/`
- 全局状态：`<cline-data>/globalState.json`
- workspace 状态：`<cline-data>/workspace/workspaceState.json`，或 host 提供的 workspace storage
- MCP 配置：`<cline-data>/settings/cline_mcp_settings.json`
- workspace rules：`<project>/.clinerules/`
- global rules：macOS `~/Documents/Cline/Rules`，Linux/WSL `~/Documents/Cline/Rules` 或 `~/Cline/Rules`

### Rules 映射

Cline 识别多种规则来源：

| 类型 | 位置 |
|------|------|
| Cline Rules | `.clinerules/` |
| Cursor Rules | `.cursorrules` |
| Windsurf Rules | `.windsurfrules` |
| AGENTS.md | `AGENTS.md` |

`.clinerules/` 中的 `.md` 和 `.txt` 会被合并，可用 YAML frontmatter `paths` 做条件加载。

Phase 1 中，Cline 没有稳定的本地 Agent Profile 文件。AVM 生成 Cline Agent 时应把 agent instructions 渲染为 `.clinerules/avm/<agent>.md` 或全局 Cline Rules 中 AVM 管理的文件，并标记 `rendered_as_instructions`。

### MCP 映射

`cline_mcp_settings.json` 格式一致，但路径取决于 Cline 运行形态：

- CLI/standalone: `~/.cline/data/settings/cline_mcp_settings.json`
- VS Code/Cursor 扩展: `<IDE globalStorage>/saoudrizwan.claude-dev/settings/cline_mcp_settings.json`

```json
{
  "mcpServers": {
    "github": {
      "command": "node",
      "args": ["/path/to/server.js"],
      "env": {
        "GITHUB_TOKEN": "${GITHUB_TOKEN}"
      },
      "alwaysAllow": ["tool1"],
      "disabled": false
    }
  }
}
```

远程 server 使用 `url` 和 `headers`。新增 server 默认 `disabled: false`，自动批准工具用 `alwaysAllow`。

### Auto Approval 和 Subagents

全局状态 key：

- `autoApprovalSettings.actions.readFiles`
- `autoApprovalSettings.actions.editFiles`
- `autoApprovalSettings.actions.executeSafeCommands`
- `autoApprovalSettings.actions.executeAllCommands`
- `autoApprovalSettings.actions.useBrowser`
- `autoApprovalSettings.actions.useMcp`
- `subagentsEnabled`
- `globalClineRulesToggles`
- `localClineRulesToggles`
- `globalSkillsToggles`
- `hooksEnabled`

Cline subagents 是实验性并行研究工具：只能读文件、搜索、列目录、运行只读命令和使用 skills，不能编辑、浏览、访问 MCP 或继续 spawn。AVM 不把它当作 Agent Profile 的等价物，只能作为 capability 状态显示。

---

## Cursor PoC

Phase 1 只做文件级 PoC：

- MCP：`<project>/.cursor/mcp.json`
- rules：`.cursor/rules/` 或 `.cursorrules`

Cursor adapter 必须在 `status` 中标记为 `partial`，只承诺 MCP/rules 文件渲染，不承诺 Agent Profile、memory、permissions 的完整映射。

---

## OpenClaw 设计约束

OpenClaw 是 gateway/control plane，不是本地 IDE 配置同步工具。Phase 1 不实现 adapter，但统一模型需要保留未来字段。

已确认配置面：

- 配置文件：`OPENCLAW_CONFIG_PATH` 或 `~/.openclaw/openclaw.json`
- 状态目录：`OPENCLAW_STATE_DIR` 或 `~/.openclaw`
- workspace：默认 `~/.openclaw/workspace`，可通过 `agents.defaults.workspace` 配置
- skills：`~/.openclaw/workspace/skills/<skill>/SKILL.md`
- agent 列表：`agents.list[]`
- agent defaults：`agents.defaults`
- channel routing：`bindings[]`
- sandbox：`agents.defaults.sandbox` 或 per-agent `sandbox`
- MCP：`mcp.servers`
- channels：`channels.<provider>`，例如 Slack/Telegram/Feishu/Discord 等

OpenClaw 的关键差异：

- 支持多 channel inbound routing，需要 `bindings.match.channel/accountId/peer` 这类字段。
- 支持 per-agent workspace、agentDir、runtime、sandbox、subagents、skills。
- 暴露到 IM/群聊时默认要考虑 DM pairing、allowlist、sandbox。

因此 AVM 的 Agent Profile 不应只绑定到本地项目路径。Phase 1 先把 workspace、gateway、routing、channel binding 等 OpenClaw 原生字段保存在 `runtime_extensions.openclaw`，不提升为 `workspace_isolation` 统一主干字段；future adapter 通过 mapping status 展示这些字段是否生效。
