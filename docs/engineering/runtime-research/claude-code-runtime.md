# Claude Code Runtime Research

## Summary

This document traces the Claude Code runtime implementation in
`frameworks/claude-code-main` for Agent VM PRD validation. The findings are
source-derived; README material was not used as evidence.
Unless noted otherwise, source references are relative to
`frameworks/claude-code-main/`.

Key findings:

- The executable bootstrap is `src/entrypoints/cli.tsx`. The normal path does
  early flag dispatch, enables special entrypoints, then imports `src/main.tsx`
  and calls `cliMain()` (`src/entrypoints/cli.tsx:28`,
  `src/entrypoints/cli.tsx:287`).
- Runtime initialization is deliberately phased. `init()` applies only trusted
  environment sources before project trust, while `setup()` later establishes
  cwd, workspace identity, messaging, session memory, watchers, hooks, and
  permission bypass checks (`src/entrypoints/init.ts:57`,
  `src/utils/managedEnv.ts:93`, `src/setup.ts:56`,
  `src/setup.ts:160`).
- The runtime distinguishes operational cwd from project identity. `originalCwd`
  and `projectRoot` are used for project-scoped state such as history, skills,
  and sessions, not as a direct file-operation boundary
  (`src/bootstrap/state.ts:45`, `src/bootstrap/state.ts:496`).
- File and command isolation is policy-driven rather than VM-like by default.
  A sandbox adapter exists, but sandboxing depends on platform, dependencies,
  and settings; default sandbox settings enable auto-allow semantics but do not
  force sandboxing on (`src/utils/sandbox/sandbox-adapter.ts:459`,
  `src/utils/sandbox/sandbox-adapter.ts:532`).
- Skills are prompt-command files loaded from managed, user, project, added
  directories, plugins, bundled skill registries, and MCP-provided skill
  builders (`src/skills/loadSkillsDir.ts:67`,
  `src/skills/loadSkillsDir.ts:638`,
  `src/skills/bundledSkills.ts:43`,
  `src/skills/loadSkillsDir.ts:1077`).
- MCP configurations are scoped and merged across enterprise, local, project,
  user, plugin, dynamic, managed, Claude.ai, and SDK sources. Project MCP files
  require approval before loading; enterprise policy can make managed MCP
  exclusive (`src/services/mcp/types.ts:10`,
  `src/services/mcp/config.ts:888`,
  `src/services/mcp/config.ts:1071`,
  `src/services/mcp/utils.ts:351`).
- Persistent state is spread across `~/.claude`, `~/.claude.json`,
  project-scoped directories under `~/.claude/projects`, env-path cache dirs,
  keychain or plaintext secure storage, and optional remote/config overrides
  (`src/utils/env.ts:13`, `src/utils/sessionStorage.ts:198`,
  `src/utils/cachePaths.ts:1`, `src/utils/secureStorage/plainTextStorage.ts:13`).

Safe verification performed:

- The framework snapshot contains `src/`, `Doc/`, `README.md`, and `CLAUDE.md`,
  but no `package.json`, lockfile, Makefile, or built executable. Therefore no
  runnable `--help`, build, or test command was available in this checkout.
- Entrypoint and runtime behavior were verified by tracing TypeScript source
  files under `frameworks/claude-code-main/src`.

## Runtime Startup Path

### Entrypoint Dispatch

The CLI starts in `src/entrypoints/cli.tsx`. Before importing the full runtime,
it sets process-level flags and handles fast paths:

- Disables Corepack auto-pinning via `COREPACK_ENABLE_AUTO_PIN=0`
  (`src/entrypoints/cli.tsx:1`).
- For remote mode, appends `--max-old-space-size=8192` to `NODE_OPTIONS`
  (`src/entrypoints/cli.tsx:7`).
- Performs early ablation environment processing
  (`src/entrypoints/cli.tsx:21`).
- Parses raw args and handles `--version` before full startup
  (`src/entrypoints/cli.tsx:33`).
- Dispatches special MCP/native-host modes before normal runtime:
  Chrome MCP, Chrome native host, and computer-use MCP
  (`src/entrypoints/cli.tsx:72`, `src/entrypoints/cli.tsx:82`,
  `src/entrypoints/cli.tsx:87`).
- Dispatches daemon, daemon-worker, remote-control, bridge, remote/sync, and
  background-session commands before normal interactive startup
  (`src/entrypoints/cli.tsx:95`, `src/entrypoints/cli.tsx:112`,
  `src/entrypoints/cli.tsx:164`, `src/entrypoints/cli.tsx:182`).
- Handles `--worktree --tmux` before full CLI import because it may exec into
  tmux (`src/entrypoints/cli.tsx:247`).
- Handles `--bare` before full import by setting `CLAUDE_CODE_SIMPLE=1`
  (`src/entrypoints/cli.tsx:281`).
- Normal startup captures early input, imports `../main.js`, and calls
  `cliMain()` (`src/entrypoints/cli.tsx:287`).

### Main Runtime Initialization

`src/main.tsx` owns the Commander CLI and the main runtime path:

- `main()` sets `NoDefaultCurrentDirectoryInExePath=1`, installs signal
  handlers, and detects interactive versus non-interactive modes
  (`src/main.tsx:585`, `src/main.tsx:595`, `src/main.tsx:797`).
- Client type is inferred from environment, entrypoint, session-ingress, and
  runtime context (`src/main.tsx:817`).
- Settings are eagerly loaded before the command runner is built
  (`src/main.tsx:851`).
- `run()` constructs the Commander program (`src/main.tsx:884`).
- The Commander `preAction` waits for managed policy/keychain prefetch, calls
  `init()`, initializes process metadata and event sinks, propagates plugin-dir
  settings, runs migrations, and starts background remote settings/policy sync
  (`src/main.tsx:905`).

CLI options establish several runtime boundaries:

- Permission and tool boundaries: `--dangerously-skip-permissions`,
  `--allow-dangerously-skip-permissions`, `--permission-mode`,
  `--allowed-tools`, `--tools`, `--disallowed-tools`
  (`src/main.tsx:968`).
- MCP boundaries: `--mcp-config`, `--strict-mcp-config`
  (`src/main.tsx:986`, `src/main.tsx:1003`).
- State/session boundaries: `--continue`, `--resume`, `--fork-session`,
  `--no-session-persistence`, `--session-id`
  (`src/main.tsx:991`, `src/main.tsx:1005`).
- Workspace extension and plugin boundaries: `--add-dir`, `--agents`,
  `--setting-sources`, `--plugin-dir`, `--disable-slash-commands`
  (`src/main.tsx:999`, `src/main.tsx:1006`).

### Config And Environment Loading

Configuration has both global and source-scoped settings:

- The global config file defaults to `~/.claude.json`, or
  `$CLAUDE_CONFIG_DIR/.claude.json` when `CLAUDE_CONFIG_DIR` is set
  (`src/utils/env.ts:13`).
- The config home defaults to `~/.claude`, or `CLAUDE_CONFIG_DIR`
  (`src/utils/envUtils.ts:5`).
- Settings source priority is user, project, local, flag, then policy, with
  later sources overriding earlier ones (`src/utils/settings/constants.ts:3`).
- Setting file paths are user `~/.claude/settings.json`, project
  `.claude/settings.json`, local `.claude/settings.local.json`, policy managed
  settings, and optional flag settings (`src/utils/settings/settings.ts:274`).
- Managed settings are loaded from a platform managed path and optional
  `managed-settings.d/*.json` drop-ins, where drop-ins override alphabetically
  (`src/utils/settings/settings.ts:55`, `src/utils/settings/settings.ts:74`).
- Managed path defaults are `/etc/claude-code` on Linux,
  `/Library/Application Support/ClaudeCode` on macOS, and
  `C:\Program Files\ClaudeCode` on Windows, with an Ant-specific environment
  override (`src/utils/settings/managedPath.ts:8`).

Environment application is split by trust:

- Before project trust, only user, flag, and policy environment sources are
  trusted; project and local settings are excluded because they could redirect
  traffic (`src/utils/managedEnv.ts:93`).
- Before trust, only allowlisted safe environment variables from the full
  merged settings may be applied (`src/utils/managedEnv.ts:124`,
  `src/utils/managedEnvConstants.ts:108`).
- After trust, all environment variables from global config and merged settings
  are applied and proxy/mTLS/CA caches are reset (`src/utils/managedEnv.ts:180`).
- Bare mode disables hooks, LSP, plugin sync, skill directory walking,
  attribution, background prefetches, and keychain/credential reads by default
  (`src/utils/envUtils.ts:49`).

### Workspace And Session Context

`setup()` is the main workspace/session initializer:

- It accepts cwd, permission mode, dangerous-permission flags, worktree/tmux
  options, custom session ID, PR number, and optional messaging socket path
  (`src/setup.ts:56`).
- It validates Node version >= 18 (`src/setup.ts:69`).
- It can switch to a custom session ID before runtime state is built
  (`src/setup.ts:81`).
- It starts a Unix domain socket messaging server unless bare mode is active
  without an explicit socket path, then exports `CLAUDE_CODE_MESSAGING_SOCKET`
  (`src/setup.ts:86`).
- It sets cwd, snapshots hook config, and starts a file-changed watcher
  (`src/setup.ts:160`).
- In `--worktree` mode, it validates git/hook state, creates or enters a
  worktree, optionally manages tmux, changes process cwd, updates original cwd
  and project root, saves worktree state, and clears memory caches
  (`src/setup.ts:174`).
- In non-bare mode, it initializes session memory/context collapse
  (`src/setup.ts:293`).
- It preloads commands and plugin hooks unless plugin prefetch is skipped
  (`src/setup.ts:321`).
- It registers attribution, session file access hooks, and team memory watcher
  in non-bare mode (`src/setup.ts:336`).

Runtime identity state is centralized in `src/bootstrap/state.ts`:

- `originalCwd` and `projectRoot` are explicitly for project identity such as
  history, skills, and sessions, not file operations (`src/bootstrap/state.ts:45`).
- Initial cwd, original cwd, and project root are realpath-based at process
  initialization (`src/bootstrap/state.ts:259`).
- A random session ID is generated by default (`src/bootstrap/state.ts:331`).
- Session trust and session-persistence flags are in-memory session state
  (`src/bootstrap/state.ts:362`).
- Additional directories and session project directory are tracked separately
  (`src/bootstrap/state.ts:402`).
- `switchSession()` can replace the active session ID and optionally its
  project directory (`src/bootstrap/state.ts:456`).

Session context loads system and user memory:

- Git status includes current branch/status/log and configured user name when
  inside a git repository (`src/context.ts:36`).
- System context skips git status in remote mode or when disabled
  (`src/context.ts:113`).
- User context loads CLAUDE.md memory files unless disabled or bare without
  additional directories (`src/context.ts:155`).

## Isolation Model

### Process Isolation

Claude Code does not create a per-agent VM/container boundary in the traced
source. Process isolation is mostly per-command subprocess execution plus
optional sandbox wrapping:

- Each shell command creates a new shell process (`src/utils/Shell.ts:177`).
- Shell execution accepts `preventCwdChanges` and `shouldUseSandbox`
  (`src/utils/Shell.ts:181`).
- If sandboxing is enabled for a command, the command is wrapped and a
  per-command temporary directory is created with mode `0700`
  (`src/utils/Shell.ts:259`).
- Subprocesses inherit `subprocessEnv()`, shell/editor markers, cwd, and Claude
  runtime markers (`src/utils/Shell.ts:315`).
- Foreground shell tasks can update the runtime cwd through a tracking file;
  after that, session environment and hooks are invalidated
  (`src/utils/Shell.ts:394`).
- Sandbox cleanup runs after sandboxed shell commands
  (`src/utils/Shell.ts:385`).

The Bash tool decides whether to request sandboxing:

- Excluded commands are explicitly documented as not being a security boundary
  (`src/tools/BashTool/shouldUseSandbox.ts:18`).
- Sandbox use is disabled when sandboxing is unavailable/disabled,
  dangerous-disable is allowed, the command is empty, or the command is excluded
  (`src/tools/BashTool/shouldUseSandbox.ts:130`).
- Bash execution passes timeout, cwd-change prevention, sandbox decision, and
  auto-background settings into `Shell.exec()` (`src/tools/BashTool/BashTool.tsx:877`).

### Filesystem Boundary

The filesystem model combines workspace roots, sensitive path classification,
permission rules, and sandbox rules:

- Dangerous file basenames include shell rc files, git config/module files,
  `.mcp.json`, and Claude config files
  (`src/utils/permissions/filesystem.ts:57`).
- Dangerous directories include `.git`, `.vscode`, `.idea`, and `.claude`
  (`src/utils/permissions/filesystem.ts:74`).
- Allowed working directories are the original cwd plus any additional
  directories (`src/utils/permissions/filesystem.ts:667`).
- `pathInWorkingPath()` resolves the target path against the current working
  path set (`src/utils/permissions/filesystem.ts:709`).
- Read-deny patterns are loaded from permission rules
  (`src/utils/permissions/filesystem.ts:837`).
- `checkReadPermissionForTool()` blocks UNC/suspicious paths, honors deny/ask
  rules, then checks edit access, working dirs, internal paths, and allow rules
  (`src/utils/permissions/filesystem.ts:1030`).
- Tool result directories are readable for tool output recovery
  (`src/utils/permissions/filesystem.ts:1660`).
- Scratchpad files for the current session are readable
  (`src/utils/permissions/filesystem.ts:1676`).
- Project temp directories are readable across all sessions in the same project
  (`src/utils/permissions/filesystem.ts:1688`).
- Agent memory and auto-memory paths are readable as special internal paths
  (`src/utils/permissions/filesystem.ts:1703`).

Skill directories receive explicit path recognition:

- Project and global `.claude/skills/<skill>/**` scopes are recognized as skill
  paths (`src/utils/permissions/filesystem.ts:94`).
- Claude-owned config files include settings, commands, agents, and skills
  under `.claude` (`src/utils/permissions/filesystem.ts:224`).

### Sandbox Boundary

The sandbox adapter delegates enforcement to `@anthropic-ai/sandbox-runtime`:

- The external runtime is imported as `BaseSandboxManager`
  (`src/utils/sandbox/sandbox-adapter.ts:1`).
- Settings permission rules are translated into sandbox filesystem path rules
  (`src/utils/sandbox/sandbox-adapter.ts:83`,
  `src/utils/sandbox/sandbox-adapter.ts:121`).
- Managed-only network and read-path policies are represented in sandbox config
  (`src/utils/sandbox/sandbox-adapter.ts:148`).
- Network allow/deny is derived from `sandbox.network.allowedDomains` and
  `WebFetch(domain:...)` permission rules (`src/utils/sandbox/sandbox-adapter.ts:177`).
- Sandbox writes are initially allowed for `.` and the Claude temp directory
  (`src/utils/sandbox/sandbox-adapter.ts:222`).
- Settings paths and managed settings drop-ins are always denied
  (`src/utils/sandbox/sandbox-adapter.ts:230`).
- The current and original `.claude/settings*.json` and `.claude/skills` paths
  are explicitly denied (`src/utils/sandbox/sandbox-adapter.ts:238`).
- Bare git repo files are denied or scrubbed
  (`src/utils/sandbox/sandbox-adapter.ts:257`).
- Worktree main repo and additional directories are allowed when configured
  (`src/utils/sandbox/sandbox-adapter.ts:282`).
- Default sandbox settings set `enabled: false`, `autoAllow: true`, and
  `allowUnsandboxedCommands: true` (`src/utils/sandbox/sandbox-adapter.ts:459`).
- Sandbox enablement requires platform support, dependencies, configured
  enabled platforms, and `sandbox.enabled` (`src/utils/sandbox/sandbox-adapter.ts:532`).
- Initialization updates runtime sandbox config and registers settings-change
  handling (`src/utils/sandbox/sandbox-adapter.ts:730`).

### Tool Permission Model

Tool permission context is constructed from CLI flags, settings, policy, and
session state:

- Permission mode resolution accounts for CLI, settings, dangerous bypass,
  and auto mode (`src/utils/permissions/permissionSetup.ts:721`).
- CLI allowed/disallowed/base tool lists are parsed into permission rules
  (`src/utils/permissions/permissionSetup.ts:892`).
- Broad Bash and PowerShell permissions are detected and may be stripped in auto
  mode (`src/utils/permissions/permissionSetup.ts:948`).
- Loaded permission rules come from all enabled settings sources unless policy
  restricts them to managed rules only
  (`src/utils/permissions/permissionsLoader.ts:120`).
- Editable permission sources are user, project, and local
  (`src/utils/permissions/permissionsLoader.ts:151`).
- Permission sources include settings sources, CLI args, commands, and session
  source (`src/utils/permissions/permissions.ts:109`).
- `hasPermissionsToUseTool()` is the central tool permission evaluator
  (`src/utils/permissions/permissions.ts:473`).

Dangerous permission bypass is constrained:

- `setup()` blocks bypass mode as root unless a sandbox environment marker is
  present (`src/setup.ts:395`).
- Ant builds require Docker, bubblewrap, or `IS_SANDBOX=1` and no internet
  environment for bypass mode (`src/setup.ts:414`).

## Skill Installation And Loading

### Skill Format And Sources

Skills are loaded as command-like prompt definitions:

- `LoadedFrom` distinguishes `skills`, `plugin`, `managed`, `bundled`, and
  `mcp` sources (`src/skills/loadSkillsDir.ts:67`).
- Source roots include policy managed, user `~/.claude`, project `.claude`, and
  plugin directories (`src/skills/loadSkillsDir.ts:78`).
- Skill frontmatter supports `description`, `allowed-tools`, `argument-hint`,
  `when_to_use`, `version`, model/effort fields, user-invocable flag, hooks,
  context forking, agent, and shell fields (`src/skills/loadSkillsDir.ts:185`).
- Skill directories are discovered only as `skills/<skill-name>/SKILL.md`; single
  Markdown files under `skills/` are not supported as skills
  (`src/skills/loadSkillsDir.ts:403`).
- The `/skills` menu filters commands loaded from skills, deprecated command
  files, plugin skills, and MCP skills (`src/components/skills/SkillsMenu.tsx:234`).

### Discovery Priority

Normal auto-discovery loads skills from multiple roots:

- Managed, user, and project directories are computed first
  (`src/skills/loadSkillsDir.ts:638`).
- Bare mode skips auto-discovery except explicit `--add-dir` paths when project
  settings are enabled and skills are not locked (`src/skills/loadSkillsDir.ts:648`).
- Normal mode loads managed, user, project, additional, and legacy command
  directories in parallel; user/project loading is gated by settings and
  plugin-only policy (`src/skills/loadSkillsDir.ts:677`).
- Loaded commands are de-duplicated by realpath, with first-wins behavior
  (`src/skills/loadSkillsDir.ts:716`).
- Nested `.claude/skills` directories are discovered while walking from an
  operated file up to cwd, excluding cwd and gitignored directories; deeper
  directories take priority (`src/skills/loadSkillsDir.ts:861`).
- Dynamic skill directories are added only when project settings are enabled
  and plugin-only policy is not active (`src/skills/loadSkillsDir.ts:923`).
- Conditional skills can be activated by path-based frontmatter
  (`src/skills/loadSkillsDir.ts:997`).
- MCP skill builders are registered separately from filesystem skill discovery
  (`src/skills/loadSkillsDir.ts:1077`).

### Bundled Skills

Bundled skills are compiled into the CLI and extracted lazily:

- The bundled skill registry is internal to the process
  (`src/skills/bundledSkills.ts:43`).
- Reference files are extracted on first invocation, not during startup
  (`src/skills/bundledSkills.ts:59`).
- Bundled skill command objects identify their source as `bundled`
  (`src/skills/bundledSkills.ts:75`).
- Extraction writes into a versioned bundled-skills directory under the Claude
  temp root and uses restrictive directory/file modes plus `O_NOFOLLOW` and
  `O_EXCL` (`src/skills/bundledSkills.ts:120`,
  `src/skills/bundledSkills.ts:147`).
- Extracted reference paths are validated to prevent traversal
  (`src/skills/bundledSkills.ts:195`).
- Built-in bundled skills are registered by `initBundledSkills()`
  (`src/skills/bundled/index.ts:24`).

### Plugin-Provided Skills

Claude Code also treats plugins as skill/package carriers:

- The plugin cache dir defaults to `~/.claude/plugins`, with
  `CLAUDE_CODE_PLUGIN_CACHE_DIR` and `--plugin-dir` overrides
  (`src/utils/plugins/pluginDirectories.ts:1`,
  `src/utils/plugins/pluginDirectories.ts:49`).
- Read-only seed directories can be supplied via `CLAUDE_CODE_PLUGIN_SEED_DIR`;
  the first seed hit wins (`src/utils/plugins/pluginDirectories.ts:66`).
- Per-plugin data is stored under `plugins/data/<sanitizedPluginId>` and exposed
  to plugin code as `CLAUDE_PLUGIN_DATA`; it survives updates and is removed on
  last-scope uninstall (`src/utils/plugins/pluginDirectories.ts:97`).
- Plugin installation metadata is global, while enable/disable state is stored
  per repository/settings scope (`src/utils/plugins/installedPluginsManager.ts:1`).
- Installed plugin metadata is stored in `<pluginsDir>/installed_plugins.json`
  (`src/utils/plugins/installedPluginsManager.ts:76`).
- Versioned plugin cache paths are
  `~/.claude/plugins/cache/{marketplace}/{plugin}/{version}`
  (`src/utils/plugins/installedPluginsManager.ts:184`).
- Plugin relevance is global for user/managed scopes and project-specific for
  project/local scopes (`src/utils/plugins/installedPluginsManager.ts:785`).
- Version resolution prioritizes manifest version, provided version, git SHA,
  then `unknown` (`src/utils/plugins/pluginVersioning.ts:19`).
- Plugin cache installation supports npm, git, git-subdir, and local sources,
  then validates `.claude-plugin/plugin.json` or legacy metadata before moving
  the temp cache to the final versioned path (`src/utils/plugins/pluginLoader.ts:492`,
  `src/utils/plugins/pluginLoader.ts:534`,
  `src/utils/plugins/pluginLoader.ts:718`,
  `src/utils/plugins/pluginLoader.ts:856`,
  `src/utils/plugins/pluginLoader.ts:911`).
- Plugin skills can come from direct `SKILL.md` files or skill subdirectories
  and are named with a `pluginName:skill` prefix
  (`src/utils/plugins/loadPluginCommands.ts:687`).
- Bare mode skips marketplace plugin auto-loading unless an explicit plugin dir
  is supplied (`src/utils/plugins/loadPluginCommands.ts:840`).

## MCP Installation And Loading

### MCP Config Model

MCP config is typed by scope and transport:

- Config scopes are `local`, `user`, `project`, `dynamic`, `enterprise`,
  `claudeai`, and `managed` (`src/services/mcp/types.ts:10`).
- Supported transport types include stdio, SSE, HTTP, WebSocket, SDK, and
  Claude.ai proxy (`src/services/mcp/types.ts:23`).
- Stdio server config includes command, args, and env fields
  (`src/services/mcp/types.ts:28`).
- HTTP/SSE configs support OAuth and headers (`src/services/mcp/types.ts:43`,
  `src/services/mcp/types.ts:89`).
- `.mcp.json` stores project-scoped MCP servers as `mcpServers`
  (`src/services/mcp/types.ts:163`).

### MCP Write Paths And Scope Priority

MCP config can be added through CLI or parsed from files:

- `mcp add <name> <commandOrUrl> [args...]` supports `--scope`, `--transport`,
  env vars, headers, OAuth client data, and XAA options
  (`src/commands/mcp/addCommand.ts:33`).
- Project MCP writes to `.mcp.json`; user MCP writes to global config; local MCP
  writes into the current project config. Dynamic, enterprise, and Claude.ai
  scopes are not addable through `addMcpConfig()`
  (`src/services/mcp/config.ts:625`).
- Project `.mcp.json` is written atomically while preserving permissions
  (`src/services/mcp/config.ts:83`).
- Enterprise MCP config path is `<managedPath>/managed-mcp.json`
  (`src/services/mcp/config.ts:62`).
- Project config walks from cwd up to root, then applies configs root-down so
  closer `.mcp.json` files override parent directories
  (`src/services/mcp/config.ts:888`, `src/services/mcp/config.ts:913`).
- User MCP config is loaded from `getGlobalConfig().mcpServers`
  (`src/services/mcp/config.ts:963`).
- Local MCP config is loaded from `getCurrentProjectConfig().mcpServers`
  (`src/services/mcp/config.ts:979`).
- Enterprise MCP config is loaded from the managed MCP file
  (`src/services/mcp/config.ts:996`).
- Lookup by name uses enterprise, local, project, then user precedence; when
  plugin-only policy is enabled, only enterprise survives by name
  (`src/services/mcp/config.ts:1033`).

The main merged runtime path has additional precedence:

- Enterprise MCP can be exclusive and prevent non-enterprise config loading
  (`src/services/mcp/config.ts:1071`).
- Otherwise Claude Code loads user, project, and local configs unless
  plugin-only policy blocks them, then plugin MCP configs, requires project MCP
  approval, de-duplicates plugin servers against manual servers, and merges with
  effective precedence plugin < user < project < local
  (`src/services/mcp/config.ts:1071`).
- Dynamic `--mcp-config` input is parsed from JSON or a file and filtered by
  enterprise policy; reserved names are rejected (`src/main.tsx:1413`).
- During runtime startup, strict MCP config or bare mode skips auto-discovered
  MCP configs, but dynamic MCP config survives downstream
  (`src/main.tsx:1799`).
- Later startup merges existing MCP config with dynamic config; dynamic servers
  override file configs (`src/main.tsx:2380`).

### MCP Approval, Authorization, And Connection

Project MCP and remote MCP have authorization gates:

- Project `.mcp.json` server approval is tracked through disabled/enabled MCP
  settings and can be auto-approved in bypass/non-interactive mode when project
  settings are enabled (`src/services/mcp/utils.ts:351`).
- Policy filtering supports enterprise allowlists and denylists by name,
  command, and URL (`src/services/mcp/config.ts:341`,
  `src/services/mcp/config.ts:364`).
- Environment variables in MCP command, args, env, URL, and headers are expanded
  before use (`src/services/mcp/config.ts:556`).
- Project/local `headersHelper` commands cannot run before trust in interactive
  sessions; non-interactive mode skips that trust check
  (`src/services/mcp/headersHelper.ts:40`).
- OAuth authorization-server metadata URLs must be HTTPS; project MCP approval
  is noted as the defense against malicious project MCP config
  (`src/services/mcp/auth.ts:239`).
- MCP OAuth server keys include server name and a SHA-256 hash of type, URL, and
  headers (`src/services/mcp/auth.ts:325`).
- OAuth and XAA tokens are stored through secure storage under MCP-specific keys
  (`src/services/mcp/auth.ts:349`, `src/services/mcp/auth.ts:647`,
  `src/services/mcp/auth.ts:793`).
- XAA IDP tokens are also cached in secure storage
  (`src/services/mcp/xaaIdpLogin.ts:95`).

Transport behavior:

- MCP connection timeout defaults to 30 seconds or `MCP_TIMEOUT`
  (`src/services/mcp/client.ts:456`).
- Batch sizes are environment-tunable (`src/services/mcp/client.ts:552`).
- Stdio and SDK transports are classified as local
  (`src/services/mcp/client.ts:563`).
- SSE and HTTP clients attach OAuth providers, static and dynamic headers, and
  proxy/timeout behavior (`src/services/mcp/client.ts:595`,
  `src/services/mcp/client.ts:784`).
- Claude.ai proxy transport uses Claude.ai OAuth and
  `X-Mcp-Client-Session-Id` (`src/services/mcp/client.ts:868`).
- Chrome and computer-use MCP servers run as in-process stdio transports
  (`src/services/mcp/client.ts:905`).
- Generic stdio MCP servers spawn the configured command with args and env
  composed from `subprocessEnv()` and server env
  (`src/services/mcp/client.ts:944`).
- MCP roots requests expose only `file://getOriginalCwd()`
  (`src/services/mcp/client.ts:1009`).
- Cleanup closes or terminates transports; stdio transports receive SIGINT,
  SIGTERM, then SIGKILL if needed (`src/services/mcp/client.ts:1404`).
- A local needs-auth cache is stored at
  `~/.claude/mcp-needs-auth-cache.json` with a 15-minute TTL
  (`src/services/mcp/client.ts:257`).

## Memory And State Storage

### CLAUDE.md Memory

CLAUDE.md loading is layered:

- Documented load order is managed, user, project, `.claude/CLAUDE.md`,
  `.claude/rules/*.md`, and local, with traversal from current directory to
  root and closer files taking higher priority (`src/utils/claudemd.ts:1`).
- Memory prompt content is capped at 40,000 characters
  (`src/utils/claudemd.ts:89`).
- Includes support text file extensions and detect circular includes
  (`src/utils/claudemd.ts:94`, `src/utils/claudemd.ts:618`).
- `claudeMdExcludes` applies to user, project, and local memory, but not
  managed, auto memory, or team memory (`src/utils/claudemd.ts:537`).
- Managed and user memory are loaded first
  (`src/utils/claudemd.ts:790`).
- Project and local memory are loaded while walking upward from cwd
  (`src/utils/claudemd.ts:849`).
- Local `CLAUDE.local.md` is gated by local settings
  (`src/utils/claudemd.ts:922`).
- Additional directory memory loading is controlled by
  `CLAUDE_CODE_ADDITIONAL_DIRECTORIES_CLAUDE_MD`
  (`src/utils/claudemd.ts:936`).
- Auto-memory and team-memory entrypoints are appended when enabled
  (`src/utils/claudemd.ts:979`, `src/utils/claudemd.ts:994`).

Global memory paths:

- User memory is `~/.claude/CLAUDE.md`
  (`src/utils/config.ts:1779`).
- Local memory is `cwd/CLAUDE.local.md`
  (`src/utils/config.ts:1784`).
- Project memory is `cwd/CLAUDE.md`
  (`src/utils/config.ts:1787`).
- Managed memory is `<managedPath>/CLAUDE.md`
  (`src/utils/config.ts:1791`).
- Auto-memory uses the auto-memory entrypoint
  (`src/utils/config.ts:1793`).

### Auto Memory

Auto-memory is project-oriented and file-backed:

- Auto-memory can be disabled by env, bare mode, remote mode without persistent
  memory, settings, or defaults (`src/memdir/paths.ts:21`).
- Memory base defaults to `~/.claude`, or `CLAUDE_CODE_REMOTE_MEMORY_DIR` in
  remote mode (`src/memdir/paths.ts:80`).
- Configured memory paths must be absolute, non-root, non-UNC, and non-null
  (`src/memdir/paths.ts:95`).
- Full path override is available through `CLAUDE_COWORK_MEMORY_PATH_OVERRIDE`
  (`src/memdir/paths.ts:152`).
- Trusted settings overrides exclude project settings for security
  (`src/memdir/paths.ts:168`).
- The base project identity is canonical git root or project root, so worktrees
  share memory (`src/memdir/paths.ts:198`).
- Default auto-memory path is
  `<memoryBase>/projects/<sanitized-git-root>/memory/`
  (`src/memdir/paths.ts:208`).
- Auto-memory entrypoint is `MEMORY.md`
  (`src/memdir/paths.ts:253`).
- `MEMORY.md` is capped at 200 lines and 25 KB
  (`src/memdir/memdir.ts:34`).
- Auto-memory prompt explicitly excludes data derivable from current project
  state (`src/memdir/memdir.ts:187`).

### Agent And Session Memory

Agent memory has user, project, and local scopes:

- Agent memory scopes are `user`, `project`, and `local`
  (`src/tools/AgentTool/agentMemory.ts:12`).
- User agent memory is under `<memoryBase>/agent-memory/<agentType>/`
  (`src/tools/AgentTool/agentMemory.ts:47`).
- Project agent memory is `.claude/agent-memory/<agentType>/`
  (`src/tools/AgentTool/agentMemory.ts:47`).
- Local agent memory is remote project memory or
  `.claude/agent-memory-local/<agentType>/`
  (`src/tools/AgentTool/agentMemory.ts:24`).
- Agent memory entrypoint is `MEMORY.md`
  (`src/tools/AgentTool/agentMemory.ts:109`).

Session memory is per session and file-backed:

- Session memory runs through a background forked subagent
  (`src/services/SessionMemory/sessionMemory.ts:1`).
- It has feature-gated cached enablement/config checks
  (`src/services/SessionMemory/sessionMemory.ts:80`).
- Session memory setup creates a directory and summary file with mode `0700`
  and `0600`, then reads through `FileReadTool`
  (`src/services/SessionMemory/sessionMemory.ts:183`).
- Session memory registers a post-sampling hook
  (`src/services/SessionMemory/sessionMemory.ts:357`).
- The recognized session memory path is
  `{projectDir}/{sessionId}/session-memory/summary.md`
  (`src/utils/permissions/filesystem.ts:257`).

### Session History And Runtime State

Session history is stored in project-scoped transcript files:

- Project directories live under `~/.claude/projects`
  (`src/utils/sessionStorage.ts:198`).
- Current transcript path is `<projectDir>/<sessionId>.jsonl`
  (`src/utils/sessionStorage.ts:198`).
- Current session path can honor a separate `sessionProjectDir`; other session
  IDs fall back to original cwd (`src/utils/sessionStorage.ts:207`).
- Raw transcript reads are capped at 50 MB
  (`src/utils/sessionStorage.ts:227`).
- Subagent transcripts are stored under
  `<projectDir>/<sessionId>/subagents[/subdir]/agent-<id>.jsonl`
  (`src/utils/sessionStorage.ts:231`).
- Sidecar agent metadata is stored beside subagent transcript paths as
  `.meta.json` (`src/utils/sessionStorage.ts:260`).
- Remote-agent metadata is stored under
  `<projectDir>/<sessionId>/remote-agents/`
  (`src/utils/sessionStorage.ts:320`).
- Transcript writes use mode `0600` and parent dirs use `0700`
  (`src/utils/sessionStorage.ts:634`).
- Session persistence is skipped in tests, when cleanup period is 0, with
  `--no-session-persistence`, or when `CLAUDE_CODE_SKIP_PROMPT_HISTORY` is set
  (`src/utils/sessionStorage.ts:953`).

Cleanup lifecycle:

- Default cleanup retention is 30 days (`src/utils/cleanup.ts:23`).
- Cleanup period comes from `settings.cleanupPeriodDays`
  (`src/utils/cleanup.ts:25`).
- Cleanup covers messages, sessions, plans, file history, session-env, debug
  logs, image caches, paste caches, and stale worktrees
  (`src/utils/cleanup.ts:575`).
- Old sessions are cleaned across `~/.claude/projects`, including JSONL,
  asciinema casts, and tool results (`src/utils/cleanup.ts:155`).
- Debug logs under `~/.claude/debug` are cleaned while preserving the latest
  symlink (`src/utils/cleanup.ts:390`).

### Cache, Logs, Credentials, And Temporary Files

Runtime cache and logs:

- Cache paths use `env-paths('claude-cli')` and include project-scoped base,
  errors, messages, and MCP logs (`src/utils/cachePaths.ts:1`,
  `src/utils/cachePaths.ts:25`).
- XDG defaults are used for state/cache paths
  (`src/utils/xdg.ts:32`).
- Debug logs are written to `--debug-file`,
  `CLAUDE_CODE_DEBUG_LOGS_DIR`, or `~/.claude/debug/<sessionId>.txt`
  (`src/utils/debug.ts:230`).
- `latest` debug-log symlink maintenance is implemented separately
  (`src/utils/debug.ts:238`).
- Error logs are loaded from `CACHE_PATHS.errors`
  (`src/utils/log.ts:209`).
- MCP error/debug logs are routed through log sinks
  (`src/utils/log.ts:300`).
- API request logging can retain request parameters, but Ant internal builds
  keep message contents in memory only (`src/utils/log.ts:341`).

Tool and media outputs:

- Tool results are stored under
  `<projectDir>/<sessionId>/tool-results/<id>`
  (`src/utils/toolResultStorage.ts:94`).
- Tool-result writes use exclusive creation (`wx`)
  (`src/utils/toolResultStorage.ts:122`).
- Image cache uses `~/.claude/image-cache/<sessionId>`
  (`src/utils/imageStore.ts:18`).
- Image files are stored with mode `0600`
  (`src/utils/imageStore.ts:54`).
- Paste cache uses `~/.claude/paste-cache`
  (`src/utils/pasteStore.ts:13`).
- Paste cache files are named by hash and written with mode `0600`
  (`src/utils/pasteStore.ts:37`).

Session environment and temp dirs:

- Session env files live under `~/.claude/session-env/<sessionId>`
  (`src/utils/sessionEnvironment.ts:15`).
- Runtime loads `CLAUDE_ENV_FILE` plus hook env files in deterministic order
  (`src/utils/sessionEnvironment.ts:60`).
- Claude temp dirs default to `/tmp/claude-<uid>` on Unix or the OS temp dir on
  Windows, with `CLAUDE_CODE_TMPDIR` override
  (`src/utils/permissions/filesystem.ts:302`).
- Bundled skills are extracted under
  `/tmp/claude-<uid>/bundled-skills/<VERSION>/<nonce>`
  (`src/utils/permissions/filesystem.ts:365`).

Credentials and tokens:

- Remote CCR token files are
  `/home/claude/.claude/remote/.oauth_token`,
  `/home/claude/.claude/remote/.api_key`, and
  `/home/claude/.claude/remote/.session_ingress_token`
  (`src/utils/authFileDescriptor.ts:13`).
- Remote auth-file writes use dir mode `0700` and file mode `0600`
  (`src/utils/authFileDescriptor.ts:30`).
- Secure storage uses macOS Keychain with plaintext fallback on macOS, and
  plaintext storage on other platforms (`src/utils/secureStorage/index.ts:9`).
- Plaintext secure storage file is `~/.claude/.credentials.json`
  (`src/utils/secureStorage/plainTextStorage.ts:13`).
- Plaintext secure storage writes JSON with mode `0600`
  (`src/utils/secureStorage/plainTextStorage.ts:44`).
- Bare auth uses only `ANTHROPIC_API_KEY` or an `apiKeyHelper` from
  `--settings` (`src/utils/auth.ts:226`).
- Normal API-key resolution checks approved env, file descriptors, API key
  helper cache, config, and keychain (`src/utils/auth.ts:298`).
- OAuth tokens are saved to secure storage under `claudeAiOauth`, except
  inference-only environment tokens (`src/utils/auth.ts:1198`).
- OAuth read path checks env, file descriptors, then secure storage; bare mode
  skips OAuth (`src/utils/auth.ts:1255`).

## Data Boundary Matrix

| Boundary | Source/Path | Isolation Key | Load/Write Behavior | AVM Mapping |
| --- | --- | --- | --- | --- |
| Process entry | `src/entrypoints/cli.tsx` | process argv/env | Early special entrypoints, then `main.tsx` normal runtime | `runtime`: process bootstrap and mode dispatch |
| Runtime cwd | `src/bootstrap/state.ts` | cwd/originalCwd/projectRoot | `originalCwd` and `projectRoot` identify project state, not direct file permission | `runtime` + `state`: separate execution cwd from state identity |
| Settings | `~/.claude/settings.json`, `.claude/settings.json`, `.claude/settings.local.json`, managed path, flag file | setting source | User < project < local < flag < policy merge priority | `state`: scoped config model |
| Global config | `~/.claude.json` or `$CLAUDE_CONFIG_DIR/.claude.json` | global user | Stores project map, MCP servers, auth metadata, env, trusted dirs | `state`: user-global config boundary |
| Project config | global config `projects` map | canonical git root or original cwd | `getCurrentProjectConfig()` keys project state by canonical path | `state`: project identity resolver |
| Tool permissions | settings, CLI, command, session | permission source + tool pattern | Central `ToolPermissionContext` and rule evaluation | `adapter`: permission adapter contract |
| Filesystem reads | original cwd + additional dirs + internal paths | workspace/add-dir/project session | Sensitive path classifier plus deny/ask/allow rules | `runtime`: workspace filesystem policy |
| Sandbox | external sandbox runtime config | platform/deps/settings | Disabled by default; optional command wrapper and network/fs policies | `runtime`: optional isolation provider |
| Shell commands | subprocess per command | cwd + env + sandbox flag | Spawns shell with `subprocessEnv()` and runtime markers | `adapter`: shell execution contract |
| Skills | managed/user/project/add-dir/plugin/bundled/MCP | source root, realpath, plugin ID | Directory `SKILL.md`, nested discovery, conditional activation, MCP builders | `packageio`: skill package loader; `adapter`: prompt command surface |
| Plugins | `~/.claude/plugins` or override | plugin ID + marketplace + version + scope | Global installed metadata, versioned cache, per-plugin data | `packageio`: versioned package cache |
| MCP config | enterprise/local/project/user/plugin/dynamic/Claude.ai/managed | scope + server name + transport signature | Project approval, policy filter, merge precedence, dynamic override | `adapter`: MCP config resolver and launcher |
| MCP tokens | secure storage and needs-auth cache | server key hash | OAuth/XAA tokens in secure storage; needs-auth TTL cache | `state`: credential store boundary |
| User memory | `~/.claude/CLAUDE.md` | user | Loaded before project memory | `state`: user memory |
| Project memory | project `CLAUDE.md`, `.claude/CLAUDE.md`, `.claude/rules/*.md` | cwd traversal | Root-down traversal, closer higher priority, includes | `state`: project memory |
| Auto memory | `~/.claude/projects/<project>/memory/MEMORY.md` | canonical git root/project root | Worktrees share memory; project settings cannot select arbitrary path | `state`: long-term project memory |
| Agent memory | user/project/local agent memory dirs | agent type + scope | `MEMORY.md` per scope and agent type | `state`: agent-scoped memory |
| Session history | `~/.claude/projects/<project>/<sessionId>.jsonl` | project + session ID | JSONL transcripts, subagent paths, sidecars, remote-agent metadata | `state`: session store |
| Tool results | `~/.claude/projects/<project>/<sessionId>/tool-results` | project + session ID | Exclusive write files, special readable internal path | `state`: artifact store |
| Logs/cache | env-path cache, `~/.claude/debug`, `~/.claude/image-cache`, `~/.claude/paste-cache` | project/session/cache type | Retention cleanup, mode-restricted files | `state`: cache/log lifecycle |

## Evidence Table

| Claim | Evidence |
| --- | --- |
| Normal CLI enters through `cli.tsx`, then imports `main.tsx` | `src/entrypoints/cli.tsx:28`, `src/entrypoints/cli.tsx:287` |
| Special MCP/native-host entrypoints bypass normal runtime startup | `src/entrypoints/cli.tsx:72`, `src/entrypoints/cli.tsx:82`, `src/entrypoints/cli.tsx:87` |
| Daemon, worker, remote, bridge, and background session paths are separate early modes | `src/entrypoints/cli.tsx:95`, `src/entrypoints/cli.tsx:112`, `src/entrypoints/cli.tsx:164`, `src/entrypoints/cli.tsx:182` |
| Main runtime detects interactive/headless mode and loads settings early | `src/main.tsx:797`, `src/main.tsx:851` |
| `init()` applies safe environment before trust | `src/entrypoints/init.ts:57`, `src/utils/managedEnv.ts:93`, `src/utils/managedEnv.ts:124` |
| Full environment application happens only after trust | `src/utils/managedEnv.ts:180` |
| Settings merge order is user, project, local, flag, policy | `src/utils/settings/constants.ts:3` |
| Global config defaults to `~/.claude.json`; config home defaults to `~/.claude` | `src/utils/env.ts:13`, `src/utils/envUtils.ts:5` |
| Project identity is separate from file-operation boundary | `src/bootstrap/state.ts:45`, `src/bootstrap/state.ts:496` |
| `setup()` owns cwd/session/messaging/worktree initialization | `src/setup.ts:56`, `src/setup.ts:86`, `src/setup.ts:160`, `src/setup.ts:174` |
| Shell commands are subprocesses and can be sandbox-wrapped | `src/utils/Shell.ts:177`, `src/utils/Shell.ts:259`, `src/utils/Shell.ts:315` |
| Sandbox defaults do not force sandboxing | `src/utils/sandbox/sandbox-adapter.ts:459`, `src/utils/sandbox/sandbox-adapter.ts:532` |
| Filesystem permissions include workspace dirs, sensitive paths, and explicit rule checks | `src/utils/permissions/filesystem.ts:57`, `src/utils/permissions/filesystem.ts:667`, `src/utils/permissions/filesystem.ts:1030` |
| Project temp can be read across sessions in the same project | `src/utils/permissions/filesystem.ts:1688` |
| Tool permission context comes from CLI/settings/policy/session | `src/utils/permissions/permissionSetup.ts:721`, `src/utils/permissions/permissionsLoader.ts:120`, `src/utils/permissions/permissions.ts:109` |
| Skills load from managed/user/project/add-dir/plugin/bundled/MCP sources | `src/skills/loadSkillsDir.ts:67`, `src/skills/loadSkillsDir.ts:638`, `src/skills/loadSkillsDir.ts:677`, `src/skills/bundledSkills.ts:43`, `src/skills/loadSkillsDir.ts:1077` |
| Filesystem skills require `skills/<name>/SKILL.md` | `src/skills/loadSkillsDir.ts:403` |
| Plugin installs use global metadata plus versioned cache | `src/utils/plugins/installedPluginsManager.ts:76`, `src/utils/plugins/installedPluginsManager.ts:184` |
| Plugin skills are named `pluginName:skill` | `src/utils/plugins/loadPluginCommands.ts:687` |
| MCP scopes and transports are typed centrally | `src/services/mcp/types.ts:10`, `src/services/mcp/types.ts:23` |
| MCP project/user/local/enterprise paths and precedence are explicit | `src/services/mcp/config.ts:625`, `src/services/mcp/config.ts:888`, `src/services/mcp/config.ts:1033`, `src/services/mcp/config.ts:1071` |
| Dynamic MCP config can override file configs at runtime | `src/main.tsx:1413`, `src/main.tsx:2380` |
| Project MCP requires approval logic before use | `src/services/mcp/utils.ts:351` |
| Stdio MCP launches configured external command | `src/services/mcp/client.ts:944` |
| MCP roots expose only original cwd | `src/services/mcp/client.ts:1009` |
| CLAUDE.md memory load order and include behavior are implemented in `claudemd.ts` | `src/utils/claudemd.ts:1`, `src/utils/claudemd.ts:618`, `src/utils/claudemd.ts:790`, `src/utils/claudemd.ts:849` |
| Auto-memory is project-root based and worktree-shared | `src/memdir/paths.ts:198`, `src/memdir/paths.ts:208`, `src/memdir/paths.ts:253` |
| Agent memory has user/project/local scopes | `src/tools/AgentTool/agentMemory.ts:12`, `src/tools/AgentTool/agentMemory.ts:47` |
| Session transcript paths are project/session scoped | `src/utils/sessionStorage.ts:198`, `src/utils/sessionStorage.ts:231`, `src/utils/sessionStorage.ts:634` |
| Cleanup retention defaults to 30 days and covers major state classes | `src/utils/cleanup.ts:23`, `src/utils/cleanup.ts:575` |
| Non-macOS secure storage is plaintext under `~/.claude/.credentials.json` | `src/utils/secureStorage/index.ts:9`, `src/utils/secureStorage/plainTextStorage.ts:13` |

## Risks For AVM PRD

- A PRD assumption that Claude Code state is contained only in `~/.claude` is
  incomplete. Global config defaults to `~/.claude.json`, plugin/cache paths can
  be overridden, and cache/log state also uses env-path directories
  (`src/utils/env.ts:13`, `src/utils/plugins/pluginDirectories.ts:49`,
  `src/utils/cachePaths.ts:1`).
- A PRD assumption that cwd is the only project boundary is incomplete.
  Claude Code separately tracks cwd, original cwd, project root, session project
  dir, canonical git root, and additional dirs (`src/bootstrap/state.ts:45`,
  `src/bootstrap/state.ts:402`, `src/memdir/paths.ts:198`).
- A PRD assumption that sandboxing is always active is false for this source.
  Sandbox defaults set `enabled: false`, and `allowUnsandboxedCommands` defaults
  true (`src/utils/sandbox/sandbox-adapter.ts:459`).
- A PRD assumption that sandboxing alone defines isolation is incomplete.
  Without sandbox enablement, filesystem and shell boundaries rely on permission
  rules, path classifiers, trust prompts, and tool-specific validators
  (`src/utils/permissions/filesystem.ts:1030`,
  `src/utils/permissions/permissions.ts:473`).
- A PRD assumption that project state is fully isolated by session is false for
  some internal paths. Project temp directories are readable across sessions in
  the same project (`src/utils/permissions/filesystem.ts:1688`).
- A PRD assumption that worktrees have separate long-term project memory is
  false by default. Auto-memory keys by canonical git root or project root, so
  worktrees share memory (`src/memdir/paths.ts:198`).
- A PRD assumption that skills are static packages is incomplete. Skills can be
  discovered from nested project paths, loaded conditionally, supplied by
  plugins, bundled into the CLI, or exposed by MCP (`src/skills/loadSkillsDir.ts:861`,
  `src/skills/loadSkillsDir.ts:997`, `src/skills/loadSkillsDir.ts:1077`).
- A PRD assumption that plugin install scope equals storage scope is incomplete.
  Installation metadata is global, versioned cache is global, and project/local
  relevance is filtered by project path (`src/utils/plugins/installedPluginsManager.ts:1`,
  `src/utils/plugins/installedPluginsManager.ts:785`).
- A PRD assumption that MCP config has a simple global/project precedence is
  false. Enterprise exclusivity, local/project/user/plugin precedence, project
  approval, policy filters, strict mode, bare mode, and dynamic overrides all
  affect the final runtime MCP set (`src/services/mcp/config.ts:1071`,
  `src/main.tsx:1799`, `src/main.tsx:2380`).
- A PRD assumption that remote MCP authorization is entirely external is
  incomplete. Claude Code stores OAuth/XAA tokens through its secure storage and
  maintains a local needs-auth cache (`src/services/mcp/auth.ts:349`,
  `src/services/mcp/client.ts:257`).
- A PRD assumption that credential storage is always OS-secure is false on
  non-macOS platforms in this source. Secure storage falls back to plaintext
  `~/.claude/.credentials.json` (`src/utils/secureStorage/index.ts:9`,
  `src/utils/secureStorage/plainTextStorage.ts:13`).

## Open Questions

- The concrete OS-level guarantees of `@anthropic-ai/sandbox-runtime` are not
  visible in this repository snapshot. This research can verify when Claude
  Code calls the sandbox adapter, but not the external sandbox implementation.
- Build-time feature flags affect TeamMem, Buddy, Kairos, bundled skills, and
  several remote behaviors. The exact production feature set is not derivable
  from static source alone.
- The snapshot lacks `package.json`, lockfiles, Makefile, or a built executable,
  so CLI `--help`, build, and test validation could not be run locally.
- Claude.ai connector config is fetched asynchronously in runtime startup, but
  server-side connector selection and policy are outside this source snapshot.
- Some comments describe Ant-internal behavior and sandbox requirements. Those
  paths may not apply to public Claude Code builds without confirming build
  macros and distribution settings.
