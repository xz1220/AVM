# Runtime Memory Isolation Feasibility

> Last updated: 2026-05-03

This document records runtime-native memory behavior for AVM design work. It is
not an AVM memory schema. AVM should not introduce portable memory files,
`memory_refs`, or `avm memory` until isolation can be expressed in terms of the
target runtime's real storage model.

Current scope is isolation only. This document does not design memory content
management, import/export, reset, sync, or runtime memory enable/disable
behavior. The only question is: can AVM keep global/user-level memory state for
different AVM Agent configurations separated, and what runtime boundary makes
that possible?

## Design Position

AVM can only promise memory isolation when the runtime exposes a storage boundary
that AVM can control. In practice, that means one of:

- a runtime home directory controlled by env vars
- a native profile/agent identity that the runtime itself uses for memory
- a provider-specific namespace that can be derived from the AVM Agent identity

Project-level memory is explicitly out of scope. If the runtime writes memory to
repository or workspace files, AVM treats those files as project assets and does
not manage, import, export, reset, or isolate them. AVM may document or diagnose
their existence, but it should not mutate them as part of Agent configuration.

If the runtime writes memory to shared global app data or a remote provider
workspace that AVM cannot namespace, AVM cannot claim isolation. AVM should not
pretend isolation exists by inventing a portable memory layer above the runtime.

## Feasibility Matrix

| Runtime | Native memory/storage anchor | Isolation AVM can provide | Caveats |
|---------|------------------------------|---------------------------|---------|
| Codex | `codex_home/memories`, `codex_home/memories_extensions`, and `codex_home/state_*.sqlite` | Strong if each AVM Agent gets its own `CODEX_HOME` | If multiple AVM Agents share the same runtime home, memories and state DB overlap. |
| Claude Code | `~/.claude/projects/<project>/memory`, `~/.claude/agent-memory/<agent>`, project `.claude/agent-memory*`, plus `CLAUDE.md`/rules | Global-only partial. `CLAUDE_CONFIG_DIR` isolates user-level `.claude`; project memory is unmanaged | Project `CLAUDE.md`, rules, and project-scoped subagent memory are shared project assets. User-scope subagent memory is isolated only if `CLAUDE_CONFIG_DIR` is per AVM Agent. |
| OpenCode | No general long-term memory module found in source. Session/history/storage live under XDG data/state; DB path can be overridden with `OPENCODE_DB` | Weak by default. `OPENCODE_CONFIG_DIR` isolates config, not all data. AVM would need XDG data/state isolation or `OPENCODE_DB` per Agent for session/history separation | Repo files such as `.github/instructions/memory.instruction.md` are project assets and out of AVM memory scope. |
| OpenClaw | Workspace `MEMORY.md`, workspace `memory/**/*.md`, builtin store `state/memory/<agentId>.sqlite`, QMD index under `state/agents/<agentId>/qmd` | Partial. AVM can isolate global state by controlling `agentId` and OpenClaw state dir; workspace memory files are unmanaged | Extra memory paths/collections can intentionally cross agent boundaries, so AVM must not enable shared `extraPaths` by default. |
| Hermes | Built-in `HERMES_HOME/memories/MEMORY.md` and `USER.md`, `HERMES_HOME/state.db`, memory provider plugins selected by `memory.provider` | Strong for built-in memory if AVM uses one `HERMES_HOME` or Hermes profile per AVM Agent | External providers are provider-specific. Honcho workspaces can be shared; Hindsight supports templated bank IDs using profile/workspace/user/session. AVM must configure the provider namespace, not just local files. |

## Codex

Codex stores memory under the runtime home. In the source, `memory_root` returns
`codex_home.join("memories")`, the clear path removes both `memories` and
`memories_extensions`, and read-path prompt construction reads
`memory_summary.md` under that memory root. Codex state DB is also anchored to
`codex_home`.

Codex config has native memory toggles such as `generate_memories` and
`use_memories`, defaulting to enabled. This means the isolation boundary is not
a Codex profile name; it is the Codex home directory.

AVM implication:

- Use a distinct `CODEX_HOME` per AVM Agent identity when memory isolation is
  required.
- Environment-level activation that maps several AVM Agents into one Codex home
  should be treated as shared memory.
- AVM's current adapter already accepts `RuntimeHome` and maps it to
  `CODEX_HOME`; the missing design choice is whether runtime homes are keyed by
  active environment or by Agent identity.

Source evidence:

- `/root/repos/ai-startup/research/agent-frameworks/codex/codex-rs/core/src/memories/mod.rs`
- `/root/repos/ai-startup/research/agent-frameworks/codex/codex-rs/core/src/memories/control.rs`
- `/root/repos/ai-startup/research/agent-frameworks/codex/codex-rs/core/src/memories/prompts.rs`
- `/root/repos/ai-startup/research/agent-frameworks/codex/codex-rs/config/src/types.rs`
- `/root/repos/ai-startup/research/agent-frameworks/codex/codex-rs/state/src/runtime.rs`

## Claude Code

Claude Code has multiple native memory surfaces:

- `CLAUDE.md`, `.claude/CLAUDE.md`, and `.claude/rules/*.md` are persistent
  instructions.
- Auto memory stores project notes under `~/.claude/projects/<project>/memory`.
- Subagent persistent memory stores by scope:
  - user: `~/.claude/agent-memory/<agent>/`
  - project: `.claude/agent-memory/<agent>/`
  - local: `.claude/agent-memory-local/<agent>/`

The official docs also state that `CLAUDE_CONFIG_DIR` relocates `~/.claude`.
Therefore AVM can isolate user-level Claude data by giving each AVM Agent a
different `CLAUDE_CONFIG_DIR`. That does not automatically isolate project files
or project-derived auto memory when two Agents run against the same git project.

AVM implication:

- For Claude user-level data, use per-Agent `CLAUDE_CONFIG_DIR`.
- For project `CLAUDE.md`, rules, and project-scoped subagent memory, treat them
  as shared project configuration outside AVM memory management.
- For project auto memory under `~/.claude/projects/<project>/memory`, AVM can
  isolate only by changing the global Claude home. AVM should not manage the
  project files that feed it.
- Do not import these files as AVM memory. They are runtime-native context.

Primary docs:

- https://code.claude.com/docs/en/memory
- https://code.claude.com/docs/en/claude-directory
- https://code.claude.com/docs/en/sub-agents
- https://code.claude.com/docs/en/configuration

## OpenCode

In the inspected source, `memory` in `v2/session-entry-stepper.ts` is an
in-process reducer state, not durable memory. Durable data is stored as
OpenCode app data:

- global paths come from XDG data/cache/config/state directories
- JSON storage lives under `Global.Path.data/storage`
- SQLite DB defaults to `Global.Path.data/opencode.db`
- `OPENCODE_DB` can override the DB path
- prompt history, kv, and prompt stash use `Global.Path.state`

The source did not show a framework-level long-term memory provider comparable
to Codex, OpenClaw, Claude auto memory, or Hermes. One prompt template tells the
model it may use `.github/instructions/memory.instruction.md`; that is a
project file convention, not an isolated runtime store.

AVM implication:

- `OPENCODE_CONFIG_DIR` isolates config only.
- If session/history isolation matters, AVM must also isolate XDG data/state or
  set `OPENCODE_DB` per AVM Agent.
- Project-file memory conventions are outside AVM memory management.

Source evidence:

- `/root/repos/ai-startup/research/agent-frameworks/opencode/packages/core/src/global.ts`
- `/root/repos/ai-startup/research/agent-frameworks/opencode/packages/core/src/flag/flag.ts`
- `/root/repos/ai-startup/research/agent-frameworks/opencode/packages/opencode/src/storage/db.ts`
- `/root/repos/ai-startup/research/agent-frameworks/opencode/packages/opencode/src/storage/storage.ts`
- `/root/repos/ai-startup/research/agent-frameworks/opencode/packages/opencode/src/session/prompt/beast.txt`

## OpenClaw

OpenClaw has first-class memory configuration with `builtin` and `qmd`
backends. The native memory shape mixes project/workspace files with global
state:

- workspace root memory file: `MEMORY.md`
- workspace memory directory: `memory/**/*.md`
- builtin store: `state/memory/<agentId>.sqlite`
- QMD state/index: `state/agents/<agentId>/qmd`

OpenClaw resolves a per-agent workspace. Workspace memory files are project
assets from AVM's perspective, even when OpenClaw names them per agent. QMD
collection names include the agent ID, and QMD state/index files are under
global state.

AVM implication:

- OpenClaw global state can be isolated if AVM controls `agentId` and state dir.
- OpenClaw workspace memory files remain unmanaged project assets.
- Do not share `memory.qmd.paths`, `agents.defaults.memorySearch.extraPaths`, or
  QMD extra collections unless the user explicitly chooses shared memory.
- A future AVM OpenClaw adapter should expose memory isolation as runtime policy,
  not as portable AVM memory files.

Source evidence:

- `/root/repos/ai-startup/research/agent-frameworks/openclaw/src/config/types.memory.ts`
- `/root/repos/ai-startup/research/agent-frameworks/openclaw/src/agents/agent-scope-config.ts`
- `/root/repos/ai-startup/research/agent-frameworks/openclaw/src/memory/root-memory-files.ts`
- `/root/repos/ai-startup/research/agent-frameworks/openclaw/src/agents/memory-search.ts`
- `/root/repos/ai-startup/research/agent-frameworks/openclaw/packages/memory-host-sdk/src/host/backend-config.ts`
- `/root/repos/ai-startup/research/agent-frameworks/openclaw/extensions/memory-core/src/memory/qmd-manager.ts`

## Hermes

Hermes uses `HERMES_HOME` as the main local boundary. The built-in memory tool
uses `HERMES_HOME/memories/MEMORY.md` and `USER.md`; session persistence uses
`HERMES_HOME/state.db`. Hermes profiles also bootstrap `memories`, `sessions`,
`skills`, `logs`, `workspace`, and a per-profile `home` directory.

Hermes also has pluggable external memory providers. The MemoryManager allows
the built-in provider plus at most one external provider selected by
`memory.provider`. Provider initialization receives `hermes_home`,
`agent_identity`, `agent_workspace`, and gateway user/chat identifiers. Some
providers can use those fields for isolation; others can still be shared if
configured with a shared workspace.

AVM implication:

- For built-in Hermes memory, one `HERMES_HOME` per AVM Agent is enough.
- For Hermes native profiles, one profile per AVM Agent is also feasible.
- For external providers, AVM must configure provider-specific namespaces:
  Hindsight can template bank IDs using profile/workspace/user/session; Honcho
  workspaces can intentionally share identity and memory.

Source evidence:

- `/root/repos/ai-startup/research/agent-frameworks/hermes-agent/hermes_constants.py`
- `/root/repos/ai-startup/research/agent-frameworks/hermes-agent/tools/memory_tool.py`
- `/root/repos/ai-startup/research/agent-frameworks/hermes-agent/hermes_state.py`
- `/root/repos/ai-startup/research/agent-frameworks/hermes-agent/run_agent.py`
- `/root/repos/ai-startup/research/agent-frameworks/hermes-agent/agent/memory_provider.py`
- `/root/repos/ai-startup/research/agent-frameworks/hermes-agent/agent/memory_manager.py`
- `/root/repos/ai-startup/research/agent-frameworks/hermes-agent/plugins/memory/honcho/README.md`
- `/root/repos/ai-startup/research/agent-frameworks/hermes-agent/plugins/memory/hindsight/__init__.py`

## AVM Isolation Rule

For the current scope, AVM should only record or expose runtime memory isolation
status. It is not a memory feature and not a portable content object:

- `isolated`: AVM gives this Agent a private runtime memory boundary.
- `shared`: AVM intentionally uses a shared runtime memory boundary.
- `unsupported`: AVM cannot prove or enforce isolation for that runtime/provider.

This policy only applies to global/user-level runtime state. Project-level
memory stays with the project and is not managed by AVM. This should live next
to runtime activation behavior because activation decides the runtime home,
state directory, and provider namespace. It should not add portable
`memory_refs` to Agent profiles and should not participate in package
import/export.
