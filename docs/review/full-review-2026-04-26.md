# Agent VM 全量代码审查报告

> 日期：2026-04-26
> 范围：全代码库（~12,000 LOC Go 代码 + ~4,000 LOC 测试）
> 基准：main 分支 commit b9e8f48

## 1. 总体评估

AVM 是一个架构清晰的早期 Go CLI 项目。包边界合理，Cobra 用法规范，依赖极少（仅 cobra + yaml.v3）。核心设计模式一致：原子写入（temp+rename）、确定性输出（renderplan.Normalize + 排序迭代）、严格 YAML 解析（KnownFields + 单文档强制）。测试覆盖了主要正向路径和部分边界情况。

主要风险集中在三个方面：
1. 安全加固缺口（无大小限制的 I/O、无 symlink 防护、无并发锁）
2. Adapter 间行为不一致（MCP merge 策略三种不同实现）
3. 测试覆盖盲区（backup/packageio/runtime 三个包零测试，merge 逻辑未测试）

---

## 2. Critical 发现

### C1. packageio import 无 zip 条目大小限制

**位置：** `internal/packageio/import.go:153`

```go
data, readErr := io.ReadAll(rc)
```

`readZip` 对每个 zip 条目调用 `io.ReadAll`，无大小限制。恶意构造的 zip 文件（zip bomb）可包含压缩比极高的条目，解压后耗尽内存。

**影响：** 拒绝服务。用户执行 `avm import malicious.avm.zip` 时进程 OOM。

**修复建议：** 使用 `io.LimitReader(rc, maxEntrySize)` 包装，建议上限 10MB。同时校验 zip 条目的 `UncompressedSize64`。

---

### C2. 无并发保护，并行 sync 可竞争

**位置：** `internal/sync/syncer.go:36-127`、`internal/state/store.go`

整个 sync 流程无文件锁。两个并行的 `avm use` 或 `avm sync` 可以同时：
- 重建 active 目录（`active.go:46-89` 的三阶段 rename swap）
- 写入 runtime 配置文件
- 读写 `sync-state.json`

**影响：** 状态损坏、active 目录丢失、runtime 配置文件不一致。

**修复建议：** 在 sync 入口获取 `~/.avm/state/.lock` 文件锁（`syscall.Flock` 或 `os.OpenFile` + `LOCK_EX`），sync 完成后释放。

---

### C3. Adapter writeFileAtomic 写入前无 symlink 检查

**位置：** 4 个 adapter 的 `writeFileAtomic` 实现：
- `internal/adapter/claude/claude.go:594`
- `internal/adapter/codex/codex.go:639`
- `internal/adapter/cline/cline.go:889`
- `internal/adapter/cursor/cursor.go:513`

所有实现使用 `os.CreateTemp(dir, ...)` + `os.Rename(tmp, path)` 模式，但不检查目标 `path` 是否为 symlink。如果攻击者在 managed path 位置放置 symlink，`os.Rename` 会替换 symlink 本身（而非跟随），但 `os.ReadFile(path)` 在比较现有内容时会跟随 symlink 读取。

此外，只有 Codex adapter 通过 `validateCodexManagedPath`（`codex.go:954-982`）验证 managed path 在允许的目录范围内。Claude、Cline、Cursor 无等效检查。

**影响：** 路径穿越风险。在多用户环境或不可信项目目录下，managed path 可被重定向。

**修复建议：** 在 `writeFileAtomic` 开头添加 `filepath.EvalSymlinks` 检查，拒绝写入 symlink 目标。将 Codex 的路径边界验证模式推广到其他 adapter。

---

### C4. Cursor MCP merge 无所有权追踪，静默覆盖用户 server

**位置：** `internal/adapter/cursor/cursor.go:549-594`

```go
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

`mergeMCPServers` 对每个 desired server 直接写入 `servers[name]`，不检查该 name 是否已被用户手动配置。对比：
- Claude（`claude.go:670-672`）：检测到非 AVM-managed 的同名 server 时**报错**
- Cline（`cline.go:768-770`）：检测到时**警告并跳过**
- Cursor：**静默覆盖**

**影响：** 用户在 `.cursor/mcp.json` 中手动配置的 MCP server 会被 AVM 静默覆盖，且无法恢复（无 `_avm` metadata 追踪哪些是 AVM 管理的）。

**修复建议：** 为 Cursor adapter 添加 `_avm` ownership metadata，与 Claude/Cline 对齐。至少应在覆盖前发出 warning。

---

## 3. Major 发现

### M1. Sync 中途失败时的状态处理

**位置：** `internal/sync/syncer.go:97-121`

```go
for _, input := range inputs {
    // ...renderTarget 可能写入 runtime 文件...
    result.Targets = append(result.Targets, targetResult)
    syncState.Runtimes[input.Runtime] = runtimeStateFromTarget(...)
}
// ...
if syncErr == nil {
    cleanupStaleRuntimeSkills(...)  // 仅在全部成功时执行
}
state.SaveSyncState(opts.StatePath, syncState)  // 总是保存
```

如果第一个 runtime render 成功（文件已写入磁盘），第二个 runtime 失败：
- `cleanupStaleRuntimeSkills` 被跳过，可能留下 stale skill 文件
- `SaveSyncState` 仍然执行，但第一个 runtime 的 hash 已更新，下次 sync 不会检测到冲突

这不是数据丢失，但会导致 stale 文件残留和状态不一致。

**修复建议：** 考虑按 runtime 增量保存状态，或在部分失败时仍执行 stale cleanup（仅针对成功的 runtime）。

---

### M2. ResolvedActivation.Memory 字段从未被填充

**位置：** `internal/config/resolve.go:13`

```go
Memory map[string][]PortableMemory `yaml:"memory,omitempty" json:"memory,omitempty"`
```

`resolveProfileActivation` 和 `resolveEnvironmentActivation` 都不设置 `Memory` 字段。`adapter/config_input.go` 中 `portableMemoryFromConfig` 读取 `resolved.Memory[runtime]` 时总是得到 nil。

这意味着 adapter 的 `portableMemoryLines` 永远收到空列表，portable memory 内容不会被渲染到 runtime 配置中。

**影响：** Phase 1 的 portable memory 功能实际上未生效。Agent profile 中定义的 memory refs 不会传递到 adapter。

**修复建议：** 在 resolve 阶段填充 `Memory` 字段，或者如果这是 Phase 1 的有意限制，添加注释说明并在 Phase 3 实现。

---

### M3. mergeRuntimeOverrides 零测试覆盖

**位置：** `internal/config/merge.go:121-156`

`mergeRuntimeOverrides` 和 `mergeAnyMap` 实现了递归 deep merge，是 env override 的核心逻辑。但整个 `merge.go` 文件无直接测试覆盖。

`mergeAnyMap` 的递归 merge 对类型不匹配的情况（base 是 map 但 override 是 string，或反过来）直接用 override 替换 base，这可能不是所有场景下的期望行为。

**影响：** 未测试的 merge 逻辑是正确性风险。env override 可能在边界情况下产生意外结果。

**修复建议：** 为 `mergeRuntimeOverrides` 和 `mergeAnyMap` 添加单元测试，覆盖：nil base/override、类型不匹配、深层嵌套、空 map。

---

### M4. MCP merge 行为三个 adapter 各不相同

**位置：**
- Claude `claude.go:670-672`：冲突时 **error**
- Cline `cline.go:768-770`：冲突时 **warn + skip**
- Cursor `cursor.go:573-581`：**silent overwrite**

同一个 AVM profile 在不同 runtime 下对 MCP server 冲突的处理完全不同。用户无法预期行为。

**影响：** 用户困惑。在 Claude 下 `avm use` 失败的 profile，在 Cursor 下会静默覆盖用户配置。

**修复建议：** 统一为一种策略（建议 Cline 的 warn+skip 模式），或在 adapter contract 中明确定义 MCP 冲突策略接口。

---

### M5. ~475 行工具函数在 4 个 adapter 间复制粘贴

**位置：** `internal/adapter/{claude,codex,cline,cursor}/`

以下函数在 4 个 adapter 中逐字复制：

| 函数 | 行数 | 出现次数 |
|------|------|----------|
| `writeFileAtomic` | ~30 | 4 |
| `slug` | ~15 | 4 |
| `sortedStrings` | ~5 | 4 |
| `portableMemoryLines` | ~25 | 4（Claude 有差异） |
| `sortedMCPServers` | ~10 | 4 |
| `mcpServerRenderable` | ~5 | 4 |
| `firstNonEmpty` | ~5 | 4 |
| `section` / `bulletSection` | ~15 | 4 |
| `skillLines` / `capabilityLines` | ~15 | 4 |
| `memoryRefLines` / `toolsetLines` | ~15 | 4 |
| `managedPathIndex` | ~10 | 3（Codex 版本不同） |

其中 `portableMemoryLines` 在 Claude 版本中包含 `item.Content`（`claude.go:1217-1218`），其他版本不包含。这个差异隐藏在复制粘贴代码中，难以发现。

**影响：** 维护负担高。修改一个工具函数需要同步修改 4 处。隐藏的行为差异可能导致 bug。

**修复建议：** 提取到 `internal/adapter/adapterutil/` 共享包。`portableMemoryLines` 的 Claude 差异通过参数或选项显式化。

---

### M6. 4 个包无测试文件

| 包 | LOC | 风险 |
|----|-----|------|
| `internal/backup/` | 156 | 中 — 文件复制、symlink 处理 |
| `internal/packageio/` | 1054 | 高 — zip 处理、路径穿越防护、secret 检测 |
| `internal/runtime/` | 32 | 低 — 简单 map |
| `internal/version/` | 28 | 低 — 常量 |

`packageio` 是最大的无测试包，且包含安全敏感逻辑（zip 解析、`cleanPackagePath` 路径穿越防护、`containsPlaintextSecret` secret 检测）。

**修复建议：** 优先为 `packageio` 添加测试，覆盖 zip slip 防护、大文件拒绝、secret 检测、冲突处理。

---

## 4. Minor 发现

### m1. conflict.go hashDir 中 symlink 检测是死代码

**位置：** `internal/sync/conflict.go:180`

```go
case info.Mode()&os.ModeSymlink != 0:
```

`filepath.WalkDir` 的 `entry.Info()` 返回的是跟随 symlink 后的 `FileInfo`，永远不会报告 `ModeSymlink`。此分支永远不会执行。

注意：`hashPath`（`conflict.go:146`）使用 `os.Lstat` 正确检测 symlink，但 `hashDir` 内部的 walk 不行。

**修复建议：** 在 walk 回调中使用 `os.Lstat(path)` 替代 `entry.Info()`。

---

### m2. activationTargetExists 把权限错误当"存在"

**位置：** `cmd/avm/use_activation.go:116`

```go
if _, err := os.Stat(path); err == nil || !os.IsNotExist(err) {
    return true
}
```

当 `os.Stat` 返回权限错误时，`!os.IsNotExist(err)` 为 true，函数返回 true。后续错误信息显示 "could not be resolved" 而非 "not found"，对用户不够清晰。

**修复建议：** 单独处理 `os.IsPermission(err)` 情况，或在错误信息中包含具体原因。

---

### m3. deactivate 硬编码 profile:default 不检查存在性

**位置：** `cmd/avm/deactivate.go:24`

```go
resolved, err := resolveActivationRef(config.ActiveRef{
    Kind: config.ActiveKindProfile,
    Name: "default",
}, cwd)
```

如果用户删除了 default profile，`deactivate` 会失败并显示 resolve 错误，而非清晰的 "default profile not found, please create one" 提示。

**修复建议：** 在 resolve 前检查 default profile 是否存在，提供更友好的错误信息。

---

### m4. agent create 静默默认 codex runtime

**位置：** `cmd/avm/agent.go:121-122`

```go
if runtime == "" {
    runtime = "codex"
}
```

用户未指定 `--runtime` 时静默使用 codex，无任何提示。

**修复建议：** 打印一行提示 `using default runtime: codex`，或改为必填参数。

---

### m5. 所有运行时操作使用 context.Background()

**位置：**
- `cmd/avm/use_activation.go:130`
- `cmd/avm/sync.go:48`
- `cmd/avm/agent.go:221`
- `cmd/avm/init_import_report.go:51`

CLI 命令不传递 `cmd.Context()`，导致 Ctrl+C 信号无法取消正在进行的 sync 或 adapter 操作。

**修复建议：** 将 `context.Background()` 替换为 `cmd.Context()`。

---

### m6. 8+ 个 CLI 命令重复 os.Getwd() 样板

**位置：** `cmd/avm/` 下 10 处 `os.Getwd()` 调用（agent.go x3, status.go, use.go, sync.go, deactivate.go, env.go, export.go, import.go）

每处都是相同的 3 行模式：
```go
cwd, err := os.Getwd()
if err != nil {
    return err
}
```

**修复建议：** 在 root command 的 `PersistentPreRunE` 中获取 cwd 并存入 context，各命令从 context 读取。

---

### m7. readYAML 无文件大小限制

**位置：** `internal/config/yaml.go:15-16`

```go
file, err := os.Open(path)
// ...
decoder := yaml.NewDecoder(file)
```

直接打开文件并解码，无大小限制。恶意或损坏的 YAML 文件可能很大。

**影响：** 低 — config 文件通常由 AVM 自身写入，但 project override 可能来自不可信来源。

**修复建议：** 使用 `io.LimitReader(file, maxConfigSize)` 包装，建议上限 1MB。

---

### m8. 所有原子写入无 fsync

**位置：** `config/yaml.go:66-69`、`state/store.go:74-80`、所有 adapter `writeFileAtomic`

```go
if err := tmp.Close(); err != nil { ... }
if err := os.Rename(tmpPath, path); err != nil { ... }
```

`Close()` 和 `Rename()` 之间无 `tmp.Sync()` 调用。在断电场景下，rename 后的文件可能为空或截断。

**影响：** 极低 — CLI 工具断电场景罕见，且 sync-state 可重建。

**修复建议：** 在 `Close()` 前添加 `tmp.Sync()`，成本极低。

---

### m9. TargetStatusPartial / RuntimeStatusPartial 定义但从未赋值

**位置：**
- `internal/sync/types.go:28`：`TargetStatusPartial TargetStatus = "partial"`
- `internal/state/types.go:18`：`RuntimeStatusPartial RuntimeStatus = "partial"`

两个常量定义但无代码赋值。sync 循环只产生 `synced`、`skipped`、`failed`。

**修复建议：** 删除未使用的常量，或在部分成功场景中使用。

---

### m10. TargetCapability.Level 从未被读取

**位置：** `internal/config/models.go:18-20`

```go
type TargetCapability struct {
    Level string
}
```

`KnownTargets` map 的 value 包含 `Level` 字段（"full" 或 "partial"），但无代码读取 `Level`。`isKnownTarget` 仅检查 map 是否包含 key。

**修复建议：** 删除 `TargetCapability` struct，将 `KnownTargets` 改为 `map[string]bool` 或 `map[string]struct{}`。或者在 validation/status 中使用 `Level`。

---

### m11. writeCurrentActive 使用非原子写入

**位置：** `cmd/avm/use_activation.go:197-202`

```go
func writeCurrentActive(active config.ActiveRef) error {
    dir := filepath.Dir(currentActivePath())
    if err := os.MkdirAll(dir, 0o700); err != nil {
        return err
    }
    return os.WriteFile(currentActivePath(), []byte(formatActiveRef(active)+"\n"), 0o600)
}
```

使用 `os.WriteFile` 直接写入，不是 temp+rename 模式。与项目其他地方的原子写入约定不一致。

**修复建议：** 改用 temp+rename 模式，与 `config.writeYAML` 和 `state.SaveSyncState` 保持一致。

---

### m12. state/types.go 导入 adapter 包

**位置：** `internal/state/types.go:6`

```go
import "github.com/xz1220/agent-vm/internal/adapter"
```

`state` 包导入 `adapter` 包用于 `ManagedPathStates` 和 `MappingStates` 转换函数。自然的依赖方向应该是 `sync` -> `state` 和 `sync` -> `adapter`，而 `state` 应该是纯数据+持久化包。

**修复建议：** 将 `ManagedPathStates` 和 `MappingStates` 移到 `sync` 包（已同时导入两者）。

---

### m13. portableMemoryLines Claude 版本与其他 adapter 行为不一致

**位置：**
- Claude `claude.go` 的 `portableMemoryLines`：包含 `item.Content`
- Codex/Cline 版本：不包含 `Content`

Claude adapter 在渲染 agent 文件时会内联 memory 内容，其他 adapter 只渲染 memory 引用。这可能是有意设计（Claude Code 支持更丰富的 agent 文件格式），但差异隐藏在复制粘贴代码中。

**修复建议：** 如果是有意设计，提取到共享函数并通过参数控制。如果是 bug，统一行为。

---

## 5. Suggestion

### S1. skillLines 和 capabilityLines 完全相同

**位置：** `internal/adapter/claude/claude.go` 中两个函数实现完全一致。

**建议：** 删除其中一个，复用另一个。

---

### S2. runtime registry 与 config.KnownTargets 硬编码重复

**位置：**
- `internal/runtime/registry.go:14-22`：硬编码 4 个 adapter
- `internal/config/models.go:22-27`：硬编码 4 个 target

两处独立维护相同的 runtime 列表，修改一处可能遗漏另一处。

**建议：** 统一为单一来源，或添加测试断言两者一致。

---

### S3. secret 检测启发式较浅

**位置：** `internal/packageio/export.go:549-589`

`containsPlaintextSecret` 只检查少量 key pattern（token/secret/password/api_key/private_key），不覆盖 `auth_token`、`access_key`、`client_secret`、`jwt`、`bearer` 等常见模式。

**建议：** 扩展 pattern 列表，或使用更通用的正则匹配。

---

## 6. 积极发现

- **原子写入一致使用** — temp+rename 模式在 config、state、adapter、packageio 中统一使用
- **确定性输出** — `renderplan.Normalize` + 排序迭代确保稳定 hash 和可复现 CLI 输出
- **严格 YAML 解析** — `KnownFields(true)` + 多文档拒绝，尽早捕获 schema 漂移
- **zip slip 防护** — `cleanPackagePath` 拒绝绝对路径、`..` 穿越、反斜杠
- **深拷贝纪律** — `ManagedPaths` 返回副本，`cloneOperations` 深拷贝字节切片
- **幂等渲染** — 所有 adapter 测试验证第二次 render 报告 `Changed: false`
- **secret 不展开** — `${GITHUB_TOKEN}` 占位符保留，有测试断言
- **shell 注入防护** — `shell.go` 的 snippet 用严格正则验证 `current-active` 内容
- **Codex 路径边界验证** — 唯一验证 managed path 在允许目录范围内的 adapter
- **最小依赖** — 仅 cobra + yaml.v3，攻击面小

---

## 7. 优先级排序的修复建议

### 立即处理（共享使用前）

| # | 发现 | 工作量 |
|---|------|--------|
| 1 | C1 — packageio import 添加 `io.LimitReader` | 小 |
| 2 | C4 — Cursor MCP merge 添加所有权追踪 | 中 |
| 3 | C3 — writeFileAtomic 添加 symlink 检查 | 小 |
| 4 | M4 — 统一 MCP merge 冲突策略 | 中 |

### 短期（beta 前）

| # | 发现 | 工作量 |
|---|------|--------|
| 5 | C2 — sync 流程添加文件锁 | 中 |
| 6 | M6 — 为 packageio 添加测试 | 大 |
| 7 | M3 — 为 mergeRuntimeOverrides 添加测试 | 小 |
| 8 | M1 — 按 runtime 增量持久化 sync 状态 | 中 |
| 9 | M5 — 提取 adapter 共享工具函数 | 中 |
| 10 | m1 — 修复 hashDir symlink 检测 | 小 |

### 中期（质量打磨）

| # | 发现 | 工作量 |
|---|------|--------|
| 11 | M2 — 填充 ResolvedActivation.Memory 或明确标注 | 小 |
| 12 | m5 — context.Background() 替换为 cmd.Context() | 小 |
| 13 | m6 — 消除 os.Getwd() 样板 | 小 |
| 14 | m7 — readYAML 添加大小限制 | 小 |
| 15 | m8 — 原子写入添加 fsync | 小 |
| 16 | m11 — writeCurrentActive 改为原子写入 | 小 |
| 17 | m12 — state/types.go 转换函数移到 sync | 小 |

### 低优先级（清理）

| # | 发现 | 工作量 |
|---|------|--------|
| 18 | m9 — 删除未使用的 Partial 常量 | 小 |
| 19 | m10 — 删除未使用的 TargetCapability.Level | 小 |
| 20 | m2 — activationTargetExists 权限错误处理 | 小 |
| 21 | m3 — deactivate 检查 default profile 存在性 | 小 |
| 22 | m4 — agent create 默认 runtime 提示 | 小 |
| 23 | S1-S3 — 代码清理和启发式改进 | 小 |
