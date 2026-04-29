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
  <img src="https://img.shields.io/badge/runtime-Codex%20%7C%20Claude%20Code%20%7C%20OpenCode-1d4ed8" alt="目标 Runtime">
  <img src="https://img.shields.io/badge/language-Go-00ADD8" alt="Go">
</p>

<p align="center">
  <a href="README.md">English</a> | 简体中文
</p>

Agent VM，简称 `avm`，是一个面向 AI Coding Agent 的本地控制平面。它把 Agent 的角色、工具、权限、模型偏好和 memory refs 放进一个可迁移 Profile，再通过 adapter 渲染到 Codex、Claude Code、OpenCode 等 runtime。Cline 和 Cursor 作为兼容 adapter 保留；Cursor 支持是保守的 Phase 1 rules/MCP PoC。

<p align="center">
  <img src="assets/avm-before-after.svg" alt="使用 AVM 前配置散落在各 runtime；使用 AVM 后一个 profile 激活 agent" width="100%">
</p>

## 核心动作

```bash
avm create
eval "$(avm activate <agent-name>)"
```

从 package、已有 profile 或 runtime 导入候选项创建 agent，在当前 shell 中激活它，然后启动 runtime。你不再需要在 prompt 文件、MCP 配置、rules 目录和 memory 笔记里重复搭建同一个角色；AVM 把 Agent Profile 变成 source of truth。`avm use` 仍保留用于显式激活和 sync profile/env。

```text
package / profile / runtime import candidate
  -> avm create
    -> <agent-name>.yaml
      -> eval "$(avm activate <agent-name>)"
        -> Codex profile
        -> Claude Code agent
        -> OpenCode config/agent/skills
        -> Cline rules/MCP settings
        -> Cursor rules/MCP PoC
```

## 差异点

| 方案 | 管理什么 | 缺什么 |
| --- | --- | --- |
| Dotfiles | 文件和软链 | 没有 Agent 对象，也没有字段映射状态 |
| MCP 配置管理器 | 工具服务器配置 | 通常不管理角色、memory、模型和权限 |
| Runtime 原生 profile | 单一生态 | 很难迁移到其他工具 |
| Agent VM | Agent Profile、capabilities、memory refs、adapters | 早期项目；Phase 1 adapter 保守写入并报告 mapping status |

AVM 不试图把所有 runtime 强行抹平成一个接口。每个 adapter 都必须报告字段映射结果：`native`、`rendered_as_instructions`、`ignored` 或 `unsupported`。

## 一个 Profile 携带什么

| 层级 | 示例 |
| --- | --- |
| 身份 | `backend-coder`、`pr-reviewer`、`incident-runner` |
| Runtime | `codex`、`claude-code`、`opencode`、`cline`、`cursor` |
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

这是一个早期预览项目。核心模型、Stage 5 CLI hardening 和受控 activation 路径已经就位。

现在可用：

- `avm init`
- `avm create <package>`、`avm create --from <profile>`、`avm create --from-import <runtime>/<candidate>`，用于首次创建 profile
- `avm package list/show`，用于查看内置 create package
- `avm skill list`，用于查看已安装 skills
- `avm runtime list/scan`，用于查看 runtime 探测结果和可导入候选项
- `avm agent create/list/show`，包括 `avm agent show --runtime <runtime>`
- `avm env create`，包括 `avm env create --local`
- `avm memory import --from <file> --dry-run`
- `avm use`、`avm status`、`avm deactivate`
- `avm sync`
- `avm shell init bash|zsh|fish`
- `avm export`、`avm import` 和 `avm install <file.avm.zip>`
- `avm init` runtime import/report scan 和 `state/import-report.json`
- Codex、Claude Code、OpenCode、Cline 和 Cursor 的 AVM-managed render 输出
- config validation 和 resolution tests
- adapter contract、fake adapter、Phase 1 fixtures

Cursor Phase 1 成功写入时状态保持 `synced`；partial support 通过 warnings 和 mapping status 暴露，而不是 Cursor 独有的 sync 状态。

仍属于 post-MVP 或策略 follow-up：

- config/defaults/active state、项目覆盖、runtime 输出以及交互式 overwrite/rename 的更完整 package policy

## 快速开始

安装已发布的 preview release。默认会把 `avm` 安装到 `$HOME/.local/bin`，
把 shell integration 写入当前 shell 的 rc 文件，并自动初始化 `~/.avm`；
如果只想安装二进制，可以设置 `AVM_SKIP_INIT=1`。

```bash
curl -fsSL https://raw.githubusercontent.com/xz1220/Agent-VM/main/scripts/install.sh | sh
```

安装后重启 shell，或者 source 安装脚本提示的 rc 文件。

创建第一个 agent。无参数执行 `avm create` 时会进入交互式终端向导，你可以用
方向键和空格，从内置 package、已有 AVM profile，或者 runtime 扫描出来的候选项开始，
并选择 runtimes、skills 和 MCP servers。

```bash
avm create
```

如果你已经知道要创建什么，也可以直接用 flags：

```bash
avm create backend-coder
avm create --from default --name api-coder
avm create --from-import claude-code/reviewer --name reviewer-copy
```

在当前 shell 中激活 profile，然后启动 runtime：

```bash
avm use backend-coder
codex
```

如果想用其他 runtime，或者把同一个 profile 渲染到多个 runtime，可以在交互式流程里选择，
也可以直接传 flag：

```bash
avm create backend-coder --runtime opencode
avm create backend-coder --runtimes codex,opencode
avm use backend-coder
opencode
```

如果还没有加载 shell integration，可以用 eval-safe 的 fallback：

```bash
eval "$(avm activate backend-coder)"
```

创建前可以先看 AVM 在本地能发现什么：

```bash
avm package list
avm skill list
avm runtime scan
avm runtime list
```

`avm skill list` 在未激活时展示已安装 skills 和简介；在已激活 shell 中默认只展示
当前 profile 选中的 skills，想看全局 registry 可以用 `avm skill list --all`。
`avm runtime scan` 会刷新只读 runtime import report，并把原生 runtime 中发现的
skills/MCP servers 导入 AVM 的全局 registry；`avm runtime list` 会给出可直接复制的
`avm create --from-import ...` 创建命令。

如果想从源码试一遍真实首次使用路径，并且不碰真实 `~/.avm`：

```bash
git clone https://github.com/xz1220/Agent-VM.git
cd Agent-VM
scripts/dev/avm-runtime-home-test-env.sh start
```

这个测试 shell 会创建一个干净的临时 `HOME/project`，默认不安装也不初始化 AVM。
进入后会打印一个 helper；它会运行本地源码安装脚本，并在当前测试 shell 里加载
shell integration：

```bash
avm-install-local
avm create
avm use <agent-name>
```

如果想让干净 HOME 带上当前机器的 Codex、Claude Code、OpenCode 配置快照，
可以在 `start` 前设置 `AVM_TEST_COPY_RUNTIME_CONFIG=1`。测试 shell 也会从当前
shell 或真实 shell 中复制 allowlist 内的 Claude/Anthropic 登录相关环境变量。

如果需要旧的预置 demo 环境，可以显式执行：

```bash
scripts/dev/avm-runtime-home-test-env.sh seed
```

更多 CLI 示例：

```bash
avm agent create backend-coder \
  --runtime codex \
  --model gpt-5.4 \
  --reasoning high \
  --skills git,test \
  --mcps github \
  --memory backend-standards:project

avm agent create reviewer --runtime claude-code --skills review
avm agent create opencode-coder --runtime opencode --skills git,test
avm agent create cline-helper --runtime cline --skills test
avm agent create cursor-helper --runtime cursor --skills rules

avm agent list
avm agent show backend-coder
avm skill list
```

创建一个 environment，把不同 runtime 映射到不同 profile：

```bash
avm env create all-runtimes \
  --codex backend-coder \
  --claude-code reviewer \
  --opencode opencode-coder \
  --cline cline-helper \
  --cursor cursor-helper

avm env create all-runtimes --local --codex backend-coder
```

写入前预览 portable memory 导入：

```bash
avm memory import \
  --from testdata/memory/backend-standards.md \
  --dry-run \
  --format json
```

激活、查看、重新 sync、退出：

```bash
avm use --kind env all-runtimes
avm status
avm sync
avm deactivate
```

Shell integration 会输出可被 eval 的 snippet，并用 shell function 包装
`avm use`，让当前 shell 立即拿到 `CODEX_HOME`、`CLAUDE_CONFIG_DIR` 等 runtime env：

```bash
eval "$(avm shell init zsh)"
```

导出和安装 package：

```bash
avm export backend-coder --kind agent --output backend-coder.avm.zip
avm install backend-coder.avm.zip
```

不进入测试 shell，直接本地构建：

```bash
make build
./bin/avm --help
```

## 当前 status 形态

当前 activation 主循环是：

```bash
avm create backend-coder
eval "$(avm activate backend-coder)"
avm status
```

期望的 status 形态：

```text
active: profile:backend-coder
runtime status:
  codex: synced (agent backend-coder)
managed paths:
  codex:
    - ~/.avm/runtime-homes/profile-backend-coder/codex/config.toml owner=avm merge=whole-file
    - ~/.avm/runtime-homes/profile-backend-coder/codex/agents/backend-coder.toml owner=avm merge=whole-file
mapping status:
  codex:
    - capabilities.skills -> .../agents/backend-coder.toml#developer_instructions: rendered_as_instructions
warnings:
  none
```

## 安全模型

AVM 默认采取保守策略：

- installer 初始化和 `avm init` 只写入 `~/.avm`。
- runtime-native memory 只通过显式命令导入。
- memory import 支持写入前 dry-run 报告。
- adapter 只能写自己声明管理的路径。
- runtime 无法表达的字段必须被报告，不能静默丢弃。
- secrets 应通过环境变量引用，不应以明文导出到 portable profile。

## Roadmap

| Phase | 主题 | 重点 |
| --- | --- | --- |
| 1 | 本地 Profile Activation | `avm use <profile>` |
| 2 | Runtime Coverage | Codex、Claude Code、OpenCode，加上 Cline/Cursor 兼容 adapters |
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

- Codex、Claude Code、OpenCode、Cline、Cursor、GitHub Copilot custom agents 的 runtime mapping 调研
- adapter fixtures
- CLI 行为测试
- 解释真实工作流的文档
- 来自多 Agent 工具用户的 bug report 和使用反馈

详见 [CONTRIBUTING.md](CONTRIBUTING.md)。

## License

项目还没有选择开源协议。在 license 被添加之前，代码可以阅读，但不能被默认视作具备广泛复用权利的开源项目。
