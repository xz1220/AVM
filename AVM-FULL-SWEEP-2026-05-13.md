# AVM 完整功能 sweep — 2026-05-13

commit 8307a79；环境 darwin-arm64；视角：clean install + 每个子命令至少 1 happy + 1 error path。

## TL;DR

13 个顶层命令、~30 个子命令实测。**daily path 全部跑通**，但发现 **19 个具体问题**，
其中 **3 个 P0**（含 2 个安全回归延伸 / 已在上次报告 + Codex auth.json 同样泄密），
**8 个 P1**（功能错或显式 "not implemented"），**8 个 P2**（UX / 一致性）。

整体测试通过率（命令级）：约 70%。

## 测了什么

| 命令 / 子命令 | happy | error | 备注 |
|---|---|---|---|
| `--version` `--help` `--json` | ✅ | — | 顶层 OK |
| `init`、`init` 二次 | ✅ | — | idempotent ✓ |
| `status` `--json` | ✅ | — | 但会隐式建 `~/.avm` |
| `doctor` | ✅ | — | 同 status 隐式建 home |
| `setup` 默认 / `--runtime X` / `--no-capabilities` / `--on-conflict skip/overwrite` | ✅ | `--on-conflict bogus` ✓ | |
| `runtime list` `--json` | ✅ | — | |
| `capability list` `discover` `show <id>` `import` `bootstrap` | ✅ | bogus runtime / kind 缺失 ✓ | bootstrap 二次跑藏错误 |
| `agent create / list / show / show --runtime / edit / clone / rename / delete` | ✅ | dup / no-name / bogus-skill ✓ | `--runtime alien` 不校验；`show --runtime` 输出空 |
| `run --preview` 单 / 多 runtime / `--drift` / `--runtime` 覆盖 | ✅ | ghost agent / bogus drift ✓ | `--runtime` 可绕过 agent 声明的 runtime |
| `package export / inspect / install / list / show / uninstall` | ✅ | bogus / corrupt zip ✓ | `list/show` "not implemented" |
| `shell install / uninstall` | 未实装 | 不支持 shell ✓ | 无 `shell status` |
| `uninstall` dry-run / `--yes` / `--json` | dry-run ✓ | — | `--json` 输出仍是文本 |
| `completion zsh/bash/fish` | ✅ | — | 标准 cobra 模板 |

## 上次未修的 + 这次新发现（19 个）

### P0（3 个，含上次 2 个 + 新 1 个）

1. **boundary 泄露 OAuth token**（上次发现的）—— 用户 `~/.claude/settings.local.json`
   里的真实 OAuth token 被原样 copy 进 boundary。**确认这次仍存在。**
2. **bypassPermissions 默认被继承**（上次发现的）—— 用户全局
   `defaultMode=bypassPermissions` 被原样写进每个 boundary。**确认仍存在。**
3. **Codex 也泄密** 🆕 —— `~/.avm/boundaries/codex/<agent>/auth.json` 是
   `~/.codex/auth.json` 的字节级 copy（`diff` 空），含 ChatGPT id_token JWT。
   是同一个"wholesale-copy 用户 home"模式的另一处发作，不是孤立 bug。
   （好消息：文件 perm=600，比 claude-code 这边的 perm 同样规范）

### P1（8 个，功能错误或显式 not implemented）

4. **`agent show <name> --runtime <r>` 输出空**——README 明文承诺 "how each field
   maps to the runtime"，实际 stdout 一字没有，`--json` 也空，exit=1。
5. **`agent create --runtime alien` 不校验 runtime 名**——成功创建 `runtimes: [alien]`
   的 agent，到 `avm run` 才报错。对比 `--skill bogus` 是即时拒绝，runtime 校验缺失。
6. **`run --runtime X` 可覆盖 agent 声明的 runtime**——agent 只声明
   `claude-code`，`run --runtime codex` 仍然渲染并尝试启动，应该报
   "agent does not declare runtime codex"。
7. **`capability bootstrap` 二次跑 exit=0 但藏 27 行错误**——`runtime "claude-code"
   has no skill named X`（plugin marketplace skill）。discover 能看到但 import
   不能装，两个命令视角不一致。
8. **`package list/show` 显式 "not yet implemented"**——但 README §5 把
   `avm package list` 列为 daily path。`show` 还 exit=0，应该 exit=1。
9. **`package list --json` 返回 `null`**——应该是 `[]`（empty array）。
10. **`uninstall --json` 输出纯文本**——`--json` 全局标志被忽略。
11. **`uninstall` 显示要删源码构建路径**——`os.Executable()` 指向我源码 build
    的 `bin/avm`，不是 README 推荐的 `~/.local/bin/avm`。源码运行 uninstall
    会留下真正的安装不动。

### P2（8 个，UX / 一致性）

12. **`status` 和 `doctor` 隐式建 `~/.avm`**——无 mutate 意图的命令也产生副作用。
    跑 `status` 之后 `init` 就报 "already initialized"。
13. **`agent show` 把 skills 显示成 `cap_XXX` hash**——`agent list` 用 name，
    一致性差。
14. **第一次 `avm run` 必报 `drift detected`**——空 boundary 也判 drift，
    第一次 run 应视为 first-create。
15. **`avm shell status` 静默回退到 `shell --help`**——应报 unknown subcommand。
16. **没有 `shell status`**——`doctor` 报 "Shell integration: FAIL" 但 shell
    子命令本身没有 status 命令，必须从 doctor 看。
17. **`shell install` 没有 dry-run**——只能装。
18. **`run` 没有 `--render-only`**——`--preview` 不写文件，写文件就必须
    `spawn runtime`，QA / snapshot test 无法不启动 runtime 的情况下 inspect 输出。
19. **`agent show` mapping 表只列 agent 配过的字段**——MCP 字段为空就不显示
    `mcp -- not configured`，用户无法知道 runtime 究竟支持哪些字段。

## 这次做对的地方

- 全部 CRUD（create/list/show/edit/clone/rename/delete）+ package roundtrip 跑通
- `agent edit` 的 replace 语义符合 README ✓
- `--on-conflict` 在 setup / capability / package 三处行为一致
- `setup` 自动检测 3 个 runtime + 列每个 4 个 caveat
- `run --preview` 五段输出（Env / Will write / Mapping / Drift / Warnings）
- `runtime list --json` shape 干净
- error 信息绝大部分清晰（regex 约束、unknown drift policy、agent not found 等）
- `go test ./...` 11 个包全绿

## 优先级建议（如果只能修 3 个）

1. **secret 扫描 / boundary 渲染重写**——P0 #1+#2+#3，影响 3 个 adapter，
   "isolated boundary" 心智完全靠这个
2. **`agent show <name> --runtime <r>` 实装**——README 承诺的核心功能 + 这是
   fidelity reporting UI 的入口
3. **runtime 名校验 + `run --runtime` 不应覆盖未声明 runtime**——P1 #5+#6，
   数据完整性

## 给 Agent Deck 的额外启示

1. **`--json` 必须每个命令都走同一渲染层**——uninstall/`package list` 的不一致
   说明 JSON 输出是按命令各自实现的，不是统一管道。Agent Deck 应该从一开始就
   把 `Render(out, model)` 强制走双轨。
2. **read-only 命令不该有副作用**——`status` / `doctor` 隐式建 home 是反模式。
   Agent Deck 的 readonly views（看面板、看 agent 列表）必须 strictly read-only。
3. **"not yet implemented" 应该 exit ≠ 0**——`package show` exit=0 但返回
   "not implemented" 是脚本 / CI 的隐形地雷。
