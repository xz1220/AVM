# 可读性与可维护性审查：config、sync、state

## 总结

代码库结构良好，关注点分离清晰，函数一致地保持简短。主要问题是：(1) 跨包重复的工具函数（`writeYAML`、`uniqueStrings`/`uniqueNonEmptyStrings`），(2) 每个 Write 函数中的双重验证，(3) 多个已定义但从未被有意义消费的模型字段和类型，增加了数据模型的死重量。单个文件的认知负担总体较低，但 `config`、`sync` 和 `state` 之间的隐式契约（特别是围绕基于 hash 的冲突检测）需要仔细阅读才能理解。

## 函数长度分析

超过 50 行的函数：

| 函数 | 文件 | 行数 | 评估 |
|------|------|------|------|
| `validateAgentProfile` | `internal/config/validation.go:101` | 68 | 可接受 — 线性验证，易于扫描 |
| `rebuildActive` | `internal/sync/active.go:28` | 62 | 边界 — 原子交换逻辑本质上是顺序的，但 tmp/prev/rename 操作可以提取 |
| `buildActiveTree` | `internal/sync/active.go:91` | 51 | 没问题 |
| `SyncActivation` | `internal/sync/syncer.go:32` | 85 | **有问题** — 编排 + 状态变更 + 错误聚合全在一个方法中 |
| `renderTarget` | `internal/sync/syncer.go:118` | 82 | **有问题** — 长线性的 early return 链；每个 adapter 调用 + 错误处理块是一个可以命名的小阶段 |
| `resolveEnvironmentActivation` | `internal/config/resolve.go:67` | 50 | 边界但可读 |

## 过度设计发现

### 1. `TargetCapability.Level` 从未被读取（中影响）

```go
// internal/config/models.go:18-27
type TargetCapability struct {
    Level string
}

var KnownTargets = map[string]TargetCapability{
    "claude-code": {Level: "full"},
    "codex":       {Level: "full"},
    ...
}
```

`Level` 在代码库中从未被访问。`KnownTargets` 仅通过 `isKnownTarget()` 用作集合，后者检查 `_, ok := KnownTargets[target]`。该结构体添加了一个没有任何代码作用的概念（"capability level"）。在 `Level` 真正需要之前，用 `map[string]struct{}` 或 `set` 替换。

### 2. `LifecycleHooks`、`IOContract`、`AgentIdentity` 被携带但从未被消费（中影响）

```go
// internal/config/models.go:65-66,68
Identity          AgentIdentity             `yaml:"identity,omitempty"`
Instructions      Instructions              `yaml:"instructions,omitempty"`
IOContract        IOContract                `yaml:"io_contract,omitempty"`
LifecycleHooks    LifecycleHooks            `yaml:"lifecycle_hooks,omitempty"`
```

这些字段从 YAML 解析并通过验证往返，但 `config`、`sync`、`state` 或 adapter 层中没有代码读取 `Identity.Role`、`IOContract.OutputStyle` 或 `LifecycleHooks.BeforeRun` 来做决策。它们今天是纯 schema 重量。无害，但新开发者会花时间试图理解这些在哪里被消费。

### 3. `RuntimeStatusPartial` / `TargetStatusPartial` 已定义但从未被赋值（低影响）

```go
// internal/state/types.go:18
RuntimeStatusPartial RuntimeStatus = "partial"
// internal/sync/types.go:28
TargetStatusPartial TargetStatus = "partial"
```

没有代码路径将状态设置为 `"partial"`。如果这是为未来使用计划的，注释会有帮助。否则就是死代码。

### 4. `sha256String` 是 `sha256Bytes` 的简单别名（低影响）

```go
// internal/sync/conflict.go:233-235
func sha256String(value []byte) string {
    return sha256Bytes(value)
}
```

调用一次。与 `sha256Bytes` 签名和行为完全相同。名称暗示它 hash 一个字符串，但它接受 `[]byte`。删除并直接调用 `sha256Bytes`。

### 5. `Validate()` type-switch 分发器价值有限（低影响）

```go
// internal/config/validation.go:10-35
func Validate(value any) error {
    switch v := value.(type) {
    case *GlobalConfig:
        return validateGlobalConfig(v, "")
    case GlobalConfig:
        ...
```

这接受 `any`，在调用点丢失类型安全。第 37-55 行的类型化公开包装器（`ValidateActiveRef`、`ValidateAgentProfile` 等）存在且是调用者应该使用的。泛型 `Validate` 鼓励传递无类型值。如果没有外部调用者，考虑删除它。

## 可读性问题

### 1. 跨包重复的 `writeYAML`（高影响）

`config/yaml.go` 有一个原子的 `writeYAML`（temp file + rename，37 行）。`sync/active.go` 有一个更简单的 `writeYAML`（marshal + WriteFile，7 行）。同名函数，不同行为，不同安全保证。在包之间移动的读者会假设它们行为相同。

```go
// internal/config/yaml.go:38 -- 使用 temp file 的原子写入
func writeYAML(path string, value any) error { ... }

// internal/sync/active.go:185 -- 直接写入，无原子性
func writeYAML(path string, value any) error {
    raw, err := yaml.Marshal(value)
    ...
    return os.WriteFile(path, raw, 0o600)
}
```

sync 版本安全性较低（无 temp file，无 fsync）。由于 `buildActiveTree` 已经使用了一个被原子 rename 的临时目录，更简单的写入是可以辩护的——但名称冲突令人困惑。要么提取共享辅助函数，要么将 sync 本地版本重命名为 `writeManifestYAML` 或类似名称。

### 2. 重复的 unique-string 辅助函数（中影响）

存在三个近乎相同的去重函数：

- `config/resolve.go:183` — `uniqueStrings`（跳过空值，保留顺序）
- `sync/syncer.go:209` — `uniqueNonEmptyStrings`（跳过空值，保留顺序）
- `sync/active.go:164` — `sortedUniqueValues`（map 值，跳过空值，排序）

`uniqueStrings` 和 `uniqueNonEmptyStrings` 功能完全相同。提取到共享的内部工具包或合并为一个。

### 3. Write 函数中的双重验证（中影响）

每个 `Write*` 函数都验证两次——一次不带路径，然后带路径再验证一次：

```go
// internal/config/agent.go:32-42
func WriteAgent(agent *AgentProfile, scope Scope, cwd string) error {
    ...
    agent.ApplyDefaults(defaultSourceScopeForAgent(scope))
    if err := validateAgentProfile(agent, ""); err != nil {   // 第一次验证
        return err
    }
    path, err := agentPathForScope(agent.Name, scope, cwd)
    ...
    if err := validateAgentProfile(agent, path); err != nil {  // 第二次验证
        return err
    }
    return writeYAML(path, agent)
}
```

`WritePortableMemory`（memory.go:31-43）和 `WriteEnvironment`（env.go:25-38）中相同的模式。唯一的区别是第二次调用的错误信息包含文件路径。这令人困惑——读者会疑惑两次调用是否可能产生不同结果。考虑在路径已知后验证一次，或将路径传入单次验证调用。

### 4. `resolveEnvironmentActivation` 混合了覆盖加载和验证路径跟踪（中影响）

```go
// internal/config/resolve.go:67-116
func resolveEnvironmentActivation(ref ActiveRef, cwd string) (*ResolvedActivation, error) {
    ...
    validatePath := EnvPath(ref.Name)          // 跟踪错误信息中使用的路径
    ...
    if override.Extends != ref.Name { ... }
    env = MergeEnvironment(base, override)
    sourceFiles = append(sourceFiles, projectOverridePath)
    validatePath = projectOverridePath          // 为错误上下文重新赋值
    ...
    if err := validateEnvironment(env, validatePath); err != nil {
```

`validatePath` 变量在函数中途被重新赋值以改变错误上下文。这很微妙。读者必须追踪变量才能理解哪个路径最终出现在错误信息中。考虑在每个阶段用相关路径验证，而不是跟踪一个可变的"当前路径"。

### 5. 验证中的魔法字符串（低影响）

枚举类字段的允许值是内联字符串字面量：

```go
// internal/config/validation.go:86
if !oneOf(cfg.Defaults.ConflictStrategy, "prompt", "overwrite", "skip", "fail", "rename") {
// internal/config/validation.go:117
if !oneOf(agent.Runtime.Kind, "local", "remote") {
// internal/config/validation.go:120
if !oneOf(agent.Runtime.Mode, "primary", "subagent", "all") {
```

这些在验证中重复但没有对应的常量。如果新开发者添加一个 mode，他们需要找到每个 `oneOf` 调用。考虑将这些定义为 `const` 组（像 `Scope` 已经做的那样），使允许值在一个地方可发现。

## 可维护性问题

### 1. 隐式契约：基于 hash 的冲突检测（高影响）

冲突检测流程跨越三个包且无契约文档：

1. `sync/conflict.go:DetectConflicts` 将 `prior.ManagedHash` / `prior.FileHash` 与新计算的 hash 比较
2. `sync/syncer.go:runtimeStateFromTarget` 根据 `target.Status == TargetStatusSynced && target.RenderResult != nil` 决定是计算新 hash 还是沿用先前的 hash
3. `state/types.go:ManagedPathState` 存储 `FileHash` 和 `ManagedHash`，但无文档解释何时填充每个

`DetectConflicts`（第 54-57 行）中的回退逻辑在 managed hash 为空时静默从 `ManagedHash` 比较切换到 `FileHash` 比较。这是一个正确性敏感的代码路径，没有注释解释不变量。

```go
// internal/sync/conflict.go:53-57
expected := prior.ManagedHash
actual := managedHash
if expected == "" || actual == "" {
    expected = prior.FileHash
    actual = fileHash
}
```

修改此处的新开发者如果不阅读不同文件中的 `runtimeStateFromTarget` 和 `ManagedPathStatesWithHashes`，将无法理解 `ManagedHash` 何时为空与何时被填充。

### 2. `SyncActivation` 混合了编排与状态变更（高影响）

`SyncActivation`（syncer.go:32-116，85 行）做了太多事情：
- 渲染 adapter 输入
- 按目标过滤
- 重建 active 目录
- 加载/创建 sync 状态
- 迭代目标（缺失的和存在的）
- 保存状态
- 可选地更新全局 active 引用
- 聚合错误

这使得测试单个阶段变得困难，修改一个阶段时也难以避免对另一个阶段的副作用。该方法将受益于拆分为命名阶段：`prepareInputs`、`syncTargets`、`persistState`。

### 3. `state` 包依赖 `adapter` 类型（中影响）

```go
// internal/state/types.go:6
import "github.com/xz1220/agent-vm/internal/adapter"

// internal/state/types.go:64
func ManagedPathStates(paths []adapter.ManagedPath) []ManagedPathState {
```

`state` 包导入 `adapter` 来提供转换辅助函数（`ManagedPathStates`、`MappingStates`）。这创建了从持久化层到 adapter 层的依赖。这些转换函数可以放在 `sync`（已同时导入两者）中，以保持 `state` 为纯数据 + 持久化包。

### 4. `rebuildActive` 原子交换在失败时很脆弱（中影响）

```go
// internal/sync/active.go:75-79
if err := os.Rename(tmpDir, activeDir); err != nil {
    if movedOld {
        _ = os.Rename(prevDir, activeDir)  // 尽力回滚
    }
    return err
}
```

第 77 行的回滚静默丢弃其错误。如果回滚也失败，active 目录就消失了且无任何指示。这是一个已知的困难问题，但至少回滚错误应该被记录或包装到返回的错误中。

## 认知负担热点

### 1. `renderTarget` 需要理解 5 个 adapter 接口方法（高影响）

`renderTarget`（syncer.go:118-199）依次调用 `Detect`、`Plan`、`ManagedPaths`、`DetectConflicts`、`BackupManagedPaths` 和 `Render`。读者需要理解完整的 adapter 生命周期才能跟随此函数。每个调用都有自己的错误处理和结果合并逻辑。如果 adapter 生命周期被文档化（即使只是注释）或阶段被命名，函数会更清晰。

### 2. `map[string]any` 的 merge 逻辑是递归的且无深度限制（低影响）

```go
// internal/config/merge.go:141-156
func mergeAnyMap(base, override map[string]any) map[string]any {
    ...
    merged[key] = mergeAnyMap(baseMap, overrideMap)  // 递归
    ...
}
```

`RuntimeOverrides` 是 `map[string]any`，merge 递归进入嵌套 map 且无限制。对于当前用例（runtime 特定的配置覆盖），这可能没问题，但没有对病态输入的防护。最大深度或关于预期深度的注释会帮助未来的维护者。

### 3. `readAgentPreferProject` 级联回退（低影响）

```go
// internal/config/resolve.go:118-137
func readAgentPreferProject(name, cwd string) (*AgentProfile, string, error) {
    ...
    agent, err = ReadAgent(name, ScopeProject, cwd)
    if err == nil { return ... }
    if !os.IsNotExist(err) { return nil, "", err }
    agent, err = ReadAgent(name, ScopeGlobal, cwd)
    if err == nil { return ... }
    if os.IsNotExist(err) { return nil, "", fieldError(...) }
    return nil, "", err
}
```

级联的 `IsNotExist` 检查是惯用的 Go，但需要仔细阅读才能区分"未找到，尝试下一个"、"找到但无效"和"任何地方都未找到"。顶部的简短注释（"先尝试 project scope，回退到 global"）可以减少认知负担。

## 改进建议

按影响从高到低排序：

1. **将 `SyncActivation` 提取为命名阶段** — 将 85 行的编排器拆分为 `prepareInputs`、`syncTargets`、`persistState`。这是最大的可读性和可测试性改进。

2. **文档化基于 hash 的冲突契约** — 在 `conflict.go` 中添加块注释，解释 `ManagedHash` vs `FileHash` 何时被填充，以及为什么需要回退逻辑。这是代码库中最危险的隐式契约。

3. **消除 Write 函数中的双重验证** — 在路径已知后验证一次。当前模式令人困惑且浪费。

4. **合并 `writeYAML` 和去重辅助函数** — 要么通过内部工具包共享，要么重命名 sync 本地版本以避免名称冲突。合并 `uniqueStrings` 和 `uniqueNonEmptyStrings`。

5. **将 `ManagedPathStates`/`MappingStates` 从 `state` 包移出** — 这些 adapter 到 state 的转换器属于 `sync`，以保持 `state` 不依赖 `adapter`。

6. **为枚举类验证值定义常量** — `"prompt"`、`"overwrite"`、`"local"`、`"remote"`、`"primary"` 等应该是命名常量，而非散落的字符串字面量。

7. **裁剪未使用的模型字段** — 用 `map[string]struct{}` 替换 `TargetCapability`。如果 `LifecycleHooks`、`IOContract`、`AgentIdentity` 是计划中的，添加 `// TODO` 注释，否则删除以减少模型表面积。

8. **记录或包装 `rebuildActive` 中的回滚错误** — 失败时静默的 `_ = os.Rename(prevDir, activeDir)` 是数据丢失风险。
