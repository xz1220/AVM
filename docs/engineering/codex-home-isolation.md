# Codex Home 配置隔离方案讨论

> 记录时间：2026-04-27
>
> 议题：当前 Codex adapter 采用 marked-block 合并策略管理 `~/.codex/` 下的配置文件，这种方式存在根本性问题。讨论提出了一种更干净的替代方案：每个 AVM 配置对应一个完整的独立 Codex home 目录，通过 `CODEX_HOME` 环境变量切换。

---

## 一、当前设计的运作方式

当前 `codex.Plan()` + `codex.Render()` 的工作流程：

1. **Plan 阶段**：从 `.avm` 读取 agent profile、environment、memory 等配置，生成 `RenderPlan`
2. **Render 阶段**：把 `RenderPlan` 应用到 `~/.codex/` 下的文件

Codex adapter 管理三类文件：

| 文件 | 路径 | 合并策略 | 说明 |
|------|------|---------|------|
| 主配置 | `~/.codex/config.toml` | Marked block 合并 | 只改 `# >>> avm:codex` 标记之间的内容 |
| Role 文件 | `~/.codex/agents/{role}.toml` | 整文件覆盖 | name、description、developer_instructions、model 参数 |
| Skill 文件 | `~/.codex/skills/{name}/SKILL.md` | 整文件覆盖 | 带 `avm_managed: true` frontmatter |

切换配置时（`avm use env review` + `avm sync`），需要先清理旧的 role/skill 文件，再写入新的，同时更新 `config.toml` 里的 marked block。

---

## 二、当前设计的问题

### 1. 反复修改同一目录
每次 sync 都会修改 `~/.codex/` 下的文件，产生持续的文件系统副作用。如果 sync 过程中断，容易留下不一致的中间状态。

### 2. Marked block 合并的脆弱性
`config.toml` 采用 marked block（`# >>> avm:codex:codex-config` ... `# <<< avm:codex:codex-config`）策略，依赖字符串级别的文本扫描和替换：
- 如果用户手动编辑时不小心破坏了标记，AVM 无法正确识别
- 如果 Codex 自身的配置格式变化，marked block 的插入逻辑可能失效
- merge 过程需要扫描整行、验证标记嵌套（`markedBlockSpan()`），复杂度不低

### 3. 新旧配置切换不是原子的
切换配置需要先检测 stale skill 文件（`staleCodexSkillFiles()`）、删除旧文件、写入新文件、更新 config.toml。这个过程不是原子的，中间状态可能出现：
- 旧 role 已删但新 role 还没写完
- config.toml 里的 profile 引用指向一个不存在的 role 文件

### 4. 与用户手写配置混在一起
Codex 用户可能会在 `~/.codex/` 下手动创建文件（如自定义 agents、自定义 MCP 配置）。AVM 的 marked block 合并虽然尽量只改标记区间，但整文件覆盖的 role/skill 仍然会跟用户手动文件共存，长期可能导致混乱。

---

## 三、替代方案：完整 Codex Home 隔离

### 核心思路

每个 AVM 配置（agent profile + environment）对应一个**完整的、自包含的 Codex home 目录**。切换配置时**不修改任何文件**，只切换 `CODEX_HOME` 环境变量。

```
~/.avm/
├── config.yaml
├── agents/
├── envs/
└── runtimes/
    └── codex/
        ├── coding/              ← env=coding 的完整 codex home
        │   ├── config.toml
        │   ├── agents/
        │   │   └── backend-coder.toml
        │   └── skills/
        │       └── ...
        └── review/              ← env=review 的完整 codex home
            ├── config.toml
            ├── agents/
            │   └── code-reviewer.toml
            └── skills/
                └── ...
```

### 切换流程

```bash
# 当前在 coding 配置
$ echo $CODEX_HOME
~/.avm/runtimes/codex/coding

# 切换到 review
$ avm use env review
$ avm sync      # 生成 ~/.avm/runtimes/codex/review/
$ export CODEX_HOME=~/.avm/runtimes/codex/review
```

切换后 `~/.avm/runtimes/codex/coding/` 保持不动，`review/` 是全新生成的完整快照。

### 优势

1. **原子切换**：改一个环境变量 vs 改一堆文件。切换是瞬间完成的，不存在中间状态。
2. **无合并逻辑**：不需要 marked block、不需要检测 stale 文件、不需要整文件覆盖。每个 home 目录是全新 render 出来的，跟用户原有 `~/.codex/` 完全隔离。
3. **多配置共存**：coding、review、experiment... 多个配置可以同时存在，互相不干扰。想回滚就指回旧目录。
4. **Codex 原生支持**：Codex CLI 本身就支持 `CODEX_HOME` 环境变量，不需要 AVM 做 hack。
5. **可预测**：`~/.avm/runtimes/codex/{env}/` 下的所有文件都是 AVM 生成的，没有"跟用户手写配置混合"的问题。

---

## 四、对现有架构的影响

### Adapter 接口层面

当前 `Adapter` 接口：

```go
type Adapter interface {
    Name() string
    Detect(ctx Context) Detection
    Import(ctx Context) (*ImportResult, error)
    Plan(ctx Context, input RenderInput) (*RenderPlan, error)
    Render(ctx Context, plan *RenderPlan) (*RenderResult, error)
    ManagedPaths(ctx Context, plan *RenderPlan) []ManagedPath
}
```

核心变化：`Render()` 的 target 路径从固定的 `~/.codex/` 变为动态的 `~/.avm/runtimes/codex/{env}/`。这个路径可以通过 `RenderInput` 或 `Context` 传入，对 `Plan()` 阶段没有影响 — Plan 阶段只决定"写什么内容"，不决定"写到哪"。

### Sync 层

`sync.SyncActivation()` 需要在调用 `adapter.Render()` 之前确保目标目录存在，并且在渲染完成后把目标目录路径记录到 `SyncState` 中（以便 shell 集成使用）。

### Shell 集成

`avm shell` 或 shell hook 需要在激活配置时设置对应 runtime 的环境变量：
- `CODEX_HOME=~/.avm/runtimes/codex/{env}`
- Claude Code 可能也需要类似处理（Claude Code 是否支持类似的环境变量需要验证）

### Paths 层

`internal/config/paths.go` 需要新增路径函数：
- `RuntimeHomeDir(runtime, activeName string) string`
- `RuntimeCodexHome(activeName string) string`

---

## 五、待决策事项

1. **Claude Code 是否支持类似 `CODEX_HOME` 的环境变量？** 如果不支持，Claude adapter 可能需要继续用 marked block 模式，或者寻找 symlink 等其他隔离手段。
2. **是否需要保留"直接渲染到 `~/.codex/`" 的模式作为可选行为？** 某些用户可能不希望 AVM 接管他们的 home 目录结构。
3. **如何同步更新 `~/.avm/active/manifest.yaml` 中的路径信息？** 目前 manifest 只记录 runtime_agents 和 targets，可能需要扩展以记录各 runtime 的 home 目录路径。
4. **Import 流程是否需要调整？** 当前 `Import()` 从 `~/.codex/` 反向读取已有配置，如果采用隔离方案，Import 的目标路径也需要相应变化。

---

## 六、结论

当前 marked block 合并方案在 Phase 1 是可行的，但长期来看，**完整 Codex home 隔离方案是更干净、更可维护的架构选择**。它能消除合并逻辑的复杂性、避免文件系统副作用、实现原子切换，并且与 Codex CLI 的原生能力（`CODEX_HOME`）天然契合。

## 七、实现状态

2026-04-27 的 runtime home isolation 重构已把 Codex 从默认写用户真实 `~/.codex` 调整为写 AVM-owned runtime home：

```text
~/.avm/runtime-homes/<active>/codex/
├── config.toml
├── agents/<agent>.toml
└── skills/<skill>/SKILL.md
```

`config.toml` 在隔离 home 内按整文件快照输出，不再使用 marked block 合并。当前 shell 通过 `avm activate <profile-or-env>` 输出 `CODEX_HOME=<runtime-home>` 来让 Codex 读取这份配置。

真实 CLI 验证发现 Codex 的登录态也受 `CODEX_HOME` 影响。为避免隔离 home 变成未登录状态，sync 在重建 Codex runtime home 时会保留或复制 `auth.json`：

- 优先保留当前隔离 home 内已有的 `auth.json`，支持用户在隔离 home 内重新登录。
- 其次从进入 AVM 前的 `CODEX_HOME` 复制。
- 最后从默认 `~/.codex/auth.json` 复制。

这只读取用户原始 Codex home 的认证侧车文件，并写入 AVM-owned runtime home；不会修改原始 `~/.codex`，也不使用软链接。

下一步建议：
1. 继续验证 Claude Code runtime home 在真实 CLI 下的行为，尤其是 MCP 配置加载。
2. 评估 Cline/Cursor 这类 IDE/GUI runtime 是否保留项目级持久渲染。
3. 若 shell-local 模型稳定，再决定是否让 `avm use` 在 shell integration 下自动走 `avm activate`。
