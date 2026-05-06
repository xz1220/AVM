# Codex 运行时源码调研

本文调研 `frameworks/codex` 下的本地 Codex runtime 实现，重点关注可验证 Agent VM PRD 前提的源码事实。本文刻意避免产品建议。

行号引用来自调研时的本地 checkout。

## 摘要

- Codex runtime 主要是 `frameworks/codex/codex-rs` 下的 Rust workspace。顶层 `frameworks/codex/package.json` 是 private Node monorepo，包含 formatting/schema scripts，不是 runtime entrypoint（`frameworks/codex/package.json:1-35`）。可执行入口是 `codex-rs/cli` 中的 Rust `codex` CLI，其 `main()` 分发 TUI、exec、MCP-server、app-server、sandbox debug、plugin、login 和 cloud commands（`frameworks/codex/codex-rs/cli/src/main.rs:69-83`、`frameworks/codex/codex-rs/cli/src/main.rs:101-172`、`frameworks/codex/codex-rs/cli/src/main.rs:704-832`）。
- Runtime startup 是分层的：CLI flags 变成 `ConfigOverrides`，config loader 合并 requirement、global、user、project 和 runtime layers；随后 session startup 创建或恢复 thread，加载 auth、MCP、plugin、skill、AGENTS.md、history 和 state DB services（`frameworks/codex/codex-rs/core/src/config_loader/mod.rs:99-124`、`frameworks/codex/codex-rs/core/src/config/mod.rs:690-827`、`frameworks/codex/codex-rs/core/src/session/session.rs:300-415`、`frameworks/codex/codex-rs/core/src/session/mod.rs:450-575`）。
- 隔离被表达为互相独立的 approval、sandbox、file-system sandbox、network 和 writable-root models。Enforcement 分布在 config derivation、approval orchestration、shell runtime、platform sandbox command transforms 和 helper binaries 中（`frameworks/codex/codex-rs/protocol/src/protocol.rs:939-1081`、`frameworks/codex/codex-rs/core/src/tools/sandboxing.rs:198-277`、`frameworks/codex/codex-rs/sandboxing/src/manager.rs:142-272`）。
- Skills、MCP servers 和 plugins 不使用一个统一 registry。Skills 从 configured local roots、repo roots、user/admin roots、bundled system skill cache 和 plugin skill roots 发现。MCP servers 来自 config entries、plugin MCP configs，以及受 feature gate 控制的 skill-dependency installs。Plugins 有自己的 marketplace/cache/manifest store（`frameworks/codex/codex-rs/core-skills/src/loader.rs:221-357`、`frameworks/codex/codex-rs/core/src/config/mod.rs:848-873`、`frameworks/codex/codex-rs/core-plugins/src/store.rs:29-54`、`frameworks/codex/codex-rs/core/src/mcp_skill_dependencies.rs:75-126`）。
- Runtime state 分散在 `CODEX_HOME` 文件、SQLite DB、JSONL rollout files、logs、caches、keyring/file auth stores 和 in-memory session stores 中。默认 `CODEX_HOME` 是 `~/.codex`，除非设置了 `CODEX_HOME`（`frameworks/codex/codex-rs/core/src/config/mod.rs:2664-2674`）。
- 已尝试安全验证命令：`cargo metadata --manifest-path frameworks/codex/codex-rs/Cargo.toml --no-deps --format-version 1` 和 `cargo run --manifest-path frameworks/codex/codex-rs/Cargo.toml -p codex-cli -- --help`，但当前环境 PATH 中没有 `cargo`。这些命令未修改源码文件。

## 运行时启动路径

### 可执行入口与语言分层

- 顶层 Node package 是 private monorepo，要求 `pnpm` 和 Node >= 22，并包含 formatting/schema scripts（`frameworks/codex/package.json:1-35`）。源码追踪显示 runtime entrypoints 在 Rust 中，而不是 Node 中。
- Rust CLI root 是 `TopCli`；Clap 在 `frameworks/codex/codex-rs/cli/src/main.rs:69-172` 定义 global flags 和 subcommands。
- `main()` 调用 `arg0_dispatch_or_else`，随后调用 `cli_main`（`frameworks/codex/codex-rs/cli/src/main.rs:704-709`）。`cli_main` 解析 root flags 和 subcommands（`frameworks/codex/codex-rs/cli/src/main.rs:711-726`）。
- 没有 subcommand 时，Codex 通过 `run_interactive_tui(...)` 启动 interactive TUI（`frameworks/codex/codex-rs/cli/src/main.rs:727-740`）。
- `codex exec` 会把 root flags 合并进 exec CLI，并调用 `codex_exec::run_main`（`frameworks/codex/codex-rs/cli/src/main.rs:741-755`）。
- `codex mcp-server`、`codex mcp`、plugin marketplace commands、app-server 和 debug sandbox commands 都从同一个 CLI root 分发（`frameworks/codex/codex-rs/cli/src/main.rs:770-832`、`frameworks/codex/codex-rs/cli/src/main.rs:997-1045`）。
- Interactive TUI path 在调用 `codex_tui::run_main` 前会拒绝不支持的 terminal conditions（`frameworks/codex/codex-rs/cli/src/main.rs:1490-1542`）。
- 独立的 `codex-tui`、`codex-exec` 和 `codex-app-server` 也作为 Rust binary entrypoints 存在（`frameworks/codex/codex-rs/tui/src/main.rs:48-63`、`frameworks/codex/codex-rs/exec/src/main.rs:28-40`、`frameworks/codex/codex-rs/app-server/src/main.rs:41-66`）。

### 无头 exec 启动

- `codex_exec::run_main` 接收 image/model/profile/sandbox/cwd/add-dir 以及其他 CLI options（`frameworks/codex/codex-rs/exec/src/lib.rs:218-249`）。
- `--full-auto` 和 bypass flags 映射到 `WorkspaceWrite` 与 `DangerFullAccess` sandbox modes；否则使用 CLI sandbox 值（`frameworks/codex/codex-rs/exec/src/lib.rs:272-278`）。
- Exec 在构建 config 前解析 cwd 和 `CODEX_HOME`（`frameworks/codex/codex-rs/exec/src/lib.rs:290-306`）。
- 它预加载 config TOML，构建包含 cwd、model、approval、sandbox、permission profile、sandbox helper paths、ephemeral flag 和 additional writable roots 的 `ConfigOverrides`，然后调用 `ConfigBuilder::build`（`frameworks/codex/codex-rs/exec/src/lib.rs:315-339`、`frameworks/codex/codex-rs/exec/src/lib.rs:386-420`）。
- Exec 随后用 built config、environment manager、loader overrides、session source `Exec` 和 `client_name = "codex_exec"` 创建 `InProcessClientStartArgs`（`frameworks/codex/codex-rs/exec/src/lib.rs:493-512`）。
- 它启动或恢复一个 in-process app-server thread，然后以 cwd、approval policy、sandbox policy 或 permission profile、effort 和 output schema 启动一轮 turn（`frameworks/codex/codex-rs/exec/src/lib.rs:651-768`）。
- Exec 把 core `SandboxPolicy` 映射到 app-server protocol sandbox fields，并输出 configured-session fields，包括 session id、model/provider、approval、sandbox、permission profile、cwd 和 rollout path（`frameworks/codex/codex-rs/exec/src/lib.rs:913-980`、`frameworks/codex/codex-rs/exec/src/lib.rs:1064-1099`）。

### 配置与工作区加载

- Config layering 顺序是显式的：requirement layers、system/admin `/etc/codex/config.toml`、user `${CODEX_HOME}/config.toml`、cwd config、project tree `.codex/config.toml`、repo-root `.codex/config.toml`，最后是 runtime CLI/session flags（`frameworks/codex/codex-rs/core/src/config_loader/mod.rs:99-124`、`frameworks/codex/codex-rs/core/src/config_loader/mod.rs:211-319`）。
- Project config layers 从 cwd ancestors 到 project root 发现，受 trust gate 控制，并在 `.codex` 等于 `CODEX_HOME` 时跳过（`frameworks/codex/codex-rs/core/src/config_loader/mod.rs:930-1024`）。
- `ConfigBuilder` 解析 `CODEX_HOME` 和 cwd，加载 layer stack，反序列化 merged TOML，并构建最终 `Config`（`frameworks/codex/codex-rs/core/src/config/mod.rs:690-827`）。
- 最终 config 包含 cwd、auth mode、MCP servers、OAuth store mode、AGENTS settings、memories config、codex/sqlite/log homes、history settings 和 helper executable paths（`frameworks/codex/codex-rs/core/src/config/mod.rs:400-421`、`frameworks/codex/codex-rs/core/src/config/mod.rs:438-499`）。
- `CODEX_HOME` 默认是 `~/.codex`；如果设置了 `CODEX_HOME`，该路径必须可以 canonicalize（`frameworks/codex/codex-rs/core/src/config/mod.rs:2664-2674`）。
- Model provider 和 model settings 是 config fields，会与 built-in providers、profiles 和可选 model catalog JSON 合并（`frameworks/codex/codex-rs/core/src/config/mod.rs:257-278`、`frameworks/codex/codex-rs/core/src/config/mod.rs:1988-2002`、`frameworks/codex/codex-rs/core/src/config/mod.rs:2111-2217`）。

### 会话与 turn context 加载

- Session startup 准备 thread persistence，除非是 ephemeral；ephemeral sessions 会禁用 state DB；root sessions 会读取 history metadata，并加载 auth 与 effective MCP servers（`frameworks/codex/codex-rs/core/src/session/session.rs:300-415`）。
- `Session::new` 在 AGENTS instructions 前加载 plugin outcomes 和 skills，然后通过 `AgentsMdManager` 加载 user/project AGENTS.md instructions（`frameworks/codex/codex-rs/core/src/session/mod.rs:450-503`）。
- Session startup 计算 exec policy、model default/base instructions，并在 resume/fork 时从 DB 或 rollout 恢复 dynamic tools（`frameworks/codex/codex-rs/core/src/session/mod.rs:510-575`）。
- `SessionConfiguration` 携带 provider/model/reasoning/instructions、approval、sandbox、cwd、codex_home、dynamic tools 和 persistence state（`frameworks/codex/codex-rs/core/src/session/mod.rs:595-624`）。
- Session 在 MCP startup events 前发出 `SessionConfiguredEvent`，随后启动 skill watching 并初始化 MCP manager（`frameworks/codex/codex-rs/core/src/session/session.rs:717-890`）。
- Memory startup 在 session configuration 后启动（`frameworks/codex/codex-rs/core/src/session/session.rs:930-934`）。
- 每个 turn 会解析 environment/cwd，构建 per-turn config，更新 MCP approval/sandbox policies，加载 plugins/skill roots/skills，并创建 `TurnContext`（`frameworks/codex/codex-rs/core/src/session/turn_context.rs:615-690`）。

## 隔离模型

### 策略类型

- Approval modes 是 protocol types：`UnlessTrusted`、`OnFailure`、`OnRequest`、`Granular` 和 `Never`（`frameworks/codex/codex-rs/protocol/src/protocol.rs:939-970`）。
- Granular approval config 分别控制 sandbox approval、rules、skill approval、permission requests 和 MCP elicitations（`frameworks/codex/codex-rs/protocol/src/protocol.rs:972-1009`）。
- Network access 是一等 enum：`Restricted` 或 `Enabled`（`frameworks/codex/codex-rs/protocol/src/protocol.rs:1011-1027`）。
- Sandbox policy 是 `DangerFullAccess`、`ReadOnly`、`ExternalSandbox` 或 `WorkspaceWrite` 之一；workspace-write 携带 writable roots、network access 和 tmp exclusion flags（`frameworks/codex/codex-rs/protocol/src/protocol.rs:1029-1081`）。
- `WritableRoot` 支持 read-only subpaths 和 path-level writability checks（`frameworks/codex/codex-rs/protocol/src/protocol.rs:1083-1111`）。
- Workspace-write 允许写 writable roots、cwd、未排除的 `/tmp` 和未排除的 `TMPDIR`；writable roots 下默认 read-only subpaths 包括 `.git`、`.agents` 和 `.codex`（`frameworks/codex/codex-rs/protocol/src/protocol.rs:1180-1316`）。
- 面向模型的 workspace-write prompt 说明相同的高层 contract：可读所有文件，可编辑 cwd 和 writable roots，其他位置需 approval，并应用 network constraints（`frameworks/codex/codex-rs/core/src/context/prompts/permissions/sandbox_mode/workspace_write.md:1`）。

### 配置派生与 writable roots

- `Permissions` 拥有 approval policy、sandbox policy、file-system sandbox policy、network sandbox policy、network proxy、login-shell allowance、shell environment policy 和 Windows sandbox level（`frameworks/codex/codex-rs/core/src/config/mod.rs:189-231`）。
- Additional writable roots 相对于 cwd 解析（`frameworks/codex/codex-rs/core/src/config/mod.rs:1737-1759`）。
- Memory root 会被创建，并在缺失时总是追加到 additional writable roots（`frameworks/codex/codex-rs/core/src/config/mod.rs:1794-1801`）。
- Sandbox、file-system 和 network policies 从 permission profiles、named profiles 或 legacy sandbox mode 计算，再用 additional writable roots 增强（`frameworks/codex/codex-rs/core/src/config/mod.rs:1808-1932`）。
- Approval defaults 取决于 project trust：trusted 默认 `OnRequest`，untrusted 默认 `UnlessTrusted`，除非显式配置（`frameworks/codex/codex-rs/core/src/config/mod.rs:1933-1972`）。
- Helper-readable roots 会被加入 file-system sandbox policy，用于 `CODEX_HOME`、`zsh` 和 execve wrapper 支持（`frameworks/codex/codex-rs/core/src/config/mod.rs:2312-2340`）。

### 审批生命周期

- Approval cache 是内存中的 `ApprovalStore`，按序列化请求作为 key；它按 session 缓存，且该模块不持久化它（`frameworks/codex/codex-rs/core/src/tools/sandboxing.rs:40-63`）。
- `with_cached_approval` 只有在当前 session 已批准所有 keys 时才跳过 prompt（`frameworks/codex/codex-rs/core/src/tools/sandboxing.rs:71-117`）。
- Exec approval outcomes 是 `Skip { bypass_sandbox }`、`NeedsApproval` 或 `Forbidden`（`frameworks/codex/codex-rs/core/src/tools/sandboxing.rs:159-180`）。
- Policy checks 决定命令无需 approval、需要 user approval，还是 forbidden。`Never` 和 deprecated `OnFailure` 不询问；`OnRequest`/`Granular` 对 restricted file-system access 询问；`UnlessTrusted` 默认询问（`frameworks/codex/codex-rs/core/src/tools/sandboxing.rs:198-239`）。
- Escalated sandbox permissions 可以请求绕过 sandbox，且 escalated permissions 会禁用 managed network（`frameworks/codex/codex-rs/core/src/tools/sandboxing.rs:247-277`）。

### Shell 执行边界

- Shell handling 合并 environment，应用 granted permissions，拦截 `apply_patch`，计算 approval requirements，并构建 `ShellRequest`（`frameworks/codex/codex-rs/core/src/tools/handlers/shell.rs:420-548`）。
- `ShellRequest` 携带 command、cwd、sandbox preference、sandbox/permission requests、environment 和 timeout（`frameworks/codex/codex-rs/core/src/tools/runtimes/shell.rs:48-63`）。
- Shell runtime 根据 canonical command、cwd、sandbox permissions 和 additional permissions 构建 approval key，然后通过 orchestrator 请求 approval（`frameworks/codex/codex-rs/core/src/tools/runtimes/shell.rs:134-215`）。
- 在 spawn 前，shell runtime 应用 network/env changes，包装 command，将其转换成 platform sandbox command，然后调用 `execute_env`（`frameworks/codex/codex-rs/core/src/tools/runtimes/shell.rs:242-289`）。
- 底层 process spawn function 会清空 environment，只设置选定 env vars，设置 cwd，并 pipe 或继承 stdio。它假设 caller 已经为了 sandboxing 转换过 command（`frameworks/codex/codex-rs/core/src/spawn.rs:51-125`、`frameworks/codex/codex-rs/core/src/exec.rs:840-899`）。

### 平台沙箱与 network

- Platform sandbox types 是 `None`、macOS Seatbelt、Linux seccomp 和 Windows restricted token（`frameworks/codex/codex-rs/sandboxing/src/manager.rs:23-63`）。
- `SandboxManager::select_initial` 选择是否需要 platform sandbox，`transform` 添加 platform-specific wrappers 和 policy arguments（`frameworks/codex/codex-rs/sandboxing/src/manager.rs:142-272`）。
- Additional permissions 可在 execution 前放宽 file-system 或 network policies（`frameworks/codex/codex-rs/sandboxing/src/policy_transforms.rs:516-619`）。
- 除非 policy 是 external，当 managed network 或 restricted network 生效时需要 platform sandbox；在 network enabled 的情况下，restricted file-system 仍需要 platform sandbox（`frameworks/codex/codex-rs/sandboxing/src/policy_transforms.rs:631-651`）。
- Linux helper 应用 `no_new_privs`、seccomp 和 bubblewrap 来设置 file-system view（`frameworks/codex/codex-rs/linux-sandbox/src/lib.rs:1-5`）。
- Linux sandbox CLI arguments 包含 command cwd、sandbox policy、file-system policy、network policy、seccomp inner mode、proxy settings 和 command（`frameworks/codex/codex-rs/linux-sandbox/src/linux_run_main.rs:30-91`）。
- Linux setup 先运行 bubblewrap 做 file-system isolation，再应用 no_new_privs/seccomp，最后 `execvp`（`frameworks/codex/codex-rs/linux-sandbox/src/linux_run_main.rs:94-221`）。
- Restricted network 也通过 `CODEX_SANDBOX_NETWORK_DISABLED` 等环境变量暴露（`frameworks/codex/codex-rs/core/src/sandboxing/mod.rs:100-150`、`frameworks/codex/codex-rs/core/src/spawn.rs:12-25`）。

## Skill 安装与加载

- Bundled system skills 嵌入 Rust `codex-skills` crate，并安装到 `CODEX_HOME/skills/.system`（`frameworks/codex/codex-rs/skills/src/lib.rs:10-22`）。
- System skill installation 创建 `CODEX_HOME/skills`，写入 marker fingerprint，必要时删除旧 `.system` 目录，并写入 embedded directory tree（`frameworks/codex/codex-rs/skills/src/lib.rs:24-55`、`frameworks/codex/codex-rs/skills/src/lib.rs:101-127`）。
- `SkillsManager` 根据 config 安装或卸载 bundled skills，按 cwd/config 缓存加载结果，在禁用 bundled skills 时过滤 roots，并维护独立 caches 以防 rules 跨 roots 泄漏（`frameworks/codex/codex-rs/core-skills/src/manager.rs:50-126`）。
- Bundled skills 默认启用，除非 `[skills].bundled.enabled=false`（`frameworks/codex/codex-rs/core-skills/src/manager.rs:246-266`）。
- Skill 主要是包含 `SKILL.md` 的目录；可选 metadata 从 skill directory 下类似 `.agents/agents/openai.yaml` 的路径读取（`frameworks/codex/codex-rs/core-skills/src/loader.rs:37-123`）。
- Skill roots 从 project `.codex/skills`、deprecated `CODEX_HOME/skills`、user `$HOME/.agents/skills`、system cache、admin `/etc/codex/skills`、repo `.agents/skills`、configured extra roots 和 plugin roots 发现（`frameworks/codex/codex-rs/core-skills/src/loader.rs:221-357`）。
- Loader 扫描 roots 时有 depth 和 count caps，解析 `SKILL.md`，对 truncation 发 warning，验证 metadata，并按 scope rank 排序/去重（`frameworks/codex/codex-rs/core-skills/src/loader.rs:157-219`、`frameworks/codex/codex-rs/core-skills/src/loader.rs:440-642`）。
- Skill names 按最近的 plugin manifest 加 namespace，因此 plugin-installed skills 可与普通 local skills 区分（`frameworks/codex/codex-rs/core-skills/src/loader.rs:657-665`）。
- Available skills 被渲染进 developer instructions，并使用 progressive disclosure，要求模型只在需要时打开 `SKILL.md`（`frameworks/codex/codex-rs/core-skills/src/render.rs:21-83`）。
- 显式 skill mentions 会把完整 `SKILL.md` 内容注入该 turn 的 user context（`frameworks/codex/codex-rs/core-skills/src/injection.rs:31-85`、`frameworks/codex/codex-rs/core-skills/src/injection.rs:103-170`、`frameworks/codex/codex-rs/core/src/context/skill_instructions.rs:22-32`）。
- Per-turn processing 在把 skill/plugin items 记录进 conversation history 前，会解析显式 skill mentions、dependency prompts 和 skill injections（`frameworks/codex/codex-rs/core/src/session/turn.rs:167-271`、`frameworks/codex/codex-rs/core/src/session/turn.rs:349-355`）。

## MCP 安装与加载

- MCP server config 支持 stdio 和 streamable HTTP transports、enabled/required flags、startup/tool timeouts、approval mode、allow/deny tool lists、OAuth scopes/resource、per-tool config、cwd、env、headers，以及 bearer-token env/header mapping（`frameworks/codex/codex-rs/config/src/mcp_types.rs:117-241`）。
- Raw MCP config 被 normalize 成 `Stdio` 或 `StreamableHttp` variants，校验 incompatible fields，并默认 `enabled = true`（`frameworks/codex/codex-rs/config/src/mcp_types.rs:275-391`）。
- `codex mcp add` 支持一个 stdio command 或一个 `--url`，并支持 env 和 bearer token env var fields（`frameworks/codex/codex-rs/cli/src/mcp_cmd.rs:36-135`）。
- `codex mcp add` 从 `CODEX_HOME` 读取 global server config，并通过 `ConfigEditsBuilder.replace_mcp_servers` 将替换结果持久化到 global config（`frameworks/codex/codex-rs/cli/src/mcp_cmd.rs:255-324`）。
- Runtime MCP config 合并 config-defined servers 与 plugin MCP servers，并携带 auth、OAuth store mode、skill dependency flag、approval、sandbox、Linux sandbox helper、app summaries 和 plugin summaries（`frameworks/codex/codex-rs/core/src/config/mod.rs:848-873`）。
- Session startup 在构造 `McpConnectionManager` 前计算 effective MCP servers 和 auth statuses（`frameworks/codex/codex-rs/core/src/session/session.rs:383-397`、`frameworks/codex/codex-rs/core/src/session/session.rs:853-872`）。
- Session MCP refresh 可以重新计算 config/plugins/provenance 并替换 manager（`frameworks/codex/codex-rs/core/src/session/mcp.rs:205-303`）。
- `McpConnectionManager` 为每个配置的 server 拥有一个 `RmcpClient`，并用 fully qualified names 聚合 tools（`frameworks/codex/codex-rs/codex-mcp/src/mcp_connection_manager.rs:1-7`、`frameworks/codex/codex-rs/codex-mcp/src/mcp_connection_manager.rs:887-899`）。
- MCP startup 创建 managed clients，启动 enabled servers，发出 startup events，存储 origins，初始化 protocol version `2025-06-18`，列出 tools，并把 Codex Apps tools 缓存到 `CODEX_HOME/cache/codex_apps_tools`（`frameworks/codex/codex-rs/codex-mcp/src/mcp_connection_manager.rs:507-588`、`frameworks/codex/codex-rs/codex-mcp/src/mcp_connection_manager.rs:666-838`、`frameworks/codex/codex-rs/codex-mcp/src/mcp_connection_manager.rs:1424-1512`、`frameworks/codex/codex-rs/codex-mcp/src/mcp_connection_manager.rs:1617-1699`）。
- MCP calls 通过 session-owned manager 路由，然后到所选 server/client/tool（`frameworks/codex/codex-rs/core/src/tools/handlers/mcp.rs:56-100`、`frameworks/codex/codex-rs/core/src/session/mcp.rs:122-190`、`frameworks/codex/codex-rs/codex-mcp/src/mcp_connection_manager.rs:1088-1124`）。
- Stdio MCP servers 可作为本地 child processes 启动，也可通过 executor process API 远程启动（`frameworks/codex/codex-rs/rmcp-client/src/stdio_server_launcher.rs:1-12`、`frameworks/codex/codex-rs/rmcp-client/src/stdio_server_launcher.rs:60-72`、`frameworks/codex/codex-rs/rmcp-client/src/stdio_server_launcher.rs:159-265`、`frameworks/codex/codex-rs/rmcp-client/src/stdio_server_launcher.rs:315-460`）。
- HTTP MCP clients 使用 bearer tokens 或 OAuth；OAuth tokens 通过 keyring/file modes 加载/持久化（`frameworks/codex/codex-rs/rmcp-client/src/rmcp_client.rs:658-760`、`frameworks/codex/codex-rs/rmcp-client/src/oauth.rs:79-172`）。
- Skill-declared MCP dependencies 受 feature gate 控制，只支持 first-party，从 mentioned skills 检测，可选提示用户，然后把缺失 server configs 安装进 global MCP config（`frameworks/codex/codex-rs/core/src/mcp_skill_dependencies.rs:32-73`、`frameworks/codex/codex-rs/core/src/mcp_skill_dependencies.rs:75-126`、`frameworks/codex/codex-rs/core/src/mcp_skill_dependencies.rs:216-288`、`frameworks/codex/codex-rs/core/src/mcp_skill_dependencies.rs:415-470`）。

### 插件、连接器与 registry 边界

- Plugins 存储在 `CODEX_HOME/plugins/cache/{marketplace}/{plugin}/{version}`（`frameworks/codex/codex-rs/core-plugins/src/store.rs:14-54`）。
- Plugin install 验证 source 和 manifest，然后 atomic replace cached plugin directory（`frameworks/codex/codex-rs/core-plugins/src/store.rs:88-130`、`frameworks/codex/codex-rs/core-plugins/src/store.rs:246-310`）。
- Plugin manifests 是 `.codex-plugin/plugin.json` 文件，包含 name、version、description、skills、MCP servers、apps 和 interface fields（`frameworks/codex/codex-rs/core-plugins/src/manifest.rs:11-64`、`frameworks/codex/codex-rs/core-plugins/src/manifest.rs:117-224`）。
- Marketplace manifests 在 `.agents/plugins/marketplace.json` 或 `.claude-plugin/marketplace.json` 中发现（`frameworks/codex/codex-rs/core-plugins/src/marketplace.rs:20-23`、`frameworks/codex/codex-rs/core-plugins/src/marketplace.rs:220-247`）。
- Installed marketplace roots 可来自 user `[marketplaces]` config，或默认的 `CODEX_HOME/.tmp/marketplaces`（`frameworks/codex/codex-rs/core-plugins/src/installed_marketplaces.rs:11-60`）。
- Configured plugins 只从 user config layer 加载；active plugins 贡献 skill roots、MCP servers 和 apps（`frameworks/codex/codex-rs/core-plugins/src/loader.rs:103-140`、`frameworks/codex/codex-rs/core-plugins/src/loader.rs:333-359`、`frameworks/codex/codex-rs/core-plugins/src/loader.rs:451-548`）。
- Plugin skills 通过同一个 skill loader 加载；plugin MCP config 从 manifest override 或默认 `.mcp.json` 读取；plugin apps 默认是 `.app.json`（`frameworks/codex/codex-rs/core-plugins/src/loader.rs:568-641`、`frameworks/codex/codex-rs/core-plugins/src/loader.rs:643-714`、`frameworks/codex/codex-rs/core-plugins/src/loader.rs:755-880`）。
- Effective plugin skill roots 和 MCP servers 是 plugin load outcome 中的独立 collections（`frameworks/codex/codex-rs/plugin/src/load_outcome.rs:11-25`、`frameworks/codex/codex-rs/plugin/src/load_outcome.rs:104-140`）。

## 记忆与状态存储

### 全局、项目与用户 memory/config

- `CODEX_HOME` 默认是 `~/.codex`，是 user config、auth、history、skills、plugins、memory artifacts、logs、caches 和 SQLite state 的锚点（`frameworks/codex/codex-rs/core/src/config/mod.rs:2664-2674`）。
- User config 位于 `${CODEX_HOME}/config.toml`；project config 可根据 trust 存在于 cwd/repo `.codex/config.toml` layers（`frameworks/codex/codex-rs/core/src/config_loader/mod.rs:211-319`、`frameworks/codex/codex-rs/core/src/config_loader/mod.rs:930-1024`）。
- AGENTS instructions 会从 `CODEX_HOME` 和 project roots 发现，project 路径从 project root 到 cwd；override filenames 先于 fallback filenames 检查（`frameworks/codex/codex-rs/core/src/agents_md.rs:1-17`、`frameworks/codex/codex-rs/core/src/agents_md.rs:36-78`、`frameworks/codex/codex-rs/core/src/agents_md.rs:213-320`）。
- AGENTS/user instructions 被渲染为 user-context fragments（`frameworks/codex/codex-rs/core/src/context/user_instructions.rs:9-16`）。

### 历史、rollout 与压缩

- Prompt history 是 `CODEX_HOME` 下名为 `history.jsonl` 的 append-only JSONL 文件；persistence 可被禁用，Unix 上 file mode 为 owner-only，并通过修剪旧行强制 size limits（`frameworks/codex/codex-rs/core/src/message_history.rs:1-17`、`frameworks/codex/codex-rs/core/src/message_history.rs:47-90`、`frameworks/codex/codex-rs/core/src/message_history.rs:95-160`、`frameworks/codex/codex-rs/core/src/message_history.rs:165-292`）。
- Thread rollout files 存在 `CODEX_HOME/sessions/YYYY/MM/DD/rollout-YYYY-MM-DDThh-mm-ss-<thread-id>.jsonl` 下；创建时预计算路径，并把文件创建延迟到 persistence 阶段（`frameworks/codex/codex-rs/rollout/src/lib.rs:21-22`、`frameworks/codex/codex-rs/rollout/src/recorder.rs:629-693`、`frameworks/codex/codex-rs/rollout/src/recorder.rs:1320-1350`）。
- Rollout writer 拥有 background async writer，缓冲 pending items，重试失败 writes，在 materialized 时写 session meta，并在 persist、flush 或 shutdown 时 flush（`frameworks/codex/codex-rs/rollout/src/recorder.rs:714-760`、`frameworks/codex/codex-rs/rollout/src/recorder.rs:1366-1535`）。
- `session_index.jsonl` 是 `CODEX_HOME` 下记录 thread names 的 append-only 文件；最新 entry 生效，name lookup 解析到 readable rollout（`frameworks/codex/codex-rs/rollout/src/session_index.rs:17-64`、`frameworks/codex/codex-rs/rollout/src/session_index.rs:115-154`）。
- Compaction 作为带 message 和可选 replacement history 的 rollout item 存储（`frameworks/codex/codex-rs/protocol/src/protocol.rs:2844-2860`）。
- Local compaction 重写 live history，持久化 `RolloutItem::Compacted`，可选持久化后续 `TurnContext`，并推进 model window generation（`frameworks/codex/codex-rs/core/src/session/mod.rs:2434-2450`）。
- Manual/auto compact tasks 通过 `CompactTask` 路由，由它选择 local 或 remote compaction（`frameworks/codex/codex-rs/core/src/tasks/compact.rs:10-46`）。

### SQLite state 与日志

- `StateRuntime::init` 创建 `CODEX_HOME`，迁移一个 state SQLite database 和一个独立 logs SQLite database，并运行 startup log maintenance（`frameworks/codex/codex-rs/state/src/runtime.rs:95-156`）。
- State 与 logs DB 文件名按 `<base>_<version>.sqlite` 版本化，并都直接位于 `CODEX_HOME` 下（`frameworks/codex/codex-rs/state/src/runtime.rs:210-228`）。
- `threads` table 存储 id、rollout path、timestamps、source、model provider、cwd、title、sandbox policy、approval mode、token usage、archive flags 和 git info（`frameworks/codex/codex-rs/state/migrations/0001_threads.sql:1-25`）。
- Memory migration 添加 `stage1_outputs` 和 `jobs`（`frameworks/codex/codex-rs/state/migrations/0006_memories.sql:1-31`）。
- Logs DB schema 存储 timestamp、level、target、feedback log body、module/file metadata、thread id、process uuid 和 estimated bytes（`frameworks/codex/codex-rs/state/logs_migrations/0002_logs_feedback_log_body.sql:3-52`）。
- `log_db` 把 tracing events 捕获进 bounded queue，并分批插入 dedicated logs SQLite（`frameworks/codex/codex-rs/state/src/log_db.rs:1-6`、`frameworks/codex/codex-rs/state/src/log_db.rs:47-127`、`frameworks/codex/codex-rs/state/src/log_db.rs:397-403`）。
- Log retention 是 10 天，加上每个 partition 约 10 MiB 或 1000 行的 caps；startup maintenance 删除 old rows、checkpoint WAL 并 vacuum（`frameworks/codex/codex-rs/state/src/runtime.rs:77-84`、`frameworks/codex/codex-rs/state/src/runtime/logs.rs:1-61`、`frameworks/codex/codex-rs/state/src/runtime/logs.rs:288-310`）。

### 记忆子系统

- Memory startup 会在 ephemeral sessions、disabled feature flags 和 subagents 下跳过；它要求 state DB，然后 prune phase 1 data，运行 phase 1，再运行 phase 2（`frameworks/codex/codex-rs/core/src/memories/start.rs:10-43`）。
- Memory root 是 `CODEX_HOME/memories`；memory artifacts 包括 `rollout_summaries/`、`raw_memories.md` 和同级 `memories_extensions`（`frameworks/codex/codex-rs/core/src/memories/mod.rs:27-31`、`frameworks/codex/codex-rs/core/src/memories/mod.rs:105-122`）。
- Phase 1 选择 stale eligible threads，排除 current thread，要求 `memory_mode = 'enabled'`，并从 state DB claim jobs（`frameworks/codex/codex-rs/state/src/runtime/memories.rs:97-125`）。
- 成功的 phase 1 会把 raw memory 和 rollout summary 写入 `stage1_outputs`，并推进 global phase 2 job watermark（`frameworks/codex/codex-rs/state/src/runtime/memories.rs:707-780`）。
- Phase 2 从 `stage1_outputs` join threads 选择 memory rows，同步 rollout summary files，重建 `raw_memories.md`，并可能 spawn memory consolidation subagent（`frameworks/codex/codex-rs/state/src/runtime/memories.rs:249-390`、`frameworks/codex/codex-rs/core/src/memories/phase2.rs:47-155`）。
- Filesystem memory artifacts 从 DB-backed `Stage1Output` rows 重建；stale summaries 被 prune，retained summaries 被写入，empty sets 会删除 memory root 下的 `MEMORY.md`、`memory_summary.md` 和 `skills`（`frameworks/codex/codex-rs/core/src/memories/storage.rs:12-60`、`frameworks/codex/codex-rs/core/src/memories/storage.rs:62-153`）。

### Auth、credentials、caches 与其他运行时状态

- CLI auth file storage 使用 `CODEX_HOME/auth.json`，Unix 上 mode 为 `0600`（`frameworks/codex/codex-rs/login/src/auth/storage.rs:29-61`、`frameworks/codex/codex-rs/login/src/auth/storage.rs:100-128`）。
- Auth 也可以存入 keyring，或以 `CODEX_HOME` 为 key 存入 in-memory ephemeral store；auto mode 优先 keyring，fallback 到 file（`frameworks/codex/codex-rs/login/src/auth/storage.rs:135-223`、`frameworks/codex/codex-rs/login/src/auth/storage.rs:225-332`）。
- MCP OAuth credentials 优先 keyring，并 fallback 到 `CODEX_HOME/.credentials.json`；fallback file 在 Unix 上以 `0600` 写入（`frameworks/codex/codex-rs/rmcp-client/src/oauth.rs:1-18`、`frameworks/codex/codex-rs/rmcp-client/src/oauth.rs:79-172`、`frameworks/codex/codex-rs/rmcp-client/src/oauth.rs:371-459`、`frameworks/codex/codex-rs/rmcp-client/src/oauth.rs:530-580`）。
- TUI file logs 默认写到 `CODEX_HOME/log/codex-tui.log`；direct login logs 默认写到 `CODEX_HOME/log/codex-login.log`（`frameworks/codex/codex-rs/core/src/config/mod.rs:2218-2228`、`frameworks/codex/codex-rs/tui/src/lib.rs:900-1001`、`frameworks/codex/codex-rs/cli/src/login.rs:39-105`）。
- Optional TUI session recording 会在 `log_dir` 中写 `session-<timestamp>.jsonl`，除非设置了 `CODEX_TUI_SESSION_LOG_PATH`（`frameworks/codex/codex-rs/tui/src/session_log.rs:80-119`）。
- Model metadata cache 是 `CODEX_HOME/models_cache.json`，默认 TTL 300 秒（`frameworks/codex/codex-rs/models-manager/src/manager.rs:23-24`、`frameworks/codex/codex-rs/models-manager/src/manager.rs:198-217`、`frameworks/codex/codex-rs/models-manager/src/cache.rs:14-123`、`frameworks/codex/codex-rs/models-manager/src/cache.rs:160-182`）。
- Plugin cache 位于 `CODEX_HOME/plugins/cache`，marketplace installs 使用 `CODEX_HOME/.tmp/marketplaces`，plugin materialization 使用 plugin area 下的 staging path（`frameworks/codex/codex-rs/core-plugins/src/store.rs:14-54`、`frameworks/codex/codex-rs/core-plugins/src/installed_marketplaces.rs:11-60`、`frameworks/codex/codex-rs/core-plugins/src/loader.rs:894-947`）。
- MCP Codex Apps tool cache 位于 `CODEX_HOME/cache/codex_apps_tools`（`frameworks/codex/codex-rs/codex-mcp/src/mcp_connection_manager.rs:100-111`、`frameworks/codex/codex-rs/codex-mcp/src/mcp_connection_manager.rs:1617-1699`）。
- Tool approval state 没有表现为 durable file；approval cache 是 per-session in-memory（`frameworks/codex/codex-rs/core/src/tools/sandboxing.rs:40-63`）。

## 数据边界矩阵

| 边界 | Codex 源码归属 | Codex 存储或行为 | 对 AVM package 放置的信号 |
|---|---|---|---|
| CLI entry 与 subcommand dispatch | `cli` crate | Rust `codex` 分发 TUI、exec、MCP、app-server、plugin、sandbox（`frameworks/codex/codex-rs/cli/src/main.rs:704-832`） | Adapter-owned signal: Codex binary/entry discovery 与 command mapping 是 target-specific。Runtime-owned signal: generic lifecycle 不应假设 Codex subcommand。 |
| Config layer merge | `core::config_loader`、`core::config` | Requirements、system、user、project、runtime layers（`frameworks/codex/codex-rs/core/src/config_loader/mod.rs:99-124`） | Config-owned signal: declarative policy/profile data 与 trust layering。Adapter-owned signal: Codex-specific TOML translation。 |
| Session lifecycle | `core::session`、app-server client/server | Thread persistence、state DB、history metadata、auth、MCP、skills、AGENTS（`frameworks/codex/codex-rs/core/src/session/session.rs:300-415`） | Runtime-owned signal: start/resume/turn lifecycle 与 event wiring。State-owned signal: AVM session metadata references。 |
| Sandbox policy | `protocol`、`core::config`、`codex-sandboxing` | Approval、file-system、network、writable roots 是独立模型（`frameworks/codex/codex-rs/protocol/src/protocol.rs:939-1081`） | Runtime-owned signal: normalized isolation semantics。Adapter-owned signal: Codex flags/config/helper mapping。 |
| Shell execution | `core::tools`、`core::spawn` | Approval -> sandbox transform -> env-clean spawn（`frameworks/codex/codex-rs/core/src/tools/runtimes/shell.rs:134-289`、`frameworks/codex/codex-rs/core/src/spawn.rs:51-125`） | Runtime-owned signal: execution boundary。Adapter-owned signal: Codex concrete tool permissions 与 helper paths。 |
| Skills | `skills`、`core-skills`、plugin loader | Bundled cache 加 local/admin/repo/plugin roots（`frameworks/codex/codex-rs/core-skills/src/loader.rs:221-357`） | Package IO-owned signal: package skill directories。Adapter-owned signal: Codex root conventions 与 prompt injection behavior。 |
| MCP servers | `config`、`cli::mcp_cmd`、`codex-mcp`、`rmcp-client` | Global config 加 plugin MCP 加 skill dependency auto-install（`frameworks/codex/codex-rs/core/src/config/mod.rs:848-873`、`frameworks/codex/codex-rs/core/src/mcp_skill_dependencies.rs:75-126`） | Config owns declarative MCP records。Adapter owns Codex serialization、OAuth store 和 start/connect details。 |
| Plugins/apps/connectors | `core-plugins`、`plugin` | Plugin cache/marketplaces/manifest；plugin 贡献 skills/MCP/apps（`frameworks/codex/codex-rs/core-plugins/src/loader.rs:451-548`） | Package IO owns plugin bundle import/export。Adapter maps Codex plugin manifests 与 marketplaces。 |
| Rollout/session files | `rollout`、`thread-store`、`state` | `CODEX_HOME/sessions` 下 JSONL rollout，SQLite thread metadata（`frameworks/codex/codex-rs/rollout/src/recorder.rs:1320-1350`、`frameworks/codex/codex-rs/state/migrations/0001_threads.sql:1-25`） | State-owned signal: portable session indexes 与 references。Adapter-owned signal: Codex rollout 仍是 runtime-specific state。 |
| Memory artifacts | `core::memories`、`state::runtime::memories` | DB-backed stage1 rows 加 `CODEX_HOME/memories` files（`frameworks/codex/codex-rs/core/src/memories/mod.rs:105-122`） | State owns memory metadata policy。Adapter maps Codex memory root 与 writable-root side effect。 |
| Auth 与 tokens | `login`、`rmcp-client::oauth` | `auth.json`、keyring、ephemeral store、`.credentials.json`（`frameworks/codex/codex-rs/login/src/auth/storage.rs:29-61`、`frameworks/codex/codex-rs/rmcp-client/src/oauth.rs:1-18`） | Config/state-owned signal: secret references。Adapter-owned signal: token files/keyring entries 留在 portable packages 之外。 |

## 证据表

| 结论 | 源码证据 |
|---|---|
| Rust CLI 是 runtime 入口，不是 Node package scripts。 | `frameworks/codex/package.json:1-35`、`frameworks/codex/codex-rs/cli/src/main.rs:69-83`、`frameworks/codex/codex-rs/cli/src/main.rs:704-832` |
| 无 subcommand 路径启动 interactive TUI。 | `frameworks/codex/codex-rs/cli/src/main.rs:727-740`、`frameworks/codex/codex-rs/cli/src/main.rs:1490-1542` |
| Exec path 构建 config，并启动 in-process app-server client/thread。 | `frameworks/codex/codex-rs/exec/src/lib.rs:386-420`、`frameworks/codex/codex-rs/exec/src/lib.rs:493-512`、`frameworks/codex/codex-rs/exec/src/lib.rs:651-768` |
| Config layers 包括 system、user、project 和 runtime layers。 | `frameworks/codex/codex-rs/core/src/config_loader/mod.rs:99-124`、`frameworks/codex/codex-rs/core/src/config_loader/mod.rs:211-319`、`frameworks/codex/codex-rs/core/src/config_loader/mod.rs:930-1024` |
| Workspace/write policy 与 approval policy 分离。 | `frameworks/codex/codex-rs/protocol/src/protocol.rs:939-1081`、`frameworks/codex/codex-rs/core/src/config/mod.rs:1808-1932` |
| Workspace-write 可写 cwd/writable roots/tmp，但 writable roots 下默认 `.git`、`.agents`、`.codex` 只读。 | `frameworks/codex/codex-rs/protocol/src/protocol.rs:1180-1316` |
| Approval decisions 按 session 缓存，不持久化。 | `frameworks/codex/codex-rs/core/src/tools/sandboxing.rs:40-117` |
| Shell execution boundary 是 approval -> sandbox transform -> spawn。 | `frameworks/codex/codex-rs/core/src/tools/runtimes/shell.rs:134-289`、`frameworks/codex/codex-rs/core/src/spawn.rs:51-125` |
| Linux sandbox 使用 bubblewrap/seccomp/no_new_privs。 | `frameworks/codex/codex-rs/linux-sandbox/src/lib.rs:1-5`、`frameworks/codex/codex-rs/linux-sandbox/src/linux_run_main.rs:94-221` |
| Skills 是带 `SKILL.md` 的本地目录，并从多个 roots 加载。 | `frameworks/codex/codex-rs/core-skills/src/loader.rs:105-123`、`frameworks/codex/codex-rs/core-skills/src/loader.rs:221-357` |
| Bundled system skills 被 materialize 到 `CODEX_HOME/skills/.system`。 | `frameworks/codex/codex-rs/skills/src/lib.rs:10-55` |
| 显式 skill mentions 会把完整 `SKILL.md` 注入 context。 | `frameworks/codex/codex-rs/core-skills/src/injection.rs:31-85`、`frameworks/codex/codex-rs/core/src/context/skill_instructions.rs:22-32` |
| MCP servers 是 config records 与 plugin/skill-derived records。 | `frameworks/codex/codex-rs/config/src/mcp_types.rs:117-391`、`frameworks/codex/codex-rs/core/src/config/mod.rs:848-873`、`frameworks/codex/codex-rs/core/src/mcp_skill_dependencies.rs:75-126` |
| MCP connection manager 启动 clients 并缓存 app tools。 | `frameworks/codex/codex-rs/codex-mcp/src/mcp_connection_manager.rs:666-838`、`frameworks/codex/codex-rs/codex-mcp/src/mcp_connection_manager.rs:1424-1512`、`frameworks/codex/codex-rs/codex-mcp/src/mcp_connection_manager.rs:1617-1699` |
| Plugins 有独立 marketplace/cache/manifest 机制。 | `frameworks/codex/codex-rs/core-plugins/src/store.rs:29-54`、`frameworks/codex/codex-rs/core-plugins/src/manifest.rs:11-64`、`frameworks/codex/codex-rs/core-plugins/src/marketplace.rs:20-23` |
| AGENTS.md 从 global 与 project path 加载。 | `frameworks/codex/codex-rs/core/src/agents_md.rs:1-17`、`frameworks/codex/codex-rs/core/src/agents_md.rs:36-78`、`frameworks/codex/codex-rs/core/src/agents_md.rs:213-320` |
| Prompt history 是 `CODEX_HOME/history.jsonl`。 | `frameworks/codex/codex-rs/core/src/message_history.rs:1-17`、`frameworks/codex/codex-rs/core/src/message_history.rs:47-160` |
| Session rollout 是 `CODEX_HOME/sessions/YYYY/MM/DD` 下的 JSONL。 | `frameworks/codex/codex-rs/rollout/src/recorder.rs:1320-1350` |
| State 与 logs 是 `CODEX_HOME` 下独立 SQLite DB。 | `frameworks/codex/codex-rs/state/src/runtime.rs:95-156`、`frameworks/codex/codex-rs/state/src/runtime.rs:210-228` |
| Memory 同时是 DB-backed 和 filesystem-backed。 | `frameworks/codex/codex-rs/state/migrations/0006_memories.sql:1-31`、`frameworks/codex/codex-rs/core/src/memories/storage.rs:12-153` |
| Auth/token storage 包含 file、keyring 和 ephemeral modes。 | `frameworks/codex/codex-rs/login/src/auth/storage.rs:29-61`、`frameworks/codex/codex-rs/login/src/auth/storage.rs:135-332`、`frameworks/codex/codex-rs/rmcp-client/src/oauth.rs:1-18` |
| Model cache 是 `CODEX_HOME/models_cache.json`。 | `frameworks/codex/codex-rs/models-manager/src/manager.rs:23-24`、`frameworks/codex/codex-rs/models-manager/src/manager.rs:198-217` |
| 当前环境无法运行 Cargo verification。 | 尝试执行 `cargo metadata` 和 `cargo run ... -- --help` 的 shell 输出是：`zsh:1: command not found: cargo`。 |

## AVM PRD 风险

- PRD 若认为 Codex 只有一个 extension registry，在此源码树中是错误的。Skills、MCP servers、plugins、marketplaces、apps 和 skill dependency installs 有独立的加载与存储路径（`frameworks/codex/codex-rs/core-skills/src/loader.rs:221-357`、`frameworks/codex/codex-rs/core/src/config/mod.rs:848-873`、`frameworks/codex/codex-rs/core-plugins/src/store.rs:29-54`）。
- PRD 若认为 sandbox 是一个开关，是错误的。Codex 分离 approval、legacy sandbox policy、file-system sandbox policy、network policy、managed network、writable roots 和 platform helper transforms（`frameworks/codex/codex-rs/protocol/src/protocol.rs:939-1081`、`frameworks/codex/codex-rs/core/src/config/mod.rs:189-231`、`frameworks/codex/codex-rs/sandboxing/src/manager.rs:142-272`）。
- PRD 若认为 runtime permissions 都是 durable user state，只是部分成立。Configured permissions 是 durable，但 command approvals 是 per-session in-memory（`frameworks/codex/codex-rs/core/src/tools/sandboxing.rs:40-117`）。
- PRD 若认为 memory 只是被动 context，是不完整的。Codex 会创建 `CODEX_HOME/memories`，把它加入 writable roots，写 DB-backed memory artifacts，并可 spawn memory consolidation subagent（`frameworks/codex/codex-rs/core/src/config/mod.rs:1794-1801`、`frameworks/codex/codex-rs/core/src/memories/phase2.rs:140-155`）。
- PRD 若把 `CODEX_HOME` 当成简单 portable profile，风险较高。它包含 config、auth、OAuth credentials、history、rollouts、state DBs、logs、caches、skills、plugins、plugin temp data 和 memory artifacts（`frameworks/codex/codex-rs/core/src/config/mod.rs:461-475`、`frameworks/codex/codex-rs/login/src/auth/storage.rs:29-61`、`frameworks/codex/codex-rs/rollout/src/recorder.rs:1320-1350`、`frameworks/codex/codex-rs/state/src/runtime.rs:210-228`）。
- PRD 若认为 runtime state 可仅从 JSONL 推断，是错误的。Codex 使用 rollout JSONL、SQLite thread/log/memory/job tables，以及 session index JSONL（`frameworks/codex/codex-rs/state/migrations/0001_threads.sql:1-25`、`frameworks/codex/codex-rs/state/migrations/0006_memories.sql:1-31`、`frameworks/codex/codex-rs/rollout/src/session_index.rs:17-64`）。
- PRD 若想原样采用 Codex 隔离模型，只能部分验证。它是有价值的 baseline，因为 policies 是显式的；但 enforcement 依赖 Codex-specific helpers、platform features 和 config semantics（`frameworks/codex/codex-rs/linux-sandbox/src/linux_run_main.rs:30-91`、`frameworks/codex/codex-rs/sandboxing/src/policy_transforms.rs:631-651`）。

## 未决问题

- 本文没有识别本地 source checkout 的具体 version/commit。不同 Codex 版本的精确行为可能不同。
- 由于 PATH 中缺少 `cargo`，无法验证 Rust build/runtime help。启动结论来自源码追踪，不是 binary validation。
- Remote executor 和 cloud session 行为只在它们触及 exec/app-server/MCP 路径时做了追踪；完整 state 与 isolation model 需要单独调研。
- Windows sandboxing 已通过 helper modules 识别，但追踪深度不如 Linux sandboxing。
- Codex Apps/connectors 通过 plugin app loading 和 MCP tool cache 做了追踪，但 connector-specific auth 与 UI 行为没有穷尽追踪。
- Feature flags 可以启用或禁用 bundled skills、skill MCP dependency installation、memory tool、plugin marketplaces 和 sandbox modes 等实质行为。若要把这些路径视为必然行为，需要固定 feature-flag scope。
