# Agent VM Phase 1 黑盒测试方案

> 日期：2026-04-26
> 目标：用接近真实用户的路径验证 AVM 安装、切换和 runtime 生效情况
> 结论口径：只看外部行为，不依赖 Go test helper 或内部 package

## 0. 用例依赖关系

```text
BBT-001 (安装)
  └─ BBT-002 (init)
       └─ BBT-003 (创建 agent/env)
            └─ BBT-004 (MCP registry)
                 └─ BBT-005 (use env)
                      ├─ BBT-006 (runtime probe)
                      └─ BBT-007 (切换 profile)
                           └─ BBT-008 (deactivate)
                                └─ BBT-009 (幂等性)

BBT-N001 ~ BBT-N005 可在 BBT-005 之后独立执行。
```

如果某个用例失败，其下游用例应标记为 `BLOCKED`，不能标 Pass 或 Fail。

## 1. 为什么改成黑盒测试

之前的测试主要验证 `bin/avm` 在临时目录里写出了哪些文件。这能发现一部分问题，但还不够接近用户真实路径，因为用户真正关心的是：

```text
我安装了 avm
我执行 avm use <profile|env>
我再启动 Codex / Claude Code / Cline / Cursor
它们是否真的按当前 AVM profile 工作
```

因此 Phase 1 的集成测试应该从“检查内部实现和文件产物”升级为“模拟用户实际使用”：

1. 在独立目录安装 AVM。
2. 用独立 `HOME` 和 runtime config dir。
3. 只通过 `PATH` 调用 `avm`，不直接调用 repo 内 helper。
4. 执行用户路径：`init -> agent create -> env create -> use -> status -> runtime probe`。
5. 同时验证 AVM 状态、runtime 配置文件和 runtime 实际读取结果。

## 2. 测试分层

### L0：安装级测试

验证用户能在干净环境里安装并调用 AVM。

```bash
TEST_ROOT="$(mktemp -d)"
export GOBIN="$TEST_ROOT/bin"
export PATH="$GOBIN:$PATH"
go install ./cmd/avm
avm --version
avm --help
```

通过标准：

- `avm` 来自 `$TEST_ROOT/bin/avm`。
- 不依赖 repo 当前工作目录中的 `bin/avm`。
- `avm --help` 能列出 Phase 1 命令。

### L1：AVM 黑盒流程测试

验证 AVM 自己的 source-of-truth、active 状态和 runtime 文件写入。

这一层必须使用隔离环境：

```bash
export HOME="$TEST_ROOT/home"
export CODEX_HOME="$HOME/.codex"                    # Codex adapter: os.Getenv("CODEX_HOME"), 默认 ~/.codex
export CLAUDE_CONFIG_DIR="$HOME/.claude"            # Claude adapter: os.Getenv("CLAUDE_CONFIG_DIR"), 默认 ~/.claude
export OPENCODE_CONFIG_DIR="$HOME/.config/opencode" # OpenCode adapter: os.Getenv("OPENCODE_CONFIG_DIR"), 默认 ~/.config/opencode
export CLINE_DATA_HOME="$HOME/.cline/data"          # Cline adapter: os.Getenv("CLINE_DATA_HOME"), 默认 ~/.cline/data
export PROJECT_ROOT="$TEST_ROOT/project"
mkdir -p "$HOME" "$CODEX_HOME" "$CLAUDE_CONFIG_DIR" "$OPENCODE_CONFIG_DIR" "$CLINE_DATA_HOME" "$PROJECT_ROOT"
cd "$PROJECT_ROOT"
```

> 注：以上环境变量均为各 runtime adapter 在代码中显式支持的隔离机制（见 `internal/adapter/*/`），非 AVM 自定义。Codex、Claude Code、OpenCode、Cline 的隔离目录放在 `$HOME` 默认路径下，是为了让真实 runtime CLI 即使只按默认路径读取配置，也能读到 AVM 写入结果。Cursor adapter 不使用环境变量，而是通过 `projectRoot` 参数定位 `<projectRoot>/.cursor/`。

通过标准：

- `avm init` 只写 `$HOME/.avm/**`。
- `avm use` 后 AVM active、runtime config、managed paths 一致。
- 用户已有 `AGENTS.md`、`CLAUDE.md`、`.cursorrules` 不被覆盖。

### L2：Runtime 生效测试

验证启动 runtime 后，runtime 确实读到了 AVM 切换后的配置。

这一层依赖测试机是否安装真实 runtime。策略：

- 如果 runtime 可执行文件存在，则执行 runtime probe。
- 如果不存在，则标记为 `SKIP`，不能当作 Pass。
- runtime probe 必须运行在隔离 config dir 中，不能读写用户真实配置。

本机可用性探测：

```bash
command -v codex >/dev/null && codex --version
command -v claude >/dev/null && claude --version
command -v cline >/dev/null && cline --version
```

具体 probe 需要按 runtime 能力实现：

| Runtime | Managed Paths | 必须验证 | 可接受 probe |
|---------|---------------|----------|--------------|
| Codex | `$CODEX_HOME/config.toml` (shared-section)、`$CODEX_HOME/agents/<role>.toml` (whole-file) | 当前 active profile selector 指向 `avm-*`，role/config/MCP 被 Codex 读取 | `codex mcp list`、`codex mcp get <name>`、必要时 `codex debug prompt-input`，并配合 config selector 检查 |
| Claude Code | `<projectRoot>/.claude/agents/<agent>.md` (whole-file)、`<projectRoot>/.mcp.json` (shared-section) | agent 文件和 MCP 配置被当前项目读取 | `claude agents --setting-sources project`、`claude mcp list`、`claude mcp get <name>`，并配合项目文件检查 |
| Cline | `<projectRoot>/.clinerules/avm/<agent>.md` (whole-file)、`$CLINE_DATA_HOME/settings/cline_mcp_settings.json` (shared-section) | rules 文件和 MCP settings 被扩展读取 | 当前阶段通常只能做文件级检查；未安装 Cline CLI 时标 `SKIP` |
| Cursor | `<projectRoot>/.cursor/rules/<rule>.md` (whole-file)、`<projectRoot>/.cursor/mcp.json` (shared-section) | rules 文件和 MCP 配置存在且格式有效 | 文件级 PoC；真实 IDE probe 可后补 |

重要：L2 的目标不是“运行一次 AI 任务”，而是证明 runtime 的配置入口已经指向 AVM 当前 profile。

当前测试机若只安装 Codex 和 Claude Code，则执行 Codex / Claude Code 的 L2 probe；Cline 不安装时只执行 L1 文件级检查并把 Cline runtime probe 标为 `SKIP`。

### 并行执行与职责切分

Codex 和 Claude Code 可以由两个人分开测试，但不能共用同一个测试环境。`avm use` 会写 `$HOME/.avm/state/current-active`、`sync-state.json` 以及当前 runtime 配置；如果两个人共用 `TEST_ROOT`、`HOME` 或 `PROJECT_ROOT`，后执行的人会覆盖前一个人的 active 状态和项目级文件。

并行测试必须满足：

- 两个测试进程各自创建独立 `TEST_ROOT`。
- `HOME`、`GOBIN`、`CODEX_HOME`、`CLAUDE_CONFIG_DIR`、`CLINE_DATA_HOME`、`PROJECT_ROOT` 都从各自的 `TEST_ROOT` 派生。
- 两个人可以只读同一个 repo 源码；`go install ./cmd/avm` 的输出必须写入各自 `$TEST_ROOT/bin/avm`。
- 两个人不能在同一个 shell 会话里交替 export 环境变量后继续跑；每个测试应使用独立 shell/session。
- Codex 测试者只执行 Codex L2 probe，并可顺带执行 Cline 文件级检查；Claude Code 测试者只执行 Claude Code L2 probe。
- Cline 未安装时，Cline runtime probe 标记为 `SKIP`，不能算 Pass；`.clinerules` 和 MCP settings 仍可做文件级验证。

推荐命名：

```bash
# Codex 测试者
export TEST_ROOT="$(mktemp -d /tmp/avm-codex.XXXXXX)"

# Claude Code 测试者
export TEST_ROOT="$(mktemp -d /tmp/avm-claude.XXXXXX)"
```

如果需要验证“一个 env 同时切换 Codex + Claude Code + Cline + Cursor”的联动行为，应由一个测试进程单独执行完整 BBT-001 ~ BBT-009，不能拆成两个进程共享同一状态目录。

## 3. 标准测试环境

### Assert 工具函数

所有用例使用统一的 assert 函数，失败时输出上下文并以非零退出码终止：

```bash
PASS=0; FAIL=0; SKIP=0

assert_eq() {
  local label="$1" expected="$2" actual="$3"
  if [ "$expected" = "$actual" ]; then
    printf "  PASS: %s\n" "$label"; PASS=$((PASS+1))
  else
    printf "  FAIL: %s\n    expected: %s\n    actual:   %s\n" "$label" "$expected" "$actual"; FAIL=$((FAIL+1))
  fi
}

assert_grep() {
  local label="$1" pattern="$2" file="$3"
  if grep -q "$pattern" "$file" 2>/dev/null; then
    printf "  PASS: %s\n" "$label"; PASS=$((PASS+1))
  else
    printf "  FAIL: %s — pattern '%s' not found in %s\n" "$label" "$pattern" "$file"; FAIL=$((FAIL+1))
  fi
}

assert_grep_not() {
  local label="$1" pattern="$2" file="$3"
  if grep -q "$pattern" "$file" 2>/dev/null; then
    printf "  FAIL: %s — pattern '%s' found in %s\n" "$label" "$pattern" "$file"; FAIL=$((FAIL+1))
  else
    printf "  PASS: %s\n" "$label"; PASS=$((PASS+1))
  fi
}

assert_file_exists() {
  local label="$1" path="$2"
  if [ -f "$path" ]; then
    printf "  PASS: %s\n" "$label"; PASS=$((PASS+1))
  else
    printf "  FAIL: %s — file not found: %s\n" "$label" "$path"; FAIL=$((FAIL+1))
  fi
}

assert_file_not_exists() {
  local label="$1" path="$2"
  if [ ! -f "$path" ]; then
    printf "  PASS: %s\n" "$label"; PASS=$((PASS+1))
  else
    printf "  FAIL: %s — file should not exist: %s\n" "$label" "$path"; FAIL=$((FAIL+1))
  fi
}

assert_exit_code() {
  local label="$1" expected="$2"
  shift 2
  "$@" >/dev/null 2>&1
  local actual=$?
  if [ "$expected" -eq "$actual" ]; then
    printf "  PASS: %s\n" "$label"; PASS=$((PASS+1))
  else
    printf "  FAIL: %s — expected exit %s, got %s\n" "$label" "$expected" "$actual"; FAIL=$((FAIL+1))
  fi
}

mark_skip() {
  local label="$1" reason="$2"
  printf "  SKIP: %s — %s\n" "$label" "$reason"; SKIP=$((SKIP+1))
}

summary() {
  printf "\n=== Results: %d PASS, %d FAIL, %d SKIP ===\n" "$PASS" "$FAIL" "$SKIP"
  [ "$FAIL" -eq 0 ]
}
```

### 测试目录结构：

```text
$TEST_ROOT/
├── bin/
│   └── avm
├── home/
│   ├── .avm/
│   ├── .codex/                      # CODEX_HOME
│   │   ├── config.toml
│   │   └── agents/                  # Codex agent role 文件写入位置
│   ├── .claude/                     # CLAUDE_CONFIG_DIR
│   └── .cline/
│       └── data/                    # CLINE_DATA_HOME
│           └── settings/
│               └── cline_mcp_settings.json
├── project/                          # PROJECT_ROOT, 模拟用户项目目录
│   ├── AGENTS.md                     # 用户已有文件，不可覆盖
│   ├── CLAUDE.md                     # 用户已有文件，不可覆盖
│   ├── .cursorrules                  # 用户已有文件，不可覆盖
│   ├── .cursor/
│   │   └── mcp.json                  # 用户已有 Cursor MCP 配置
│   ├── .claude/
│   │   └── agents/                   # Claude Code agent 文件写入位置
│   ├── .clinerules/
│   │   └── avm/                      # Cline rules 写入位置
│   └── .mcp.json                     # Claude Code MCP 配置写入位置
└── logs/
```

测试前准备用户已有文件：

```bash
printf 'user AGENTS - must stay\n' > "$PROJECT_ROOT/AGENTS.md"
printf 'user CLAUDE - must stay\n' > "$PROJECT_ROOT/CLAUDE.md"
printf 'user cursor rules - must stay\n' > "$PROJECT_ROOT/.cursorrules"
mkdir -p "$PROJECT_ROOT/.cursor" "$CODEX_HOME" "$CLINE_DATA_HOME/settings"
printf 'profile = "user"\nmodel = "gpt-5"\n' > "$CODEX_HOME/config.toml"
printf '{"mcpServers":{"user_existing":{"command":"node","args":["server.js"]}}}\n' > "$PROJECT_ROOT/.cursor/mcp.json"
printf '{"mcpServers":{"user_existing":{"command":"node","args":["server.js"]}}}\n' > "$CLINE_DATA_HOME/settings/cline_mcp_settings.json"
```

记录 hash：

```bash
shasum "$PROJECT_ROOT/AGENTS.md" \
       "$PROJECT_ROOT/CLAUDE.md" \
       "$PROJECT_ROOT/.cursorrules" \
       "$PROJECT_ROOT/.cursor/mcp.json" \
       "$CODEX_HOME/config.toml" \
       "$CLINE_DATA_HOME/settings/cline_mcp_settings.json" \
       > "$TEST_ROOT/logs/hash-before-init.txt"
```

## 4. 主路径用例

### BBT-001 安装后可用

步骤：

```bash
export GOBIN="$TEST_ROOT/bin"
go install ./cmd/avm
export PATH="$TEST_ROOT/bin:$PATH"
which avm
avm --version
avm --help
```

预期：

- `which avm` 指向 `$TEST_ROOT/bin/avm`。
- `avm --help` 正常输出。

### BBT-002 初始化只读

步骤：

```bash
avm init
shasum "$PROJECT_ROOT/AGENTS.md" \
       "$PROJECT_ROOT/CLAUDE.md" \
       "$PROJECT_ROOT/.cursorrules" \
       "$PROJECT_ROOT/.cursor/mcp.json" \
       "$CODEX_HOME/config.toml" \
       "$CLINE_DATA_HOME/settings/cline_mcp_settings.json" \
       > "$TEST_ROOT/logs/hash-after-init.txt"
diff -u "$TEST_ROOT/logs/hash-before-init.txt" "$TEST_ROOT/logs/hash-after-init.txt"
```

预期：

- `diff` 无变化（用户已有文件 hash 不变）。
- 创建 `$HOME/.avm/config.yaml`。
- 创建 `$HOME/.avm/state/import-report.json`。
- 不创建或修改 runtime managed files。

验证：

```bash
assert_eq "BBT-002 hash unchanged" "" \
  "$(diff -u "$TEST_ROOT/logs/hash-before-init.txt" "$TEST_ROOT/logs/hash-after-init.txt")"
assert_file_exists "BBT-002 config.yaml" "$HOME/.avm/config.yaml"
assert_file_exists "BBT-002 import-report.json" "$HOME/.avm/state/import-report.json"
```

### BBT-003 创建 Agent 和 Environment

步骤：

```bash
avm agent create codex-agent --runtime codex --model gpt-5.4 --reasoning medium --skills test --mcps github
avm agent create claude-agent --runtime claude-code --model claude-sonnet --reasoning medium --skills test --mcps github
avm agent create cline-agent --runtime cline --model cline-model --reasoning medium --skills test --mcps github
avm agent create cursor-agent --runtime cursor --model cursor-model --reasoning medium --skills test --mcps github
avm env create all-runtimes --codex codex-agent --claude-code claude-agent --cline cline-agent --cursor cursor-agent
```

预期：

- 只写 `$HOME/.avm/agents/*.yaml` 和 `$HOME/.avm/envs/all-runtimes.yaml`。
- 不写 runtime 配置。

验证：

```bash
# source-of-truth 可查询
avm agent list | grep -q 'codex-agent'
assert_eq "BBT-003 agent list codex-agent" 0 $?
avm agent list | grep -q 'claude-agent'
assert_eq "BBT-003 agent list claude-agent" 0 $?
avm agent list | grep -q 'cline-agent'
assert_eq "BBT-003 agent list cline-agent" 0 $?
avm agent list | grep -q 'cursor-agent'
assert_eq "BBT-003 agent list cursor-agent" 0 $?

# env 可查询
assert_file_exists "BBT-003 env yaml" "$HOME/.avm/envs/all-runtimes.yaml"

# runtime 文件未被写入
find "$CODEX_HOME" "$CLAUDE_CONFIG_DIR" "$CLINE_DATA_HOME" \
     "$PROJECT_ROOT/.cursor" "$PROJECT_ROOT/.claude" \
     "$PROJECT_ROOT/.clinerules" -type f 2>/dev/null | sort > "$TEST_ROOT/logs/files-after-create.txt"
diff -u "$TEST_ROOT/logs/hash-before-init.txt" "$TEST_ROOT/logs/files-after-create.txt" \
  && printf "  PASS: BBT-003 no runtime files written\n" \
  || printf "  WARN: BBT-003 unexpected runtime files\n"
```

### BBT-004 准备可渲染 MCP Registry

步骤：

```bash
mkdir -p "$HOME/.avm/registry/mcps"
cat > "$HOME/.avm/registry/mcps/github.yaml" <<'YAML'
name: github
kind: mcp
server:
  type: stdio
  command: printf
  args:
    - "avm-test-mcp"
  env:
    GITHUB_TOKEN: "${GITHUB_TOKEN}"
YAML
```

预期：

- 后续 `avm use` 必须把 command/args/env 传到 runtime adapter。
- `${GITHUB_TOKEN}` 必须保留为占位符，不能展开成真实值。

说明：黑盒测试里的 MCP server 用 harmless 本地命令，不用真实 GitHub MCP。Claude Code 的 `mcp list/get` 可能会启动 stdio server 做健康检查，测试用 server 不能依赖网络、token 或长时间运行。

### BBT-005 激活多 runtime env

步骤：

```bash
export GITHUB_TOKEN="REAL_SECRET_SHOULD_NOT_APPEAR"
avm use --kind env all-runtimes
avm status
```

预期：

- `status` 显示 `active: env:all-runtimes`。
- Codex、Claude Code、Cline、Cursor target 状态可解释。
- Codex 写入当前 AVM profile selector：

```toml
profile = "avm-all-runtimes"
```

- Codex 写入 `[profiles.avm-all-runtimes]` 和 `[agents.codex-agent]`。
- MCP server `github` 被写入可支持的 runtime。
- runtime 配置文件中不得出现 `REAL_SECRET_SHOULD_NOT_APPEAR`。
- runtime 配置文件中应保留 `${GITHUB_TOKEN}`。

验证：

```bash
assert_grep "BBT-005 codex selector" '^profile = "avm-all-runtimes"$' "$CODEX_HOME/config.toml"
assert_grep "BBT-005 codex profile section" '\[profiles.avm-all-runtimes\]' "$CODEX_HOME/config.toml"
assert_grep "BBT-005 codex agent section" '\[agents.codex-agent\]' "$CODEX_HOME/config.toml"
assert_file_exists "BBT-005 claude agent" "$PROJECT_ROOT/.claude/agents/claude-agent.md"
assert_file_exists "BBT-005 claude mcp" "$PROJECT_ROOT/.mcp.json"
assert_file_exists "BBT-005 cline rules" "$PROJECT_ROOT/.clinerules/avm/cline-agent.md"
assert_file_exists "BBT-005 cursor rules" "$PROJECT_ROOT/.cursor/rules/avm-cursor-agent.md"

# secret 不泄漏（只检查 runtime 配置文件，避免 shell env 误报）
for f in "$CODEX_HOME/config.toml" \
         "$PROJECT_ROOT/.mcp.json" \
         "$PROJECT_ROOT/.cursor/mcp.json" \
         "$CLINE_DATA_HOME/settings/cline_mcp_settings.json"; do
  [ -f "$f" ] && assert_grep_not "BBT-005 no secret in $(basename "$f")" "REAL_SECRET_SHOULD_NOT_APPEAR" "$f"
done

# 占位符保留
assert_grep "BBT-005 codex env placeholder" '\${GITHUB_TOKEN}' "$CODEX_HOME/config.toml"
```

### BBT-006 启动 runtime probe

前置条件：当前工作目录必须为 `$PROJECT_ROOT`（Claude Code 的项目级命令依赖 cwd）。

步骤：

```bash
cd "$PROJECT_ROOT"

if command -v codex >/dev/null; then
  codex --version
  codex mcp list > "$TEST_ROOT/logs/codex-mcp-list.txt" 2>&1
  codex mcp get github > "$TEST_ROOT/logs/codex-mcp-github.txt" 2>&1
else
  mark_skip "BBT-006 codex probe" "codex CLI not installed"
fi

if command -v claude >/dev/null; then
  claude --version
  claude agents --setting-sources project > "$TEST_ROOT/logs/claude-agents.txt" 2>&1
  claude mcp list > "$TEST_ROOT/logs/claude-mcp-list.txt" 2>&1
  claude mcp get github > "$TEST_ROOT/logs/claude-mcp-github.txt" 2>&1
else
  mark_skip "BBT-006 claude probe" "claude CLI not installed"
fi

if command -v cline >/dev/null; then
  cline --version
else
  mark_skip "BBT-006 cline probe" "cline CLI not installed"
fi
```

预期：

- runtime probe 不读取用户真实 HOME。
- runtime 能看到 AVM 写入的 profile/agent/MCP。
- 若 runtime 没有安全可用的 probe 命令，记录为 `SKIP: runtime probe unavailable`，不能标 Pass。

Codex 文件级验证：

```bash
assert_grep "BBT-006 codex selector" '^profile = "avm-all-runtimes"$' "$CODEX_HOME/config.toml"
assert_grep "BBT-006 codex profile" '\[profiles.avm-all-runtimes\]' "$CODEX_HOME/config.toml"
assert_grep "BBT-006 codex agent" '\[agents.codex-agent\]' "$CODEX_HOME/config.toml"
assert_file_exists "BBT-006 codex role file" "$CODEX_HOME/agents/codex-agent.toml"
```

Claude Code 文件级验证：

```bash
assert_file_exists "BBT-006 claude agent file" "$PROJECT_ROOT/.claude/agents/claude-agent.md"
assert_file_exists "BBT-006 claude mcp file" "$PROJECT_ROOT/.mcp.json"
assert_grep "BBT-006 claude agent content" 'claude-agent' "$PROJECT_ROOT/.claude/agents/claude-agent.md"
assert_grep "BBT-006 claude mcp github" 'github' "$PROJECT_ROOT/.mcp.json"
```

Cline 文件级验证：

```bash
assert_file_exists "BBT-006 cline rules" "$PROJECT_ROOT/.clinerules/avm/cline-agent.md"
if [ -f "$CLINE_DATA_HOME/settings/cline_mcp_settings.json" ]; then
  assert_grep "BBT-006 cline mcp github" 'github' "$CLINE_DATA_HOME/settings/cline_mcp_settings.json"
fi
```

Cursor 文件级验证：

```bash
assert_file_exists "BBT-006 cursor rules" "$PROJECT_ROOT/.cursor/rules/avm-cursor-agent.md"
if [ -f "$PROJECT_ROOT/.cursor/mcp.json" ]; then
  assert_grep "BBT-006 cursor mcp github" 'github' "$PROJECT_ROOT/.cursor/mcp.json"
fi
```

### BBT-007 Profile 切换生效

步骤：

```bash
avm agent create writer-agent --runtime codex --model gpt-5.4 --reasoning low --skills test
avm use writer-agent
avm status
```

预期：

- `$HOME/.avm/state/current-active` 为 `profile:writer-agent`。
- Codex 顶层 selector 变成：

```toml
profile = "avm-writer-agent"
```

- `status` 不得继续把旧 env 的 Cursor/Claude/Cline 显示为当前 synced。
- 如果旧 runtime 文件保留，必须标记 stale 或说明来自上一轮 active。

验证：

```bash
assert_grep "BBT-007 codex selector" '^profile = "avm-writer-agent"$' "$CODEX_HOME/config.toml"

# 旧 env 的非 Codex runtime 不应显示为 synced
STATUS_OUT=$(avm status 2>&1)
echo "$STATUS_OUT" | grep -qi 'cursor.*synced' \
  && printf "  FAIL: BBT-007 cursor still shows synced\n" \
  || printf "  PASS: BBT-007 cursor not synced\n"
echo "$STATUS_OUT" | grep -qi 'claude.*synced' \
  && printf "  FAIL: BBT-007 claude still shows synced\n" \
  || printf "  PASS: BBT-007 claude not synced\n"
echo "$STATUS_OUT" | grep -qi 'cline.*synced' \
  && printf "  FAIL: BBT-007 cline still shows synced\n" \
  || printf "  PASS: BBT-007 cline not synced\n"
```

### BBT-008 Deactivate 生效

步骤：

```bash
avm deactivate
avm status
```

预期：

- `$HOME/.avm/state/current-active` 为 `profile:default`。
- Codex 顶层 selector 变成：

```toml
profile = "avm-default"
```

- status 不显示旧 env runtime 为当前 synced。

验证：

```bash
assert_grep "BBT-008 codex selector" '^profile = "avm-default"$' "$CODEX_HOME/config.toml"
STATUS_OUT=$(avm status 2>&1)
echo "$STATUS_OUT" | grep -qi 'writer-agent.*synced' \
  && printf "  FAIL: BBT-008 old profile still synced\n" \
  || printf "  PASS: BBT-008 old profile not synced\n"
```

### BBT-009 幂等性

验证 `avm use` 连续执行两次同一 profile，结果不变且不产生副作用。

步骤：

```bash
avm use writer-agent

# 记录第一次 use 后的状态
cp "$CODEX_HOME/config.toml" "$TEST_ROOT/logs/config-after-use1.toml"
avm status > "$TEST_ROOT/logs/status-after-use1.txt" 2>&1

# 第二次 use 同一 profile
avm use writer-agent

# 记录第二次 use 后的状态
cp "$CODEX_HOME/config.toml" "$TEST_ROOT/logs/config-after-use2.toml"
avm status > "$TEST_ROOT/logs/status-after-use2.txt" 2>&1
```

预期：

- 两次 `config.toml` 内容完全一致。
- 两次 `status` 输出一致（忽略时间戳差异）。
- 不产生额外的 backup、stale 文件或重复的 TOML section。

验证：

```bash
assert_eq "BBT-009 config idempotent" "" \
  "$(diff -u "$TEST_ROOT/logs/config-after-use1.toml" "$TEST_ROOT/logs/config-after-use2.toml")"

# 检查不会出现重复的 profile section
COUNT=$(grep -c '\[profiles.avm-writer-agent\]' "$CODEX_HOME/config.toml")
assert_eq "BBT-009 no duplicate profile section" "1" "$COUNT"

COUNT=$(grep -c '\[agents.writer-agent\]' "$CODEX_HOME/config.toml")
assert_eq "BBT-009 no duplicate agent section" "1" "$COUNT"
```

## 5. 负向测试

### BBT-N001 Preview 不写文件

步骤：

```bash
find "$CODEX_HOME" "$CLAUDE_CONFIG_DIR" "$CLINE_DATA_HOME" "$PROJECT_ROOT" -type f | sort > "$TEST_ROOT/logs/before-preview.txt"
avm agent show codex-agent --runtime codex
find "$CODEX_HOME" "$CLAUDE_CONFIG_DIR" "$CLINE_DATA_HOME" "$PROJECT_ROOT" -type f | sort > "$TEST_ROOT/logs/after-preview.txt"
diff -u "$TEST_ROOT/logs/before-preview.txt" "$TEST_ROOT/logs/after-preview.txt"
```

预期：无新增 runtime managed files。

验证：

```bash
assert_eq "BBT-N001 no files written" "" \
  "$(diff -u "$TEST_ROOT/logs/before-preview.txt" "$TEST_ROOT/logs/after-preview.txt")"
```

### BBT-N002 Conflict 检测

步骤：

```bash
avm use writer-agent
# 模拟用户在 Codex managed file 上做了外部修改
printf 'external change\n' >> "$CODEX_HOME/config.toml"
avm sync --target codex
SYNC_EXIT=$?
avm status > "$TEST_ROOT/logs/status-after-conflict.txt" 2>&1
```

预期：

- `sync` 以非零退出码失败。
- 用户修改保留（`external change` 仍在文件中）。
- `status` 显示 Codex failed 和 conflict reason。

验证：

```bash
assert_eq "BBT-N002 sync fails" 1 "$([ "$SYNC_EXIT" -ne 0 ] && echo 1 || echo 0)"
assert_grep "BBT-N002 user change preserved" 'external change' "$CODEX_HOME/config.toml"
```

### BBT-N003 Secret 不泄漏

步骤：

```bash
# 只搜索 runtime 配置文件目录，避免 shell env / history 误报
grep -r "REAL_SECRET_SHOULD_NOT_APPEAR" \
  "$CODEX_HOME" \
  "$CLAUDE_CONFIG_DIR" \
  "$CLINE_DATA_HOME" \
  "$PROJECT_ROOT/.mcp.json" \
  "$PROJECT_ROOT/.cursor" \
  "$PROJECT_ROOT/.clinerules" \
  2>/dev/null
```

预期：无结果。

验证：

```bash
MATCHES=$(grep -r "REAL_SECRET_SHOULD_NOT_APPEAR" \
  "$CODEX_HOME" \
  "$CLAUDE_CONFIG_DIR" \
  "$CLINE_DATA_HOME" \
  "$PROJECT_ROOT/.mcp.json" \
  "$PROJECT_ROOT/.cursor" \
  "$PROJECT_ROOT/.clinerules" \
  2>/dev/null | wc -l)
assert_eq "BBT-N003 no secret leak" "0" "$(echo "$MATCHES" | tr -d ' ')"
```

### BBT-N004 Package 不携带 runtime 输出

步骤：

```bash
avm export codex-agent --output "$TEST_ROOT/codex-agent.avm.zip"
unzip -l "$TEST_ROOT/codex-agent.avm.zip" > "$TEST_ROOT/logs/package-contents.txt"
```

预期：

- 包含 agent YAML 和 referenced registry 文件。
- 不包含 runtime config、state、backup、cache。

验证：

```bash
assert_grep "BBT-N004 has agent yaml" '\.yaml' "$TEST_ROOT/logs/package-contents.txt"
assert_grep_not "BBT-N004 no config.toml" 'config\.toml' "$TEST_ROOT/logs/package-contents.txt"
assert_grep_not "BBT-N004 no state" 'state/' "$TEST_ROOT/logs/package-contents.txt"
assert_grep_not "BBT-N004 no backup" 'backup' "$TEST_ROOT/logs/package-contents.txt"
```

### BBT-N005 无效输入处理

验证 AVM 对无效输入给出合理的错误提示和非零退出码。

步骤：

```bash
# 不存在的 profile
avm use non-existent-profile 2>"$TEST_ROOT/logs/err-use-missing.txt"
USE_EXIT=$?

# 不存在的 env
avm use --kind env non-existent-env 2>"$TEST_ROOT/logs/err-use-missing-env.txt"
ENV_EXIT=$?

# 无效的 kind
avm use --kind team something 2>"$TEST_ROOT/logs/err-use-invalid-kind.txt"
KIND_EXIT=$?

# agent create 缺少 --runtime
avm agent create incomplete-agent 2>"$TEST_ROOT/logs/err-create-no-runtime.txt"
CREATE_EXIT=$?
```

预期：

- 所有命令以非零退出码退出。
- 错误信息包含有意义的提示（如 `not found`、`invalid`、`required`）。
- 不产生 panic 或 stack trace。

验证：

```bash
assert_eq "BBT-N005 use missing profile exits non-zero" 1 "$([ "$USE_EXIT" -ne 0 ] && echo 1 || echo 0)"
assert_eq "BBT-N005 use missing env exits non-zero" 1 "$([ "$ENV_EXIT" -ne 0 ] && echo 1 || echo 0)"
assert_eq "BBT-N005 use invalid kind exits non-zero" 1 "$([ "$KIND_EXIT" -ne 0 ] && echo 1 || echo 0)"
assert_eq "BBT-N005 create no runtime exits non-zero" 1 "$([ "$CREATE_EXIT" -ne 0 ] && echo 1 || echo 0)"

# 不应出现 Go panic
for f in "$TEST_ROOT/logs"/err-*.txt; do
  assert_grep_not "BBT-N005 no panic in $(basename "$f")" 'goroutine\|panic:' "$f"
done
```

## 6. 判定标准

### 必须通过

- `avm` 从独立安装目录执行。
- `init` 不修改 runtime 文件。
- `use` 后 AVM active 和 runtime active selector 一致。
- `status` 不显示过期 runtime 为当前 synced。
- referenced MCP registry 能真正写入 runtime。
- secret 不展开。
- 用户 instruction 文件不覆盖。
- conflict 不覆盖用户修改。

### 可以跳过

- 未安装真实 runtime CLI 时，L2 runtime probe 可标记 `SKIP`。
- Cursor/Cline IDE 级真实读取验证可以先跳过，但文件级 PoC 必须通过。

### 失败即阻塞

- AVM 显示 synced，但 runtime active selector 没切换。
- `status` 把旧 active 的 runtime 状态显示为当前 synced。
- Agent 引用了 MCP registry，但 runtime 没拿到 MCP command/url/env。
- 任一用户文件被覆盖。
- secret 被展开写入磁盘。

## 7. 本方案会发现的问题

这套黑盒方案能直接发现此前真实 smoke 暴露的问题：

1. Codex 只写 `[profiles.avm-*]`，没有改顶层 `profile`。
2. MCP registry 文件存在，但没有被 resolver 读入 adapter input。
3. 从 env 切到单 profile 后，`status` 仍展示旧 Cursor synced。
4. 旧 runtime managed files 保留但没有 stale 说明。

这些问题用单元测试或只检查“文件是否写出”很容易漏掉，因为关键不是写没写文件，而是用户下次启动 runtime 时是否会用到当前 AVM profile。

## 8. 测试报告模板

```markdown
# Phase 1 黑盒测试报告

日期：
测试人：
AVM commit：
安装路径：
TEST_ROOT：

## 结果

Overall: Pass / Fail / Conditional Pass
Stats: X PASS, Y FAIL, Z SKIP

## 环境

- OS：
- Shell：
- Go：
- Runtime CLIs：
  - codex：
  - claude：
  - cline：
  - cursor：

## 主路径用例

| 用例 | 结果 | 证据 |
|------|------|------|
| BBT-001 安装后可用 | | |
| BBT-002 初始化只读 | | |
| BBT-003 创建 Agent/Env | | |
| BBT-004 MCP Registry | | |
| BBT-005 激活多 runtime env | | |
| BBT-006 Runtime probe | | |
| BBT-007 Profile 切换 | | |
| BBT-008 Deactivate | | |
| BBT-009 幂等性 | | |

## 负向测试

| 用例 | 结果 | 证据 |
|------|------|------|
| BBT-N001 Preview 不写文件 | | |
| BBT-N002 Conflict 检测 | | |
| BBT-N003 Secret 不泄漏 | | |
| BBT-N004 Package 不携带 runtime | | |
| BBT-N005 无效输入处理 | | |

## 阻塞缺陷

| ID | 现象 | 影响 | 复现步骤 | 建议 |
|----|------|------|----------|------|

## 附件

- `$TEST_ROOT/logs`
- `config.toml`
- `.mcp.json`
- `sync-state.json`
- `import-report.json`
```
