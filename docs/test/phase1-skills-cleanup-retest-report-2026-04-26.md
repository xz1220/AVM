# Agent VM Phase 1 Skills Cleanup 复测报告

> 日期：2026-04-26
> 目标：复验 runtime skills stale 清理链路
> 结论：通过。`probe-skill` 在 active 时可被 Codex / Claude runtime 看到；切换到 no-skill profile 后，active、Codex runtime、Claude runtime 均清理，Codex 不再列出该 skill。

## 1. 测试环境

- Repo：`/Users/danielxing/code/agent-vm`
- AVM 安装方式：`GOBIN=$TEST_ROOT/bin go install ./cmd/avm`
- 测试目录：`/tmp/avm-skills-cleanup.00cPSY`
- 日志目录：`/tmp/avm-skills-cleanup.00cPSY/logs`
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

## 2. 基础验证

```text
go test ./internal/adapter/codex ./internal/adapter/claude ./internal/sync ./cmd/avm \
  -run 'TestRenderLinksActiveSkillDirectories|TestSyncActivationCleansStaleRuntimeSkillsFromPreviousActivation|TestUseActivatesSkillContentForRuntimeSkillDirs' \
  -count=1
PASS

go test ./...    PASS
go vet ./...     PASS
git diff --check PASS
```

## 3. 黑盒结果

```text
PASS=44
FAIL=0
SKIP=1
```

`SKIP=1`：本机未安装 Cline CLI；Cline 文件级规则已验证。

## 4. 激活 skill-env 验证

测试创建：

```text
$HOME/.avm/registry/skills/probe-skill/SKILL.md
```

marker：

```text
AVM_SKILL_PROBE_MARKER_20260426
```

执行：

```bash
avm env create skill-env \
  --codex codex-skill-agent \
  --claude-code claude-skill-agent \
  --cline cline-skill-agent \
  --cursor cursor-skill-agent
avm use --kind env skill-env
```

已验证：

- `~/.avm/active/skills/probe-skill/SKILL.md` 存在并包含 marker。
- `~/.avm/active/skills/unused-skill` 不存在。
- `$CODEX_HOME/skills/probe-skill/SKILL.md` 存在并包含 marker。
- `$CLAUDE_CONFIG_DIR/skills/probe-skill/SKILL.md` 存在并包含 marker。
- Codex / Claude runtime skill 文件含 `avm_managed: true`。
- `$CODEX_HOME/skills/unused-skill` 和 `$CLAUDE_CONFIG_DIR/skills/unused-skill` 均不存在。
- Cline rules 包含 active skill path。
- `codex debug prompt-input "skill probe"` 退出码为 0，并列出 `probe-skill`。

## 5. 切换 no-skill 验证

执行：

```bash
avm agent create no-skill-agent --runtime codex --model gpt-5.4 --reasoning low
avm use --kind profile no-skill-agent
```

已验证：

- `~/.avm/active/skills/probe-skill/SKILL.md` 被删除。
- `$CODEX_HOME/skills/probe-skill/SKILL.md` 被删除。
- `$CLAUDE_CONFIG_DIR/skills/probe-skill/SKILL.md` 被删除。
- active / Codex runtime / Claude runtime 目录中不再出现 `AVM_SKILL_PROBE_MARKER_20260426`。
- Codex selector 切到 `avm-no-skill-agent`。
- `codex debug prompt-input "after no skill"` 退出码为 0。
- `codex debug prompt-input "after no skill"` 不再列出 `probe-skill`。

## 6. 结论

上一轮 blocker 已修复：

```text
skill registry 解析：通过
active/skills 当前集合：通过
Codex runtime skill 安装：通过
Claude runtime skill 安装：通过
Codex skill discovery：通过
切换 profile 后 stale runtime skill 清理：通过
Cline runtime probe：未验证，本机无 Cline CLI
Cursor skills：Phase 1 unsupported
```

当前可对外描述为：Phase 1 在 Codex / Claude Code 上的 skills 正向激活和切换清理链路已通过黑盒验证；Cline 仍只做文件级验证，Cursor skills 仍是 unsupported。
