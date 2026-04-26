# Agent VM Phase 1 Skills 黑盒复测报告

> 日期：2026-04-26
> 目标：复验 skills 修复后的 registry -> active -> runtime skills 全链路
> 结论：部分通过。含 skill 的激活链路已修复，但切换到不带 skill 的 profile 后，Codex / Claude runtime skills 目录残留旧 skill，导致 Codex 仍能看到已不属于当前 profile 的 skill。

## 1. 测试环境

- Repo：`/Users/danielxing/code/agent-vm`
- AVM 安装方式：`GOBIN=$TEST_ROOT/bin go install ./cmd/avm`
- 测试目录：`/tmp/avm-skills-retest.PL8CPd`
- 日志目录：`/tmp/avm-skills-retest.PL8CPd/logs`
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
go test ./...      PASS
go vet ./...       PASS
git diff --check   PASS
```

## 3. 黑盒结果

原始结果：

```text
PASS=43
FAIL=3
SKIP=2
```

`SKIP=2`：

- Codex `debug prompt-input` 会列出 skill，但不内联 `SKILL.md` body marker；按 Codex lazy-load 行为标为 SKIP。
- 本机未安装 Cline CLI。

`FAIL=3` 都来自切换到 no-skill profile 后的 runtime 旧 skill 残留。

## 4. 已修复部分

测试创建真实 skill：

```text
$HOME/.avm/registry/skills/probe-skill/SKILL.md
```

其中包含 marker：

```text
AVM_SKILL_PROBE_MARKER_20260426
```

激活 `skill-env` 后已通过：

- `~/.avm/active/skills/probe-skill/SKILL.md` 存在。
- active skill 文件包含 marker。
- `~/.avm/active/skills/unused-skill` 不存在。
- `~/.avm/active/manifest.yaml` 包含 `probe-skill`，不包含 `unused-skill`。
- `$CODEX_HOME/skills/probe-skill/SKILL.md` 存在并包含 marker。
- `$CLAUDE_CONFIG_DIR/skills/probe-skill/SKILL.md` 存在并包含 marker。
- Codex / Claude runtime skill 文件包含自动补的 frontmatter：

```yaml
name: "probe-skill"
description: "AVM skill probe-skill."
```

- Codex role、Claude agent、Cline rules 都包含 active skill path：

```text
$HOME/.avm/active/skills/probe-skill/SKILL.md
```

- `codex debug prompt-input "skill probe"` 退出码为 0，并能列出：

```text
probe-skill: AVM skill probe-skill.
```

这说明 registry -> active -> Codex runtime skill discovery 的正向链路已经生效。

## 5. 仍失败部分

复测切换到无 skill profile：

```bash
avm agent create no-skill-agent --runtime codex --model gpt-5.4 --reasoning low
avm use --kind profile no-skill-agent
```

`~/.avm/active/skills/probe-skill/SKILL.md` 已被移除，这部分通过。

但 runtime 目录仍残留：

```text
$CODEX_HOME/skills/probe-skill/SKILL.md
$CLAUDE_CONFIG_DIR/skills/probe-skill/SKILL.md
```

两个文件仍包含：

```text
AVM_SKILL_PROBE_MARKER_20260426
```

实际 Codex probe 确认影响：

```bash
codex debug prompt-input "after no skill"
```

结果：

```text
probe-skill still listed
```

prompt input 中仍有：

```text
- probe-skill: AVM skill probe-skill. (file: .../home/.codex/skills/probe-skill/SKILL.md)
```

这表示当前 profile 已经不引用 `probe-skill`，但 Codex runtime 仍会把它当作可用 skill。

## 6. 可能原因

Codex / Claude adapter 已把 skill 文件加入当前 render plan 的 `ManagedPaths` 和 write operations：

```text
$CODEX_HOME/skills/<skill>/SKILL.md
$CLAUDE_CONFIG_DIR/skills/<skill>/SKILL.md
```

但当下一次 active 不包含这个 skill 时：

- 新 render plan 不再包含旧 skill managed path。
- adapter 只执行当前 operations。
- sync/status 也只记录当前 managed paths。
- 旧 runtime whole-file managed artifact 没有被删除或标记 stale。

结果就是 active tree 是正确的，但 runtime skills 目录不是当前 active 的闭包。

## 7. 结论

当前 skills 状态应更新为：

```text
skill registry 解析：通过
active/skills 当前集合：通过
Codex/Claude runtime skill 安装：通过
Codex skill discovery：通过
切换 profile 后 runtime skill 清理：未通过
Cline runtime probe：未验证
Cursor skills：Phase 1 unsupported
```

因此还不能说 skills 全链路完全可用。对于实际用户，风险是：切换到不带某个 skill 的 profile 后，Codex 仍可能继续看到旧 skill。

## 8. 建议修复

1. 对 Codex / Claude runtime skill 目录引入 AVM-owned manifest，例如记录上次写入的 skill names。
2. 每次 `avm use` 后删除上次由 AVM 写入、但当前 active 不再引用的 `$RUNTIME_HOME/skills/<skill>`。
3. 删除时只处理 AVM-owned skill，避免删除用户手动安装的 runtime skills。
4. `sync-state` 可以记录 managed whole-file artifact 的过期清理结果。
5. 修复后重跑本报告同样的 no-skill 切换用例，并确认 `codex debug prompt-input` 不再列出 `probe-skill`。
