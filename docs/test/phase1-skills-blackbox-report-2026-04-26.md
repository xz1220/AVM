# Agent VM Phase 1 Skills 黑盒测试报告

> 日期：2026-04-26
> 目标：验证真实 skill 内容是否随 `avm use` 激活，并被 Codex / Claude Code / Cline 读取
> 结论：未通过。当前实现只渲染 skill 名称或 metadata，没有把 registry 中的 skill 内容安装、挂载或同步到 runtime 可读取位置。

## 1. 测试环境

- Repo：`/Users/danielxing/code/agent-vm`
- AVM 安装方式：`GOBIN=$TEST_ROOT/bin go install ./cmd/avm`
- 测试目录：`/tmp/avm-skills.DuSylP`
- 日志目录：`/tmp/avm-skills.DuSylP/logs`
- Codex CLI：`codex-cli 0.125.0`
- Claude Code CLI：`2.1.119 (Claude Code)`
- Cline CLI：未安装，runtime probe 标记为 `SKIP`

隔离变量：

```bash
HOME="$TEST_ROOT/home"
CODEX_HOME="$HOME/.codex"
CLAUDE_CONFIG_DIR="$HOME/.claude"
CLINE_DATA_HOME="$HOME/.cline/data"
PROJECT_ROOT="$TEST_ROOT/project"
PATH="$TEST_ROOT/bin:$ORIGINAL_PATH"
```

## 2. 测试数据

创建真实 skill registry：

```text
$HOME/.avm/registry/skills/probe-skill/
├── SKILL.md
└── meta.yaml
```

`SKILL.md` 中包含唯一 marker：

```text
AVM_SKILL_PROBE_MARKER_20260426
```

同时创建未引用的 `unused-skill`，用于验证 active tree 不应包含未引用 skill。

创建并激活：

```bash
avm agent create codex-skill-agent --runtime codex --skills probe-skill
avm agent create claude-skill-agent --runtime claude-code --skills probe-skill
avm agent create cline-skill-agent --runtime cline --skills probe-skill
avm agent create cursor-skill-agent --runtime cursor --skills probe-skill
avm env create skill-env \
  --codex codex-skill-agent \
  --claude-code claude-skill-agent \
  --cline cline-skill-agent \
  --cursor cursor-skill-agent
avm use --kind env skill-env
```

## 3. 执行结果

原始结果：

```text
PASS=25
FAIL=9
SKIP=1
```

其中两类失败是测试断言噪音：

- Cursor 在本机无 `cursor` runtime 时被标记为 `skipped`，没有生成 Cursor rule。
- `claude agents --setting-sources project` 只列 agent 名称，不展示 agent 的 skills 字段。

去掉以上噪音后，实质问题仍然存在。

## 4. 已验证通过

- `avm init`、agent/env 创建、`avm use --kind env skill-env` 可执行。
- registry 下的 `probe-skill/SKILL.md` 和 `unused-skill/SKILL.md` 都存在。
- `~/.avm/active/agents/` 包含当前 env 的 agent YAML。
- `~/.avm/active/` 没有包含未引用的 `unused-skill` marker。
- Codex role 文件包含 skill 名称 `probe-skill`。
- Claude Code agent 文件包含 `skills: ["probe-skill"]` 和正文中的 `Active AVM skills`。
- Cline rules 文件包含 skill 名称 `probe-skill`。

## 5. 失败点

### P0：`~/.avm/active/skills` 没有包含当前 profile 引用的 skill

实际 active tree：

```text
$HOME/.avm/active/
├── agents/
├── commands/
├── hooks/
├── manifest.yaml
├── mcps/
├── memory/
├── render/
└── skills/
```

`skills/` 目录为空：

```text
FAIL: active skills missing probe-skill/SKILL.md
FAIL: active skills does not expose probe marker
```

影响：

- AVM source-of-truth 中有 skill 本体，但 active set 不包含它。
- runtime 即使挂载 `~/.avm/active/skills`，也读不到当前 skill 内容。

### P0：Claude Code 没有 skills mount

检查结果：

```text
FAIL: claude global skills mount missing
FAIL: claude project skills mount missing
FAIL: claude skills mount does not expose probe-skill
```

也就是说以下路径都不存在：

```text
$CLAUDE_CONFIG_DIR/skills
$PROJECT_ROOT/.claude/skills
```

Claude agent 文件虽然包含：

```yaml
skills:
  - "probe-skill"
```

但没有对应 `SKILL.md` 可供 Claude Code 读取。

### P0：Codex runtime 没有看到 AVM skill 内容

Codex role 文件只包含 skill 名称：

```toml
developer_instructions = "Active AVM skills:\n- probe-skill\n\nResponse verbosity:\nnormal"
```

不包含 marker：

```text
AVM_SKILL_PROBE_MARKER_20260426
```

真实 Codex probe：

```bash
codex debug prompt-input "skill probe"
```

结果：

- 命令退出码为 0。
- prompt input 中有 Codex 自带 system skills。
- prompt input 中没有 `probe-skill`。
- prompt input 中没有 `AVM_SKILL_PROBE_MARKER_20260426`。

这说明 AVM 没有把 registry skill 安装到 `$CODEX_HOME/skills`，也没有通过 active skills 目录暴露给 Codex。

### P1：Cline 只收到 skill 名称，没有收到 skill 内容

Cline rules 文件包含：

```md
## Active AVM skills

- probe-skill
```

但不包含 marker。因为本机未安装 Cline CLI，无法验证 extension 真实加载；从文件产物看，当前只是说明性渲染。

## 6. 代码观察

- `internal/sync/active.go` 会创建 `active/skills/` 目录，但 `buildActiveTree` 只写 `active/agents/*.yaml`，没有为 skills 建 symlink 或复制内容。
- `internal/adapter/config_input.go` 的 `capabilityRefs` 只把 skill 名称转为 `CapabilityRef{Name: name}`，没有 path 或 source metadata。
- Codex adapter 把 skills 写入 `developer_instructions`，并在 mapping 中标记为 `rendered_as_instructions`。
- Claude adapter 把 skills 写入 agent frontmatter，但没有创建 `$CLAUDE_CONFIG_DIR/skills` 或 `$PROJECT_ROOT/.claude/skills`。
- Cursor adapter 明确标记 `capabilities.skills` 为 unsupported。

## 7. 结论

当前 AVM 的 skills 状态是：

```text
skill 引用渲染：通过
active skill 内容切换：未通过
Codex skill 内容加载：未通过
Claude Code skill 内容加载：未通过
Cline skill 内容加载：未验证，文件级只看到名称
Cursor skills：Phase 1 unsupported
```

因此，Phase 1 不能宣称 skills 已 runtime 级可用。当前只能说 agent profile 可以携带 skill 引用，并把名称渲染到部分 runtime 文件中。

## 8. 建议修复

1. 在 resolve 或 active rebuild 阶段解析 `registry/skills/<name>/`，校验引用存在。
2. `avm use` 时重建 `~/.avm/active/skills/<name>`，指向或复制 `~/.avm/registry/skills/<name>`。
3. Claude Code adapter 创建 `$CLAUDE_CONFIG_DIR/skills -> ~/.avm/active/skills` 或项目级 `.claude/skills -> ~/.avm/active/skills`。
4. Codex adapter 若支持 `$CODEX_HOME/skills`，应把当前 active skills 安装或链接到 `$CODEX_HOME/skills`，并用 `codex debug prompt-input` 验证能看到 `probe-skill`。
5. Cline 若无 native skills 机制，至少应把 `SKILL.md` 内容或路径渲染进 `.clinerules/avm/<agent>.md`，否则只能称为说明性引用。
6. 修复后重跑本报告同样的 marker 黑盒测试。
