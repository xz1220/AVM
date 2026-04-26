# 编排与状态审查 (internal/sync, state, backup, memory, packageio)

## 总结

编排和状态管理包结构良好，关注点分离清晰，原子写入模式（temp-file + rename）一致使用，全面的防御性编程。最显著的风险是：(1) 整个 sync 流程无并发保护，意味着并行的 `avm sync` 调用可能损坏状态或产生部分写入；(2) packageio 中对 zip 条目的无限制 `io.ReadAll` 创建了 zip bomb / 内存耗尽向量；(3) 冲突检测中的 `hashDir` 函数永远无法检测 symlink，因为 `filepath.WalkDir` 不会通过 `entry.Info()` 报告 symlink。sync/state/memory 的正常路径和基本失败模式测试覆盖良好，但 backup 和 packageio 没有测试文件。

## 严重问题

### C1. 包导入中的 zip bomb / 内存耗尽 (packageio/import.go:153)

`readZip` 对每个 zip 条目调用 `io.ReadAll(rc)` 且无大小限制。一个精心构造的 `.avm` 包如果包含多 GB 的解压条目将耗尽内存。

```go
// import.go:153
data, readErr := io.ReadAll(rc)
```

建议：使用 `io.LimitReader(rc, maxFileSize)` 并在达到限制时失败。对配置文件来说，每个条目 10-50 MB 的上限是合理的。

### C2. sync 状态无并发保护 (state/store.go, sync/syncer.go)

sync 流程中没有文件锁、建议锁或 pid 文件机制。两个并发的 `avm sync` 调用将：
- 都读取相同的 `sync-state.json`，各自覆盖对方的 runtime 状态条目。
- 都调用 `rebuildActive`，在 active 目录的 rename 上产生竞争。
- 都调用 `BackupManagedPaths` 和 `Render`，可能同时写入相同的 managed path。

`SaveSyncState`（store.go:60-80）中的原子 temp-file-then-rename 模式保护单个进程免受部分写入，但不保护并发进程。最后一个写入者获胜并静默丢弃另一个的状态更新。

建议：在进入 `SyncActivation` 之前，在状态目录中获取建议文件锁（例如通过 `syscall.Flock` 的 `flock` 或跨平台库）。返回时释放。

### C3. `hashDir` 中的 symlink 检测是死代码 (sync/conflict.go:175-180)

`filepath.WalkDir` 使用 `fs.DirEntry`，`entry.Info()` 返回目录条目本身的 `fs.FileInfo`。在大多数平台上，`WalkDir` 不跟随 symlink，也不会在遍历树内的条目的 `DirEntry.Info()` 结果上设置 `ModeSymlink`。第 180 行的 symlink 分支对于被 hash 的目录内的 symlink 永远不会被到达。这意味着目录 hash 静默忽略 symlink，managed 目录内的 symlink 变更不会被检测为冲突。

```go
// conflict.go:175-180
info, err := entry.Info()
// ...
case info.Mode()&os.ModeSymlink != 0:
    // 对于 WalkDir 内的 symlink，此分支不可达
```

建议：使用 `os.Lstat(path)` 代替 `entry.Info()` 来获取真实的文件模式，或检查 `entry.Type()&fs.ModeSymlink != 0`，这是在 `WalkDir` 回调中检测 symlink 的正确方式。

### C4. sync 中途失败时的部分写入状态 (sync/syncer.go:92-98)

如果 sync 循环处理多个 runtime，且第二个 runtime 的 `renderTarget` 在第一个已通过 `adp.Render` 写入文件后失败，系统将处于以下状态：
- 部分 runtime 已被渲染（文件已写入磁盘）。
- sync 状态未被持久化（第 102 行的 SaveSyncState 尚未运行）。
- 在下次 sync 时，冲突检测会看到已写入的文件但没有先前的 hash 记录，因此无法检测它们是否被外部修改。

active 目录已被重建（第 55 行），因此系统处于半 sync 状态，没有已写入内容的记录。

建议：要么 (a) 在每个成功的 runtime 渲染后增量持久化 sync 状态，要么 (b) 实现两阶段方法，先规划所有 runtime，然后全部渲染，失败时使用备份快照回滚。

## 重要问题

### M1. 原子写入中 rename 前无 fsync (state/store.go:69-80, packageio/export.go:423-430, packageio/import.go:305-315)

所有三个原子写入模式（SaveSyncState、writeZip、writeImportFile）都写入临时文件然后 rename，但都没有在关闭前调用 `file.Sync()`。在 `Close()` 和 `Rename()` 之间崩溃时，文件内容可能未持久化到磁盘，rename 可能指向空文件或部分文件。

```go
// store.go:77-80
if err := tmp.Close(); err != nil {
    return err
}
return os.Rename(tmpPath, path)
```

建议：在所有三个位置的 `tmp.Close()` 前添加 `tmp.Sync()`。

### M2. `copyFile` 在成功路径上双重 close (backup/backup.go:141-146)

第 141 行的 `defer out.Close()` 和第 146 行的显式 `return out.Close()` 意味着在成功路径上 `Close()` 被调用两次。第二次 close（来自 defer）会返回一个被静默丢弃的错误。虽然不是数据丢失 bug，但第一次 `Close()` 的错误才是写入持久性的关键，这种模式很脆弱。

```go
// backup.go:141,146
defer out.Close()
// ...
return out.Close()
```

这是一个常见的 Go 模式，因为对已关闭文件的延迟 close 返回无害错误，所以能正确工作。然而，使用代码库其他地方使用的显式-close-with-defer-remove 模式会更清晰。

### M3. backup 和 packageio 包零测试覆盖

`internal/backup/` 和 `internal/packageio/` 都不包含任何测试文件。这些包处理：
- 带 symlink 处理的文件复制（backup）
- Zip 归档创建和提取（packageio）
- 路径穿越防护（packageio）
- 密钥检测启发式（packageio）
- 导入时的冲突检测（packageio）

packageio 中的 `cleanPackagePath` 函数是防御 zip slip 攻击的主要手段，却没有单元测试。密钥检测启发式（`containsPlaintextSecret`）没有测试来验证其模式匹配。

建议：为两个包添加测试文件，至少覆盖：zip slip 路径穿越尝试、zip bomb 大小限制（添加后）、symlink/目录/普通文件的备份、密钥检测的真阳性/假阳性，以及导入冲突场景。

### M4. `RequireConfirmation` 始终被强制为 `true` (memory/import.go:255,265-266)

`buildCandidate` 函数在第 255 行无条件设置 `RequireConfirmation: true`，然后在第 265-266 行如果源文档这样说则再次条件性地设置为 `true`。源文档永远无法将其设置为 `false`。这似乎是 Phase 1 安全性的有意设计，但第 265 行的条件检查是死代码，因为值已经是 `true`。

```go
// import.go:255
RequireConfirmation: true,
// ...
// import.go:265-266（死代码 - 已经是 true）
if doc.WritePolicy.RequireConfirmation {
    candidate.WritePolicy.RequireConfirmation = true
}
```

建议：删除死条件或添加注释解释 Phase 1 的意图。

### M5. Active 目录重建在所有错误路径上未清理 `.prev` (sync/active.go:60-88)

如果第 75 行的 `os.Rename(tmpDir, activeDir)` 成功但第 84 行的 `os.RemoveAll(prevDir)` 失败，函数返回错误但新的 active 目录已就位。调用者看到错误但 active 目录已成功更新。`.prev` 目录作为孤儿被遗留。

```go
// active.go:83-86
if movedOld {
    if err := os.RemoveAll(prevDir); err != nil {
        return err  // Active 目录已更新，但调用者看到错误
    }
}
```

建议：对 `.prev` 清理失败记录警告而非返回错误，因为主操作已成功。

## 次要问题

### m1. `sha256String` 是冗余的 (sync/conflict.go:233-235)

`sha256String` 与 `sha256Bytes` 完全相同，没有附加价值：

```go
func sha256String(value []byte) string {
    return sha256Bytes(value)
}
```

建议：删除 `sha256String`，在第 126 行直接调用 `sha256Bytes`。

### m2. `hashPath` 将完整文件内容作为第二个返回值返回 (sync/conflict.go:134-159)

对于大文件，`hashPath` 将整个文件读入内存并与 hash 一起返回。对于 `MergeModeStructuredSection` 和 `hashManagedPath` 中的 default 分支，内容从未被使用——只使用了 hash。这对大型 managed path 来说是浪费的。

```go
// conflict.go:127-129
case adapter.MergeModeStructuredSection:
    return fileHash, fileHash, nil
default:
    return fileHash, fileHash, nil
```

建议：考虑两遍方法或对只需要 hash 的 merge 模式使用延迟内容加载。

### m3. `runtimeAgentNames` 中的非确定性 map 迭代 (sync/active.go:148-160)

`runtimeAgentNames` 迭代 `resolved.RuntimeAgents`（一个 map）来构建 names map。虽然输出 map 本身与顺序无关，但该函数从 `buildActiveTree` 调用，后者在 `sortedUniqueValues` 中使用结果。这没问题，但第 110 行 `buildActiveTree` 中 `resolved.RuntimeAgents` 的迭代顺序意味着 `safeActiveName` 验证检查顺序是非确定性的。不是 bug，但值得注意以获得可复现的错误信息。

### m4. `expandHome` 回退很脆弱 (packageio/export.go:527-529)

当 `os.UserHomeDir()` 失败时，回退从 `config.AvmDir()` 中去除 `/.avm`：

```go
home = strings.TrimSuffix(config.AvmDir(), string(filepath.Separator)+".avm")
```

这假设 `AvmDir()` 总是以 `/.avm` 结尾。如果 AVM 目录结构改变，这会静默产生错误的 home 目录。

### m5. Memory 导入测试使用相对路径 (memory/import_test.go:14)

```go
source := filepath.Join("..", "..", "testdata", "memory", "backend-standards.md")
```

这个相对路径使测试依赖于工作目录。如果从不同目录运行 `go test`，测试将失败。考虑使用 `runtime.Caller` 或 `os.Getwd` 来构建绝对路径。

### m6. `secretLikeKey` 模式匹配不完整 (packageio/export.go:581-590)

密钥检测只检查特定子字符串。它遗漏了常见模式如 `auth_token`、`access_key`、`client_secret`、`signing_key`、`encryption_key` 和 `credential`。该函数也不处理 SCREAMING_CASE 变体如 `API_KEY`，因为 key 在检查前被转为小写。

等等——key 确实在第 556 行被转为小写了。`contains` 检查对两种大小写都有效。然而，`api_key` 在转小写后不会匹配 `apiKey`（camelCase）。考虑也检查不带下划线的 `apikey`（已存在）和 `accesskey`、`authtoken`。

### m7. 备份快照目录使用纳秒时间戳 (backup/backup.go:34)

```go
snapshotDir := filepath.Join(backupRoot, now.UTC().Format("20060102T150405.000000000Z"), ...)
```

纳秒精度格式 `000000000` 创建了很长的目录名。如果两次 sync 在同一秒内发生（并发问题使这成为可能），它们会得到不同的目录，但纳秒精度对于人类可导航的备份目录来说过于精确。考虑使用秒或毫秒精度。

## 积极发现

1. **原子写入模式一致使用**：`SaveSyncState`、`writeZip`、`writeImportFile` 和 `rebuildActive` 都使用 temp-file-then-rename 模式。这是 POSIX 系统上崩溃安全的正确方法（除了上面提到的缺失 fsync）。

2. **Zip slip 防护全面**：packageio/import.go:358-373 中的 `cleanPackagePath` 拒绝绝对路径、`..` 穿越、反斜杠和不能通过 `path.Clean` 往返的路径。这是对 zip slip 攻击的可靠防御。

3. **Active 目录重建设计良好**：`rebuildActive` 函数（sync/active.go:28-89）使用三阶段方法（在临时目录中构建、将旧目录交换到 `.prev`、将临时目录 rename 为 active），如果最终 rename 失败则回滚。`cleanupTmp` defer flag 模式很简洁。

4. **冲突检测设计合理**：`DetectConflicts` 中的双 hash 方法（文件 hash + managed block hash）正确处理了 AVM 只管理文件一部分的情况。对未管理部分的外部更改不会触发误报冲突。

5. **dry-run 正确隔离**：`SyncActivation`（sync/syncer.go:54,101,170）和 `ImportDryRun`（memory/import.go:43-45）都正确地将所有写操作置于 dry-run 检查之后。memory 导入在 API 层面强制 Phase 1 仅支持 dry-run。

6. **全面的防御性 nil 检查**：`SyncActivation` 等函数在继续之前检查 nil context、nil resolved activation 和 nil registry。`renderTarget` 检查 nil adapter、nil plan 和 nil render result。

7. **导出中的密钥检测**：`containsPlaintextSecret` 函数（packageio/export.go:549-569）是一个深思熟虑的补充，防止在 registry 元数据文件中意外导出密钥。该启发式对第一版来说是合理的。

8. **sync 包的测试质量**：syncer 测试覆盖了正常路径、dry-run 隔离、冲突检测、缺失 adapter 处理和 active 重建失败保留。测试辅助函数（`testSyncer`、`testOptions`、`testResolved`）简洁且可复用。

9. **状态版本控制**：`StateVersion` 常量和 `SyncState` 中的 version 字段为未来的状态格式变更提供了迁移路径。
