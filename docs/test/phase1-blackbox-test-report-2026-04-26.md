# Agent VM Phase 1 黑盒测试报告

> 日期：2026-04-26
> 执行方式：真实安装 AVM 到临时 `GOBIN`，使用隔离 `HOME` 和隔离项目目录，只通过 `PATH` 调用 `avm`
> 结论：未通过。Phase 1 主流程能写出大部分文件，但 Codex selector、MCP registry 到 runtime 的渲染、切换后的 stale runtime 状态仍有问题。

## 1. 测试环境

- Repo：`/Users/danielxing/code/agent-vm`
- AVM 安装方式：`GOBIN=$TEST_ROOT/bin go install ./cmd/avm`
- 主测试目录：`/tmp/avm-blackbox.X38mkv`
- Codex 定向复测目录：`/tmp/avm-codex-focused.wCCVEv`
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

主测试汇总：

```text
PASS=45
FAIL=15
SKIP=1
```

其中 1 个失败是测试断言问题：Cursor rule 文件实际是 `.cursor/rules/avm-cursor-agent.md`，不是 `.cursor/rules/cursor-agent.md`。测试方案已修正。

已验证通过：

- `go install ./cmd/avm` 后，`avm` 来自隔离 `$TEST_ROOT/bin/avm`。
- `avm init` 创建 `$HOME/.avm/**`，未改写已有 `AGENTS.md`、`CLAUDE.md`、`.cursorrules`、Codex config 和 Cline settings。
- `avm agent create` 和 `avm env create all-runtimes` 可完成。
- `avm use --kind env all-runtimes` 返回 `sync: completed`。
- Claude Code 能通过 `claude agents --setting-sources project` 看到 `claude-agent`。
- Cline 文件级产物 `.clinerules/avm/cline-agent.md` 能写出。
- 重复 `avm use --kind profile writer-agent` 后 Codex managed block 没有重复追加。
- `REAL_SECRET_SHOULD_NOT_APPEAR` 未出现在 runtime 配置文件中。
- `avm agent show --runtime codex` 预览没有创建或删除 runtime 文件。

## 3. 主要问题

### P0：Codex active selector 没有切到 AVM profile

现象：

`avm use --kind env all-runtimes` 后，Codex config 中有 `[profiles.avm-all-runtimes]` 和 `[agents.codex-agent]`，但顶层仍保留用户原来的 selector：

```toml
profile = "user"
model = "gpt-5"

# >>> avm:codex:codex-config
[profiles.avm-writer-agent]
model = "gpt-5.4"
model_reasoning_effort = "low"
approval_policy = "on-request"
sandbox_mode = "workspace-write"
```

影响：

- 如果原来的 `profile = "user"` 没有对应 `[profiles.user]`，真实 `codex mcp list` 直接失败：`config profile user not found`。
- 即使补上合法 `[profiles.user]`，Codex 也继续使用 `user` profile，不会使用 AVM 当前 active profile。

定向复测：

在 `/tmp/avm-codex-focused.wCCVEv` 中预置合法 `profile = "user"` 和 `[profiles.user]` 后执行：

```bash
avm use --kind profile codex-agent
codex mcp list
```

结果：`codex mcp list` 退出码为 0，但输出 `No MCP servers configured yet`，且 `config.toml` 顶层仍是 `profile = "user"`。

可能原因：

- `internal/adapter/codex/codex.go` 只渲染 `[profiles.avm-*]`、`[mcp_servers.*]`、`[agents.*]`。
- `mergeMarkedBlock` 只替换 AVM managed block，保留顶层用户配置。
- `docs/engineering/acceptance.md` 和 `docs/engineering/file-layout.md` 明确要求把顶层 `profile` 指向当前 AVM profile/env。

### P1：MCP registry 没有被解析成 runtime 可渲染配置

现象：

测试准备了可渲染 registry：

```yaml
name: github
kind: mcp
server:
  type: stdio
  command: printf
  args:
    - "avm-test-mcp"
  env:
    GITHUB_TOKEN: "${GITHUB_TOKEN}"
```

但 `avm use --kind env all-runtimes` 输出：

```text
codex: mcp server "github" was not rendered because command or URL is missing
claude-code: mcp server "github" was not rendered because command or URL is missing
cline: mcp server "github" was not rendered because name and command or URL are required
cursor: mcp server "github" was not rendered because name and command or URL are required
```

runtime probe 结果：

- `codex mcp list` 看不到 `github`。
- `codex mcp get github` 失败。
- `claude mcp list` 输出 `No MCP servers configured`。
- `claude mcp get github` 失败。
- `PROJECT_ROOT/.mcp.json` 没有生成。
- `CLINE_DATA_HOME/settings/cline_mcp_settings.json` 没有加入 `github`。

可能原因：

- `internal/config/resolve.go` 的 `capabilitiesForAgent` 只保留 MCP 名称。
- `internal/adapter/config_input.go` 的 `mcpServers` 只构造 `adapter.MCPServer{Name: name}`。
- registry 中的 `command`、`args`、`env` 没有进入 adapter input。

### P1：切换到单 Codex profile 后，旧 Cursor runtime 状态仍显示 synced

步骤：

```bash
avm use --kind env all-runtimes
avm agent create writer-agent --runtime codex --model gpt-5.4 --reasoning low --skills test
avm use --kind profile writer-agent
avm status
```

实际输出：

```text
active: profile:writer-agent
runtime status:
  claude-code: skipped
  cline: skipped
  codex: synced (agent writer-agent)
  cursor: synced (agent cursor-agent)
```

`cursor` 的状态来自上一次 `env:all-runtimes`，当前 `profile:writer-agent` 并不应该继续显示 `cursor-agent` 为 synced。

可能原因：

- syncer 只更新当前输入和 missing targets。
- 对于已经不在当前 targets 中的旧 runtime，没有清理、过期标记或 stale 状态。
- `status` 按 `sync-state.runtimes` 全量打印，因此旧 runtime 会继续展示为 synced。

## 4. 其他观察

- Claude Code agent 文件是有效的：`claude agents --setting-sources project` 显示 `claude-agent · claude-sonnet`。
- Claude Code CLI 在隔离 HOME 下会写 `$CLAUDE_CONFIG_DIR/.claude.json` 和 backup，这是 Claude Code 自身行为，不是 AVM 写入。
- Cursor 文件路径实际为 `.cursor/rules/avm-cursor-agent.md`，测试方案中旧断言已修正。
- Cline 未安装，所以这次没有真实 runtime probe；只验证了 `.clinerules/avm/cline-agent.md` 和 settings 文件级行为。

## 5. 建议修复顺序

1. Codex adapter 写入或安全更新顶层 `profile = "avm-<active>"`，并设计退出/恢复策略，避免永久覆盖用户偏好。
2. 在 resolve 阶段读取 MCP registry，把 `command`、`args`、`env`、`url` 等字段传入 adapter input。
3. sync-state 对不属于当前 active targets 的旧 runtime 标记为 `stale`、`inactive` 或从 status 展示中过滤。
4. 修复后重跑本报告同样的黑盒路径，尤其是 `codex mcp list/get` 和 `claude mcp list/get`。
