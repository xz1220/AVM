# Config 包审查 (internal/config/)

## 总结

config 包是一个结构良好、防御性编码的 AVM 配置层。它涵盖了带严格字段检查的 YAML 解析、原子写入、深拷贝 merge 逻辑、多 scope 解析和全面验证。代码紧密遵循 Go 惯例，架构合理。以下问题主要是加固缺口（路径穿越、文件大小限制、缺失的 `Temperature` 验证）和测试覆盖空白（总体 62.2%，merge 内部和多个公开 API 为 0%）。

## 严重问题

### C1. 路径构建中通过 `name` 参数的路径穿越

`paths.go:24`（`AgentPath`）、`paths.go:32`（`EnvPath`）、`paths.go:52`（`MemoryPath`）和 `paths.go:80`（`ProjectAgentPath`）都将用户提供的 `name`/`id` 值直接拼接到文件系统路径中。虽然 `validName` 将名称限制为 `[a-z0-9][a-z0-9-]{0,63}`，但验证和路径构建是分开的调用点。任何在验证*之前*构建路径（或跳过验证）的调用者都可能允许 `../../etc/passwd` 风格的穿越。

```go
// paths.go:24
func AgentPath(name string) string {
    return filepath.Join(AgentsDir(), name+".yaml")
}
```

建议：在每个路径构建函数内部添加 `filepath.Rel` 或 `strings.Contains(name, "..")` 守卫，或使它们返回 `(string, error)` 并在内部验证。这是纵深防御——正则表达式目前阻止了它，但路径函数是导出的，可以被独立调用。

### C2. YAML 读取无文件大小限制

`yaml.go:15`（`readYAML`）打开并解码文件时无大小检查。恶意或损坏的配置文件可以任意大，导致 OOM。由于这些是磁盘上用户控制的文件（可能通过 `.avm/` 项目目录来自共享仓库），这是一个真实的攻击向量。

```go
// yaml.go:15-25
func readYAML(path string, out any) error {
    file, err := os.Open(path)
    if err != nil {
        return err
    }
    defer file.Close()
    decoder := yaml.NewDecoder(file)
    // 无大小限制 — 将整个文件读入内存
```

建议：用 `io.LimitReader(file, maxConfigSize)` 包装 reader，其中 `maxConfigSize` 为合理值如 1MB。

### C3. `Temperature` 字段未验证 — 允许超出范围的值

`validation.go:101`（`validateAgentProfile`）验证了 `ReasoningEffort`、`Verbosity` 和许多其他字段，但从未检查 `ModelRun.Temperature`。该字段是 `*float64`（正确使用指针以区分零值），但 `-5.0` 或 `999.0` 这样的值会静默通过验证。

```go
// models.go:99
Temperature     *float64 `yaml:"temperature,omitempty"`
```

`validateAgentProfile` 中没有对应的检查。建议：添加范围检查，例如 `0.0 <= *t <= 2.0`。

## 重要问题

### M1. Write 函数中的双重验证既浪费又不一致

`WriteAgent`（agent.go:28-43）、`WriteEnvironment`（env.go:25-37）和 `WritePortableMemory`（memory.go:31-43）都调用 `validate*` 两次——一次使用空路径，然后使用真实路径再次调用。这意味着每次写入都做两次完整的验证。唯一的区别是第二次调用的错误信息包含文件路径。

```go
// agent.go:33-42
func WriteAgent(agent *AgentProfile, scope Scope, cwd string) error {
    // ...
    if err := validateAgentProfile(agent, ""); err != nil {   // 第一次验证
        return err
    }
    path, err := agentPathForScope(agent.Name, scope, cwd)
    // ...
    if err := validateAgentProfile(agent, path); err != nil {  // 第二次验证（逻辑相同）
        return err
    }
```

建议：先计算路径，然后用路径验证一次。当前的顺序（先验证再计算路径）不提供有意义的安全性，因为 `agentPathForScope` 只做 scope 切换。

### M2. `WriteEnvironment` 用不同路径验证两次

`env.go:25-37` 先用 `""` 验证，然后用 `EnvPath(env.Name)` 验证。但第一次调用无论如何都使用 `env.Name` 来推导路径。如果用 `""` 验证通过，用路径也一定会通过——路径只影响错误信息，不影响验证逻辑。

```go
// env.go:30-35
if err := validateEnvironment(env, ""); err != nil {
    return err
}
path := EnvPath(env.Name)
if err := validateEnvironment(env, path); err != nil {
    return err
}
```

### M3. `Validate` 泛型函数和导出的类型化验证器未经测试

`validation.go:10-35`（`Validate`）和 `validation.go:37-39`（`ValidateActiveRef`、`ValidateGlobalConfig`）覆盖率为 0%。`Validate` 函数通过 type switch 处理指针和值类型——这是一个公开 API，应该有测试确认每个分支都能工作。

### M4. `UpdateActive` 测试覆盖率为 0%

`global.go:30-45` 处理更新 active profile/env 引用的重要场景，包括 `os.IsNotExist` 回退路径（创建新的 `GlobalConfig`）。这是一个关键的面向用户的操作，却没有测试。

### M5. `mergeRuntimeOverrides` 和 `mergeAnyMap` 覆盖率为 0%

`merge.go:121-156` 实现了 `map[string]any` 树的递归深度 merge。这是 merge 层中最复杂的逻辑，测试覆盖率为零。嵌套 map 冲突、同一 key 的混合类型和 nil 中间 map 等边界情况未经测试。

### M6. Memory ref 允许任意 `Path` 值而无清理

`validation.go:238-252`（`validateMemoryRef`）和 `validation.go:210-236`（`validatePortableMemory`）检查 `Path` 非空但不验证路径内容。fixture 显示路径如 `~/.avm/memory/project/backend-standards.md`——波浪号展开不被 Go 的 `filepath` 包处理，因此这些路径会被字面使用。此外，`Path` 中的绝对路径或 `../` 序列未被检查。

```go
// validation.go:246-247
if ref.Path == "" {
    return fieldError(path, field+".path", "required")
}
// 无进一步的路径验证
```

### M7. `MemoryRef.Scope` 是 `string` 但 `PortableMemory.Scope` 也是 `string` — 应该是 `Scope` 类型

`models.go:126` 将 `MemoryRef.Scope` 定义为 `string`，`models.go:152` 将 `PortableMemory.Scope` 也定义为 `string`。两者都针对相同的 scope 常量集进行验证。使用 `Scope` 类型别名可以提供编译时安全性，并与 `Scope` 在其他地方的使用方式保持一致（例如函数参数）。

```go
// models.go:125-126
type MemoryRef struct {
    ID    string `yaml:"id"`
    Scope string `yaml:"scope"`  // 应该是 Scope
```

## 次要问题

### m1. `ScopeLocal` 和 `ScopeProject` 在路径解析中被同等对待

`paths.go:84-93`（`agentPathForScope`）将 `ScopeProject` 和 `ScopeLocal` 都映射到相同的项目路径。如果这些 scope 在语义上不同，路径解析没有反映这一点。如果它们是同义词，应该废弃其中一个。

```go
// paths.go:88-89
case ScopeProject, ScopeLocal:
    return ProjectAgentPath(cwd, name), nil
```

### m2. `AvmDir` 回退到根目录令人意外

`paths.go:8-14`：当 `os.UserHomeDir()` 失败时，回退为 `/.avm`（根级别）。这几乎肯定不可写，会在下游产生令人困惑的错误。更有用的回退可能是当前工作目录，或返回错误。

```go
// paths.go:10-11
if err != nil || home == "" {
    return filepath.Join(string(os.PathSeparator), ".avm")
}
```

### m3. `writeYAML` 未在最终文件上设置文件权限

`yaml.go:38-74`：临时文件以默认权限创建（可能是 `os.CreateTemp` 的 0600），`os.Rename` 保留这些权限。但父目录以 `0o700` 创建。最终文件上没有显式的 `os.Chmod`。在 umask 宽松的系统上，配置文件可能变为全局可读。建议在 rename 前显式设置临时文件为 `0o600`。

### m4. `listYAMLFiles` 接受 `.yaml` 和 `.yml` 但所有代码生成 `.yaml`

`yaml.go:91` 接受 `.yml` 文件，但 `AgentPath`、`EnvPath`、`MemoryPath` 都硬编码 `.yaml`。这意味着 `.yml` 文件会被列出但在读取时可能导致名称不匹配错误（因为名称从文件内容而非文件名推导）。要么记录这是有意为之，要么限制为仅 `.yaml`。

### m5. `Verbosity` 允许重叠的值集

`validation.go:132`：Verbosity 同时接受语义名称（`quiet`、`concise`、`normal`、`verbose`）和级别名称（`low`、`medium`、`high`）。这造成了歧义——`low` 等同于 `quiet` 还是 `concise`？映射从未定义。

```go
if agent.ModelRun.Verbosity != "" && !oneOf(agent.ModelRun.Verbosity,
    "quiet", "concise", "normal", "verbose", "low", "medium", "high") {
```

### m6. `cloneAny` 不处理 `gopkg.in/yaml.v3` 可能产生的 `map[any]any`

`merge.go:169-186`：`cloneAny` 函数处理 `map[string]any`、`[]any`、`[]string` 和 `map[string]string`。然而，YAML v3 对某些输入（非字符串 key）可以产生 `map[any]any`。这些会落入 default 分支，通过引用共享而非克隆。

### m7. `WriteProjectOverride` 覆盖率为 0%

`env.go:44-53` 是一个完全没有测试覆盖的公开 API。

### m8. 错误包装使用 `%v` 而非 `%w` 处理嵌套错误

`resolve.go:103`：在环境解析期间包装 agent 读取错误时，使用 `%v` 而非 `%w`，这破坏了 `errors.Is`/`errors.As` 链。

```go
// resolve.go:103
return nil, fieldError("", "runtime_agents."+runtime+".primary", "%v", err)
```

这意味着调用者无法使用 `os.IsNotExist(err)` 来区分此代码路径中的"agent 文件缺失"和其他错误。

### m9. `ProjectOverride.Extends` 是必需的但在 `WriteProjectOverride` 中未作为名称验证

`env.go:44-53` 调用 `validateProjectOverride` 来检查 `Extends`，但该函数也用路径调用 `validateProjectOverride`——与其他 Write 函数相同的双重验证模式。然而，`WriteProjectOverride` 不调用 `ApplyDefaults`（`ProjectOverride` 没有 `ApplyDefaults`），这没问题因为它没有可默认的字段，但与其他地方使用的模式不一致。

## 积极发现

1. **通过 temp-file-and-rename 的原子写入**（`yaml.go:38-74`）：这是配置文件的正确方法。错误时临时文件的延迟清理实现良好。

2. **严格的 YAML 解析**，使用 `decoder.KnownFields(true)` 和多文档拒绝（`yaml.go:22-34`）：这可以尽早捕获拼写错误和 schema 漂移，对配置层来说完全正确。

3. **稳定的往返测试**（`config_test.go:183-202`）：`assertStableWrite` 辅助函数验证读取后写入产生字节相同的输出，并拒绝空集合（`[]`、`{}`）。这是一个深思熟虑的测试，可以捕获序列化漂移。

4. **merge 前深拷贝**（`merge.go:93-105`）：merge 层在应用覆盖前正确克隆基础环境，防止原始数据被修改。克隆函数很全面。

5. **一致的验证模式**：每个 Read/Write 路径都运行带文件路径上下文的验证以生成错误信息。`fieldError` 辅助函数产生结构化的、可 grep 的错误信息如 `path: field: message`。

6. **`omitempty` 使用正确**：可选字段使用 `omitempty` YAML 标签，必需字段不使用。这意味着空的可选字段不会污染序列化输出。

7. **确定性输出**：`ListAgents`、`ListEnvironments`、`ListPortableMemory` 和 `sortedRuntimeKeys` 都对输出排序，确保稳定的 CLI 输出和可测试性。

8. **项目 scope 覆盖模型设计良好**：`extends` + merge 模式简洁，允许团队共享基础环境同时按项目自定义。
