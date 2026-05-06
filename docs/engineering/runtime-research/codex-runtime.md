# Codex Runtime Source Research

This note researches the local Codex runtime implementation under
`frameworks/codex`, with emphasis on source facts that validate Agent VM PRD
premises. It intentionally avoids product recommendations.

Line references are from the local checkout at research time.

## Summary

- Codex runtime is primarily a Rust workspace under `frameworks/codex/codex-rs`.
  The top-level `frameworks/codex/package.json` is a private Node monorepo with
  formatting/schema scripts, not the runtime entrypoint
  (`frameworks/codex/package.json:1-35`). The executable entry is the Rust
  `codex` CLI in `codex-rs/cli`, whose `main()` dispatches TUI, exec,
  MCP-server, app-server, sandbox debug, plugin, login, and cloud commands
  (`frameworks/codex/codex-rs/cli/src/main.rs:69-83`,
  `frameworks/codex/codex-rs/cli/src/main.rs:101-172`,
  `frameworks/codex/codex-rs/cli/src/main.rs:704-832`).
- Runtime startup is layered: CLI flags become `ConfigOverrides`, the config
  loader merges requirement, global, user, project, and runtime layers, then
  session startup creates or resumes a thread, loads auth, MCP, plugin, skill,
  AGENTS.md, history, and state DB services
  (`frameworks/codex/codex-rs/core/src/config_loader/mod.rs:99-124`,
  `frameworks/codex/codex-rs/core/src/config/mod.rs:690-827`,
  `frameworks/codex/codex-rs/core/src/session/session.rs:300-415`,
  `frameworks/codex/codex-rs/core/src/session/mod.rs:450-575`).
- Isolation is expressed as separate approval, sandbox, file-system sandbox,
  network, and writable-root models. Enforcement is distributed across config
  derivation, approval orchestration, shell runtime, platform sandbox command
  transforms, and helper binaries
  (`frameworks/codex/codex-rs/protocol/src/protocol.rs:939-1081`,
  `frameworks/codex/codex-rs/core/src/tools/sandboxing.rs:198-277`,
  `frameworks/codex/codex-rs/sandboxing/src/manager.rs:142-272`).
- Skills, MCP servers, and plugins do not use one unified registry. Skills are
  discovered from configured local roots, repo roots, user/admin roots, bundled
  system skill cache, and plugin skill roots. MCP servers are config entries
  plus plugin MCP configs plus feature-gated skill-dependency installs.
  Plugins have their own marketplace/cache/manifest store
  (`frameworks/codex/codex-rs/core-skills/src/loader.rs:221-357`,
  `frameworks/codex/codex-rs/core/src/config/mod.rs:848-873`,
  `frameworks/codex/codex-rs/core-plugins/src/store.rs:29-54`,
  `frameworks/codex/codex-rs/core/src/mcp_skill_dependencies.rs:75-126`).
- Runtime state is split across `CODEX_HOME` files, SQLite DBs, JSONL rollout
  files, logs, caches, keyring/file auth stores, and in-memory session stores.
  Default `CODEX_HOME` is `~/.codex` unless `CODEX_HOME` is set
  (`frameworks/codex/codex-rs/core/src/config/mod.rs:2664-2674`).
- Safe verification was attempted with `cargo metadata --manifest-path
  frameworks/codex/codex-rs/Cargo.toml --no-deps --format-version 1` and
  `cargo run --manifest-path frameworks/codex/codex-rs/Cargo.toml -p codex-cli
  -- --help`, but this environment has no `cargo` in PATH. No source files were
  modified by those commands.

## Runtime Startup Path

### Executable and language layering

- The top-level Node package is a private monorepo with `pnpm` and Node >= 22
  requirements plus formatting/schema scripts (`frameworks/codex/package.json:1-35`).
  Source tracing shows runtime entrypoints in Rust, not Node.
- The Rust CLI root is `TopCli`; Clap defines global flags and subcommands in
  `frameworks/codex/codex-rs/cli/src/main.rs:69-172`.
- `main()` calls `arg0_dispatch_or_else` and then `cli_main`
  (`frameworks/codex/codex-rs/cli/src/main.rs:704-709`). `cli_main` parses root
  flags and subcommands (`frameworks/codex/codex-rs/cli/src/main.rs:711-726`).
- With no subcommand, Codex starts the interactive TUI via
  `run_interactive_tui(...)` (`frameworks/codex/codex-rs/cli/src/main.rs:727-740`).
- `codex exec` merges root flags into the exec CLI and calls
  `codex_exec::run_main` (`frameworks/codex/codex-rs/cli/src/main.rs:741-755`).
- `codex mcp-server`, `codex mcp`, plugin marketplace commands, app-server, and
  debug sandbox commands are dispatched from the same CLI root
  (`frameworks/codex/codex-rs/cli/src/main.rs:770-832`,
  `frameworks/codex/codex-rs/cli/src/main.rs:997-1045`).
- The interactive TUI path rejects unsupported terminal conditions before it
  calls `codex_tui::run_main` (`frameworks/codex/codex-rs/cli/src/main.rs:1490-1542`).
- Standalone `codex-tui`, `codex-exec`, and `codex-app-server` also exist as
  Rust binary entrypoints (`frameworks/codex/codex-rs/tui/src/main.rs:48-63`,
  `frameworks/codex/codex-rs/exec/src/main.rs:28-40`,
  `frameworks/codex/codex-rs/app-server/src/main.rs:41-66`).

### Headless exec startup

- `codex_exec::run_main` receives image/model/profile/sandbox/cwd/add-dir and
  other CLI options (`frameworks/codex/codex-rs/exec/src/lib.rs:218-249`).
- The `--full-auto` and bypass flags map to `WorkspaceWrite` and
  `DangerFullAccess` sandbox modes; otherwise the CLI sandbox value is used
  (`frameworks/codex/codex-rs/exec/src/lib.rs:272-278`).
- Exec resolves cwd and `CODEX_HOME` before building config
  (`frameworks/codex/codex-rs/exec/src/lib.rs:290-306`).
- It preloads config TOML, builds `ConfigOverrides` including cwd, model,
  approval, sandbox, permission profile, sandbox helper paths, ephemeral flag,
  and additional writable roots, then calls `ConfigBuilder::build`
  (`frameworks/codex/codex-rs/exec/src/lib.rs:315-339`,
  `frameworks/codex/codex-rs/exec/src/lib.rs:386-420`).
- Exec then creates `InProcessClientStartArgs` with the built config,
  environment manager, loader overrides, session source `Exec`, and
  `client_name = "codex_exec"` (`frameworks/codex/codex-rs/exec/src/lib.rs:493-512`).
- It starts or resumes an in-process app-server thread, then starts a turn with
  cwd, approval policy, sandbox policy or permission profile, effort, and output
  schema (`frameworks/codex/codex-rs/exec/src/lib.rs:651-768`).
- Exec maps core `SandboxPolicy` to app-server protocol sandbox fields and
  emits configured-session fields including session id, model/provider,
  approval, sandbox, permission profile, cwd, and rollout path
  (`frameworks/codex/codex-rs/exec/src/lib.rs:913-980`,
  `frameworks/codex/codex-rs/exec/src/lib.rs:1064-1099`).

### Config and workspace loading

- Config layering order is explicit: requirement layers, system/admin
  `/etc/codex/config.toml`, user `${CODEX_HOME}/config.toml`, cwd config,
  project tree `.codex/config.toml`, repo-root `.codex/config.toml`, then runtime
  CLI/session flags (`frameworks/codex/codex-rs/core/src/config_loader/mod.rs:99-124`,
  `frameworks/codex/codex-rs/core/src/config_loader/mod.rs:211-319`).
- Project config layers are discovered from cwd ancestors to project root,
  trust-gated, and skipped when `.codex` equals `CODEX_HOME`
  (`frameworks/codex/codex-rs/core/src/config_loader/mod.rs:930-1024`).
- `ConfigBuilder` resolves `CODEX_HOME`, cwd, loads the layer stack,
  deserializes merged TOML, and builds the final `Config`
  (`frameworks/codex/codex-rs/core/src/config/mod.rs:690-827`).
- The final config includes cwd, auth mode, MCP servers, OAuth store mode,
  AGENTS settings, memories config, codex/sqlite/log homes, history settings,
  and helper executable paths (`frameworks/codex/codex-rs/core/src/config/mod.rs:400-421`,
  `frameworks/codex/codex-rs/core/src/config/mod.rs:438-499`).
- `CODEX_HOME` defaults to `~/.codex`; if `CODEX_HOME` is set, the path must
  canonicalize (`frameworks/codex/codex-rs/core/src/config/mod.rs:2664-2674`).
- Model provider and model settings are config fields, merged with built-in
  providers, profiles, and optional model catalog JSON
  (`frameworks/codex/codex-rs/core/src/config/mod.rs:257-278`,
  `frameworks/codex/codex-rs/core/src/config/mod.rs:1988-2002`,
  `frameworks/codex/codex-rs/core/src/config/mod.rs:2111-2217`).

### Session and turn context loading

- Session startup prepares thread persistence unless ephemeral, disables state
  DB for ephemeral sessions, reads history metadata for root sessions, and
  loads auth plus effective MCP servers (`frameworks/codex/codex-rs/core/src/session/session.rs:300-415`).
- `Session::new` loads plugin outcomes and skills before AGENTS instructions,
  then loads user/project AGENTS.md instructions via `AgentsMdManager`
  (`frameworks/codex/codex-rs/core/src/session/mod.rs:450-503`).
- Session startup computes exec policy, model default/base instructions, and
  restores dynamic tools from DB or rollout on resume/fork
  (`frameworks/codex/codex-rs/core/src/session/mod.rs:510-575`).
- `SessionConfiguration` carries provider/model/reasoning/instructions,
  approval, sandbox, cwd, codex_home, dynamic tools, and persistence state
  (`frameworks/codex/codex-rs/core/src/session/mod.rs:595-624`).
- The session emits `SessionConfiguredEvent` before MCP startup events, then
  starts skill watching and initializes the MCP manager
  (`frameworks/codex/codex-rs/core/src/session/session.rs:717-890`).
- Memory startup is launched after session configuration
  (`frameworks/codex/codex-rs/core/src/session/session.rs:930-934`).
- Each turn resolves an environment/cwd, builds per-turn config, updates MCP
  approval/sandbox policies, loads plugins/skill roots/skills, and creates
  `TurnContext` (`frameworks/codex/codex-rs/core/src/session/turn_context.rs:615-690`).

## Isolation Model

### Policy types

- Approval modes are protocol types: `UnlessTrusted`, `OnFailure`,
  `OnRequest`, `Granular`, and `Never`
  (`frameworks/codex/codex-rs/protocol/src/protocol.rs:939-970`).
- Granular approval config separately controls sandbox approval, rules,
  skill approval, permission requests, and MCP elicitations
  (`frameworks/codex/codex-rs/protocol/src/protocol.rs:972-1009`).
- Network access is a first-class enum, `Restricted` or `Enabled`
  (`frameworks/codex/codex-rs/protocol/src/protocol.rs:1011-1027`).
- Sandbox policy is one of `DangerFullAccess`, `ReadOnly`, `ExternalSandbox`,
  or `WorkspaceWrite`; workspace-write carries writable roots, network access,
  and tmp exclusion flags (`frameworks/codex/codex-rs/protocol/src/protocol.rs:1029-1081`).
- `WritableRoot` supports read-only subpaths and path-level writability checks
  (`frameworks/codex/codex-rs/protocol/src/protocol.rs:1083-1111`).
- Workspace-write permits writable roots, cwd, `/tmp` unless excluded, and
  `TMPDIR` unless excluded; read-only subpaths under writable roots default to
  `.git`, `.agents`, and `.codex`
  (`frameworks/codex/codex-rs/protocol/src/protocol.rs:1180-1316`).
- The model-facing workspace-write prompt states the same high-level contract:
  read all files, edit cwd and writable roots, require approval elsewhere, and
  apply network constraints
  (`frameworks/codex/codex-rs/core/src/context/prompts/permissions/sandbox_mode/workspace_write.md:1`).

### Config derivation and writable roots

- `Permissions` owns approval policy, sandbox policy, file-system sandbox
  policy, network sandbox policy, network proxy, login-shell allowance, shell
  environment policy, and Windows sandbox level
  (`frameworks/codex/codex-rs/core/src/config/mod.rs:189-231`).
- Additional writable roots are resolved relative to cwd
  (`frameworks/codex/codex-rs/core/src/config/mod.rs:1737-1759`).
- The memory root is created and always appended to additional writable roots
  when absent (`frameworks/codex/codex-rs/core/src/config/mod.rs:1794-1801`).
- Sandbox, file-system, and network policies are computed from permission
  profiles, named profiles, or legacy sandbox mode, then enriched with
  additional writable roots (`frameworks/codex/codex-rs/core/src/config/mod.rs:1808-1932`).
- Approval defaults depend on project trust: trusted defaults to `OnRequest`,
  untrusted defaults to `UnlessTrusted`, unless explicitly configured
  (`frameworks/codex/codex-rs/core/src/config/mod.rs:1933-1972`).
- Helper-readable roots are added to file-system sandbox policy for
  `CODEX_HOME`, `zsh`, and execve wrapper support
  (`frameworks/codex/codex-rs/core/src/config/mod.rs:2312-2340`).

### Approval lifecycle

- Approval cache is an in-memory `ApprovalStore`, keyed by serialized requests;
  it is cached per session and is not persisted by this module
  (`frameworks/codex/codex-rs/core/src/tools/sandboxing.rs:40-63`).
- `with_cached_approval` skips prompting only when all keys were approved for
  the current session (`frameworks/codex/codex-rs/core/src/tools/sandboxing.rs:71-117`).
- Exec approval outcomes are `Skip { bypass_sandbox }`, `NeedsApproval`, or
  `Forbidden` (`frameworks/codex/codex-rs/core/src/tools/sandboxing.rs:159-180`).
- Policy checks decide whether a command can run without approval, needs user
  approval, or is forbidden. `Never` and deprecated `OnFailure` do not ask;
  `OnRequest`/`Granular` ask for restricted file-system access; `UnlessTrusted`
  asks by default (`frameworks/codex/codex-rs/core/src/tools/sandboxing.rs:198-239`).
- Escalated sandbox permissions can request bypassing the sandbox, and managed
  network is disabled for escalated permissions
  (`frameworks/codex/codex-rs/core/src/tools/sandboxing.rs:247-277`).

### Shell execution boundary

- Shell handling merges environment, applies granted permissions, intercepts
  `apply_patch`, computes approval requirements, and builds a `ShellRequest`
  (`frameworks/codex/codex-rs/core/src/tools/handlers/shell.rs:420-548`).
- `ShellRequest` carries command, cwd, sandbox preference, sandbox/permission
  requests, environment, and timeout (`frameworks/codex/codex-rs/core/src/tools/runtimes/shell.rs:48-63`).
- The shell runtime builds an approval key from canonical command, cwd,
  sandbox permissions, and additional permissions, then requests approval
  through the orchestrator (`frameworks/codex/codex-rs/core/src/tools/runtimes/shell.rs:134-215`).
- Before spawning, the shell runtime applies network/env changes, wraps the
  command, transforms it into a platform sandbox command, and calls `execute_env`
  (`frameworks/codex/codex-rs/core/src/tools/runtimes/shell.rs:242-289`).
- The low-level process spawn function clears the environment, sets only the
  selected env vars, sets cwd, and pipes or inherits stdio. It assumes callers
  already transformed the command for sandboxing
  (`frameworks/codex/codex-rs/core/src/spawn.rs:51-125`,
  `frameworks/codex/codex-rs/core/src/exec.rs:840-899`).

### Platform sandboxing and network

- Platform sandbox types are `None`, macOS Seatbelt, Linux seccomp, and Windows
  restricted token (`frameworks/codex/codex-rs/sandboxing/src/manager.rs:23-63`).
- `SandboxManager::select_initial` chooses whether a platform sandbox is
  required, and `transform` adds platform-specific wrappers and policy
  arguments (`frameworks/codex/codex-rs/sandboxing/src/manager.rs:142-272`).
- Additional permissions can widen file-system or network policies before
  execution (`frameworks/codex/codex-rs/sandboxing/src/policy_transforms.rs:516-619`).
- Platform sandbox is required when managed network or restricted network is in
  effect unless the policy is external; with enabled network, restricted
  file-system still requires platform sandbox
  (`frameworks/codex/codex-rs/sandboxing/src/policy_transforms.rs:631-651`).
- The Linux helper applies `no_new_privs`, seccomp, and bubblewrap for
  file-system view setup (`frameworks/codex/codex-rs/linux-sandbox/src/lib.rs:1-5`).
- Linux sandbox CLI arguments include command cwd, sandbox policy, file-system
  policy, network policy, seccomp inner mode, proxy settings, and command
  (`frameworks/codex/codex-rs/linux-sandbox/src/linux_run_main.rs:30-91`).
- Linux setup runs bubblewrap for file-system isolation, then
  no_new_privs/seccomp, then `execvp`
  (`frameworks/codex/codex-rs/linux-sandbox/src/linux_run_main.rs:94-221`).
- Restricted network is also surfaced through environment variables such as
  `CODEX_SANDBOX_NETWORK_DISABLED`
  (`frameworks/codex/codex-rs/core/src/sandboxing/mod.rs:100-150`,
  `frameworks/codex/codex-rs/core/src/spawn.rs:12-25`).

## Skill Installation And Loading

- Bundled system skills are embedded in the Rust `codex-skills` crate and are
  installed under `CODEX_HOME/skills/.system`
  (`frameworks/codex/codex-rs/skills/src/lib.rs:10-22`).
- System skill installation creates `CODEX_HOME/skills`, writes a marker
  fingerprint, removes the old `.system` directory when needed, and writes the
  embedded directory tree (`frameworks/codex/codex-rs/skills/src/lib.rs:24-55`,
  `frameworks/codex/codex-rs/skills/src/lib.rs:101-127`).
- `SkillsManager` installs or uninstalls bundled skills based on config, caches
  loaded results by cwd/config, filters roots when bundled skills are disabled,
  and keeps separate caches to prevent rule leakage across roots
  (`frameworks/codex/codex-rs/core-skills/src/manager.rs:50-126`).
- Bundled skills are enabled by default unless `[skills].bundled.enabled=false`
  (`frameworks/codex/codex-rs/core-skills/src/manager.rs:246-266`).
- A skill is primarily a directory containing `SKILL.md`; optional metadata is
  read from `.agents/agents/openai.yaml` style paths under the skill directory
  (`frameworks/codex/codex-rs/core-skills/src/loader.rs:37-123`).
- Skill roots are discovered from project `.codex/skills`, deprecated
  `CODEX_HOME/skills`, user `$HOME/.agents/skills`, system cache, admin
  `/etc/codex/skills`, repo `.agents/skills`, configured extra roots, and
  plugin roots (`frameworks/codex/codex-rs/core-skills/src/loader.rs:221-357`).
- The loader scans roots with depth and count caps, parses `SKILL.md`, warns on
  truncation, validates metadata, and sorts/deduplicates by scope rank
  (`frameworks/codex/codex-rs/core-skills/src/loader.rs:157-219`,
  `frameworks/codex/codex-rs/core-skills/src/loader.rs:440-642`).
- Skill names are namespaced by nearest plugin manifest, so plugin-installed
  skills can be distinguished from plain local skills
  (`frameworks/codex/codex-rs/core-skills/src/loader.rs:657-665`).
- Available skills are rendered into developer instructions, with progressive
  disclosure telling the model to open `SKILL.md` only when needed
  (`frameworks/codex/codex-rs/core-skills/src/render.rs:21-83`).
- Explicit skill mentions cause full `SKILL.md` contents to be injected into the
  user context for the turn (`frameworks/codex/codex-rs/core-skills/src/injection.rs:31-85`,
  `frameworks/codex/codex-rs/core-skills/src/injection.rs:103-170`,
  `frameworks/codex/codex-rs/core/src/context/skill_instructions.rs:22-32`).
- Per-turn processing resolves explicit skill mentions, dependency prompts, and
  skill injections before recording skill/plugin items into conversation history
  (`frameworks/codex/codex-rs/core/src/session/turn.rs:167-271`,
  `frameworks/codex/codex-rs/core/src/session/turn.rs:349-355`).

## MCP Installation And Loading

- MCP server config supports stdio and streamable HTTP transports, enabled and
  required flags, startup/tool timeouts, approval mode, allow/deny tool lists,
  OAuth scopes/resource, per-tool config, cwd, env, headers, and bearer-token
  env/header mapping (`frameworks/codex/codex-rs/config/src/mcp_types.rs:117-241`).
- Raw MCP config is normalized into `Stdio` or `StreamableHttp` variants with
  validation of incompatible fields and default `enabled = true`
  (`frameworks/codex/codex-rs/config/src/mcp_types.rs:275-391`).
- `codex mcp add` supports one stdio command or one `--url`, plus env and
  bearer token env var fields (`frameworks/codex/codex-rs/cli/src/mcp_cmd.rs:36-135`).
- `codex mcp add` reads global server config from `CODEX_HOME` and persists
  replacements to the global config via `ConfigEditsBuilder.replace_mcp_servers`
  (`frameworks/codex/codex-rs/cli/src/mcp_cmd.rs:255-324`).
- Runtime MCP config merges config-defined servers with plugin MCP servers and
  carries auth, OAuth store mode, skill dependency flag, approval, sandbox,
  Linux sandbox helper, app summaries, and plugin summaries
  (`frameworks/codex/codex-rs/core/src/config/mod.rs:848-873`).
- Session startup computes effective MCP servers and auth statuses before
  constructing `McpConnectionManager`
  (`frameworks/codex/codex-rs/core/src/session/session.rs:383-397`,
  `frameworks/codex/codex-rs/core/src/session/session.rs:853-872`).
- Session MCP refresh can recompute config/plugins/provenance and replace the
  manager (`frameworks/codex/codex-rs/core/src/session/mcp.rs:205-303`).
- `McpConnectionManager` owns one `RmcpClient` per configured server and
  aggregates tools under fully qualified names
  (`frameworks/codex/codex-rs/codex-mcp/src/mcp_connection_manager.rs:1-7`,
  `frameworks/codex/codex-rs/codex-mcp/src/mcp_connection_manager.rs:887-899`).
- MCP startup creates managed clients, starts enabled servers, emits startup
  events, stores origins, initializes protocol version `2025-06-18`, lists
  tools, and caches Codex Apps tools under `CODEX_HOME/cache/codex_apps_tools`
  (`frameworks/codex/codex-rs/codex-mcp/src/mcp_connection_manager.rs:507-588`,
  `frameworks/codex/codex-rs/codex-mcp/src/mcp_connection_manager.rs:666-838`,
  `frameworks/codex/codex-rs/codex-mcp/src/mcp_connection_manager.rs:1424-1512`,
  `frameworks/codex/codex-rs/codex-mcp/src/mcp_connection_manager.rs:1617-1699`).
- MCP calls route through the session-owned manager and then to the selected
  server/client/tool (`frameworks/codex/codex-rs/core/src/tools/handlers/mcp.rs:56-100`,
  `frameworks/codex/codex-rs/core/src/session/mcp.rs:122-190`,
  `frameworks/codex/codex-rs/codex-mcp/src/mcp_connection_manager.rs:1088-1124`).
- Stdio MCP servers can be launched locally as child processes or remotely
  through an executor process API (`frameworks/codex/codex-rs/rmcp-client/src/stdio_server_launcher.rs:1-12`,
  `frameworks/codex/codex-rs/rmcp-client/src/stdio_server_launcher.rs:60-72`,
  `frameworks/codex/codex-rs/rmcp-client/src/stdio_server_launcher.rs:159-265`,
  `frameworks/codex/codex-rs/rmcp-client/src/stdio_server_launcher.rs:315-460`).
- HTTP MCP clients use bearer tokens or OAuth; OAuth tokens load/persist through
  keyring/file modes (`frameworks/codex/codex-rs/rmcp-client/src/rmcp_client.rs:658-760`,
  `frameworks/codex/codex-rs/rmcp-client/src/oauth.rs:79-172`).
- Skill-declared MCP dependencies are feature-gated, first-party only, detected
  from mentioned skills, optionally prompt the user, and then install missing
  server configs into global MCP config
  (`frameworks/codex/codex-rs/core/src/mcp_skill_dependencies.rs:32-73`,
  `frameworks/codex/codex-rs/core/src/mcp_skill_dependencies.rs:75-126`,
  `frameworks/codex/codex-rs/core/src/mcp_skill_dependencies.rs:216-288`,
  `frameworks/codex/codex-rs/core/src/mcp_skill_dependencies.rs:415-470`).

### Plugins, connectors, and registry boundaries

- Plugins are stored under `CODEX_HOME/plugins/cache/{marketplace}/{plugin}/{version}`
  (`frameworks/codex/codex-rs/core-plugins/src/store.rs:14-54`).
- Plugin install validates source and manifest, then atomically replaces the
  cached plugin directory (`frameworks/codex/codex-rs/core-plugins/src/store.rs:88-130`,
  `frameworks/codex/codex-rs/core-plugins/src/store.rs:246-310`).
- Plugin manifests are `.codex-plugin/plugin.json` files with name, version,
  description, skills, MCP servers, apps, and interface fields
  (`frameworks/codex/codex-rs/core-plugins/src/manifest.rs:11-64`,
  `frameworks/codex/codex-rs/core-plugins/src/manifest.rs:117-224`).
- Marketplace manifests are discovered at `.agents/plugins/marketplace.json`
  or `.claude-plugin/marketplace.json`
  (`frameworks/codex/codex-rs/core-plugins/src/marketplace.rs:20-23`,
  `frameworks/codex/codex-rs/core-plugins/src/marketplace.rs:220-247`).
- Installed marketplace roots can come from user `[marketplaces]` config or the
  default `CODEX_HOME/.tmp/marketplaces`
  (`frameworks/codex/codex-rs/core-plugins/src/installed_marketplaces.rs:11-60`).
- Configured plugins are loaded only from the user config layer; active plugins
  contribute skill roots, MCP servers, and apps
  (`frameworks/codex/codex-rs/core-plugins/src/loader.rs:103-140`,
  `frameworks/codex/codex-rs/core-plugins/src/loader.rs:333-359`,
  `frameworks/codex/codex-rs/core-plugins/src/loader.rs:451-548`).
- Plugin skills are loaded through the same skill loader, plugin MCP config is
  read from manifest override or default `.mcp.json`, and plugin apps default
  to `.app.json` (`frameworks/codex/codex-rs/core-plugins/src/loader.rs:568-641`,
  `frameworks/codex/codex-rs/core-plugins/src/loader.rs:643-714`,
  `frameworks/codex/codex-rs/core-plugins/src/loader.rs:755-880`).
- Effective plugin skill roots and MCP servers are separate collections in the
  plugin load outcome (`frameworks/codex/codex-rs/plugin/src/load_outcome.rs:11-25`,
  `frameworks/codex/codex-rs/plugin/src/load_outcome.rs:104-140`).

## Memory And State Storage

### Global, project, and user memory/config

- `CODEX_HOME` defaults to `~/.codex` and is the anchor for user config, auth,
  history, skills, plugins, memory artifacts, logs, caches, and SQLite state
  (`frameworks/codex/codex-rs/core/src/config/mod.rs:2664-2674`).
- User config lives at `${CODEX_HOME}/config.toml`; project config can live in
  cwd/repo `.codex/config.toml` layers depending on trust
  (`frameworks/codex/codex-rs/core/src/config_loader/mod.rs:211-319`,
  `frameworks/codex/codex-rs/core/src/config_loader/mod.rs:930-1024`).
- AGENTS instructions are discovered globally from `CODEX_HOME` and project
  roots from project root to cwd; override filenames are checked before
  fallback filenames (`frameworks/codex/codex-rs/core/src/agents_md.rs:1-17`,
  `frameworks/codex/codex-rs/core/src/agents_md.rs:36-78`,
  `frameworks/codex/codex-rs/core/src/agents_md.rs:213-320`).
- AGENTS/user instructions are rendered as user-context fragments
  (`frameworks/codex/codex-rs/core/src/context/user_instructions.rs:9-16`).

### History, rollout, and compaction

- Prompt history is an append-only JSONL file named `history.jsonl` under
  `CODEX_HOME`; persistence can be disabled, file mode is owner-only on Unix,
  and size limits are enforced by trimming older lines
  (`frameworks/codex/codex-rs/core/src/message_history.rs:1-17`,
  `frameworks/codex/codex-rs/core/src/message_history.rs:47-90`,
  `frameworks/codex/codex-rs/core/src/message_history.rs:95-160`,
  `frameworks/codex/codex-rs/core/src/message_history.rs:165-292`).
- Thread rollout files are stored under
  `CODEX_HOME/sessions/YYYY/MM/DD/rollout-YYYY-MM-DDThh-mm-ss-<thread-id>.jsonl`;
  creation precomputes the path and defers file creation until persistence
  (`frameworks/codex/codex-rs/rollout/src/lib.rs:21-22`,
  `frameworks/codex/codex-rs/rollout/src/recorder.rs:629-693`,
  `frameworks/codex/codex-rs/rollout/src/recorder.rs:1320-1350`).
- Rollout writer owns a background async writer, buffers pending items, retries
  failed writes, writes session meta when materialized, and flushes on persist,
  flush, or shutdown (`frameworks/codex/codex-rs/rollout/src/recorder.rs:714-760`,
  `frameworks/codex/codex-rs/rollout/src/recorder.rs:1366-1535`).
- `session_index.jsonl` is append-only under `CODEX_HOME` for thread names;
  the newest entry wins and name lookup resolves to a readable rollout
  (`frameworks/codex/codex-rs/rollout/src/session_index.rs:17-64`,
  `frameworks/codex/codex-rs/rollout/src/session_index.rs:115-154`).
- Compaction is stored as a rollout item with message and optional replacement
  history (`frameworks/codex/codex-rs/protocol/src/protocol.rs:2844-2860`).
- Local compaction rewrites live history, persists `RolloutItem::Compacted`,
  optionally persists a following `TurnContext`, and advances the model window
  generation (`frameworks/codex/codex-rs/core/src/session/mod.rs:2434-2450`).
- Manual/auto compact tasks route through `CompactTask`, which chooses local or
  remote compaction (`frameworks/codex/codex-rs/core/src/tasks/compact.rs:10-46`).

### SQLite state and logs

- `StateRuntime::init` creates `CODEX_HOME`, migrates a state SQLite database
  and a separate logs SQLite database, and runs startup log maintenance
  (`frameworks/codex/codex-rs/state/src/runtime.rs:95-156`).
- State and logs DB filenames are versioned as `<base>_<version>.sqlite` and
  both are joined directly under `CODEX_HOME`
  (`frameworks/codex/codex-rs/state/src/runtime.rs:210-228`).
- The `threads` table stores id, rollout path, timestamps, source, model
  provider, cwd, title, sandbox policy, approval mode, token usage, archive
  flags, and git info (`frameworks/codex/codex-rs/state/migrations/0001_threads.sql:1-25`).
- Memory migration adds `stage1_outputs` and `jobs`
  (`frameworks/codex/codex-rs/state/migrations/0006_memories.sql:1-31`).
- Logs DB schema stores timestamp, level, target, feedback log body, module/file
  metadata, thread id, process uuid, and estimated bytes
  (`frameworks/codex/codex-rs/state/logs_migrations/0002_logs_feedback_log_body.sql:3-52`).
- `log_db` captures tracing events into a bounded queue and inserts them into
  dedicated logs SQLite in batches (`frameworks/codex/codex-rs/state/src/log_db.rs:1-6`,
  `frameworks/codex/codex-rs/state/src/log_db.rs:47-127`,
  `frameworks/codex/codex-rs/state/src/log_db.rs:397-403`).
- Log retention is 10 days plus per-partition caps of about 10 MiB or 1000 rows;
  startup maintenance deletes old rows, checkpoints WAL, and vacuums
  (`frameworks/codex/codex-rs/state/src/runtime.rs:77-84`,
  `frameworks/codex/codex-rs/state/src/runtime/logs.rs:1-61`,
  `frameworks/codex/codex-rs/state/src/runtime/logs.rs:288-310`).

### Memory subsystem

- Memory startup is skipped for ephemeral sessions, disabled feature flags, and
  subagents; it requires state DB, then prunes phase 1 data, runs phase 1, and
  runs phase 2 (`frameworks/codex/codex-rs/core/src/memories/start.rs:10-43`).
- Memory root is `CODEX_HOME/memories`; memory artifacts include
  `rollout_summaries/`, `raw_memories.md`, and sibling
  `memories_extensions`
  (`frameworks/codex/codex-rs/core/src/memories/mod.rs:27-31`,
  `frameworks/codex/codex-rs/core/src/memories/mod.rs:105-122`).
- Phase 1 selects stale eligible threads, excludes the current thread, requires
  `memory_mode = 'enabled'`, and claims jobs from the state DB
  (`frameworks/codex/codex-rs/state/src/runtime/memories.rs:97-125`).
- Successful phase 1 writes raw memory and rollout summary into
  `stage1_outputs` and advances the global phase 2 job watermark
  (`frameworks/codex/codex-rs/state/src/runtime/memories.rs:707-780`).
- Phase 2 selects memory rows from `stage1_outputs` joined with threads, syncs
  rollout summary files, rebuilds `raw_memories.md`, and may spawn a memory
  consolidation subagent (`frameworks/codex/codex-rs/state/src/runtime/memories.rs:249-390`,
  `frameworks/codex/codex-rs/core/src/memories/phase2.rs:47-155`).
- Filesystem memory artifacts are rebuilt from DB-backed `Stage1Output` rows;
  stale summaries are pruned, retained summaries are written, empty sets remove
  `MEMORY.md`, `memory_summary.md`, and `skills` under the memory root
  (`frameworks/codex/codex-rs/core/src/memories/storage.rs:12-60`,
  `frameworks/codex/codex-rs/core/src/memories/storage.rs:62-153`).

### Auth, credentials, caches, and other runtime state

- CLI auth file storage uses `CODEX_HOME/auth.json`, mode `0600` on Unix
  (`frameworks/codex/codex-rs/login/src/auth/storage.rs:29-61`,
  `frameworks/codex/codex-rs/login/src/auth/storage.rs:100-128`).
- Auth can also be stored in keyring or an in-memory ephemeral store keyed by
  `CODEX_HOME`; auto mode prefers keyring and falls back to file
  (`frameworks/codex/codex-rs/login/src/auth/storage.rs:135-223`,
  `frameworks/codex/codex-rs/login/src/auth/storage.rs:225-332`).
- MCP OAuth credentials prefer keyring and fall back to
  `CODEX_HOME/.credentials.json`; fallback file mode writes `0600` on Unix
  (`frameworks/codex/codex-rs/rmcp-client/src/oauth.rs:1-18`,
  `frameworks/codex/codex-rs/rmcp-client/src/oauth.rs:79-172`,
  `frameworks/codex/codex-rs/rmcp-client/src/oauth.rs:371-459`,
  `frameworks/codex/codex-rs/rmcp-client/src/oauth.rs:530-580`).
- TUI file logs default to `CODEX_HOME/log/codex-tui.log`; direct login logs
  default to `CODEX_HOME/log/codex-login.log`
  (`frameworks/codex/codex-rs/core/src/config/mod.rs:2218-2228`,
  `frameworks/codex/codex-rs/tui/src/lib.rs:900-1001`,
  `frameworks/codex/codex-rs/cli/src/login.rs:39-105`).
- Optional TUI session recording writes a JSONL `session-<timestamp>.jsonl` in
  `log_dir` unless `CODEX_TUI_SESSION_LOG_PATH` is set
  (`frameworks/codex/codex-rs/tui/src/session_log.rs:80-119`).
- Model metadata cache is `CODEX_HOME/models_cache.json` with a default TTL of
  300 seconds (`frameworks/codex/codex-rs/models-manager/src/manager.rs:23-24`,
  `frameworks/codex/codex-rs/models-manager/src/manager.rs:198-217`,
  `frameworks/codex/codex-rs/models-manager/src/cache.rs:14-123`,
  `frameworks/codex/codex-rs/models-manager/src/cache.rs:160-182`).
- Plugin cache lives under `CODEX_HOME/plugins/cache`, marketplace installs use
  `CODEX_HOME/.tmp/marketplaces`, and plugin materialization uses a staging
  path under the plugin area (`frameworks/codex/codex-rs/core-plugins/src/store.rs:14-54`,
  `frameworks/codex/codex-rs/core-plugins/src/installed_marketplaces.rs:11-60`,
  `frameworks/codex/codex-rs/core-plugins/src/loader.rs:894-947`).
- MCP Codex Apps tool cache lives under `CODEX_HOME/cache/codex_apps_tools`
  (`frameworks/codex/codex-rs/codex-mcp/src/mcp_connection_manager.rs:100-111`,
  `frameworks/codex/codex-rs/codex-mcp/src/mcp_connection_manager.rs:1617-1699`).
- Tool approval state is not shown as a durable file; approval cache is
  in-memory per session (`frameworks/codex/codex-rs/core/src/tools/sandboxing.rs:40-63`).

## Data Boundary Matrix

| Boundary | Codex source owner | Codex storage or behavior | AVM package placement signal |
|---|---|---|---|
| CLI entry and subcommand dispatch | `cli` crate | Rust `codex` dispatches TUI, exec, MCP, app-server, plugin, sandbox (`frameworks/codex/codex-rs/cli/src/main.rs:704-832`) | Adapter-owned signal: Codex binary/entry discovery and command mapping are target-specific. Runtime-owned signal: no Codex subcommand assumptions in generic lifecycle. |
| Config layer merge | `core::config_loader`, `core::config` | Requirements, system, user, project, runtime layers (`frameworks/codex/codex-rs/core/src/config_loader/mod.rs:99-124`) | Config-owned signal: declarative policy/profile data and trust layering. Adapter-owned signal: Codex-specific TOML translation. |
| Session lifecycle | `core::session`, app-server client/server | Thread persistence, state DB, history metadata, auth, MCP, skills, AGENTS (`frameworks/codex/codex-rs/core/src/session/session.rs:300-415`) | Runtime-owned signal: start/resume/turn lifecycle and event wiring. State-owned signal: AVM session metadata references. |
| Sandbox policy | `protocol`, `core::config`, `codex-sandboxing` | Approval, file-system, network, writable roots are distinct (`frameworks/codex/codex-rs/protocol/src/protocol.rs:939-1081`) | Runtime-owned signal: normalized isolation semantics. Adapter-owned signal: Codex flags/config/helper mapping. |
| Shell execution | `core::tools`, `core::spawn` | Approval -> sandbox transform -> env-clean spawn (`frameworks/codex/codex-rs/core/src/tools/runtimes/shell.rs:134-289`, `frameworks/codex/codex-rs/core/src/spawn.rs:51-125`) | Runtime-owned signal: execution boundary. Adapter-owned signal: Codex concrete tool permissions and helper paths. |
| Skills | `skills`, `core-skills`, plugin loader | Bundled cache plus local/admin/repo/plugin roots (`frameworks/codex/codex-rs/core-skills/src/loader.rs:221-357`) | Package IO-owned signal: package skill directories. Adapter-owned signal: Codex root conventions and prompt injection behavior. |
| MCP servers | `config`, `cli::mcp_cmd`, `codex-mcp`, `rmcp-client` | Global config plus plugin MCP plus skill dependency auto-install (`frameworks/codex/codex-rs/core/src/config/mod.rs:848-873`, `frameworks/codex/codex-rs/core/src/mcp_skill_dependencies.rs:75-126`) | Config owns declarative MCP records. Adapter owns Codex serialization, OAuth store, and start/connect details. |
| Plugins/apps/connectors | `core-plugins`, `plugin` | Plugin cache/marketplaces/manifest; plugin contributes skills/MCP/apps (`frameworks/codex/codex-rs/core-plugins/src/loader.rs:451-548`) | Package IO owns plugin bundle import/export. Adapter maps Codex plugin manifests and marketplaces. |
| Rollout/session files | `rollout`, `thread-store`, `state` | JSONL rollout under `CODEX_HOME/sessions`, SQLite thread metadata (`frameworks/codex/codex-rs/rollout/src/recorder.rs:1320-1350`, `frameworks/codex/codex-rs/state/migrations/0001_threads.sql:1-25`) | State-owned signal: portable session indexes and references. Adapter-owned signal: Codex rollout remains runtime-specific state. |
| Memory artifacts | `core::memories`, `state::runtime::memories` | DB-backed stage1 rows plus `CODEX_HOME/memories` files (`frameworks/codex/codex-rs/core/src/memories/mod.rs:105-122`) | State owns memory metadata policy. Adapter maps Codex's memory root and writable-root side effect. |
| Auth and tokens | `login`, `rmcp-client::oauth` | `auth.json`, keyring, ephemeral store, `.credentials.json` (`frameworks/codex/codex-rs/login/src/auth/storage.rs:29-61`, `frameworks/codex/codex-rs/rmcp-client/src/oauth.rs:1-18`) | Config/state-owned signal: secret references. Adapter-owned signal: token files/keyring entries stay outside portable packages. |

## Evidence Table

| Claim | Evidence |
|---|---|
| Rust CLI is the runtime entry, not Node package scripts. | `frameworks/codex/package.json:1-35`; `frameworks/codex/codex-rs/cli/src/main.rs:69-83`; `frameworks/codex/codex-rs/cli/src/main.rs:704-832` |
| No-subcommand path starts interactive TUI. | `frameworks/codex/codex-rs/cli/src/main.rs:727-740`; `frameworks/codex/codex-rs/cli/src/main.rs:1490-1542` |
| Exec path builds config and starts an in-process app-server client/thread. | `frameworks/codex/codex-rs/exec/src/lib.rs:386-420`; `frameworks/codex/codex-rs/exec/src/lib.rs:493-512`; `frameworks/codex/codex-rs/exec/src/lib.rs:651-768` |
| Config layers include system, user, project, and runtime layers. | `frameworks/codex/codex-rs/core/src/config_loader/mod.rs:99-124`; `frameworks/codex/codex-rs/core/src/config_loader/mod.rs:211-319`; `frameworks/codex/codex-rs/core/src/config_loader/mod.rs:930-1024` |
| Workspace/write policy is separate from approval policy. | `frameworks/codex/codex-rs/protocol/src/protocol.rs:939-1081`; `frameworks/codex/codex-rs/core/src/config/mod.rs:1808-1932` |
| Workspace-write writes cwd/writable roots/tmp but defaults `.git`, `.agents`, `.codex` under writable roots to read-only. | `frameworks/codex/codex-rs/protocol/src/protocol.rs:1180-1316` |
| Approval decisions are per-session cached, not durable. | `frameworks/codex/codex-rs/core/src/tools/sandboxing.rs:40-117` |
| Shell execution boundary is approval -> sandbox transform -> spawn. | `frameworks/codex/codex-rs/core/src/tools/runtimes/shell.rs:134-289`; `frameworks/codex/codex-rs/core/src/spawn.rs:51-125` |
| Linux sandbox uses bubblewrap/seccomp/no_new_privs. | `frameworks/codex/codex-rs/linux-sandbox/src/lib.rs:1-5`; `frameworks/codex/codex-rs/linux-sandbox/src/linux_run_main.rs:94-221` |
| Skills are local directories with `SKILL.md` loaded from several roots. | `frameworks/codex/codex-rs/core-skills/src/loader.rs:105-123`; `frameworks/codex/codex-rs/core-skills/src/loader.rs:221-357` |
| Bundled system skills are materialized into `CODEX_HOME/skills/.system`. | `frameworks/codex/codex-rs/skills/src/lib.rs:10-55` |
| Explicit skill mentions inject full `SKILL.md` into context. | `frameworks/codex/codex-rs/core-skills/src/injection.rs:31-85`; `frameworks/codex/codex-rs/core/src/context/skill_instructions.rs:22-32` |
| MCP servers are config records and plugin/skill-derived records. | `frameworks/codex/codex-rs/config/src/mcp_types.rs:117-391`; `frameworks/codex/codex-rs/core/src/config/mod.rs:848-873`; `frameworks/codex/codex-rs/core/src/mcp_skill_dependencies.rs:75-126` |
| MCP connection manager starts clients and caches app tools. | `frameworks/codex/codex-rs/codex-mcp/src/mcp_connection_manager.rs:666-838`; `frameworks/codex/codex-rs/codex-mcp/src/mcp_connection_manager.rs:1424-1512`; `frameworks/codex/codex-rs/codex-mcp/src/mcp_connection_manager.rs:1617-1699` |
| Plugins have separate marketplace/cache/manifest mechanics. | `frameworks/codex/codex-rs/core-plugins/src/store.rs:29-54`; `frameworks/codex/codex-rs/core-plugins/src/manifest.rs:11-64`; `frameworks/codex/codex-rs/core-plugins/src/marketplace.rs:20-23` |
| AGENTS.md is loaded globally and along the project path. | `frameworks/codex/codex-rs/core/src/agents_md.rs:1-17`; `frameworks/codex/codex-rs/core/src/agents_md.rs:36-78`; `frameworks/codex/codex-rs/core/src/agents_md.rs:213-320` |
| Prompt history is `CODEX_HOME/history.jsonl`. | `frameworks/codex/codex-rs/core/src/message_history.rs:1-17`; `frameworks/codex/codex-rs/core/src/message_history.rs:47-160` |
| Session rollout is JSONL under `CODEX_HOME/sessions/YYYY/MM/DD`. | `frameworks/codex/codex-rs/rollout/src/recorder.rs:1320-1350` |
| State and logs are separate SQLite DBs under `CODEX_HOME`. | `frameworks/codex/codex-rs/state/src/runtime.rs:95-156`; `frameworks/codex/codex-rs/state/src/runtime.rs:210-228` |
| Memory is both DB-backed and filesystem-backed. | `frameworks/codex/codex-rs/state/migrations/0006_memories.sql:1-31`; `frameworks/codex/codex-rs/core/src/memories/storage.rs:12-153` |
| Auth/token storage includes file, keyring, and ephemeral modes. | `frameworks/codex/codex-rs/login/src/auth/storage.rs:29-61`; `frameworks/codex/codex-rs/login/src/auth/storage.rs:135-332`; `frameworks/codex/codex-rs/rmcp-client/src/oauth.rs:1-18` |
| Model cache is `CODEX_HOME/models_cache.json`. | `frameworks/codex/codex-rs/models-manager/src/manager.rs:23-24`; `frameworks/codex/codex-rs/models-manager/src/manager.rs:198-217` |
| Cargo verification could not run in this environment. | Shell output from attempted `cargo metadata` and `cargo run ... -- --help`: `zsh:1: command not found: cargo`. |

## Risks For AVM PRD

- A PRD premise that Codex has one extension registry is false in this source
  tree. Skills, MCP servers, plugins, marketplaces, apps, and skill dependency
  installs have separate loading and storage paths
  (`frameworks/codex/codex-rs/core-skills/src/loader.rs:221-357`,
  `frameworks/codex/codex-rs/core/src/config/mod.rs:848-873`,
  `frameworks/codex/codex-rs/core-plugins/src/store.rs:29-54`).
- A PRD premise that sandbox is one switch is false. Codex separates approval,
  legacy sandbox policy, file-system sandbox policy, network policy, managed
  network, writable roots, and platform helper transforms
  (`frameworks/codex/codex-rs/protocol/src/protocol.rs:939-1081`,
  `frameworks/codex/codex-rs/core/src/config/mod.rs:189-231`,
  `frameworks/codex/codex-rs/sandboxing/src/manager.rs:142-272`).
- A PRD premise that runtime permissions are durable user state is only partly
  true. Configured permissions are durable, but command approvals are
  per-session in-memory (`frameworks/codex/codex-rs/core/src/tools/sandboxing.rs:40-117`).
- A PRD premise that memory is passive context is incomplete. Codex creates
  `CODEX_HOME/memories`, adds it to writable roots, writes DB-backed memory
  artifacts, and can spawn a memory consolidation subagent
  (`frameworks/codex/codex-rs/core/src/config/mod.rs:1794-1801`,
  `frameworks/codex/codex-rs/core/src/memories/phase2.rs:140-155`).
- A PRD premise that `CODEX_HOME` can be treated as a simple portable profile
  is risky. It contains config, auth, OAuth credentials, history, rollouts,
  state DBs, logs, caches, skills, plugins, plugin temp data, and memory
  artifacts (`frameworks/codex/codex-rs/core/src/config/mod.rs:461-475`,
  `frameworks/codex/codex-rs/login/src/auth/storage.rs:29-61`,
  `frameworks/codex/codex-rs/rollout/src/recorder.rs:1320-1350`,
  `frameworks/codex/codex-rs/state/src/runtime.rs:210-228`).
- A PRD premise that runtime state can be inferred from JSONL alone is false.
  Codex uses rollout JSONL plus SQLite thread/log/memory/job tables plus
  session index JSONL (`frameworks/codex/codex-rs/state/migrations/0001_threads.sql:1-25`,
  `frameworks/codex/codex-rs/state/migrations/0006_memories.sql:1-31`,
  `frameworks/codex/codex-rs/rollout/src/session_index.rs:17-64`).
- A PRD premise that Codex isolation can be adopted verbatim is only partially
  validated. The boundary is a useful baseline because policies are explicit,
  but enforcement depends on Codex-specific helpers, platform features, and
  config semantics (`frameworks/codex/codex-rs/linux-sandbox/src/linux_run_main.rs:30-91`,
  `frameworks/codex/codex-rs/sandboxing/src/policy_transforms.rs:631-651`).

## Open Questions

- The local source checkout version/commit was not identified in this note.
  Exact behavior may differ across Codex versions.
- Rust build/runtime help could not be verified because `cargo` was missing
  from PATH. The startup findings are source-traced, not binary-validated.
- Remote executor and cloud session behavior were only traced where they touched
  exec/app-server/MCP paths; their full state and isolation model need a
  separate pass.
- Windows sandboxing was identified through helper modules but not traced as
  deeply as Linux sandboxing.
- Codex Apps/connectors were traced through plugin app loading and MCP tool
  cache, but connector-specific auth and UI behavior were not exhaustively
  traced.
- Feature flags can enable or disable material behavior such as bundled skills,
  skill MCP dependency installation, memory tool, plugin marketplaces, and
  sandbox modes. Treating these paths as mandatory behavior requires a pinned
  feature-flag scope.
