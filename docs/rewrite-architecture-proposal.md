# AVM 重写架构方案草案

> 状态：草案
>
> 范围：本文只描述重写后的总体分层、职责边界和调用方向，不定义具体数据结构、接口或函数。

## 1. 总体方向

AVM 重写后的主线应当从用户理解的 Agent 出发，而不是从底层 runtime、同步状态或 shell activation 出发。

用户日常面对的核心问题是：

1. 我有哪些 Agent？
2. 每个 Agent 包含哪些 instructions、skills、MCP？
3. 这次要运行哪个 Agent？
4. 这个 Agent 应该通过哪个 runtime 执行？
5. AVM 本次写入了什么、隔离了什么、哪些能力被 runtime 原生承接或降级？

因此，新版本 AVM 的主流程应收敛为：

```text
创建 / 编辑 Agent
  -> 选择或确认 runtime
    -> 为 Agent/runtime 组合建立运行边界
      -> 将 Agent 定义转换为 runtime managed config
        -> 启动 runtime
          -> 解释本次运行结果和差异
```

这意味着旧版本里的长期 active 状态、Environment 切换、shell activation 和手动 sync 不再是产品主路径。它们最多只能作为迁移参考、兼容能力或诊断入口存在，不能继续决定核心模型。

## 2. 分层概览

AVM 重写后的最粗粒度可以分为四层：

```text
Presentation / 表现层
  -> Application / 应用层
    -> Runtime Integration / Runtime 适配层
      -> Infrastructure / 基础设施层
```

调用方向应保持单向。上层可以调用下层，下层不应该反向知道上层的交互方式或产品流程。

这四层的核心分工是：

- Presentation 负责怎么和用户交互。
- Application 负责一个用户动作在 AVM 产品里意味着什么。
- Runtime Integration 负责 AVM 语义如何落到具体 runtime。
- Infrastructure 负责可靠读写和外部副作用。

## 3. Presentation / 表现层

表现层负责用户入口和输出形态。

它包含：

- CLI 命令和参数解析。
- interactive / non-interactive 模式处理。
- 选择、确认、取消等交互流程。
- preview、diff、status、doctor、run 输出的文本或 JSON 展示。
- 用户可理解的错误信息格式化。

表现层不承载产品规则，也不直接拼 runtime 路径、写 managed config 或解释 adapter mapping。

例如，表现层可以接收：

```bash
avm run backend-coder --runtime codex
```

然后构造请求交给应用层。至于 runtime 是否唯一、冲突如何处理、哪些 warnings 需要返回，应该由应用层和 runtime 适配层产出结构化结果，再由表现层展示。

## 4. Application / 应用层

应用层负责 AVM 的产品语义和用户动作编排。

它是重写后最重要的业务中心，负责把 PRD 中的用户需求组织成完整流程。

它包含：

- Agent 创建、编辑、删除、复制、重命名。
- Agent 运行流程。
- Package 安装、导出、检查、卸载。
- 能力选择流程，包括 skills、MCP 和后续可能出现的 commands、hooks、toolsets。
- runtime 选择规则，例如自动解析、交互确认、非交互报错。
- run preview 和冲突决策。
- drift 对齐策略，例如合并回 Agent、丢弃、本次保留。
- doctor / status 这类诊断用例的应用层编排。

应用层拥有产品规则，但不拥有 runtime-specific 文件细节。

例如，`RunAgent` 这类应用流程可以负责：

```text
读取 Agent
  -> 解析或确认 runtime
    -> 获取能力发现结果
      -> 请求 Runtime Integration 生成运行计划
        -> 请求 Infrastructure 执行写入和启动进程
          -> 记录 run log
            -> 返回可展示结果
```

应用层不应该知道 Codex 的 `CODEX_HOME` 如何组织、Claude Code 的配置文件如何落盘，也不应该自己 `os.WriteFile` 修改 runtime config。这些属于 Runtime Integration 和 Infrastructure。

## 5. Runtime Integration / Runtime 适配层

Runtime 适配层负责把 AVM 的 Agent 语义转换成具体 runtime 能理解和执行的形态。

它包含：

- runtime facts，例如名称规范化、binary detection、版本探测、支持能力清单、已知风险。
- runtime adapter，例如 Codex、Claude Code、OpenCode、Cline、Cursor 的字段映射。
- mapping status，例如 native、rendered_as_instructions、ignored、unsupported。
- Agent/runtime 组合的 boundary isolation。
- managed config plan。
- runtime-specific warnings。
- runtime launch env。

它回答的是：

```text
这个 Agent 通过 Codex 怎么表达？
这个字段 Codex 是否原生支持？
如果不支持，是否可以降级为 instructions？
这个 Agent/runtime 的私有状态目录在哪里？
启动 runtime 时要注入哪些环境变量？
哪些 runtime-native 风险需要展示给用户？
```

Runtime 适配层不决定用户是否覆盖冲突，也不负责 CLI 文案。它产出结构化计划、映射结果和 warnings，让应用层决定流程，让表现层负责展示。

## 6. Infrastructure / 基础设施层

基础设施层负责可靠读写和外部副作用。

它包含：

- AVM home 读写。
- Agent、Package metadata、run log、drift state 等持久化。
- YAML / JSON 解析和校验辅助。
- 原子写入、备份、路径安全检查。
- zip package IO、checksum 校验。
- 文件系统扫描。
- runtime managed file 写入。
- 进程启动。
- 系统环境读取。

基础设施层不拥有产品决策。

它只提供能力，例如：

```text
读取 Agent 文件
写入 Agent 文件
扫描某个目录
写入 managed file
创建备份
保存 run log
启动进程
```

它不应该决定用户应该选择哪个 runtime、是否覆盖某个冲突、某个字段应该 native 还是 unsupported。

## 7. 核心对象的归属

### Agent

Agent 是用户直接创建、修改、删除、复制、运行和分享的主对象。

Agent 的产品语义属于 Application 层；Agent 的纯数据模型可以放在应用层或独立 domain 包中；Agent 文件的读写属于 Infrastructure 层。

### Runtime

Runtime 是执行载体，不是用户主对象。

Runtime 的差异、能力边界、映射状态、运行边界和启动环境属于 Runtime Integration 层。

### Package

Package 是分发单元，不是运行对象。

Package 的安装、导出、冲突策略属于 Application 层；zip 读写、checksum、路径安全属于 Infrastructure 层。

### Capability

Capability 包括 skills、MCP，以及后续可能出现的 commands、hooks、toolsets。

能力选择流程属于 Application 层；runtime 全局能力如何扫描、如何识别来源，可能需要 Runtime Integration 和 Infrastructure 协作。

关键原则是：create/edit 必须看到全量可发现能力，并保留来源信息；但 AVM 不能把 runtime 原生文件扫描结果伪装成完整 AVM Agent。

### Environment

Environment 不进入用户主线。

当前阶段只保留内部 default 上下文，不提供用户可见的 Environment CRUD、切换、导入导出，也不让 Package 携带 Environment。

### Memory

Memory 不成为 AVM 对象。

AVM 不提供 memory CRUD，不导入、导出、编辑或同步 runtime-native memory。AVM 只负责同一个 Agent/runtime 组合的运行边界隔离，并向用户解释边界和风险。

## 8. 主要用户路径

### 创建 / 编辑 Agent

```text
Presentation
  -> Application: CreateAgent / EditAgent
    -> Runtime Integration: 预览 runtime mapping 和能力边界
    -> Infrastructure: 保存 Agent
```

创建和编辑路径不写 runtime managed config，也不启动 runtime。

它的重点是帮助用户定义 Agent，并从全量可发现能力中选择 skills、MCP 等能力。

### 运行 Agent

```text
Presentation
  -> Application: RunAgent
    -> Runtime Integration: 生成 Agent/runtime 运行计划
      -> Infrastructure: 写入 managed config，启动 runtime，记录日志
```

运行路径是命令级行为，不产生需要用户长期管理的 active 状态。

每次运行都必须能解释：

- 使用了哪个 Agent。
- 使用了哪个 runtime。
- 写入了哪些 managed paths。
- Agent/runtime 的隔离边界是什么。
- 哪些字段 native、rendered_as_instructions、ignored 或 unsupported。
- managed config 与 Agent 定义是否存在 drift。

### 打包 / 安装

```text
Presentation
  -> Application: InstallPackage / ExportPackage
    -> Infrastructure: 读取或写入 package 文件和 AVM home
```

Package 安装后的结果应该是用户可以继续维护和运行的 Agent。

Package 不携带 Environment，也不应该把 runtime 原生文件扫描结果伪装成完整 AVM Agent。

### Doctor / Status

```text
Presentation
  -> Application: Doctor / Status
    -> Runtime Integration: runtime facts、boundary、mapping 状态
    -> Infrastructure: 文件系统、AVM home、runtime managed paths 状态
```

Doctor 和 status 是解释系统状态的入口，不是普通用户运行 Agent 前必须执行的步骤。

## 9. 当前设计原则

1. Agent 是唯一用户主对象。
2. Runtime 是执行载体，不反向选择 Agent。
3. Environment 不进入用户主线。
4. Package 是分发单元，不是运行对象。
5. Memory 不成为 AVM 对象。
6. Application 层是产品规则中心。
7. Runtime Integration 层是 runtime 差异中心。
8. Infrastructure 层是副作用中心。
9. Presentation 层只负责入口和展示。
10. 核心调用方向保持单向，避免 CLI、adapter、store 互相承载业务逻辑。

## 10. 后续需要继续讨论的问题

当前草案还没有展开以下细节：

- Agent 数据模型和 runtime config 字段边界。
- Application 层是否拆分 domain model 与 application service。
- Runtime Integration 层是否以 RuntimeDriver 聚合 facts、adapter、boundary、launcher。
- Capability discovery 的来源模型和引用策略。
- managed config 与 Agent 定义 drift 的具体对齐机制。
- Package 是否携带能力本体、引用还是两者都支持。
- 从旧 activation/sync/environment 模型迁移到新模型的兼容路径。
