# Agent VM — 验收标准

> 最后更新：2026-04-29（v10 — Create wizard and runtime discovery UX）

本文档定义 Phase 1 MVP 的验收标准，并标注当前 `main` 的可执行基线。验收重点是 Agent Profile、能力引用、多 runtime Environment 映射、Claude Code/Codex/OpenCode/Cline/Cursor adapter、render mapping 和数据安全。

---

## 验收原则

1. 当前已合入的 Phase 1 命令可用：`init`、`create`、`skill list`、`runtime list/scan`、`agent create/list/show`、`agent show --runtime`、`env create`、`env create --local`、`memory import --dry-run`、`use/status/deactivate`、`sync`、`shell init`、`export/import`。
2. `~/.avm` 是 Agent Profile 的 source of truth。
3. `avm init` 只写 `~/.avm/**` 默认配置和 state，包括 read-only runtime scan 的 `state/import-report.json`；不得修改 runtime 配置。
4. 不默认覆盖用户 instruction 文件。
5. adapter 不静默丢字段，必须记录 mapping status。
6. 写 runtime 文件前有冲突检测和备份。
7. export/import 能迁移 Agent Profile、Environment 及已存在的被引用 capability/memory 文件；更完整 package scope 和交互式冲突处理属于后续策略。

---

## 1. `avm init`

### 1.1 首次初始化

```bash
avm init
```

预期：

- 创建 `~/.avm/config.yaml`。
- 创建 `agents/ envs/ registry/ memory/ active/ state/ backup/ cache/`。
- 创建 `agents/default.yaml`。
- 创建 `envs/default.yaml`。
- 创建 `state/sync-state.json`。
- 退出码 0。

### 1.2 已存在配置

```bash
avm init
```

预期：

- 报错提示使用 `--force`。
- 不删除已有 `~/.avm/`。
- 退出码 1。

### 1.3 Runtime import-report

前置：

- 存在 `~/.codex/config.toml`，包含 profile/MCP。
- 存在 Claude Code `.claude/agents/reviewer.md`。

预期：

- 写入 `state/import-report.json`，包含 version、generated_at、runtimes[]、found/config_dir/version、agent_candidates、warnings/errors。
- 不创建 imported agent/env。
- 不自动激活 imported candidates。
- `~/.codex/config.toml` 和 `.claude/agents/reviewer.md` 内容 hash 不变。

### 1.4 只读初始化

前置：

- 准备 Claude Code、Codex、OpenCode、Cline、Cursor 的 fixture 配置文件。

执行：

```bash
avm init
```

预期：

- 只创建或修改 `~/.avm/**`。
- 不创建、不修改、不删除任何 runtime 配置文件。
- 写入 `state/import-report.json`，并按 runtime 区分 found、agent candidates、warnings 和 errors。

---

## 2. `avm agent create/list/show`

### 2.1 创建 Codex Agent Profile

```bash
avm agent create backend-coder \
  --runtime codex \
  --model gpt-5.4 \
  --reasoning medium \
  --skills test,migration \
  --mcps github,postgres-readonly \
  --memory project-architecture
```

预期：

- 创建 `~/.avm/agents/backend-coder.yaml`。
- `model_run.model = gpt-5.4`。
- `model_run.reasoning_effort = medium`。
- `runtime.preferred = codex`。
- `runtime.kind = local`。
- `capabilities.skills = [test, migration]`。
- `capabilities.mcps = [github, postgres-readonly]`。
- `memory_refs` 包含 `project-architecture`。
- `runtime_extensions.codex` 存在或为空 map。
- 不写 `~/.codex/config.toml`。

### 2.2 创建 Claude Code agent

```bash
avm agent create reviewer --runtime claude-code --scope global
```

预期：

- 创建 agent YAML。
- 默认 permissions/sandbox 合法。
- 不写 `.claude/agents/reviewer.md`，直到 `avm use/sync`。

### 2.3 列表

```bash
avm agent list
```

预期显示：

- `NAME`
- `SCOPE`
- `VERSION`
- `DESCRIPTION`

### 2.4 映射预览

```bash
avm agent show backend-coder --runtime codex
```

- 输出 adapter mapping preview，而不是 agent YAML。
- 显示 runtime、agent、managed_paths、warnings。
- 按 native、rendered_as_instructions、ignored、unsupported 分组显示 mappings。
- 对 memory/skills 在 Codex 中标记 `rendered_as_instructions`。
- 不调用 adapter Render，不创建或修改 runtime managed files。

---

## 3. `avm env create`

### 3.1 创建全局 Environment runtime 映射

```bash
avm env create backend-dev \
  --codex backend-coder \
  --claude-code code-reviewer \
  --opencode opencode-coder \
  --cline backend-assistant \
  --cursor cursor-helper
```

预期：

- 创建 `~/.avm/envs/backend-dev.yaml`。
- `runtime_agents.codex.primary = backend-coder`。
- `runtime_agents.claude-code.primary = code-reviewer`。
- `runtime_agents.opencode.primary = opencode-coder`。
- `runtime_agents.cline.primary = backend-assistant`。
- `runtime_agents.cursor.primary = cursor-helper`。
- env YAML 不包含 `capabilities` 或 `memory_layers`。
- 引用不存在时失败。

### 3.2 创建项目级覆盖

```bash
avm env create backend-dev --local --codex project-backend-coder
```

预期：

- 创建 `<project>/.avm/env.yaml`。
- 显式传入 env name 时包含 `extends: backend-dev`；未传 name 时要求当前 active 是 env，并使用当前 active env。
- 覆盖 `runtime_agents.codex.primary`，不影响其他 runtime 绑定。

### 3.3 不允许在 Environment 声明能力

前置：用户误把能力写入 env。

项目 `.avm/env.yaml`：

```yaml
extends: backend-dev
capabilities:
  mcps: [github]
```

预期：

- `avm use backend-dev` 校验失败。
- 错误提示 capabilities 应写入 Agent Profile。

---

## 4. `avm use`

### 4.1 激活单个 Agent Profile

```bash
avm use backend-coder
```

预期：

- 重建 `~/.avm/active/manifest.yaml`。
- `active/agents/` 包含 `backend-coder.yaml`。
- `active/skills/` 只包含 `backend-coder` 引用的 symlink。
- `active/mcps/` 只包含 `backend-coder` 引用的 MCP YAML。
- 更新 `config.yaml.active.kind = profile`。
- 更新 `config.yaml.active.name = backend-coder`。
- 更新 `state/current-active = profile:backend-coder`。
- 更新 `sync-state.json`。
- 普通本地 fixture 耗时 < 500ms。
- 只写 adapter 声明的 managed paths 或 AVM 管理片段。
- 每个 target 都写入 render mapping status。
- target 部分失败时仍保留 AVM active composition，并以非 0 退出码和 `avm status` 暴露失败 runtime。

### 4.2 激活多 runtime Environment

```bash
avm use --kind env backend-dev
```

预期：

- `active/manifest.yaml` 记录 `active.kind = env` 和 `active.name = backend-dev`。
- `runtime_agents.codex = backend-coder`。
- `runtime_agents.claude-code = code-reviewer`。
- `runtime_agents.opencode = opencode-coder`。
- `runtime_agents.cline = backend-assistant`。
- `runtime_agents.cursor = cursor-helper`。
- Codex 只渲染 `backend-coder` 的 capabilities/memory refs。
- Claude Code 只渲染 `code-reviewer` 的 capabilities/memory refs。
- OpenCode 只渲染 `opencode-coder` 的 capabilities/memory refs。
- Cline 只渲染 `backend-assistant` 的 capabilities/memory refs。
- Cursor 只渲染 `cursor-helper` 的 rules/MCP PoC。
- 更新 `state/current-active = env:backend-dev`。

### 4.3 Claude Code 输出

前置：target 包含 `claude-code`。

预期：

- 写 `.claude/agents/<agent>.md` 或 `~/.claude/agents/<agent>.md`。
- agent frontmatter 含 `name`、`description`、`tools/skills/mcpServers` 可表达字段。
- 写项目 `.mcp.json` 或 settings 中 AVM 管理 MCP section。
- `~/.claude/skills` 指向 `~/.avm/active/skills`，或 adapter 标记未启用 symlink。
- 不覆盖 `CLAUDE.md`。

### 4.4 Codex 输出

前置：target 包含 `codex`。

预期：

- 单 profile 激活时，`~/.codex/config.toml` 包含 active-level `[profiles.avm-backend-coder]`，并将 `profile` 指向当前 active profile。
- env 激活时，`~/.codex/config.toml` 包含 `[profiles.avm-backend-dev]`，但 role TOML 来自 `runtime_agents.codex.primary`。
- `~/.codex/config.toml` 包含 `[mcp_servers.<name>]`。
- `~/.codex/config.toml` 包含 `[agents.<role>]`。
- `.codex/agents/<role>.toml` 包含 `name`、`description`、`developer_instructions`。
- 不覆盖项目 `AGENTS.md`。
- `sync-state.json` 记录 skills/memory `rendered_as_instructions`。

### 4.5 OpenCode 输出

前置：target 包含 `opencode`。

预期：

- `avm activate` 导出 `OPENCODE_CONFIG=<runtime-home>/opencode.json`。
- `avm activate` 导出 `OPENCODE_CONFIG_DIR=<runtime-home>`。
- 写 `<runtime-home>/opencode.json`，包含 `default_agent`、`permission` 和可表达 MCP。
- 写 `<runtime-home>/agents/<agent>.md`。
- 写 `<runtime-home>/skills/<skill>/SKILL.md`，仅清理 AVM-managed stale skill。
- 不覆盖 `~/.config/opencode` 或项目 `.opencode/`。

### 4.6 Cline 输出

前置：target 包含 `cline`。

预期：

- 更新 `~/.cline/data/settings/cline_mcp_settings.json` 的 `mcpServers`。
- 若检测到 VS Code/Cursor 扩展安装，则可写 IDE `globalStorage` 下对应的 `settings/cline_mcp_settings.json`。
- 保留已有非 AVM MCP server。
- 写 `<project>/.clinerules/avm/<agent>.md`。
- 保留已有 `.clinerules/` 文件。
- 高风险 `autoApprovalSettings.actions.executeAllCommands` 默认不被打开。

### 4.7 Cursor PoC

前置：target 包含 `cursor`。

预期：

- 写 AVM 管理的 rules 文件。
- 当 agent 有 MCP refs 时，写入或合并 `<project>/.cursor/mcp.json`。
- 成功写入时 runtime status 保持 `synced`。
- Phase 1 partial support 必须通过 warnings 和 mapping status 明确展示 native、rendered_as_instructions、ignored、unsupported 的边界。

---

## 5. Shell 激活展示

### 5.1 初始化 shell hook

```bash
avm shell init zsh
avm shell init bash
avm shell init fish
```

预期：

- 输出可被 `eval` 的 shell 脚本。
- 脚本只读取 `~/.avm/state/current-active` 或等价只读状态。
- 不修改 runtime 配置文件。
- 不要求每次 prompt 渲染都解析 `~/.avm/config.yaml`。

### 5.2 Prompt 显示 active profile/env

前置：

- 已执行 `avm use backend-coder`。
- shell 已启用 `eval "$(avm shell init zsh)"`。

预期：

- prompt 显示 `(avm:backend-coder)` 或按配置等价显示。
- 执行 `avm use tech-writer` 后，下一次 prompt 显示 `(avm:tech-writer)`。
- 执行 `avm use backend-dev` 后，下一次 prompt 显示 `(avm:backend-dev)`。
- 在同一 shell 中运行 `codex`、`claude`、`opencode`、`cline` 时，使用的是 `avm use` 已渲染的当前 runtime 配置。

### 5.3 退出环境

```bash
avm deactivate
```

预期：

- 等价于 `avm use default`。
- 更新 `config.yaml.active.kind = profile`。
- 更新 `config.yaml.active.name = default`。
- 更新 `state/current-active = profile:default`。
- prompt 显示 `(avm:default)` 或按配置等价显示。

---

## 6. 冲突和备份

### 6.1 外部修改检测

前置：

- 上次 sync 后用户手动改了 `~/.codex/config.toml`。

执行：

```bash
avm sync
```

预期：

- 检测 hash 不匹配。
- prompt 策略下给出选择。
- 选择 local-wins 时跳过 Codex，其他 runtime 继续。

### 6.2 自动备份

预期：

- 写入前创建 `~/.avm/backup/<timestamp>/<runtime>/...`。
- 备份包含将被覆盖的 managed paths。
- 备份失败时不覆盖目标文件。

---

## 7. `avm status`

```bash
avm status
```

预期输出包含：

- active profile/env。
- targets 状态：synced/skipped/failed；partial adapter support 通过 warnings 和 mapping status 展示。
- managed paths。
- ignored/unsupported/rendered mappings。
- warnings；冲突会以 failed runtime error/warning 暴露。
- 耗时 < 500ms。

示例：

```text
active: env:backend-dev
runtime status:
  cline: synced (agent backend-assistant)
  codex: synced (agent backend-coder)
  cursor: synced (agent cursor-helper)
managed paths:
  codex:
    - ~/.codex/config.toml owner=shared-section merge=structured-section
mapping status:
  cursor:
    - model_run.model: unsupported (Cursor Phase 1 has no stable local Agent Profile field)
warnings:
  - cursor: Cursor Phase 1 support is partial; rules/MCP are rendered, profile semantics are reported as mappings.
```

---

## 8. `avm memory import --dry-run`

```bash
avm memory import --from testdata/memory/backend-standards.md --dry-run
avm memory import --from testdata/memory/backend-standards.md --dry-run --format json
```

预期：

- 不修改 runtime native memory。
- 不写入 `~/.avm/memory/` 正式文件。
- 输出可迁移 memory candidates。
- 输出 new/changed/conflict/skipped diff 状态。
- 当前 `main` 只输出 report，不持久化 `~/.avm/state/memory-import-report.json`。
- 对无法安全归一化的条目标记 candidate 或 skipped，不静默丢弃。

---

## 9. Export / Import

### 9.1 Export

```bash
avm export backend-coder --output backend-coder.avm.zip
avm export backend-dev --output backend-dev.avm.zip
```

预期包内包含：

- `manifest.yaml`
- agent YAML, or env YAML plus referenced agent YAML
- referenced MCP/skill metadata/files
- referenced memory files

默认不包含：

- runtime 输出文件
- backup
- 明文 secrets

### 9.2 Import

```bash
avm import backend-coder.avm.zip
```

预期：

- 校验 manifest version。
- 同名同内容 skip。
- 同名不同内容失败并报告 package import conflict；交互式 rename/overwrite 属于后续 package policy。
- import 后不自动 `avm use`。

---

## 10. 数据安全

必须满足：

- 不默认覆盖 `AGENTS.md`、`CLAUDE.md`、`.cursorrules`。
- JSON/TOML 用结构化 parser 合并。
- `${ENV_VAR}` 不被展开写成明文。
- `~/.avm` 和 `~/.avm/memory` 默认权限不放宽到其他本机用户可读。
- backup 权限为 `0700` 或不放宽原文件权限。
- adapter unsupported 字段可见。

---

## 11. 性能指标

| 操作 | 目标 |
|------|------|
| `avm agent list` | < 100ms |
| `avm status` | < 500ms |
| `avm use backend-coder` | < 500ms（fixture：1 agent、3 skills、2 MCP、1 target） |
| `avm use backend-dev` | < 500ms（fixture：3 agents、3 skills、2 MCP、3 targets） |
| `avm memory import --dry-run` | < 1 秒（fixture：1 个 runtime memory 文件，10 条以内） |
