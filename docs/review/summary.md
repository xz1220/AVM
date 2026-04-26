# AVM 代码审查总结

**日期：** 2026-04-26
**范围：** 5 个领域的全代码库审查
**审查者：** 5 个并行审查 agent

## 总体评估

AVM 是一个架构良好的早期 Go CLI 项目，具有清晰的包边界、一致的模式（原子写入、确定性输出、防御性 nil 检查）以及对正常路径的全面测试覆盖。代码在结构上达到了生产质量，但在更广泛使用之前需要解决几个加固缺口——特别是在并发安全、symlink 处理和输入大小限制方面。

`go build`、`go vet` 和 `go test` 均通过，无报错。

---

## 严重问题 (8)

| # | 领域 | 问题 | 位置 |
|---|------|------|------|
| C1 | packageio | **zip bomb / 内存耗尽** — `io.ReadAll` 读取 zip 条目时无大小限制 | `internal/packageio/import.go:153` |
| C2 | sync | **无并发保护** — 并行 `avm sync` 可能损坏状态，在 active 目录重建和 managed path 写入上产生竞争 | `internal/state/store.go`, `internal/sync/syncer.go` |
| C3 | sync | **symlink 检测是死代码** — `filepath.WalkDir` + `entry.Info()` 永远不会报告 `ModeSymlink`；managed 目录中的 symlink 变更无法被检测 | `internal/sync/conflict.go:175-180` |
| C4 | sync | **sync 中途失败时的部分写入状态** — 第一个 runtime 已渲染，第二个失败，状态未持久化，下次 sync 无法检测冲突 | `internal/sync/syncer.go:92-98` |
| C5 | adapter | **Cursor MCP merge 静默覆盖用户自有 server** — 不像 Claude/Cline adapter 那样有所有权跟踪元数据 | `internal/adapter/cursor/cursor.go:549-595` |
| C6 | adapter | **文件写入前无 symlink 解析** — `writeFileAtomic` 通过 `os.Rename` 跟随 symlink，可能导致路径穿越 | 所有 4 个 adapter 的 `writeFileAtomic` 实现 |
| C7 | config | **YAML 读取无文件大小限制** — `readYAML` 在读取恶意/损坏的配置文件时可能导致 OOM | `internal/config/yaml.go:15` |
| C8 | CLI | **`activationTargetExists` 将权限错误视为"存在"** — 将用户看到的错误信息从清晰变为模糊 | `cmd/avm/use_activation.go:116` |

## 重要问题 (15)

| # | 领域 | 问题 | 位置 |
|---|------|------|------|
| M1 | adapter | **~500 行工具函数重复** 分布在 4 个 adapter 包中（15 个函数复制粘贴） | `claude/`, `codex/`, `cline/`, `cursor/` |
| M2 | adapter | **MCP 冲突行为不一致** — Claude 硬报错，Cline 警告并跳过，Cursor 静默覆盖 | 3 个 adapter 包 |
| M3 | adapter | **Claude/Cline 未对 managed path 进行所有权边界验证** — Codex 通过 `validateCodexManagedPath` 做了验证 | `claude/claude.go:200`, `cline/cline.go:215` |
| M4 | state | **rename 前无 fsync** — `Close()` 和 `Rename()` 之间崩溃可能丢失数据 | `state/store.go:69-80`, `packageio/export.go:423`, `packageio/import.go:305` |
| M5 | tests | **零测试覆盖** 涉及 `internal/backup/`、`internal/packageio/`、`internal/runtime/` | 3 个包 |
| M6 | sync | **active 目录 `.prev` 清理失败被当作错误返回**，即使主操作已成功 | `internal/sync/active.go:83-86` |
| M7 | CLI | **`applyActivation` 在部分错误时未将 sync 状态标记为失败**（而 `runSync` 正确地做了） | `cmd/avm/use_activation.go:130-139` |
| M8 | CLI | **`runSync` 在 `UpdateActive: false` 时仍写入 `current-active`** | `cmd/avm/sync.go:58` |
| M9 | CLI | **`deactivate` 硬编码回退到 `profile:default`** 但未检查其是否存在 | `cmd/avm/deactivate.go:24` |
| M10 | CLI | **`agent create` 静默默认使用 `codex` runtime** — 未文档化 | `cmd/avm/agent.go:83` |
| M11 | config | **所有 Write 函数中的双重验证** — 每次写入验证两次，浪费资源 | `agent.go:33-42`, `env.go:30-35`, `memory.go:31-43` |
| M12 | config | **`mergeRuntimeOverrides` 和 `mergeAnyMap` 测试覆盖率为 0%** — 最复杂的 merge 逻辑未经测试 | `internal/config/merge.go:121-156` |
| M13 | config | **`Temperature` 字段未验证** — 允许超出范围的值如 -5.0 或 999.0 | `internal/config/models.go:99` |
| M14 | docs | **架构文档引用了 3 个不存在的模块** (`internal/registry`, `internal/env`, `internal/template`) | `docs/engineering/architecture.md:170-179` |
| M15 | docs | **Config 模块文档列出了不存在的文件/API** (`project.go`, `capability.go`, `LoadSkills`) | `docs/engineering/modules/config.md:30-42` |

## 次要问题 (25)

在所有 5 个审查领域中，共发现 25 个次要问题，涵盖：文档与代码之间的命名不一致（`Found` vs `Installed`）、过时的 README 状态部分、缺失的 CI 目标（`go vet`、`go build`）、使用 `context.Background()` 而非 `cmd.Context()`、重复的 `os.Getwd()` 样板代码、未文档化的 `--memory` flag 格式、相对路径的测试、错误包装中使用 `%v` 而非 `%w`、冗余的辅助函数（`sha256String`、`skillLines` = `capabilityLines`）、backup 中的双重 close 模式等。

完整详情见各单独审查文件。

## 积极发现

- **全面使用原子写入** — temp-file + rename 模式在 config、state、backup、adapter 和 packageio 中一致使用。
- **确定性输出** — `renderplan.Normalize` + 排序迭代确保稳定的 hash 和可复现的 CLI 输出。
- **严格的 YAML 解析** — `KnownFields(true)` + 多文档拒绝可以尽早捕获拼写错误和 schema 漂移。
- **全面的 zip slip 防护** — `cleanPackagePath` 拒绝绝对路径、`..` 穿越、反斜杠和非往返路径。
- **清晰的 adapter 接口** — 最小化的 5 方法契约，每个字段都有显式的 mapping 状态，`RenderPlan`/`RenderResult` 分离。
- **深拷贝纪律** — `ManagedPaths` 返回副本，`cloneOperations` 深拷贝字节切片，merge 在修改前先克隆。
- **幂等渲染** — 所有 adapter 测试验证第二次 render 报告 `Changed: false`。
- **导出时的密钥检测** — 防止在 registry bundle 中意外导出明文 token。
- **结构化验证错误** — 每条错误信息都包含文件路径 + 字段路径上下文。
- **设计良好的 active 目录重建** — 三阶段交换，失败时可回滚。

## 建议优先级

**立即处理（在任何共享使用之前）：**
1. C1 — 在 zip 导入中添加 `io.LimitReader`
2. C2 — 在 sync 流程中添加文件锁
3. C5 — 在 Cursor MCP merge 中添加所有权跟踪
4. C6 — 在写入前添加 `filepath.EvalSymlinks`

**短期（beta 之前）：**
5. C3 — 修复冲突 hash 中的 symlink 检测
6. C4 — 按 runtime 增量持久化 sync 状态
7. C7 — 为 YAML 读取添加大小限制
8. C8 — 修复 `activationTargetExists` 的权限错误处理
9. M1 — 提取共享 adapter 工具函数
10. M4 — 在原子写入的 rename 前添加 fsync
11. M5 — 为 backup、packageio、runtime 包添加测试

**中期（质量/打磨）：**
12. M2 — 统一各 adapter 的 MCP 冲突行为
13. M7-M10 — CLI bug 修复
14. M14-M15 — 同步文档与实现

---

## 可读性、可维护性与设计复杂度

第二轮 review 专门针对可读性、可维护性和设计复杂度。详细报告见 [readability-summary.md](readability-summary.md)。

### 过度设计评估

**确认不是过度设计的部分：**
- RenderPlan 中间表示 — 被 sync 层、CLI preview、state 持久化三处消费
- FieldMapping 系统 — 被 agent show 和 state 持久化消费
- Adapter 接口 5 方法设计 — 最小化且清晰

**确实存在的过度设计：**
- `TargetCapability.Level` 从未被读取，`KnownTargets` 仅用作 set (`config/models.go:18-27`)
- `LifecycleHooks`、`IOContract`、`AgentIdentity` 被解析但无代码消费 (`config/models.go:65-68`)
- `MergeModeMarkedBlock`、`OperationEnsureDir`、`OperationRemoveFile`、`OperationMergeSection` 无真实 adapter 使用 (`adapter/adapter.go`)
- `RuntimeStatusPartial` / `TargetStatusPartial` 定义但从未赋值 (`state/types.go`, `sync/types.go`)
- `skillLines()` 和 `capabilityLines()` 在同一文件中完全相同 (`claude/claude.go:890-920`)

### 可读性 Top 3

1. **Adapter 工具函数 ~475 行复制粘贴** — 15 个函数在 4 个包中逐字复制，其中 `portableMemoryLines` 有一个隐藏的行为差异
2. **`mappings()` 方法 60-189 行重复模式** — 应改为声明式表驱动，可减少每个 adapter 50-100 行
3. **Hash 冲突检测的隐式契约无文档** — `ManagedHash` 到 `FileHash` 的静默回退跨越 3 个文件，无注释

### 可维护性 Top 3

1. **添加新 adapter 需要复制 ~200 行工具代码** — 提取到共享包可消除
2. **`SyncActivation` 85 行混合编排与状态变更** — 应拆分为命名阶段
3. **`state` 包依赖 `adapter` 类型** — 转换函数应移到 `sync` 保持 `state` 纯净

### 认知负担热点

- `adapter.go` 定义 25+ 类型，新开发者入门门槛高
- 单个 adapter 文件 700-1100 行，`mappings()` 埋在最深处
- `renderTarget` 需要理解完整的 adapter 生命周期（6 个方法调用链）
- 枚举值散落为字符串字面量，无对应常量

### 改进建议优先级

| 优先级 | 建议 | 影响 |
|--------|------|------|
| P0 | 提取 adapter 共享工具函数到 `internal/adapter/adapterutil/` | 消除 ~475 行重复 |
| P0 | 文档化 hash 冲突检测契约 | 消除最危险的隐式契约 |
| P1 | 拆分 `SyncActivation` 为命名阶段 | 提高可测试性和可读性 |
| P1 | `mappings()` 改为表驱动 | 每个 adapter 减少 50-100 行 |
| P2 | 消除 CLI 层 `os.Getwd()` 样板 | 减少 10 处重复 |
| P2 | 合并重复的 `writeYAML` 和 `uniqueStrings` | 消除同名不同行为的混淆 |
| P2 | 在 sync 边界强制 `renderplan.Normalize()` | 消除隐式约定 |
| P3 | 删除未使用的模型字段和操作类型 | 减少认知负担 |
| P3 | 定义枚举常量替代散落的字符串字面量 | 提高可发现性 |
| P3 | 拆分 `agent.go` 和 `adapter.go` 大文件 | 提高导航性 |

---

## 各单独审查文件

### 第一轮：正确性、安全性、Bug
- [CLI 层](cli-layer.md) — 3 个严重、5 个重要、8 个次要
- [Config 包](config-package.md) — 3 个严重、7 个重要、9 个次要
- [Adapter 层](adapter-layer.md) — 2 个严重、4 个重要、6 个次要
- [编排与状态](orchestration-state.md) — 4 个严重、5 个重要、7 个次要
- [横切关注点与文档](cross-cutting.md) — 0 个严重、5 个重要、9 个次要

### 第二轮：可读性、可维护性、设计复杂度
- [可读性总结](readability-summary.md) — 综合总结
- [CLI 层可读性](readability-cli.md)
- [Adapter 层可读性](readability-adapters.md)
- [Config + Sync + State 可读性](readability-config-sync.md)
