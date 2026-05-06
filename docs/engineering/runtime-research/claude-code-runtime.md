# Claude Code Runtime 调研

## 摘要

本文追踪 `frameworks/claude-code-main` 中 Claude Code runtime 的实现，用于验证 Agent VM PRD 的前提。结论均来自源码追踪，README 内容未作为证据。除非特别说明，源码引用均相对于 `frameworks/claude-code-main/`。

关键发现：

- 可执行入口的 bootstrap 是 `src/entrypoints/cli.tsx`。正常路径先做早期 flag 分发，启用特殊入口，然后导入 `src/main.tsx` 并调用 `cliMain()`（`src/entrypoints/cli.tsx:28`、`src/entrypoints/cli.tsx:287`）。
- Runtime 初始化被有意分阶段处理。`init()` 在项目 trust 前只应用可信环境来源；`setup()` 随后建立 cwd、workspace identity、messaging、session memory、watcher、hook 和权限绕过检查（`src/entrypoints/init.ts:57`、`src/utils/managedEnv.ts:93`、`src/setup.ts:56`、`src/setup.ts:160`）。
- Runtime 区分执行 cwd 和项目身份。`originalCwd` 与 `projectRoot` 用于 history、skills、sessions 等项目级状态，不直接作为文件操作边界（`src/bootstrap/state.ts:45`、`src/bootstrap/state.ts:496`）。
- 文件与命令隔离默认是策略驱动，不是 VM 式强隔离。源码中存在 sandbox adapter，但是否启用取决于平台、依赖和设置；默认 sandbox 设置启用 auto-allow 语义，但不会强制开启 sandbox（`src/utils/sandbox/sandbox-adapter.ts:459`、`src/utils/sandbox/sandbox-adapter.ts:532`）。
- Skills 是类似 prompt-command 的文件，从 managed、user、project、additional directories、plugins、bundled skill registries，以及 MCP 提供的 skill builders 加载（`src/skills/loadSkillsDir.ts:67`、`src/skills/loadSkillsDir.ts:638`、`src/skills/bundledSkills.ts:43`、`src/skills/loadSkillsDir.ts:1077`）。
- MCP 配置按 scope 建模，并在 enterprise、local、project、user、plugin、dynamic、managed、Claude.ai 和 SDK 来源之间合并。Project MCP 文件加载前需要 approval；enterprise policy 可以让 managed MCP 成为唯一来源（`src/services/mcp/types.ts:10`、`src/services/mcp/config.ts:888`、`src/services/mcp/config.ts:1071`、`src/services/mcp/utils.ts:351`）。
- 持久状态分散在 `~/.claude`、`~/.claude.json`、`~/.claude/projects` 下的项目级目录、env-path cache 目录、keychain 或明文 secure storage，以及可选 remote/config override 中（`src/utils/env.ts:13`、`src/utils/sessionStorage.ts:198`、`src/utils/cachePaths.ts:1`、`src/utils/secureStorage/plainTextStorage.ts:13`）。

已执行的安全验证：

- 该 framework 快照包含 `src/`、`Doc/`、`README.md` 和 `CLAUDE.md`，但没有 `package.json`、lockfile、Makefile 或已构建的可执行文件。因此此 checkout 中没有可运行的 `--help`、build 或 test 命令。
- Entrypoint 与 runtime 行为通过追踪 `frameworks/claude-code-main/src` 下的 TypeScript 源码验证。

## 运行时启动路径

### 入口分发

CLI 从 `src/entrypoints/cli.tsx` 启动。在导入完整 runtime 前，它会设置进程级 flag，并处理 fast path：

- 通过 `COREPACK_ENABLE_AUTO_PIN=0` 禁用 Corepack 自动 pin（`src/entrypoints/cli.tsx:1`）。
- 在 remote mode 下，把 `--max-old-space-size=8192` 追加到 `NODE_OPTIONS`（`src/entrypoints/cli.tsx:7`）。
- 执行早期 ablation 环境处理（`src/entrypoints/cli.tsx:21`）。
- 解析原始 args，并在完整启动前处理 `--version`（`src/entrypoints/cli.tsx:33`）。
- 在正常 runtime 前分发特殊 MCP/native-host 模式：Chrome MCP、Chrome native host、computer-use MCP（`src/entrypoints/cli.tsx:72`、`src/entrypoints/cli.tsx:82`、`src/entrypoints/cli.tsx:87`）。
- 在正常交互启动前分发 daemon、daemon-worker、remote-control、bridge、remote/sync 和 background-session 命令（`src/entrypoints/cli.tsx:95`、`src/entrypoints/cli.tsx:112`、`src/entrypoints/cli.tsx:164`、`src/entrypoints/cli.tsx:182`）。
- 在完整 CLI import 前处理 `--worktree --tmux`，因为该路径可能 exec 进入 tmux（`src/entrypoints/cli.tsx:247`）。
- 在完整 import 前处理 `--bare`，设置 `CLAUDE_CODE_SIMPLE=1`（`src/entrypoints/cli.tsx:281`）。
- 正常启动捕获 early input，导入 `../main.js`，并调用 `cliMain()`（`src/entrypoints/cli.tsx:287`）。

### 主运行时初始化

`src/main.tsx` 负责 Commander CLI 和主 runtime 路径：

- `main()` 设置 `NoDefaultCurrentDirectoryInExePath=1`，安装 signal handlers，并区分 interactive 与 non-interactive 模式（`src/main.tsx:585`、`src/main.tsx:595`、`src/main.tsx:797`）。
- Client type 从环境、entrypoint、session-ingress 和 runtime context 推断（`src/main.tsx:817`）。
- Settings 在 command runner 构建前被 eager load（`src/main.tsx:851`）。
- `run()` 构造 Commander program（`src/main.tsx:884`）。
- Commander `preAction` 等待 managed policy/keychain prefetch，调用 `init()`，初始化 process metadata 和 event sinks，传播 plugin-dir settings，执行 migrations，并启动后台 remote settings/policy sync（`src/main.tsx:905`）。

CLI options 建立若干 runtime 边界：

- 权限与工具边界：`--dangerously-skip-permissions`、`--allow-dangerously-skip-permissions`、`--permission-mode`、`--allowed-tools`、`--tools`、`--disallowed-tools`（`src/main.tsx:968`）。
- MCP 边界：`--mcp-config`、`--strict-mcp-config`（`src/main.tsx:986`、`src/main.tsx:1003`）。
- 状态与会话边界：`--continue`、`--resume`、`--fork-session`、`--no-session-persistence`、`--session-id`（`src/main.tsx:991`、`src/main.tsx:1005`）。
- Workspace extension 与 plugin 边界：`--add-dir`、`--agents`、`--setting-sources`、`--plugin-dir`、`--disable-slash-commands`（`src/main.tsx:999`、`src/main.tsx:1006`）。

### 配置与环境加载

Configuration 同时包含全局配置和按来源划分的 settings：

- 全局 config 文件默认是 `~/.claude.json`；如果设置了 `CLAUDE_CONFIG_DIR`，则为 `$CLAUDE_CONFIG_DIR/.claude.json`（`src/utils/env.ts:13`）。
- Config home 默认是 `~/.claude`，或 `CLAUDE_CONFIG_DIR`（`src/utils/envUtils.ts:5`）。
- Settings 来源优先级是 user、project、local、flag、policy，后面的来源覆盖前面的来源（`src/utils/settings/constants.ts:3`）。
- Settings 文件路径包括 user `~/.claude/settings.json`、project `.claude/settings.json`、local `.claude/settings.local.json`、policy managed settings，以及可选 flag settings（`src/utils/settings/settings.ts:274`）。
- Managed settings 从平台 managed path 和可选 `managed-settings.d/*.json` drop-ins 加载，drop-ins 按字母顺序覆盖（`src/utils/settings/settings.ts:55`、`src/utils/settings/settings.ts:74`）。
- Managed path 默认在 Linux 上是 `/etc/claude-code`，macOS 上是 `/Library/Application Support/ClaudeCode`，Windows 上是 `C:\Program Files\ClaudeCode`，并支持 Ant-specific 环境变量 override（`src/utils/settings/managedPath.ts:8`）。

环境应用按 trust 阶段拆分：

- 在项目 trust 前，只有 user、flag 和 policy 环境来源可信；project 和 local settings 被排除，因为它们可能重定向流量（`src/utils/managedEnv.ts:93`）。
- 在 trust 前，只能从完整 merged settings 中应用 allowlisted safe environment variables（`src/utils/managedEnv.ts:124`、`src/utils/managedEnvConstants.ts:108`）。
- 在 trust 后，才会应用 global config 和 merged settings 中的所有环境变量，并重置 proxy/mTLS/CA cache（`src/utils/managedEnv.ts:180`）。
- Bare mode 默认禁用 hooks、LSP、plugin sync、skill directory walking、attribution、background prefetches 和 keychain/credential reads（`src/utils/envUtils.ts:49`）。

### 工作区与会话上下文

`setup()` 是主要的 workspace/session initializer：

- 它接收 cwd、permission mode、dangerous-permission flags、worktree/tmux options、custom session ID、PR number，以及可选 messaging socket path（`src/setup.ts:56`）。
- 它验证 Node 版本 >= 18（`src/setup.ts:69`）。
- 它可以在 runtime state 构建前切换到 custom session ID（`src/setup.ts:81`）。
- 除非 bare mode 且没有显式 socket path，否则它会启动 Unix domain socket messaging server，并导出 `CLAUDE_CODE_MESSAGING_SOCKET`（`src/setup.ts:86`）。
- 它设置 cwd，快照 hook config，并启动 file-changed watcher（`src/setup.ts:160`）。
- 在 `--worktree` mode 下，它验证 git/hook 状态，创建或进入 worktree，可选管理 tmux，改变进程 cwd，更新 original cwd 与 project root，保存 worktree state，并清空 memory caches（`src/setup.ts:174`）。
- 在 non-bare mode 下，它初始化 session memory/context collapse（`src/setup.ts:293`）。
- 除非跳过 plugin prefetch，否则它会预加载 commands 和 plugin hooks（`src/setup.ts:321`）。
- 在 non-bare mode 下，它注册 attribution、session file access hooks 和 team memory watcher（`src/setup.ts:336`）。

运行时身份状态集中在 `src/bootstrap/state.ts`：

- `originalCwd` 和 `projectRoot` 明确用于 history、skills、sessions 等项目身份，不用于文件操作（`src/bootstrap/state.ts:45`）。
- Initial cwd、original cwd 和 project root 在进程初始化时基于 realpath（`src/bootstrap/state.ts:259`）。
- 默认生成随机 session ID（`src/bootstrap/state.ts:331`）。
- Session trust 与 session-persistence flags 是内存中的 session state（`src/bootstrap/state.ts:362`）。
- Additional directories 与 session project directory 被单独追踪（`src/bootstrap/state.ts:402`）。
- `switchSession()` 可以替换 active session ID，并可选替换其 project directory（`src/bootstrap/state.ts:456`）。

会话上下文加载 system 与 user memory：

- 在 git repository 内时，Git status 包含当前 branch/status/log 和配置的 user name（`src/context.ts:36`）。
- System context 在 remote mode 或禁用时跳过 git status（`src/context.ts:113`）。
- User context 会加载 CLAUDE.md memory files，除非被禁用，或 bare mode 且没有 additional directories（`src/context.ts:155`）。

## 隔离模型

### 进程隔离

在追踪到的源码中，Claude Code 不会为每个 agent 创建 VM/container 边界。进程隔离主要是每条命令使用 subprocess 执行，并可选套上 sandbox wrapper：

- 每条 shell command 都创建一个新的 shell process（`src/utils/Shell.ts:177`）。
- Shell execution 接收 `preventCwdChanges` 和 `shouldUseSandbox`（`src/utils/Shell.ts:181`）。
- 如果某条命令启用 sandbox，该命令会被 wrapper 包裹，并创建 mode `0700` 的 per-command 临时目录（`src/utils/Shell.ts:259`）。
- Subprocess 继承 `subprocessEnv()`、shell/editor markers、cwd 和 Claude runtime markers（`src/utils/Shell.ts:315`）。
- Foreground shell tasks 可以通过 tracking file 更新 runtime cwd；随后 session environment 和 hooks 会被 invalidated（`src/utils/Shell.ts:394`）。
- Sandboxed shell commands 完成后会执行 sandbox cleanup（`src/utils/Shell.ts:385`）。

Bash tool 决定是否请求 sandbox：

- Excluded commands 被明确标注为不是 security boundary（`src/tools/BashTool/shouldUseSandbox.ts:18`）。
- 当 sandbox 不可用或禁用、允许 dangerous-disable、命令为空、或命令被排除时，sandbox 使用会被禁用（`src/tools/BashTool/shouldUseSandbox.ts:130`）。
- Bash execution 将 timeout、cwd-change prevention、sandbox decision 和 auto-background settings 传入 `Shell.exec()`（`src/tools/BashTool/BashTool.tsx:877`）。

### 文件系统边界

Filesystem model 组合了 workspace roots、sensitive path classification、permission rules 和 sandbox rules：

- Dangerous file basenames 包括 shell rc files、git config/module files、`.mcp.json` 和 Claude config files（`src/utils/permissions/filesystem.ts:57`）。
- Dangerous directories 包括 `.git`、`.vscode`、`.idea` 和 `.claude`（`src/utils/permissions/filesystem.ts:74`）。
- Allowed working directories 是 original cwd 加上 additional directories（`src/utils/permissions/filesystem.ts:667`）。
- `pathInWorkingPath()` 会把目标路径解析到当前 working path set 中判断（`src/utils/permissions/filesystem.ts:709`）。
- Read-deny patterns 从 permission rules 加载（`src/utils/permissions/filesystem.ts:837`）。
- `checkReadPermissionForTool()` 会阻止 UNC/suspicious paths，遵守 deny/ask rules，然后检查 edit access、working dirs、internal paths 和 allow rules（`src/utils/permissions/filesystem.ts:1030`）。
- Tool result directories 可读，用于 tool output recovery（`src/utils/permissions/filesystem.ts:1660`）。
- 当前 session 的 scratchpad files 可读（`src/utils/permissions/filesystem.ts:1676`）。
- 同一 project 内所有 sessions 的 project temp directories 可读（`src/utils/permissions/filesystem.ts:1688`）。
- Agent memory 和 auto-memory paths 作为 special internal paths 可读（`src/utils/permissions/filesystem.ts:1703`）。

Skill 目录有显式路径识别：

- Project 与 global `.claude/skills/<skill>/**` scope 被识别为 skill paths（`src/utils/permissions/filesystem.ts:94`）。
- Claude-owned config files 包括 `.claude` 下的 settings、commands、agents 和 skills（`src/utils/permissions/filesystem.ts:224`）。

### 沙箱边界

沙箱 adapter 把 enforcement 委托给 `@anthropic-ai/sandbox-runtime`：

- 外部 runtime 以 `BaseSandboxManager` 形式导入（`src/utils/sandbox/sandbox-adapter.ts:1`）。
- Settings permission rules 被转换成 sandbox filesystem path rules（`src/utils/sandbox/sandbox-adapter.ts:83`、`src/utils/sandbox/sandbox-adapter.ts:121`）。
- Managed-only network 和 read-path policies 在 sandbox config 中表达（`src/utils/sandbox/sandbox-adapter.ts:148`）。
- Network allow/deny 从 `sandbox.network.allowedDomains` 和 `WebFetch(domain:...)` permission rules 派生（`src/utils/sandbox/sandbox-adapter.ts:177`）。
- Sandbox writes 初始允许 `.` 和 Claude temp directory（`src/utils/sandbox/sandbox-adapter.ts:222`）。
- Settings paths 和 managed settings drop-ins 始终 deny（`src/utils/sandbox/sandbox-adapter.ts:230`）。
- 当前和 original `.claude/settings*.json` 以及 `.claude/skills` paths 被显式 deny（`src/utils/sandbox/sandbox-adapter.ts:238`）。
- Bare git repo files 会被 deny 或 scrub（`src/utils/sandbox/sandbox-adapter.ts:257`）。
- 配置允许时，worktree main repo 和 additional directories 会被 allow（`src/utils/sandbox/sandbox-adapter.ts:282`）。
- 默认 sandbox settings 是 `enabled: false`、`autoAllow: true`、`allowUnsandboxedCommands: true`（`src/utils/sandbox/sandbox-adapter.ts:459`）。
- Sandbox enablement 需要 platform support、dependencies、configured enabled platforms 和 `sandbox.enabled`（`src/utils/sandbox/sandbox-adapter.ts:532`）。
- Initialization 会更新 runtime sandbox config，并注册 settings-change handling（`src/utils/sandbox/sandbox-adapter.ts:730`）。

### 工具权限模型

Tool permission context 由 CLI flags、settings、policy 和 session state 构造：

- Permission mode resolution 会综合 CLI、settings、dangerous bypass 和 auto mode（`src/utils/permissions/permissionSetup.ts:721`）。
- CLI allowed/disallowed/base tool lists 被解析为 permission rules（`src/utils/permissions/permissionSetup.ts:892`）。
- Broad Bash 与 PowerShell permissions 会被检测，并可能在 auto mode 中移除（`src/utils/permissions/permissionSetup.ts:948`）。
- Loaded permission rules 来自所有 enabled settings sources，除非 policy 限制为 managed rules only（`src/utils/permissions/permissionsLoader.ts:120`）。
- 可编辑 permission sources 是 user、project 和 local（`src/utils/permissions/permissionsLoader.ts:151`）。
- Permission sources 包括 settings sources、CLI args、commands 和 session source（`src/utils/permissions/permissions.ts:109`）。
- `hasPermissionsToUseTool()` 是中心化的 tool permission evaluator（`src/utils/permissions/permissions.ts:473`）。

Dangerous permission bypass 也有约束：

- `setup()` 在 root 身份下阻止 bypass mode，除非存在 sandbox environment marker（`src/setup.ts:395`）。
- Ant builds 要求 Docker、bubblewrap 或 `IS_SANDBOX=1`，且没有 internet environment，才允许 bypass mode（`src/setup.ts:414`）。

## Skill 安装与加载

### Skill 格式与来源

Skills 作为类似 command 的 prompt definitions 被加载：

- `LoadedFrom` 区分 `skills`、`plugin`、`managed`、`bundled` 和 `mcp` 来源（`src/skills/loadSkillsDir.ts:67`）。
- Source roots 包括 policy managed、user `~/.claude`、project `.claude` 和 plugin directories（`src/skills/loadSkillsDir.ts:78`）。
- Skill frontmatter 支持 `description`、`allowed-tools`、`argument-hint`、`when_to_use`、`version`、model/effort fields、user-invocable flag、hooks、context forking、agent 和 shell fields（`src/skills/loadSkillsDir.ts:185`）。
- Skill directories 只以 `skills/<skill-name>/SKILL.md` 形式被发现；`skills/` 下的单个 Markdown 文件不被支持为 skill（`src/skills/loadSkillsDir.ts:403`）。
- `/skills` menu 会筛选从 skills、deprecated command files、plugin skills 和 MCP skills 加载的 commands（`src/components/skills/SkillsMenu.tsx:234`）。

### 发现优先级

正常 auto-discovery 从多个 root 加载 skills：

- 先计算 managed、user 和 project directories（`src/skills/loadSkillsDir.ts:638`）。
- Bare mode 会跳过 auto-discovery；只有 project settings 启用且 skills 未锁定时，显式 `--add-dir` paths 例外（`src/skills/loadSkillsDir.ts:648`）。
- Normal mode 并行加载 managed、user、project、additional 和 legacy command directories；user/project loading 受 settings 和 plugin-only policy 控制（`src/skills/loadSkillsDir.ts:677`）。
- Loaded commands 按 realpath 去重，采用 first-wins 行为（`src/skills/loadSkillsDir.ts:716`）。
- 从 operated file 向上走到 cwd 时会发现嵌套 `.claude/skills` directories，排除 cwd 和 gitignored directories；更深目录优先级更高（`src/skills/loadSkillsDir.ts:861`）。
- 只有 project settings 启用且 plugin-only policy 未激活时，才会添加 dynamic skill directories（`src/skills/loadSkillsDir.ts:923`）。
- Conditional skills 可以通过 path-based frontmatter 激活（`src/skills/loadSkillsDir.ts:997`）。
- MCP skill builders 与 filesystem skill discovery 分开注册（`src/skills/loadSkillsDir.ts:1077`）。

### 内置 Skills

Bundled skills 被编译进 CLI，并延迟抽取：

- Bundled skill registry 是进程内部 registry（`src/skills/bundledSkills.ts:43`）。
- Reference files 在第一次调用时抽取，不在 startup 时抽取（`src/skills/bundledSkills.ts:59`）。
- Bundled skill command objects 将 source 标识为 `bundled`（`src/skills/bundledSkills.ts:75`）。
- 抽取写入 Claude temp root 下 versioned bundled-skills directory，并使用受限目录/文件 mode 以及 `O_NOFOLLOW`、`O_EXCL`（`src/skills/bundledSkills.ts:120`、`src/skills/bundledSkills.ts:147`）。
- Extracted reference paths 会被验证以防 traversal（`src/skills/bundledSkills.ts:195`）。
- Built-in bundled skills 由 `initBundledSkills()` 注册（`src/skills/bundled/index.ts:24`）。

### 插件提供的 Skills

Claude Code 也把插件作为 skill/package 载体：

- Plugin cache dir 默认是 `~/.claude/plugins`，可由 `CLAUDE_CODE_PLUGIN_CACHE_DIR` 和 `--plugin-dir` override（`src/utils/plugins/pluginDirectories.ts:1`、`src/utils/plugins/pluginDirectories.ts:49`）。
- Read-only seed directories 可通过 `CLAUDE_CODE_PLUGIN_SEED_DIR` 提供；第一个 seed hit 生效（`src/utils/plugins/pluginDirectories.ts:66`）。
- Per-plugin data 存在 `plugins/data/<sanitizedPluginId>` 下，并通过 `CLAUDE_PLUGIN_DATA` 暴露给 plugin code；它会跨 update 保留，并在最后一个 scope uninstall 时删除（`src/utils/plugins/pluginDirectories.ts:97`）。
- Plugin installation metadata 是全局的，enable/disable state 则按 repository/settings scope 存储（`src/utils/plugins/installedPluginsManager.ts:1`）。
- Installed plugin metadata 存在 `<pluginsDir>/installed_plugins.json`（`src/utils/plugins/installedPluginsManager.ts:76`）。
- Versioned plugin cache paths 是 `~/.claude/plugins/cache/{marketplace}/{plugin}/{version}`（`src/utils/plugins/installedPluginsManager.ts:184`）。
- Plugin relevance 对 user/managed scopes 是 global，对 project/local scopes 是 project-specific（`src/utils/plugins/installedPluginsManager.ts:785`）。
- Version resolution 优先级是 manifest version、provided version、git SHA，然后是 `unknown`（`src/utils/plugins/pluginVersioning.ts:19`）。
- Plugin cache installation 支持 npm、git、git-subdir 和 local sources，然后验证 `.claude-plugin/plugin.json` 或 legacy metadata，再把 temp cache 移动到最终 versioned path（`src/utils/plugins/pluginLoader.ts:492`、`src/utils/plugins/pluginLoader.ts:534`、`src/utils/plugins/pluginLoader.ts:718`、`src/utils/plugins/pluginLoader.ts:856`、`src/utils/plugins/pluginLoader.ts:911`）。
- Plugin skills 可来自直接的 `SKILL.md` 文件或 skill subdirectories，并以 `pluginName:skill` 前缀命名（`src/utils/plugins/loadPluginCommands.ts:687`）。
- Bare mode 会跳过 marketplace plugin auto-loading，除非显式提供 plugin dir（`src/utils/plugins/loadPluginCommands.ts:840`）。

## MCP 安装与加载

### MCP 配置模型

MCP config 按作用域与 transport 建模：

- Config scopes 是 `local`、`user`、`project`、`dynamic`、`enterprise`、`claudeai` 和 `managed`（`src/services/mcp/types.ts:10`）。
- 支持的 transport types 包括 stdio、SSE、HTTP、WebSocket、SDK 和 Claude.ai proxy（`src/services/mcp/types.ts:23`）。
- Stdio server config 包含 command、args 和 env fields（`src/services/mcp/types.ts:28`）。
- HTTP/SSE configs 支持 OAuth 和 headers（`src/services/mcp/types.ts:43`、`src/services/mcp/types.ts:89`）。
- `.mcp.json` 以 `mcpServers` 存储 project-scoped MCP servers（`src/services/mcp/types.ts:163`）。

### MCP 写入路径与作用域优先级

MCP config 可以通过 CLI 添加，也可以从文件解析：

- `mcp add <name> <commandOrUrl> [args...]` 支持 `--scope`、`--transport`、env vars、headers、OAuth client data 和 XAA options（`src/commands/mcp/addCommand.ts:33`）。
- Project MCP 写入 `.mcp.json`；user MCP 写入 global config；local MCP 写入当前 project config。Dynamic、enterprise 和 Claude.ai scopes 不能通过 `addMcpConfig()` 添加（`src/services/mcp/config.ts:625`）。
- Project `.mcp.json` 以 atomic 方式写入，同时保留 permissions（`src/services/mcp/config.ts:83`）。
- Enterprise MCP config path 是 `<managedPath>/managed-mcp.json`（`src/services/mcp/config.ts:62`）。
- Project config 从 cwd 向 root 遍历，然后 root-down 应用 configs，因此更近的 `.mcp.json` 覆盖父目录（`src/services/mcp/config.ts:888`、`src/services/mcp/config.ts:913`）。
- User MCP config 从 `getGlobalConfig().mcpServers` 加载（`src/services/mcp/config.ts:963`）。
- Local MCP config 从 `getCurrentProjectConfig().mcpServers` 加载（`src/services/mcp/config.ts:979`）。
- Enterprise MCP config 从 managed MCP file 加载（`src/services/mcp/config.ts:996`）。
- 按名称查找使用 enterprise、local、project、user 的优先级；当 plugin-only policy 启用时，只有 enterprise 会按名称保留（`src/services/mcp/config.ts:1033`）。

主 merged runtime 路径有额外优先级：

- Enterprise MCP 可以设置为 exclusive，并阻止加载 non-enterprise configs（`src/services/mcp/config.ts:1071`）。
- 否则 Claude Code 会加载 user、project 和 local configs，除非 plugin-only policy 阻止；随后加载 plugin MCP configs，要求 project MCP approval，对 plugin servers 与 manual servers 去重，并按 plugin < user < project < local 的有效优先级合并（`src/services/mcp/config.ts:1071`）。
- Dynamic `--mcp-config` input 从 JSON 或文件解析，并受 enterprise policy 过滤；reserved names 会被拒绝（`src/main.tsx:1413`）。
- Runtime startup 中，strict MCP config 或 bare mode 会跳过 auto-discovered MCP configs，但 dynamic MCP config 会继续向下传递（`src/main.tsx:1799`）。
- 后续 startup 会把现有 MCP config 与 dynamic config 合并；dynamic servers 覆盖 file configs（`src/main.tsx:2380`）。

### MCP 审批、授权与连接

Project MCP 与 remote MCP 都有 authorization gates：

- Project `.mcp.json` server approval 通过 disabled/enabled MCP settings 追踪；在 bypass/non-interactive mode 且 project settings 启用时可 auto-approve（`src/services/mcp/utils.ts:351`）。
- Policy filtering 支持按 name、command 和 URL 设置 enterprise allowlists 与 denylists（`src/services/mcp/config.ts:341`、`src/services/mcp/config.ts:364`）。
- MCP command、args、env、URL 和 headers 中的环境变量会在使用前展开（`src/services/mcp/config.ts:556`）。
- Project/local `headersHelper` commands 在 interactive sessions 且 trust 前不能运行；non-interactive mode 跳过该 trust check（`src/services/mcp/headersHelper.ts:40`）。
- OAuth authorization-server metadata URLs 必须是 HTTPS；源码注释把 project MCP approval 视为抵御恶意 project MCP config 的防线（`src/services/mcp/auth.ts:239`）。
- MCP OAuth server keys 包含 server name，以及 type、URL、headers 的 SHA-256 hash（`src/services/mcp/auth.ts:325`）。
- OAuth 和 XAA tokens 通过 secure storage 存到 MCP-specific keys 下（`src/services/mcp/auth.ts:349`、`src/services/mcp/auth.ts:647`、`src/services/mcp/auth.ts:793`）。
- XAA IDP tokens 也缓存在 secure storage 中（`src/services/mcp/xaaIdpLogin.ts:95`）。

Transport 行为：

- MCP connection timeout 默认是 30 秒，或使用 `MCP_TIMEOUT`（`src/services/mcp/client.ts:456`）。
- Batch sizes 可通过环境变量调整（`src/services/mcp/client.ts:552`）。
- Stdio 和 SDK transports 被归类为 local（`src/services/mcp/client.ts:563`）。
- SSE 和 HTTP clients 附加 OAuth providers、static/dynamic headers，以及 proxy/timeout 行为（`src/services/mcp/client.ts:595`、`src/services/mcp/client.ts:784`）。
- Claude.ai proxy transport 使用 Claude.ai OAuth 和 `X-Mcp-Client-Session-Id`（`src/services/mcp/client.ts:868`）。
- Chrome 与 computer-use MCP servers 作为 in-process stdio transports 运行（`src/services/mcp/client.ts:905`）。
- 通用 stdio MCP servers 会 spawn 配置的 command，args 和 env 由 `subprocessEnv()` 与 server env 组合（`src/services/mcp/client.ts:944`）。
- MCP roots requests 只暴露 `file://getOriginalCwd()`（`src/services/mcp/client.ts:1009`）。
- Cleanup 会 close 或 terminate transports；stdio transports 依次收到 SIGINT、SIGTERM，必要时 SIGKILL（`src/services/mcp/client.ts:1404`）。
- 本地 needs-auth cache 存在 `~/.claude/mcp-needs-auth-cache.json`，TTL 为 15 分钟（`src/services/mcp/client.ts:257`）。

## 记忆与状态存储

### CLAUDE.md 记忆

CLAUDE.md 加载是分层的：

- 文档化的加载顺序是 managed、user、project、`.claude/CLAUDE.md`、`.claude/rules/*.md` 和 local；遍历从当前目录到 root，距离更近的文件优先级更高（`src/utils/claudemd.ts:1`）。
- Memory prompt content 上限为 40,000 字符（`src/utils/claudemd.ts:89`）。
- Includes 支持 text file extensions，并检测 circular includes（`src/utils/claudemd.ts:94`、`src/utils/claudemd.ts:618`）。
- `claudeMdExcludes` 适用于 user、project 和 local memory，但不适用于 managed、auto memory 或 team memory（`src/utils/claudemd.ts:537`）。
- Managed 与 user memory 先加载（`src/utils/claudemd.ts:790`）。
- Project 与 local memory 在从 cwd 向上遍历时加载（`src/utils/claudemd.ts:849`）。
- Local `CLAUDE.local.md` 受 local settings 控制（`src/utils/claudemd.ts:922`）。
- Additional directory memory loading 由 `CLAUDE_CODE_ADDITIONAL_DIRECTORIES_CLAUDE_MD` 控制（`src/utils/claudemd.ts:936`）。
- 启用时，auto-memory 与 team-memory entrypoints 会被追加（`src/utils/claudemd.ts:979`、`src/utils/claudemd.ts:994`）。

全局 memory 路径：

- User memory 是 `~/.claude/CLAUDE.md`（`src/utils/config.ts:1779`）。
- Local memory 是 `cwd/CLAUDE.local.md`（`src/utils/config.ts:1784`）。
- Project memory 是 `cwd/CLAUDE.md`（`src/utils/config.ts:1787`）。
- Managed memory 是 `<managedPath>/CLAUDE.md`（`src/utils/config.ts:1791`）。
- Auto-memory 使用 auto-memory entrypoint（`src/utils/config.ts:1793`）。

### 自动记忆

Auto-memory 面向 project，并以文件为后端：

- Auto-memory 可被 env、bare mode、没有 persistent memory 的 remote mode、settings 或 defaults 禁用（`src/memdir/paths.ts:21`）。
- Memory base 默认是 `~/.claude`；remote mode 下可用 `CLAUDE_CODE_REMOTE_MEMORY_DIR`（`src/memdir/paths.ts:80`）。
- 配置的 memory paths 必须是 absolute、non-root、non-UNC 且 non-null（`src/memdir/paths.ts:95`）。
- 可通过 `CLAUDE_COWORK_MEMORY_PATH_OVERRIDE` 完整覆盖路径（`src/memdir/paths.ts:152`）。
- Trusted settings overrides 出于安全排除 project settings（`src/memdir/paths.ts:168`）。
- Base project identity 是 canonical git root 或 project root，因此 worktrees 共享 memory（`src/memdir/paths.ts:198`）。
- 默认 auto-memory path 是 `<memoryBase>/projects/<sanitized-git-root>/memory/`（`src/memdir/paths.ts:208`）。
- Auto-memory entrypoint 是 `MEMORY.md`（`src/memdir/paths.ts:253`）。
- `MEMORY.md` 上限为 200 行和 25 KB（`src/memdir/memdir.ts:34`）。
- Auto-memory prompt 明确排除可从当前 project state 推导出的数据（`src/memdir/memdir.ts:187`）。

### Agent 与会话记忆

Agent memory 有 user、project 和 local scopes：

- Agent memory scopes 是 `user`、`project` 和 `local`（`src/tools/AgentTool/agentMemory.ts:12`）。
- User agent memory 位于 `<memoryBase>/agent-memory/<agentType>/`（`src/tools/AgentTool/agentMemory.ts:47`）。
- Project agent memory 位于 `.claude/agent-memory/<agentType>/`（`src/tools/AgentTool/agentMemory.ts:47`）。
- Local agent memory 是 remote project memory 或 `.claude/agent-memory-local/<agentType>/`（`src/tools/AgentTool/agentMemory.ts:24`）。
- Agent memory entrypoint 是 `MEMORY.md`（`src/tools/AgentTool/agentMemory.ts:109`）。

Session memory 是 per-session 且以文件为后端：

- Session memory 通过一个 background forked subagent 运行（`src/services/SessionMemory/sessionMemory.ts:1`）。
- 它有 feature-gated 的 cached enablement/config checks（`src/services/SessionMemory/sessionMemory.ts:80`）。
- Session memory setup 创建目录和 summary file，mode 分别为 `0700` 与 `0600`，然后通过 `FileReadTool` 读取（`src/services/SessionMemory/sessionMemory.ts:183`）。
- Session memory 注册 post-sampling hook（`src/services/SessionMemory/sessionMemory.ts:357`）。
- 被识别的 session memory path 是 `{projectDir}/{sessionId}/session-memory/summary.md`（`src/utils/permissions/filesystem.ts:257`）。

### 会话历史与运行时状态

Session history 存储在 project-scoped transcript files 中：

- Project directories 位于 `~/.claude/projects` 下（`src/utils/sessionStorage.ts:198`）。
- 当前 transcript path 是 `<projectDir>/<sessionId>.jsonl`（`src/utils/sessionStorage.ts:198`）。
- Current session path 可以使用单独的 `sessionProjectDir`；其他 session IDs 回退到 original cwd（`src/utils/sessionStorage.ts:207`）。
- Raw transcript reads 上限为 50 MB（`src/utils/sessionStorage.ts:227`）。
- Subagent transcripts 存在 `<projectDir>/<sessionId>/subagents[/subdir]/agent-<id>.jsonl`（`src/utils/sessionStorage.ts:231`）。
- Sidecar agent metadata 作为 `.meta.json` 存在 subagent transcript paths 旁边（`src/utils/sessionStorage.ts:260`）。
- Remote-agent metadata 存在 `<projectDir>/<sessionId>/remote-agents/` 下（`src/utils/sessionStorage.ts:320`）。
- Transcript writes 使用 mode `0600`，parent dirs 使用 `0700`（`src/utils/sessionStorage.ts:634`）。
- 在 tests、cleanup period 为 0、使用 `--no-session-persistence`，或设置 `CLAUDE_CODE_SKIP_PROMPT_HISTORY` 时，会跳过 session persistence（`src/utils/sessionStorage.ts:953`）。

Cleanup lifecycle：

- 默认 cleanup retention 是 30 天（`src/utils/cleanup.ts:23`）。
- Cleanup period 来自 `settings.cleanupPeriodDays`（`src/utils/cleanup.ts:25`）。
- Cleanup 覆盖 messages、sessions、plans、file history、session-env、debug logs、image caches、paste caches 和 stale worktrees（`src/utils/cleanup.ts:575`）。
- Old sessions 会跨 `~/.claude/projects` 清理，包括 JSONL、asciinema casts 和 tool results（`src/utils/cleanup.ts:155`）。
- `~/.claude/debug` 下的 debug logs 会被清理，同时保留 latest symlink（`src/utils/cleanup.ts:390`）。

### Cache、日志、凭据与临时文件

运行时 cache 与 logs：

- Cache paths 使用 `env-paths('claude-cli')`，包含 project-scoped base、errors、messages 和 MCP logs（`src/utils/cachePaths.ts:1`、`src/utils/cachePaths.ts:25`）。
- State/cache paths 使用 XDG defaults（`src/utils/xdg.ts:32`）。
- Debug logs 写入 `--debug-file`、`CLAUDE_CODE_DEBUG_LOGS_DIR`，或 `~/.claude/debug/<sessionId>.txt`（`src/utils/debug.ts:230`）。
- `latest` debug-log symlink 维护单独实现（`src/utils/debug.ts:238`）。
- Error logs 从 `CACHE_PATHS.errors` 加载（`src/utils/log.ts:209`）。
- MCP error/debug logs 通过 log sinks 路由（`src/utils/log.ts:300`）。
- API request logging 可保留 request parameters，但 Ant internal builds 会只在内存中保留 message contents（`src/utils/log.ts:341`）。

Tool 与 media outputs：

- Tool results 存储在 `<projectDir>/<sessionId>/tool-results/<id>`（`src/utils/toolResultStorage.ts:94`）。
- Tool-result writes 使用 exclusive creation（`wx`）（`src/utils/toolResultStorage.ts:122`）。
- Image cache 使用 `~/.claude/image-cache/<sessionId>`（`src/utils/imageStore.ts:18`）。
- Image files 以 mode `0600` 存储（`src/utils/imageStore.ts:54`）。
- Paste cache 使用 `~/.claude/paste-cache`（`src/utils/pasteStore.ts:13`）。
- Paste cache files 按 hash 命名，并以 mode `0600` 写入（`src/utils/pasteStore.ts:37`）。

Session environment 与临时目录：

- Session env files 位于 `~/.claude/session-env/<sessionId>`（`src/utils/sessionEnvironment.ts:15`）。
- Runtime 按确定性顺序加载 `CLAUDE_ENV_FILE` 和 hook env files（`src/utils/sessionEnvironment.ts:60`）。
- Claude temp dirs 在 Unix 上默认是 `/tmp/claude-<uid>`，在 Windows 上默认使用 OS temp dir，并支持 `CLAUDE_CODE_TMPDIR` override（`src/utils/permissions/filesystem.ts:302`）。
- Bundled skills 抽取到 `/tmp/claude-<uid>/bundled-skills/<VERSION>/<nonce>`（`src/utils/permissions/filesystem.ts:365`）。

Credentials 与 tokens：

- Remote CCR token files 是 `/home/claude/.claude/remote/.oauth_token`、`/home/claude/.claude/remote/.api_key` 和 `/home/claude/.claude/remote/.session_ingress_token`（`src/utils/authFileDescriptor.ts:13`）。
- Remote auth-file writes 使用 dir mode `0700` 和 file mode `0600`（`src/utils/authFileDescriptor.ts:30`）。
- Secure storage 在 macOS 上使用 Keychain 并有 plaintext fallback，在其他平台上使用 plaintext storage（`src/utils/secureStorage/index.ts:9`）。
- Plaintext secure storage file 是 `~/.claude/.credentials.json`（`src/utils/secureStorage/plainTextStorage.ts:13`）。
- Plaintext secure storage 以 mode `0600` 写入 JSON（`src/utils/secureStorage/plainTextStorage.ts:44`）。
- Bare auth 只使用 `ANTHROPIC_API_KEY` 或来自 `--settings` 的 `apiKeyHelper`（`src/utils/auth.ts:226`）。
- Normal API-key resolution 检查 approved env、file descriptors、API key helper cache、config 和 keychain（`src/utils/auth.ts:298`）。
- OAuth tokens 保存到 secure storage 的 `claudeAiOauth` 下，但 inference-only environment tokens 例外（`src/utils/auth.ts:1198`）。
- OAuth read path 检查 env、file descriptors，然后检查 secure storage；bare mode 跳过 OAuth（`src/utils/auth.ts:1255`）。

## 数据边界矩阵

| 边界 | 来源/路径 | 隔离 key | 加载/写入行为 | AVM 映射 |
| --- | --- | --- | --- | --- |
| 进程入口 | `src/entrypoints/cli.tsx` | process argv/env | 先处理早期特殊入口，再进入 `main.tsx` 正常 runtime | `runtime`: 进程 bootstrap 与模式分发 |
| Runtime cwd | `src/bootstrap/state.ts` | cwd/originalCwd/projectRoot | `originalCwd` 和 `projectRoot` 标识项目状态，不直接表示文件权限 | `runtime` + `state`: 区分执行 cwd 与状态身份 |
| Settings | `~/.claude/settings.json`、`.claude/settings.json`、`.claude/settings.local.json`、managed path、flag file | setting source | User < project < local < flag < policy 的合并优先级 | `state`: scoped config model |
| Global config | `~/.claude.json` 或 `$CLAUDE_CONFIG_DIR/.claude.json` | global user | 存储 project map、MCP servers、auth metadata、env、trusted dirs | `state`: user-global config boundary |
| Project config | global config 的 `projects` map | canonical git root 或 original cwd | `getCurrentProjectConfig()` 以 canonical path 标识项目状态 | `state`: project identity resolver |
| Tool permissions | settings、CLI、command、session | permission source + tool pattern | 中心化 `ToolPermissionContext` 和 rule evaluation | `adapter`: permission adapter contract |
| Filesystem reads | original cwd + additional dirs + internal paths | workspace/add-dir/project session | Sensitive path classifier 加 deny/ask/allow rules | `runtime`: workspace filesystem policy |
| Sandbox | external sandbox runtime config | platform/deps/settings | 默认禁用；可选 command wrapper 和 network/fs policies | `runtime`: optional isolation provider |
| Shell commands | 每条 command 一个 subprocess | cwd + env + sandbox flag | 使用 `subprocessEnv()` 和 runtime markers spawn shell | `adapter`: shell execution contract |
| Skills | managed/user/project/add-dir/plugin/bundled/MCP | source root、realpath、plugin ID | 目录 `SKILL.md`、nested discovery、conditional activation、MCP builders | `packageio`: skill package loader；`adapter`: prompt command surface |
| Plugins | `~/.claude/plugins` 或 override | plugin ID + marketplace + version + scope | Global installed metadata、versioned cache、per-plugin data | `packageio`: versioned package cache |
| MCP config | enterprise/local/project/user/plugin/dynamic/Claude.ai/managed | scope + server name + transport signature | Project approval、policy filter、merge precedence、dynamic override | `adapter`: MCP config resolver and launcher |
| MCP tokens | secure storage 与 needs-auth cache | server key hash | OAuth/XAA tokens 存在 secure storage；needs-auth TTL cache | `state`: credential store boundary |
| User memory | `~/.claude/CLAUDE.md` | user | 先于 project memory 加载 | `state`: user memory |
| Project memory | project `CLAUDE.md`、`.claude/CLAUDE.md`、`.claude/rules/*.md` | cwd traversal | root-down traversal，越近优先级越高，支持 includes | `state`: project memory |
| Auto memory | `~/.claude/projects/<project>/memory/MEMORY.md` | canonical git root/project root | Worktrees 共享 memory；project settings 不能选择任意路径 | `state`: long-term project memory |
| Agent memory | user/project/local agent memory dirs | agent type + scope | 每个 scope 与 agent type 一个 `MEMORY.md` | `state`: agent-scoped memory |
| Session history | `~/.claude/projects/<project>/<sessionId>.jsonl` | project + session ID | JSONL transcripts、subagent paths、sidecars、remote-agent metadata | `state`: session store |
| Tool results | `~/.claude/projects/<project>/<sessionId>/tool-results` | project + session ID | Exclusive write files，特殊 readable internal path | `state`: artifact store |
| Logs/cache | env-path cache、`~/.claude/debug`、`~/.claude/image-cache`、`~/.claude/paste-cache` | project/session/cache type | Retention cleanup，mode-restricted files | `state`: cache/log lifecycle |

## 证据表

| 结论 | 源码证据 |
| --- | --- |
| 正常 CLI 通过 `cli.tsx` 进入，然后导入 `main.tsx` | `src/entrypoints/cli.tsx:28`、`src/entrypoints/cli.tsx:287` |
| 特殊 MCP/native-host 入口绕过正常 runtime startup | `src/entrypoints/cli.tsx:72`、`src/entrypoints/cli.tsx:82`、`src/entrypoints/cli.tsx:87` |
| Daemon、worker、remote、bridge 和 background session paths 是独立 early modes | `src/entrypoints/cli.tsx:95`、`src/entrypoints/cli.tsx:112`、`src/entrypoints/cli.tsx:164`、`src/entrypoints/cli.tsx:182` |
| 主 runtime 检测 interactive/headless mode 并早期加载 settings | `src/main.tsx:797`、`src/main.tsx:851` |
| `init()` 在 trust 前只应用 safe environment | `src/entrypoints/init.ts:57`、`src/utils/managedEnv.ts:93`、`src/utils/managedEnv.ts:124` |
| 完整 environment application 只在 trust 后发生 | `src/utils/managedEnv.ts:180` |
| Settings merge order 是 user、project、local、flag、policy | `src/utils/settings/constants.ts:3` |
| Global config 默认 `~/.claude.json`；config home 默认 `~/.claude` | `src/utils/env.ts:13`、`src/utils/envUtils.ts:5` |
| Project identity 与 file-operation boundary 分离 | `src/bootstrap/state.ts:45`、`src/bootstrap/state.ts:496` |
| `setup()` 负责 cwd/session/messaging/worktree 初始化 | `src/setup.ts:56`、`src/setup.ts:86`、`src/setup.ts:160`、`src/setup.ts:174` |
| Shell commands 是 subprocesses，且可以 sandbox-wrapped | `src/utils/Shell.ts:177`、`src/utils/Shell.ts:259`、`src/utils/Shell.ts:315` |
| Sandbox defaults 不强制开启 sandbox | `src/utils/sandbox/sandbox-adapter.ts:459`、`src/utils/sandbox/sandbox-adapter.ts:532` |
| Filesystem permissions 包含 workspace dirs、sensitive paths 和显式 rule checks | `src/utils/permissions/filesystem.ts:57`、`src/utils/permissions/filesystem.ts:667`、`src/utils/permissions/filesystem.ts:1030` |
| 同一项目内的 project temp 可跨 sessions 读取 | `src/utils/permissions/filesystem.ts:1688` |
| Tool permission context 来自 CLI/settings/policy/session | `src/utils/permissions/permissionSetup.ts:721`、`src/utils/permissions/permissionsLoader.ts:120`、`src/utils/permissions/permissions.ts:109` |
| Skills 从 managed/user/project/add-dir/plugin/bundled/MCP 来源加载 | `src/skills/loadSkillsDir.ts:67`、`src/skills/loadSkillsDir.ts:638`、`src/skills/loadSkillsDir.ts:677`、`src/skills/bundledSkills.ts:43`、`src/skills/loadSkillsDir.ts:1077` |
| Filesystem skills 要求 `skills/<name>/SKILL.md` | `src/skills/loadSkillsDir.ts:403` |
| Plugin installs 使用 global metadata 加 versioned cache | `src/utils/plugins/installedPluginsManager.ts:76`、`src/utils/plugins/installedPluginsManager.ts:184` |
| Plugin skills 命名为 `pluginName:skill` | `src/utils/plugins/loadPluginCommands.ts:687` |
| MCP scopes 与 transports 集中建模 | `src/services/mcp/types.ts:10`、`src/services/mcp/types.ts:23` |
| MCP project/user/local/enterprise paths 与 precedence 是显式的 | `src/services/mcp/config.ts:625`、`src/services/mcp/config.ts:888`、`src/services/mcp/config.ts:1033`、`src/services/mcp/config.ts:1071` |
| Dynamic MCP config 可以在 runtime 覆盖 file configs | `src/main.tsx:1413`、`src/main.tsx:2380` |
| Project MCP 使用前需要 approval logic | `src/services/mcp/utils.ts:351` |
| Stdio MCP 会启动配置的 external command | `src/services/mcp/client.ts:944` |
| MCP roots 只暴露 original cwd | `src/services/mcp/client.ts:1009` |
| CLAUDE.md memory 加载顺序与 include 行为在 `claudemd.ts` 实现 | `src/utils/claudemd.ts:1`、`src/utils/claudemd.ts:618`、`src/utils/claudemd.ts:790`、`src/utils/claudemd.ts:849` |
| Auto-memory 基于 project-root，且 worktree-shared | `src/memdir/paths.ts:198`、`src/memdir/paths.ts:208`、`src/memdir/paths.ts:253` |
| Agent memory 有 user/project/local scopes | `src/tools/AgentTool/agentMemory.ts:12`、`src/tools/AgentTool/agentMemory.ts:47` |
| Session transcript paths 按 project/session scope 存储 | `src/utils/sessionStorage.ts:198`、`src/utils/sessionStorage.ts:231`、`src/utils/sessionStorage.ts:634` |
| Cleanup retention 默认 30 天，并覆盖主要状态类别 | `src/utils/cleanup.ts:23`、`src/utils/cleanup.ts:575` |
| 非 macOS secure storage 是 `~/.claude/.credentials.json` 明文文件 | `src/utils/secureStorage/index.ts:9`、`src/utils/secureStorage/plainTextStorage.ts:13` |

## AVM PRD 风险

- PRD 若假设 Claude Code state 只包含在 `~/.claude` 中，是不完整的。Global config 默认在 `~/.claude.json`，plugin/cache paths 可被 override，cache/log state 也使用 env-path directories（`src/utils/env.ts:13`、`src/utils/plugins/pluginDirectories.ts:49`、`src/utils/cachePaths.ts:1`）。
- PRD 若假设 cwd 是唯一 project boundary，是不完整的。Claude Code 分别追踪 cwd、original cwd、project root、session project dir、canonical git root 和 additional dirs（`src/bootstrap/state.ts:45`、`src/bootstrap/state.ts:402`、`src/memdir/paths.ts:198`）。
- PRD 若假设 sandboxing 总是 active，在此源码中是错误的。Sandbox defaults 设置 `enabled: false`，且 `allowUnsandboxedCommands` 默认 true（`src/utils/sandbox/sandbox-adapter.ts:459`）。
- PRD 若假设只靠 sandboxing 定义隔离，是不完整的。在未启用 sandbox 时，filesystem 与 shell 边界依赖 permission rules、path classifiers、trust prompts 和 tool-specific validators（`src/utils/permissions/filesystem.ts:1030`、`src/utils/permissions/permissions.ts:473`）。
- PRD 若假设 project state 完全按 session 隔离，对部分 internal paths 是错误的。同一 project 内 project temp directories 可跨 sessions 读取（`src/utils/permissions/filesystem.ts:1688`）。
- PRD 若假设 worktrees 拥有独立 long-term project memory，默认是错误的。Auto-memory 以 canonical git root 或 project root 作为 key，因此 worktrees 共享 memory（`src/memdir/paths.ts:198`）。
- PRD 若假设 skills 是静态 packages，是不完整的。Skills 可从嵌套 project paths 发现，可 conditional load，可由 plugins 提供，可 bundled 到 CLI 中，也可由 MCP 暴露（`src/skills/loadSkillsDir.ts:861`、`src/skills/loadSkillsDir.ts:997`、`src/skills/loadSkillsDir.ts:1077`）。
- PRD 若假设 plugin install scope 等同于 storage scope，是不完整的。Installation metadata 是 global，versioned cache 是 global，project/local relevance 再按 project path 过滤（`src/utils/plugins/installedPluginsManager.ts:1`、`src/utils/plugins/installedPluginsManager.ts:785`）。
- PRD 若假设 MCP config 只有简单 global/project precedence，是错误的。Enterprise exclusivity、local/project/user/plugin precedence、project approval、policy filters、strict mode、bare mode 和 dynamic overrides 都会影响最终 runtime MCP 集合（`src/services/mcp/config.ts:1071`、`src/main.tsx:1799`、`src/main.tsx:2380`）。
- PRD 若假设 remote MCP authorization 完全外部化，是不完整的。Claude Code 会通过 secure storage 存储 OAuth/XAA tokens，并维护本地 needs-auth cache（`src/services/mcp/auth.ts:349`、`src/services/mcp/client.ts:257`）。
- PRD 若假设 credential storage 总是 OS-secure，在该源码的非 macOS 平台上是错误的。Secure storage fallback 到明文 `~/.claude/.credentials.json`（`src/utils/secureStorage/index.ts:9`、`src/utils/secureStorage/plainTextStorage.ts:13`）。

## 未决问题

- `@anthropic-ai/sandbox-runtime` 的具体 OS-level guarantees 在本仓库快照中不可见。本调研能验证 Claude Code 何时调用 sandbox adapter，但不能验证外部 sandbox 实现。
- Build-time feature flags 会影响 TeamMem、Buddy、Kairos、bundled skills 和若干 remote behaviors。准确的 production feature set 不能仅从静态源码推导。
- 该快照缺少 `package.json`、lockfiles、Makefile 和已构建 executable，因此无法在本地运行 CLI `--help`、build 和 test validation。
- Claude.ai connector config 在 runtime startup 中异步获取，但 server-side connector selection 与 policy 不在此源码快照中。
- 部分注释描述 Ant-internal behavior 和 sandbox requirements。若不确认 build macros 与 distribution settings，这些路径不一定适用于 public Claude Code builds。
