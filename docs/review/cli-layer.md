# CLI 层审查 (cmd/avm/)

## 总结

CLI 层结构良好，命令接线、激活逻辑和输出格式化之间有清晰的分离。Cobra 用法遵循惯用模式，错误信息对用户友好，测试套件覆盖了正常路径和重要的边界情况（冲突、缺失 profile、幂等导入）。有几个值得修复的 bug，一些关于全局进程状态的测试安全问题，以及收紧输入验证的小机会。

## 严重问题

### 1. `activationTargetExists` 在权限错误时返回 true（Bug）

**文件：** `cmd/avm/use_activation.go:116`

```go
if _, err := os.Stat(path); err == nil || !os.IsNotExist(err) {
    return true
}
```

当 `os.Stat` 返回权限错误（或任何非 `ENOENT` 错误）时，此表达式求值为 `true`，意味着函数报告目标"存在"，即使它无法被读取。这导致 `activationResolveError.notFound` 为 `false`，将用户看到的错误信息从清晰的 `"profile \"X\" not found"` 变为更模糊的 `"could not be resolved"` 信息。条件应改为：

```go
if _, err := os.Stat(path); err == nil {
    return true
}
```

### 2. `applyActivation` 在 `syncResult` 非 nil 时静默吞掉 sync 错误（Bug）

**文件：** `cmd/avm/use_activation.go:130-139`

```go
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

当 `SyncActivation` 同时返回非 nil 的 result 和非 nil 的 error 时，`writeCurrentActive` 调用会继续执行，error 与 result 一起返回。然而，`runUse`（use.go:29-32）中的调用者在打印*之后*执行 `if err != nil { return err }`，这意味着即使在部分失败时激活也会被持久化到磁盘。对比 `runSync`（sync.go:58-67），它在这种情况下正确地设置了 `result.SyncStatus = "failed"`。`use` 命令路径没有将状态标记为失败，因此用户看到的是 `sync: completed`，即使 sync 部分失败了。

### 3. 测试修改全局进程状态（`os.Chdir`、`os.Setenv`）但无并行保护

**文件：** 所有使用 `chdir()` 和 `t.Setenv("HOME", ...)` 的 `*_test.go` 文件

每个调用 `chdir(t, project)` 的测试都会修改进程级别的工作目录。虽然目前没有测试调用 `t.Parallel()`，但这是一个潜在隐患：如果有人给这些测试添加 `t.Parallel()`，它们将在 `os.Chdir` 和 `HOME` 上产生竞争。`chdir` 辅助函数（agent_cli_test.go:172-186）在 `t.Cleanup` 中恢复原始目录，但并行子测试之间的清理顺序不能保证防止交错。

建议添加注释或 build tag 来记录这些测试不能并行运行，或者重构命令以接受工作目录参数而不是内部调用 `os.Getwd()`。

## 重要问题

### 4. `runSync` 在 `UpdateActive` 为 false 时仍写入 `current-active`

**文件：** `cmd/avm/sync.go:58`

```go
currentErr := writeCurrentActive(syncResult.Active)
```

sync 命令在其选项中设置 `UpdateActive: false`（第 51 行），表示不应更改全局配置的 active 引用。但它无条件调用 `writeCurrentActive`，写入状态文件。这是不一致的：`UpdateActive: false` 的意图是重新 sync 而不改变当前激活的内容，但状态文件被覆盖了。如果 sync 中途失败，状态文件现在反映的是尝试的 active 引用，即使配置并未更新。

### 5. `deactivate` 硬编码回退到 `profile:default` 但未检查其是否存在

**文件：** `cmd/avm/deactivate.go:24-27`

```go
resolved, err := resolveActivationRef(config.ActiveRef{
    Kind: config.ActiveKindProfile,
    Name: "default",
}, cwd)
```

如果用户删除了默认 profile（或从未运行 `init`），`deactivate` 会以令人困惑的解析错误失败，而不是给出清晰的信息如 `"cannot deactivate: default profile not found; run avm init"`。错误路径没有区分"停用目标缺失"和一般的解析失败。

### 6. `agent create` 静默默认使用 `codex` runtime

**文件：** `cmd/avm/agent.go:83-85`

```go
if runtime == "" {
    runtime = "codex"
}
```

当用户省略 `--runtime` 时，agent 会静默地以 `codex` 作为首选 runtime 创建。这个默认值未在 flag 帮助文本中记录（flag 描述仅为 `"preferred runtime for this agent profile"`）。用户可能不会意识到他们的 agent 目标是 codex。默认值应该在 flag 的用法字符串中记录，或者该 flag 应该是必需的。

### 7. `export` 命令中 `--output` 路径无输入验证

**文件：** `cmd/avm/export.go:28-31`

```go
func runExport(cmd *cobra.Command, name, output, kind string) error {
    if output == "" {
        return fmt.Errorf("%s: --output is required", cmd.CommandPath())
    }
```

`--output` flag 值直接传递给 `packageio.ExportPackage` 而无任何验证。用户可以传递 `--output /etc/something` 或 `--output ../../../sensitive-path.zip`。虽然这是一个 CLI 工具（用户控制自己的机器），但至少警告或验证输出路径在合理位置内，或至少确保以 `.avm.zip` 结尾以防止意外覆盖非包文件，是一个好的实践。

### 8. `import` 命令未验证文件扩展名

**文件：** `cmd/avm/import.go:13`

`Use` 字符串写的是 `import <file.avm.zip>`，但 `RunE` 处理器直接将 `args[0]` 传递给 `packageio.ImportPackage` 而不检查扩展名。用户运行 `avm import config.yaml` 会得到一个令人困惑的 zip 解析错误，而不是清晰的验证信息。

## 次要问题

### 9. `agent list` 使用 tab 分隔输出但未使用 tabwriter

**文件：** `cmd/avm/agent.go:154-157`

```go
fmt.Fprintln(out, "NAME\tSCOPE\tVERSION\tDESCRIPTION")
for _, agent := range agents {
    fmt.Fprintf(out, "%s\t%s\t%s\t%s\n", agent.Name, agent.SourceScope, agent.Version, agent.Description)
}
```

原始的 `\t` 字符在终端中会产生列不对齐的输出，除非通过 `column -t` 或类似工具管道处理。使用 `text/tabwriter` 可以为人类阅读产生正确对齐的输出。

### 10. 使用 `context.Background()` 而非 `cmd.Context()`

**文件：** `cmd/avm/use_activation.go:130`, `cmd/avm/sync.go:48`

Cobra 提供的 `cmd.Context()` 携带取消信号（例如来自 SIGINT）。使用 `context.Background()` 意味着如果用户在长时间 sync 期间按 Ctrl+C，sync 操作无法被优雅地取消。

### 11. 每个命令处理器中重复的 `os.Getwd()` 调用

**文件：** `agent.go:74`, `agent.go:144`, `agent.go:166`, `deactivate.go:20`, `env.go:59`, `export.go:32`, `import.go:21`, `status.go:25`, `sync.go:25`, `use.go:21`

十个命令处理器各自独立调用 `os.Getwd()`。这可以通过在 root command 上使用 `PersistentPreRunE` 将 cwd 存储在 cobra context 中来集中化，减少样板代码并确保一致的行为。

### 12. `memory import` 有令人困惑的守卫逻辑

**文件：** `cmd/avm/memory.go:43-51`

```go
if from == "" && !dryRun {
    return notImplemented(cmd, args)
}
if from == "" {
    return fmt.Errorf("%s: --from is required", cmd.CommandPath())
}
if !dryRun {
    return fmt.Errorf("%s: only --dry-run is implemented", cmd.CommandPath())
}
```

第一个条件（`from == "" && !dryRun`）返回"not implemented"，但第二个条件（`from == ""`）返回"--from is required"。当 `dryRun` 为 true 时，第一个守卫对 `from == ""` 的情况不可达，然后落入第二个守卫。逻辑可以工作但不必要地复杂。简化为先检查 `from`，再检查 `dryRun`，会更清晰。

### 13. `parseMemoryRefs` 使用冒号分隔格式但无文档

**文件：** `cmd/avm/agent.go:208-240`

`--memory` flag 接受 `id:scope:path:mode` 格式的值，最多 4 个冒号分隔的部分，但此格式未在 flag 帮助文本中记录（`"portable memory refs to attach"`）。用户在不阅读源代码的情况下无法发现此格式。

### 14. `envCreateRuntimeOrder` 是包级别的可变切片

**文件：** `cmd/avm/env.go:180`

```go
var envCreateRuntimeOrder = []string{"codex", "claude-code", "cline", "cursor"}
```

虽然目前未被修改，但这是一个可变的包级别变量。使用类似常量的模式（例如返回新切片的函数）会更安全，以防止意外修改。

### 15. Shell init 代码片段嵌入 `$HOME/.avm/state` 作为默认路径

**文件：** `cmd/avm/shell.go:53, 87, 118`

Shell 代码片段硬编码 `$HOME/.avm/state` 作为默认状态目录，以 `AVM_STATE_DIR` 作为覆盖。如果 Go 代码更改了默认状态目录路径（通过 `config.StateDir()`），shell 代码片段将不同步。建议从同一个事实来源生成默认路径，或至少添加注释说明这种耦合。

### 16. `memory_test.go` 使用相对路径作为测试 fixture

**文件：** `cmd/avm/memory_test.go:15`

```go
source := filepath.Join("..", "..", "testdata", "memory", "backend-standards.md")
```

这依赖于测试二进制文件的工作目录为 `cmd/avm/`，对于 `go test ./cmd/avm/` 来说是正确的但很脆弱。验收测试文件（`stage5_acceptance_test.go:411-418`）正确地使用 `runtime.Caller` 来计算绝对路径。memory 测试应遵循相同的模式以保持一致性和健壮性。

## 积极发现

- **清晰的命令接线：** `commands.go` + `addCommands` 模式保持 root command 精简，使添加新子命令变得简单。
- **一致的输出格式化：** 所有命令写入 `cmd.OutOrStdout()`，使它们完全可测试而无需捕获 os.Stdout。
- **良好的错误信息质量：** 面向用户的错误包含命令路径（例如 `"avm memory import: --from is required"`），清楚地表明哪个命令失败了。
- **全面的激活测试覆盖：** `cli_activation_test.go` 覆盖了完整的 use/status/deactivate 生命周期、基于 kind 的分发、自动解析偏好以及缺失激活的稳定错误信息。
- **sync 测试中的冲突检测：** `sync_test.go` 验证了对 managed 文件的外部修改被检测和报告，且冲突不会静默覆盖用户更改。
- **包 I/O 往返测试：** `package_io_test.go` 测试了 agent 和 env 的导出后导入，验证引用的元数据被包含，检查幂等重新导入，并验证不同内容的冲突检测。
- **Zip 路径穿越防护：** `internal/packageio/import.go` 中的 `cleanPackagePath` 函数正确拒绝绝对路径、`..` 组件和反斜杠分隔符，缓解 zip-slip 攻击。
- **Shell 集成精心制作：** bash/zsh/fish 代码片段包含正确的输入验证（对 active 名称的正则字符类检查）、幂等 hook 注册和原始 prompt 保留。
- **root 上的 `SilenceUsage` 和 `SilenceErrors`：** 防止 cobra 在每个错误时打印用法，这对于有多个子命令的工具来说是正确的用户体验。
- **导入中的原子文件写入：** `writeImportFile` 使用 temp-file-then-rename，防止崩溃时的部分写入。
