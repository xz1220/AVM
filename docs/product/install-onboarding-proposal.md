# AVM 安装与首次使用路径方案

> 状态：proposal
> 分支：`proposal/install-onboarding-path`
> 目标：把 AVM 的首次体验从“开发者源码路径”收敛为“安装后直接从 package 创建并激活一个 Agent”。

## 1. 当前问题

现在的 README 和 CLI 更像工程预览，不像用户可直接安装使用的工具。

主要摩擦点：

- 安装路径仍以 `git clone`、`go run ./cmd/avm`、`make build` 为主。用户看不到一个稳定的 release binary、安装脚本或包管理入口。
- 安装后还要求用户手动执行 `avm init`。这一步对用户没有明显价值，且容易让首次路径变长。
- `agent create` 依赖很多 flag，例如 `--runtime`、`--skills`、`--mcps`、`--memory`。这适合脚本和 CI，不适合第一次理解概念。
- README 主路径强调 `avm use`，但 CLI runtime 的真实生效路径还需要 `avm activate` 输出 shell env，例如 `CODEX_HOME`、`CLAUDE_CONFIG_DIR`、`OPENCODE_CONFIG`。首次用户容易误解“use 之后我到底怎么启动 Codex/Claude/OpenCode”。
- `export/import` 已经能表达 `.avm.zip` package，但还没有面向用户的 package 发现、创建和安装路径。

## 2. 推荐的首次用户路径

目标路径应该短到用户能在 1 分钟内完成：

```bash
curl -fsSL https://raw.githubusercontent.com/xz1220/Agent-VM/main/scripts/install.sh | sh
avm create backend-coder
eval "$(avm activate backend-coder)"
codex
```

安装后，用户不需要显式执行 `avm init`。`avm create backend-coder` 从内置 package 进入交互式终端 UI，引导用户创建第一个 Agent Profile。

如果用户已经启用 shell integration，最终路径可以进一步变成：

```bash
avm create backend-coder
avm use backend-coder
codex
```

当前实现已经通过 `avm shell init` 安装 shell function 包装 `avm use`。二进制 `avm use` 仍只负责 sync/persist；加载 shell integration 后，用户输入的 `avm use <profile>` 会在当前 shell 中执行 `avm activate <profile>` 并导出 runtime env。

## 3. 安装分发设计

### 3.1 一键安装脚本

新增 `scripts/install.sh`，职责：

1. 检测 OS/arch。
2. 从 GitHub Releases 下载对应 `avm` binary。
3. 校验 checksum。
4. 安装到用户目录，默认 `~/.local/bin/avm`。
5. 写入 shell integration，包含 `PATH` 和 gvm-style `avm use` wrapper。
6. 默认执行一次 `avm init --yes`，如果已经初始化则 no-op。

建议环境变量：

```bash
AVM_VERSION=0.1.0
AVM_INSTALL_DIR="$HOME/.local/bin"
AVM_INSTALL_SHELL_INTEGRATION=0
AVM_SKIP_INIT=1
```

安装脚本可以默认初始化，因为用户明确执行的是 AVM 的 installer。但要保留 `AVM_SKIP_INIT=1` 给 CI、容器镜像和 package manager 使用。

### 3.2 GitHub Releases

每个 preview release 发布：

- `avm_Darwin_arm64.tar.gz`
- `avm_Darwin_x86_64.tar.gz`
- `avm_Linux_arm64.tar.gz`
- `avm_Linux_x86_64.tar.gz`
- `checksums.txt`

后续可加入 Windows，但第一轮可以不阻塞。

### 3.3 源码安装仍保留

源码路径作为高级路径保留：

```bash
go install github.com/xz1220/agent-vm/cmd/avm@latest
avm create backend-coder
```

`go install` 不能可靠地执行 post-install hook，所以不能依赖它自动 init。这里应由 CLI lazy init 保证首次 `avm create` 可直接工作。

## 4. 初始化策略

推荐采用“双保险”：

1. installer 默认执行 `avm init --yes`。
2. CLI 对需要 AVM home 的命令做 lazy init。

也就是说，`avm init` 不再是用户必须记住的第一步，而是一个显式修复/重建命令。

### 4.1 命令行为

- `avm --help`、`avm version`、completion：不写文件。
- `avm status`：如果未初始化，输出 “not initialized; run avm create or avm init”，不自动写入。
- `avm create`、`avm agent create`、`avm env create`、`avm import`、`avm install`、`avm activate`、`avm use`、`avm sync`：如果 `~/.avm/config.yaml` 不存在，自动执行初始化。
- `avm init`：保留手动入口；已初始化时默认 no-op 或给出清晰状态，不应把正常用户带入错误路径。
- `avm init --force`：仍用于重建默认 config/agent/env，但不得删除用户额外文件。

### 4.2 实现建议

新增内部 helper：

```go
EnsureInitialized(mode InitMode) error
```

其中 mode 区分：

- `InitNever`：help、version、completion。
- `InitReadOnly`：status，只报告不写入。
- `InitAuto`：create/import/activate/use/sync 自动初始化。
- `InitForce`：显式 `init --force`。

这样能避免每个 command 自己判断 `config.GlobalConfigPath()`。

## 5. 交互式创建

### 5.1 推荐入口

新增顶层命令：

```bash
avm create [package]
```

它是新用户默认入口，从 package 创建本地对象并打开交互式 wizard。现有命令继续保留：

```bash
avm agent create backend-coder --runtime codex --skills git,test
avm env create coding --codex backend-coder --claude-code reviewer
```

原则：交互式命令服务人，flag 命令服务脚本。

### 5.2 Wizard 流程

首版 wizard 不需要做全屏 TUI，用 form-style terminal UI 即可。

建议流程：

1. 选择创建对象：
   - Single agent profile
   - Multi-runtime environment
   - Install package as-is
2. 如果选择 profile：
   - 选择来源：package、已有 AVM profile、runtime import-report candidate
   - 如果选择 package，默认 `backend-coder`
   - 如果选择已有 profile，可从 `default` 复制出新 profile
   - 如果选择 import candidate，可把 Claude/OpenCode 等 runtime 已有 agent 抽取成 AVM profile
   - 输入本地 agent name，默认使用 package 建议名
   - 选择 runtime：Codex、Claude Code、OpenCode、Cline、Cursor
   - 选择 model/reasoning，提供默认值
   - 从本地 registry 已安装 skills/MCP 中选择，也可手动输入未安装引用
   - 选择 memory refs，可先跳过
   - 选择保存范围：global 或 current project
3. 展示 preview：
   - 即将写入的 AVM YAML
   - runtime mapping preview
   - managed paths
4. 确认写入。
5. 询问是否立即激活。

结束时输出下一步，而不是只说 created：

```text
created agent backend-coder

To use it in this shell:
  eval "$(avm activate backend-coder)"

Then start your runtime:
  codex
```

### 5.3 库选择

建议优先使用轻量 form 库，而不是直接上全屏 TUI：

- `github.com/charmbracelet/huh`：适合 prompt、select、multi-select、confirm。
- `github.com/charmbracelet/bubbletea`：适合复杂全屏应用，首版没必要。

交互式能力必须满足：

- 非 TTY 环境不弹 prompt，给出稳定错误和建议 `--yes` 或完整 flags。
- 所有 wizard 都有等价的 flag 命令，便于 CI 和文档复现。
- 支持 `--name`、`--runtime`、`--yes`、`--no-input`。
- 支持 `--from <profile>`，用于从已有 AVM profile 克隆。
- 支持 `--from-import <runtime>/<candidate>`，用于从 import report 候选项创建。
- skills 选择需要展示 `SKILL.md` 摘要，避免用户只能靠目录名猜用途。
- 支持 `avm runtime list/scan`，让用户先看清楚现有 runtime 配置能抽取出哪些候选项。

## 6. Package 模型

公开概念只保留 `package`。不再把 template 作为用户需要理解的平级概念。

Package 是 AVM 可分发单元，可以支持两种用法：

| 用法 | 命令 | 用户心智 | 结果 |
| --- | --- | --- | --- |
| 从 package 创建 | `avm create backend-coder` | 用一个起点创建我自己的 agent/env | 生成可改名、可定制的本地对象 |
| 安装 package | `avm install github-reviewer` | 把别人配好的对象装进来 | 原样写入 package 中的 agent/env/capability/memory |

底层 manifest 可以区分 package 是否支持 `create`、`install`，但这不是新的公开名词。

### 6.1 Create Package

示例命令：

```bash
avm create backend-coder
avm create backend-coder --name api-coder --runtime codex --yes
avm package show backend-coder
```

Create package 用于回答“我要创建一个什么”。package 内容应该是数据，不执行任意脚本。首版字段可以包括：

- suggested name，例如 `backend-coder`
- runtime defaults
- model defaults
- skills/MCP/memory suggestions
- prompt/instruction starter
- wizard questions

### 6.2 Install Package

Install package 用于回答“我想把别人已经配好的东西装下来”。

现有 `import <file.avm.zip>` 可以继续作为低层能力。面向用户新增：

```bash
avm install github/backend-coder
avm install https://example.com/backend-coder.avm.zip
avm package list
avm package show github/backend-coder
```

安装 package 必须先 preview：

- 将写入哪些 `~/.avm/**` 文件
- 是否覆盖已有 profile/env
- 是否包含 skills/MCP/memory refs
- 是否引用 secrets 环境变量

默认不自动 activate，除非用户确认或显式传 `--activate`。

### 6.3 Registry

首版 registry 可以是一个 GitHub repo 中的静态 index：

```yaml
version: 1
entries:
  - name: backend-coder
    kind: package
    modes: [create]
    source: packages/backend-coder.yaml
  - name: github/reviewer
    kind: package
    modes: [install]
    source: packages/github-reviewer.avm.zip
    sha256: ...
```

命令：

```bash
avm registry add official https://raw.githubusercontent.com/xz1220/avm-registry/main/index.yaml
avm search reviewer
avm install official/github-reviewer
```

安全边界：

- registry install 只下载数据，不执行 hooks。
- 必须校验 checksum。
- package 不应包含明文 secrets。
- 冲突默认停止，交互式确认后才 overwrite/rename。
- 远程 registry 默认只信任 HTTPS；后续再加签名。

## 7. 推荐阶段拆分

### P1：安装和文档

- 新增 `scripts/install.sh`。
- 新增 release build workflow，产出多平台 binary 和 checksum。
- README Quickstart 改成 install script + `avm create`。
- 源码安装移到 “Install from source”。

验收：

```bash
curl -fsSL .../install.sh | sh
avm --version
avm status
```

### P2：自动初始化

- `avm init` 变成可重复执行的友好命令。
- create/import/activate/use/sync 自动初始化。
- status 未初始化时只提示，不写入。

验收：

```bash
rm -rf ~/.avm
avm agent create backend-coder --runtime codex
avm agent create opencode-coder --runtime opencode
test -f ~/.avm/config.yaml
```

### P3：交互式创建

- 新增 `avm create` wizard。
- `avm agent create` 无参数且在 TTY 中可进入 wizard，或者提示使用 `avm create`。
- 输出明确下一步 `eval "$(avm activate ...)"`。

验收：

```bash
rm -rf ~/.avm
avm create backend-coder
eval "$(avm activate backend-coder)"
codex
```

### P4：Package Create MVP

- 内置 3 个 create package：backend-coder、reviewer、writer。
- `avm create <package>`。
- `avm package list/show`。

验收：

```bash
avm create backend-coder --name api-coder --runtime codex --yes
avm create backend-coder --name api-coder --runtime opencode --yes
avm agent show api-coder
```

### P5：Package/Registry MVP

- 保留现有 `.avm.zip` import/export。
- 新增 `avm install <url-or-registry-ref>`。
- 新增 registry index 格式和 `avm registry add/list`。
- install 先 preview，再确认写入。

验收：

```bash
avm install https://example.com/backend-coder.avm.zip --preview
avm install https://example.com/backend-coder.avm.zip --yes
avm agent list
```

## 8. 需要拍板的问题

1. 顶层命令是否叫 `avm create`？
   - 推荐：是。它是最符合新用户直觉的入口。
   - 现有 `avm agent create` 保持脚本化入口。

2. installer 是否默认执行 init？
   - 推荐：install script 默认执行，package manager 和 `go install` 依赖 lazy init。
   - 原因：一键脚本是用户主动安装 AVM，自动 init 合理；系统包管理器 post-install 写用户 home 不稳定。

3. README 主路径用 `use` 还是 `activate`？
   - 推荐：短期用 `eval "$(avm activate ...)"`，因为它真正影响当前 shell 的 runtime env。
   - 后续 shell integration 可以把 `avm use` 包装成更自然的当前 shell 激活。

4. 是否保留公开的 `template` 概念？
   - 结论：不保留。公开层只有 package；底层可以有 create/install modes。

5. 第一版 registry 是否必须有签名？
   - 推荐：第一版用 HTTPS + checksum + no-script policy，后续再引入签名。

## 9. README 首屏建议

未来 README 的 Quickstart 应该变成：

```bash
curl -fsSL https://raw.githubusercontent.com/xz1220/Agent-VM/main/scripts/install.sh | sh
avm create backend-coder
eval "$(avm activate backend-coder)"
codex
```

然后再解释：

- `Agent Profile`：一个 agent 角色，例如 `backend-coder`。
- `Environment`：多 runtime 映射，例如 Codex 用 backend-coder，Claude Code 用 reviewer，OpenCode 用 opencode-coder。
- `Package`：可创建、分享和安装的 AVM 分发单元。
