# Agent VM — 产品需求文档（PRD）

> 最后更新：2026-05-06（v16 — Package 边界收敛）

## 1. 产品定位

Agent VM（`avm`）是 AI Coding Agent 的本地配置管理器。它让用户管理 **Agent**，再把选中的 Agent 应用到 Codex、Claude Code、OpenCode、Cline、Cursor 等 runtime。

AVM 不应该让用户先理解 runtime 配置文件、skills registry 或 sync。用户真正关心的是：

1. 我有哪些 Agent？
2. 每个 Agent 会加载哪些 instructions、skills、MCP？
3. 我当前 shell 或当前命令正在使用哪个 Agent？
4. 这个 Agent 应该通过哪个 runtime 启动？
5. 我能不能把一个好用的配置复制、修改、删除、分享给别人？

因此产品主线必须收敛为：

```text
安装 AVM
  -> 创建/管理 Agent
    -> run Agent
      -> AVM 解析或确认 runtime
        -> 启动 runtime 或写入 runtime managed config
```

Environment 可以作为未来的组合能力存在，但当前产品主线只保留一个 default Environment，
不把 Environment 作为核心用户对象推广。

## 2. 用户需求与产品要求

本章只描述用户需求和产品要求，不定义 AVM 有哪些核心对象，也不列出具体命令或操作。
核心对象在第 3 章定义；用户操作和命令在第 4 章定义。

### 2.1 用户需要清楚知道自己有哪些可复用工作配置

用户希望 AVM 回答的是：

- 我已经沉淀了哪些可复用的 AI coding 工作配置？
- 每个配置代表什么工作角色、适合什么任务？
- 每个配置会带来哪些指令、能力、外部工具和运行方式？
- 当前生效的是哪一个配置？

产品要求：

- 用户不需要先理解底层工具的配置文件结构。
- 用户不需要手写配置文件才能完成最常见的配置维护。
- 产品必须能用用户能理解的语言解释一个配置包含什么、适合做什么、会影响什么。
- 当配置可能影响当前工作状态时，产品必须展示影响范围和风险。

### 2.2 用户需要明确选择当前工作角色

用户会把不同配置理解为不同工作角色，例如编码、调研、评审、修复。底层工具只是这些
工作角色的执行载体。

当多个工作角色都能通过同一个底层工具运行时，用户只选择底层工具是不够的；产品不能
反向猜测用户想使用哪一个工作角色。

产品要求：

- 日常启动路径必须围绕用户想使用的工作角色展开。
- 底层工具选择应由产品根据配置自动解析，或在无法唯一判断时让用户确认。
- 产品不能在多个候选工作角色之间静默选择。
- 非交互场景下，无法唯一判断时必须给出明确错误和下一步。

### 2.3 用户需要在配置过程中理解能力边界

用户在配置工作角色时，关心的是它能做什么、能连接什么、适合通过什么底层工具运行，
而不是底层能力仓库或同步机制如何实现。

产品要求：

- 能力选择必须出现在用户配置工作角色的流程里，而不是拆成用户必须单独理解的模块。
- 产品必须解释所选能力会怎样影响运行结果。
- 当某个底层工具不能完整承接某项能力时，产品必须明确说明是原生支持、降级为说明文本、被有意忽略，还是暂不支持。

### 2.4 用户需要可信的复用和迁移来源

用户希望复用已有配置或安装别人分享的配置，但不希望产品把语义不等价的文件自动伪装
成完整配置。

产品要求：

- 可复用来源必须可解释、可预览、可追踪。
- 安装或导入前必须说明会写入什么、可能冲突什么。
- 产品不应把底层工具的原生文件扫描结果伪装成语义完整的 AVM 配置。
- 复用后的结果必须仍然是用户可以继续维护的 AVM 配置。

### 2.5 用户需要生效过程透明但不需要理解内部同步

用户希望选择一个配置后，当前工作状态和底层工具都进入一致状态。用户不应该为了日常
使用而理解同步、渲染计划、受控路径等内部机制。

产品要求：

- 选择配置后，产品必须负责让当前工作状态和底层工具配置一致。
- 产品必须能解释当前到底生效了什么、哪些底层工具已经就绪、哪些没有就绪。
- 异常、冲突和降级必须对用户可见，并给出可执行的下一步。
- 内部修复或调试能力可以存在，但不应成为普通用户的日常路径。

### 2.6 用户暂时不直接管理运行记忆

用户会关心不同工作角色之间的运行状态不要串，但当前阶段不让用户直接管理底层工具的
记忆内容。

产品要求：

- 产品可以隔离不同工作角色的运行状态边界。
- 产品不提供记忆内容的导入、导出、重置、同步或编辑。
- 产品不把底层工具的记忆内容提升为 AVM 的核心配置。
- 是否需要更高级的记忆抽象，必须在用户语义明确后再定义。

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
| runtime config | 这个 Agent 可作用到哪些 runtime，以及每个 runtime 需要的配置映射 |

Agent 回答：

> 当我 `avm run backend-coder` 或 `avm use backend-coder` 时，我到底启动或激活了一个什么样的 Agent？

Agent 不回答：

> 当前 Codex 应该从多个 Agent 里自动选择哪一个？

这个选择必须由用户通过 Agent name 完成。

### 3.2 Environment（未来扩展）

Environment 不是当前核心对象。当前阶段只需要一个 default Environment；已有 env 相关实现可以作为实验能力或兼容能力保留，但不进入 README 主线、P0 验收或日常用户路径。

未来如果扩展 Environment，它表示一个命名工作上下文，管理这个上下文下有哪些可用的 Agent 配置。
Agent 自己已经包含 runtime config，因此 Environment 不再做 runtime 到 Agent 的映射。

例如：

```yaml
name: work
agents:
  - backend-coder
  - reviewer
  - research
```

Environment 回答：

> 在这个工作上下文下，我有哪些 Agent 配置可以使用？

只有当用户需要按工作上下文组织一组 Agent 时，才需要 Environment。单 Agent 场景必须直接 `avm run <agent>` 或 `avm use <agent>`。

### 3.3 Package

Package 是分发单元，不是日常激活对象。

Package 可以包含：

- Agent
- skills
- MCP definitions
- hooks / commands / toolsets
- metadata 和版本信息

用户安装 Package 后，得到的是可管理的 Agent。Package 不安装、不导出、不携带 Environment；
即使未来支持 Environment，Environment 也不进入 Package 的分发边界。日常使用仍然是：

```bash
avm run <agent>
```

### 3.4 Runtime

Runtime 是 Agent 生效的目标工具，例如 Codex、Claude Code、OpenCode、Cline、Cursor。

Runtime 不应成为用户主要管理对象。它在用户界面中只应该出现在：

- Agent 创建/编辑时选择首选 runtime
- `avm run <agent>` 无法唯一确定 runtime 时让用户选择
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
3. 设置 instructions。
4. 选择 skills 和 MCP servers。
5. 设置 runtime 配置。
6. 展示 preview。
7. 确认写入。

编辑流程应支持：

- 修改基础信息
- 修改 instructions
- 修改 skills
- 修改 MCP
- 修改 runtime 配置
- 预览 runtime mapping 影响

删除流程应支持：

- 删除前展示引用关系，例如 active Agent、Package 元数据或未来 Environment 正在使用这个 Agent。
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

### 4.3 Default Environment 与未来扩展

当前阶段不把 Environment 做成核心用户模块。AVM 只需要一个 default Environment 来表示当前
Agent-first 的激活上下文。

当前规则：

- 用户不需要理解或管理 Environment。
- README 和日常路径不推广 `avm env`。
- 已有 `avm env` 命令可以暂时保留，但视为实验/兼容入口。
- 默认不为 Environment 补齐完整 CRUD。
- Agent 删除、重命名等 P0 流程不应因为 Environment 扩展而复杂化。
- Package 导入导出不处理 Environment。

未来如果确实需要按工作上下文组织一组 Agent，可以继续扩展 Environment。届时再引入完整能力：

```bash
avm env create
avm env list
avm env show <name>
avm env edit <name>
avm env delete <name>
avm env clone <name> --name <new-name>
avm env rename <old-name> <new-name>
avm env switch <name>
```

Environment 创建流程：

1. 设置 Environment 名称，例如 `work`、`review`、`frontend`。
2. 选择这个 Environment 下有哪些 Agent。
3. 展示 Agent 列表 preview。
4. 确认写入。

验收标准：

- Environment 不重复定义 skills/MCP/instructions/runtime config。
- Environment 只引用 Agent。
- Environment 不负责 runtime 到 Agent 的映射。
- 删除 Agent 时能发现 Environment 引用。
- 切换 Environment 应使用独立命令，例如 `avm env switch <name>`，不复用 `avm use <agent>` 的 Agent 激活语义。

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
- Package 不包含 Environment；未来 Environment 配置也不通过 Package 分发。
- Package 安装前必须展示将写入哪些对象。
- 冲突时必须提供 rename、skip、overwrite、cancel。
- export 时能选择是否包含被引用的 skills/MCP。

当前 preview：

- 已有 package inspect/install/export。
- export/import/install 的用户心智还需要统一到 package 模块。

### 4.6 Memory

Memory 已从当前 AVM 产品模型中删除。

当前原则：

- 不提供 `avm memory` 命令。
- 不在 Agent / Environment / Package schema 中声明 memory。
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
    -> runtime config

Environment (future)
  references
    -> Agent set

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
- Environment 是未来用于组织一组 Agent 的上下文，不是当前核心对象。
- Package 是 Agent 的分发载体，不关注 Environment。
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
3. 用户选择来源、runtime 配置、skills 和 MCP。
4. AVM 展示 preview。
5. 创建成功后给出下一步：`avm run <agent>`，以及可选的 `avm use <agent>`。

### 6.2 从已有 Agent 创建新 Agent

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

### 6.4 未来：创建 Environment

```bash
avm env create work
avm env switch work
avm run backend-coder
```

期望体验：

- 用户选择这个 Environment 下有哪些 Agent。
- `status` 能说明当前 Environment 下有哪些可用 Agent。
- 用户仍然通过 Agent name 启动具体 Agent。
- Environment 不复制 Agent 配置，也不决定 runtime 映射。
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

## 8. 非目标

当前阶段不做：

- 把 skills 做成用户主模块。
- 把 Environment 做成当前核心能力。
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
| Environment | 部分 `env` 实验实现 | 降级为 default/未来扩展，不进入 P0 主线 |
| Run/Use | `use`、`activate`、`status`、`deactivate`、`sync`、runtime-first `run` | 收敛到 `run <agent> [--runtime]` 和 `use <agent>` |
| Package | package inspect/install/export | package list/show/uninstall、冲突策略、命令归属统一 |
| Skills | skill list、create 时选择 | 放入 agent edit/create 主流程 |

## 10. 成功标准

P0 成功标准：

- 新用户能在安装后 3 分钟内创建并使用第一个 Agent。
- 用户能完整 CRUD Agent，而不需要手写 YAML。
- 删除或覆盖 Agent 前有明确确认和引用检查。
- 用户启动的是明确 Agent；多个 Agent 支持同一 runtime 时不会产生隐式选择。
- `avm run <agent>` 能根据 Agent runtime config 启动或要求用户明确选择 runtime。
- README 主路径不再要求用户理解 skill registry 或 sync。

P1 成功标准：

- Package 能可靠安装/导出 Agent，并处理冲突。
- Environment 是否进入产品主线经过重新确认。

P2 成功标准：

- Environment 如需支持，完整定义命名、CRUD、引用检查和 Agent-first 关系。
- Memory 原则被重新定义，并且不会破坏 Agent / Environment 的清晰心智。
