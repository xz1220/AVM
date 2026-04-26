# Agent VM Phase 1 黑盒复测报告

> 日期：2026-04-26
> 目标：复验修复后的真实用户路径
> 结论：通过。上次发现的 Codex selector、MCP registry 渲染、切换后 stale runtime 状态问题均已修复。

## 1. 测试环境

- Repo：`/Users/danielxing/code/agent-vm`
- AVM 安装方式：`GOBIN=$TEST_ROOT/bin go install ./cmd/avm`
- 黑盒测试目录：`/tmp/avm-retest.3PmwSo`
- 日志目录：`/tmp/avm-retest.3PmwSo/logs`
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

## 2. 执行结果

仓库测试：

```text
go test ./...  PASS
go vet ./...   PASS
```

黑盒复测结果：

```text
PASS=66
FAIL=0
SKIP=1
```

说明：原始脚本输出为 `PASS=65 FAIL=1 SKIP=1`，其中 1 个失败是测试断言过窄。`claude mcp get github` 不输出 command path，但退出码为 0 且显示 `Status: ✓ Connected`，补验后按通过计算。

## 3. 复验结论

### Codex selector 已生效

`avm use --kind env all-runtimes` 后，Codex 顶层 selector 已切到 AVM profile：

```toml
profile = "avm-all-runtimes"
```

`avm use --kind profile writer-agent` 后切换为：

```toml
profile = "avm-writer-agent"
```

`avm deactivate` 后切换为：

```toml
profile = "avm-default"
```

真实 Codex probe 通过：

```text
codex mcp list       PASS, sees github
codex mcp get github PASS, command=node, args=/tmp/avm-retest.3PmwSo/mcp-server.js
```

### MCP registry 已渲染到 runtime

测试 registry：

```yaml
name: github
kind: mcp
server:
  type: stdio
  command: node
  args:
    - "/tmp/avm-retest.3PmwSo/mcp-server.js"
  env:
    GITHUB_TOKEN: "${GITHUB_TOKEN}"
```

复测结果：

- Codex：`~/.codex/config.toml` 写入 `mcp_servers.github`。
- Claude Code：`$PROJECT_ROOT/.mcp.json` 写入 `mcpServers.github`。
- Cline：`$CLINE_DATA_HOME/settings/cline_mcp_settings.json` 写入 `mcpServers.github`。
- Cursor：`$PROJECT_ROOT/.cursor/mcp.json` 写入 `mcpServers.github`，并保留已有 `user_existing`。
- `${GITHUB_TOKEN}` 占位符保留。
- `REAL_SECRET_SHOULD_NOT_APPEAR` 未泄漏到 runtime 文件。

真实 Claude Code probe 通过：

```text
claude agents --setting-sources project PASS, sees claude-agent · claude-sonnet
claude mcp list                         PASS, github connected
claude mcp get github                   PASS, Status: ✓ Connected
```

### stale runtime 状态已修复

从多 runtime env 切换到单 Codex profile 后：

```text
active: profile:writer-agent
runtime status:
  claude-code: skipped
  cline: skipped
  codex: synced (agent writer-agent)
```

不再显示旧的 `cursor: synced (agent cursor-agent)`。

`avm deactivate` 后：

```text
active: profile:default
runtime status:
  claude-code: skipped
  cline: skipped
  codex: synced (agent default)
```

同样没有 stale Cursor 状态。

## 4. 仍需注意

- Cline CLI 本机未安装，本次只验证了 Cline 文件级产物，真实 runtime probe 仍是 `SKIP`。
- Cursor 仍按 Phase 1 partial support 处理，`avm use` 只保留预期的 partial warnings。
- Claude Code CLI 会在隔离 `$CLAUDE_CONFIG_DIR` 下写 `.claude.json` 和 backup，这是 Claude Code 自身行为，不是 AVM 写入用户真实配置。

## 5. 结论

Phase 1 这轮修复后，当前机器可执行范围内的真实黑盒路径通过。建议把这条黑盒流程沉淀为可重复脚本，避免后续只靠 Go 单测覆盖不到 runtime CLI 是否真实读取配置。
