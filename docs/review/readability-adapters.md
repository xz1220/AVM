# 可读性与可维护性审查：Adapter 层

## 总结

Adapter 层结构良好，具有清晰的接口和所有四个真实 adapter 之间一致的模式。主要问题是 claude/codex/cursor/cline 之间 ~500 行逐字重复的工具函数，以及 `mappings()` 方法已膨胀到 130-189 行的重复条件字段映射声明。RenderPlan 中间表示和 FieldMapping 系统有真实消费者，证明了其存在价值，但 MergeMode/ManagedPath 系统携带了未使用的变体。

## 函数长度分析

超过 50 行的函数（按严重程度排序）：

| 函数 | 文件 | 行数 | 行范围 |
|------|------|------|--------|
| `renderContext.mappings()` | cline/cline.go | 189 | L478-L666 |
| `renderContext.mappings()` | codex/codex.go | 167 | L364-L530 |
| `renderContext.mappings()` | claude/claude.go | 129 | L373-L501 |
| `mergeMCPSettings()` | cline/cline.go | 95 | L717-L811 |
| `Adapter.Plan()` | 所有四个真实 adapter | 73-74 | 各异 |
| `renderContext.mappings()` | cursor/cursor.go | 61 | L313-L373 |
| `Adapter.Render()` | codex/codex.go | 59 | L165-L223 |
| `Adapter.Import()` | cline/cline.go | 58 | L81-L138 |
| `renderContext.renderRulesFile()` | cline/cline.go | 53 | L357-L409 |

`mappings()` 方法是最严重的违规者。每个都是一长串 `if field != zero { append mapping }` 块，没有结构变化——只是不同的字符串字面量。

## 过度设计发现

### 1. `skillLines()` 和 `capabilityLines()` 是完全相同的函数 — 高影响

在 `claude/claude.go`（L890-L920）和 `codex/codex.go`（L838-L868）中，`skillLines()` 和 `capabilityLines()` 是逐字节相同的函数，只是名称不同。两者都接受 `[]adapter.CapabilityRef` 并以相同逻辑产生 `[]string`：

```go
func skillLines(refs []adapter.CapabilityRef) []string {
    // ... 完全相同的函数体 ...
}

func capabilityLines(refs []adapter.CapabilityRef) []string {
    // ... 完全相同的函数体 ...
}
```

这是同一文件内的纯重复。一个名称清晰的函数就够了。

### 2. `MergeModeMarkedBlock` 已声明但无 adapter 使用 — 中影响

`adapter.go` L206 声明：
```go
MergeModeMarkedBlock MergeMode = "marked-block"
```

没有 adapter 在 `ManagedPath` 上设置 `MergeMode: adapter.MergeModeMarkedBlock`。codex adapter 内部使用 marked-block 逻辑（`mergeMarkedBlock`），但将其 managed path 声明为 `MergeModeStructuredSection`。`internal/sync/conflict.go` L121 中唯一的消费者检查它，但没有 adapter 产生它。这是死抽象。

### 3. `MemoryImportCapable` 接口只有一个实现者 — 低影响

在 `adapter.go` L20-23 声明，只有 `fake.Adapter` 实现了 `ImportMemory()`。没有真实 adapter 实现它，测试之外也没有调用者使用它。这是前瞻性设计，目前增加了接口表面积但无生产价值。

### 4. `OperationEnsureDir`、`OperationRemoveFile`、`OperationMergeSection` 未被真实 adapter 使用 — 中影响

在 `adapter.go` L184-190 声明：
```go
OperationEnsureDir     RenderAction = "ensure_dir"
OperationMergeSection  RenderAction = "merge_section"
OperationRemoveFile    RenderAction = "remove_file"
```

只有 fake adapter 处理 `EnsureDir` 和 `RemoveFile`。没有真实 adapter 产生或消费 `MergeSection`。这些是推测性的操作类型，膨胀了操作模型。

### 5. FieldMapping 系统有其价值但 `mappings()` 方法过于冗长 — 中影响

四种 mapping 状态（`native`、`rendered_as_instructions`、`ignored`、`unsupported`）被 `cmd/avm/agent.go` L378-384 和 `internal/state/types.go` L76 消费，因此系统证明了其存在价值。然而，每个 adapter 的 `mappings()` 方法是 60-189 行几乎相同的条件追加块。模式始终是：

```go
if len(r.input.SomeField) > 0 {
    mappings = append(mappings, adapter.FieldMapping{
        SourcePath: "some.path",
        TargetPath: targetX,
        Status:     adapter.MappingRenderedAsInstructions,
        Reason:     "<runtime> Phase 1 does not support <feature>.",
    })
}
```

这可以改为声明式表驱动，将每个 `mappings()` 方法减少 50-70%。

### 6. RenderPlan 中间表示有其价值 — 无需操作

Plan/Render 分离被 sync 层（`internal/sync/syncer.go` L151, L194）、CLI preview（`cmd/avm/agent.go` L313）和状态持久化（`internal/state/types.go`）消费。两阶段设计使得在写入前可以进行 dry-run preview 和冲突检测。这个抽象证明了其存在价值。

## 可读性问题

### 1. 4 个 adapter 之间大量复制粘贴的工具函数 — 高影响

以下函数在 claude.go、codex.go、cursor.go 和 cline.go 之间逐字（或近乎逐字）复制：

| 函数 | 每份行数 | 份数 | 总重复行数 |
|------|---------|------|-----------|
| `writeFileAtomic` | 35 | 4 | 105（3 份冗余） |
| `slug` | 23 | 4 | 69 |
| `memoryRefLines` | 25 | 3 | 50 |
| `portableMemoryLines` | 25-28 | 3 | 50 |
| `skillLines` | 15 | 2 | 15 |
| `capabilityLines` | 15 | 3 | 30 |
| `toolsetLines` | 15 | 3 | 30 |
| `sortedMCPServers` | 7 | 4 | 21 |
| `mcpServerRenderable` | 3 | 4 | 9 |
| `sortedStrings` | 5 | 4 | 15 |
| `firstNonEmpty` | 8 | 4 | 24 |
| `managedPathIndex` | 7 | 3 | 14 |
| `section` | 3 | 3 | 6 |
| `bulletSection` | 10 | 3 | 20 |
| `writeLine` | 4 | 3 | 8 |
| `marshalJSON` | 9 | 2 | 9 |

总计：大约 475 行重复的工具代码。claude.go 中的 `portableMemoryLines` 有一个微妙的差异（它追加 `item.Content`），这使得重复变得积极危险——一个副本中的 bug 修复可能不会传播。

### 2. `_ = ctx` 模式到处重复 — 低影响

每个接收 `ctx adapter.Context` 但不使用它的方法都以 `_ = ctx` 开头。这在代码库中出现 20+ 次。虽然它抑制了未使用变量的警告，但它是视觉噪音。方法可以简单地在参数名中使用 `_`。

### 3. 魔法字符串 "Phase 1" 在 reason 字符串中出现 50+ 次 — 低影响

每个 `FieldMapping.Reason` 和警告字符串都包含 "Phase 1" 作为限定词。当 Phase 2 到来时，所有四个 adapter 中的每个 reason 字符串都需要更新。一个常量或模板可以集中化这一点。

## 可维护性问题

### 1. 添加新 adapter 需要复制 ~200 行工具代码 — 高影响

要添加一个新 adapter，开发者今天必须：
1. 在 `internal/adapter/<name>/` 下创建新包
2. 实现 5 方法的 `Adapter` 接口（Name、Detect、Import、Plan、Render、ManagedPaths）
3. 从现有 adapter 复制 `writeFileAtomic`、`slug`、`firstNonEmpty`、`sortedMCPServers`、`mcpServerRenderable`、`sortedStrings`、`managedPathIndex`
4. 复制所需的 `memoryRefLines`、`portableMemoryLines`、`capabilityLines`、`toolsetLines`、`section`、`bulletSection`
5. 通过从最接近的现有 adapter 复制并调整字符串来编写 `mappings()` 方法
6. 按照相同模式编写 `warnings()` 方法

步骤 3-4 是纯复制粘贴的维护负担。这些函数没有 adapter 特定的行为。

### 2. 接口变更的脆弱性是可接受的 — 低影响

如果 `adapter.go` 中的 `Adapter` 接口改变，恰好 5 个文件会中断（claude、codex、cursor、cline、fake）。对于一个 5 adapter 系统来说，这是合理的扇出。Go 编译器在构建时捕获所有中断。

### 3. 隐藏假设：所有 adapter 必须调用 `renderplan.Normalize()` — 中影响

每个 adapter 的 `Plan()`、`Render()` 和 `ManagedPaths()` 方法都调用 `renderplan.Normalize()`。这是一个仅通过约定存在的契约——接口或类型系统中没有任何东西强制它。如果新 adapter 忘记 normalize，sync 层可能看到非确定性排序。这应该被显著记录或通过包装器强制。

### 4. claude 和 codex/cline 之间 `portableMemoryLines` 的分歧 — 中影响

`claude/claude.go` L959-986 包含这个 codex 和 cline 省略的代码块：
```go
if item.Content != "" {
    line += ": " + strings.TrimSpace(item.Content)
}
```

这是隐藏在看起来相同的代码中的微妙行为差异。如果这些函数是共享的，差异会是显式的（例如 `withContent bool` 参数）。作为复制粘贴的代码，它是潜在的 bug 来源。

## 认知负担热点

### 1. adapter.go 在一个文件中定义了 25+ 类型 — 中影响

开发者打开 `adapter.go`（285 行）会遇到：`Adapter` 接口、`MemoryImportCapable` 接口、`Detection`、`ImportResult`、`ImportedAgent`、`RenderInput`、`ActiveRef`、`Agent`、`Instructions`、`ModelConfig`、`PermissionConfig`、`CapabilitySet`、`CapabilityRef`、`MCPServer`、`EnvVar`、`Toolset`、`MemoryRef`、`PortableMemory`、`RenderPlan`、`RenderOperation`、`RenderAction`（+ 5 个常量）、`ManagedPath`、`MergeMode`（+ 3 个常量）、`FieldMapping`、`MappingStatus`（+ 4 个常量）、`RenderResult`、`RenderOperationResult`、`MemoryImportOptions`、`MemoryImportPlan`、`MemoryDiff`、`MemoryDiffStatus`（+ 4 个常量）。

这是 ~30 个命名类型和 ~16 个常量。虽然每个都很小，但在编写 adapter 之前理解完整的类型词汇表的认知负担很大。拆分为 `adapter_types.go`（数据类型）、`adapter_render.go`（RenderPlan/Operation/Result 类型）和 `adapter.go`（仅接口）会有帮助。

### 2. 理解一个 adapter 需要阅读 700-1100 行 — 中影响

每个真实 adapter 是一个单文件，从 718 行（cursor）到 1128 行（cline）不等。开发者必须滚动整个文件才能理解 adapter，因为 `mappings()` 方法（最复杂的部分）埋在最深处。文件结构是：struct + options + 接口方法 + 辅助函数 + render context + render 方法 + mappings + warnings + 工具函数。没有目录或清晰的分节标记。

### 3. Plan -> Render -> ManagedPaths 管道需要理解 5 个类型 — 低影响

要追踪一个 sync 操作，开发者必须理解：`RenderInput` -> `RenderPlan`（包含 `RenderOperation`、`ManagedPath`、`FieldMapping`）-> `RenderResult`（包含 `RenderOperationResult`）。对于问题域来说，这是合理的类型数量。

## 改进建议

### 优先级 1：提取共享工具函数（高影响、低风险）

创建 `internal/adapter/adapterutil/`（或直接添加到 `adapter` 包）包含：
- `writeFileAtomic`
- `slug`
- `firstNonEmpty`
- `sortedMCPServers`
- `mcpServerRenderable`
- `sortedStrings`
- `managedPathIndex`
- `marshalJSON`
- `memoryRefLines`
- `portableMemoryLines`（为 claude 变体提供 `WithContent` 选项）
- `capabilityLines`（替换 `skillLines` 和 `capabilityLines`）
- `toolsetLines`
- `section`、`bulletSection`、`writeLine`

这消除了 ~475 行重复，并使 adapter 之间的行为差异显式化。

### 优先级 2：将 `mappings()` 转换为声明式表（中影响、中风险）

用表驱动方法替换 60-189 行的 `mappings()` 方法：

```go
type mappingRule struct {
    Source    string
    Target    string
    Status    adapter.MappingStatus
    Reason    string
    Condition func(adapter.RenderInput) bool
}
```

每个 adapter 定义一个 `[]mappingRule` 切片，共享函数迭代它。这将每个 adapter 减少 50-100 行，并使 mapping 逻辑可扫描。

### 优先级 3：删除未使用的 RenderAction 和 MergeMode 变体（低影响、低风险）

删除 `OperationEnsureDir`、`OperationRemoveFile`、`OperationMergeSection` 和 `MergeModeMarkedBlock`，直到它们有真实消费者。它们可以在未来阶段重新添加。死抽象会误导读者以为他们需要处理从未发生的情况。

### 优先级 4：在边界强制 `renderplan.Normalize()`（中影响、低风险）

要么在 sync 层用规范化装饰器包装 `Adapter` 接口，要么给 `RenderPlan` 添加 `Validate()` 方法供 sync 层调用。这消除了每个 adapter 都必须记住 normalize 的隐式约定。

### 优先级 5：删除与 `capabilityLines` 重复的 `skillLines`（低影响、无风险）

在 `claude/claude.go` 和 `codex/codex.go` 中，`skillLines` 和 `capabilityLines` 完全相同。删除 `skillLines` 并在所有地方使用 `capabilityLines`。
