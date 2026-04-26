# 可读性与可维护性审查：CLI 层

## 总结

CLI 层对于这个规模的项目来说结构良好——命令很薄，业务逻辑正确地委托给 `internal/` 包，cobra 接线很干净。主要问题是：`agent.go` 已经膨胀为一个 500 行的杂物文件，混合了命令定义、preview 渲染和共享工具函数；10 个调用点重复了 `os.Getwd()` + 错误检查样板代码；多字段排序比较器在四个地方逐字重复。没有函数超过 69 行，因此没有严重的长度违规，但有几个文件承担了与其名称不匹配的职责。

## 函数长度分析

没有函数超过 100 行。超过 50 行的函数：

| 函数 | 文件 | 行数 | 备注 |
|------|------|------|------|
| `runAgentCreate` | `agent.go:107` | 69 | 顺序提取 flag；长但线性 |
| `runEnvCreate` | `env.go:53` | 51 | 两条代码路径（local vs global）；边界 |
| `printStatusWithSyncState` | `status.go:92` | 51 | 过程式输出格式化；边界 |

三个都是顺序/过程式的，尽管长度较长但可读。都不是拆分的优先项。

## 过度设计发现

### 1. `agent.go` 是一个 500 行的多关注点文件 — 高影响

`agent.go` 包含：
- 5 个 YAML preview 类型（`agentMappingPreview`、`agentPreviewManagedPath` 等）
- 3 个命令构造器 + 3 个 RunE 处理器
- 2 个激活 preview 辅助函数（`resolvedActivationForAgentPreview`、`resolvedCapabilitiesForAgentPreview`）
- 6 个共享工具函数（`scopeFromFlag`、`encodeYAML`、`normalizeStringList`、`isKnownRuntime`、`parseMemoryRefs`、`firstNonEmptyString`、`uniqueSortedStrings`）

共享工具函数被 `agent.go` 本身和 `env.go`（间接通过 `isKnownRuntime`）消费，但它们位于名为 `agent.go` 的文件中，使其难以被发现。preview 类型和渲染逻辑（`previewManagedPaths`、`previewMappingGroups`、`mappingPreviewFromPlan`）仅被 `runAgentShow` 使用，可以放在单独的文件中。

### 2. `agentMappingPreviewRegistry` 接口仅为单个调用点定义 — 低影响

```go
// agent.go:18
type agentMappingPreviewRegistry interface {
    Get(runtime string) (adapter.Adapter, bool)
}
```

此接口仅为 `buildAgentMappingPreview` 存在。它复制了 `init_import_report.go:18` 中 `initAdapterRegistry` 的形状。两者都是具有相同签名的单方法接口。这对可测试性来说没问题，但值得注意为合并机会。

### 3. `resolvedActivationForAgentPreview` + `resolvedCapabilitiesForAgentPreview` — 中影响

这两个函数（agent.go:278-311）手动构建 `config.ResolvedActivation` 来供给 preview 管道。这是一条必须与 `internal/config` 中真正的 `config.ResolveActivation` 保持同步的并行构建路径。如果 `ResolvedActivation` 结构体增加了新的必需字段，这条 preview 路径将静默产生不完整的数据。没有编译时或测试时的漂移防护。

### 4. `notImplemented` 哨兵仅被一个调用点使用 — 低影响

```go
// commands.go:25
func notImplemented(cmd *cobra.Command, args []string) error {
    return fmt.Errorf("%s: not implemented", cmd.CommandPath())
}
```

仅从 `memory.go:44` 调用。这可能是为未来命令准备的脚手架。无害，但如果没有计划新的存根则是死代码。

## 可读性问题

### 1. `parseMemoryRefs` 使用位置化的冒号分隔解析但无文档 — 高影响

```go
// agent.go:451-484
func parseMemoryRefs(values []string) ([]config.MemoryRef, error) {
    // ...
    parts := strings.Split(value, ":")
    if len(parts) > 4 {
        return nil, fmt.Errorf("invalid memory ref %q", value)
    }
    id := strings.TrimSpace(parts[0])
    scope := string(config.ScopeProject)
    // ...
    if len(parts) > 1 && strings.TrimSpace(parts[1]) != "" {
        scope = strings.TrimSpace(parts[1])
    }
    if len(parts) > 2 {
        path = strings.TrimSpace(parts[2])
    }
    if len(parts) > 3 && strings.TrimSpace(parts[3]) != "" {
        mode = strings.TrimSpace(parts[3])
    }
```

`id:scope:path:mode` 格式未在 CLI 帮助文本或注释中记录。读者必须逆向工程位置语义。parts[1]（检查空值）和 parts[2]（不检查空值）之间的不一致增加了困惑。

### 2. 多字段排序比较器逐字重复 — 中影响

四个排序块使用相同的链式 `if a != b { return a < b }` 比较模式：

- `agent.go:334-344` — `previewManagedPaths` 按 Path/Owner/MergeMode 排序
- `agent.go:350-363` — `previewMappingGroups` 按 SourcePath/TargetPath/Status/Reason 排序
- `status.go:182-190` — `printManagedPaths` 按 Path/Owner/MergeMode 排序
- `status.go:202-213` — `printMappings` 按 SourcePath/TargetPath/Status/Reason 排序

`previewManagedPaths` 和 `printManagedPaths` 的排序在结构上完全相同（相同字段、相同顺序）但操作不同类型。mapping 排序同理。这是一个维护陷阱——如果排序顺序需要改变，四个地方都必须更新。

### 3. `applyActivation` 在 `syncResult` 非 nil 时静默吞掉 sync 错误 — 中影响

```go
// use_activation.go:130-139
syncResult, err := syncer.SyncActivation(context.Background(), resolved, syncOpts)
if syncResult == nil {
    return nil, err
}
if !syncOpts.DryRun {
    if currentErr := writeCurrentActive(syncResult.Active); currentErr != nil {
        return nil, currentErr
    }
}
return activationResultFromSync(syncResult), err
```

当 `syncResult != nil` 且 `err != nil` 时，error 与 result 一起返回。`runUse`（use.go:29-34）中的调用者在 `applyActivation` 之后执行 `if err != nil { return err }`，因此 result 被丢弃。但 `runSync`（sync.go:58-67）处理方式不同——它在 err 非 nil 时仍使用 result 并设置 `SyncStatus = "failed"`。这种不一致意味着同一个激活管道根据调用命令的不同而表现不同。

### 4. 魔法字符串 `"default"` 出现在多个文件中但无常量 — 低影响

字符串 `"default"` 在以下位置用作默认 profile/env 名称：
- `init.go:85`（active ref 名称）
- `init.go:109`（agent profile 名称）
- `init.go:121`（environment 名称）
- `deactivate.go:25`（硬编码回退）
- `env.go:120`（默认 runtime agent）

一个 `const defaultProfileName = "default"` 可以使意图更清晰并防止拼写错误导致的 bug。

## 可维护性问题

### 1. `os.Getwd()` 样板代码重复 10 次 — 高影响

每个命令处理器都以此开头：
```go
cwd, err := os.Getwd()
if err != nil {
    return err
}
```

出现在：`runAgentCreate`、`runAgentList`、`runAgentShow`、`runDeactivate`、`runExport`、`runEnvCreate`、`runImport`、`runSync`、`runStatus`、`runUse`。

root command 上的 `PersistentPreRunE` 可以解析一次 `cwd` 并存储在 cobra context 或共享结构体中，消除 10 个相同的错误处理块。

### 2. 共享工具函数埋在 `agent.go` 中 — 中影响

跨多个命令文件使用的函数：
- `scopeFromFlag` — 仅在 `agent.go` 中使用但是通用的 flag 辅助函数
- `encodeYAML` — 仅在 `agent.go` 中使用但是通用的输出辅助函数
- `isKnownRuntime` — 在 `agent.go` 中使用但与 `env.go` 和 `sync.go` 相关
- `formatActiveRef` — 定义在 `use_activation.go`，被 `status.go` 和 `sync.go` 使用
- `writeCurrentActive` — 定义在 `use_activation.go`，被 `sync.go` 使用
- `syncStatePath` — 定义在 `use_activation.go`，被 `init.go` 和 `status.go` 使用

这些横切辅助函数应该放在 `helpers.go` 或 `common.go` 文件中。目前，开发者寻找 `syncStatePath` 时不会直觉地去看 `use_activation.go`。

### 3. `sync.go` 复制了 `use_activation.go` 的激活逻辑 — 中影响

`runSync`（sync.go:24-68）手动构建 sync 选项、调用 `syncer.SyncActivation`、调用 `writeCurrentActive` 和 `activationResultFromSync`——大部分复制了 `use_activation.go:123-140` 中 `applyActivation` 的功能，但错误处理略有不同且增加了 `--target` 过滤。不同的错误处理（见可读性问题 #3）是这种重复的直接后果。

### 4. 添加新 CLI 命令需要修改 `commands.go` — 低影响

这是标准的 cobra 实践，不是真正的问题。`commands.go` 中的 `addCommands` 函数是一个干净的注册表。新命令需要：(1) 一个包含 `newXCommand()` 的新文件，(2) `addCommands` 中的一行。这很好。

## 认知负担热点

### 1. 理解 `avm use` 端到端需要 4 个文件 — 中影响

`avm use` 流程涉及：
1. `use.go` — 命令定义和 `runUse`（薄层）
2. `use_activation.go` — 解析、应用、sync 编排、输出格式化、状态写入
3. `internal/config` — `ResolveActivation`（真正的解析逻辑）
4. `internal/sync` — `SyncActivation`（真正的 sync 逻辑）

对于涉及的复杂度来说，这实际上是合理的深度。`use.go` 和 `use_activation.go` 之间的分离很清晰：`use.go` 是 cobra 胶水，`use_activation.go` 是与 `deactivate.go` 和 `sync.go` 共享的激活领域逻辑。命名可以更好——`activation.go` 比 `use_activation.go` 更容易被发现。

### 2. `agent.go` 混合了三个抽象层次 — 中影响

`agent.go` 的读者会遇到：
- CLI 接线（命令构造器、flag 解析）
- 领域逻辑（preview 构建、preview 的激活解析）
- 工具函数（YAML 编码、字符串规范化、scope 解析）

这三个关注点需要不同的心智模型。如果拆分为 `agent.go`（命令）、`agent_preview.go`（preview 类型 + 渲染）和 `helpers.go`（共享工具函数），文件会更容易导航。

### 3. `deactivate` 实际上是"激活 default" — 低影响

```go
// deactivate.go:24-27
resolved, err := resolveActivationRef(config.ActiveRef{
    Kind: config.ActiveKindProfile,
    Name: "default",
}, cwd)
```

`deactivate` 命令实际上并不停用任何东西——它激活"default" profile。这是一个语义上的意外。一条解释设计选择的注释会有帮助，或者命令的 `Long` 描述应该澄清此行为。

## 改进建议

按影响从高到低排序：

1. **将共享工具函数提取到 `helpers.go`** — 将 `scopeFromFlag`、`encodeYAML`、`normalizeStringList`、`isKnownRuntime`、`formatActiveRef`、`writeCurrentActive`、`syncStatePath`、`currentActivePath`、`firstNonEmptyString`、`uniqueSortedStrings` 移到专用文件。这是纯移动，无行为变更，立即提高可发现性。

2. **消除 `os.Getwd()` 样板代码** — 在 root command 上添加 `PersistentPreRunE` 来解析 `cwd` 并存储在命令 context 中。每个处理器用一行辅助函数获取，而不是重复 3 行错误块。

3. **拆分 `agent.go`** — 将 preview 类型和渲染提取到 `agent_preview.go`（~170 行）。这将 `agent.go` 缩减到 ~330 行，并给 preview 关注点一个清晰的归属。

4. **统一 sync 错误处理** — 让 `runSync` 使用 `applyActivation`（或接受 target 覆盖的变体），而不是复制 sync-then-write-state 序列。这消除了 `use` 和 `sync` 之间不同的错误处理。

5. **记录 memory ref 格式** — 在 `parseMemoryRefs` 上方添加注释，并更新 `--memory` flag 帮助文本以显示 `id:scope:path:mode` 格式及默认值。

6. **将 `use_activation.go` 重命名为 `activation.go`** — 该文件被 `use.go`、`deactivate.go`、`sync.go` 和 `status.go` 使用。`use_` 前缀暗示它仅属于 `use` 命令。

7. **添加 `const defaultProfileName`** — 用命名常量替换 5 处裸 `"default"` 字符串。
