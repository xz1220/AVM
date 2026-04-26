# 横切关注点与文档审查

## 总结

AVM 代码库结构良好，包边界清晰，无循环依赖，验证全面。文档内容丰富且与实现基本一致，但有几处文档引用了代码中尚不存在的模块和 API。主要问题是 adapter 包之间大量的代码重复、Detection 结构体在文档和实现之间的命名不一致，以及三个包缺少测试覆盖。

## 严重问题

未发现。`go build ./...`、`go vet ./...` 和 `go test ./...` 均通过无报错。未发现安全漏洞、数据丢失风险或正确性 bug。

## 重要问题

### 1. adapter 包之间大量的代码重复

`writeFileAtomic`、`slug`、`firstNonEmpty`、`sortedStrings`、`sortedMCPServers`、`mcpServerRenderable`、`managedPathIndex`、`writeLine`、`section`、`bulletSection`、`capabilityLines`、`memoryRefLines`、`portableMemoryLines` 和 `toolsetLines` 这些函数在 3-4 个 adapter 包中被逐字（或近乎逐字）复制：

- `/Users/danielxing/code/agent-vm/internal/adapter/claude/claude.go`
- `/Users/danielxing/code/agent-vm/internal/adapter/codex/codex.go`
- `/Users/danielxing/code/agent-vm/internal/adapter/cline/cline.go`
- `/Users/danielxing/code/agent-vm/internal/adapter/cursor/cursor.go`

这大约是 ~15 个函数各复制 4 次。一个共享的 `internal/adapter/shared` 或 `internal/adapter/render` 包可以消除这种重复，而不违反 adapter 不依赖 sync 的架构约束。

### 2. Detection 结构体字段名在文档和代码之间不匹配

架构文档（`/Users/danielxing/code/agent-vm/docs/engineering/architecture.md`，第 115 行）引用 `detection.Installed`，但 `/Users/danielxing/code/agent-vm/internal/adapter/adapter.go:30` 中的实际 `Detection` 结构体使用 `Found`：

```go
type Detection struct {
    Runtime   string   `json:"runtime"`
    Found     bool     `json:"found"`       // 文档写的是 "Installed"
```

sync 模块文档（`/Users/danielxing/code/agent-vm/docs/engineering/modules/sync.md`，第 115 行）也引用 `detection.Installed`。两处文档都应更新为 `Found`。

### 3. 架构文档引用了代码中不存在的模块

架构文档（`/Users/danielxing/code/agent-vm/docs/engineering/architecture.md`，第 170-179 行）列出了这些模块：

| 模块 | 文档中的包路径 | 实际状态 |
|------|---------------|---------|
| registry | `internal/registry` | 不存在 |
| env | `internal/env` | 不存在 |
| template | `internal/template` | 不存在 |

依赖图（第 184-193 行）显示 `env -> config, registry` 和 `cmd/avm -> template`，但 `env`、`registry` 和 `template` 包都不存在。`env` 和 `registry` 的职责目前由 `internal/config` 处理。这是一个显著的文档漂移，可能误导贡献者。

### 4. Config 模块文档列出了不存在的文件

`/Users/danielxing/code/agent-vm/docs/engineering/modules/config.md`（第 30-42 行）列出了这个包结构：

```
internal/config/
├── project.go        # 不存在（代码在 merge.go 中）
├── capability.go     # 不存在
```

`project.go` 的功能（ProjectOverride、MergeEnvironment）在 `merge.go` 中。`capability.go` 文件（LoadSkills、LoadMCPs 等）不存在——这些 API 有文档但未实现。

### 5. 三个包缺少测试覆盖

`go test ./...` 对以下包报告 `[no test files]`：

- `internal/backup` — 包含文件 I/O、symlink 处理和目录复制的备份逻辑
- `internal/packageio` — 带验证的 zip 归档导入/导出
- `internal/runtime` — adapter registry

backup 和 packageio 包包含非平凡的逻辑（路径清理、zip 处理、密钥检测），值得有测试覆盖。

## 次要问题

### 6. Sync 包有自己的 `writeYAML` 绕过了原子写入

`/Users/danielxing/code/agent-vm/internal/sync/active.go:185-191` 定义了一个本地 `writeYAML`，直接使用 `os.WriteFile`：

```go
func writeYAML(path string, value any) error {
    raw, err := yaml.Marshal(value)
    if err != nil {
        return err
    }
    return os.WriteFile(path, raw, 0o600)
}
```

而 `internal/config/yaml.go:38-74` 有一个使用 temp-file + rename 的正确原子 `writeYAML`。sync 版本用于 active 目录文件，这些文件在目录级别被原子重建，所以这不是正确性问题，但它造成了命名冲突，可能让贡献者困惑。

### 7. README runtime 徽章列出了 "OpenClaw | Hermes Agent" 但代码只有 4 个 adapter

`/Users/danielxing/code/agent-vm/README.md:16` 徽章写的是：

```
runtime-Codex%20%7C%20Claude%20Code%20%7C%20OpenClaw%20%7C%20Hermes%20Agent
```

但 `/Users/danielxing/code/agent-vm/internal/config/models.go:22-27` 中的实际 `KnownTargets` 和 `/Users/danielxing/code/agent-vm/internal/runtime/registry.go:17-23` 中的 runtime registry 只包含 `claude-code`、`codex`、`cline` 和 `cursor`。OpenClaw 和 Hermes Agent 是未来目标。徽章应反映当前状态或标注为"计划中"。

### 8. README "状态" 部分将 `avm status` 和 `avm deactivate` 列为"进行中"但它们已存在

`/Users/danielxing/code/agent-vm/README.md:159-163` 将这些列为"进行中"：

```
- `avm use <profile-or-env>`
- `avm status`
- `avm deactivate`
```

但 `cmd/avm/status.go`、`cmd/avm/deactivate.go` 和 `cmd/avm/use.go` 都已有实现。README 状态部分已过时。

### 9. CI 工作流只运行 `go test`，缺少 `go vet` 和 `go build`

`/Users/danielxing/code/agent-vm/.github/workflows/ci.yml` 只运行：

```yaml
- run: go test ./...
```

Makefile 有 `vet` 和 `build` 目标。CI 也应运行 `go vet ./...` 和 `go build -o /dev/null ./cmd/avm` 来捕获仅靠测试无法发现的问题。

### 10. Makefile 缺少 `lint` 和 `coverage` 目标

`/Users/danielxing/code/agent-vm/Makefile` 有 `build`、`test`、`fmt`、`vet`、`clean` 但没有：

- `lint`（例如 `golangci-lint run`）
- `coverage`（例如 `go test -coverprofile=coverage.out ./...`）
- `install`（例如 `go install ./cmd/avm`）

这些是贡献者期望的常见目标。

### 11. 不存在 CLAUDE.md 文件

仓库中未找到 `CLAUDE.md`。鉴于这是一个管理 AI agent profile（包括 Claude Code）的工具，有一个包含项目约定的 `CLAUDE.md` 对于在此仓库上进行 AI 辅助开发会很有用。

### 12. `go.sum` 包含未使用的依赖条目

`/Users/danielxing/code/agent-vm/go.sum:9` 有：

```
go.yaml.in/yaml/v3 v3.0.4/go.mod h1:...
```

这是与项目实际使用的 `gopkg.in/yaml.v3` 不同的模块。看起来是一个过时的条目。运行 `go mod tidy` 可以清理它。

### 13. Fixture manifest 使用占位符路径但无验证

`/Users/danielxing/code/agent-vm/fixtures/phase1/minimal/manifest.yaml:8-12` 使用 `<AVM_HOME>`、`<PROJECT_ROOT>` 等作为占位符值。fixture 约定文档应说明测试代码如何替换这些，理想情况下 fixture 加载代码应在使用前验证占位符已被替换。

### 14. 数据模型文档中的 Go 结构体与实现略有偏差

数据模型文档（`/Users/danielxing/code/agent-vm/docs/engineering/data-model.md`）显示的 Go 结构体定义与实际代码接近但不完全相同。例如，文档的 `SyncState`（第 503 行）使用 `Targets map[string]TargetState`，但 `/Users/danielxing/code/agent-vm/internal/state/types.go:22` 中的实际代码使用 `Runtimes map[string]RuntimeState`。文档使用 `TargetState` / `FileState` / `FieldMappingState`，而代码使用 `RuntimeState` / `ManagedPathState` / `MappingState`。

## 积极发现

- 清晰的依赖图，无循环导入。分层与文档完全一致：`config` 无上游依赖，`adapter` 仅依赖 `config`，`state` 依赖 `config` + `adapter`，`sync` 依赖所有这些，`backup` 依赖 `adapter` + `config`。
- 原子文件写入在 config、state、backup 和所有四个 adapter 包中一致使用（temp file + rename 模式）。
- 验证全面，产生带文件路径和字段路径上下文的结构化错误信息（例如 `~/.avm/agents/backend.yaml: permissions.sandbox: invalid value "full"`）。
- `renderplan.Normalize` 函数确保所有 adapter 的确定性输出排序，这对冲突检测的 hash 稳定性很重要。
- YAML 解析使用 `KnownFields(true)` 并拒绝多文档文件，防止静默的配置错误。
- Adapter 契约设计良好：每个字段都必须有显式的 `MappingStatus` 说明，`RenderPlan` / `RenderResult` 分离提供了良好的可观测性。
- `MergeEnvironment` 函数正确区分 nil 和空切片的覆盖语义。
- `packageio/export.go` 中的密钥检测防止意外导出明文 token——一个深思熟虑的安全措施。
- 所有四个具体 adapter（Claude Code、Codex、Cline、Cursor）遵循相同的结构模式，使得通过阅读一个就能理解另一个。
- `merge.go` 中的 `cloneAny` 深拷贝工具正确处理所有 YAML 可表示的类型。
