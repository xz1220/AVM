# OpenClaw 运行时调研

## 摘要

本调研只追踪 `frameworks/openclaw` 源码事实，用于验证 Agent VM PRD 的运行时前提。

- OpenClaw 是 Node/TypeScript CLI。发布入口是 `openclaw.mjs`，`package.json` 将 `openclaw` bin 指向该文件；入口再加载构建产物 `dist/entry.js` 或 `dist/entry.mjs`。源码证据：`frameworks/openclaw/package.json:16`、`frameworks/openclaw/openclaw.mjs:191`、`frameworks/openclaw/openclaw.mjs:197`。
- `openclaw agent` 默认走 Gateway RPC，`--local` 才直接走本地 embedded agent；Gateway 失败时命令层会 fallback 到本地。源码证据：`frameworks/openclaw/src/cli/program/register.agent.ts:26`、`frameworks/openclaw/src/commands/agent-via-gateway.ts:189`、`frameworks/openclaw/src/commands/agent-via-gateway.ts:198`。
- Embedded agent 最终使用 `@mariozechner/pi-coding-agent` 的 session/agent runtime，构造模型、工具、skills、MCP、memory、workspace 后调用 `activeSession.prompt(...)` 启动 agent loop。源码证据：`frameworks/openclaw/src/agents/pi-embedded-runner/run/attempt.ts:1380`、`frameworks/openclaw/src/agents/pi-embedded-runner/run/attempt.ts:1540`、`frameworks/openclaw/src/agents/pi-embedded-runner/run/attempt.ts:2637`。
- 隔离不是默认强隔离。Docker sandbox 默认 `mode: "off"`；没有 sandbox 时 shell 默认落到 host/gateway，执行安全级别默认可到 `"full"`，且 approval 默认 `"off"`；文件写入工具在 `workspaceOnly=false` 时允许写 host 任意路径。源码证据：`frameworks/openclaw/src/agents/sandbox/config.ts:220`、`frameworks/openclaw/src/agents/exec-defaults.ts:91`、`frameworks/openclaw/src/agents/pi-tools.read.ts:757`。
- Skills 支持发现、加载、安装和环境变量注入，但本质是 prompt/files/install recipes，不是隔离的运行时模块。源码证据：`frameworks/openclaw/src/agents/skills/workspace.ts:533`、`frameworks/openclaw/src/agents/skills/workspace.ts:645`、`frameworks/openclaw/src/agents/skills-install.ts:457`。
- Plugins 支持 manifest 发现、安装、加载和运行时注册工具、hook、provider、gateway method、memory 等能力，且加载后进入进程级 registry/cache。源码证据：`frameworks/openclaw/src/plugins/roots.ts:16`、`frameworks/openclaw/src/plugins/registry.ts:1438`、`frameworks/openclaw/src/plugins/loader.ts:2131`。
- MCP 同时支持作为 client 连接外部 MCP server，并支持以 stdio server 暴露 OpenClaw built-in/plugin tools。配置型 MCP 支持 stdio/http/sse/streamable-http，bundle MCP runtime 当前按源码告警偏向 stdio。源码证据：`frameworks/openclaw/src/config/types.mcp.ts:1`、`frameworks/openclaw/src/agents/mcp-transport.ts:78`、`frameworks/openclaw/src/mcp/plugin-tools-serve.ts:2`、`frameworks/openclaw/src/plugins/loader.ts:2635`。
- 默认可变状态集中在 state dir，默认 `~/.openclaw`；workspace memory 主要在 workspace 内，但 auth、sessions、plugin index、cache、logs、OAuth credentials 是 state-dir 级别。源码证据：`frameworks/openclaw/src/config/paths.ts:55`、`frameworks/openclaw/src/config/sessions/paths.ts:10`、`frameworks/openclaw/src/config/paths.ts:227`。

安全验证命令结果：

- `node --version` 成功，版本 `v22.22.0`。
- `pnpm --dir frameworks/openclaw --version` 成功，版本 `10.33.0`。
- `node frameworks/openclaw/openclaw.mjs --help` 失败，原因是当前源码树缺少 `dist/entry.(m)js` 构建产物；这与入口源码的 missing dist 分支一致。源码证据：`frameworks/openclaw/openclaw.mjs:191`、`frameworks/openclaw/openclaw.mjs:197`。

## 运行时启动路径

### CLI 启动引导

1. NPM bin 入口是 `openclaw.mjs`。`package.json` 中 `bin.openclaw` 指向 `openclaw.mjs`，包类型是 ESM。源码证据：`frameworks/openclaw/package.json:16`、`frameworks/openclaw/package.json:55`。
2. `openclaw.mjs` 先检查 Node 版本，要求 `>=22.12.0`；帮助类参数有快速路径；正常路径加载 `dist/entry.js` 或 `dist/entry.mjs`，缺失时抛出构建产物错误。源码证据：`frameworks/openclaw/openclaw.mjs:8`、`frameworks/openclaw/openclaw.mjs:128`、`frameworks/openclaw/openclaw.mjs:191`、`frameworks/openclaw/openclaw.mjs:197`。
3. 源码入口 `src/entry.ts` 设置 process title、exec marker、只读 auth 初始化、profile/container 解析，然后调用 `runMainOrRootHelp`。源码证据：`frameworks/openclaw/src/entry.ts:45`、`frameworks/openclaw/src/entry.ts:67`、`frameworks/openclaw/src/entry.ts:100`。
4. `runMainOrRootHelp` 动态 import `./cli/run-main.js` 并调用 `runCli(argv)`。源码证据：`frameworks/openclaw/src/entry.ts:175`。
5. `run-main.ts` 从当前目录或 state dir 的 `.env` 加载环境变量，规范化 proxy/CLI path/runtime guard 后构造 Commander program。源码证据：`frameworks/openclaw/src/cli/run-main.ts:173`、`frameworks/openclaw/src/cli/run-main.ts:180`、`frameworks/openclaw/src/cli/run-main.ts:286`。
6. Commander program 注册 `agent`、`gateway`、`skills`、`plugins`、`mcp` 等子命令。源码证据：`frameworks/openclaw/src/cli/program/build-program.ts:9`、`frameworks/openclaw/src/cli/program/build-program.ts:16`。

### Agent 命令路径

1. `openclaw agent` 的描述明确为 “Run an agent turn via Gateway (use --local for embedded)”，并提供 `--local`、`--workspace`、`--session`、`--provider`、`--model` 等参数。源码证据：`frameworks/openclaw/src/cli/program/register.agent.ts:26`、`frameworks/openclaw/src/cli/program/register.agent.ts:44`、`frameworks/openclaw/src/cli/program/register.agent.ts:60`。
2. `agentCliCommand` 默认先调用 `agentCommandViaGateway`；如果 `--local` 被设置则直接调用 `agentCommand`；Gateway 路径报错后会 fallback 到本地执行。源码证据：`frameworks/openclaw/src/commands/agent-via-gateway.ts:189`、`frameworks/openclaw/src/commands/agent-via-gateway.ts:198`。
3. Gateway CLI 启动时解析 config、端口和 auth；非 loopback bind 在无 auth 且无 token bootstrap/trusted proxy 时会拒绝。源码证据：`frameworks/openclaw/src/cli/gateway-cli/run.ts:418`、`frameworks/openclaw/src/cli/gateway-cli/run.ts:561`、`frameworks/openclaw/src/cli/gateway-cli/run.ts:644`。
4. Gateway server 注册 core handlers，其中包含 `agentHandlers`；每个 RPC method 会经过 role/scope 授权，再在 plugin runtime request scope 内执行。源码证据：`frameworks/openclaw/src/gateway/server-methods.ts:73`、`frameworks/openclaw/src/gateway/server-methods.ts:110`。
5. `agent` RPC handler 解析 sender owner、model override 权限、session key、workspace、delivery policy、abort controller，然后调用 `agentCommandFromIngress`。源码证据：`frameworks/openclaw/src/gateway/server-methods/agent.ts:387`、`frameworks/openclaw/src/gateway/server-methods/agent.ts:442`、`frameworks/openclaw/src/gateway/server-methods/agent.ts:768`、`frameworks/openclaw/src/gateway/server-methods/agent.ts:1054`、`frameworks/openclaw/src/gateway/server-methods/agent.ts:1140`。
6. `agentCommandFromIngress` 要求调用方显式传入 `senderIsOwner` 和 `allowModelOverride`；本地命令路径则默认 trusted owner 且允许 model override。源码证据：`frameworks/openclaw/src/agents/agent-command.ts:1154`、`frameworks/openclaw/src/agents/agent-command.ts:1174`。

### Agent 准备与循环

1. `prepareAgentCommandExecution` 负责解析 agent runtime config、session、workspace、agentDir，并调用 `ensureAgentWorkspace`。源码证据：`frameworks/openclaw/src/agents/agent-command.ts:251`、`frameworks/openclaw/src/agents/agent-command.ts:295`、`frameworks/openclaw/src/agents/agent-command.ts:356`。
2. 新 session 或 stale session 会构建 workspace skill snapshot，并持久化进 session store。源码证据：`frameworks/openclaw/src/agents/agent-command.ts:591`、`frameworks/openclaw/src/agents/agent-command.ts:631`。
3. 模型解析包含默认 provider/model、allowlist/catalog 校验，以及仅在授权时接受显式 override。源码证据：`frameworks/openclaw/src/agents/agent-command.ts:679`、`frameworks/openclaw/src/agents/agent-command.ts:761`。
4. 执行层通过 `runWithModelFallback` 调用 `runAgentAttempt`；CLI provider 走 `runCliAgent`，否则走 `runEmbeddedPiAgent`。源码证据：`frameworks/openclaw/src/agents/agent-command.ts:901`、`frameworks/openclaw/src/agents/command/attempt-execution.ts:306`、`frameworks/openclaw/src/agents/command/attempt-execution.ts:426`。
5. `runEmbeddedPiAgent` 在每个 session 或全局 lane 中排队执行，加载 runtime plugins，解析 provider/model/plugin harness/auth profile，然后进入 attempt。源码证据：`frameworks/openclaw/src/agents/pi-embedded-runner/run.ts:239`、`frameworks/openclaw/src/agents/pi-embedded-runner/run.ts:253`、`frameworks/openclaw/src/agents/pi-embedded-runner/run.ts:287`、`frameworks/openclaw/src/agents/pi-embedded-runner/run.ts:317`、`frameworks/openclaw/src/agents/pi-embedded-runner/run.ts:421`。
6. `runEmbeddedAttempt` 创建 workspace/sandbox context、解析 skills、创建工具、materialize MCP/LSP bundled tools、构造 system prompt、打开 session manager、创建 Pi agent session、绑定 stream function，最后 `activeSession.prompt(...)` 提交用户 prompt。源码证据：`frameworks/openclaw/src/agents/pi-embedded-runner/run/attempt.ts:569`、`frameworks/openclaw/src/agents/pi-embedded-runner/run/attempt.ts:612`、`frameworks/openclaw/src/agents/pi-embedded-runner/run/attempt.ts:684`、`frameworks/openclaw/src/agents/pi-embedded-runner/run/attempt.ts:854`、`frameworks/openclaw/src/agents/pi-embedded-runner/run/attempt.ts:1037`、`frameworks/openclaw/src/agents/pi-embedded-runner/run/attempt.ts:1224`、`frameworks/openclaw/src/agents/pi-embedded-runner/run/attempt.ts:1380`、`frameworks/openclaw/src/agents/pi-embedded-runner/run/attempt.ts:1540`、`frameworks/openclaw/src/agents/pi-embedded-runner/run/attempt.ts:2637`。

## 隔离模型

### 工作区/root 边界

- Agent workspace 默认来自配置；没有配置时，main/default agent 使用默认 workspace，非默认 agent 使用 state dir 下 `workspace-${agentId}`。源码证据：`frameworks/openclaw/src/agents/agent-scope-config.ts:154`、`frameworks/openclaw/src/agents/workspace-default.ts:6`。
- 默认 workspace 路径是 `$HOME/.openclaw/workspace`，profile 模式会变成 `$HOME/.openclaw/workspace-${profile}`。源码证据：`frameworks/openclaw/src/agents/workspace-default.ts:6`、`frameworks/openclaw/src/agents/workspace-default.ts:12`。
- workspace 初始化会创建目录、bootstrap 文件和 `.openclaw/workspace-state.json`，必要时初始化 git。源码证据：`frameworks/openclaw/src/agents/workspace.ts:27`、`frameworks/openclaw/src/agents/workspace.ts:467`。
- runtime 对部分 workspace 文件读取使用 boundary open 和 max size 限制。源码证据：`frameworks/openclaw/src/agents/workspace.ts:54`、`frameworks/openclaw/src/infra/boundary-file-read.ts:69`、`frameworks/openclaw/src/infra/fs-safe.ts:207`。
- 但显式 `--workspace` 或 ingress workspace 可进入 session/run workspace 解析，源码层主要做路径解析和 session key 校验；没有发现 OpenClaw 对 host 目录的统一 allowlist。源码证据：`frameworks/openclaw/src/agents/workspace-run.ts:74`、`frameworks/openclaw/src/agents/workspace-run.ts:105`。

### 文件工具

- 文件工具策略默认 `workspaceOnly=false`；只有 config/global/agent 明确设为 true，读写工具才被 workspace root guard 约束。源码证据：`frameworks/openclaw/src/agents/tool-fs-policy.ts:11`、`frameworks/openclaw/src/agents/tool-fs-policy.ts:29`。
- `read` 工具只有在 `workspaceOnly` 生效时才包 workspace root guard；否则可按工具自身规则读 host path。源码证据：`frameworks/openclaw/src/agents/pi-tools.ts:456`、`frameworks/openclaw/src/agents/tool-fs-policy.ts:36`。
- host write/edit 工具在 `workspaceOnly=false` 时源码注释和实现都允许写 host 任意路径；`workspaceOnly=true` 时才调用 within-root 写入。源码证据：`frameworks/openclaw/src/agents/pi-tools.read.ts:751`、`frameworks/openclaw/src/agents/pi-tools.read.ts:757`、`frameworks/openclaw/src/agents/pi-tools.read.ts:785`。
- `apply_patch` 默认 workspace-only；sandbox read-only 时会禁用 apply_patch。源码证据：`frameworks/openclaw/src/agents/pi-tools.ts:428`、`frameworks/openclaw/src/agents/pi-tools.ts:543`。

### Shell/命令执行

- sandbox 配置默认 `mode: "off"`，backend 默认 Docker，workspace access 默认 `"none"`。源码证据：`frameworks/openclaw/src/agents/sandbox/config.ts:220`、`frameworks/openclaw/src/agents/sandbox/config.ts:240`。
- Docker backend 默认包含较强约束：read-only root、tmpfs、network none、drop capabilities、no-new-privileges 等。源码证据：`frameworks/openclaw/src/agents/sandbox/config.ts:101`、`frameworks/openclaw/src/agents/sandbox/docker.ts:391`。
- Docker bind mount 有安全校验，会阻止敏感 host 路径、危险 home 子路径、host network、container namespace join 等 footgun。源码证据：`frameworks/openclaw/src/agents/sandbox/validate-sandbox-security.ts:20`、`frameworks/openclaw/src/agents/sandbox/validate-sandbox-security.ts:39`、`frameworks/openclaw/src/agents/sandbox/validate-sandbox-security.ts:359`。
- 没有 sandbox 时，exec host 默认 `auto` 会解析到 gateway/host；安全级别默认可能是 `"full"`，approval 默认 `"off"`。源码证据：`frameworks/openclaw/src/agents/exec-defaults.ts:43`、`frameworks/openclaw/src/agents/exec-defaults.ts:91`、`frameworks/openclaw/src/agents/exec-defaults.ts:124`。
- host/gateway exec 仍有 allowlist/approval 机制，但这是配置和 gateway 层控制，不是 OS 级隔离。源码证据：`frameworks/openclaw/src/agents/bash-tools.exec-host-gateway.ts:95`、`frameworks/openclaw/src/agents/bash-tools.exec-host-gateway.ts:173`、`frameworks/openclaw/src/infra/exec-approvals.ts:169`。
- exec runtime 会为 sandbox 走 Docker exec，为 host 走 shell；host env 会过滤 `PATH` 等危险覆盖，但命令本身仍在 host 侧执行。源码证据：`frameworks/openclaw/src/agents/bash-tools.exec.ts:1536`、`frameworks/openclaw/src/agents/bash-tools.exec-runtime.ts:681`、`frameworks/openclaw/src/agents/bash-tools.exec-runtime.ts:708`。

### 工具权限与并发 agent

- Gateway RPC 根据 method 的 role/scope 授权；agent handler 还根据 sender owner 控制 model override。源码证据：`frameworks/openclaw/src/gateway/server-methods.ts:110`、`frameworks/openclaw/src/gateway/server-methods/agent.ts:442`。
- Tool policy 有 owner-only tool guard、非 owner filter、subagent deny list、leaf session deny list。源码证据：`frameworks/openclaw/src/agents/tool-policy.ts:22`、`frameworks/openclaw/src/agents/tool-policy.ts:54`、`frameworks/openclaw/src/agents/pi-tools.policy.ts:34`、`frameworks/openclaw/src/agents/pi-tools.policy.ts:49`。
- Embedded runner 通过 per-session/global lane 排队；session 写入有 write lock；process registry 的 scopeKey 使用 sessionKey 或 agent id 限制进程可见性。源码证据：`frameworks/openclaw/src/agents/pi-embedded-runner/run.ts:253`、`frameworks/openclaw/src/agents/pi-embedded-runner/run/attempt.ts:636`、`frameworks/openclaw/src/agents/pi-tools.ts:400`。
- 并发隔离因此主要是逻辑隔离：session queue、write lock、scopeKey、tool policy。没有证据显示默认每个 agent/task 都被自动放进独立 OS sandbox；sandbox 要配置开启。源码证据：`frameworks/openclaw/src/agents/sandbox/config.ts:220`、`frameworks/openclaw/src/agents/sandbox/runtime-status.ts:16`。

## Skill 安装与加载

### Skill 发现与 prompt 加载

- Skill loader 支持多个来源：managed `CONFIG_DIR/skills`、workspace `skills`、bundled、extraDirs、plugin skills、personal `~/.agents/skills`、project `<workspace>/.agents/skills`。源码证据：`frameworks/openclaw/src/agents/skills/workspace.ts:533`、`frameworks/openclaw/src/agents/skills/workspace.ts:561`。
- Precedence 是 extra < bundled < managed < personal < project < workspace。源码证据：`frameworks/openclaw/src/agents/skills/workspace.ts:579`。
- 单个 skill 必须有 `SKILL.md`，并通过 frontmatter 提供 name/description；loader 会限制 symlink、路径逃逸和最大文件大小。源码证据：`frameworks/openclaw/src/agents/skills/local-loader.ts:21`、`frameworks/openclaw/src/agents/skills/local-loader.ts:44`、`frameworks/openclaw/src/agents/skills/workspace.ts:268`。
- `skills.entries` 配置控制启用、bundled/runtime eligibility 等。源码证据：`frameworks/openclaw/src/agents/skills/config.ts:26`、`frameworks/openclaw/src/agents/skills/config.ts:58`、`frameworks/openclaw/src/agents/skills/config.ts:73`。
- Skill prompt 只把紧凑清单注入 system prompt，并指示模型在匹配任务时读取 `SKILL.md`，相对路径按 skill directory 解析。源码证据：`frameworks/openclaw/src/agents/skills/workspace.ts:645`。
- New/stale session 会保存 skill snapshot，后续可优先使用 session snapshot。源码证据：`frameworks/openclaw/src/agents/agent-command.ts:591`、`frameworks/openclaw/src/agents/skills/workspace.ts:814`。
- sandbox workspace 会同步 filtered skill files 到 sandbox 目标目录，过滤 `.git`、`node_modules` 等。源码证据：`frameworks/openclaw/src/agents/skills/workspace.ts:910`。

### Skill 安装

- `openclaw skills` CLI 支持 search/install/update/list/info/check；install 从 ClawHub slug 安装到 active workspace。源码证据：`frameworks/openclaw/src/cli/skills-cli.ts:52`、`frameworks/openclaw/src/cli/skills-cli.ts:93`。
- ClawHub 安装目标是 `<workspace>/skills/<slug>`，使用 `.clawhub` origin/lock 文件跟踪。源码证据：`frameworks/openclaw/src/agents/skills-clawhub.ts:18`、`frameworks/openclaw/src/agents/skills-clawhub.ts:122`、`frameworks/openclaw/src/agents/skills-clawhub.ts:144`。
- Skill frontmatter 支持 `install` spec，包括 brew/node/go/uv/download；download URL 仅接受 http/https。源码证据：`frameworks/openclaw/src/agents/skills/frontmatter.ts:93`、`frameworks/openclaw/src/agents/skills/frontmatter.ts:112`。
- `installSkill` 会解析 skill、扫描 install source、提示 untrusted、下载依赖、构造并执行安装命令。源码证据：`frameworks/openclaw/src/agents/skills-install.ts:457`、`frameworks/openclaw/src/agents/skills-install.ts:503`、`frameworks/openclaw/src/agents/skills-install.ts:549`。
- Download 型安装默认落在 per-skill tools root，且拒绝写到 root 外。源码证据：`frameworks/openclaw/src/agents/skills/tools-dir.ts:7`、`frameworks/openclaw/src/agents/skills-install-download.ts:31`。
- Skill env override 是进程级环境变量注入，带 acquire/release reverter，并屏蔽一组危险变量。源码证据：`frameworks/openclaw/src/agents/skills/env-overrides.ts:24`、`frameworks/openclaw/src/agents/skills/env-overrides.ts:37`、`frameworks/openclaw/src/agents/skills/env-overrides.ts:83`。

### 插件与工具 registry

- Plugin roots 包括 bundled stock、global `<configDir>/extensions`、workspace `<workspace>/.openclaw/extensions`，也可以从 config 加载额外路径。源码证据：`frameworks/openclaw/src/plugins/roots.ts:16`、`frameworks/openclaw/src/plugins/roots.ts:29`。
- Manifest registry origin precedence 是 config > workspace > global > bundled。源码证据：`frameworks/openclaw/src/plugins/manifest-registry.ts:91`。
- Plugin loader 会发现 manifest、计算 enable/startup/compat、加载 setup/runtime entry，并激活 runtime registry。源码证据：`frameworks/openclaw/src/plugins/loader.ts:2311`、`frameworks/openclaw/src/plugins/loader.ts:2788`、`frameworks/openclaw/src/plugins/loader.ts:2131`。
- Plugin API 可以注册 tool、hook、provider、agent harness、detached task runtime、gateway method、media/web/search 能力等。源码证据：`frameworks/openclaw/src/plugins/registry.ts:372`、`frameworks/openclaw/src/plugins/registry.ts:404`、`frameworks/openclaw/src/plugins/registry.ts:1438`。
- Plugin install 支持 archive/package，安装前有 security scan，并可通过 `--dangerously-force-unsafe-install` 覆盖部分安全拦截。源码证据：`frameworks/openclaw/src/plugins/install.ts:39`、`frameworks/openclaw/src/plugins/install.ts:769`、`frameworks/openclaw/src/cli/plugins-cli.ts:760`。
- Plugin install index 持久化在 state dir 下 `plugins/installs.json`。源码证据：`frameworks/openclaw/src/plugins/installed-plugin-index-store-path.ts:4`、`frameworks/openclaw/src/plugins/installed-plugin-index-store-path.ts:20`。

## MCP 安装与加载

### MCP 客户端侧

- Config 模型支持 `mcp.servers`；每个 server 可配置 stdio `command/args/env/cwd` 或 http `url/transport/headers`。源码证据：`frameworks/openclaw/src/config/types.mcp.ts:1`、`frameworks/openclaw/src/config/types.mcp.ts:9`。
- MCP config CLI/manager 支持 list/set/unset configured servers。源码证据：`frameworks/openclaw/src/config/mcp-config.ts:29`、`frameworks/openclaw/src/config/mcp-config.ts:59`、`frameworks/openclaw/src/config/mcp-config.ts:105`。
- Agent run 会把 configured MCP 与 bundle MCP 默认配置合并，configured server 覆盖 bundle 默认值。源码证据：`frameworks/openclaw/src/agents/bundle-mcp-config.ts:49`。
- Transport 解析优先 stdio，然后 http/sse/streamable-http；unsupported transport 会产生 warning。源码证据：`frameworks/openclaw/src/agents/mcp-transport-config.ts:95`。
- Stdio MCP 会 spawn 配置中的 command，使用给定 args/env/cwd。源码证据：`frameworks/openclaw/src/agents/mcp-stdio.ts:14`、`frameworks/openclaw/src/agents/mcp-stdio-transport.ts:27`。
- HTTP MCP 接受 http/https URL 和 headers。源码证据：`frameworks/openclaw/src/agents/mcp-http.ts:19`。
- Session MCP runtime 按 server 遍历连接，`listTools` 后生成 catalog；tool 调用时通过 MCP client `callTool`。源码证据：`frameworks/openclaw/src/agents/pi-bundle-mcp-runtime.ts:181`、`frameworks/openclaw/src/agents/pi-bundle-mcp-runtime.ts:228`、`frameworks/openclaw/src/agents/pi-bundle-mcp-runtime.ts:351`。
- MCP runtime manager 按 session/config fingerprint 复用 runtime，并有 idle TTL、dispose/retire 生命周期。源码证据：`frameworks/openclaw/src/agents/pi-bundle-mcp-runtime.ts:483`、`frameworks/openclaw/src/agents/pi-bundle-mcp-runtime.ts:580`。
- MCP catalog tools 在 agent attempt 中被 materialize 成 agent tools，并进入同一套 tool policy 过滤。源码证据：`frameworks/openclaw/src/agents/pi-bundle-mcp-materialize.ts:64`、`frameworks/openclaw/src/agents/pi-embedded-runner/run/attempt.ts:854`、`frameworks/openclaw/src/agents/pi-embedded-runner/run/attempt.ts:905`。

### MCP 服务端侧

- OpenClaw 可以作为 stdio MCP server 暴露 selected built-in tools。源码证据：`frameworks/openclaw/src/mcp/openclaw-tools-serve.ts:2`。
- OpenClaw 也可以作为 stdio MCP server 暴露 plugin-registered tools；handler 会过滤 `ownerOnly` tool，并以 `mcp-${Date.now()}` 作为执行 id 调用工具。源码证据：`frameworks/openclaw/src/mcp/plugin-tools-serve.ts:2`、`frameworks/openclaw/src/mcp/plugin-tools-handlers.ts:21`、`frameworks/openclaw/src/mcp/plugin-tools-handlers.ts:53`。
- MCP stdio server stdout 只用于协议输出。源码证据：`frameworks/openclaw/src/mcp/tools-stdio-server.ts:9`、`frameworks/openclaw/src/mcp/tools-stdio-server.ts:24`。

### Bundle/plugin MCP

- Plugin bundle capability 包含 `skills`、`mcpServers`、settings、commands、agents、outputStyles、lspServers、hooks 等。源码证据：`frameworks/openclaw/src/plugins/loader.ts:2603`。
- Loader 对 bundle MCP runtime support 做检查，并对 unsupported/incomplete configs 产生 diagnostics；源码信息显示 bundle MCP runtime 仍偏向 stdio 支持。源码证据：`frameworks/openclaw/src/plugins/loader.ts:2635`。

## 记忆与状态存储

### 状态与配置根目录

- `resolveStateDir` 用于 sessions/logs/caches 等可变状态；优先 `OPENCLAW_STATE_DIR`，默认 `~/.openclaw`，兼容 legacy `.clawdbot`。源码证据：`frameworks/openclaw/src/config/paths.ts:55`。
- Canonical config path 默认是 `<stateDir>/openclaw.json`，可被 `OPENCLAW_CONFIG_PATH` 覆盖。源码证据：`frameworks/openclaw/src/config/paths.ts:101`。
- `resolveConfigDir` 也会受 `OPENCLAW_STATE_DIR` 和 `OPENCLAW_CONFIG_PATH` 影响，默认 `~/.openclaw`。源码证据：`frameworks/openclaw/src/utils.ts:130`。
- Agent dir 默认是 `<stateDir>/agents/main/agent`，可由 `OPENCLAW_AGENT_DIR` 或 `PI_CODING_AGENT_DIR` 覆盖。源码证据：`frameworks/openclaw/src/agents/agent-paths.ts:6`。

### 会话、转录与认证

- Session store 默认在 `<stateDir>/agents/<agentId>/sessions/sessions.json`，session transcript 在同一 sessions 目录下。源码证据：`frameworks/openclaw/src/config/sessions/paths.ts:10`、`frameworks/openclaw/src/config/sessions/paths.ts:35`、`frameworks/openclaw/src/config/sessions/paths.ts:240`。
- Session file 解析有 safe id regex 和 within-dir 检查，并保留兼容路径处理。源码证据：`frameworks/openclaw/src/config/sessions/paths.ts:62`、`frameworks/openclaw/src/config/sessions/paths.ts:176`。
- Session store 写入会在 file lock 下更新，并用 atomic write、0600 mode 持久化；还包括 prune/cap/disk budget/archive/rotate 维护。源码证据：`frameworks/openclaw/src/config/sessions/store.ts:232`、`frameworks/openclaw/src/config/sessions/store.ts:411`、`frameworks/openclaw/src/config/sessions/store.ts:493`。
- Transcript header 记录 session version、cwd、创建时间等，写入模式是 0600。源码证据：`frameworks/openclaw/src/config/sessions/transcript.ts:19`。
- Auth profile/state 文件在 agentDir 下；OAuth credentials 默认在 `<stateDir>/credentials/oauth.json`。源码证据：`frameworks/openclaw/src/agents/auth-profiles/path-resolve.ts:12`、`frameworks/openclaw/src/config/paths.ts:227`。
- Auth store 有进程级 cache，按路径和 mtime 复用。源码证据：`frameworks/openclaw/src/agents/auth-profiles/store.ts:73`、`frameworks/openclaw/src/agents/auth-profiles/store.ts:197`。
- Persisted auth 会把 secret-backed field 规范化为 refs，并移除 raw value。源码证据：`frameworks/openclaw/src/agents/auth-profiles/persisted.ts:95`。

### 记忆

- Workspace root memory 文件是 `MEMORY.md`，legacy `memory.md` 和 repair auxiliary 有特殊处理；canonical file 必须是真实文件且不是 symlink。源码证据：`frameworks/openclaw/src/memory/root-memory-files.ts:4`、`frameworks/openclaw/src/memory/root-memory-files.ts:33`。
- 默认 QMD memory collections 来自 `<workspace>/MEMORY.md` 和 `<workspace>/memory/**/*.md`。源码证据：`frameworks/openclaw/src/memory-host-sdk/host/backend-config.ts:329`。
- Memory backend config 可从 agent workspace、config extra paths/collections、sessions 组合。源码证据：`frameworks/openclaw/src/memory-host-sdk/host/backend-config.ts:350`。
- Memory event log 默认相对 workspace 路径是 `memory/.dreams/events.jsonl`。源码证据：`frameworks/openclaw/src/memory-host-sdk/events.ts:5`。
- Memory plugin state 是进程级全局对象，注册 corpus/capability 并持有 memory search managers。源码证据：`frameworks/openclaw/src/plugins/memory-state.ts:159`、`frameworks/openclaw/src/plugins/memory-state.ts:202`。

### 日志、临时文件、缓存与插件状态

- 默认日志目录是 OpenClaw preferred tmp dir，默认文件 `openclaw.log`；可由 config/env 覆盖，支持 rotate/prune/redact，logger 本身是进程级 cache。源码证据：`frameworks/openclaw/src/logging/logger.ts:42`、`frameworks/openclaw/src/logging/logger.ts:336`、`frameworks/openclaw/src/logging/logger.ts:401`、`frameworks/openclaw/src/logging/logger.ts:474`。
- Preferred tmp dir 优先 `/tmp/openclaw`，否则按 uid 创建 secure fallback，并要求非 symlink、user-owned、非 group-writable、0700。源码证据：`frameworks/openclaw/src/infra/tmp-openclaw-dir.ts:5`、`frameworks/openclaw/src/infra/tmp-openclaw-dir.ts:33`、`frameworks/openclaw/src/infra/tmp-openclaw-dir.ts:75`。
- Cache trace 是 opt-in，默认写 `<stateDir>/logs/cache-trace.jsonl`，writer map 是进程级。源码证据：`frameworks/openclaw/src/agents/cache-trace.ts:81`。
- Bootstrap snapshot cache 是进程内 map，按 sessionKey 维护。源码证据：`frameworks/openclaw/src/agents/bootstrap-cache.ts:3`。
- OpenRouter model capability cache 有内存层和 `<stateDir>/cache/openrouter-models.json` 磁盘层。源码证据：`frameworks/openclaw/src/agents/pi-embedded-runner/openrouter-model-capabilities.ts:1`、`frameworks/openclaw/src/agents/pi-embedded-runner/openrouter-model-capabilities.ts:82`、`frameworks/openclaw/src/agents/pi-embedded-runner/openrouter-model-capabilities.ts:120`。
- Plugin install index 默认 `<stateDir>/plugins/installs.json`，plugin loader 会缓存并激活 runtime registry。源码证据：`frameworks/openclaw/src/plugins/installed-plugin-index-store-path.ts:4`、`frameworks/openclaw/src/plugins/installed-plugin-index-store-path.ts:20`、`frameworks/openclaw/src/plugins/loader.ts:3148`。

## 数据边界矩阵

| 数据/能力 | 默认位置或边界 | OpenClaw 自身约束 | 对 AVM 边界的事实含义 |
| --- | --- | --- | --- |
| CLI 入口 | `openclaw.mjs` -> `dist/entry` | 未构建源码树无法直接 help | AVM adapter 需要处理源码树和发布包差异；证据：`frameworks/openclaw/openclaw.mjs:191` |
| Config | `<stateDir>/openclaw.json` | 可被 env 覆盖 | AVM 需要注入 per-VM state/config env；证据：`frameworks/openclaw/src/config/paths.ts:101` |
| Workspace | 默认 `~/.openclaw/workspace` | 部分读文件有 boundary open；显式 workspace 可指定 | AVM 不能只依赖默认 workspace；证据：`frameworks/openclaw/src/agents/workspace-default.ts:6`、`frameworks/openclaw/src/agents/workspace-run.ts:74` |
| Agent 目录 | `<stateDir>/agents/main/agent` | 可 env 覆盖 | Adapter 需要显式绑定 agentDir；证据：`frameworks/openclaw/src/agents/agent-paths.ts:6` |
| Sessions | `<stateDir>/agents/<agentId>/sessions` | safe id、lock、atomic write | 可由 runtime adapter 托管/迁移；证据：`frameworks/openclaw/src/config/sessions/paths.ts:10` |
| Transcript | session 目录 JSONL | 0600 mode、session file 解析 | 需要纳入 VM 生命周期清理；证据：`frameworks/openclaw/src/config/sessions/transcript.ts:19` |
| Auth | agentDir auth files + `<stateDir>/credentials/oauth.json` | secret refs；进程 cache | 需要 per-VM credentials/state 隔离；证据：`frameworks/openclaw/src/config/paths.ts:227` |
| 文件工具 | host path 或 workspace guard | `workspaceOnly` 默认 false | AVM runtime 需要统一兜底文件边界；证据：`frameworks/openclaw/src/agents/tool-fs-policy.ts:11` |
| Shell 执行 | host/gateway 或 sandbox | sandbox 默认 off；approval 默认 off | AVM 不能默认信任 OpenClaw exec policy；证据：`frameworks/openclaw/src/agents/sandbox/config.ts:220`、`frameworks/openclaw/src/agents/exec-defaults.ts:124` |
| Docker sandbox | Docker container | 强约束在启用后生效 | AVM 可调用但不能假定默认启用；证据：`frameworks/openclaw/src/agents/sandbox/docker.ts:391` |
| Skills | bundled/global/personal/project/workspace/plugin | 文件 loader 有路径/大小防护；install 可执行包管理命令 | Adapter 需要控制 skill roots/install；证据：`frameworks/openclaw/src/agents/skills/workspace.ts:533`、`frameworks/openclaw/src/agents/skills-install.ts:457` |
| Plugins | bundled/global/workspace/config roots | runtime registry 进程级激活 | Adapter 需要控制 plugin roots和启用列表；证据：`frameworks/openclaw/src/plugins/roots.ts:16`、`frameworks/openclaw/src/plugins/loader.ts:2131` |
| MCP client | config/bundle servers | stdio 可 spawn command；HTTP 可带 headers | AVM 需要 MCP allowlist/credentials/env 策略；证据：`frameworks/openclaw/src/agents/mcp-stdio-transport.ts:27` |
| MCP server | stdio 暴露 OpenClaw/plugin tools | plugin tools 会过滤 ownerOnly | 作为外部 tool provider 时需审计可暴露工具；证据：`frameworks/openclaw/src/mcp/plugin-tools-handlers.ts:21` |
| Memory 文件 | workspace `MEMORY.md`/`memory/**/*.md` | canonical memory file 拒绝 symlink | workspace 内存较好隔离，但 extra paths/config 可扩展边界；证据：`frameworks/openclaw/src/memory-host-sdk/host/backend-config.ts:350` |
| Logs/tmp/cache | tmp dir + state logs/cache | tmp dir 做安全检查；logger/cache 是进程级 | VM 生命周期需清理 tmp/state；证据：`frameworks/openclaw/src/infra/tmp-openclaw-dir.ts:75` |

## 证据表

| 结论 | 源码证据 |
| --- | --- |
| CLI bin 是 `openclaw.mjs` | `frameworks/openclaw/package.json:16` |
| 入口要求 Node >= 22.12 | `frameworks/openclaw/openclaw.mjs:8` |
| 未构建源码树缺少 dist 会失败 | `frameworks/openclaw/openclaw.mjs:191`、`frameworks/openclaw/openclaw.mjs:197` |
| `agent` 默认 Gateway，`--local` embedded | `frameworks/openclaw/src/cli/program/register.agent.ts:26`、`frameworks/openclaw/src/commands/agent-via-gateway.ts:189` |
| Gateway agent handler 负责 auth/session/workspace/dispatch | `frameworks/openclaw/src/gateway/server-methods/agent.ts:387`、`frameworks/openclaw/src/gateway/server-methods/agent.ts:1140` |
| Embedded attempt 构造 Pi agent session 并 prompt | `frameworks/openclaw/src/agents/pi-embedded-runner/run/attempt.ts:1380`、`frameworks/openclaw/src/agents/pi-embedded-runner/run/attempt.ts:2637` |
| Workspace 默认在 `~/.openclaw/workspace` | `frameworks/openclaw/src/agents/workspace-default.ts:6` |
| `workspaceOnly` 默认 false | `frameworks/openclaw/src/agents/tool-fs-policy.ts:11` |
| Host write/edit 可在非 workspaceOnly 时写任意 host path | `frameworks/openclaw/src/agents/pi-tools.read.ts:757` |
| Sandbox 默认 off | `frameworks/openclaw/src/agents/sandbox/config.ts:220` |
| Docker sandbox 启用后有 read-only root/network none/cap drop | `frameworks/openclaw/src/agents/sandbox/docker.ts:391` |
| Exec 默认 host auto/security/approval 行为是配置驱动 | `frameworks/openclaw/src/agents/exec-defaults.ts:43`、`frameworks/openclaw/src/agents/exec-defaults.ts:91`、`frameworks/openclaw/src/agents/exec-defaults.ts:124` |
| Tool policy 区分 owner/non-owner/subagent | `frameworks/openclaw/src/agents/tool-policy.ts:22`、`frameworks/openclaw/src/agents/pi-tools.policy.ts:34` |
| Skills 多 root 发现并按 precedence 合并 | `frameworks/openclaw/src/agents/skills/workspace.ts:533`、`frameworks/openclaw/src/agents/skills/workspace.ts:579` |
| Skill install 可执行 package/download recipes | `frameworks/openclaw/src/agents/skills/frontmatter.ts:112`、`frameworks/openclaw/src/agents/skills-install.ts:457` |
| Plugins 可注册工具/hook/provider/gateway method | `frameworks/openclaw/src/plugins/registry.ts:372`、`frameworks/openclaw/src/plugins/registry.ts:1438` |
| Plugin roots 包含 global/workspace/bundled/config | `frameworks/openclaw/src/plugins/roots.ts:16`、`frameworks/openclaw/src/plugins/roots.ts:29` |
| MCP config 支持 stdio/http server | `frameworks/openclaw/src/config/types.mcp.ts:1` |
| MCP stdio 会 spawn configured command | `frameworks/openclaw/src/agents/mcp-stdio-transport.ts:27` |
| MCP tools materialize 成 agent tools | `frameworks/openclaw/src/agents/pi-bundle-mcp-materialize.ts:64` |
| OpenClaw 可作为 MCP stdio server 暴露 built-in/plugin tools | `frameworks/openclaw/src/mcp/openclaw-tools-serve.ts:2`、`frameworks/openclaw/src/mcp/plugin-tools-serve.ts:2` |
| State dir 默认 `~/.openclaw` | `frameworks/openclaw/src/config/paths.ts:55` |
| Session store/transcript 在 per-agent sessions dir | `frameworks/openclaw/src/config/sessions/paths.ts:10`、`frameworks/openclaw/src/config/sessions/paths.ts:240` |
| OAuth credentials 默认 state credentials | `frameworks/openclaw/src/config/paths.ts:227` |
| Workspace memory 默认 `MEMORY.md`/`memory/**/*.md` | `frameworks/openclaw/src/memory/root-memory-files.ts:4`、`frameworks/openclaw/src/memory-host-sdk/host/backend-config.ts:329` |
| Plugin/memory/auth/cache 存在进程级状态 | `frameworks/openclaw/src/plugins/loader.ts:3148`、`frameworks/openclaw/src/plugins/memory-state.ts:202`、`frameworks/openclaw/src/agents/auth-profiles/store.ts:73`、`frameworks/openclaw/src/agents/bootstrap-cache.ts:3` |

## AVM PRD 风险

这些不是产品建议，而是 PRD 需要承认或验证的源码事实。

1. AVM 不能把 OpenClaw 默认 workspace 当作隔离边界。默认 workspace 在用户 home 的 `~/.openclaw/workspace`，并且 host file tools 默认不是 workspace-only。证据：`frameworks/openclaw/src/agents/workspace-default.ts:6`、`frameworks/openclaw/src/agents/tool-fs-policy.ts:11`、`frameworks/openclaw/src/agents/pi-tools.read.ts:757`。
2. AVM 不能依赖 OpenClaw 默认 sandbox 实现隔离。sandbox 默认 off；只有显式配置后 Docker sandbox 的安全属性才生效。证据：`frameworks/openclaw/src/agents/sandbox/config.ts:220`、`frameworks/openclaw/src/agents/sandbox/docker.ts:391`。
3. AVM 不能默认允许 OpenClaw host exec。没有 sandbox 时 exec 会走 host/gateway，approval 默认 off，安全策略是配置驱动。证据：`frameworks/openclaw/src/agents/exec-defaults.ts:91`、`frameworks/openclaw/src/agents/exec-defaults.ts:124`、`frameworks/openclaw/src/agents/bash-tools.exec-runtime.ts:708`。
4. Skill 安装、plugin 安装、MCP stdio 都可能执行 host command 或加载进程内代码。证据：`frameworks/openclaw/src/agents/skills-install.ts:457`、`frameworks/openclaw/src/plugins/loader.ts:2788`、`frameworks/openclaw/src/agents/mcp-stdio-transport.ts:27`。
5. OpenClaw 有多个全局或 state-dir 级数据面：auth store cache、plugin registry、memory plugin state、bootstrap cache、OpenRouter cache、plugin install index。证据：`frameworks/openclaw/src/agents/auth-profiles/store.ts:73`、`frameworks/openclaw/src/plugins/memory-state.ts:202`、`frameworks/openclaw/src/agents/bootstrap-cache.ts:3`、`frameworks/openclaw/src/plugins/installed-plugin-index-store-path.ts:4`。
6. AVM adapter 需要抽象的行为边界包括：CLI/Gateway/local invocation、state/config/agentDir/workspace env injection、model/auth profile mapping、session store/transcript mapping、tool policy mapping、sandbox/exec backend mapping、skills/plugins/MCP roots 与 install/loading 生命周期。证据分别见 `frameworks/openclaw/src/commands/agent-via-gateway.ts:189`、`frameworks/openclaw/src/config/paths.ts:55`、`frameworks/openclaw/src/agents/agent-command.ts:679`、`frameworks/openclaw/src/config/sessions/paths.ts:10`、`frameworks/openclaw/src/agents/pi-tools.ts:673`、`frameworks/openclaw/src/agents/sandbox/config.ts:220`、`frameworks/openclaw/src/agents/skills/workspace.ts:533`。
7. AVM runtime 层需要统一兜底的行为包括：per-VM HOME/state/config/agentDir/workspace、host fs deny-by-default、host exec deny-by-default、MCP stdio allowlist、plugin/skill install gate、tmp/cache/log cleanup。源码原因是这些能力在 OpenClaw 里分散在 config、tools、sandbox、plugins、skills、MCP，而非一个统一强制边界。证据：`frameworks/openclaw/src/config/paths.ts:55`、`frameworks/openclaw/src/agents/tool-fs-policy.ts:11`、`frameworks/openclaw/src/agents/exec-defaults.ts:91`、`frameworks/openclaw/src/plugins/roots.ts:16`、`frameworks/openclaw/src/agents/mcp-stdio-transport.ts:27`。
8. 不能依赖 OpenClaw 自身实现的隔离能力包括：默认 OS sandbox、默认 workspace-only 文件访问、默认禁止 host shell、默认禁止 personal/global skills、默认禁止 workspace/global plugins、默认禁止 MCP stdio spawn。证据：`frameworks/openclaw/src/agents/sandbox/config.ts:220`、`frameworks/openclaw/src/agents/tool-fs-policy.ts:11`、`frameworks/openclaw/src/agents/exec-defaults.ts:124`、`frameworks/openclaw/src/agents/skills/workspace.ts:561`、`frameworks/openclaw/src/plugins/roots.ts:16`、`frameworks/openclaw/src/agents/mcp-stdio-transport.ts:27`。

## 未决问题

1. 当前 PRD 是否要求 AVM 在 OpenClaw runtime 内使用 Gateway 作为常驻服务，还是每个 VM/task 使用 `--local` 短进程？源码支持两条路径，但 session 生命周期、auth surface 和并发模型不同。证据：`frameworks/openclaw/src/commands/agent-via-gateway.ts:189`、`frameworks/openclaw/src/gateway/server-methods/agent.ts:1054`。
2. AVM 是否允许 OpenClaw 使用 Docker sandbox，还是必须由 AVM 自己提供外层 sandbox？源码显示 OpenClaw Docker sandbox 可以加强隔离，但默认 off 且由 OpenClaw config 驱动。证据：`frameworks/openclaw/src/agents/sandbox/config.ts:220`、`frameworks/openclaw/src/agents/sandbox/docker.ts:391`。
3. AVM 是否要支持 OpenClaw skills/plugins/MCP 完整生态，还是只允许预安装白名单？源码显示三者都有 host execution 或 in-process loading surface。证据：`frameworks/openclaw/src/agents/skills-install.ts:457`、`frameworks/openclaw/src/plugins/loader.ts:2788`、`frameworks/openclaw/src/agents/mcp-stdio-transport.ts:27`。
4. AVM 需要怎样处理已有用户 `~/.openclaw` 数据？源码有 legacy/compat 路径和默认 global state；若 AVM 改用 per-VM state，需要明确迁移或隔离策略。证据：`frameworks/openclaw/src/config/paths.ts:55`、`frameworks/openclaw/src/config/sessions/paths.ts:176`。
