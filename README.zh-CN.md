<p align="center">
  <img src="assets/avm-hero.svg" alt="Agent VM：一个 Profile，投射到所有 Coding Agent Runtime" width="100%">
</p>

<h1 align="center">Agent VM</h1>

<p align="center">
  <strong>AI Coding Agent 时代的 nvm。</strong>
  <br>
  用一个可迁移 Profile 管理工具、权限、模型设置和长期记忆引用。
</p>

<p align="center">
  <a href="https://github.com/xz1220/Agent-VM/actions/workflows/ci.yml"><img src="https://github.com/xz1220/Agent-VM/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <img src="https://img.shields.io/badge/status-early_preview-0f766e" alt="状态：早期预览">
  <img src="https://img.shields.io/badge/runtime-Codex%20%7C%20Claude%20Code%20%7C%20Cline%20%7C%20Cursor-1d4ed8" alt="目标 Runtime">
  <img src="https://img.shields.io/badge/language-Go-00ADD8" alt="Go">
</p>

<p align="center">
  <a href="README.md">English</a> | 简体中文 | <a href="README.ja.md">日本語</a> | <a href="README.ko.md">한국어</a> | <a href="README.es.md">Español</a> | <a href="README.pt-BR.md">Português</a> | <a href="README.fr.md">Français</a>
</p>

Agent VM，简称 `avm`，是一个面向 AI Coding Agent 的本地控制平面。它让你只定义一次 Agent Profile，再把这个 Profile 渲染到 Codex、Claude Code、Cline、Cursor 等不同 Agent runtime。

核心判断：开发者不会只用一个 Coding Agent。真正缺失的是一个可迁移对象，用来描述“这个 Agent 是谁、能用什么、偏好什么模型设置、拥有什么权限、默认携带哪些长期记忆”。

<p align="center">
  <img src="assets/avm-before-after.svg" alt="使用 AVM 前配置散落在各 runtime；使用 AVM 后一个 profile 激活 agent" width="100%">
</p>

## 核心动作

```bash
avm use backend-coder
```

这个命令应该成为切换本地 AI Coding 环境的肌肉记忆。你不再需要在 prompt 文件、MCP 配置、rules 目录和 memory 笔记里重复搭建同一个角色；AVM 把 Agent Profile 变成 source of truth。

```text
backend-coder.yaml
  -> avm use backend-coder
    -> Codex profile
    -> Claude Code agent
    -> Cline rules
    -> Cursor rules
```

## 差异点

| 方案 | 管理什么 | 缺什么 |
| --- | --- | --- |
| Dotfiles | 文件和软链 | 没有 Agent 对象，也没有字段映射状态 |
| MCP 配置管理器 | 工具服务器配置 | 通常不管理角色、memory、模型和权限 |
| Runtime 原生 profile | 单一生态 | 很难迁移到其他工具 |
| Agent VM | Agent Profile、capabilities、memory refs、adapters | 早期项目，具体 adapter 仍在建设中 |

AVM 不试图把所有 runtime 强行抹平成一个接口。每个 adapter 都必须报告字段映射结果：`native`、`rendered_as_instructions`、`ignored` 或 `unsupported`。

## 一个 Profile 携带什么

| 层级 | 示例 |
| --- | --- |
| 身份 | `backend-coder`、`pr-reviewer`、`incident-runner` |
| Runtime | `codex`、`claude-code`、`cline`、`cursor` |
| 模型运行参数 | 模型名、reasoning effort、verbosity |
| 能力 | skills、commands、hooks、MCP servers、toolsets |
| 权限 | approval mode、sandbox intent、allow/deny policy |
| 记忆引用 | 项目架构、团队约定、用户偏好 |

## Recipes

<details open>
<summary><strong>backend-coder</strong></summary>

```yaml
name: backend-coder
runtime:
  preferred: codex
model_run:
  model: gpt-5.4
  reasoning_effort: high
capabilities:
  skills: [git, test, migration]
  mcps: [github, postgres-readonly]
permissions:
  approval: on-risky-actions
  sandbox: workspace-write
memory_refs:
  - id: backend-standards
    scope: project
    mode: read
```

</details>

<details>
<summary><strong>pr-reviewer</strong></summary>

```yaml
name: pr-reviewer
runtime:
  preferred: claude-code
capabilities:
  skills: [review, security, test-analysis]
  mcps: [github]
permissions:
  approval: never
  sandbox: read-only
memory_refs:
  - id: review-policy
    scope: team
    mode: read
```

</details>

<details>
<summary><strong>incident-runner</strong></summary>

```yaml
name: incident-runner
runtime:
  preferred: codex
capabilities:
  skills: [diagnose, summarize, runbook]
  mcps: [logs-readonly, github]
permissions:
  approval: prompt
  sandbox: read-only
memory_refs:
  - id: incident-runbooks
    scope: team
    mode: read
```

</details>

## 当前状态

这是一个早期预览项目。核心模型和第一批 CLI vertical slice 已经就位；下一阶段重点是 profile activation。

现在可用：

- `avm init`
- `avm agent create/list/show`
- `avm env create`
- `avm memory import --from <file> --dry-run`
- config validation 和 resolution tests
- adapter contract、fake adapter、Phase 1 fixtures

正在建设：

- `avm use <profile-or-env>`
- `avm status`
- `avm deactivate`
- 具体 Codex 和 Claude Code adapter 写入路径
- release packaging

## 快速开始

前置条件：

- Go 1.22+

从源码运行：

```bash
git clone https://github.com/xz1220/Agent-VM.git
cd Agent-VM

go run ./cmd/avm --help
go run ./cmd/avm init
```

创建一个 profile：

```bash
go run ./cmd/avm agent create backend-coder \
  --runtime codex \
  --model gpt-5.4 \
  --reasoning high \
  --skills git,test \
  --mcps github \
  --memory backend-standards:project
```

查看 profile：

```bash
go run ./cmd/avm agent list
go run ./cmd/avm agent show backend-coder
```

预览 portable memory 导入：

```bash
go run ./cmd/avm memory import \
  --from testdata/memory/backend-standards.md \
  --dry-run
```

本地构建：

```bash
make build
./bin/avm --help
```

## 目标 CLI 体验

这是 activation 完成后的 Phase 1 目标主循环：

```bash
avm init
avm agent create backend-coder --runtime codex --skills git,test
avm use backend-coder
avm status
```

期望的 status 形态：

```text
active   profile:backend-coder
runtime  codex          native: model, permissions
runtime  claude-code    rendered: skills, memory_refs
runtime  cline          unsupported: lifecycle_hooks
```

## 安全模型

AVM 默认采取保守策略：

- `avm init` 只写入 `~/.avm`。
- runtime-native memory 只通过显式命令导入。
- memory import 支持写入前 dry-run 报告。
- adapter 只能写自己声明管理的路径。
- runtime 无法表达的字段必须被报告，不能静默丢弃。
- secrets 应通过环境变量引用，不应以明文导出到 portable profile。

## Roadmap

| Phase | 主题 | 重点 |
| --- | --- | --- |
| 1 | 本地 Profile Activation | `avm use <profile>` |
| 2 | Runtime Coverage | Codex、Claude Code、Cline、Cursor adapters |
| 3 | Portable Memory | 显式 import/export/diff/push/pull |
| 4 | Team Registry | 可共享、可审计、有策略约束的团队 Agent Profile |

详见 [ROADMAP.md](ROADMAP.md)。

## 项目文档

- [设计系统](DESIGN.md)
- [产品需求文档](docs/product/prd.md)
- [技术设计](docs/design/tech-design.md)
- [架构](docs/engineering/architecture.md)
- [数据模型](docs/engineering/data-model.md)
- [实现计划](docs/engineering/implementation-plan.md)
- [验收标准](docs/engineering/acceptance.md)
- [GitHub Launch Checklist](docs/marketing/github-launch-checklist.md)

## 开发

```bash
make test
make vet
make fmt
make build
```

主包是 `cmd/avm`。核心包位于 `internal/config`、`internal/adapter`、`internal/memory`、`internal/sync` 和 `internal/state`。

## 贡献

AVM 还在早期阶段。当前最有价值的贡献是小而具体的改动：

- Codex、Claude Code、Cline、Cursor、GitHub Copilot custom agents 的 runtime mapping 调研
- adapter fixtures
- CLI 行为测试
- 解释真实工作流的文档
- 来自多 Agent 工具用户的 bug report 和使用反馈

详见 [CONTRIBUTING.md](CONTRIBUTING.md)。

## License

项目还没有选择开源协议。在 license 被添加之前，代码可以阅读，但不能被默认视作具备广泛复用权利的开源项目。
