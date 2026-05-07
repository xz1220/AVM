# Agent VM — 产品需求文档（PRD）

> 最后更新：2026-05-07（v23 — run 路径新增 Agent 定义与 managed config 的对齐原则）

## 1. 产品定位

Agent VM（`avm`）是 AI Coding Agent 的本地配置管理器。它让用户管理 **Agent**，再把选中的 Agent 应用到 Codex、Claude Code、OpenCode、Cline、Cursor 等 runtime。

AVM 不应该让用户先理解 runtime 配置文件、skills registry 或 sync。用户真正关心的是：

1. 我有哪些 Agent？
2. 每个 Agent 会加载哪些 instructions、skills、MCP？
3. 我当前命令将运行哪个 Agent？
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

Environment 只作为 AVM 内部的 default 上下文存在，不作为用户需要理解或管理的对象。

## 2. 用户需求与产品要求

本章只描述用户需求和产品要求，不定义 AVM 有哪些核心对象，也不列出具体命令或操作。
核心对象在第 3 章定义；用户操作和命令在第 4 章定义。

### 2.1 用户需要清楚知道自己有哪些可复用工作配置

用户希望 AVM 回答的是：

- 我已经沉淀了哪些可复用的 AI coding 工作配置？
- 每个配置代表什么工作角色、适合什么任务？
- 每个配置会带来哪些指令、能力、外部工具和运行方式？
- 本次运行使用的是哪一个配置？

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

### 2.5 用户需要运行过程透明但不需要理解内部同步

用户希望运行一个配置后，当前命令和底层工具都进入一致状态。用户不应该为了日常
使用而理解同步、渲染计划、受控路径等内部机制。

产品要求：

- 运行 Agent 时，产品必须负责让本次命令和底层工具配置一致。
- 产品必须能解释本次运行到底使用了什么、哪些底层工具已经就绪、哪些没有就绪。
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

Agent 是用户直接创建、修改、删除、复制、启动和分享的主对象。

Agent 包含：

| 字段 | 用户含义 |
| --- | --- |
| identity | 名称、描述、角色 |
| instructions | system/developer 指令、引用文件 |
| skills | 这个 Agent 应具备的任务能力 |
| MCP servers | 这个 Agent 可连接的外部工具 |
| runtime config | 这个 Agent 可作用到哪些 runtime，以及每个 runtime 需要的配置映射 |

Agent 回答：

> 当我 `avm run backend-coder` 时，我到底启动了一个什么样的 Agent？

Agent 不回答：

> 当前 Codex 应该从多个 Agent 里自动选择哪一个？

这个选择必须由用户通过 Agent name 完成。

### 3.2 Environment

Environment 不是当前核心对象，也不是用户需要创建、切换或维护的对象。

当前产品只保留一个 default Environment，用来表达 AVM 的内部默认上下文。它不出现在用户日常路径里，也不要求用户理解。

Environment 不回答：

> 我应该创建哪个工作环境？

Environment 只回答内部问题：

> 如果系统需要一个上下文边界，默认上下文是什么？

用户始终通过 Agent name 运行 Agent，例如 `avm run <agent>`。Environment 不负责 runtime 到 Agent 的映射，不负责 Agent 分组，也不进入 Package 分发边界。

### 3.3 Package

Package 是分发单元，不是日常运行对象。

Package 可以包含：

- Agent
- skills
- MCP definitions
- hooks / commands / toolsets
- metadata 和版本信息

用户安装 Package 后，得到的是可管理的 Agent。Package 不安装、不导出、不携带 Environment。
日常使用仍然是：

```bash
avm run <agent>
```

### 3.4 Runtime

Runtime 是 Agent 生效的目标工具，例如 Codex、Claude Code、OpenCode、Cline、Cursor。

Runtime 不应成为用户主要管理对象。它在用户界面中只应该出现在：

- Agent 创建/编辑时选择首选 runtime
- `avm run <agent>` 无法唯一确定 runtime 时让用户选择
- run preview 或 run 输出中显示实际映射和 warnings
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

### 4.2 Agent 配置 CRUD

这是当前阶段的核心配置模块。

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
existing package（用户自己创建的或已经安装的）
```

创建流程应包含：

1. 选择来源。
2. 设置 Agent 名称和描述。
3. 设置 instructions。
4. 从全量可发现能力中选择 skills 和 MCP servers。
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

Skills 和 MCP servers 的选择列表必须来自实时全量发现，而不是只读取 AVM 自己管理的 registry。

全量发现包括：

- AVM 管理或 Package 安装的 skills/MCP。
- 用户已经安装在 runtime 全局目录里的 skills/MCP，即使这些能力不是 AVM 管理的。
- 用户在 AVM 外部新增或删除全局 skills/MCP 后，下一次 `create` 或 `edit` 必须看到更新后的列表。

例如用户机器上原本有 10 个 runtime 全局 skills，之后通过 Codex、Claude Code 或其他方式又安装了 1 个，
那么下一次创建或编辑 Agent 时，候选列表应显示 11 个。

能力来源要求：

- 每个候选项必须能解释来源，例如 AVM 管理、Package 安装、或 runtime 全局目录发现。
- AVM 不能在用户不知情的情况下移动、覆盖、删除或接管 runtime 全局能力。
- 当不同来源出现同名 skills/MCP 时，产品必须让用户能区分来源，不能静默合并或随机选择。
- Agent 对 runtime 全局能力的引用、复制、导入、导出和同步策略，需要在调研各 runtime 的实际能力模型后再确定。
- 在策略确定前，PRD 只要求 create/edit 能看到这些能力，并能解释来源和潜在风险。

删除流程应支持：

- 删除前展示将删除的 Agent 名称、来源、runtime 配置摘要，以及它引用的 skills/MCP。
- 删除只删除 Agent 配置本身，不删除被引用的 skills/MCP 能力本体。
- 删除后用户不能再通过该 Agent name 发起新的 `avm run`。
- 已经启动的 runtime 进程不由 Agent delete 管理；Agent delete 不引入长期运行状态心智。

验收标准：

- `create` 不是隐式 overwrite。
- 已存在同名 Agent 时必须明确提示 rename、overwrite 或 cancel。
- `show` 能展示 source path 和 runtime mapping 状态。
- `create` 和 `edit` 的 skills/MCP 候选列表必须反映当前机器上的全量发现结果。
- `edit` 和 `delete` 都有 non-interactive flags 或可脚本化模式。

### 4.3 Default Environment

当前阶段不把 Environment 做成用户模块。AVM 只保留一个 default Environment 作为内部上下文。

当前规则：

- 用户不需要理解或管理 Environment。
- README 和日常路径不暴露 Environment 命令。
- 不提供 Environment CRUD。
- 不提供 Environment 切换。
- 不为 Environment 设计演进路线或未来扩展承诺。
- Agent 删除、重命名等核心流程不应因为 Environment 而复杂化。
- Package 导入导出不处理 Environment。

验收标准：

- default Environment 不重复定义 skills/MCP/instructions/runtime config。
- default Environment 不引用 Agent。
- Environment 不负责 runtime 到 Agent 的映射。
- 用户无法通过命令创建、切换、删除或导出 Environment。

### 4.4 运行 Agent

这是用户每天使用 AVM 的入口。

目标能力：

```bash
avm run <agent>
avm run <agent> --runtime <runtime>
```

行为要求：

- `avm run <agent>` 是最清晰的日常启动入口。
- `avm run <agent> --runtime <runtime>` 显式选择执行载体。
- 当一个 Agent 支持多个 runtime 且没有唯一选择时，交互模式询问，非交互模式报错。
- 多个 Agent 可以支持同一个 runtime，但 runtime 启动时不能反向猜测 Agent。
- `run` 是命令级行为，不产生需要用户管理的长期状态。
- 产品主线不提供先切换再运行的长期状态模式。
- 每次运行都必须能解释所选 Agent、runtime、managed paths、memory isolation boundary 和 warnings。
- adapter mapping 必须在 preview 或 run 输出中清楚展示 unsupported / rendered-as-instructions。
- AVM Agent 定义和 runtime 实际 managed config 是最终一致的，不是强一致。runtime 自身或用户可能在 AVM 之外修改 managed config（例如 runtime 自动写入依赖、用户直接编辑配置文件）。AVM 必须在 `run` 的启动和退出路径上核对两者差异，并让用户对每一项差异做决定（合并进 Agent 定义 / 丢弃 / 本次保留）。非交互模式下采取默认保留并写入 run log。具体对齐机制的实现方式后续再定。

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
- Package 不应成为可运行对象。
- Package 不包含 Environment，也不通过 Package 分发 Environment。
- Package 安装前必须展示将写入哪些对象。
- 冲突时必须提供 rename、skip、overwrite、cancel。
- export 时能选择是否包含被引用的 skills/MCP。

### 4.6 Memory

Memory 已从当前 AVM 产品模型中删除。

当前原则：

- 不提供 `avm memory` 命令。
- 不在 Agent / Environment / Package schema 中声明 memory。
- 不把 runtime-native memory 自动导入成 AVM 对象。
- AVM 只提供全局/用户级 runtime memory isolation：同一个 Agent/runtime
  使用稳定 Agent ID 对应的私有 runtime boundary。
- 需要进程级隔离变量的 runtime，其隔离边界应由 `avm run <agent> --runtime <runtime>`
  注入，不长期污染用户 shell。

未来再定义：

- AVM 是否需要 memory 抽象。
- runtime-native memory 是否只做只读诊断。
- 更高级的共享/审计/遗忘语义。
- 审计、遗忘、冲突和用户确认机制。

## 5. 关键用户流程

### 5.1 新用户首次创建 Agent

```bash
curl -fsSL https://raw.githubusercontent.com/xz1220/Agent-VM/main/scripts/install.sh | sh
avm agent create
avm run backend-coder
```

期望体验：

1. 安装后默认初始化。
2. `agent create` 打开交互式 UI。
3. 用户选择 blank/default 或 existing package 作为来源，并从实时全量能力列表选择 skills/MCP。
4. AVM 展示 preview。
5. 创建成功后给出下一步：`avm run <agent>`。

### 5.2 复制已有 Agent

```bash
avm agent clone default --name api-coder
avm agent edit api-coder
avm run api-coder
```

期望体验：

- 这是显式 `clone` 操作，不属于 `create` 的来源选择。
- 用户不需要手写 YAML。
- clone 后可以交互式修改 skills/MCP/runtime。
- 修改前后可以看到 diff。

### 5.3 同一个 Agent 使用不同 Runtime

```bash
avm run backend-coder --runtime codex
avm run backend-coder --runtime claude-code
```

期望体验：

- 用户启动的是同一个 Agent，只是选择不同 runtime 作为执行载体。
- AVM 根据 runtime adapter 渲染对应 managed config。
- 如果某个 Agent 字段在目标 runtime 中无法原生表达，preview 或 run 输出必须说明 native、rendered_as_instructions 或 unsupported。

### 5.4 分享一个 Agent

```bash
avm package export backend-coder
avm package inspect backend-coder.avm.zip
```

期望体验：

- export 前展示包含哪些对象。
- install 前展示将写入哪些对象。
- 冲突处理清楚。

## 6. Runtime 映射策略

AVM 不承诺所有 runtime 支持相同字段。每个 adapter 必须报告字段映射状态：

| 状态 | 含义 |
| --- | --- |
| native | runtime 有原生字段承接 |
| rendered_as_instructions | runtime 没有结构化字段，只能渲染成说明文本 |
| ignored | AVM 有意不写入，通常为了保护用户文件 |
| unsupported | runtime 当前无法表达 |
