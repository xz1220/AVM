# 熵控制：AI Coding 迭代中的仓库剪枝指南

> 审计日期：2026-04-26
> 审计范围：agent-vm 全仓库（67 commits，约 2 周迭代）

## 背景

AI coding 的迭代节奏是"一路狂奔"——先让功能跑起来，文档和设计稿在早期写好后就很少回头对齐。两周、67 次提交之后，仓库里积累了大量"信息熵"：文档描述的模块不存在、ROADMAP 的勾选状态落后于代码、README 宣传了尚未实现的 runtime。

这些过时信息不会让 `go build` 失败，但会让下一个读代码的人（包括 AI agent 自己）做出错误假设。

本文档列出当前仓库中已识别的熵源，按优先级排序，并给出持续控制熵的操作规范。

---

## 一、当前熵源清单

### P0：文档与代码严重不一致

| 位置 | 问题 | 修复方式 |
|------|------|----------|
| `docs/engineering/architecture.md` | 引用了 `internal/registry`、`internal/env`、`internal/template` 三个不存在的包。实际功能全部在 `internal/config` 中 | 重写模块依赖图，对齐实际的 9 个 internal 包 |
| `docs/engineering/modules/config.md:36-37` | 列出 `project.go`、`capability.go` 两个不存在的文件；引用 `LoadSkills` 函数（未实现） | 删除幽灵文件引用，对齐实际的 14 个 .go 文件 |
| `docs/engineering/workflows.md:41` | 使用 `detection.Installed`，代码中实际字段名为 `detection.Found` | 全局替换字段名 |
| `docs/engineering/modules/sync.md:114` | 同上 | 同上 |
| `ROADMAP.md` Phase 1 | `avm use`、`avm status`、`avm deactivate`、active manifest rebuild 等已实现功能仍标记为 `[ ]` | 对齐实际实现状态；Phase 2 中 Codex/Claude Code/Cline/Cursor adapter 均已实现，也需勾选 |

### P1：README 宣传了不存在的能力

| 位置 | 问题 | 修复方式 |
|------|------|----------|
| `README.{es,fr,ja,ko,pt-BR}.md` | badge 和正文中列出 OpenClaw、Hermes Agent 两个 runtime，但代码中只有 4 个 adapter（claude-code, codex, cline, cursor） | 删除或标注为 "planned"。主 `README.md` 已无此问题，仅翻译版本残留 |

### P2：代码中的死重

| 位置 | 问题 | 修复方式 |
|------|------|----------|
| `internal/config/models.go` | `LifecycleHooks`、`IOContract`、`AgentIdentity` 三个 struct 已定义但全仓库零引用（仅在 models.go 自身的字段声明中出现） | 删除，或加 `// placeholder: Phase 3` 注释明确其规划意图 |
| 4 个 adapter 包 | `writeFileAtomic`、`slug`、`firstNonEmpty` 等 15 个工具函数在 claude/codex/cline/cursor 四个包中逐字复制，约 500 行重复代码 | 提取到 `internal/adapter/shared/` |

### P3：测试与 CI 缺口

| 位置 | 问题 | 修复方式 |
|------|------|----------|
| `internal/backup/` | 2 个文件，0 个测试 | 补测试 |
| `internal/packageio/` | 3 个文件，0 个测试 | 补测试 |
| `internal/runtime/` | 2 个文件，0 个测试 | 补测试 |
| `.github/workflows/ci.yml` | 仅跑 `go test`，缺少 `go vet`、`go build`、`gofmt -l` | 补齐 CI 步骤 |

### P4：小问题

- `internal/config/models.go` 中 `Temperature` 字段无范围校验
- `internal/sync/` 有自己的 `writeYAML` 绕过了 atomic write
- 部分 CLI 命令使用 `context.Background()` 而非 `cmd.Context()`
- 测试中使用相对路径而非绝对路径

---

## 二、熵是怎么产生的

在 AI coding 迭代中，熵的来源有固定模式：

1. **设计先行，实现偏移**：先写了 architecture.md 规划 5 个包，实际编码时合并成了 3 个，但文档没人回头改。
2. **翻译滞后**：主 README 更新了，5 个翻译版本没跟上。
3. **ROADMAP 只加不勾**：功能做完了，但 `[ ]` 没变成 `[x]`。
4. **占位代码遗忘**：早期定义的 struct 是为未来 phase 准备的，但没有任何标记说明这一点，看起来像是应该被使用但被遗漏了。
5. **复制粘贴扩散**：adapter 之间的公共逻辑通过复制而非抽取来复用，每次改动需要改 4 个地方。

---

## 三、持续控制熵的操作规范

### 规则 1：每个 Stage 结束时跑一次文档对齐

每完成一个 Stage（或一组相关 PR），执行以下检查：

```bash
# 检查文档中引用的包/文件是否存在
grep -rn 'internal/' docs/engineering/ | \
  sed 's/.*\(internal\/[a-z_]*\).*/\1/' | sort -u | \
  while read pkg; do [ -d "$pkg" ] || echo "STALE: $pkg"; done

# 检查文档中引用的函数是否存在
grep -oP '[A-Z][a-zA-Z]+\(' docs/engineering/modules/*.md | \
  sort -u | while IFS=: read file func; do
    name=${func%\(}
    grep -rq "func.*$name" internal/ || echo "STALE in $file: $name"
  done
```

### 规则 2：翻译版本要么同步，要么删除

维护 5 个语言的 README 翻译在快速迭代期是净负担。两个选择：
- 删除翻译，等 v1.0 稳定后再翻译
- 保留翻译，但在每个翻译文件顶部加 `> ⚠️ 本翻译可能落后于英文版，以 README.md 为准`

### 规则 3：占位代码必须标注意图

如果一个 struct/function/file 是为未来 phase 准备的：
- 加一行注释说明属于哪个 phase
- 或者直接不写，等到那个 phase 再加

"先占个坑"在 AI coding 中特别危险——下一轮对话的 agent 不知道这是占位还是遗漏。

### 规则 4：ROADMAP 跟着代码走，不跟着计划走

`ROADMAP.md` 的勾选状态应该反映"代码中已实现且有测试"，而不是"计划中要做"。每次 merge 到 main 时检查一次。

### 规则 5：重复代码超过 3 处就提取

当同一段逻辑出现在第 3 个地方时，立即提取为共享包。AI coding 的复制粘贴速度远快于人类，但技术债积累的速度也是。

### 规则 6：CI 是熵的最后防线

CI 应该覆盖所有能自动化检查的一致性：
- `go vet` 捕获代码级问题
- `go build` 确保编译通过
- `gofmt -l` 确保格式一致
- 未来可加：文档中引用路径的存在性检查

---

## 四、建议的剪枝执行顺序

1. 修复 `ROADMAP.md` 的勾选状态（5 分钟，影响所有新读者的第一印象）
2. 修复 `architecture.md` 的模块引用（15 分钟，消除最大的误导源）
3. 全局替换 `detection.Installed` → `detection.Found`（2 分钟）
4. 处理翻译 README 中的 OpenClaw/Hermes 引用（10 分钟）
5. 删除或标注 models.go 中的占位 struct（5 分钟）
6. 提取 adapter 公共函数到 shared 包（30 分钟，可单独 PR）
7. 补齐 CI 步骤（10 分钟）
8. 补齐 backup/packageio/runtime 的测试（按需排期）
