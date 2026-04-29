# Agent VM — 激活模型设计

> 最后更新：2026-04-27
> 目的：明确 AVM 当前的生效机制、它和 GVM/NVM 式 shell 环境切换的差异，以及后续应补齐的 shell-local activation 设计。

---

## 1. 当前结论

AVM Phase 1 当前不是一个纯粹依赖软连接生效的工具，也不是 GVM 那种只依赖当前 shell 环境变量生效的工具。

当前实际模型是：

1. `~/.avm` 是 source of truth，保存 Agent Profile、Environment、capability registry、portable memory 和 sync state。
2. `avm use` 解析 active profile/env，重建 `~/.avm/active/`。
3. sync 层调用各 runtime adapter，把统一模型写入 runtime 原生配置文件。
4. `avm shell init` 只读取 `state/current-active` 展示 prompt，不创建 shell-local runtime home。

所以当前 AVM 更接近“持久化配置翻译器”：一次 `avm use` 会修改 Codex、Claude Code、Cline、Cursor 等 runtime 的原生配置，后续从任意 shell 启动这些工具都可能看到同一个持久 active 状态。

这和 “gvm for agents” 的用户直觉存在差异。GVM 的核心体验是当前 shell 生效：`gvm use` 通过被 shell source 的函数修改 `GOROOT`、`GOPATH` 和 `PATH`，影响当前 shell 及其子进程，而不是全局改写某个运行时配置。

---

## 2. 软连接在 AVM 里的真实角色

`~/.avm/active/` 是当前 profile/env 的派生展开目录，不是最终生效目录。

Phase 1 中软连接只用于减少重复复制，例如 active skills：

```text
~/.avm/active/
├── manifest.yaml
├── agents/
├── skills/
│   └── git -> ~/.avm/registry/skills/git
└── render/
```

如果创建 symlink 失败，代码会 fallback 到 copy。也就是说，软连接是 active tree 的实现细节，不是 AVM 激活 runtime 的核心机制。

真正让 runtime 生效的是 adapter render：

| Runtime | 当前写入目标 | 生效模型 |
|---------|--------------|----------|
| Codex | `$CODEX_HOME/config.toml`、`$CODEX_HOME/agents/*.toml`、`$CODEX_HOME/skills/*/SKILL.md` | 写原生 Codex 配置 |
| Claude Code | `~/.claude/agents/*.md`、skills、项目 `.mcp.json` | 写 Claude Code 原生/项目配置 |
| Cline | `.clinerules/avm/*.md`、`cline_mcp_settings.json` | 写规则和 Cline 设置 |
| Cursor | `.cursor/rules/*.md`、`.cursor/mcp.json` | 写项目配置 |

---

## 3. 当前设计的问题

### 3.1 无法表达 shell-local active

如果两个终端分别想使用不同 agent：

```bash
# terminal A
avm use backend-coder
codex

# terminal B
avm use reviewer
codex
```

当前实现会竞争同一组 runtime 配置，例如 `~/.codex/config.toml`。后执行的 `avm use` 会覆盖前一个 shell 的语义。

这不符合 GVM/NVM 类工具的直觉：版本或 profile 应该可以在不同 shell 中并存。

### 3.2 污染用户真实 runtime home

当前 `avm use` 直接写用户真实 runtime 配置目录。虽然有 managed block、conflict detection 和 backup，但它仍然会改变用户原始配置环境。

对于 Codex 这类 CLI，如果 runtime 本身支持 `CODEX_HOME`，更理想的体验是给当前 shell 分配一个 AVM-managed Codex home，而不是修改用户真实 `~/.codex`。

### 3.3 shell init 只展示状态

当前 `avm shell init` 只做 prompt 展示：

```text
(avm:backend-coder) ~/repo $
```

它不会导出 `CODEX_HOME`、`CLAUDE_CONFIG_DIR`、`CLINE_DATA_HOME`，也不会让后续启动的 agent CLI 自动进入 AVM 隔离环境。

因此 prompt 里的 active 状态和 runtime 的真实读取路径之间没有 session-level 绑定。

---

## 4. 目标模型：持久渲染 + shell-local 激活

后续建议采用混合模型，而不是把当前实现完全推倒。

### 4.1 保留持久渲染模式

持久渲染模式适合：

- GUI/IDE 插件：Cursor、Cline、VS Code extension 不一定从当前 shell 继承环境变量。
- 项目级配置：`.cursor/rules`、`.clinerules`、`.mcp.json` 本来就是项目文件。
- 用户明确希望把某个 AVM profile 应用到默认 runtime home。

现有 `avm use` 可以继续承担这个角色，或者未来显式命名为：

```bash
avm use backend-coder --persist
avm sync
```

### 4.2 新增 shell-local 激活模式

新增一个和 GVM 更接近的模式：

```bash
eval "$(avm activate backend-coder)"
codex
```

或由 shell integration 包装成：

```bash
avm use backend-coder
codex
```

但在 shell integration 启用时，`avm use` 应该修改当前 shell 的环境，而不是只写持久 runtime 配置。

激活后导出的环境变量示例：

```bash
export AVM_HOME="$HOME/.avm"
export AVM_ACTIVE="profile:backend-coder"
export AVM_ACTIVE_DIR="$HOME/.avm/active"
export AVM_STATE_DIR="$HOME/.avm/state"

export CODEX_HOME="$HOME/.avm/runtime-homes/profile-backend-coder/codex"
export CLAUDE_CONFIG_DIR="$HOME/.avm/runtime-homes/profile-backend-coder/claude"
export CLINE_DATA_HOME="$HOME/.avm/runtime-homes/profile-backend-coder/cline-data"
```

adapter 在这个模式下不写用户真实 runtime home，而是写 AVM-managed runtime home：

```text
~/.avm/runtime-homes/
└── profile-backend-coder/
    ├── codex/
    │   ├── config.toml
    │   ├── agents/backend-coder.toml
    │   └── skills/
    ├── claude/
    └── cline-data/
```

这样不同 shell 可以有不同的 active profile，并且互不覆盖。

---

## 5. Runtime 分层策略

不同 runtime 对环境变量的支持程度不一样，不能强行用同一种机制。

| Runtime 类型 | 建议策略 | 说明 |
|--------------|----------|------|
| CLI runtime，支持 config home env | shell-local 优先 | Codex 可通过 `CODEX_HOME` 指向 AVM-managed home |
| CLI runtime，部分支持 env | shell-local + adapter fallback | Claude Code 可尝试 `CLAUDE_CONFIG_DIR`，但需以真实 runtime 行为验证 |
| IDE/GUI extension | 项目级/持久渲染优先 | Cursor/Cline 可能不继承当前 shell 环境 |
| 项目 rules/MCP | 项目文件渲染 | `.cursor/`、`.clinerules/`、`.mcp.json` 本身就是 workspace state |

核心原则：

1. 能用环境变量隔离的 CLI runtime，不默认污染用户真实 home。
2. 必须写项目文件的 runtime，继续走 managed path、conflict detection 和 backup。
3. adapter 必须报告当前字段是 native、rendered_as_instructions、ignored 还是 unsupported。

---

## 6. 实现影响

### 6.1 `AVM_HOME` 支持

当前 AVM 自己的 home 固定为 `$HOME/.avm`。要实现 GVM-like 激活，AVM 应该读取：

```bash
AVM_HOME
```

优先级建议：

1. 显式 CLI flag，例如 `--avm-home`
2. `AVM_HOME`
3. `$HOME/.avm`

这能支持测试、隔离和项目级实验环境。

### 6.2 新增 shell 输出命令

需要新增 eval-safe 命令，例如：

```bash
avm activate <profile-or-env>
avm deactivate --shell
```

输出内容必须只包含 shell assignment，不能混入日志。bash/zsh/fish 需要分别处理。

### 6.3 shell init 从“prompt hook”升级为“activation hook”

`avm shell init zsh` 安装 shell function，让用户执行：

```bash
avm use backend-coder
```

时可以在当前 shell 中生效。普通二进制进程无法修改父 shell 环境，所以这一步必须通过 shell function 完成。

### 6.4 adapter 输入支持 runtime home override

sync/render 输入需要能携带目标 runtime home：

```go
type RenderInputOptions struct {
    ProjectRoot  string
    ActiveDir    string
    RuntimeHomes map[string]string
}
```

或在 adapter registry 构造时注入 runtime-specific config dir。

### 6.5 状态区分 active mode

`state/current-active` 只记录 active ref 不够，还需要区分：

```yaml
active: profile:backend-coder
mode: shell-local | persistent
runtime_homes:
  codex: ~/.avm/runtime-homes/profile-backend-coder/codex
```

否则 `avm status` 无法解释当前 shell 的 active 和全局持久 active 是否一致。

---

## 7. 兼容性建议

短期不要破坏已有命令语义。

当前 runtime home isolation 重构已经落地第一步：

- Codex 和 Claude Code 的 `avm use` / `avm sync` 默认渲染到 `~/.avm/runtime-homes/<active>/...`，不再写用户真实 `~/.codex`、`~/.claude` 或项目 `.claude/agents`。
- `avm activate <profile-or-env>` 输出 eval-safe shell assignments，包括 `CODEX_HOME`、`CLAUDE_CONFIG_DIR` 和 `AVM_CLAUDE_MCP_CONFIG`。
- `avm shell init` 仍负责 prompt 展示，并在有 `AVM_CLAUDE_MCP_CONFIG` 时包装 `claude` 命令，自动传入 `--strict-mcp-config --mcp-config=<file>`。这里使用等号形式，避免 Claude Code 的可变长 `--mcp-config` 参数吞掉后面的 prompt 或 `agents` 等子命令。
- Codex runtime home 重建时会保留或复制 `auth.json` 认证侧车文件，避免 `CODEX_HOME` 指到隔离目录后真实 `codex exec` 变成未登录状态；原始 `~/.codex` 仍只读、不回写。
- Claude Code runtime home 重建时会保留或复制 `.credentials.json` 认证侧车文件。Claude Code 官方认证文档说明 Linux/Windows 凭据默认在 `~/.claude/.credentials.json`，设置 `CLAUDE_CONFIG_DIR` 后则在该目录下的 `.credentials.json`。

推荐路线：

1. 保留非 CLI runtime 的持久/项目渲染行为。
2. 继续让 `avm activate` 专门输出 shell-local env。
3. 让 `avm shell init` 可选地把 `avm use` 包装为 shell-local activation。
4. 文档中明确区分：
   - `avm activate`: 当前 shell 生效，适合 Codex 等 CLI。
   - `avm use`: 重建 active 和 AVM-owned runtime homes；不修改用户真实 Codex/Claude Code home。
5. 等 shell-local 模型稳定后，再评估是否把 `avm use` 默认语义切换到 shell-local。

---

## 8. 设计判断

AVM 当前设计不是“软连接方案有问题”，而是缺少一层 shell-local activation。

合理的长期架构应该是：

```text
AVM source of truth
  -> active composition
  -> shell-local runtime homes for CLI tools
  -> persistent/project render for IDE tools
```

这样才能同时满足两类使用场景：

- 像 GVM/NVM 一样，在不同 shell 中快速切换不同 agent profile。
- 像配置管理器一样，把 profile 安全投影到 Cursor、Cline、Claude Code 等需要文件配置的 runtime。
