# Agent VM — 产品需求文档（PRD）

> 最后更新：2026-05-01（v13 — 用户模块重构）

## 1. 产品定位

Agent VM（`avm`）是 AI Coding Agent 的本地配置管理器。它让用户管理 **Agent** 和 **Environment**，再把这些配置应用到 Codex、Claude Code、OpenCode、Cline、Cursor 等 runtime。

AVM 不应该让用户先理解 runtime 配置文件、skills registry、sync 或 memory 迁移。用户真正关心的是：

1. 我有哪些 Agent？
2. 每个 Agent 会加载哪些 instructions、skills、MCP、权限和模型偏好？
3. 我当前 shell 正在使用哪个 Agent 或工作环境？
4. 我能不能把一个好用的配置复制、修改、删除、分享给别人？

因此产品主线必须收敛为：

```text
安装 AVM
  -> 创建/管理 Agent
    -> 可选：创建/管理 Environment
      -> use Agent 或 Environment
        -> 启动 runtime
```

## 2. 核心原则

### 2.1 用户管理的是 Agent 和 Environment

Agent 是最核心对象。Environment 是多 runtime 场景下的组合对象。Package 是分发机制。Runtime 是生效目标。

不应该把以下内部机制提升为用户主模块：

- skills registry
- sync
- render plan
- runtime managed paths

这些能力可以存在，但应该被包装在 create、edit、use、status 等用户动作里。

### 2.2 Skills 属于 Agent 配置

用户不是为了“管理 skills”而来。用户是在创建或修改 Agent 时，决定这个 Agent 应该具备哪些 skills。

因此 skills 的主要入口应该是：

```bash
avm agent create
avm agent edit <agent>
```

`avm skill list` 可以保留为高级查看/调试命令，但不应该是 README 或产品叙事中的独立模块。

### 2.3 创建来源应保持 AVM 原生

Agent 创建来源应只包含 AVM 能明确表达语义的对象：

- 空白/default Agent
- Package
- 已有 AVM Agent

Claude Code subagents、OpenCode agent markdown 等 runtime-native 文件不是 AVM
Agent 的等价对象，不应自动导入或提升为 AVM Agent。用户如需复用其中的
prompt 内容，应通过明确的 instructions 文件能力完成，而不是通过 runtime
scan/import 伪装成语义等价迁移。

### 2.4 Sync 是 use 的实现细节

用户想要的是：

```bash
avm use backend-coder
codex
```

而不是：

```bash
avm use backend-coder
avm sync
codex
```

`avm sync` 可以作为高级修复/调试命令保留，但主路径应该是 `avm use` 负责让当前 shell 和 runtime managed config 进入正确状态。

### 2.5 Memory 暂缓进入主路径

Memory 很重要，但目前产品心智还没有收敛。Agent / Environment / Package 的 CRUD 和使用路径应先稳定。Memory 在当前阶段只保留实验性能力，不进入主导航。

## 3. 核心对象

### 3.1 Agent

Agent 是用户直接创建、修改、删除、复制、激活和分享的主对象。

Agent 包含：

| 字段 | 用户含义 |
| --- | --- |
| identity | 名称、描述、角色 |
| instructions | system/developer 指令、引用文件 |
| skills | 这个 Agent 应具备的任务能力 |
| MCP servers | 这个 Agent 可连接的外部工具 |
| permissions | 文件、shell、网络、审批和 sandbox 意图 |
| model preferences | model、reasoning、verbosity 等运行偏好 |
| runtime preferences | 首选 runtime 和 fallback runtimes |
| memory refs | 未来可选的长期记忆引用 |

Agent 回答：

> 当我 `avm use backend-coder` 时，我到底激活了一个什么样的 Agent？

### 3.2 Environment

Environment 是一个工作场景。它把一个或多个 runtime 映射到 Agent。

例如：

```yaml
name: work
runtime_agents:
  codex:
    primary: backend-coder
  claude-code:
    primary: reviewer
  opencode:
    primary: opencode-coder
```

Environment 回答：

> 在这个工作场景下，我启动不同 runtime 时应该默认使用哪个 Agent？

只有当用户需要多 runtime 或多角色组合时，才需要 Environment。单 Agent 场景应直接 `avm use <agent>`。

### 3.3 Package

Package 是分发单元，不是日常激活对象。

Package 可以包含：

- Agent
- Environment
- skills
- MCP definitions
- hooks / commands / toolsets
- metadata 和版本信息

用户安装 Package 后，得到的是可管理的 Agent / Environment。日常使用仍然是：

```bash
avm use <agent-or-env>
```

### 3.4 Runtime

Runtime 是 Agent 生效的目标工具，例如 Codex、Claude Code、OpenCode、Cline、Cursor。

Runtime 不应成为用户主要管理对象。它在用户界面中只应该出现在：

- Agent 创建/编辑时选择首选 runtime
- Environment 创建/编辑时做 runtime -> Agent 映射
- status 中显示实际映射和 warnings
- adapter 无法表达某些字段时显示 mapping status

## 4. 用户模块

### 4.1 安装、初始化和卸载

这个模块负责 AVM 自身生命周期。

目标能力：

```bash
avm init
avm doctor
avm uninstall
avm shell install
avm shell uninstall
```

验收标准：

- 安装后用户可以直接运行 `avm agent create`。
- 默认初始化只写入 `~/.avm`。
- shell integration 可安装、可移除、可检测。
- `doctor` 能解释 PATH、shell integration、AVM home、runtime managed homes 的状态。
- `uninstall` 能清楚区分删除二进制、shell integration、`~/.avm` 数据。

当前 preview：

- 已有安装脚本、`avm init`、`avm shell init`。
- 缺少一等的 `doctor`、`uninstall`、`shell install/uninstall`。

### 4.2 Agent 配置 CRUD

这是 P0 模块。

目标能力：

```bash
avm agent create
avm agent list
avm agent show <name>
avm agent edit <name>
avm agent delete <name>
avm agent clone <name> --name <new-name>
avm agent rename <old-name> <new-name>
```

创建来源：

```text
blank/default
package
existing agent
```

创建流程应包含：

1. 选择来源。
2. 设置 Agent 名称和描述。
3. 选择 runtime 偏好。
4. 选择 skills 和 MCP servers。
5. 设置权限和高级模型参数。
6. 展示 preview。
7. 确认写入。

编辑流程应支持：

- 修改基础信息
- 修改 instructions
- 修改 skills
- 修改 MCP
- 修改 runtime/model/permissions
- 预览 runtime mapping 影响

删除流程应支持：

- 删除前展示引用关系，例如哪些 Environment 正在使用这个 Agent。
- 默认不删除 Package 来源或能力本体。
- 如 Agent 正在 active，应提示先 deactivate 或确认切换。

验收标准：

- `create` 不是隐式 overwrite。
- 已存在同名 Agent 时必须明确提示 rename、overwrite 或 cancel。
- `show` 能展示 source path 和 runtime mapping 状态。
- `edit` 和 `delete` 都有 non-interactive flags 或可脚本化模式。

当前 preview：

- 已有 `avm create`、`avm agent create/list/show/edit/delete/rename/clone`。
- 已有 `--from`，`agent clone` 也可复用已有 profile。
- `agent create` 已收紧为同名失败，不再隐式 overwrite。
- `agent edit` 默认交互式，传 field flags 时可脚本化。
- `agent rename/delete` 会保护 active profile 和 env 引用。

### 4.3 Environment 配置 CRUD

Environment 是 P1 模块，但需要完整 CRUD，否则用户无法把“工作场景”作为稳定对象理解。

目标能力：

```bash
avm env create
avm env list
avm env show <name>
avm env edit <name>
avm env delete <name>
avm env clone <name> --name <new-name>
avm env rename <old-name> <new-name>
```

Environment 创建流程：

1. 设置环境名称，例如 `work`、`review`、`frontend`。
2. 选择该环境包含哪些 runtime。
3. 为每个 runtime 选择默认 Agent。
4. 展示映射 preview。
5. 确认写入。

验收标准：

- Environment 不重复定义 skills/MCP/instructions。
- Environment 只引用 Agent。
- 删除 Agent 时能发现 Environment 引用。
- `avm use --kind env <name>` 后，启动不同 runtime 时会使用对应 Agent。

当前 preview：

- 已有 `avm env create`。
- 缺少 list/show/edit/delete/rename/clone。

### 4.4 使用和激活

这是用户每天使用 AVM 的入口。

目标能力：

```bash
avm use <agent-or-env>
avm status
avm deactivate
```

行为要求：

- `avm use <agent>` 激活单个 Agent。
- `avm use --kind env <env>` 激活一个工作场景。
- shell integration 应让当前 shell 立即获得 runtime 环境变量。
- `status` 显示当前 active 对象、runtime 映射、managed paths、warnings。
- adapter mapping 必须清楚展示 unsupported / rendered-as-instructions。

高级能力：

```bash
avm sync
avm activate <agent-or-env>
```

其中：

- `activate` 是 eval-safe fallback。
- `sync` 是调试/修复命令，不应成为主路径。

### 4.5 Package 管理

Package 是复用和分发模块。

目标能力：

```bash
avm package list
avm package show <package>
avm package install <package-or-file>
avm package uninstall <package>
avm package export <agent-or-env>
avm package inspect <file.avm.zip>
```

验收标准：

- 安装 Package 后，用户得到 Agent / Environment。
- Package 不应成为 active 对象。
- Package 安装前必须展示将写入哪些对象。
- 冲突时必须提供 rename、skip、overwrite、cancel。
- export 时能选择是否包含被引用的 skills/MCP/memory。

当前 preview：

- 已有 package list/show/inspect。
- 已有 export/import/install，但命令归属和用户心智还需要统一到 package 模块。

### 4.6 Memory

Memory 暂不进入主路径。

当前原则：

- 保留 `avm memory import --dry-run` 作为实验能力。
- 不在 README 主流程中强调 memory。
- 不让用户在创建第一个 Agent 前理解 memory。

未来再定义：

- memory 是 Agent 引用，还是 Environment override。
- runtime-native memory 如何导入、diff、回滚。
- 团队共享 memory 如何审计。

## 5. 模块关系

```text
Package
  installs
    -> Agent
    -> Environment
    -> referenced capabilities

Agent
  owns
    -> instructions
    -> skills
    -> MCP servers
    -> permissions
    -> model/runtime preferences

Environment
  references
    -> Agent per runtime

Use
  activates
    -> Agent or Environment
  applies to
    -> Runtime managed homes

Runtime
  receives
    -> rendered config from Agent/Environment
  reports
    -> mapping status and warnings
```

核心关系：

- Agent 是能力和行为的归属。
- Environment 是 Agent 的场景映射。
- Package 是 Agent / Environment 的分发载体。
- Runtime 是生效目标，不是主配置对象。
- Skills 是 Agent 的组成部分，不是主模块。
- Sync 是 use 的实现细节，不是主模块。

## 6. 关键用户流程

### 6.1 新用户首次创建 Agent

```bash
curl -fsSL https://raw.githubusercontent.com/xz1220/Agent-VM/main/scripts/install.sh | sh
avm agent create
avm use backend-coder
codex
```

期望体验：

1. 安装后默认初始化。
2. `agent create` 打开交互式 UI。
3. 用户选择来源、runtime、skills、MCP、权限。
4. AVM 展示 preview。
5. 创建成功后给出下一步：`avm use <agent>`。

### 6.2 从已有 Agent 创建新场景

```bash
avm agent clone default --name api-coder
avm agent edit api-coder
avm use api-coder
```

期望体验：

- 用户不需要手写 YAML。
- clone 后可以交互式修改 skills/MCP/runtime。
- 修改前后可以看到 diff。

### 6.3 创建多 runtime 工作环境

```bash
avm env create work
avm use --kind env work
```

期望体验：

- 用户为每个 runtime 选择 Agent。
- `status` 能说明当前 environment 下每个 runtime 使用哪个 Agent。
- Environment 不复制 Agent 配置。

### 6.4 分享一个 Agent

```bash
avm package export backend-coder
avm package inspect backend-coder.avm.zip
```

期望体验：

- export 前展示包含哪些对象。
- install 前展示将写入哪些对象。
- 冲突处理清楚。

## 7. Runtime 映射策略

AVM 不承诺所有 runtime 支持相同字段。每个 adapter 必须报告字段映射状态：

| 状态 | 含义 |
| --- | --- |
| native | runtime 有原生字段承接 |
| rendered_as_instructions | runtime 没有结构化字段，只能渲染成说明文本 |
| ignored | AVM 有意不写入，通常为了保护用户文件 |
| unsupported | runtime 当前无法表达 |

模型和 reasoning 的用户呈现规则：

- 它们是高级 Agent 设置。
- 创建/编辑时应根据所选 runtime 显示支持状态。
- Codex / Claude Code 支持更接近原生映射。
- OpenCode、Cline、Cursor 等 runtime 应明确显示降级或 unsupported 状态。

## 8. 非目标

当前阶段不做：

- 把 skills 做成用户主模块。
- 从 runtime-native subagents 或 agent markdown 自动创建 AVM Agent。
- 把 sync 作为用户必须理解的日常操作。
- 在 Agent / Environment CRUD 完整前扩展 memory 主线。
- 假装所有 runtime 都有一致的 Agent Profile 能力。
- 默认覆盖用户 runtime 原生配置文件。

## 9. 当前实现差距

| 模块 | 当前已有 | 需要补齐 |
| --- | --- | --- |
| 安装/初始化 | installer、`init`、`shell init` | doctor、uninstall、shell install/uninstall |
| Agent | `create`、`agent create/list/show/edit/delete/rename/clone` | 更完整的交互式 create、批量引用迁移体验 |
| Environment | `env create` | list/show/edit/delete/rename/clone |
| Use | `use`、`activate`、`status`、`deactivate`、`sync` | 把 sync 从主路径隐藏到 use/apply 语义 |
| Package | package list/show/inspect、export/import/install | 命令归属统一、冲突策略、package uninstall |
| Skills | skill list、create 时选择 | 放入 agent edit/create 主流程 |
| Memory | import dry-run | 暂缓产品化 |

## 10. 成功标准

P0 成功标准：

- 新用户能在安装后 3 分钟内创建并使用第一个 Agent。
- 用户能完整 CRUD Agent，而不需要手写 YAML。
- 删除或覆盖 Agent 前有明确确认和引用检查。
- README 主路径不再要求用户理解 skill registry 或 sync。

P1 成功标准：

- 用户能完整 CRUD Environment。
- Package 能可靠安装/导出 Agent 和 Environment，并处理冲突。

P2 成功标准：

- Memory 产品边界被重新定义，并且不会破坏 Agent / Environment 的清晰心智。
