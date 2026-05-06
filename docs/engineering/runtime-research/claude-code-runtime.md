# Claude Code 运行时源码调研

本文基于 `frameworks/claude-code-main` 源码，目标只有一个：判断 Agent VM PRD 里关于 runtime 的假设在 Claude Code 上是否成立。行号引用为调研当时的本地 checkout，相对路径都从 `frameworks/claude-code-main/` 起算。

## 一、核心结论（摘要）

1. Claude Code runtime 是 TypeScript 实现，入口 `src/entrypoints/cli.tsx` → `src/main.tsx`。CLI 启动时先做早期 flag 分发（daemon、MCP 服务、Chrome native host、worktree+tmux、bare 模式等），再进入正常 runtime；正常 runtime 由 `init()` 和 `setup()` 两阶段建立工作区与会话上下文（`src/entrypoints/cli.tsx:28`、`287`，`src/main.tsx:585`、`905`，`src/setup.ts:56`）。

2. Claude Code 的**配置是多源合并**的：user `~/.claude/settings.json` < project `.claude/settings.json` < local `.claude/settings.local.json` < flag file < managed policy。AVM 想"写入 runtime managed config"是可以落在 user 或 project 层的，这条路径是通的（`src/utils/settings/constants.ts:3`、`src/utils/settings/settings.ts:274`）。

3. Claude Code **没有统一 extension registry**。Skills 来自 7 个来源（managed/user/project/additional dirs/plugin/bundled/MCP），MCP 来自 8 个 scope（enterprise/local/project/user/plugin/dynamic/managed/Claude.ai）。PRD 说的"全量发现"在 Claude Code 这里是**多源合并**，且每个来源都有自己的 trust/approval 闸口。

4. Claude Code 的**隔离不是一个开关**。默认 sandbox `enabled: false`，落地的隔离能力分散在四个正交维度：permission rules（tool allow/deny）、filesystem path classifier（敏感路径硬拦截）、可选的 `@anthropic-ai/sandbox-runtime`（平台依赖、默认关）、subprocess 边界（每条 shell 命令一个进程）。PRD 把 sandbox 当一个 mapping 字段会丢语义（`src/utils/sandbox/sandbox-adapter.ts:459`、`532`，`src/utils/permissions/permissions.ts:473`，`src/utils/Shell.ts:177`）。

5. Claude Code 的状态边界**不止 `~/.claude`**。Global config 在 `~/.claude.json`（独立文件，不在 `.claude/` 里），plugin cache 可被 `CLAUDE_CODE_PLUGIN_CACHE_DIR` 改向，env-paths cache 走 XDG，secure storage 在非 macOS 上 fallback 到明文 `~/.claude/.credentials.json`。PRD 想做"AVM home"边界要意识到这是**多目录**而不是单点（`src/utils/env.ts:13`，`src/utils/plugins/pluginDirectories.ts:49`，`src/utils/cachePaths.ts:1`，`src/utils/secureStorage/plainTextStorage.ts:13`）。

6. Claude Code 的**项目身份是 canonical git root**，不是 cwd。Auto-memory、project memory、session 历史都按 canonical path 索引，**worktree 共享 memory**。PRD 想把每个 worktree 当独立 agent，默认会撞车（`src/bootstrap/state.ts:45`、`496`，`src/memdir/paths.ts:198`）。

7. Project-level 配置（`.mcp.json`、`.claude/settings.json`、`headersHelper`、project memory）默认**不可信**：未通过 trust gate 之前，env 变量、MCP server、headersHelper 都被 hold 住。AVM 写 project 层的东西要意识到首次运行需要 user trust（`src/utils/managedEnv.ts:93`、`124`，`src/services/mcp/utils.ts:351`，`src/services/mcp/headersHelper.ts:40`）。

**验证局限**：当前 framework 快照只有 `src/`、`Doc/`、`README.md`、`CLAUDE.md`，**没有 `package.json`、lockfile、build artifact**，无法跑 `--help`、build 或 test 验证。所有结论都来自 TypeScript 源码追踪。

---

## 二、PRD 可行性对照

按 PRD 模块逐条对齐。"可行"指 Claude Code 有现成能力；"有坑"指能做但要处理额外语义；"不可行"指结构上不支持。

| PRD 诉求 | Claude Code 现状 | 判断 |
|---|---|---|
| **AVM 把 Agent 渲染为 runtime managed config** | settings 是 JSON，分 user/project/local 三层，AVM 写哪一层都行。Managed policy 层属于 OS 全局策略目录（Linux `/etc/claude-code` 等），不适合 AVM 写。 | **可行**。 |
| **Agent 绑定 instructions** | `CLAUDE.md` 多层加载（managed/user/project/`.claude/CLAUDE.md`/`.claude/rules/*.md`/local），支持 include 引用文件，上限 40000 字符。AVM 可以 map 到 user 或 project `CLAUDE.md`。 | **可行**。 |
| **Agent 绑定 skills（含来源区分）** | Skills 从 7 个来源加载：managed、user `~/.claude/skills`、project `.claude/skills`、`--add-dir`、plugin、bundled（编译进 CLI）、MCP server 提供。`LoadedFrom` enum 自带来源字段。形态上 skill 必须是 `skills/<name>/SKILL.md`（不是单文件 markdown）。 | **可行但有坑**。来源现成，但 nested discovery（从操作文件向 cwd 走的过程中累加 `.claude/skills`）是 AVM 没设计过的发现路径。 |
| **Agent 绑定 MCP servers（含来源区分）** | MCP 来自 8 个 scope：enterprise/local/project/user/plugin/dynamic/managed/Claude.ai。`mcp add` 命令按 `--scope` 写到不同位置（project 写 `.mcp.json`、user 写 global config、local 写 project config 块）。 | **可行但有坑**。Project 层的 `.mcp.json` 首次使用需要 user approve；plugin-only policy 会让所有非 plugin/enterprise MCP 失效。AVM 要决定写哪一层。 |
| **Runtime mapping 状态：native / rendered_as_instructions / ignored / unsupported** | instructions、skills、MCP 都有 native 字段。permission/sandbox 是多维组合，AVM 单字段表达会降级到 partial native 或 rendered_as_instructions。 | **可行**。 |
| **全量发现 skills/MCP（包括 runtime 全局目录里非 AVM 管理的）** | Skills 每次都按 7 路扫；MCP 每次都按 scope 合并。AVM 在 create/edit 时调一次发现就能拿到全集。bare 模式会跳过自动发现，AVM 不要在 bare 模式下做 discovery。 | **可行**。 |
| **AVM 不能静默覆盖 runtime 全局能力** | Skills loader 按 source 排序、realpath 去重、first-wins，不互删。MCP 不会神奇合并同名 server。AVM 只要不主动改不属于自己的键就安全。 | **可行**。 |
| **agent/runtime 隔离边界（不含 memory）** | 硬隔离只能靠环境变量切目录：`CLAUDE_CONFIG_DIR` 改 settings 与 global config 根，`CLAUDE_CODE_PLUGIN_CACHE_DIR` 改 plugin cache，`CLAUDE_CODE_TMPDIR` 改临时目录。但有些路径（debug log、secure storage、env-paths cache）不一定都跟着 `CLAUDE_CONFIG_DIR` 走。 | **可行但有坑**。**没有一个总开关**能把所有 Claude Code 状态搬到隔离目录。 |
| **Package 导入导出** | Claude Code 有 plugin 体系：marketplace + versioned cache + `installed_plugins.json` + per-plugin data dir，plugin 能携带 skills + commands + agents + MCP。语义上比 AVM package 重。 | **有坑**。AVM package 可以不走 plugin 体系自己渲染；要导出成 plugin 则要对齐 `.claude-plugin/plugin.json` schema。 |
| **Memory 只做隔离不做内容管理** | 三套 memory 同时跑：CLAUDE.md（被动加载）、auto-memory（按 canonical git root 写文件，**worktree 共享**）、agent memory（user/project/local scope）、session memory（forked subagent + post-sampling hook）。auto-memory 默认开，写 `~/.claude/projects/<git-root>/memory/MEMORY.md`。 | **有坑**。"不管 memory" ≠ "memory 不发生"。worktree 共享 auto-memory 这点会让 PRD 的"不同 agent 不串状态"假设失效。 |

**一句话**：PRD 主线（Agent CRUD + runtime managed config + 全量发现 + runtime mapping）在 Claude Code 上能做。三个主要坑：**隔离粒度不存在单一目录开关**、**worktree 共享 auto-memory**、**project 层配置默认不可信，首次需要 trust**。

---

## 三、运行时启动路径

### 入口分发（cli.tsx 阶段）

`src/entrypoints/cli.tsx` 在 import 完整 runtime 之前先做几件事：
- 设置进程 flag：`COREPACK_ENABLE_AUTO_PIN=0`，remote 模式追加 `--max-old-space-size=8192`，处理早期 ablation 环境（`cli.tsx:1`、`7`、`21`）。
- 解析原始 argv，处理 `--version`（`cli.tsx:33`）。
- 分发特殊入口，**不进正常 runtime**：Chrome MCP / native host / computer-use MCP（`cli.tsx:72-87`）、daemon / daemon-worker / remote-control / bridge / remote-sync / background-session（`cli.tsx:95-182`）、`--worktree --tmux`（可能 exec 进 tmux，`cli.tsx:247`）、`--bare`（设 `CLAUDE_CODE_SIMPLE=1`，`cli.tsx:281`）。
- 正常路径捕获 early input 后 `import('../main.js')` 并调 `cliMain()`（`cli.tsx:287`）。

### 主 runtime（main.tsx 阶段）

`src/main.tsx` 用 Commander 组织 CLI：
- `main()` 设 `NoDefaultCurrentDirectoryInExePath=1`，装 signal handler，分 interactive / non-interactive 模式（`main.tsx:585`、`595`、`797`）。
- Client type 从 env、entrypoint、session-ingress、runtime context 推断（`main.tsx:817`）。
- Settings 在 command runner 构造前 eager load（`main.tsx:851`）。
- Commander `preAction` 等 managed policy / keychain prefetch，调用 `init()`，初始化 process metadata 和 event sink，传播 plugin-dir settings，跑 migration，启动后台 remote settings/policy sync（`main.tsx:905`）。

CLI flag 直接对应几条 runtime 边界：
- 权限：`--dangerously-skip-permissions`、`--allow-dangerously-skip-permissions`、`--permission-mode`、`--allowed-tools`、`--tools`、`--disallowed-tools`（`main.tsx:968`）。
- MCP：`--mcp-config`、`--strict-mcp-config`（`main.tsx:986`、`1003`）。
- 状态/会话：`--continue`、`--resume`、`--fork-session`、`--no-session-persistence`、`--session-id`（`main.tsx:991`、`1005`）。
- 工作区扩展：`--add-dir`、`--agents`、`--setting-sources`、`--plugin-dir`、`--disable-slash-commands`（`main.tsx:999`、`1006`）。

### init() 与 setup() 两阶段初始化

**`init()`** 在项目 trust 前只应用可信环境来源（user / flag / policy），不读 project / local，因为它们可能被攻击者用来重定向流量到恶意代理（`src/entrypoints/init.ts:57`、`src/utils/managedEnv.ts:93`、`124`）。Trust 后才把所有 settings 与 global config 的环境变量都加载，并重置 proxy/mTLS/CA cache（`managedEnv.ts:180`）。

**`setup()`** 真正建立工作区上下文（`src/setup.ts:56`）：
- 校验 Node ≥ 18；可选切换到 custom session ID（`setup.ts:69`、`81`）。
- 启动 Unix domain socket messaging server，导出 `CLAUDE_CODE_MESSAGING_SOCKET`（`setup.ts:86`）。
- 设置 cwd、快照 hook config、启 file-changed watcher（`setup.ts:160`）。
- `--worktree` 模式：校验 git 状态、创建/进入 worktree、可选管理 tmux、改进程 cwd、更新 `originalCwd` 与 `projectRoot`、清空 memory cache（`setup.ts:174`）。
- non-bare 模式才初始化 session memory、attribution、session file access hooks、team memory watcher（`setup.ts:293`、`336`）。

**Bare 模式**默认禁用 hooks、LSP、plugin sync、skill directory walking、attribution、background prefetches、keychain reads（`src/utils/envUtils.ts:49`）。AVM 想做"最小副作用启动"可以用 `--bare`，但代价是 skill / plugin 自动发现也没了。

**对 AVM 的含义**：AVM 既可以在 `avm run` 时用 CLI flag 控制 Claude Code 行为（最干净），也可以预写 settings 文件让用户后续 `claude` 直接拉到。两路并存。

---

## 四、配置与设置分层

### Settings 合并顺序

后者覆盖前者（`src/utils/settings/constants.ts:3`、`src/utils/settings/settings.ts:274`）：

```
user      ~/.claude/settings.json
  → project     .claude/settings.json
  → local       .claude/settings.local.json
  → flag        --settings 指向的文件
  → policy      managed settings
```

Managed settings 路径默认：
- Linux `/etc/claude-code`，macOS `/Library/Application Support/ClaudeCode`，Windows `C:\Program Files\ClaudeCode`（`src/utils/settings/managedPath.ts:8`）。
- 同时支持 `managed-settings.d/*.json` drop-in，按字母序覆盖（`settings.ts:55`、`74`）。

### Global config

- 默认 `~/.claude.json`，可被 `CLAUDE_CONFIG_DIR` 改成 `$CLAUDE_CONFIG_DIR/.claude.json`（`src/utils/env.ts:13`）。
- Config home 默认 `~/.claude`，同样受 `CLAUDE_CONFIG_DIR` 控制（`src/utils/envUtils.ts:5`）。
- Global config 内含 project map（`.claude.json` 里以 cwd 索引）、global MCP servers、auth metadata、env、trusted dirs。这是**和 settings 文件并列的另一份状态**，AVM 想要完整捕获用户配置必须两个都看。

### Trust gate

- 项目第一次跑时未信任，**project 与 local settings 的 env 变量、MCP headersHelper、project MCP server 都被 hold**（`src/utils/managedEnv.ts:93`、`src/services/mcp/utils.ts:351`、`src/services/mcp/headersHelper.ts:40`）。
- Trust 状态记录在 global config 的 trusted dirs 里。
- Bypass mode（`--dangerously-skip-permissions`）受额外约束：root 用户必须有 sandbox marker；Ant 内部 build 还要求 Docker / bubblewrap / `IS_SANDBOX=1` 且无网（`src/setup.ts:395`、`414`）。

**对 AVM 的含义**：
- AVM 写 project `.claude/settings.json` 是合法的，但要么提示用户 trust，要么把关键内容（env、MCP）写到 user 层。
- "AVM home"边界至少要管两个路径：`$CLAUDE_CONFIG_DIR`（默认 `~/.claude`）和 `$CLAUDE_CONFIG_DIR/.claude.json`。

---

## 五、隔离模型

PRD 单字段 sandbox 在 Claude Code 这里要展开成四个正交维度：

| 维度 | 取值 / 形态 | 源码 |
|---|---|---|
| Permission rules | tool allow/ask/deny pattern，按 source（user/project/local/CLI/command/session）合并 | `src/utils/permissions/permissions.ts:109`、`473`，`permissionSetup.ts:721` |
| Filesystem path classifier | 敏感 basename / dir / 内部路径硬拦截，与 permission rules 正交 | `src/utils/permissions/filesystem.ts:57`、`74`、`1030` |
| External sandbox runtime | `@anthropic-ai/sandbox-runtime`，受平台/依赖/settings 控制；**默认关** | `src/utils/sandbox/sandbox-adapter.ts:459`、`532` |
| Subprocess 边界 | 每条 shell 命令一个新进程；可选套 sandbox wrapper；temp dir mode 0700 | `src/utils/Shell.ts:177`、`259`、`315` |

### Permission rules

- 中心化的 `hasPermissionsToUseTool()` 是所有 tool 调用的闸口（`permissions.ts:473`）。
- 规则来源：所有启用的 settings sources + CLI args + commands + session（`permissionsLoader.ts:120`、`permissions.ts:109`）。可编辑来源是 user / project / local（`permissionsLoader.ts:151`）。
- Auto mode 会检测并移除过宽的 Bash/PowerShell 权限（`permissionSetup.ts:948`）。
- Policy 可以限定为 managed rules only（`permissionsLoader.ts:120`）。

### Filesystem 边界

- 敏感 basename：shell rc files、git config/module、`.mcp.json`、Claude config（`filesystem.ts:57`）。
- 敏感目录：`.git`、`.vscode`、`.idea`、`.claude`（`filesystem.ts:74`）。
- 允许的 working dirs = original cwd + `--add-dir`（`filesystem.ts:667`）。
- `checkReadPermissionForTool()` 阻止 UNC / suspicious path，按 deny→ask→allow 顺序判断（`filesystem.ts:1030`）。
- Tool result 目录、当前 session scratchpad、**同 project 跨 session 的 project temp dir**、agent memory / auto-memory 路径都是 special readable internal path（`filesystem.ts:1660`、`1676`、`1688`、`1703`）。
- Skill 路径识别：`.claude/skills/<skill>/**`（`filesystem.ts:94`）；Claude 自有配置文件（settings/commands/agents/skills 在 `.claude` 下）（`filesystem.ts:224`）。

### External sandbox（可选）

`@anthropic-ai/sandbox-runtime` 由 sandbox-adapter 委托（`src/utils/sandbox/sandbox-adapter.ts:1`）。打开后：
- Settings permission rules → sandbox filesystem path rules（`sandbox-adapter.ts:83`、`121`）。
- 网络 allow/deny 来自 `sandbox.network.allowedDomains` 和 `WebFetch(domain:...)` 规则（`sandbox-adapter.ts:177`）。
- 写权限：默认允许 `.` 和 Claude temp dir；settings paths / managed drop-ins / `.claude/skills` 强制 deny；bare git repo 文件 deny 或 scrub（`sandbox-adapter.ts:222`、`230`、`238`、`257`）。
- 配置允许时 worktree main repo 和 `--add-dir` 加入 allow（`sandbox-adapter.ts:282`）。

**默认值**：`enabled: false`、`autoAllow: true`、`allowUnsandboxedCommands: true`（`sandbox-adapter.ts:459`）。打开需要平台支持 + 依赖装好 + 配置启用 + `sandbox.enabled: true`（`sandbox-adapter.ts:532`）。

### Subprocess

- `Shell.ts` 每条命令 spawn 一个进程，参数含 `preventCwdChanges`、`shouldUseSandbox`（`Shell.ts:177`、`181`）。
- 启用 sandbox 时套 wrapper，建 mode 0700 per-command temp dir（`Shell.ts:259`）。
- 子进程 env 由 `subprocessEnv()` 拼，加上 shell/editor markers、cwd 与 Claude runtime markers（`Shell.ts:315`）。
- 前台 shell 任务可通过 tracking file 改 runtime cwd，session env 与 hooks 随之 invalidate（`Shell.ts:394`）。
- BashTool 决定是否要 sandbox：excluded commands 明确不是 security boundary（`shouldUseSandbox.ts:18`、`130`）。

**对 AVM 的含义**：
- AVM 想让 mapping status 表达"启用了沙箱"，至少要写 `permission.*` rules + `sandbox.enabled` + `--add-dir` 三处。单字段 sandbox 在这里只能 partial native。
- AVM 想阻止 Claude Code 改用户敏感文件，即使不开 external sandbox，filesystem path classifier 已经在拦了；这是 PRD"不能静默覆盖 runtime 全局能力"的天然保护。

---

## 六、Skills

### 形态

Skill 必须是目录加 SKILL.md：`<root>/skills/<name>/SKILL.md`（`src/skills/loadSkillsDir.ts:403`）。**单文件 markdown 不被识别为 skill**。

Frontmatter 字段：description、allowed-tools、argument-hint、when_to_use、version、model/effort、user-invocable、hooks、context forking、agent、shell（`loadSkillsDir.ts:185`）。

### 来源（7 路）

`LoadedFrom` enum：`skills` / `plugin` / `managed` / `bundled` / `mcp`（`loadSkillsDir.ts:67`）。具体扫描 root：

1. **managed**：policy managed path 下
2. **user**：`~/.claude/skills`
3. **project**：`.claude/skills`
4. **additional dirs**：`--add-dir` 指定的路径
5. **plugin**：每个安装的 plugin 提供的 skill root
6. **bundled**：编译进 CLI，第一次调用时 lazy 抽取到 `/tmp/claude-<uid>/bundled-skills/<VERSION>/<nonce>`（`bundledSkills.ts:43`、`59`、`120`、`147`、`195`）
7. **MCP**：MCP server 注册的 skill builders（`loadSkillsDir.ts:1077`）

### 发现行为

- bare 模式跳过自动发现，除非显式给了 `--add-dir` 且 project settings 启用且 skills 未锁定（`loadSkillsDir.ts:648`）。
- 正常模式并行加载所有 root，受 settings 与 plugin-only policy 控制（`loadSkillsDir.ts:677`）。
- realpath 去重，**first-wins**（`loadSkillsDir.ts:716`）。
- **Nested discovery**：从被操作文件向 cwd 走的路径上累加 `.claude/skills`，深度更深的优先级更高，排除 cwd 与 gitignored（`loadSkillsDir.ts:861`）。这是 AVM 没设计过的发现路径。
- Conditional skills：frontmatter 里的 path-based 条件激活（`loadSkillsDir.ts:997`）。

### Plugin 提供的 skill

- Plugin cache 默认 `~/.claude/plugins`，可被 `CLAUDE_CODE_PLUGIN_CACHE_DIR` 和 `--plugin-dir` 覆盖；只读 seed 目录由 `CLAUDE_CODE_PLUGIN_SEED_DIR` 提供（`pluginDirectories.ts:1`、`49`、`66`）。
- Per-plugin data 在 `plugins/data/<sanitizedPluginId>`，跨 update 保留，最后一个 scope 卸载才删（`pluginDirectories.ts:97`）。
- Plugin 全局 metadata 在 `<pluginsDir>/installed_plugins.json`；versioned cache 在 `~/.claude/plugins/cache/{marketplace}/{plugin}/{version}`（`installedPluginsManager.ts:76`、`184`）。
- Plugin relevance：user/managed 是 global，project/local 按 project path 过滤（`installedPluginsManager.ts:785`）。
- Plugin skill 命名 `pluginName:skill`（`loadPluginCommands.ts:687`）。
- bare 模式跳过 marketplace plugin，除非显式 `--plugin-dir`（`loadPluginCommands.ts:840`）。

**对 AVM 的含义**：
- "全量发现"调用同一套 loader 即可，每个 skill 自带 source 字段。
- AVM 自己管理的 skill 建议放 user `~/.claude/skills` 或 `--add-dir` 指定的目录，**不要塞 bundled skills 抽取目录**（`/tmp/claude-<uid>/...`）。
- nested `.claude/skills` 是 AVM 在做 preview 时容易漏报的来源——同一个 cwd 不同子目录看到的 skill 集合可能不一样。

---

## 七、MCP

### Scope 模型

8 个 scope（`src/services/mcp/types.ts:10`）：`enterprise`、`local`、`project`、`user`、`plugin`、`dynamic`、`managed`、`claudeai`。

支持的 transport：stdio、SSE、HTTP、WebSocket、SDK、Claude.ai proxy（`types.ts:23`）。

### 配置位置与写入路径

`mcp add <name> <commandOrUrl> [args...] --scope ...` 的写入规则（`src/commands/mcp/addCommand.ts:33`、`src/services/mcp/config.ts:625`）：

| Scope | 写到哪 |
|---|---|
| project | 当前项目的 `.mcp.json`（atomic write，保留权限位） |
| user | global config `~/.claude.json` 的 `mcpServers` |
| local | 当前 project 在 global config 里的 project block 的 `mcpServers` |
| enterprise | `<managedPath>/managed-mcp.json`，**只读**，AVM 不应写 |
| dynamic / managed / claudeai | **不能通过 add 命令写** |

### 来源合并优先级

读取顺序（按名称查找）：enterprise → local → project → user。Plugin-only policy 启用时只保留 enterprise（`config.ts:1033`）。

主 merged runtime（`config.ts:1071`）：
- Enterprise 可以设为 exclusive，直接屏蔽其他来源。
- 否则加载 user + project + local + plugin，**project MCP 需要 approval**，plugin 与 manual server 去重，按 plugin < user < project < local 合并。
- `--mcp-config` dynamic input 在 startup 前从 JSON / 文件解析，受 enterprise policy 过滤，reserved name 拒绝（`main.tsx:1413`）。
- `--strict-mcp-config` 或 bare 模式跳过自动发现的 MCP，但 dynamic 仍然向下传递（`main.tsx:1799`）。
- 后续 startup 把现有 MCP 与 dynamic 合并，**dynamic 覆盖 file config**（`main.tsx:2380`）。

### 项目 MCP approval

- `.mcp.json` 的 server 会被记进 disabled/enabled MCP settings；首次见到要 user 批准（`utils.ts:351`）。
- Bypass / non-interactive 且 project settings 启用时可以 auto-approve。
- Project / local 的 `headersHelper` 在 trust 前不能跑（`headersHelper.ts:40`）。

### 运行时

- 默认连接超时 30s，可被 `MCP_TIMEOUT` 改（`client.ts:456`）。
- stdio / SDK 是 local；SSE / HTTP 带 OAuth 与 proxy（`client.ts:563`、`595`、`784`）。
- Claude.ai proxy 用 Claude.ai OAuth + `X-Mcp-Client-Session-Id`（`client.ts:868`）。
- Chrome / computer-use 是 in-process stdio（`client.ts:905`）。
- 通用 stdio MCP spawn 配置的命令，env 由 `subprocessEnv()` + server env 组合（`client.ts:944`）。
- MCP roots 只暴露 `file://getOriginalCwd()`（`client.ts:1009`）。
- Cleanup：依次 SIGINT → SIGTERM → SIGKILL（`client.ts:1404`）。
- Needs-auth cache：`~/.claude/mcp-needs-auth-cache.json`，TTL 15 分钟（`client.ts:257`）。

### Token 存储

- OAuth / XAA token 通过 secure storage 存到 MCP-specific key（`auth.ts:349`、`647`、`793`）。
- Server key = name + type + URL + headers 的 SHA-256 hash（`auth.ts:325`）。
- OAuth authorization-server metadata URL 必须 HTTPS（`auth.ts:239`）。

**对 AVM 的含义**：
- MCP 写入是**显式定向**的（add 命令按 scope 写不同位置），不会神奇合并同名 server——AVM 控制写哪一层即可。
- 但**dynamic `--mcp-config` 会覆盖 file config**——AVM 如果用这条路径下发 MCP，要意识到 user 在同一 session 看到的 MCP 不等于 settings 文件里的 MCP。
- enterprise exclusive 模式会让 AVM 写的所有 MCP 失效，AVM 在企业环境要先检测这个。

---

## 八、Plugins / package

Claude Code 把 plugin 当 skill / command / agent / MCP 的载体：

- Plugin marketplace + versioned cache + atomic 替换。
- Manifest 在 `.claude-plugin/plugin.json`，安装路径 `~/.claude/plugins/cache/{marketplace}/{plugin}/{version}`（`installedPluginsManager.ts:184`）。
- 安装来源支持 npm、git、git-subdir、local（`pluginLoader.ts:492-911`）。
- Plugin 提供的 skill 走 7 路里的 plugin 分支；plugin MCP 走 8 个 scope 里的 plugin scope。

**对 AVM 的含义**：
- AVM package 概念可以映射到 Claude Code plugin，但需要 schema 翻译。
- 想保持正交：AVM package 自己渲染，把 skill 直接放 user `~/.claude/skills`，把 MCP 直接写 user / project 层 settings；不碰 plugin 体系。

---

## 九、状态与数据

`~/.claude` 与 `~/.claude.json` 下的关键路径（按 PRD 关心的隔离粒度组织）：

| 路径 | 内容 | 备注 |
|---|---|---|
| `~/.claude.json`（或 `$CLAUDE_CONFIG_DIR/.claude.json`） | global config，含 project map / global MCP / auth metadata / trusted dirs | **独立文件**，不在 `.claude/` 里 |
| `~/.claude/settings.json` | user settings | 三层合并的最低优先 |
| `<project>/.claude/settings.json` / `settings.local.json` | project / local settings | trust gate 控制部分键生效 |
| `~/.claude/.credentials.json` | secure storage 明文 fallback（macOS 用 Keychain） | mode 0600（`secureStorage/index.ts:9`、`plainTextStorage.ts:13`、`44`） |
| `~/.claude/projects/<canonical-git-root>/<sessionId>.jsonl` | session transcript | mode 0600，dir 0700（`sessionStorage.ts:198`、`634`） |
| `~/.claude/projects/<git-root>/<sessionId>/subagents/...` | subagent transcript + `.meta.json` sidecar | `sessionStorage.ts:231`、`260` |
| `~/.claude/projects/<git-root>/<sessionId>/remote-agents/` | remote agent metadata | `sessionStorage.ts:320` |
| `~/.claude/projects/<git-root>/<sessionId>/tool-results/<id>` | tool result，exclusive write | `toolResultStorage.ts:94`、`122` |
| `~/.claude/projects/<git-root>/memory/MEMORY.md` | **auto-memory，按 canonical git root 索引，worktree 共享** | 上限 200 行 25 KB（`memdir/paths.ts:198`、`208`、`memdir.ts:34`） |
| `~/.claude/agent-memory/<agentType>/MEMORY.md` | user-scope agent memory | `AgentTool/agentMemory.ts:47` |
| `<project>/.claude/agent-memory/<agentType>/` 与 `agent-memory-local/` | project / local agent memory | `agentMemory.ts:24`、`47` |
| `~/.claude/plugins/...` | plugin metadata + versioned cache + per-plugin data | 受 `CLAUDE_CODE_PLUGIN_CACHE_DIR` 与 `--plugin-dir` 控制 |
| `~/.claude/debug/<sessionId>.txt` | debug log | 受 `CLAUDE_CODE_DEBUG_LOGS_DIR` 与 `--debug-file` 控制（`debug.ts:230`、`238`） |
| `~/.claude/image-cache/<sessionId>` / `~/.claude/paste-cache` | 媒体缓存 | mode 0600（`imageStore.ts:54`、`pasteStore.ts:37`） |
| `~/.claude/session-env/<sessionId>` | session env file | `sessionEnvironment.ts:15`、`60` |
| `~/.claude/mcp-needs-auth-cache.json` | MCP needs-auth TTL cache | 15 分钟 |
| env-paths('claude-cli') 下 | base / errors / messages / MCP logs | XDG defaults，**不跟 `CLAUDE_CONFIG_DIR` 走**（`cachePaths.ts:1`、`xdg.ts:32`） |
| `/tmp/claude-<uid>/bundled-skills/<VERSION>/<nonce>` | bundled skill 抽取 | 受 `CLAUDE_CODE_TMPDIR` 控制（`filesystem.ts:302`、`365`） |

### Memory 子系统说明

PRD 明说"不管 memory"，这里重点提醒它**不是被动 context**：

- **CLAUDE.md** 是被动加载，AVM 控制内容即可。
- **Auto-memory** 默认开（除非 bare / remote 无持久化 / 显式禁用），按 canonical git root 索引（`memdir/paths.ts:21`、`198`）。**worktree 共享 auto-memory**——`avm run agent-a` 和 `avm run agent-b` 如果都在同一个 git root 下跑，auto-memory 文件是同一份。
- **Agent memory** 三 scope（user / project / local），每个 agent type 一个 `MEMORY.md`（`agentMemory.ts:12`、`47`）。
- **Session memory** 是 forked subagent + post-sampling hook 维护的，每个 session 自动建 mode 0700 目录与 mode 0600 summary 文件（`sessionMemory.ts:80`、`183`、`357`）。

`CLAUDE_COWORK_MEMORY_PATH_OVERRIDE` 可以完全覆盖 auto-memory 路径（`memdir/paths.ts:152`）。`CLAUDE_CODE_REMOTE_MEMORY_DIR` 控制 remote 模式的 memory base（`memdir/paths.ts:80`）。

### Cleanup

- 默认保留 30 天，从 `settings.cleanupPeriodDays` 读（`cleanup.ts:23`、`25`）。
- 覆盖 messages / sessions / plans / file history / session-env / debug / image / paste / stale worktree（`cleanup.ts:575`）。
- `~/.claude/projects` 下的旧 session（jsonl + asciinema cast + tool result）一起清（`cleanup.ts:155`）。

### 凭据

- macOS Keychain，其他平台明文 `~/.claude/.credentials.json`（`secureStorage/index.ts:9`、`plainTextStorage.ts:13`）。
- Bare 模式只用 `ANTHROPIC_API_KEY` 或 `--settings` 里的 `apiKeyHelper`（`auth.ts:226`）。
- 正常 API key 解析：approved env → file descriptor → API key helper cache → config → keychain（`auth.ts:298`）。
- OAuth token 存 secure storage `claudeAiOauth`（`auth.ts:1198`）；bare 模式跳过 OAuth（`auth.ts:1255`）。
- Remote CCR token 路径 `/home/claude/.claude/remote/{.oauth_token,.api_key,.session_ingress_token}`（`authFileDescriptor.ts:13`、`30`）。

**核心事实**：Claude Code 没有一个总开关把所有状态搬到隔离目录。PRD 想做"AVM home"边界，至少要同时管：
- `CLAUDE_CONFIG_DIR`（settings + global config）
- `CLAUDE_CODE_PLUGIN_CACHE_DIR`（plugin cache）
- `CLAUDE_CODE_TMPDIR`（temp / bundled skill）
- `CLAUDE_CODE_DEBUG_LOGS_DIR`（debug log）
- `CLAUDE_CODE_REMOTE_MEMORY_DIR`（remote memory）
- env-paths cache 不可控（XDG），但只有 cache，可接受

---

## 十、对 PRD 的风险提示

按 PRD 章节对应的风险清单：

1. **PRD 2.3（能力边界）/ 4.2（全量发现）**：Claude Code 没有"单一 registry"。Skills 走 7 路、MCP 走 8 个 scope，且 nested `.claude/skills` 的发现路径会让"同一 cwd 看到的能力集合"依赖于被操作的文件位置。AVM 要自己定义合并和展示规则。

2. **PRD 6（runtime mapping 状态）**：sandbox 在 Claude Code 是四维（permission rules + path classifier + external sandbox + subprocess），AVM 单字段 sandbox 至少要展开成 `permission.*` rules + `sandbox.enabled` + `--add-dir` 才能 round-trip；否则只能 partial native。

3. **PRD 4.2（不能静默接管）**：Claude Code 的 skill/MCP 加载是 first-wins、按 source 排序，不互删。AVM 只要不主动改不属于自己的 settings 键就安全。但 dynamic `--mcp-config` 会覆盖 file config，AVM 如果走这条路径，要在 preview 里告诉用户"本次运行的 MCP 与 settings 不一致"。

4. **PRD 4.4（运行透明）/ 4.6（隔离）**：Claude Code 的硬隔离要靠多个环境变量（`CLAUDE_CONFIG_DIR` + `CLAUDE_CODE_PLUGIN_CACHE_DIR` + `CLAUDE_CODE_TMPDIR` + `CLAUDE_CODE_DEBUG_LOGS_DIR` + ...）一起设，不是单字段开关。AVM `--runtime` 注入这些 env 是可行的，但 `avm doctor` 里要解释每条都要管。

5. **PRD 4.6（memory）**：
   - "不管 memory" ≠ "memory 不发生"。auto-memory 默认开，会写 `~/.claude/projects/<git-root>/memory/`。
   - **worktree 共享 auto-memory**——这条对 PRD 想做的"agent 之间不串状态"是直接冲突的。同 git root 下跑两个不同的 agent，默认看到同一个 MEMORY.md。
   - 要彻底隔离，要么给每个 agent 一个独立 `CLAUDE_CONFIG_DIR`，要么用 `CLAUDE_COWORK_MEMORY_PATH_OVERRIDE` 把 memory 路径按 agent ID 分；后者更便宜但要测试是否影响其他子系统。

6. **PRD 3.3（Package）**：Claude Code plugin 有 marketplace + versioned cache + manifest，比 AVM package 重。AVM package 想导出成 plugin 要做 schema 翻译；不想碰则自己渲染——直接把 skill 放 user `~/.claude/skills`，MCP 写 user 层 settings 即可。

7. **PRD 4.1（init/doctor）**：Claude Code 的状态分散在 `~/.claude.json`、`~/.claude/`、env-paths cache、可选的 plugin override 目录。`avm doctor` 要扫多个位置才能给出"runtime 已就绪"的判断。

8. **PRD 2.5（运行透明）trust gate**：project 层配置（`.claude/settings.json`、`.mcp.json`、`headersHelper`）首次加载需要 user trust。AVM 写 project 层时，如果用户没 trust 过这个目录，env / MCP / headersHelper 都不会生效，preview 显示的"将启动什么"和实际不一致。建议 AVM 默认写 user 层，project 层只在用户明确要求时写。

9. **PRD 3.4（runtime）**：Claude Code 还有 daemon / remote-control / bridge / background-session / Chrome MCP 等多种早期入口（`cli.tsx:95-247`），AVM 当前只考虑 interactive / headless 两种就够。要避免后续把这些都纳入"AVM 管"的范畴。

10. **凭据存储**：非 macOS 平台 secure storage fallback 到明文 `~/.claude/.credentials.json`。AVM 想做"agent 隔离凭据"必须用独立 `CLAUDE_CONFIG_DIR`，不能依赖 secure storage 自身做隔离。

---

## 十一、未验证项

- 该 framework 快照**没有 `package.json` / lockfile / Makefile / built executable**，所有结论都来自 TypeScript 源码追踪，没有跑过 `--help`、build 或 test。
- `@anthropic-ai/sandbox-runtime` 的具体 OS 级保证不在仓库里，本调研只能验证 Claude Code **何时调用** sandbox adapter，无法验证外部 sandbox 的真实行为。
- Build-time feature flag 影响 TeamMem、Buddy、Kairos、bundled skills 和若干 remote 行为；准确的 production feature 集合不能仅从静态源码推导。
- Claude.ai connector config 在 startup 异步获取，server-side connector 选择与 policy 不在快照里。
- 部分注释描述 Ant 内部 build 与 sandbox 要求；不确认 build macros 与 distribution settings 之前，这些路径不一定适用于 public 版本。
- Worktree 共享 auto-memory 这一点**未实测**——结论来自 `memdir/paths.ts:198` 的 canonical git root 逻辑。AVM 真要依赖隔离边界，建议先写一个最小可重现实验。
