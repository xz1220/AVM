# Adapter 层审查 (internal/adapter/)

## 总结

Adapter 层架构良好，具有清晰的接口契约、确定性的 render plan、原子文件写入，以及对所有四个生产 adapter（Claude Code、Codex、Cline、Cursor）加上一个 fake 测试 adapter 的全面测试覆盖。代码在用户自有文件保留和幂等渲染方面具有防御性。主要的结构性问题是 adapter 包之间大量的代码重复——大约 15 个工具函数被逐字复制粘贴到每个 adapter 中，且 Cursor adapter 的 MCP merge 缺少 Claude 和 Cline adapter 实现的所有权跟踪安全机制。

## 严重问题

### 1. Cursor MCP merge 静默覆盖用户自有 server

**文件：** `/Users/danielxing/code/agent-vm/internal/adapter/cursor/cursor.go:549-595`

Cursor 的 `mergeMCPServers` 函数无条件地将 AVM 管理的 MCP server 写入现有的 `.cursor/mcp.json`，而不跟踪哪些 server 是 AVM 拥有的。不像 Claude adapter（维护 `_avm.claude-code.managedMCPServers`）和 Cline adapter（维护 `_avm.managed_mcp_servers`），Cursor 没有所有权元数据。这意味着：

- 如果用户手动添加了一个名为 `github` 的 server，然后 AVM 渲染了一个 `github` server，用户的配置会被静默覆盖。
- 在后续渲染中，无法区分 AVM 管理的 server 和用户自有的 server，因此过时的 AVM server 永远不会被清理。

```go
// cursor.go:573-583 -- 覆盖前无所有权检查
for name, server := range payload.MCPServers {
    if name == "" {
        continue
    }
    raw, err := marshalJSON(server)
    if err != nil {
        return false, err
    }
    servers[name] = bytes.TrimSpace(raw)  // 无条件覆盖
}
```

Claude adapter 正确地拒绝冲突（`claude-code MCP server %q already exists and is not AVM-managed`），Cline adapter 发出警告并跳过。Cursor 两者都没做。

### 2. 文件写入路径中无 symlink 解析

**文件：** 所有 `writeFileAtomic` 实现（claude/claude.go:549, codex/codex.go:588, cline/cline.go:889, cursor/cursor.go:513）

没有任何 adapter 在写入前调用 `filepath.EvalSymlinks`。如果一个 managed path（例如 `.claude/agents/backend-coder.md`）是指向项目外部的 symlink，通过 `os.Rename` 的原子写入会跟随 symlink 并覆盖目标。当项目目录包含由其他工具或用户创建的 symlink 时，这是一个路径穿越风险。

Codex adapter 有 `cleanRenderPath` 和 `validateCodexManagedPath` 来验证路径在配置目录内，但此验证操作的是逻辑路径，而非解析后的物理路径。`~/.codex/agents/evil.toml -> /etc/something` 处的 symlink 会通过验证。

## 重要问题

### 3. 所有 adapter 之间大量的工具函数重复

**文件：** claude/claude.go, codex/codex.go, cline/cline.go, cursor/cursor.go

以下函数在 3-4 个 adapter 包中被复制粘贴（通常逐字符相同）：

| 函数 | claude | codex | cline | cursor |
|---|---|---|---|---|
| `writeFileAtomic` | :549 | :588 | :889 | :513 |
| `slug` | :1067 | :956 | :1097 | :687 |
| `firstNonEmpty` | :1091 | :980 | :1121 | :711 |
| `sortedStrings` | :1004 | :938 | :1037 | :650 |
| `sortedMCPServers` | :1010 | :944 | :1043 | :656 |
| `mcpServerRenderable` | :1038 | :952 | :1051 | :664 |
| `managedPathIndex` | :844 | :732 | :925 | :637 |
| `writeLine` | :852 | :787 | -- | :645 |
| `section` | :874 | :822 | :933 | -- |
| `bulletSection` | :878 | :826 | :937 | -- |
| `skillLines` | :890 | :838 | -- | -- |
| `capabilityLines` | :906 | :854 | :953 | -- |
| `memoryRefLines` | :933 | :870 | :969 | -- |
| `portableMemoryLines` | :959 | :896 | :995 | -- |
| `toolsetLines` | :988 | :922 | :1021 | -- |
| `marshalJSON` | :704 | -- | -- | :626 |

这大约是 400-500 行重复代码。一个共享的 `internal/adapter/adapterutil`（或类似）包可以消除这种重复，并确保 bug 修复传播到所有 adapter。`section` 和 `bulletSection` 函数在 Cline（使用 `## ` markdown 标题）和 Claude/Codex（使用纯 `title:\n` 格式）之间甚至有微妙不同的实现——这是有意的按 runtime 格式化，但相同的工具函数仍应共享。

### 4. Claude 和 Cline adapter 未对 managed path 进行所有权边界验证

**文件：** `/Users/danielxing/code/agent-vm/internal/adapter/claude/claude.go:200-214`
**文件：** `/Users/danielxing/code/agent-vm/internal/adapter/cline/cline.go:215-256`

Codex adapter 有 `validateCodexManagedPath`（codex.go:763），确保 managed path 在 `~/.codex/config.toml` 或 `~/.codex/agents/*.toml` 范围内。Claude 和 Cline adapter 没有等效的验证。它们的 `managedPathIndex` 函数（claude.go:844, cline.go:925）只是构建 map 而不检查路径是否在预期目录内。

传递给 `Render()` 的精心构造的 `RenderPlan` 可以包含像 `/etc/passwd` 这样的 managed path，只要操作路径匹配声明的 managed path，adapter 就会写入它。Codex adapter 会拒绝这种情况。

### 5. Registry 未暴露 adapter 列表或迭代功能

**文件：** `/Users/danielxing/code/agent-vm/internal/runtime/registry.go:11-33`

`Registry` 只有 `Get(runtime string)`。没有 `List()`、`All()` 或 `Names()` 方法。需要迭代所有 adapter 的调用者（例如 `avm detect --all` 或 `avm sync --all-runtimes`）必须硬编码 runtime 名称或维护一个并行列表。添加一个简单的 `Names() []string` 方法可以使 registry 自描述。

### 6. MCP server 冲突时错误与警告行为不一致

处理 MCP merge 的三个 adapter 对用户自有 server 名称冲突的处理方式不同：

- **Claude**（claude.go:616）：返回硬错误——`"claude-code MCP server %q already exists and is not AVM-managed"`。
- **Cline**（cline.go:770）：发出警告并跳过 server——`"cline mcp server %q already exists and is not AVM-managed; left unchanged"`。
- **Cursor**（cursor.go:573）：静默覆盖（见严重问题 #1）。

这种不一致意味着同一个 AVM profile 在 Cline 上成功渲染，在 Claude 上失败，在 Cursor 上静默损坏。行为应该一致——理想情况下所有 adapter 都警告并跳过，并提供强制选项。

## 次要问题

### 7. `skillLines` 和 `capabilityLines` 是完全相同的函数

**文件：** claude/claude.go:890 vs :906, codex/codex.go:838 vs :854

在 Claude 和 Codex adapter 中，`skillLines` 和 `capabilityLines` 有完全相同的实现。它们都接受 `[]adapter.CapabilityRef`，将每个格式化为 `name (path)`，并排序。一个名称清晰的函数就够了。

### 8. Fake adapter 未对操作进行 managed path 验证

**文件：** `/Users/danielxing/code/agent-vm/internal/adapter/fake/fake.go:190-223`

Fake adapter 的 `Render` 方法在应用操作时不检查它们是否针对 `ManagedPaths`。所有生产 adapter 都验证每个操作目标是声明的 managed path。Fake adapter 应该镜像此契约以保证测试保真度，否则使用 fake adapter 的测试可能通过了在真实 adapter 上会失败的 plan。

### 9. `MemoryImportOptions.DryRun` 字段格式不一致

**文件：** `/Users/danielxing/code/agent-vm/internal/adapter/adapter.go:257`

```go
type MemoryImportOptions struct {
    Runtime string `json:"runtime"`
    Source  string `json:"source,omitempty"`
    DryRun  bool   `json:"dry_run"`  // "bool" 前多了一个空格 -- 未对齐
}
```

`DryRun` 字段在 `bool` 前比其他字段多了一个空格。这是一个小的格式不一致（gofmt 不会捕获它因为这是合法的 Go，但它破坏了文件中其他地方使用的视觉对齐约定）。

### 10. `config_input.go` 未传播 `PortableMemory.Content`

**文件：** `/Users/danielxing/code/agent-vm/internal/adapter/config_input.go:154-165`

```go
func portableMemoryFromConfig(memory []config.PortableMemory) []PortableMemory {
    out := make([]PortableMemory, 0, len(memory))
    for _, item := range memory {
        out = append(out, PortableMemory{
            ID:    item.ID,
            Scope: item.Scope,
            Path:  item.Path,
            Mode:  item.Mode,
            // Content 未从 config.PortableMemory 复制
        })
    }
    return out
}
```

`PortableMemory` 的 `Content` 字段未从 config 层映射。如果 `config.PortableMemory` 有 `Content` 字段，它会被静默丢弃。这可能是有意的（内容延迟加载），但值得验证。

### 11. Codex `Detect` 冗余地同时 stat 目录和配置文件

**文件：** `/Users/danielxing/code/agent-vm/internal/adapter/codex/codex.go:54-65`

```go
found := false
if _, err := os.Stat(configDir); err == nil {
    found = true
}
if _, err := os.Stat(configPath); err == nil {
    found = true  // 冗余 -- 如果 configPath 存在，configDir 必然存在
}
```

如果 `configPath` 存在，`configDir` 必然存在。当第一个成功时，第二个 `os.Stat` 是冗余的。这无害但略有误导。

### 12. 测试辅助函数在测试文件间重复

**文件：** 所有 `*_test.go` 文件

`operationContent`、`assertMapping`、`operationChanged`、`readFile` 和 `richInput` 在 claude_test.go、codex_test.go、cline_test.go 和 cursor_test.go 之间重复。一个共享的 `internal/adapter/adaptertest` 包可以减少这种重复。

## 积极发现

- **确定性 render plan**：每个 adapter 通过 `renderplan.Normalize` 规范化 plan，每个测试套件都包含 `TestPlanIsDeterministic` 测试。这对可复现性和 diff 非常好。

- **原子文件写入**：所有四个生产 adapter 使用 temp-file-then-rename 进行写入，防止崩溃时的部分写入。文件权限显式设置为 0600。

- **幂等渲染**：每个 adapter 测试验证第二次 `Render` 调用报告 `Changed: false`。这对避免 watch/sync 循环中不必要的文件变动至关重要。

- **Managed path 边界强制**：Claude、Codex 和 Cline adapter 都拒绝针对声明的 managed 集合之外路径的操作。Codex adapter 通过 `validateCodexManagedPath` 进一步限制路径到已知安全位置。

- **用户配置保留**：Claude 和 Cline 的 MCP merge 逻辑仔细保留用户自有的 server 和环境变量引用（例如 `${GITHUB_TOKEN}` 永远不会被展开）。测试使用 `${DO_NOT_EXPAND}` 哨兵值显式验证这一点。

- **全面的字段映射文档**：每个 adapter 产生详细的 `FieldMapping` 条目，精确解释每个 AVM 字段如何映射（或不映射）到 runtime 的原生格式，附带人类可读的原因。这对 adapter 层来说异常全面。

- **清晰的接口设计**：`Adapter` 接口是最小化的（5 个方法 + Name），`MemoryImportCapable` 扩展接口是 opt-in 的，`RenderPlan` 中间表示清晰地分离了规划和执行。

- **深拷贝纪律**：`ManagedPaths` 返回副本，`cloneOperations` 深拷贝 `Content` 字节切片，`Normalize` 从不修改其输入。测试验证了这一点（`TestManagedPathsReturnsCopy`、`TestNormalizeDoesNotMutateInput`）。

- **Codex 中的 marked block 解析**：`markedBlockSpan` 函数（codex.go:686）处理所有格式错误的情况（缺少 begin、缺少 end、重复 block），附带清晰的错误信息和每种情况的测试。
