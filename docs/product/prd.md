# Agent VM — 产品需求文档（PRD）

> 最后更新：2026-04-24（v12 — Portable Memory 边界）

## 产品定位

**一句话：** Agent VM 是 AI Agent Profile 的本地管理层，让用户可以像切换开发环境一样切换一个可迁移的 Agent 配置，必要时再把多个 runtime 绑定到同一个工作场景。

**核心对象不是配置文件，也不是 Environment，而是 Agent Profile。** 配置文件只是不同运行时的落地格式；Environment 只是多 runtime 场景下的激活映射。Agent VM 要回答三个更底层的问题：

1. **这个 Agent 是谁？** 它的角色、运行时、能力、权限、行为边界是什么。
2. **这个 Agent 可以使用什么？** 它引用哪些 skills、MCP、commands、hooks 和 tool policy。
3. **这个 Agent 默认携带哪些记忆？** 它引用哪些用户偏好、项目知识和团队约定。

**核心差异：**
- 竞品多在做 Config Sync：把一份规则或 MCP 配置渲染到多个工具。
- Agent VM 做 Agent Virtualization：把 Agent 的定义、能力和可迁移记忆引用抽象出来，再适配到主流 CLI、IDE 和 OpenClaw 这类 gateway/always-on Agent 运行时。

**阶段性叙事：**
- Phase 1：对外讲「一键切换 Agent Profile」。
- Phase 2：对外讲「Agent Profile + Portable Memory + 多 runtime 工作场景」。
- Phase 3：对外讲「团队级 Agent Registry」。

## 核心论点

AI Agent 会长期多形态并存，不会收敛成一个统一工具。

- 不同运行时擅长不同事情：Codex 偏代码执行与 repo 修改，Claude Code 偏本地开发协作，IDE Agent 偏编辑器内上下文，OpenClaw 这类 gateway Agent 偏多渠道消息、路由、workspace、sandbox 和常驻自动化。Agent VM 不假设每个运行时都有完整、同构的 Agent Profile。
- 不同组织会有不同 Agent Profile：同样叫 reviewer，在一个团队里可能只能读代码，在另一个团队里可以跑测试、开 PR、改 CI。
- 不同运行时表达 Agent 的方式不会统一：有的以 profile、permission、sandbox 组织，有的以 agent frontmatter、skills、MCP、hooks 组织，有的以 recipe、extension、toolset 组织。Agent VM 只抽象可迁移的主干，并保留 runtime 原生扩展。
- 长期记忆会越来越重要，但 memory 机制不会统一：项目架构、历史决策、用户偏好和团队约定，不应该散落在每个 Agent 自己的 memory、rules 或配置文件里。

MCP 只解决“Agent 如何连接工具”，没有解决“Agent 如何被定义、复用、迁移和挂载可审计记忆”。Agent VM 的机会在于成为 Agent 的本地控制平面。

## 目标用户（ICP）

- AI Agent 重度用户：日常使用 2 个以上 Agent 工具，已经有明显的配置、上下文和工具切换成本。
- 小团队技术负责人：希望把团队里有效的 Agent 工作方式沉淀下来，让新人或其他项目复用。
- Agent workflow builder：会创建专用 Agent，比如 planner、coder、reviewer、researcher、ops-runner，并希望管理它们的权限、工具和记忆引用。

当前替代方案：

- 手动维护 `AGENTS.md`、`CLAUDE.md`、`.cursorrules`、MCP config、插件目录等多份文件。
- 用 dotfiles/symlink 同步工具管理本地文件。
- 用某个 Agent 平台自己的 profile 或 memory，但难以迁移到其他 Agent，且很难控制每个角色到底加载了哪些记忆。
- 大部分人直接忍受碎片化，只深度使用一个工具。

## 问题与痛点

### P0：Agent 的定义没有统一对象

今天用户说“我想要一个后端开发 Agent”，实际要改很多散落的东西：

- 指令文件：`AGENTS.md`、`CLAUDE.md`、`.cursorrules`
- 能力目录：skills、commands、plugins、hooks
- 工具连接：MCP servers、API keys、数据库连接
- 权限设置：是否能写文件、执行 shell、联网、开 PR、访问生产资源
- 运行时参数：模型、上下文长度、approval policy

这些信息共同定义了一个 Agent，但没有一个一等公民对象来表达它。

Agent VM 不试图把所有 runtime 的原生概念抹平。它要提供的是一条共同主干：身份、指令、能力、权限、记忆引用和运行参数；每个 adapter 再负责承接 runtime 自己的 profile、agent、recipe、extension、toolset 或 sandbox 细节。

### P0：长期记忆缺少可审计迁移层

用户在工作中会不断沉淀可复用记忆：

- 项目知识：架构边界、模块职责、常见坑
- 历史决策：为什么用了某个方案，哪些方案被否掉
- 用户偏好：回答语言、代码风格、review 口径

这些不是 Agent 的“出厂定义”。现在它们常常被塞进 rules、memory、会话摘要或本地笔记里，导致不可迁移、不可选择、不可审计。

这里的重点不是让 AVM 取代 Claude Code、Codex 或 IDE Agent 的原生 memory，而是提供一个可审计的中间层：**当前 Agent Profile 应该携带哪些 memory refs，这些 memory refs 如何从某个 runtime 读出、写入另一个 runtime、或迁移到团队共享格式？**

因此 memory 是 AVM 的关键对象，但读写必须显式发生：`avm memory import`、`avm memory export`、`avm memory push`、`avm memory pull`。`avm use` 默认只投影当前 profile 引用的 memory，不静默双向同步 runtime 原生 memory。

### P0：切换对象不清晰

用户最先想做的事情通常不是“创建一个环境”，而是：

```bash
avm use backend-coder
```

然后当前 Codex、Claude Code、OpenCode 或 Cline 就应该按这个 Agent Profile 工作。这个 Profile 应该包含角色、指令、模型、权限、能力引用和记忆引用，否则用户会分不清“到底该改 agent 还是改 env”。

如果把 skills、MCP、memory refs 主要放在 Environment 里，用户会遇到三个问题：

- 导出一个 Agent 时，不知道它需要的能力是否一起迁移。
- 一个 Env 里有多个 Agent 时，不知道运行 `codex` 到底使用哪个 Agent。
- 同一个 Agent 在不同 Env 下能力变化太大，Profile 本身失去稳定含义。

因此 Phase 1 的主路径必须是 **激活一个 Agent Profile**，而不是先要求用户理解 Environment。

### P1：多 runtime 工作场景需要明确映射

当用户同时使用多个 runtime 时，才需要 Environment。它不是能力组合器，而是把一个工作场景映射到多个 runtime 的激活表。

同一个用户可能有多套工作场景：

- 后端开发：coder + reviewer + GitHub MCP + Postgres MCP + 项目架构记忆
- 前端开发：coder + UI reviewer + Figma MCP + design system 记忆
- 技术写作：writer + researcher + Notion MCP + 文档风格记忆
- 生产排障：ops-runner + incident-summarizer + 日志 MCP + 只读权限

场景切换不是让用户在一个 env 里手动挑 Agent，而是提前声明：

```text
backend-dev 场景下：
- codex 默认使用 backend-coder
- claude-code 默认使用 code-reviewer
- cline 默认使用 backend-assistant
```

用户执行 `avm use backend-dev` 后，再运行 `codex`、`claude`、`cline`，每个 runtime 都已经切到对应的 Agent Profile。

### P1：团队无法复用成熟 Agent

一个高级工程师调出来的 reviewer Agent，很难完整分享给团队：

- 只能分享一段 prompt，缺少工具和权限配置。
- 只能分享配置文件，缺少可复用记忆和适用边界。
- 只能分享某个平台的插件，换 Agent 运行时就失效。

团队真正需要的是可版本化、可导入、可审计的 Agent Profile。

## 概念模型

### 1. Agent Profile：这个 Agent 是谁、能做什么

Agent Profile 是用户直接创建、切换、导出和分享的主对象。它定义一个 Agent 的身份、运行契约、能力引用、权限边界和记忆引用。

```yaml
agent: backend-coder
identity:
  role: "修改后端代码并保持测试通过"

runtime:
  adapter: codex
  kind: local
  mode: primary

source_scope: project

model:
  name: inherited

run:
  max_turns: 50

instructions:
  base: engineering-principles
  project: repo-guidelines

capabilities:
  skills:
    - test
    - refactor
    - migration
  mcps:
    - github
    - postgres-readonly

permissions:
  filesystem: project-write
  shell: allowlisted
  network: restricted
  approval: on-risky-actions

memory_refs:
  - project-architecture
```

它回答：

> 当我 `avm use backend-coder` 时，我到底激活了一个什么样的 Agent？

不同运行时的定义方式可以不同，但 Agent VM 内部只维护一组最小、可降级的统一抽象：

| 字段 | 含义 |
|------|------|
| identity | Agent 名称、角色、描述 |
| runtime | 目标运行时、adapter、local/remote 形态 |
| mode_kind | primary/subagent/all、local/remote、recipe 等可降级运行形态 |
| source_scope | global、user、project、local、plugin、extension、managed 等来源和优先级 |
| instructions | 基础指令、项目指令、规则文件、SOUL/CLAUDE/AGENTS 等引用 |
| model_run | 模型、推理强度、temperature、max turns/steps 等运行参数 |
| io_contract | 可选输入/输出 schema、任务参数、response schema |
| capabilities | 该 Agent Profile 引用的 skills、commands、MCP、hooks、plugins、extensions、toolsets 或工具策略 |
| permissions | 文件、shell、网络、外部系统等意图级权限边界 |
| memory_refs | 该 Agent Profile 引用的可迁移记忆层、文件或条目 |
| lifecycle_hooks | 可选初始化、setup、pre-turn、post-run、approval、sync 行为 |

Adapter 的职责是把这些字段尽量映射到目标运行时；目标运行时没有的能力就明确标记为 unsupported、ignored 或 rendered-as-instructions，而不是假设所有 Agent 都有完整一致的模块。

`workspace_isolation` 不进入 Agent Profile 的共同主干。repo、目录、git worktree、sandbox、container、remote workspace 等执行范围由 runtime 或用户启动方式决定；AVM Phase 1 只记录 adapter 是否能映射相关权限，并在 `avm status` 中说明实际生效结果。

这个模型采用 **共同主干 + 运行时扩展**：共同主干用于跨运行时迁移和团队共享，运行时扩展用于保留目标工具的原生能力。用户在 `avm status` 中看到的是每个字段的映射结果，而不是一个虚假的“全平台完全兼容”承诺。

### 2. Capability Registry：能力本体在哪里

Capability Registry 是可复用能力池，存放能力本体；Agent Profile 只引用能力。这样 `backend-coder` 导出时可以带上它依赖的能力引用和可迁移元数据，而不是把 MCP 启动方式、skill 文件和 hook 定义散落在 Environment 里。

不同 runtime 的能力形态不同，AVM 记录能力的类型、来源、作用域、启用状态和权限约束。

- Skills：操作手册、任务流程、专业能力
- Commands：显式可调用动作
- MCP servers：外部系统连接
- Hooks：事件触发行为
- Plugins / Extensions / Toolsets：运行时内建或外部扩展集合
- Tools policy：工具允许列表、拒绝列表、审批规则、持久授权

一个 Capability 可以被多个 Agent Profile 引用。Phase 1 的默认规则是：**能力绑定跟着 Agent Profile 走**。Environment 不重复定义能力，只能在未来通过 override 做临时禁用或替换。

### 3. Portable Memory：这个 Agent 默认携带哪些长期记忆

Portable Memory 是 Agent 使用时可选择加载、导出、迁移的长期记忆层。它不是 AVM 版“Agent 大脑”，也不等同于某个 runtime 内置的 memory；它是把不同 runtime memory 读写成可审计、可选择、可迁移对象的中间层。

```yaml
memory_ref: project-architecture
scope: project
type: curated-memory
source:
  type: file
  path: .avm/memory/project-architecture.md
render:
  default: instructions

entries:
  - "API 层不直接访问数据库，必须经过 service。"
  - "支付模块历史上避免引入异步重试，因为账务幂等成本高。"
```

Phase 1 只把 memory 当成可引用、可审计、可替换的文件或条目集合，不把所有工作过程产物都纳入统一模型。

| 类型 | 说明 | 默认归属 |
|------|------|---------|
| User Preferences | 用户稳定偏好，如语言、风格、review 口径 | user |
| Project Knowledge | 项目架构、模块边界、依赖约束 | project |
| Decisions | 已做过的技术/产品决策 | project/team |

AVM 与 runtime memory 的关系必须保持显式、可审计、可回滚：

| 层 | 谁管理 | AVM Phase 1 行为 |
|----|--------|------------------|
| AVM portable memory | AVM / 用户文件 | Agent Profile 选择引用，adapter 渲染为 instructions、规则片段或上下文文件 |
| Claude Code / Codex 原生 memory | 各 runtime | 可发现、可读出为 portable memory；不在 `avm use` 时静默写回 |
| `AGENTS.md` / `CLAUDE.md` / `.clinerules` | repo 或 runtime | 可引用、导入或生成片段，但不默认覆盖用户文件 |
| 会话摘要、PR、测试报告、benchmark 结果 | 运行过程产物 | Phase 1 不纳入模型；未来可作为 memory 的证据来源 |

关键原则：**Agent Profile 和 Portable Memory 必须分离，但引用关系写在 Agent Profile 上。** 前者定义“谁来做、能用什么、默认携带哪些记忆”，后者定义“这些记忆的内容、来源、读写权限和迁移方式”。

### 4. Environment Activation：多 runtime 激活映射

Environment 不是用户的第一层心智，也不是能力的主要归属。它只在用户需要同时配置多个 runtime 时出现，负责把一个工作场景映射到各 runtime 应该启用的 Agent Profile。

```yaml
environment: backend-dev

runtime_agents:
  codex:
    primary: backend-coder
  claude-code:
    primary: code-reviewer
  cline:
    primary: backend-assistant

targets:
  - codex
  - claude-code
  - cline
```

它回答：

> 我现在进入“后端开发”场景时，每个 runtime 应该默认使用哪个 Agent Profile？

激活 `backend-dev` 后，用户不需要再从 env 里挑 Agent：

```bash
(avm:backend-dev) $ codex   # 使用 backend-coder
(avm:backend-dev) $ claude  # 使用 code-reviewer
(avm:backend-dev) $ cline   # 使用 backend-assistant
```

## 产品方案

### 核心体验

用户通过 Agent VM 创建和切换 Agent Profile。最小路径应该足够直接：

```bash
avm init
avm agent create backend-coder \
  --runtime codex \
  --skills test,migration \
  --mcps github,postgres-readonly \
  --memory project-architecture

avm use backend-coder
```

激活后的体验接近 conda，但切换对象不是 Python 依赖环境，而是一个 Agent Profile：

```bash
(avm:backend-coder) $ codex
```

切换后：

- Codex 获得 `backend-coder` 对应的 config/profile、permission/sandbox、MCP、agent role 和 instructions；已有 `AGENTS.md` 可被导入或引用，但不默认覆盖。
- Agent Profile 引用的 skills、MCP 和 memory refs 被渲染到目标 runtime 能表达的位置。
- 不在当前 Agent Profile 里的 skills、MCP、记忆不会进入本次 runtime 配置。
- shell prompt 显示当前 active profile，帮助用户确认新开的 Agent CLI 正在使用同一套配置。

当用户需要同时使用多个 runtime，才创建 Environment：

```bash
avm agent create backend-coder --runtime codex --skills test,migration --mcps github,postgres-readonly
avm agent create code-reviewer --runtime claude-code --skills review --mcps github
avm agent create backend-assistant --runtime cline --skills test --mcps github

avm env create backend-dev \
  --codex backend-coder \
  --claude-code code-reviewer \
  --cline backend-assistant

avm use backend-dev
```

激活 `backend-dev` 后：

```bash
(avm:backend-dev) $ codex   # 使用 backend-coder
(avm:backend-dev) $ claude  # 使用 code-reviewer
(avm:backend-dev) $ cline   # 使用 backend-assistant
```

Environment 的作用是让多个 runtime 同时进入同一个工作场景；它不重新定义 Agent 的能力。

### 用户故事

**场景 1：定义并使用一个专用 Agent**

用户创建 `backend-coder`，指定它使用 Codex，允许修改当前 repo，能运行测试和迁移脚本，但访问数据库只能走只读 MCP。以后用户不再手动拼 prompt，也不先创建 env，而是直接执行 `avm use backend-coder` 后启动 Codex。

**场景 2：切换单个 Agent Profile**

用户上午写后端代码，执行 `avm use backend-coder`。下午写技术文档，执行 `avm use tech-writer`。shell prompt 从 `(avm:backend-coder)` 变成 `(avm:tech-writer)`，后续启动的 Agent CLI 获得不同的 instructions、skills、MCP 和 memory refs。

**场景 3：一个工作场景绑定多个 runtime**

用户在同一个 repo 里同时使用 Codex 写代码、Claude Code 做 review、OpenCode 处理开源 CLI 工作流、Cline 处理 IDE 内上下文。用户执行 `avm use backend-dev` 后，Codex 默认使用 `backend-coder`，Claude Code 默认使用 `code-reviewer`，OpenCode 默认使用 `opencode-coder`，Cline 默认使用 `backend-assistant`。用户不需要在 env 里再次选择 Agent。

**场景 4：沉淀项目知识**

Agent 在一次任务中发现“支付模块不能使用普通重试”。用户确认后，这条经验写入 `project-decisions` memory ref。以后 coder、reviewer、planner 都能加载这条知识，而不是只留在某次会话里。

**场景 5：跨运行时迁移 Agent**

用户先用 Claude Code 配好了 reviewer Agent。后来想在 Codex 中运行同样的 review 角色。Agent VM 保留统一 Agent Profile，由 adapter 翻译为目标运行时能理解的配置，并在 `avm status` 中显示哪些字段无法映射。

**场景 6：迁移 runtime memory**

用户在 Claude Code 里积累了一批项目记忆，后来开始用 Codex 写代码。用户执行 `avm memory pull --from claude-code`，AVM 读出可迁移条目并生成 diff；用户确认后写入 portable memory，再执行 `avm memory push --to codex`。Codex 获得同一批项目知识，但写入过程可审计、可回滚。

**场景 7：团队共享 Agent Profile**

团队维护一个 `company-code-reviewer.avm`，包含 reviewer 的角色定义、权限边界、review checklist、允许使用的 MCP 和团队工程原则。新人导入后直接获得同一套工作方式。

### 使用场景判定

| 用户想做什么 | 推荐命令 | 用户心智 |
|--------------|----------|----------|
| 只想让 Codex/Claude/OpenCode/Cline 按某个角色工作 | `avm use backend-coder` | 激活一个 Agent Profile |
| 想在两个角色之间切换 | `avm use backend-coder` / `avm use tech-writer` | 切换当前 active profile |
| 想让 Codex 写代码、Claude Code review、OpenCode 跑开源 CLI、Cline 辅助 IDE | `avm use backend-dev` | 激活一个多 runtime 工作场景 |
| 想迁移或分享一个专用 Agent | `avm export backend-coder` | 分享 Agent Profile，包含能力引用和 memory refs |
| 想迁移一整套多 runtime 场景 | `avm export backend-dev` | 分享 Environment，以及它引用的 Agent Profiles |

Phase 1 的产品默认从单 Agent Profile 开始。Environment 是进阶能力，用来解决“同一场景下多个 runtime 各用哪个 Agent”的问题，不是用户配置能力的主入口。

## 核心能力

| 能力 | 说明 | 优先级 |
|------|------|--------|
| Agent Profile | 创建、编辑、查看 Agent 的身份、运行时、指令、能力引用、权限和 memory refs | P0 |
| Capability Registry | 管理 skills、commands、MCP、hooks、tool policy 等能力本体，供 Agent Profile 引用 | P0 |
| Environment Activation | 将多个 runtime 绑定到各自的默认 Agent Profile，形成可切换工作场景 | P0 |
| Runtime Adapter | 将统一抽象翻译为 Codex、Claude Code、Cline/IDE、Cursor 等运行时格式 | P0 |
| 快速切换 | `avm use <agent-profile>` 激活单个 Agent；`avm use <env>` 激活多 runtime 场景 | P0 |
| 冲突检测 | 写入目标运行时配置前检测外部修改，避免覆盖用户数据 | P0 |
| 状态仪表盘 | `avm status` 显示当前 active profile/env、runtime 绑定、能力映射和同步状态 | P0 |
| Portable Memory | 保存、读取、写入、迁移用户偏好、项目知识和关键决策，并投影到 runtime 能表达的位置 | P1 |
| 导入导出 | Agent Profile / Environment 可导出为 `.avm` 文件，支持迁移和分享 | P1 |
| 项目级覆盖 | 项目可以覆盖全局 Agent Profile 或 Env 的 runtime 绑定 | P1 |
| 记忆治理 | 新知识确认、删除、合并、作用域管理、敏感信息过滤 | P2 |
| 团队 Registry | 团队级 Agent Profile 市场/私有仓库，支持版本和审批 | P2 |

## MVP 范围

### Phase 1：本地 Agent Profile 原型（2 周）

目标：验证用户是否愿意用一个统一对象来管理 Agent，而不是继续手动改散落配置。

**必须实现：**

- `avm init`：扫描本机已有 Agent 配置，建立 registry。
- `avm agent create <name> --runtime <runtime> --skills x,y --mcps m,n --memory a,b`：创建 Agent Profile。
- `avm agent list/show`：查看 Agent Profile。
- `avm use <agent-profile>`：激活单个 Agent Profile，写入目标 runtime 配置。
- `avm env create <name> --codex <agent> --claude-code <agent> --cline <agent>`：创建多 runtime 工作场景。
- `avm use <env>`：激活多 runtime 工作场景，按 runtime_agents 写入目标 runtime 配置。
- `avm shell init <shell>`：让 shell prompt 显示当前 active profile/env。
- `avm deactivate`：回到 `default` Agent Profile 或 default env。
- `avm status`：显示当前 active profile/env、runtime 绑定、能力映射和同步状态。
- `avm export/import`：导出/导入环境与 Agent Profile。

**Phase 1 只做最小 Portable Memory：**

- 支持 `memory_refs` 字段，用于手动引用已有 markdown/yaml 文件。
- memory 引用优先写在 Agent Profile 上，Environment 不重复声明 memory。
- 支持从已有规则文件或 runtime memory 做显式 import/dry-run，先验证迁移价值。
- 不做自动记忆合并。
- 不做跨 Agent 自动学习。
- 不在 `avm use` 时写入 Claude Code、Codex 或 IDE Agent 的原生 memory。
- 不做云同步。

**Phase 1 首批 runtime adapter：**

| Runtime | Phase 1 支持 | 原因 |
|---------|--------------|------|
| Claude Code | `agents/*.md` frontmatter、skills/MCP 子集、permissionMode、memory 引用渲染 | 已有一等 subagent 定义，适合验证 Agent Profile + Capability 映射 |
| Codex | `config.toml` profile/permissions/MCP 输出、agent roles/role TOML；`AGENTS.md` 只导入或引用，不默认覆盖 | 代码 Agent 代表，permission/sandbox/profile/roles 边界清晰 |
| OpenCode | `OPENCODE_CONFIG`、`OPENCODE_CONFIG_DIR`、agent markdown、permissions、MCP、skills | 当前开源 CLI Agent 代表，且官方支持环境变量隔离 |
| Cline | `.clinerules`、MCP settings、skills/auto approval 安全子集；subagents 仅作为能力状态，不等价于 Agent Profile | 覆盖 IDE Agent 场景，并验证 rules/MCP 的组合方式 |

Cursor 仍是产品目标，但 Phase 1 不承诺完整 Cursor adapter；先保留 rules/MCP 文件级兼容 PoC，用来验证 IDE 场景需求。

OpenClaw/gateway 类 Agent 先作为设计约束，不在 Phase 1 强行实现。自研、小众或外部 Agent 形态暂不进入首版 adapter 表。

**完成标准：**

- 用户能创建至少 2 个 Agent Profile，并在两个 profile 间切换。
- 用户能执行 `avm use backend-coder` 激活单个 Agent Profile，并看到 prompt/status 变化。
- 用户能创建至少 1 个 Environment，把 Codex/Claude Code/OpenCode/Cline 分别绑定到不同 Agent Profile。
- `avm use` 后，runtime 只看到当前 Agent Profile 引用的 skills、MCP 和 memory refs。
- 用户能对至少一个 runtime 或规则文件执行 `avm memory import --dry-run`，看到可迁移 memory diff。
- Codex/OpenCode/Cline 获得当前 Agent Profile 或 Environment 映射后的配置输出；Cursor 完成 rules/MCP 文件级 PoC。
- export 的 `.avm` 文件能在另一台机器 import 后复现 Agent Profile。
- 冲突检测不会覆盖用户手动修改。

### Phase 2：Portable Memory 迁移与治理（1 个月）

- 引入 memory refs 的正式 schema，并定义与 runtime native memory 的 import/export/push/pull 边界。
- 支持用户确认后将经验写入 user/project/team scope。
- 支持 `avm memory list/add/prune/export/import/diff`。
- 支持 `avm memory push --to <runtime>` 和 `avm memory pull --from <runtime>`，但必须显示 diff 并要求用户确认。
- 支持 Agent Profile 引用不同 memory refs。
- 增加 OpenClaw/gateway 类 Agent adapter 的调研和 PoC。

### Phase 3：团队 Registry + 市场（3 个月）

- 团队私有 Agent Profile registry。
- 官方/社区 Agent 模板。
- Profile 版本管理、审批、审计。
- 云同步、团队共享、加密 credential 管理。

## 设计原则

1. **Profile 和记忆内容分离**：Agent Profile 引用 memory refs，但长期记忆内容由可审计文件或条目承载；AVM 不应静默接管某个 runtime 的原生 memory。
2. **Profile 先于适配**：先定义 Agent VM 的统一模型，再由 adapter 翻译到各运行时。
3. **手动确认优先**：新知识进入长期 memory 或写回 runtime native memory 前需要用户确认，避免污染。
4. **本地优先**：Phase 1 所有数据本地存储，用户能看见、编辑、删除。
5. **不覆盖用户资产**：任何写入目标运行时配置的动作都必须可备份、可检测冲突、可恢复。
6. **渐进兼容**：允许用户继续手动维护 `AGENTS.md`、`CLAUDE.md` 等文件，Agent VM 先成为管理层，不强迫迁移。

## 数据边界

| 数据 | 是否由 avm 管理 | Phase 1 策略 |
|------|----------------|--------------|
| Agent Profile | 是 | 新建最小兼容 schema，包含能力引用和 memory refs |
| Skills | 是 | registry + Agent Profile 引用 |
| MCP servers | 是 | registry + Agent Profile 引用 + adapter 写入 |
| Rules / instructions | 部分 | 作为 Agent Profile 输入，不强行覆盖项目文件 |
| Runtime support matrix | 是 | 每个 adapter 明确 supported/unsupported/ignored/rendered-as-instructions |
| Permissions | 是 | 先描述，再逐步映射到 runtime |
| Source scope / precedence | 是 | 记录 global/user/project/local/plugin/extension 等来源，避免覆盖 |
| User preferences | 是 | 手动 memory refs |
| Project knowledge | 是 | 手动 memory refs |
| Runtime native memory | 部分 | Phase 1 只做显式发现/导入；Phase 2 支持显式 diff、push、pull、迁移 |
| Session history | 否 | 不作为独立模块；未来可作为 memory 来源 |
| Credentials | 否 | 默认只引用环境变量，不导出明文 |

## 验证假设

1. 用户真正想管理的是 Agent，而不是配置文件。
2. Agent Profile 引用 Portable Memory，但不内联长期记忆内容，用户更容易理解、复用和迁移。
3. 本地开发者愿意为了 Agent Profile 切换和 memory 迁移多引入一个 CLI。
4. 多 Agent/多 runtime 会长期存在，adapter 层有持续价值。
5. 记忆层必须可审计、可删除、可选择、可迁移，否则用户不会信任。
6. memory 迁移是比普通配置同步更强的留存点，因为它承载用户长期投入。

## 指标

### North Star

**Weekly Active Agent Profiles（WAAP）**：每周被激活过的 Agent Profile 数。

这个指标比 WAU 更贴近产品价值：用户不是只打开工具，而是真的在复用 Agent Profile。

### Phase 1 指标

- 3 个内部用户每天使用 `avm use`。
- 每个用户创建至少 2 个 Agent Profile。
- 至少 1 个 Profile 被 export/import 到另一台机器。
- 用户能说清楚“Agent Profile”和“Environment”的区别。

### Phase 2 指标

- 每个活跃项目有至少 5 条 curated memory。
- 记忆被引用后减少重复解释的次数。
- 至少 5 个真实用户主动反馈 adapter 或 memory 迁移需求。

## 商业模式

- **Free / Open Source**：本地 Agent Profile、Capability Registry、Environment Activation、导入导出。
- **Pro（$9/月）**：跨设备云同步、加密 profile 备份、高级 memory 迁移与治理。
- **Team（$29/人/月）**：团队 Agent Registry、审批流、审计日志、SSO、私有模板市场。

## 风险与应对

| 风险 | 应对 |
|------|------|
| 抽象过大，MVP 做不完 | Phase 1 只做 Agent Profile + capability 引用 + memory refs 子集，不做自动学习 |
| 用户只想同步配置，不理解 Agent Profile | CLI 和文档从具体场景开始：`backend-coder`、`reviewer`、`writer` |
| 各 runtime 差异太大 | Adapter 允许能力降级，status 明确显示哪些字段无法映射 |
| 记忆污染导致用户不信任 | 长期 memory 和 runtime 写回必须用户确认，支持 diff、删除和作用域 |
| 平台自己做 profile/memory | Agent VM 保持跨 runtime、本地优先、可迁移的中立层 |
| 凭据泄漏 | 默认环境变量引用，导出时剔除 secrets，备份目录最小权限 |

## 竞争策略

1. **不做更好的 dotfiles 工具**：配置同步只是落地手段，不是产品核心。
2. **先占 Agent Profile 心智**：把 “一个 Agent 是什么” 变成可命名、可版本、可分享的对象。
3. **兼容现有生态**：导入 `AGENTS.md`、`CLAUDE.md`、MCP config、skills 目录，而不是要求用户重写。
4. **把记忆迁移和治理作为长期壁垒**：真正难的是让可复用记忆可读写、可信、可迁移。

## 与其他方向的关系

- **agent-portable-memory**：并入 Portable Memory 迁移与治理。
- **agent-observability**：互补，未来可用运行数据优化 Agent Profile。
- **context-capsule**：并入 Agent Profile / Environment export/import 和团队共享。
