# AVM

> Agent VM——为 AI Coding Agent 定义一次配置，跑在任何 runtime 上。

<p>
  <a href="https://github.com/xz1220/Agent-VM/actions/workflows/ci.yml"><img src="https://github.com/xz1220/Agent-VM/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <img src="https://img.shields.io/badge/status-early_preview-0f766e" alt="状态：早期预览">
  <img src="https://img.shields.io/badge/runtime-Codex%20%7C%20Claude%20Code%20%7C%20OpenCode-1d4ed8" alt="目标 Runtime">
  <img src="https://img.shields.io/badge/lang-Go%20%2B%20TypeScript-00ADD8" alt="开发语言">
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-PolyForm--NC%201.0.0-6b21a8" alt="License: PolyForm Noncommercial 1.0.0"></a>
</p>

[English](README.md) | 简体中文

## 介绍

AVM（`avm`）是面向 AI Coding Agent 的本地配置管理器。你只需要构建一个可
复用的 **Agent**——包含 instructions、skills、MCP servers 和 runtime 偏好——
AVM 会把它投射到你启动的目标 runtime 上。AVM 全权负责 managed config 的
写入，明确报告每个 runtime 哪些字段是原生支持还是降级，并且不会去碰你
手工编辑的 runtime 文件。

日常会接触到的核心对象：

- **Agent**：可复用的工作配置；唯一需要直接创建和编辑的对象。
- **Capability**：Agent 引用的 skill 或 MCP server。AVM 会发现你 runtime
  里已经安装的 capability 并把它们 import 进来。
- **Package**：`.avm.zip` 包，把 Agent（以及它引用的 capability）打包导出，
  方便分享或在另一台机器上重装。
- **Runtime**：真正执行 Agent 的工具。

AVM 由两个互相配套的二进制组成：

- **`avm`**：Go CLI，非交互式管道。所有命令通过 flag 或 stdin 接收输入，
  输出人类可读文本或 `--json`。脚本和 CI 用它。
- **`avm-ui`**：[`ui/`](ui/) 下的 TypeScript 全屏 TUI，底层通过 JSON 契约
  调用 `avm`。日常交互式编辑、浏览用它。

## 架构

```
   avm-ui  (TypeScript, Ink TUI)        avm  (Go, --json plumbing)
   交互式编辑与浏览              ───▶  脚本、CI、程序化集成
                                              │
                                              │   单一 CLI 契约
                                              ▼
                                   ┌──────────────────────┐
                                   │ Application Services │
                                   │ Agent · Run · Package│
                                   │ Capability · System  │
                                   └──────────┬───────────┘
                                              │
                              ┌───────────────┴───────────────┐
                              ▼                               ▼
                    ┌──────────────────┐            ┌──────────────────┐
                    │ Runtime Drivers  │            │ Infrastructure   │
                    │  codex           │            │  home, agentstore│
                    │  claude-code     │            │  capstore, runlog│
                    │  opencode        │            │  managedfile, …  │
                    └────────┬─────────┘            └──────────────────┘
                             ▼
              Codex · Claude Code · OpenCode · …
              （managed config 写入 + 启动）
```

详细的层 / 包对应关系见
[`docs/engineering/architecture-overview.md`](docs/engineering/architecture-overview.md)。

## 安装

```bash
curl -fsSL https://raw.githubusercontent.com/xz1220/Agent-VM/main/scripts/install.sh | sh
avm init
avm shell install            # 可选：为 bash/zsh/fish 安装补全
```

安装脚本默认把 `avm` 放到 `$HOME/.local/bin`，并初始化 `~/.avm`。如果
只想安装二进制，设置 `AVM_SKIP_INIT=1` 即可。

源码构建：

```bash
make build         # bin/avm
make build-ui      # dist/avm-ui.js（交互式 TUI）
make build-all
```

## 使用指南

### 1. 把 runtime 已有的 skill / MCP 接进 AVM

第一次用时，先把 runtime 里已经装好的 capability 导入进来，让 Agent
可以引用它们：

```bash
avm capability bootstrap --runtime claude-code
avm capability bootstrap --runtime codex
avm capability list
```

### 2. 创建 Agent

flag 驱动（CLI 从不弹出交互；要向导请改用 `avm-ui`）：

```bash
avm agent create \
  --name backend-coder \
  --runtime codex \
  --description "订单服务的 API + DB 工作" \
  --skill git --skill test
```

### 3. 查看与编辑

```bash
avm agent list
avm agent show backend-coder
avm agent show backend-coder --runtime codex   # 该 runtime 下每个字段的映射
avm agent edit backend-coder --skill git --skill test --skill review
avm agent clone backend-coder --name backend-reviewer
avm agent rename backend-reviewer reviewer
avm agent delete reviewer --yes
```

`agent edit` 是非交互式的：任何列表 flag（`--skill`、`--mcp`、`--runtime`）
都是**整体替换**当前列表。如果想保留现有值，先用
`avm agent show <name> --json` 读出来，再带着完整列表回写。

### 4. 运行

```bash
avm run backend-coder
avm run backend-coder --runtime codex      # Agent 配了多个 runtime 时必填
avm run backend-coder --preview            # 只展示计划，不真正启动
avm run backend-coder --drift merge        # 显式确认 AVM 与现有 managed config 之间的 drift
```

`avm run` 会透传 runtime 自身的退出码，方便 shell 脚本根据它分支。

### 5. 通过 Package 分享

```bash
avm package export backend-coder -o backend-coder.avm.zip
avm package install ./backend-coder.avm.zip --on-conflict rename
avm package inspect ./backend-coder.avm.zip
avm package list
avm package uninstall backend-coder --yes
```

### 6. 诊断

```bash
avm doctor                  # AVM home、runtime、最近运行
avm status [agent]
avm runtime list            # 注册的 runtime 列表及可用性
```

以上每条命令都支持 `--json`，输出对应的
[`internal/app/model/`](internal/app/model/) 模型。完整 JSON 结构、错误码、
退出码语义见 [`docs/api/cli-protocol.md`](docs/api/cli-protocol.md)。

## Runtime 支持

AVM 会把选中的 Agent 渲染成各 runtime 的 managed 文件。每个 driver 都会把
Agent 的每个字段标记为 `native`、`rendered_as_instructions`、`ignored` 或
`unsupported`——`avm agent show <name> --runtime <rt>` 会展示完整映射。

| Runtime | 状态 | 说明 |
| --- | --- | --- |
| Codex | 已支持 | 原生映射 profile / model / reasoning；每次运行隔离 `CODEX_HOME` |
| Claude Code | 已支持 | 映射 agent frontmatter、MCP、skills；裁剪后的 auth state 会带入 boundary |
| OpenCode | 已支持 | 映射 config、agent、skills 和 MCP |
| OpenClaw（龙虾） | 在支持中 | runtime 调研完成（[`docs/.../openclaw-runtime.md`](docs/engineering/runtime-research/openclaw-runtime.md)），driver 尚未实现 |
| Hermes Agent（爱马仕） | 在支持中 | 在 [`DESIGN.md`](DESIGN.md) 中列为目标 runtime，driver 尚未实现 |

## License

AVM 采用 [PolyForm Noncommercial License 1.0.0](LICENSE) 协议。

简单来说：

- 你可以以**非商业**目的复制、修改、分发、学习这套代码——个人项目、
  研究、教育、评估、爱好开发、非营利组织使用都在允许范围内。
- **商用需要单独授权**，请联系项目维护者。
- 软件按 "as is" 提供，不附带任何担保。

如果你希望商用 AVM，请开 issue 或联系维护者。

## Contributors

感谢每一位为 AVM 做出贡献的开发者。

<a href="https://github.com/xz1220/Agent-VM/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=xz1220/Agent-VM" alt="Contributors" />
</a>

## Star History

<a href="https://star-history.com/#xz1220/Agent-VM&Date">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="https://api.star-history.com/svg?repos=xz1220/Agent-VM&type=Date&theme=dark" />
    <source media="(prefers-color-scheme: light)" srcset="https://api.star-history.com/svg?repos=xz1220/Agent-VM&type=Date" />
    <img alt="Star History Chart" src="https://api.star-history.com/svg?repos=xz1220/Agent-VM&type=Date" />
  </picture>
</a>
