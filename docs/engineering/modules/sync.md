# Agent VM — Sync 模块设计

> 最后更新：2026-04-24（v6 — 去除 Phase 1 Watch）

Sync 模块是执行引擎：解析 active profile/env 后重建 `~/.avm/active/`，并发调用 runtime adapter，做冲突检测、备份、写入和状态更新。

---

## 职责

1. 编排 `avm use` / `avm sync`。
2. 重建 `~/.avm/active/`。
3. 调用 adapter 生成 render plan。
4. 对 managed paths 做 hash 冲突检测。
5. 写前备份。
6. 调用 adapter render。
7. 保存 sync state，包括 mapping status。

## 不做

- 不做 YAML merge；这是 config 的职责。
- 不做 runtime 格式翻译；这是 adapter 的职责。
- 不直接修改 registry。

---

## 包结构

```
internal/sync/
├── syncer.go       # 核心编排
├── active.go       # active 目录原子重建
├── conflict.go     # hash 冲突检测
├── backup.go       # managed paths 备份
└── state.go        # sync-state.json
```

---

## 核心流程

```go
func (s *Syncer) SyncActivation(ref config.ActiveRef, cwd string, opts Options) error {
    resolved, err := config.ResolveActivation(ref, cwd)
    if err != nil {
        return err
    }

    if err := s.RebuildActive(resolved); err != nil {
        return err
    }

    results := s.RenderTargetsConcurrently(resolved, cwd)

    if opts.UpdateActive {
        if err := config.UpdateActive(ref); err != nil {
            return err
        }
    }
    if err := s.State.UpdateCurrentActive(ref); err != nil {
        return err
    }
    if err := s.State.Save(); err != nil {
        return err
    }

    return summarize(results)
}
```

语义：

- active profile/env 解析或 active 重建失败时，不更新 `config.yaml.active`、`state/current-active` 或 runtime 配置。
- target 部分失败时，`config.yaml.active` 和 `state/current-active` 仍更新为目标 active ref，因为 AVM source composition 已激活；失败 runtime 通过 `sync-state.json` 和退出码暴露。
- `avm sync` 不改变 `config.yaml.active`，但会用当前 active ref 刷新 `state/current-active`，避免 shell prompt 漂移。

---

## Active 重建

`active/` 用临时目录构建，再原子替换：

```
~/.avm/active.tmp-<pid>/
~/.avm/active.prev/
~/.avm/active/
```

流程：

1. 创建 temp active。
2. 写 `manifest.yaml`。
3. 复制 resolved Agent Profile YAML 到 `active/agents/`。
4. 按每个 runtime 的 Agent Profile 引用展开 skills/memory symlink。
5. 按每个 runtime 的 Agent Profile 引用复制 MCP/commands/hooks YAML 到 active。
6. 创建 `active/render/<runtime>/`。
7. rename 当前 active 为 `active.prev`，temp rename 为 `active`。
8. 删除旧 `active.prev`。

失败时不破坏旧 active。

---

## Render Target

```go
func (s *Syncer) renderTarget(runtime string, resolved *config.ResolvedActivation, cwd string) TargetResult {
    adp := adapter.Get(runtime)
    if adp == nil {
        return TargetResult{Runtime: runtime, Status: "failed", Error: "adapter not found"}
    }

    detection := adp.Detect(ctx)
    if !detection.Found {
        return TargetResult{Runtime: runtime, Status: "skipped", Warning: "runtime not installed"}
    }

    profile := resolved.RuntimeAgents[runtime]
    plan, err := adp.Plan(ctx, adapter.RenderInput{Active: resolved, Runtime: runtime, Agent: profile, ProjectRoot: cwd})
    if err != nil {
        return failed(runtime, err)
    }

    conflicts, err := s.Conflict.Detect(plan.ManagedPaths, s.State)
    if err != nil {
        return failed(runtime, err)
    }

    action, err := s.Conflict.Resolve(runtime, conflicts)
    if err != nil || action == ConflictSkip {
        return skipped(runtime, err)
    }

    if err := s.Backup.Backup(runtime, plan.ManagedPaths); err != nil {
        return failed(runtime, err)
    }

    result, err := adp.Render(ctx, plan)
    if err != nil {
        return failed(runtime, err)
    }

    s.State.Update(runtime, resolved.Active, profile.Name, result.WrittenFiles, plan.Mappings)
    return ok(runtime, result)
}
```

---

## 冲突检测

只检测 adapter 声明的 `ManagedPaths`。

```go
type ManagedPath struct {
    Path        string
    Owner       string // avm | shared-section
    Description string
    Required    bool
    MergeMode   string // whole-file | marked-block | structured-section
}
```

检测规则：

| 情况 | 行为 |
|------|------|
| state 没记录该文件 | 无冲突，首次写入 |
| 文件不存在 | 无冲突 |
| `whole-file` 且 file hash 与 state 相同 | 无冲突 |
| `marked-block` 且 AVM block hash 与 state 相同 | 无冲突，即使文件其他部分变化 |
| `structured-section` 且 AVM 管理 subtree hash 与 state 相同 | 无冲突，adapter 做结构化 merge |
| AVM 管理 block/subtree 被外部修改 | 冲突，提示 |
| 无法定位管理 block/subtree | 冲突或按 Required=false 跳过 |

AVM block marker 示例：

```text
# BEGIN AVM MANAGED: backend-coder
...
# END AVM MANAGED: backend-coder
```

JSON/TOML 文件不能依赖 marker，必须用结构化 parser 计算 AVM 管理 subtree 的 `managed_hash`，并保留整文件 `file_hash` 作为诊断信息。用户修改非 AVM section 不应触发冲突。

---

## 备份策略

备份目录：

```
~/.avm/backup/<timestamp>/<runtime>/<relative-path>
```

规则：

- 只备份存在且将要写入的 managed paths。
- 备份失败则当前 runtime 不写入。
- 备份目录权限 `0700`。
- 超过 `backup_max_count` 删除最旧快照。

---

## State 更新

`sync-state.json` 保存：

- `last_active`
- 每个 runtime 的 last sync
- 每个 runtime 的 active ref 和 agent_at_sync
- managed file hash
- managed block/subtree hash
- render mapping status
- warnings

`state/current-active` 单独保存一行展示字符串，供 shell prompt 快速读取。它不是 source of truth；`config.yaml.active` 才是持久配置。

格式：

```text
profile:backend-coder
env:backend-dev
```

写入后 `avm status` 可判断：

- synced
- out_of_sync
- skipped
- partial
- unsupported fields

---

## 部分失败语义

| 情况 | 退出码 |
|------|--------|
| active profile/env 解析失败 | 1 |
| 所有 runtime 成功 | 0 |
| runtime 未安装被跳过 | 0 |
| 部分 runtime render 失败 | 1 |
| 只有 unsupported mapping，无写入失败 | 0 |
| 用户选择 skip 某个冲突 runtime | 0，并显示 skipped |

`avm status` 需要显示上一次失败 runtime。

---

## 输出摘要

成功示例：

```text
Switched to profile: backend-coder

Targets:
  codex        synced   backend-coder, 2 MCPs, 4 rendered fields

Warnings:
  codex: agents.backend-coder.memory_refs rendered as instructions
```

失败示例：

```text
Sync partially failed:
  cline: ~/.cline/data/settings/cline_mcp_settings.json modified outside AVM

Synced:
  claude-code
  codex
```
