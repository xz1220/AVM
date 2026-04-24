# Agent VM — 数据模型定义

> 最后更新：2026-04-24（v6 — Portable Memory 对齐）

本文档定义 AVM Phase 1 的持久化 schema。实现时以这些结构为合同；adapter 不得私自引入只存在于某个运行时的主模型字段，运行时特有字段放入 `runtime_extensions`。

Phase 1 的主对象是 **Agent Profile**。`~/.avm/agents/<name>.yaml` 保存 profile 本体，包含能力引用和 memory refs；Environment 只在多 runtime 场景下保存 runtime 到 Agent Profile 的激活映射，不重复声明 capabilities 或 memory。

---

## 1. 全局配置 `~/.avm/config.yaml`

```yaml
version: "1"

active:
  kind: profile                 # profile | env
  name: backend-coder

defaults:
  source_scope: global
  targets:
    - claude-code
    - codex
    - cline
  conflict_strategy: prompt

settings:
  backup_enabled: true
  backup_max_count: 10
  write_mode: managed-only
  shell_prompt:
    enabled: true
    format: "avm:%s"
```

```go
type GlobalConfig struct {
    Version   string         `yaml:"version"`
    Active    ActiveRef      `yaml:"active"`
    Defaults  DefaultsConfig `yaml:"defaults"`
    Settings  Settings       `yaml:"settings"`
}

type ActiveRef struct {
    Kind string `yaml:"kind" json:"kind"` // profile | env
    Name string `yaml:"name" json:"name"`
}

type DefaultsConfig struct {
    SourceScope      string   `yaml:"source_scope"`
    Targets          []string `yaml:"targets"`
    ConflictStrategy string   `yaml:"conflict_strategy"`
}

type Settings struct {
    BackupEnabled  bool   `yaml:"backup_enabled"`
    BackupMaxCount int    `yaml:"backup_max_count"`
    WriteMode       string `yaml:"write_mode"` // managed-only | allow-runtime-overwrite
    ShellPrompt     ShellPromptSettings `yaml:"shell_prompt"`
}

type ShellPromptSettings struct {
    Enabled bool   `yaml:"enabled"`
    Format  string `yaml:"format"` // "avm:%s" renders "(avm:coding)"
}
```

---

## 2. Agent Profile `~/.avm/agents/<name>.yaml`

```yaml
name: backend-coder
description: "Backend implementation agent"
version: "1.0.0"
source_scope: global            # global | project | local

runtime:
  preferred: codex              # claude-code | codex | cline | cursor
  kind: local                   # local | remote
  mode: primary                 # primary | subagent | all
  fallback: [claude-code]

identity:
  display_name: Backend Coder
  role: implementation
  tags: [backend, coding]

instructions:
  system: |
    You implement backend changes with tests.
  developer: |
    Prefer small, reviewable changes.
  references:
    - memory/project/backend-standards.md

model_run:
  model: gpt-5.4
  reasoning_effort: medium
  verbosity: normal
  temperature: 0

io_contract:
  input_modes: [text, files]
  output_style: concise
  language: zh-CN

capabilities:
  skills: [git, test, review]
  mcps: [github, postgres]
  commands: []
  hooks: []
  toolsets:
    shell: limited
    browser: disabled

permissions:
  approval: on-request          # never | on-request | prompt | untrusted
  sandbox: workspace-write      # read-only | workspace-write | danger-full-access
  allow:
    - "Bash(git *)"
  deny:
    - "Bash(rm -rf *)"
  additional_directories: []

memory_refs:
  - id: backend-standards
    scope: project              # user | project | local
    path: ~/.avm/memory/project/backend-standards.md
    mode: read                  # read | append

lifecycle_hooks:
  before_run: []
  after_run: []

runtime_extensions:
  claude-code: {}
  codex: {}
  cline: {}
  cursor: {}
  openclaw: {}
```

```go
type AgentProfile struct {
    Name               string                       `yaml:"name"`
    Description        string                       `yaml:"description"`
    Version            string                       `yaml:"version"`
    SourceScope        string                       `yaml:"source_scope"`
    Runtime            RuntimePreferences           `yaml:"runtime"`
    Identity           AgentIdentity                `yaml:"identity"`
    Instructions       Instructions                 `yaml:"instructions"`
    ModelRun           ModelRun                     `yaml:"model_run"`
    IOContract         IOContract                   `yaml:"io_contract"`
    Capabilities       CapabilityRefs               `yaml:"capabilities"`
    Permissions        Permissions                  `yaml:"permissions"`
    MemoryRefs         []MemoryRef                  `yaml:"memory_refs"`
    LifecycleHooks     LifecycleHooks               `yaml:"lifecycle_hooks"`
    RuntimeExtensions  map[string]map[string]any    `yaml:"runtime_extensions"`
}

type RuntimePreferences struct {
    Preferred string   `yaml:"preferred"`
    Kind      string   `yaml:"kind"`
    Mode      string   `yaml:"mode"`
    Fallback  []string `yaml:"fallback"`
}
```

---

## 3. 环境定义 `~/.avm/envs/<name>.yaml`

Environment 是多 runtime 激活映射。它不重复声明 capabilities、memory refs 或 instructions；这些内容跟随被引用的 Agent Profile。

```yaml
name: coding
description: "后端开发环境"
version: "1.0.0"

runtime_agents:
  codex:
    primary: backend-coder
    available: [backend-coder, backend-reviewer]
  claude-code:
    primary: backend-reviewer
    available: [backend-coder, backend-reviewer]
  cline:
    primary: backend-assistant

targets:
  - codex
  - claude-code
  - cline

runtime_overrides:
  codex:
    model_run:
      reasoning_effort: high
  cline:
    render_agent_as: rules
```

```go
type Environment struct {
    Name             string                  `yaml:"name"`
    Description      string                  `yaml:"description"`
    Version          string                  `yaml:"version"`
    RuntimeAgents    map[string]RuntimeAgent `yaml:"runtime_agents"`
    Targets          []string                `yaml:"targets"`
    RuntimeOverrides map[string]any          `yaml:"runtime_overrides"`
}

type RuntimeAgent struct {
    Primary   string   `yaml:"primary"`
    Available []string `yaml:"available"`
}
```

---

## 4. 项目级覆盖 `<project>/.avm/env.yaml`

```yaml
extends: coding

runtime_agents:
  codex:
    primary: project-backend-coder
  claude-code:
    primary: project-reviewer

targets:
  - codex
  - claude-code

runtime_overrides:
  claude-code:
    permissions:
      additional_directories:
        - ../shared
```

合并规则：

| 字段 | 行为 |
|------|------|
| `runtime_agents.<runtime>.primary` | runtime key 级覆盖 |
| `runtime_agents.<runtime>.available` | 声明时完全替换该 runtime 的 available 列表 |
| `targets` | 声明时完全替换 |
| `runtime_overrides` | runtime + 递归 key 级覆盖 |

---

## 5. Capability Registry

### 5.1 Skills `~/.avm/registry/skills/<name>/`

```
~/.avm/registry/skills/git/
├── SKILL.md
└── meta.yaml
```

```yaml
name: git
kind: skill
description: "Git operation skill"
source: local
source_url: ""
installed_at: "2026-04-24T10:30:00Z"
checksum: "sha256:abc123"
tags: [development, vcs]
runtime_support:
  claude-code: native
  codex: rendered_as_instructions
  cline: rendered_as_instructions
```

### 5.2 MCP `~/.avm/registry/mcps/<name>.yaml`

```yaml
name: github
kind: mcp
description: "GitHub MCP Server"
source: local
server:
  type: stdio                  # stdio | http | sse
  command: npx
  args: ["-y", "@modelcontextprotocol/server-github"]
  env:
    GITHUB_TOKEN: "${GITHUB_TOKEN}"
  url: ""
  headers: {}
policy:
  enabled_tools: []
  disabled_tools: []
  default_approval: on-request
tags: [development, vcs]
```

```go
type MCPConfig struct {
    Name        string            `yaml:"name"`
    Kind        string            `yaml:"kind"`
    Description string            `yaml:"description"`
    Source      string            `yaml:"source"`
    Server      MCPServer         `yaml:"server"`
    Policy      MCPPolicy         `yaml:"policy"`
    Tags        []string          `yaml:"tags"`
}

type MCPServer struct {
    Type    string            `yaml:"type"`
    Command string            `yaml:"command"`
    Args    []string          `yaml:"args"`
    Env     map[string]string `yaml:"env"`
    URL     string            `yaml:"url"`
    Headers map[string]string `yaml:"headers"`
}
```

### 5.3 Commands/Hooks

Phase 1 只建立 registry 和引用模型；只有 Claude Code/Cline 能安全表达时才 native 写入。

```
~/.avm/registry/commands/<name>.yaml
~/.avm/registry/hooks/<name>.yaml
```

---

## 6. Portable Memory

Portable Memory 是 AVM 的可迁移记忆层。Phase 1 支持显式文件引用和 `avm memory import --dry-run`；Phase 2 再支持带 diff 的 runtime native memory push/pull。

```
~/.avm/memory/
├── user/personal-preferences.md
├── project/backend-standards.md
└── local/scratch.md
```

```yaml
# ~/.avm/memory/project/backend-standards.yaml
id: backend-standards
scope: project
format: markdown
path: ~/.avm/memory/project/backend-standards.md
description: "Backend project standards"
mode: read
tags: [backend]
origin:
  type: file                    # file | runtime-native | import
  runtime: ""
  source_path: ""
write_policy:
  allow_push: false
  require_confirmation: true
```

```go
type PortableMemory struct {
    ID          string            `yaml:"id"`
    Scope       string            `yaml:"scope"`  // user | project | local | team
    Format      string            `yaml:"format"` // markdown | yaml
    Path        string            `yaml:"path"`
    Description string            `yaml:"description"`
    Mode        string            `yaml:"mode"`   // read | append
    Tags        []string          `yaml:"tags"`
    Origin      MemoryOrigin      `yaml:"origin"`
    WritePolicy MemoryWritePolicy `yaml:"write_policy"`
}

type MemoryOrigin struct {
    Type       string `yaml:"type"` // file | runtime-native | import
    Runtime    string `yaml:"runtime"`
    SourcePath string `yaml:"source_path"`
}

type MemoryWritePolicy struct {
    AllowPush           bool `yaml:"allow_push"`
    RequireConfirmation bool `yaml:"require_confirmation"`
}
```

### Memory import dry-run report

`avm memory import --dry-run` 不写入 memory 文件，只输出候选 diff，并可保存最近一次报告到 `~/.avm/state/memory-import-report.json` 便于调试。

```json
{
  "runtime": "claude-code",
  "source": "~/.claude/agent-memory/backend-coder/MEMORY.md",
  "dry_run": true,
  "candidates": [
    {
      "id": "backend-standards",
      "scope": "project",
      "status": "new",
      "target_path": "~/.avm/memory/project/backend-standards.md",
      "preview": "API 层不直接访问数据库，必须经过 service。"
    }
  ],
  "conflicts": []
}
```

---

## 7. Active 目录 `~/.avm/active/`

`active/` 是当前环境的实例化结果，供 adapter 读取。

```
~/.avm/active/
├── manifest.yaml
├── agents/
│   ├── backend-coder.yaml
│   └── backend-reviewer.yaml
├── skills/
│   ├── git -> ../../registry/skills/git
│   └── test -> ../../registry/skills/test
├── mcps/
│   ├── github.yaml
│   └── postgres.yaml
├── memory/
│   └── backend-standards.md -> ../../memory/project/backend-standards.md
└── render/
    ├── claude-code/
    ├── codex/
    └── cline/
```

`active/manifest.yaml` 记录本次展开结果：

```yaml
active:
  kind: env
  name: coding
generated_at: "2026-04-24T10:30:00Z"
runtime_agents:
  codex: backend-coder
  claude-code: backend-reviewer
  cline: backend-assistant
profiles:
  - backend-coder
  - backend-reviewer
  - backend-assistant
targets: [codex, claude-code, cline]
```

---

## 8. 同步状态 `~/.avm/state/sync-state.json`

```json
{
  "version": "1",
  "last_active": {
    "kind": "env",
    "name": "coding"
  },
  "targets": {
    "codex": {
      "last_sync": "2026-04-24T10:30:00Z",
      "active_at_sync": {
        "kind": "env",
        "name": "coding"
      },
      "agent_at_sync": "backend-coder",
      "files": {
        "~/.codex/config.toml": {
          "file_hash": "sha256:abc123",
          "managed_hash": "sha256:def456",
          "synced_at": "2026-04-24T10:30:00Z",
          "size": 1024,
          "managed": true
        }
      },
      "mappings": [
        {
          "source_path": "agents.backend-coder.model_run.model",
          "target_path": "~/.codex/agents/backend-coder.toml model",
          "status": "native",
          "reason": ""
        },
        {
          "source_path": "agents.backend-coder.memory_refs",
          "target_path": "agents.backend-coder.developer_instructions",
          "status": "rendered_as_instructions",
          "reason": "Codex has no native memory scope in Phase 1"
        }
      ]
    }
  }
}
```

```go
type SyncState struct {
    Version    string                 `json:"version"`
    LastActive ActiveRef              `json:"last_active"`
    Targets    map[string]TargetState `json:"targets"`
}

type TargetState struct {
    LastSync     string                 `json:"last_sync"`
    ActiveAtSync ActiveRef              `json:"active_at_sync"`
    AgentAtSync  string                 `json:"agent_at_sync"`
    Files        map[string]FileState   `json:"files"`
    Mappings     []FieldMappingState    `json:"mappings"`
}

type FileState struct {
    FileHash    string `json:"file_hash"`
    ManagedHash string `json:"managed_hash"`
    SyncedAt    string `json:"synced_at"`
    Size        int64  `json:"size"`
    Managed     bool   `json:"managed"`
}
```

---

## 9. Import 报告 `~/.avm/state/import-report.json`

`avm init` 和 `avm import` 必须保存最近一次导入报告，便于用户确认候选对象，也便于测试验证只读导入行为。

```json
{
  "generated_at": "2026-04-24T10:30:00Z",
  "confirmed": {
    "agents": ["reviewer"],
    "mcps": ["github"],
    "envs": ["imported-default"]
  },
  "candidates": [
    {
      "kind": "agent",
      "name": "backend-rules",
      "source_path": ".clinerules/backend.md",
      "confidence": "medium",
      "reason": "rules file may represent an agent instruction set"
    }
  ],
  "runtime_extensions": {
    "codex": {}
  },
  "ignored": [
    {
      "source_path": "AGENTS.md",
      "reason": "project guidance imported as instruction reference only"
    }
  ]
}
```

---

## 10. Export 包

`avm export` 输出可迁移包，默认不包含 secrets。

```yaml
version: "1"
exported_at: "2026-04-24T10:30:00Z"
envs:
  - coding
agents:
  - backend-coder
  - backend-reviewer
capabilities:
  skills: [git, test]
  mcps: [github]
memory_refs:
  - backend-standards
include_files:
  - agents/backend-coder.yaml
  - envs/coding.yaml
  - registry/mcps/github.yaml
  - memory/project/backend-standards.md
```
