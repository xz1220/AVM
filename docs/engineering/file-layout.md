# Agent VM — 文件系统布局

> 最后更新：2026-04-24（v5 — Portable Memory 布局）

本文档定义 AVM 自身目录、项目级配置和各 runtime adapter 的写入目标。

---

## 全局目录 `~/.avm/`

```
~/.avm/
├── config.yaml
├── agents/
│   ├── backend-coder.yaml
│   └── backend-reviewer.yaml
├── envs/
│   ├── default.yaml
│   ├── imported-default.yaml
│   └── coding.yaml
├── registry/
│   ├── skills/
│   │   └── git/
│   │       ├── SKILL.md
│   │       └── meta.yaml
│   ├── mcps/
│   │   └── github.yaml
│   ├── commands/
│   ├── hooks/
│   ├── plugins/
│   └── extensions/
├── memory/
│   ├── user/
│   ├── project/
│   └── local/
├── active/
│   ├── manifest.yaml
│   ├── agents/
│   ├── skills/
│   ├── mcps/
│   ├── memory/
│   └── render/
├── state/
│   ├── sync-state.json
│   ├── current-active
│   ├── import-report.json
│   └── memory-import-report.json
├── backup/
└── cache/
```

### 目录职责

| 路径 | 职责 | 生命周期 |
|------|------|----------|
| `config.yaml` | 全局设置、active profile/env、默认 targets | `avm init` 创建，`avm use` 更新 |
| `agents/` | Agent Profile source of truth | `avm agent create/import` 写入 |
| `envs/` | 多 runtime 激活映射 | `avm env create/import` 写入 |
| `registry/skills/` | skill 原始目录池 | init/import/install 写入 |
| `registry/mcps/` | MCP 统一定义 | init/import/install 写入 |
| `registry/commands/` | 命令 capability | Phase 1 建模，按 runtime 支持渲染 |
| `registry/hooks/` | hook capability | Phase 1 建模，按 runtime 支持渲染 |
| `memory/` | portable memory 文件和元数据 | 用户、import 或 memory 命令写入 |
| `active/` | 当前 profile/env 展开结果 | `avm use/sync` 原子重建 |
| `state/` | hash、render status、当前 active 展示缓存、导入报告、memory import 报告 | init/sync/shell/memory 写入 |
| `backup/` | runtime managed paths 写前备份 | sync 写入，按保留数清理 |
| `cache/` | 模板、下载缓存 | 可安全删除 |

---

## Shell 激活展示

`state/current-active` 是 shell prompt 的一行文本缓存，由 `avm use`、`avm sync` 和 `avm deactivate` 更新：

```text
profile:backend-coder
env:backend-dev
```

`~/.avm/config.yaml.active` 仍是 active profile/env 的持久 source of truth；`state/current-active` 只是为了让 shell hook 快速读取，不需要解析 YAML。

用户可以在 shell rc 中启用：

```bash
eval "$(avm shell init zsh)"
```

启用后 prompt 示例：

```text
(avm:backend-coder) ~/repo $
```

Phase 1 的 shell prompt 只展示持久 active profile/env，不创建 shell-local runtime 配置目录。多个 Agent CLI 能使用当前 profile/env，依赖的是 `avm use` 已经渲染 runtime 原生配置，而不是 prompt 本身。

---

## Active 目录

`active/` 是 adapter 的输入，不是用户编辑区。

```
~/.avm/active/
├── manifest.yaml
├── agents/
│   └── backend-coder.yaml
├── skills/
│   └── git -> ../../registry/skills/git
├── mcps/
│   └── github.yaml
├── memory/
│   └── backend-standards.md -> ../../memory/project/backend-standards.md
└── render/
    ├── claude-code/
    │   ├── backend-coder.md
    │   └── .mcp.json
    ├── codex/
    │   ├── config.fragment.toml
    │   └── agents/backend-coder.toml
    └── cline/
        ├── cline_mcp_settings.fragment.json
        └── rules/backend-coder.md
```

`render/` 存放 adapter 生成的中间产物，便于 diff、status 和 debug。真正写入 runtime 前，sync 会对目标文件做冲突检测和备份。

---

## 项目目录 `<project>/.avm/`

```
<project>/.avm/
├── env.yaml
└── agents/
    └── project-only-agent.yaml
```

项目级配置用于覆盖全局 Environment 的 runtime 绑定：

- `env.yaml` 必须 `extends` 一个全局环境。
- `agents/` 可放项目私有 Agent Profile。
- 默认建议加入 `.gitignore`，除非团队明确希望共享。

---

## Runtime 写入目标

### Claude Code

```
~/.claude/
├── settings.json
├── agents/
│   └── backend-coder.md
├── skills -> ~/.avm/active/skills
└── agent-memory/
    └── backend-coder/MEMORY.md

<project>/
├── .claude/
│   ├── settings.json
│   ├── settings.local.json
│   ├── agents/backend-coder.md
│   └── agent-memory/backend-coder/MEMORY.md
└── .mcp.json
```

AVM Phase 1 策略：

- agent: 写 `.claude/agents/<name>.md` 或 `~/.claude/agents/<name>.md`。
- skills: 可把 `~/.claude/skills` 指向 `~/.avm/active/skills`。
- MCP: 优先写项目 `.mcp.json`，或 settings 中 AVM 管理 section。
- settings: 只改结构化字段，例如 `permissions`、`agent`、`hooks`、MCP allow/deny。
- memory: `avm use` 只渲染 AVM memory 引用；native memory 内容的读写通过显式 `avm memory import/pull/push` 完成。
- 不默认覆盖 `CLAUDE.md`。

### Codex

```
~/.codex/
├── config.toml
└── agents/
    └── backend-coder.toml

<project>/
└── AGENTS.md
```

AVM Phase 1 策略：

- profile: 写 active-level `[profiles.avm-<active>]`，并把 `profile` 指向当前 active profile/env。
- MCP: 写 `[mcp_servers.<name>]`。
- role: 写 `[agents.<role>]` 和 `.codex/agents/<role>.toml`。
- instructions: 写 role TOML 的 `developer_instructions`；不默认覆盖项目 `AGENTS.md`。

### OpenCode

```
~/.avm/runtime-homes/<active>/opencode/
├── opencode.json
├── agents/
│   └── backend-coder.md
└── skills/
    └── test/
        └── SKILL.md
```

AVM Phase 1 策略：

- activation: 导出 `OPENCODE_CONFIG` 和 `OPENCODE_CONFIG_DIR` 指向 isolated runtime home。
- config: 写 `default_agent`、`permission` 和 `mcp`。
- agent: 写 `agents/<agent>.md`，不写用户全局或项目 `.opencode/agents`。
- skills: 仅复制 active skill set 中有 `SKILL.md` source 的条目。
- instructions: reasoning、verbosity、unresolved skill names 写入 agent body。

### Cline

```
~/.cline/
└── data/
    ├── globalState.json
    ├── workspace/
    │   └── workspaceState.json
    └── settings/
        └── cline_mcp_settings.json

<project>/
└── .clinerules/
    └── avm/
        └── backend-coder.md
```

VS Code/Cursor 扩展形态不一定写 `~/.cline/data`，而是写 IDE `globalStorage` 下的 Cline 扩展目录，例如 `.../globalStorage/saoudrizwan.claude-dev/settings/cline_mcp_settings.json`。adapter detect 需要同时支持这两类路径。

AVM Phase 1 策略：

- MCP: 写 `cline_mcp_settings.json` 的 `mcpServers`，保留非 AVM server。
- rules: agent instructions 渲染到 `.clinerules/avm/<agent>.md`。
- permissions: 可写 `autoApprovalSettings` 的安全子集。
- subagents: 只读/写 `subagentsEnabled` 开关状态，不把 Cline subagent 当 Agent Profile。

### Cursor PoC

```
<project>/
├── .cursor/
│   ├── mcp.json
│   └── rules/
└── .cursorrules
```

Phase 1 只承诺：

- MCP 写 `.cursor/mcp.json`。
- rules/instructions 写 AVM 管理的 rules 文件。
- status 标记 `partial`。

### OpenClaw（未来约束）

```
~/.openclaw/
├── openclaw.json
└── workspace/
    ├── AGENTS.md
    ├── SOUL.md
    ├── TOOLS.md
    └── skills/<skill>/SKILL.md
```

OpenClaw adapter 不在 Phase 1 实现，但数据模型要能通过 `runtime_extensions.openclaw` 承载：

- `agents.list[]`
- `agents.defaults.workspace`
- `bindings[]` channel/account/peer routing
- `agents.defaults.sandbox` 和 per-agent sandbox
- `mcp.servers`
- `channels.<provider>`

---

## 备份布局

```
~/.avm/backup/
└── 2026-04-24T10-30-00/
    ├── claude-code/
    │   ├── settings.json
    │   └── agents/backend-coder.md
    ├── codex/
    │   ├── config.toml
    │   └── agents/backend-coder.toml
    └── cline/
        ├── globalState.json
        └── settings/cline_mcp_settings.json
```

规则：

- 只备份将要覆盖的 managed paths。
- 备份路径保留 runtime 相对路径。
- 超过 `backup_max_count` 删除最旧快照。
- export 默认不包含 backup。

---

## 权限和敏感信息

| 文件/目录 | 权限 | 说明 |
|----------|------|------|
| `~/.avm/` | `0700` | 用户级数据目录，默认不允许其他本机用户读取 |
| `~/.avm/**/*.yaml` | `0600` | 不应包含明文 secrets，但可能包含本地路径和工具信息 |
| `~/.avm/memory/` | `0700` | 可能包含个人偏好，export 默认需确认 |
| `~/.avm/backup/` | `0700` | 可能包含 runtime 配置快照 |
| runtime secrets 文件 | 保持原权限 | AVM 写入时不得放宽权限 |

MCP token 使用 `${ENV_VAR}` 引用。adapter 不应展开并写入明文 token，除非 runtime 明确只能接受明文且用户显式允许。

---

## 生命周期

### `avm init`

1. 创建目录结构。
2. 扫描 installed runtimes。
3. 导入 agents/capabilities 到 `~/.avm/`。
4. 创建 default Agent Profile 和 default env。
5. 初始化 sync state。

### `avm agent create`

1. 校验 name/runtime。
2. 根据 runtime 模板生成 `agents/<name>.yaml`。
3. 可选直接 `avm use <name>` 激活。

### `avm env create`

1. 校验 runtime_agents 引用的 Agent Profile。
2. 写全局 env 或项目 `.avm/env.yaml`。

### `avm use`

1. 解析 active profile 或 env + project override。
2. 原子重建 `active/`。
3. 并发调用 adapter plan/render。
4. 更新 state 和 active profile/env。

### `avm clean`

默认只清理：

- `state/sync-state.json`
- `cache/`
- 可选清理 `backup/`

不删除 `agents/`、`envs/`、`registry/`、`memory/`。
