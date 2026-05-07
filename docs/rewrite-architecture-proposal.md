# AVM 重写架构方案草案

> 状态：草案
>
> 范围：本文只描述重写后的总体方向和模块划分，不定义具体数据结构、接口或函数。

## 1. 总体方案概况

AVM 重写后的主线应当从用户理解的 Agent 出发，而不是从底层 runtime、同步状态或 shell activation 出发。用户日常面对的核心问题是：有哪些 Agent、每个 Agent 包含哪些指令和能力、这次要运行哪个 Agent、它将通过哪个 runtime 执行。runtime 是承载 Agent 的执行载体，不能反过来成为用户选择工作角色的入口。

因此，新版本 AVM 的产品和技术中心应收敛为一条主流程：

1. 用户创建或编辑 Agent。
2. AVM 从全量可发现来源中展示 skills、MCP 等能力，并保留每个能力的来源。
3. 用户运行某个 Agent。
4. AVM 根据 Agent 配置解析 runtime；如果无法唯一决定，则让用户确认或在非交互模式下报错。
5. AVM 为 Agent 与 runtime 组合建立独立运行边界。
6. AVM 将 Agent 定义渲染为 runtime 可理解的 managed config。
7. AVM 启动 runtime，并在启动前后解释本次运行使用了什么、写入了什么、哪些字段被原生承接、哪些字段被降级或不支持。

这个方向意味着旧版本里的长期 active 状态、Environment 切换、shell activation 和手动 sync 不再是产品主路径。它们最多只能作为开发期诊断能力或迁移参考存在，不能继续决定核心模型。

新架构的几个基本原则如下。

Agent 是唯一用户主对象。Agent 包含身份描述、指令、skills、MCP、权限偏好、模型偏好和 runtime 偏好。Agent 可以被创建、复制、编辑、删除、重命名、运行和打包分享。

Runtime 是执行载体。Runtime 不负责选择 Agent，也不应要求用户理解底层配置文件。Runtime 相关能力通过 adapter、boundary 和 runtime facts 暴露给 AVM。

Environment 不进入用户主线。当前阶段只保留内部 default 上下文，不提供用户可见的 Environment CRUD、切换、导入导出，也不让 Package 携带 Environment。

Package 是分发单元。Package 安装后产生可管理的 Agent。Package 可以选择携带 Agent 引用的 skills/MCP，但不应该把 runtime 原生文件扫描结果伪装成完整 AVM Agent。

Memory 不成为 AVM 对象。AVM 不提供 memory CRUD，不导入、导出、编辑或同步 runtime-native memory。AVM 只负责同一个 Agent/runtime 组合的运行边界隔离，并向用户解释边界和风险。

Runtime managed config 与 AVM Agent 定义是最终一致关系。runtime 自身或用户可能在 AVM 外部修改 managed config。AVM 应在 run 的启动和退出路径上发现差异，并把差异解释成用户能决策的选项，例如合并回 Agent、丢弃、或本次保留。

能力发现必须是实时和多来源的。Agent create/edit 不能只看 AVM 自己的 registry，也要能看到 runtime 全局目录、Package 安装来源、用户外部安装来源。来源相同或名称相同不代表语义相同，AVM 必须保留来源信息并把冲突展示出来。

runtime research 带来的核心约束也应进入总体设计：

- Codex 的自然隔离边界是独立的 `CODEX_HOME`，其中混合了 config、auth、history、sessions、state、skills、plugins 和 memory artifacts。AVM 需要把每个 Agent/runtime 组合放入独立边界，并处理 auth sidecar 的可用性。
- Claude Code 的状态不是单一目录，但 `CLAUDE_CONFIG_DIR` 是用户级设置、global config 和 user memory 的主要边界。插件缓存、临时目录、project trust 和 MCP scope 需要被单独识别为 warnings 或后续扩展点。
- OpenClaw 默认安全边界较弱，skills、plugins、MCP 和 exec 都可能触达 host 能力。AVM 对 OpenClaw 的支持不能只渲染配置，还必须明确运行模式、workspace、state dir、approval、sandbox 和进程隔离策略。

整体上，新版本不应复刻旧代码的 activation pipeline，而应把旧代码中已经验证过的局部能力作为参考资产：稳定 Agent ID、runtime boundary 思路、adapter mapping status、managed path 写入、YAML 校验、package zip 安全检查等都可以复用或改写；旧的 ActiveRef、Environment 映射和 run runtime 入口不应成为新核心。

## 2. 可能涉及的模块

下面的模块是逻辑模块，不一定要求一一对应到最终的目录或包名。划分目标是让产品语义、runtime 差异和持久化边界各自清晰，避免把特殊情况散落在 CLI、adapter 和存储层之间。

### Agent 模块

Agent 模块负责 AVM 的主对象语义。

它提供的能力包括：定义 Agent 字段、校验 Agent 配置、创建 Agent、复制 Agent、编辑 Agent、删除 Agent、重命名 Agent、维护稳定 Agent identity、解释 Agent 摘要，以及判断一个 Agent 支持哪些 runtime。

它不负责 runtime 文件如何写入，也不负责扫描 runtime 全局能力。Agent 模块只表达用户想要的工作角色和能力选择。

### Store 模块

Store 模块负责 AVM 自己的数据边界。

它提供的能力包括：读写 AVM home、读写 Agent、记录 Package 安装元数据、记录运行日志、保存 runtime materialize 结果、保存 drift 检查结果，以及提供原子写入和路径安全约束。

它不负责产品决策。Store 只保证数据被可靠读写，不决定用户应该选哪个 runtime、是否覆盖某个差异、或某个字段如何映射。

### Capability Discovery 模块

Capability Discovery 模块负责发现和归一化 skills、MCP 以及后续可能出现的 commands、hooks、toolsets。

它提供的能力包括：扫描 AVM 管理的能力、扫描 Package 安装的能力、扫描 runtime 全局能力、调用或模拟 runtime 原生发现规则、标注能力来源、识别同名冲突、展示候选列表、为 Agent create/edit 提供实时能力视图。

它不负责把能力写入 runtime config。它只回答“现在机器上有哪些能力可以被选择，它们来自哪里，风险是什么”。

### Runtime Facts 模块

Runtime Facts 模块负责描述每个 runtime 的稳定事实。

它提供的能力包括：runtime 名称规范化、binary detection、版本探测、支持能力清单、配置层级说明、native mapping 能力边界、已知风险、需要的启动方式、是否支持进程级隔离、是否需要 trust 或 auth sidecar。

它不负责生成具体文件内容。它为 planner、adapter、doctor 和 CLI 输出提供事实依据。

### Boundary 模块

Boundary 模块负责 Agent/runtime 组合的运行隔离边界。

它提供的能力包括：为每个 Agent/runtime 生成稳定私有目录、区分长期 shell env 和单次 run env、记录配置路径、数据路径、cache 路径、state 路径、workspace 路径、隔离状态和 warnings。

它不负责解释 Agent 字段，也不负责启动 runtime。Boundary 模块只回答“这个 Agent 通过这个 runtime 运行时，runtime 的用户级状态会落在哪里，哪些环境变量需要注入”。

### Planning 模块

Planning 模块负责把用户意图组合成一次可预览、可执行的运行计划。

它提供的能力包括：读取 Agent、选择或确认 runtime、获取能力发现结果、解析 boundary、请求 adapter 生成 mapping 和 managed path 计划、汇总 warnings，并产出给 preview、run 和 doctor 使用的统一说明。

它不直接写文件，也不直接启动进程。Planning 是核心编排层，负责让 Agent、capability、runtime facts、boundary 和 adapter 在同一条逻辑链上对齐。

### Adapter 模块

Adapter 模块负责 runtime-specific 的表达转换。

它提供的能力包括：把 AVM Agent 字段映射到 Codex、Claude Code、OpenClaw 等 runtime 的配置形态；报告 native、rendered as instructions、ignored、unsupported 等 mapping 状态；声明 managed paths；生成 runtime managed config 的目标内容；暴露 runtime-specific warnings。

它不负责 Agent CRUD，不负责全局能力发现的产品合并规则，也不负责启动 runtime。Adapter 只回答“给定一个已经解析好的 Agent 和边界，这个 runtime 能如何表达它”。

### Materialize 模块

Materialize 模块负责把 adapter 的计划落实到文件系统。

它提供的能力包括：创建必要目录、写入 managed files、合并 AVM 管理的结构化片段、检测非 AVM 管理内容冲突、创建备份、删除过期的 AVM-managed 文件、保存 materialize 结果。

它不负责决定是否运行某个 Agent，也不负责用户交互。遇到冲突时，它应该返回结构化结果，由上层决定交互或失败策略。

### Run 模块

Run 模块负责 `avm run <agent>` 的命令级生命周期。

它提供的能力包括：解析用户指定的 Agent、解析或确认 runtime、触发 planning、触发 materialize、准备 runtime process env、启动 runtime、传递用户参数、记录 run log、在退出路径触发 drift 检查。

它不产生用户需要长期管理的 active 状态。一次 run 是一次命令级行为，结束后只留下日志、materialize 记录和可解释的 runtime managed config。

### Reconcile 模块

Reconcile 模块负责 AVM Agent 定义与 runtime managed config 的差异对齐。

它提供的能力包括：在 run 启动前读取当前 managed config、在 run 退出后再次读取 managed config、识别 runtime 或用户在 AVM 外部产生的变化、把变化归类为可合并、应丢弃、本次保留或需要人工处理，并为交互模式和非交互模式提供决策输入。

它不负责扫描全部 runtime 全局能力，也不负责替代 adapter。它关注的是“AVM 认为自己管理的那部分配置是否发生了漂移”。

### Package 模块

Package 模块负责 Agent 和能力的复用分发。

它提供的能力包括：inspect、install、export、uninstall、冲突预览、写入预览、checksum 校验、Agent ID 冲突处理、可选携带被引用 skills/MCP。

它不携带 Environment，也不把 Package 当成可运行对象。Package 安装后的结果应该是用户可以继续编辑和运行的 Agent。

### Diagnostics 模块

Diagnostics 模块负责解释系统状态，而不是改变系统状态。

它提供的能力包括：检查 AVM home、检查 runtime binary、检查 Agent/runtime boundary、检查 managed paths、展示 mapping status、展示 capability 来源冲突、展示 auth sidecar 状态、展示 runtime-specific warnings。

它不承担日常 run 的职责。Doctor、status、preview 可以复用它，但它不应该成为用户必须先执行的步骤。

### CLI 模块

CLI 模块负责用户交互和文本输出。

它提供的能力包括：命令参数解析、交互式选择、non-interactive 错误、preview 展示、diff 展示、确认流程、输出格式控制。

它不应承载业务逻辑。CLI 应该调用 Agent、Capability Discovery、Planning、Run、Package、Diagnostics 等模块，而不是自己拼 runtime 路径、自己解释 mapping 或自己修改 managed config。

### 模块之间的逻辑组织

这些模块可以按三条主要路径组织。

创建和编辑路径从 CLI 进入 Agent 模块。CLI 请求 Capability Discovery 提供实时候选能力，再让 Agent 模块保存用户确认后的 Agent 定义。这个路径不写 runtime config，也不启动 runtime。

运行路径从 CLI 进入 Run 模块。Run 模块读取 Agent，交给 Planning 模块解析 runtime、capability、boundary 和 adapter mapping。Planning 生成可解释的运行计划后，Materialize 模块负责写入 managed config。写入完成后，Run 模块注入 boundary run env 并启动 runtime。进程退出后，Run 模块触发 Reconcile 检查差异并记录 run log。

打包路径从 CLI 进入 Package 模块。Package 模块读取 Agent 和用户选择携带的能力，生成可检查、可安装的分发文件。安装 Package 时只写入 Agent 和可选能力元数据，不改变当前运行状态，也不生成 runtime managed config。

从层次上看，Agent、Capability Discovery、Package 是产品语义层；Runtime Facts、Boundary、Adapter 是 runtime 边界层；Planning、Materialize、Run、Reconcile 是执行编排层；Store 和 Diagnostics 是横向支撑层；CLI 是最外层表现层。

这样的组织方式让每个模块有明确问题域：Agent 表达用户意图，Capability Discovery 表达可选择能力，Boundary 表达隔离边界，Adapter 表达 runtime 映射，Materialize 表达文件写入，Run 表达一次命令生命周期。任何 runtime 的特殊行为都应该进入 Runtime Facts、Boundary 或 Adapter，而不应该泄漏到 Agent CRUD、Package 或 CLI 里。
