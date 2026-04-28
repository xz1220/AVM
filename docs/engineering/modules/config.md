# Agent VM — Config 模块设计

> 最后更新：2026-04-27（v5 — 对齐实际文件结构）

Config 模块是 AVM 的基础设施层，负责 source of truth 的读写、验证、默认值和合并。它只处理 AVM 数据模型，不写 runtime 配置。

---

## 职责

1. 读写 `~/.avm/config.yaml`。
2. 读写 `~/.avm/agents/<name>.yaml`。
3. 读写 `~/.avm/envs/<name>.yaml`。
4. 读写 `<project>/.avm/env.yaml` 和项目级 agents。
5. 读写 portable memory metadata。
6. 解析最终生效的 `ResolvedActivation`。
7. 做 schema 验证和默认值填充。

## 不做

- 不写 Claude/Codex/Cline 配置文件。
- 不做 adapter 字段映射。
- 不做文件备份和冲突处理。
- 不执行 MCP/command/hook。

---

## 包结构

```
internal/config/
├── paths.go          # AVM 和项目路径
├── global.go         # GlobalConfig
├── agent.go          # AgentProfile CRUD
├── env.go            # Environment CRUD
├── memory.go         # PortableMemory CRUD
├── models.go         # 数据模型定义
├── resolve.go        # ResolvedActivation
├── merge.go          # 覆盖/合并策略
├── validation.go     # schema/引用校验
├── registry.go       # capability registry 路径
├── yaml.go           # YAML 读写工具
└── doc.go            # 包文档
```

---

## 路径管理

```go
func AvmDir() string
func GlobalConfigPath() string
func AgentsDir() string
func AgentPath(name string) string
func EnvsDir() string
func EnvPath(name string) string
func RegistryDir() string
func RegistryKindDir(kind string) string
func MemoryDir() string
func ActiveDir() string
func StateDir() string
func BackupDir() string

func ProjectAvmDir(cwd string) string
func ProjectEnvPath(cwd string) string
func ProjectAgentsDir(cwd string) string
```

路径函数必须只返回绝对路径，调用方负责创建目录。

---

## 默认值

```go
func (c *GlobalConfig) ApplyDefaults() {
    if c.Version == "" {
        c.Version = "1"
    }
    if c.Active.Kind == "" {
        c.Active.Kind = "profile"
    }
    if c.Active.Name == "" {
        c.Active.Name = "default"
    }
    if c.Defaults.SourceScope == "" {
        c.Defaults.SourceScope = "global"
    }
    if len(c.Defaults.Targets) == 0 {
        c.Defaults.Targets = []string{"claude-code", "codex", "opencode"}
    }
    if c.Defaults.ConflictStrategy == "" {
        c.Defaults.ConflictStrategy = "prompt"
    }
    if c.Settings.BackupMaxCount == 0 {
        c.Settings.BackupMaxCount = 10
    }
    if c.Settings.WriteMode == "" {
        c.Settings.WriteMode = "managed-only"
    }
    if c.Settings.ShellPrompt.Format == "" {
        c.Settings.ShellPrompt.Format = "avm:%s"
    }
}
```

Agent 默认值：

| 字段 | 默认 |
|------|------|
| `version` | `1.0.0` |
| `source_scope` | global config default |
| `runtime.kind` | `local` |
| `runtime.mode` | `primary` |
| `model_run.reasoning_effort` | `medium` |
| `model_run.verbosity` | `normal` |
| `permissions.approval` | `on-request` |
| `permissions.sandbox` | `workspace-write` |

---

## 读写 API

```go
func ReadGlobalConfig() (*GlobalConfig, error)
func WriteGlobalConfig(cfg *GlobalConfig) error
func UpdateActive(ref ActiveRef) error

func ReadAgent(name string, scope Scope, cwd string) (*AgentProfile, error)
func WriteAgent(agent *AgentProfile, scope Scope, cwd string) error
func ListAgents(scope Scope, cwd string) ([]AgentSummary, error)

func ReadEnvironment(name string) (*Environment, error)
func WriteEnvironment(env *Environment) error
func ListEnvironments() ([]EnvironmentSummary, error)

func ReadProjectOverride(cwd string) (*ProjectOverride, error)
func WriteProjectOverride(cwd string, override *ProjectOverride) error
```

写文件要求：

- 使用临时文件 + rename。
- 保持 YAML 字段顺序尽量稳定。
- 不写空 map/list，除非该空值有覆盖语义。

---

## ResolveActivation

`ResolveActivation` 是 config 层最重要的 API。它接收 `profile` 或 `env` active ref，返回 sync/adapter 可直接消费的展开结果。

```go
type ResolvedActivation struct {
    Active        ActiveRef
    Env           *Environment
    RuntimeAgents map[string]AgentProfile
    Capabilities  map[string]ResolvedCapabilities
    Memory        map[string][]PortableMemory
    Targets       []string
    SourceFiles   []string
    Warnings      []string
}

func ResolveActivation(ref ActiveRef, cwd string) (*ResolvedActivation, error)
```

流程：

1. 如果 active kind 是 `profile`，读取同名 Agent Profile。
2. 如果 active kind 是 `env`，读取全局 env；如果 `<project>/.avm/env.yaml` 存在，读取并按 `extends` 选择 base env，再合并 project override。
3. 根据 active kind 生成 runtime 到 profile 的绑定：
   - profile：默认绑定到 `agent.runtime.preferred`，fallback 可作为 status 提示，不自动写多个 runtime。
   - env：按 `runtime_agents.<runtime>.primary` 绑定。
4. 展开 Agent Profile：
   - 优先项目级 `.avm/agents/<name>.yaml`
   - 再读全局 `~/.avm/agents/<name>.yaml`
5. 从每个 Agent Profile 展开 capability refs。
6. 从每个 Agent Profile 展开 memory refs。
7. 对 env active 应用 `runtime_overrides`。
8. 校验引用和 target。

---

## 合并策略

```go
func MergeEnvironment(base *Environment, override *ProjectOverride) *Environment
```

| 字段 | 策略 |
|------|------|
| `runtime_agents.<runtime>.primary` | runtime key 级覆盖 |
| `runtime_agents.<runtime>.available` | override 非 nil 时完全替换该 runtime 的列表 |
| `targets` | override 非 nil 时完全替换 |
| `runtime_overrides` | runtime + 递归 key 级覆盖 |

实现上需要区分 nil 和空列表：

- nil：未声明，继承 base。
- 空列表：显式清空。

因此 Go struct 中列表字段建议用 `[]string` 配合 YAML node presence，或自定义 `OptionalList[T]`。

---

## 验证规则

### 名称

```go
var nameRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
```

适用于：

- agent name
- env name
- capability name
- memory id

### target

Phase 1 已知 target：

```go
var KnownTargets = map[string]TargetCapability{
    "claude-code": {Level: "full"},
    "codex":       {Level: "full"},
    "cline":       {Level: "full"},
    "cursor":      {Level: "partial"},
}
```

OpenClaw 可出现在 `runtime_extensions`，但不能作为 Phase 1 target，除非显式启用 experimental。

### 引用

`ValidateResolvedActivation` 必须检查：

- active profile 存在，或 env 的 `runtime_agents.<runtime>.primary` 引用的 profile 存在。
- agent 的 `runtime.preferred` 和 `runtime.fallback` 均为已知 runtime，或作为 runtime extension 明确标记 unsupported。
- agent 引用的 skills/MCP/memory 存在。
- MCP server 至少有 `command` 或 `url`。
- memory path 存在，且不越权到不可读路径。
- permissions/sandbox 枚举合法。

---

## Capability 读取

> 注意：以下函数为 Phase 2 规划，当前未实现。Phase 1 中 capability 引用通过 AgentProfile 的 `capabilities` 字段直接内联定义。

```go
// Phase 2 planned
func LoadSkills(names []string) ([]Skill, error)
func LoadMCPs(names []string) ([]MCPConfig, error)
func LoadCommands(names []string) ([]CommandConfig, error)
func LoadHooks(names []string) ([]HookConfig, error)
```

引用不存在时默认返回错误；CLI 可通过 `--allow-missing` 转成 warning。

---

## Import 写入策略

adapter.Import 产生候选对象后，config 模块负责落盘：

```go
func SaveImported(result *adapter.ImportResult, opts ImportOptions) error
```

冲突策略：

| 情况 | 默认行为 |
|------|----------|
| 同名 agent 内容相同 | skip |
| 同名 agent 内容不同 | rename 为 `<name>-imported` 并提示 |
| 同名 MCP 内容相同 | skip |
| 同名 MCP 内容不同 | fail，要求用户指定 overwrite/rename |
| 无法识别字段 | 存入 `runtime_extensions.<runtime>` |

`SaveImported` 只落盘 `result.Confirmed` 中的对象；`Candidates`、`Ignored` 和 `RuntimeExtensions` 必须写入 import report，并在 CLI 输出中展示，不能静默丢弃。

---

## 错误格式

配置错误必须包含文件路径和字段路径：

```
~/.avm/agents/backend.yaml: capabilities.mcps[1]: mcp "postgres" not found
~/.avm/agents/backend.yaml: permissions.sandbox: invalid value "full"
```

YAML 解析错误应尽量保留行列号。
