<p align="center">
  <img src="assets/avm-hero.svg" alt="Agent VM：一个 Profile，投射到所有 Agent Runtime" width="100%">
</p>

<h1 align="center">Agent VM</h1>

<p align="center">
  <strong>跨运行时管理 AI Coding Agent 配置。</strong>
  <br>
  定义一次 Agent，然后通过 Codex、Claude Code、OpenCode、Cline 或 Cursor 启动它。
</p>

<p align="center">
  <a href="https://github.com/xz1220/Agent-VM/actions/workflows/ci.yml"><img src="https://github.com/xz1220/Agent-VM/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <img src="https://img.shields.io/badge/status-early_preview-0f766e" alt="状态：早期预览">
  <img src="https://img.shields.io/badge/runtime-Codex%20%7C%20Claude%20Code%20%7C%20OpenCode-1d4ed8" alt="目标 Runtime">
  <img src="https://img.shields.io/badge/language-Go%20%2B%20TypeScript-00ADD8" alt="Go + TypeScript">
</p>

<p align="center">
  <a href="README.md">English</a> | 简体中文
</p>

Agent VM（`avm`）是面向 AI Coding Agent 的本地配置管理器。你只需要构建一个可
复用的 **Agent**——包含 instructions、skills、MCP servers 和 runtime 偏好——
AVM 会把它投射到你启动的目标 runtime 上。AVM 全权负责 managed config 的写入，
明确报告每个 runtime 哪些字段是原生支持还是降级，并且不会去碰你手工编辑的
runtime 文件。

你日常会接触到的核心对象：

- **Agent**：可复用的工作配置。这是你唯一需要直接创建和编辑的对象。
- **Capability**：Agent 引用的 skill 或 MCP server。AVM 可以发现你 runtime 里
  已经安装的 capability，并把它们 import 进来供 Agent 复用。
- **Package**：`.avm.zip` 包，把 Agent（以及它引用的 capability）打包导出，方
  便分享或在另一台机器上重装。
- **Runtime**：真正执行 Agent 的工具，例如 Codex、Claude Code、OpenCode、Cline、
  Cursor。

> Environment 只是 AVM 内部的概念。用户不需要创建、切换或管理 Environment——
> Agent 自己就持有 runtime 配置。

## 两种使用方式

AVM 由两个互相配套的二进制组成：

| 二进制 | 形态 | 适用场景 |
| --- | --- | --- |
| `avm`（Go CLI） | 非交互式管道。所有命令通过 flag 或 stdin 接收输入，输出人类可读文本或 `--json`。 | 脚本、CI、习惯用 flag 的用户、任何程序化集成。 |
| `avm-ui`（TS TUI） | 全屏交互式界面。位于 [`ui/`](ui/)，底层调用 `avm`。 | 日常编辑、浏览 Agent / Capability、创建/编辑向导。 |

Go CLI **从不弹出交互提示**。需要向导请改用 `avm-ui`。

## 快速开始

安装 CLI：

```bash
curl -fsSL https://raw.githubusercontent.com/xz1220/Agent-VM/main/scripts/install.sh | sh
avm init
avm shell install            # 可选：为当前 shell 安装补全
```

安装脚本默认把 `avm` 放到 `$HOME/.local/bin`，并初始化 `~/.avm`。如果只想
安装二进制，设置 `AVM_SKIP_INIT=1` 即可。

把你 runtime 里已经装好的 capability 导入到 AVM，让 Agent 可以引用它们：

```bash
avm capability bootstrap --runtime claude-code
avm capability bootstrap --runtime codex
```

创建第一个 Agent（flag 驱动；如果想要向导请用 `avm-ui`）：

```bash
avm agent create \
  --name backend-coder \
  --runtime codex \
  --description "订单服务的 API + DB 工作" \
  --skill git --skill test
```

运行：

```bash
avm run backend-coder
```

## 日常命令

### Agent

```bash
avm agent list
avm agent show backend-coder
avm agent show backend-coder --runtime codex   # 查看每个字段在该 runtime 下的映射
avm agent edit backend-coder --skill git --skill test --skill review
avm agent clone backend-coder --name backend-reviewer
avm agent rename backend-reviewer reviewer
avm agent delete reviewer --yes
```

`agent edit` 是非交互式的：任何列表 flag（`--skill`、`--mcp`、`--runtime`）
都是**整体替换**当前列表。如果想保留现有值，先用 `avm agent show <name> --json`
读出来，再带着完整列表回写。

### Run

```bash
avm run backend-coder
avm run backend-coder --runtime codex      # Agent 配了多个 runtime 时必填
avm run backend-coder --preview            # 只展示计划，不真正启动
avm run backend-coder --drift merge        # 显式确认 AVM 与现有 managed config 之间的 drift
```

`avm run` 会透传 runtime 自身的退出码，方便 shell 脚本根据它分支。

### Capability

Capability 是 AVM 内部的 skill / MCP 记录。bootstrap 一次之后，平时基本不
需要再操作；但能力是齐的：

```bash
avm capability discover                    # 查看当前 AVM 和 runtime 能看到的所有候选
avm capability list                        # 当前 AVM 存储里的 capability
avm capability show <id>
avm capability import --runtime codex --kind skill --name my-skill
avm capability bootstrap --runtime claude-code
```

### Package

```bash
avm package list
avm package show <name>
avm package export backend-coder -o backend-coder.avm.zip
avm package install ./backend-coder.avm.zip --on-conflict rename
avm package inspect ./backend-coder.avm.zip
avm package uninstall backend-coder --yes
```

### 系统与诊断

```bash
avm doctor                  # AVM home、runtime、最近运行
avm status [agent]
avm runtime list            # AVM 注册的 runtime 列表及可用性
avm shell install           # 为当前 shell 安装补全
avm uninstall --yes         # 删除 ~/.avm 和二进制
```

以上每条命令都支持 `--json`，输出对应的
[`internal/app/model/`](internal/app/model/) 模型。完整的 JSON 结构、错误码和
退出码语义见 [`docs/api/cli-protocol.md`](docs/api/cli-protocol.md)——这是
程序化调用 `avm` 时的唯一权威来源。

## Runtime 支持

AVM 会把选中的 Agent 渲染成各 runtime 的 managed 文件。

| Runtime | 状态 | 说明 |
| --- | --- | --- |
| Codex | 支持 | 原生映射 profile / model / reasoning；每次运行使用隔离的 `CODEX_HOME` |
| Claude Code | 支持 | 映射 agent frontmatter、MCP、skills；裁剪后的 auth state 会被带入 boundary |
| OpenCode | 支持 | 映射 config、agent、skills 和 MCP |
| Cline | 兼容 | 主要渲染为 rules 和 MCP settings |
| Cursor | 兼容 | 保守的 rules / MCP PoC |

每个 runtime driver 都会把 Agent 的每个字段标记为 `native`、
`rendered_as_instructions`、`ignored` 或 `unsupported`。AVM 不会静默丢弃 runtime
不能表达的内容——`avm agent show <name> --runtime <rt>` 会展示完整映射。

## 当前还不稳定的部分

仍处在早期预览阶段，常见情况：

- TS UI 还在第一轮联调，部分 Agent 编辑界面仍依赖 mock 的 capability 数据，
  详见 [`ui/INTEGRATION-GAPS.md`](ui/INTEGRATION-GAPS.md)。
- `avm package show` 在已安装包注册表落地之前会返回 “registry not yet
  implemented”；先用 `avm package inspect` 直接看 `.avm.zip` 文件。
- `avm init` 和 `avm uninstall` 目前只支持 human 模式（暂未支持 `--json`）。
- Cline、Cursor 是 best-effort；信任映射前先用 `avm agent show --runtime`
  确认。

## 安全模型

AVM 不碰自己不拥有的文件：

- `avm init` 只写 `~/.avm`。
- Agent 是显式 CRUD 资源，不会发生隐式覆盖。
- Runtime-native 文件只能通过 driver 声明的 managed paths 写入；其他你手工
  编辑的内容会被原样保留。
- Runtime 不能表达的字段会在映射结果里报告，不会静默丢弃。
- Secrets 通过环境变量引用，不会以明文导出到 portable Package。

## 开发

```bash
make build        # 编译 bin/avm
make build-ui     # 安装 UI 依赖，typecheck，构建 dist/avm-ui.js
make build-all    # 同时构建两个产物
make test
make vet
make fmt
```

工程结构一览：

- `cmd/avm/`：Go CLI 入口（只做 composition root）。
- `internal/presentation/cli/`：cobra 命令、flag 解析、JSON / human 渲染，
  `avm <subcommand>` 都在这里。
- `internal/app/{model,service}/`：产品模型与 service 编排（`AgentService`、
  `RunService`、`PackageService`、`CapabilityService`、`DiagnosticsService`、
  `SystemService`）。
- `internal/runtime/{codex,claudecode,opencode}/`：每个 runtime 一个 driver，
  各自负责 boundary、plan 和 launch spec。
- `internal/infra/`：副作用层：`home`、`agentstore`、`capstore`、
  `managedfile`、`process`、`runlog`、`packageio`、`fsutil`。
- `ui/`：TypeScript + Ink 实现的交互式 UI，仅消费 Go CLI 的 `--json` 契约。

相关文档：

- [产品需求文档（PRD）](docs/product/prd.md)
- [架构总览](docs/engineering/architecture-overview.md)
- [重写架构提案](docs/rewrite-architecture-proposal.md)
- [CLI 协议契约](docs/api/cli-protocol.md)
- [Runtime 研究](docs/engineering/runtime-research/)

## License

项目尚未选择开源协议。在 license 添加之前，代码可以阅读，但不能被默认视作
具备广泛复用权利的开源项目。
