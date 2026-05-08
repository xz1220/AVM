# Codex 运行时源码调研

本文基于 `frameworks/codex` 源码，目标只有一个：判断 Agent VM PRD 里关于 runtime 的假设是否成立。行号引用为调研当时的本地 checkout。

## 一、核心结论（摘要）

1. Codex runtime 是 `frameworks/codex/codex-rs` 下的 Rust workspace。顶层 `package.json` 只是 formatting/schema 脚本，不是运行入口。真正的入口是 Rust 二进制 `codex`，子命令包含 TUI、exec、mcp-server、app-server、plugin、login、cloud 等（`codex-rs/cli/src/main.rs:69-172`、`704-832`）。

2. Codex 的 **配置是分层合并** 的：requirement → 系统 `/etc/codex/config.toml` → 用户 `${CODEX_HOME}/config.toml` → 项目 `.codex/config.toml` → 运行时 CLI flags。AVM 想"写入 runtime managed config"是可以落在用户或项目层的，这条路径是通的（`codex-rs/core/src/config_loader/mod.rs:99-124`）。

3. Codex **没有统一的 extension registry**。Skills、MCP、plugin、app 各有独立的发现路径和存储位置。PRD 所说的"全量发现"在 Codex 这里是**多源合并**，不是单表查询。

4. Codex 的状态边界是 `CODEX_HOME`（默认 `~/.codex`）。里面同时混了：config、auth、OAuth credentials、history.jsonl、sessions/*.jsonl rollout、SQLite state DB、SQLite logs DB、skills cache、plugins cache、memory artifacts、model cache。PRD 要求的 per-(Agent, runtime) 私有 boundary 在 Codex 上的自然落地就是**每个 Agent 一个独立 `CODEX_HOME`**，配置和运行状态都一起分家。

5. 命令级 approval 是 **per-session 内存缓存**，不持久化；但 sandbox policy、MCP 配置等是 durable。PRD 说"不管 memory 但要隔离运行状态"——这句里的"运行状态"要区分持久的（config/rollout/DB）和短暂的（approval cache）。

**验证局限**：当前环境没有 `cargo`，只做了源码追踪，没跑 `cargo run -- --help` 或 build。

---

## 二、PRD 可行性对照

按 PRD 的模块逐条对齐。"可行"指 Codex 有对应能力让 AVM 接上；"有坑"指能做但要处理额外语义；"不可行"指 Codex 结构上不支持。

| PRD 诉求 | Codex 现状 | 判断 |
|---|---|---|
| **AVM 把 Agent 渲染为 runtime managed config** | 用户 config 在 `${CODEX_HOME}/config.toml`，项目 config 在 `.codex/config.toml`，都是 TOML。AVM 可以写入。 | **可行**。 |
| **Agent 绑定 instructions** | Codex 有 `AGENTS.md`（从 `CODEX_HOME` 和项目树发现）+ developer instructions。AVM 可以 map 到 AGENTS.md 或直接注入 config。 | **可行**。 |
| **Agent 绑定 skills（含来源区分）** | Skills 从 6 个 root 发现：项目 `.codex/skills`、`CODEX_HOME/skills`、user `~/.agents/skills`、bundled system、admin `/etc/codex/skills`、repo `.agents/skills`、configured extras、plugin roots。每个 skill 在 loader 里带 scope rank，plugin skill 还会加 namespace。 | **可行**。Codex 提供的 scope rank + plugin namespace 正好满足 PRD 4.2 对来源区分的要求。 |
| **Agent 绑定 MCP servers（含来源区分）** | MCP 来自：config entry、plugin manifest、skill dependency 自动安装（feature-gated，只允许 first-party）。`codex mcp add` 会直接写 global config。 | **有坑**。skill-dep 自动装 MCP 会往 `${CODEX_HOME}/config.toml` 写 AVM 不知道的键，导致 Agent 定义与 managed config 分叉；用户也可能直接编辑 config。需要在 `run` 启动/退出做对齐（PRD 4.4）。 |
| **Runtime mapping 状态：native / rendered_as_instructions / ignored / unsupported** | instructions、skills、MCP 三类都有 native 字段。sandbox/approval 拆成 5 维，Codex adapter 把这 5 个维度各自当一个 native 字段即可。 | **可行**。四种状态都有地方落。 |
| **全量发现 skills/MCP（包括 runtime 全局目录里非 AVM 管理的）** | Skills loader 每次都扫全部 root；MCP 也是 config + plugin + skill-dep 的实时合并。AVM 只要在 create/edit 时调用类似的扫描逻辑即可。 | **可行**。 |
| **AVM 不能静默覆盖 runtime 全局能力** | Codex 的 skill loader 按 scope 排序、不互删；`codex mcp add` 是显式写 global config。在独立 `CODEX_HOME` 方案下 AVM 只写 Agent 私有目录，根本不碰 user `~/.codex`。 | **可行**。独立 `CODEX_HOME` 天然解决。 |
| **agent/runtime 隔离边界（不含 memory）** | 硬隔离只能靠切 `CODEX_HOME`（env var 支持）。切了之后 config、auth、history、rollout、state DB、memory 全部跟着走，正好对应 PRD 要的 per-(Agent, runtime) 私有 boundary。 | **可行**。新 `CODEX_HOME` 要复制一份 user 级 `auth.json` 过去，否则每个 Agent 第一次运行都要重新 `codex login`。 |
| **Package 导入导出** | Codex plugin 有独立的 marketplace/cache/manifest 三层结构，plugin 能携带 skills + MCP + apps。语义上比 AVM package 更重。 | **有坑**。AVM package 可以不走 Codex plugin 体系，自己渲染；但如果想导出成 Codex 能直接认的 plugin，要对齐 manifest schema。 |
| **Memory 只做隔离不做内容管理** | Codex memory 是主动子系统：建 `CODEX_HOME/memories` 目录、加入 writable roots、起后台 job 写 DB `stage1_outputs` 和 `raw_memories.md`。不是被动 context。 | **有坑**。"不管 memory"≠"不发生 memory"。AVM 要知道只要用 Codex，memory 子系统就在跑。想不让它跑要显式禁用（feature flag 或 ephemeral session）。 |

**一句话**：PRD 的主线（Agent CRUD + runtime managed config + 全量发现 + runtime mapping）在 Codex 上可以做。两个要向用户披露的点：**每个 Agent 一个独立 `CODEX_HOME`，创建时要复制 user 级 `auth.json`**；**memory 子系统会自动写盘**，在独立 `CODEX_HOME` 下不会跨 Agent 串，但仍然会占磁盘并起后台 job。

---

## 三、运行时启动路径

### 入口

- Rust CLI `codex` 是真正的 runtime entrypoint。`main()` → `arg0_dispatch_or_else` → `cli_main`，再按子命令分发（`codex-rs/cli/src/main.rs:704-726`）。
- 无子命令时启动 TUI；`codex exec` 是 headless 路径；`codex mcp-server`、`codex app-server`、`codex plugin`、`codex cloud` 各有独立路径（`codex-rs/cli/src/main.rs:727-832`）。
- `codex-tui`、`codex-exec`、`codex-app-server` 也作为独立 binary 存在。

### Headless exec 如何启动

以 `codex exec` 为例，这是 AVM 最可能调用的模式：

1. CLI flag 里 `--full-auto` 映射为 `WorkspaceWrite` sandbox，bypass flag 映射为 `DangerFullAccess`，否则用显式 sandbox 值（`codex-rs/exec/src/lib.rs:272-278`）。
2. 解析 cwd 和 `CODEX_HOME`，预加载 config TOML，构造 `ConfigOverrides`（含 cwd、model、approval、sandbox、permission profile、writable roots 等），调用 `ConfigBuilder::build`（`codex-rs/exec/src/lib.rs:315-420`）。
3. 启动 in-process app-server 线程，发起一个 turn，输出 session id、model、approval、sandbox、cwd、rollout 路径（`codex-rs/exec/src/lib.rs:493-768`、`913-1099`）。

对 AVM 的含义：AVM 可以通过 CLI flag + 预写 config 文件双路径控制 Codex 行为，不需要 patch 二进制。

### Session 启动

`Session::new` 依次加载：plugins → skills → AGENTS.md → auth → MCP → state DB → memory startup（`codex-rs/core/src/session/session.rs:300-415`、`codex-rs/core/src/session/mod.rs:450-575`、`930-934`）。`SessionConfiguration` 携带 provider/model/instructions、approval、sandbox、cwd、codex_home 等（`codex-rs/core/src/session/mod.rs:595-624`）。

---

## 四、配置分层

Layer 合并顺序（后者覆盖前者）：

```
requirement (代码内置最小约束)
  → 系统   /etc/codex/config.toml
  → 用户   ${CODEX_HOME}/config.toml
  → 当前目录 config
  → 项目树  .codex/config.toml（从 cwd 向上到 project root）
  → 仓库根  .codex/config.toml
  → 运行时 CLI flags
```

关键点：

- 项目层受 **trust gate** 控制，未信任的 `.codex/config.toml` 不生效（`codex-rs/core/src/config_loader/mod.rs:930-1024`）。
- 如果 `.codex` 恰好等于 `CODEX_HOME`（比如用户把 `CODEX_HOME` 指到了当前项目的 `.codex`），项目层会被跳过，避免重复加载。
- `CODEX_HOME` 默认 `~/.codex`，可以用环境变量覆盖；设置后必须 canonicalize 成功（`codex-rs/core/src/config/mod.rs:2664-2674`）。

**对 AVM 的含义**：
- AVM 想写的 managed config 最自然放在用户层（`${CODEX_HOME}/config.toml`）或项目层（`.codex/config.toml`）。
- PRD 要的 per-(Agent, runtime) 私有 boundary，直接做法就是给每个 Agent 一个独立 `CODEX_HOME`，env var 支持，AVM 在 `avm run` 时注入。

---

## 五、隔离模型

Codex 把隔离拆成五个独立维度，Codex adapter 想 native 表达就要分别承接这些字段：

| 维度 | 取值 | 源码 |
|---|---|---|
| Approval mode | `UnlessTrusted` / `OnFailure` / `OnRequest` / `Granular` / `Never` | `protocol/src/protocol.rs:939-970` |
| Sandbox policy | `DangerFullAccess` / `ReadOnly` / `ExternalSandbox` / `WorkspaceWrite` | `protocol/src/protocol.rs:1029-1081` |
| File-system policy | 由 sandbox + writable roots 推导 | `core/src/config/mod.rs:1808-1932` |
| Network policy | `Restricted` / `Enabled` | `protocol/src/protocol.rs:1011-1027` |
| Writable roots | 路径列表，支持每条路径下的 read-only 子路径 | `protocol/src/protocol.rs:1083-1111`、`1180-1316` |

默认规则（Workspace-write 模式）：
- 所有路径可读。
- cwd、writable roots、未排除的 `/tmp` 和 `$TMPDIR` 可写。
- writable roots 下默认 read-only 的子路径：`.git`、`.agents`、`.codex`。
- 其他位置写操作需要 approval。

### Approval 生命周期

- 内存缓存 `ApprovalStore`，key 是序列化后的请求。只在当前 session 有效，**不持久化**（`codex-rs/core/src/tools/sandboxing.rs:40-117`）。
- `Never` 和 deprecated `OnFailure` 不询问；`OnRequest` / `Granular` 对受限 FS 访问询问；`UnlessTrusted` 默认询问。
- 命令 runtime 先构造 approval key（命令 + cwd + sandbox perms），拿到批准后才进入 sandbox transform 和 spawn（`codex-rs/core/src/tools/runtimes/shell.rs:134-289`）。

### 平台级 sandbox

- macOS Seatbelt、Linux seccomp + bubblewrap、Windows restricted token（`codex-rs/sandboxing/src/manager.rs:23-272`）。
- Linux 路径：先 bubblewrap 设置 FS view，再 `no_new_privs` + seccomp，最后 `execvp`（`codex-rs/linux-sandbox/src/linux_run_main.rs:30-221`）。
- Restricted network 对进程暴露为环境变量 `CODEX_SANDBOX_NETWORK_DISABLED`。

**对 AVM 的含义**：Codex adapter 想做到 native 表达，至少要能写 approval、sandbox 模式、writable roots 这三个字段。network 和 file-system policy 可以由前三个推导。

---

## 六、Skills

### 存在形态

Skill = 一个包含 `SKILL.md` 的目录。可选 metadata 在 `<skill>/.agents/agents/openai.yaml`（`codex-rs/core-skills/src/loader.rs:37-123`）。

### 发现 root（loader 会全量扫描）

1. 项目 `.codex/skills`
2. `${CODEX_HOME}/skills`（deprecated 但仍扫）
3. user `~/.agents/skills`
4. bundled system cache（`${CODEX_HOME}/skills/.system`，由 Codex 自己 materialize）
5. admin `/etc/codex/skills`
6. 仓库 `.agents/skills`
7. config 里声明的 extra roots
8. 每个 active plugin 提供的 skill root

Loader 有深度和数量上限，超限发 warning；按 scope rank 排序去重；plugin 提供的 skill 会按 plugin manifest 加 namespace（`codex-rs/core-skills/src/loader.rs:157-665`）。

### Bundled system skills

- Rust `codex-skills` crate 把一批 skill 嵌入二进制。
- 启动时安装到 `${CODEX_HOME}/skills/.system`，带 fingerprint marker，必要时删重建（`codex-rs/skills/src/lib.rs:10-127`）。
- 可以通过 `[skills].bundled.enabled=false` 关掉。

### 注入时机

- 启动时把 skill 列表渲染进 developer instructions（progressive disclosure：只给标题和简介）（`codex-rs/core-skills/src/render.rs:21-83`）。
- 用户 turn 里显式 mention 某个 skill 时，完整 `SKILL.md` 注入 context（`codex-rs/core-skills/src/injection.rs:31-170`）。

**对 AVM 的含义**：
- "全量发现"只要调用同样的 loader 或扫同样 8 个 root 就行。
- Scope + plugin namespace 给了天然的"来源"字段，PRD 要求的来源区分可以直接用。
- Phase 1 把 AVM 管理的 skill materialize 到 Agent 私有 `${CODEX_HOME}/skills`。因为 Codex 还会扫 user `~/.agents/skills`，`avm run` 必须同时把 `HOME` 指到 Agent 私有 boundary，否则用户全局 skill 会泄漏进本次运行。

---

## 七、MCP

### 配置形态

MCP server config 支持两种 transport：`Stdio` 和 `StreamableHttp`。字段包括 enabled/required、启动和工具超时、approval mode、allow/deny tool 名单、OAuth scopes、per-tool config、cwd、env、headers、bearer-token 映射（`codex-rs/config/src/mcp_types.rs:117-391`）。

### 来源合并（session startup 时）

1. config 里声明的 server（用户 config 或项目 config）
2. 每个 active plugin 的 MCP section
3. 被显式 mention 的 skill 声明的 MCP 依赖（feature-gated，只允许 first-party，可能弹确认框后写回 global config）

session 把这三路合并成 effective list，交给 `McpConnectionManager`（`codex-rs/core/src/config/mod.rs:848-873`、`codex-rs/core/src/session/session.rs:383-397`）。

### 运行时

- 每个 server 对应一个 `RmcpClient`。工具名以 `<server>/<tool>` 的完全限定形式聚合。
- Stdio server 作为本地子进程启动，也支持通过 executor 远程启动。
- HTTP server 用 bearer token 或 OAuth；OAuth token 优先 keyring，fallback 到 `${CODEX_HOME}/.credentials.json`（`codex-rs/rmcp-client/src/oauth.rs:1-580`）。
- Codex Apps 工具 metadata 缓存在 `${CODEX_HOME}/cache/codex_apps_tools`。

### `codex mcp add` 做什么

- 读 `${CODEX_HOME}` 下的 global server config。
- 用 `ConfigEditsBuilder.replace_mcp_servers` 写回。
- 即：MCP 的持久化入口只有 global user config 这一个（不写项目 config）。

**对 AVM 的含义**：
- 全量发现 = config + plugin + skill-dep，三路分别查就行。
- PRD 担心"静默合并"——MCP 这边冲突规则是 AVM 要自己定的，Codex 本身不会神奇合并同名 server。
- skill 声明 MCP 依赖会自动修改 user config 这件事，要在 AVM 里提示用户，否则会看到"我没改但 config 变了"。

---

## 八、Plugins / marketplace / apps

这是 Codex 里最复杂的部分，和 AVM Package 概念有重叠但不等同。

- Plugin manifest 在 `.codex-plugin/plugin.json`，字段含 name、version、description、skills、MCP servers、apps、interface（`codex-rs/core-plugins/src/manifest.rs:11-224`）。
- Plugin cache 路径：`${CODEX_HOME}/plugins/cache/{marketplace}/{plugin}/{version}`。安装是原子替换（`codex-rs/core-plugins/src/store.rs:14-310`）。
- Marketplace manifest 在 `.agents/plugins/marketplace.json` 或 `.claude-plugin/marketplace.json`。Installed marketplace root 从 user config 读，默认 `${CODEX_HOME}/.tmp/marketplaces`（`codex-rs/core-plugins/src/marketplace.rs`、`installed_marketplaces.rs`）。
- **Configured plugins 只从 user config 层加载**，不是项目层（`codex-rs/core-plugins/src/loader.rs:103-140`）。
- Plugin 加载结果贡献三样东西：skill roots、MCP servers、apps。这就是上面 skills/MCP 章节里提到的 "plugin-contributed"。

**对 AVM 的含义**：
- AVM package 概念可以映射到 Codex plugin，但 schema 不一样，需要 adapter 翻译。
- 如果 AVM package 只想分发 skills + MCP 而不用 Codex plugin 机制，完全可以：AVM 自己把 skill 文件放进 configured extra root、把 MCP 写进 user config。这样 AVM package 和 Codex plugin 保持正交。
- marketplace 是 Codex 自己的分发层，AVM 如果不想碰，避开即可。

---

## 九、状态与数据

`CODEX_HOME`（默认 `~/.codex`）下有什么：

| 路径 | 内容 | 备注 |
|---|---|---|
| `config.toml` | 用户级配置 | TOML，`codex mcp add` 等会改它 |
| `auth.json` | Codex 账户凭证 | Unix `0600`；也可走 keyring 或 ephemeral |
| `.credentials.json` | MCP OAuth fallback | keyring 不可用时才用 |
| `history.jsonl` | prompt 历史 | append-only，大小限幅，Unix `0600` |
| `sessions/YYYY/MM/DD/rollout-*.jsonl` | 每个 thread 的完整事件流 | 后台 async writer 缓冲写入 |
| `session_index.jsonl` | thread 名字索引 | append-only，最后一条生效 |
| `<state>_<version>.sqlite` | State DB | 存 threads、memories jobs、stage1_outputs |
| `<logs>_<version>.sqlite` | Logs DB | tracing 事件，10 天 + 每 partition 约 10MiB / 1000 行 |
| `skills/` | 用户级 skills（含 `.system` 子目录的 bundled skills） | bundled 由 Codex 自动 materialize |
| `plugins/cache/` | plugin 版本化缓存 | 原子替换 |
| `.tmp/marketplaces/` | marketplace 安装临时目录 | 用户 config 可覆盖 |
| `memories/` | memory artifacts | 含 `raw_memories.md`、`rollout_summaries/`、`memories_extensions` |
| `cache/codex_apps_tools/` | Codex Apps 工具 metadata 缓存 | MCP 启动时填 |
| `models_cache.json` | 模型 metadata 缓存 | 默认 TTL 300s |
| `log/codex-tui.log`、`log/codex-login.log` | 文本日志 | TUI 会话可额外录 JSONL |

**核心事实**：`CODEX_HOME` 是一个胖目录，里面的 config / history / sessions / state DB / memory 都是全局的。PRD 要的 per-(Agent, runtime) 私有 boundary 因此需要每个 Agent 一个独立 `CODEX_HOME`；同时 Codex 会读取 `$HOME/.agents/skills`，所以 `avm run` 也要把 `HOME` 指到同一个 Agent 私有目录，避免 user-global skills 混入。创建 Agent 时需要把 user 级 `auth.json` 复制过去，避免首次运行要求重新 `codex login`。

### Memory 子系统特别说明

因为 PRD 明确说"不管 memory"，这里重点提醒它 **不是被动 context**：

- Memory startup 在 session 启动后自动跑，除非是 ephemeral session、feature flag 关掉、或当前是 subagent（`codex-rs/core/src/memories/start.rs:10-43`）。
- 会主动创建 `${CODEX_HOME}/memories`，**自动把它加进 writable roots**（`codex-rs/core/src/config/mod.rs:1794-1801`）。
- Phase 1 从 state DB 拿 stale thread、生成 memory 存到 `stage1_outputs` table。
- Phase 2 从 DB 拉数据重建 filesystem artifacts（`raw_memories.md`、`MEMORY.md`、`memory_summary.md` 等），可能 spawn memory consolidation subagent。

对 AVM：memory 会往 `${CODEX_HOME}/memories` 写文件、起后台 job。在独立 `CODEX_HOME` 方案下这些产物被关在各自 Agent 的边界里，不会跨 Agent 串，PRD "不管 memory 但要隔离运行状态"的诉求天然成立。需要披露给用户的是：磁盘会占用、后台 job 会跑；想完全不写则需在 Codex config 里关掉 memory feature flag 或只用 ephemeral session。

---

## 十、对 PRD 的风险提示

按 PRD 章节对应的风险清单：

1. **PRD 2.3（能力边界）/ 4.2（全量发现）**：Codex 没有"单一 registry"。skills、MCP、plugin、app、skill-dep 各走各的路径。AVM 要自己定义合并和展示规则。

2. **PRD 4.4（Agent 定义 vs managed config 对齐）**：skill-dep 自动装 MCP、`codex mcp add`、用户直接编辑 config.toml 都会让 Agent 私有 `CODEX_HOME/config.toml` 与 AVM 的 Agent 定义产生 drift。独立 `CODEX_HOME` 只解决了"不污染 user 全局 config"，没解决"AVM 侧的单一真相源"问题。PRD 4.4 已加对齐原则（run 启动/退出时核对差异并让用户决定），具体机制待定。

3. **PRD 4.4（运行透明）**：Codex 命令级 approval 不持久化，下次启动就重问一遍。PRD 要展示"哪些 runtime 已就绪"的时候，approval 状态不能缓存到 AVM 侧当成 durable 属性。

4. **PRD 4.6（memory）披露项**：在独立 `CODEX_HOME` 方案下 memory 不会跨 Agent 串，PRD 的"不管 memory 但要隔离运行状态"成立。但 memory 子系统仍会自动写盘、占 writable root、起后台 job。这些副作用需要在 Agent 创建时告知用户；想完全不发生 memory，要显式禁用 feature flag 或只用 ephemeral session。

5. **PRD 3.3（Package）**：Codex plugin 语义比 AVM package 丰富（marketplace、版本、原子替换、manifest）。AVM package 导出为 Codex plugin 需要 schema 翻译；导出为"AVM 自己认"的 zip 则可以不碰 plugin 体系。

6. **PRD 3.4（Runtime）auth 分家**：独立 `CODEX_HOME` 方案的副作用是 `auth.json` 也跟着分家。AVM 必须在 Agent 创建时把 user 级 `${HOME}/.codex/auth.json` 复制到 Agent 私有 `CODEX_HOME`，否则用户每个 Agent 第一次运行都要重新 `codex login`。如果 user 侧 auth 后续轮换，AVM 还要考虑是否要同步刷新已有 Agent 的副本。

---

## 十一、未验证项

- 当前 checkout 没有记录 Codex 具体 version/commit，不同版本行为可能有差异。
- 环境没有 `cargo`，没跑 `cargo metadata` 或 `cargo run -- --help`；启动路径结论来自源码追踪，不是二进制验证。
- Cloud session、remote executor 只追到它们在 exec/app-server/MCP 路径上的接入点，完整行为未调研。
- Windows sandbox 只识别到 helper 模块，追踪深度不如 Linux。
- Codex Apps / connector 的 auth 和 UI 行为未穷尽。
- Feature flag（bundled skills、skill MCP dep、memory tool、plugin marketplaces、sandbox modes）对很多路径是必然/可选的切换，要把任一路径当成必然行为之前，要固定 feature flag scope。
