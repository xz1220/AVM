# Agent VM — 产品需求文档（PRD）

> 最后更新：2026-05-06（v14 — Agent-first 主线收敛）

## 1. 产品定位

Agent VM（`avm`）是 AI Coding Agent 的本地配置管理器。它让用户管理 **Agent**，再把选中的 Agent 应用到 Codex、Claude Code、OpenCode、Cline、Cursor 等 runtime。

AVM 不应该让用户先理解 runtime 配置文件、skills registry 或 sync。用户真正关心的是：

1. 我有哪些 Agent？
2. 每个 Agent 会加载哪些 instructions、skills、MCP、权限和模型偏好？
3. 我当前 shell 或当前命令正在使用哪个 Agent？
4. 这个 Agent 应该通过哪个 runtime 启动？
5. 我能不能把一个好用的配置复制、修改、删除、分享给别人？

因此产品主线必须收敛为：

```text
安装 AVM
  -> 创建/管理 Agent
    -> run/use Agent
      -> AVM 解析或确认 runtime
        -> 启动 runtime 或写入 runtime managed config
```

Environment/Scenario 可以作为未来的组合能力存在，但当前产品主线只保留一个
default 场景，不把 Environment 作为核心用户对象推广。

## 2. 核心原则

### 2.1 用户管理的是 Agent

Agent 是当前阶段唯一的核心用户对象。Package 是分发机制。Runtime 是 Agent 的生效目标和执行载体。

Environment 不进入当前核心能力。当前只需要一个 default 场景来承载全局 active
Agent；多 runtime 多 Agent 的 Scenario/Environment 可以作为未来扩展，但不能影响
P0 的用户心智和 CLI 主路径。

不应该把以下内部机制提升为用户主模块：

- skills registry
- sync
- render plan
- runtime managed paths

这些能力可以存在，但应该被包装在 create、edit、use、status 等用户动作里。

### 2.2 用户选择 Agent，AVM 解析 Runtime

多个 Agent 可以支持同一个 runtime，但同一个 runtime 启动时只能绑定一个明确的
Agent。例如 `coding` 和 `research` 都支持 Codex 时，用户输入 `codex` 并不能让
AVM 推断应该启动哪一个 Agent。

因此主语义必须是 Agent-first：

```bash
avm run coding
avm run research
avm run coding --runtime codex
avm run coding --runtime claude-code
```

规则：

- 启动对象是 Agent，不是 runtime。
- Runtime 是 Agent 的执行载体。
- Agent 可以声明 preferred runtime 和 fallback runtimes。
- `avm run <agent>` 在只有一个明确 runtime 时直接启动。
- 如果 Agent 支持多个 runtime 且无法从配置唯一确定，交互模式询问用户；非交互模式要求传 `--runtime`。
- `avm use <agent>` 只设置当前 shell 的 active Agent；随后用户直接运行 `codex`、`claude` 等命令时，runtime 使用这个 active Agent 的投射配置。

`avm run <runtime>` 可以作为当前实现或特殊 runtime 的兼容入口保留，但它不是目标产品语义。

### 2.3 Skills 属于 Agent 配置

用户不是为了“管理 skills”而来。用户是在创建或修改 Agent 时，决定这个 Agent 应该具备哪些 skills。

因此 skills 的主要入口应该是：

```bash
avm agent create
avm agent edit <agent>
```

`avm skill list` 可以保留为高级查看/调试命令，但不应该是 README 或产品叙事中的独立模块。

### 2.4 创建来源应保持 AVM 原生

Agent 创建来源应只包含 AVM 能明确表达语义的对象：

- 空白/default Agent
- Package
- 已有 AVM Agent

Claude Code subagents、OpenCode agent markdown 等 runtime-native 文件不是 AVM
Agent 的等价对象，不应自动导入或提升为 AVM Agent。用户如需复用其中的
prompt 内容，应通过明确的 instructions 文件能力完成，而不是通过 runtime
scan/import 伪装成语义等价迁移。

### 2.5 Sync 是 use/run 的实现细节

用户想要的是：

```bash
avm run backend-coder
```

或者在安装 shell integration 后：

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

`avm sync` 可以作为高级修复/调试命令保留，但主路径应该是 `avm run` 或 `avm use` 负责让当前 shell 和 runtime managed config 进入正确状态。

### 2.6 Memory 不属于当前配置模型

Memory 很重要，但目前产品语义还没有收敛。当前阶段不提供 `avm memory` 命令，不在 Agent schema 中暴露 memory refs，也不通过 Package 携带 memory 文件。

后续必须先讨论原则，再决定是否实现：

- memory 是否应该由 AVM 管理，还是保持 runtime-native。
- AVM 当前只讨论全局/用户级 runtime memory 的隔离，不讨论 memory 内容管理、导入导出、重置或开关。项目级 memory 是项目资产，不由 AVM 管理。
- AVM 是否只观察/审计，不成为 source of truth。
- 忘记、审计、冲突、回滚如何表达。

## 3. 核心对象

### 3.1 Agent

Agent 是用户直接创建、修改、删除、复制、启动、激活和分享的主对象。

Agent 包含：

| 字段 | 用户含义 |
| --- | --- |
| identity | 名称、描述、角色 |
| instructions | system/developer 指令、引用文件 |
| skills | 这个 Agent 应具备的任务能力 |
| MCP servers | 这个 Agent 可连接的外部工具 |
| permissions | 文件、shell、网络、审批和 sandbox 意图 |
| model preferences | model、reasoning、verbosity 等运行偏好 |
| runtime preferences | 这个 Agent 可使用的 runtime、首选 runtime 和 fallback runtimes |

Agent 回答：

> 当我 `avm run backend-coder` 或 `avm use backend-coder` 时，我到底启动或激活了一个什么样的 Agent？

Agent 不回答：

> 当前 Codex 应该从多个 Agent 里自动选择哪一个？

这个选择必须由用户通过 Agent name 完成。

### 3.2 Environment / Scenario（未来扩展）

Environment 不是当前核心对象。当前阶段只需要一个 default 场景；已有 env 相关实现可以作为实验能力或兼容能力保留，但不进入 README 主线、P0 验收或日常用户路径。

未来如果引入 Scenario，它表示一个命名工作场景，把一个或多个 runtime 映射到 Agent。

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

Scenario 回答：

> 在这个工作场景下，我启动不同 runtime 时应该默认使用哪个 Agent？

只有当用户需要多 runtime 多角色组合时，才需要 Scenario。单 Agent 场景必须直接 `avm run <agent>` 或 `avm use <agent>`。

### 3.3 Package

Package 是分发单元，不是日常激活对象。

Package 可以包含：

- Agent
- Scenario/Environment（未来扩展）
- skills
- MCP definitions
- hooks / commands / toolsets
- metadata 和版本信息

用户安装 Package 后，得到的是可管理的 Agent。未来如果支持 Scenario，Package
也可以携带 Scenario。日常使用仍然是：

```bash
avm run <agent>
```

### 3.4 Runtime

Runtime 是 Agent 生效的目标工具，例如 Codex、Claude Code、OpenCode、Cline、Cursor。

Runtime 不应成为用户主要管理对象。它在用户界面中只应该出现在：

- Agent 创建/编辑时选择首选 runtime
- `avm run <agent>` 无法唯一确定 runtime 时让用户选择
- 未来 Scenario 创建/编辑时做 runtime -> Agent 映射
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

- 删除前展示引用关系，例如 active Agent、Package 元数据或未来 Scenario 正在使用这个 Agent。
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
- `agent rename/delete` 会保护 active profile 和已有 env 引用。

### 4.3 Default 场景与未来 Scenario

当前阶段不把 Environment 做成核心用户模块。AVM 只需要一个 default 场景来表示当前
Agent-first 的激活上下文。

当前规则：

- 用户不需要理解或管理 Environment。
- README 和日常路径不推广 `avm env`。
- 已有 `avm env` 命令可以暂时保留，但视为实验/兼容入口。
- 默认不为 Environment 补齐完整 CRUD。
- Agent 删除、重命名、Package 导入导出等 P0 流程不应因为 Environment 扩展而复杂化。

未来如果确实需要多 runtime 多角色编排，可以把 Environment 重新设计为
Scenario。届时再引入完整能力：

```bash
avm scenario create
avm scenario list
avm scenario show <name>
avm scenario edit <name>
avm scenario delete <name>
avm scenario clone <name> --name <new-name>
avm scenario rename <old-name> <new-name>
```

Scenario 创建流程：

1. 设置场景名称，例如 `work`、`review`、`frontend`。
2. 选择该场景包含哪些 runtime。
3. 为每个 runtime 选择默认 Agent。
4. 展示映射 preview。
5. 确认写入。

验收标准：

- Scenario 不重复定义 skills/MCP/instructions。
- Scenario 只引用 Agent。
- 删除 Agent 时能发现 Scenario 引用。
- `avm use <scenario>` 不能和 `avm use <agent>` 产生歧义；必要时应使用独立命令或显式 kind。

当前 preview：

- 已有部分 `avm env` 实现。
- 暂不继续扩展为核心能力。
- PRD、README 和后续实现应优先收敛 Agent-first 主线。

### 4.4 使用和激活

这是用户每天使用 AVM 的入口。

目标能力：

```bash
avm run <agent>
avm run <agent> --runtime <runtime>
avm use <agent>
avm status
avm deactivate
```

行为要求：

- `avm run <agent>` 是最清晰的日常启动入口。
- `avm run <agent> --runtime <runtime>` 显式选择执行载体。
- 当一个 Agent 支持多个 runtime 且没有唯一选择时，交互模式询问，非交互模式报错。
- 多个 Agent 可以支持同一个 runtime，但 runtime 启动时不能反向猜测 Agent。
- `avm use <agent>` 激活单个 Agent，让当前 shell 后续启动 runtime 时使用这个 Agent。
- shell integration 应让当前 shell 立即获得 runtime 环境变量。
- `status` 显示当前 active Agent、runtime 映射、managed paths、memory isolation
  status、warnings。
- adapter mapping 必须清楚展示 unsupported / rendered-as-instructions。

高级能力：

```bash
avm sync
avm activate <agent>
```

其中：

- `activate` 是 eval-safe fallback。
- 当前实现中的 `run <runtime>` 可以作为过渡或 OpenCode 进程级隔离入口，但目标语义应迁移到 `run <agent> [--runtime <runtime>]`。
- `sync` 是调试/修复命令，不应成为主路径。

### 4.5 Package 管理

Package 是复用和分发模块。

目标能力：

```bash
avm package list
avm package show <package>
avm package install <package-or-file>
avm package uninstall <package>
avm package export <agent>
avm package inspect <file.avm.zip>
```

验收标准：

- 安装 Package 后，用户得到 Agent。
- Package 不应成为 active 对象。
- Package 安装前必须展示将写入哪些对象。
- 冲突时必须提供 rename、skip、overwrite、cancel。
- export 时能选择是否包含被引用的 skills/MCP。

当前 preview：

- 已有 package inspect/install/export。
- export/import/install 的用户心智还需要统一到 package 模块。
- 如果当前实现支持导出 Environment，应视为未来 Scenario 兼容能力，不进入 P0 主线。

### 4.6 Memory

Memory 已从当前 AVM 产品模型中删除。

当前原则：

- 不提供 `avm memory` 命令。
- 不在 Agent / Scenario / Package schema 中声明 memory。
- 不把 runtime-native memory 自动导入成 AVM 对象。
- AVM 只提供全局/用户级 runtime memory isolation：同一个 Agent/runtime
  使用稳定 Agent ID 对应的私有 runtime boundary。
- OpenCode 这类需要进程级变量的 runtime 当前通过 `avm run opencode` 注入
  `OPENCODE_DB` 和 `XDG_*`；目标语义应收敛为 `avm run <agent> --runtime opencode`，不长期污染用户 shell。

未来再定义：

- AVM 是否需要 memory 抽象。
- runtime-native memory 是否只做只读诊断。
- 更高级的共享/审计/遗忘语义。
- 审计、遗忘、冲突和用户确认机制。

## 5. 模块关系

```text
Package
  installs
    -> Agent
    -> referenced capabilities

Agent
  owns
    -> instructions
    -> skills
    -> MCP servers
    -> permissions
    -> model/runtime preferences

Scenario (future)
  references
    -> Agent per runtime

Run / Use
  selects
    -> Agent
  resolves
    -> Runtime
  applies to
    -> Runtime managed homes

Runtime
  receives
    -> rendered config from selected Agent
  reports
    -> mapping status and warnings
```

核心关系：

- Agent 是能力和行为的归属。
- Runtime 是 Agent 的执行载体和生效目标，不负责从多个 Agent 中反向选择，也不是主配置对象。
- Scenario/Environment 是未来的 Agent 场景映射，不是当前核心对象。
- Package 是 Agent 的分发载体，未来可扩展到 Scenario。
- Skills 是 Agent 的组成部分，不是主模块。
- Sync 是 run/use 的实现细节，不是主模块。

## 6. 关键用户流程

### 6.1 新用户首次创建 Agent

```bash
curl -fsSL https://raw.githubusercontent.com/xz1220/Agent-VM/main/scripts/install.sh | sh
avm agent create
avm run backend-coder
```

期望体验：

1. 安装后默认初始化。
2. `agent create` 打开交互式 UI。
3. 用户选择来源、runtime、skills、MCP、权限。
4. AVM 展示 preview。
5. 创建成功后给出下一步：`avm run <agent>`，以及可选的 `avm use <agent>`。

### 6.2 从已有 Agent 创建新场景

```bash
avm agent clone default --name api-coder
avm agent edit api-coder
avm run api-coder
```

期望体验：

- 用户不需要手写 YAML。
- clone 后可以交互式修改 skills/MCP/runtime。
- 修改前后可以看到 diff。

### 6.3 同一个 Agent 使用不同 Runtime

```bash
avm run backend-coder --runtime codex
avm run backend-coder --runtime claude-code
```

期望体验：

- 用户启动的是同一个 Agent，只是选择不同 runtime 作为执行载体。
- AVM 根据 runtime adapter 渲染对应 managed config。
- 如果某个 Agent 字段在目标 runtime 中无法原生表达，preview/status 必须说明 native、rendered_as_instructions 或 unsupported。

### 6.4 未来：创建多 runtime 多角色 Scenario

```bash
avm scenario create work
avm use --kind scenario work
```

期望体验：

- 用户为每个 runtime 选择 Agent。
- `status` 能说明当前 Scenario 下每个 runtime 使用哪个 Agent。
- Scenario 不复制 Agent 配置。
- 该能力不进入当前 P0。

### 6.5 分享一个 Agent

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
- 把 Environment/Scenario 做成当前核心能力。
- 让用户输入 runtime 后由 AVM 从多个 Agent 中反向猜测启动对象。
- 从 runtime-native subagents 或 agent markdown 自动创建 AVM Agent。
- 把 sync 作为用户必须理解的日常操作。
- 在 memory 原则明确前新增 memory 主线。
- 假装所有 runtime 都有一致的 Agent Profile 能力。
- 默认覆盖用户 runtime 原生配置文件。

## 9. 当前实现差距

| 模块 | 当前已有 | 需要补齐 |
| --- | --- | --- |
| 安装/初始化 | installer、`init`、`shell init` | doctor、uninstall、shell install/uninstall |
| Agent | `create`、`agent create/list/show/edit/delete/rename/clone` | 更完整的交互式 create、批量引用迁移体验 |
| Environment | 部分 `env` 实验实现 | 降级为 default/未来 Scenario，不进入 P0 主线 |
| Run/Use | `use`、`activate`、`status`、`deactivate`、`sync`、runtime-first `run` | 收敛到 `run <agent> [--runtime]` 和 `use <agent>` |
| Package | package inspect/install/export | package list/show/uninstall、冲突策略、命令归属统一 |
| Skills | skill list、create 时选择 | 放入 agent edit/create 主流程 |

## 10. 成功标准

P0 成功标准：

- 新用户能在安装后 3 分钟内创建并使用第一个 Agent。
- 用户能完整 CRUD Agent，而不需要手写 YAML。
- 删除或覆盖 Agent 前有明确确认和引用检查。
- 用户启动的是明确 Agent；多个 Agent 支持同一 runtime 时不会产生隐式选择。
- `avm run <agent>` 能根据 Agent runtime preference 启动或要求用户明确选择 runtime。
- README 主路径不再要求用户理解 skill registry 或 sync。

P1 成功标准：

- Package 能可靠安装/导出 Agent，并处理冲突。
- Scenario/Environment 是否进入产品主线经过重新确认。

P2 成功标准：

- Scenario/Environment 如需支持，完整定义命名、CRUD、引用检查和 Agent-first 关系。
- Memory 原则被重新定义，并且不会破坏 Agent / Scenario 的清晰心智。
