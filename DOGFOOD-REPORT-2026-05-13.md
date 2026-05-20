# Agent-VM Dogfood 报告 — 2026-05-13

视角同上次：darwin-arm64，BENZEMA216，README daily path。
基线 = DOGFOOD-REPORT-2026-05-08.md，对比 commit db58dc1 → 8307a79（5 commit，主体是 fix: repair package roundtrip + README 重写）。

## TL;DR — daily path 终于跑通；但 boundary 在泄密

README 5 步 daily path 全部跑通了（上次只有 2/4）。
但 avm run 把用户的 OAuth token 和 bypassPermissions 默认设置写进了 boundary。

## 上次 P0 复盘（8/8 全部修了）

| # | 上次现象 | 这次状态 |
|---|---|---|
| 1 | install.sh URL 404 | 修了 — curl 200 OK，6.6 KB |
| 6 | avm create backend-coder 位置参数报错 | 修了 — README 已统一为 --name |
| 7 | avm create 提示与 daily path 不一致 | 修了 — 提示 avm run |
| 10 | avm run 接 runtime 名 vs README 的 agent 名 | 修了 — Usage: avm run <agent> |
| 11 | AVM_CLAUDE_MCP_CONFIG 指向不存在文件 | 修了 — 该 env 不再导出 |
| 12 | settings.json = {"agent":"X"} 非法 | 形式修了，语义新出回归（见下）|
| 13 | subagent frontmatter effort 非法字段 | 修了（schema 整体撤掉 subagents） |
| 15 | avm skill 只有 list，看不见 ~/.claude/skills | 修了 — avm setup 一行导入 45+21+12=68 个 capability |

## 上次 P1 复盘（3/4 修了）

| # | 现象 | 状态 |
|---|---|---|
| 2 | 顶层暴露 activate/deactivate/env/use/sync | 修了 |
| 4 | ~/.avm/envs/default.yaml 仍带 runtime_agents | 修了 — envs/ 整体删除 |
| 9 | profile 在 cmd/ 出现 135 次 vs agent 47 次 | 修了 — 全代码库 profile 0 次 |
| 14 | "Claude Code Supported" 名不副实 | 部分修了 — skills 真渲染了，但 mcp 仍空 |

## 上次 P2 复盘（2/2 修了）

| # | 现象 | 状态 |
|---|---|---|
| 3 | --version 输出 (none, unknown) | 修了 — 0.0.0-dev (8307a79, 2026-05-13T...) |
| 5 | avm init 没说创了什么 | 修了 — 列了所有 dir + Next |

## 新发现

### P0 安全/隐私回归

**Boundary 泄露 OAuth token + 用户身份**
~/.avm/boundaries/claude-code/backend-coder/ 出现：
- settings.local.json 里有 Bash(TOKEN="sk-ant-oat01-...") — 用户真实 Claude OAuth token
- .claude.json 里完整保留 oauthAccount block（accountUuid / emailAddress / organizationUuid / subscriptionCreatedAt）

修复方向：
1. boundary settings.* 不应整段 copy 用户 home；应从空白开始
2. token 字段必须 redact
3. 加 secret scrubber：扫到 sk-ant- / Bearer / OAuth pattern 时拒绝写入

**bypassPermissions 默认被继承**
用户 home 的 defaultMode=bypassPermissions + skipDangerousModePermissionPrompt=true 被原样写进每个 boundary settings.json。
后果：AVM 跑出来的 agent 看似"隔离 boundary"，实际默认 bypass 所有权限。

修复：boundary permissions 应明确 opt-in 而非继承。安全策略上 boundary 应比 home 更严。

### P1

- ~/.avm 权限 700 → 755（上次报告夸过的 secrets 处理意识没了；结合 OAuth token 写进 boundary，杀伤力比上次大）
- agent show 把 skills 显示成 cap_XXX hash（agent list 用 name，不一致）
- 全新 install 第一次 avm run 必报 drift detected — 第一次 run 不该判 drift

### P2

- avm run 没有 --no-launch / --render-only — 想检查 boundary 文件就必须真 spawn runtime
- agent show 的 mapping 表里没有 mcp / instructions 字段；建议显示所有 driver 知道的字段并标 "-- not configured"

## 这次做对的地方

- avm setup 自动检测 3 个 runtime + 列出每个 4 条 caveat ✓ 最大产品亮点
- agent show <name> --runtime <r> 把 fidelity mapping 显式暴露 ✓ 上次点名要求
- avm run --preview 五段输出（Env / Will write / Mapping / Drift / Warnings）✓
- go test ./... 11 个包全绿 ✓
- package export → delete → install → show roundtrip 完美 ✓
- 术语清洗到位（profile 0 次）✓

## 给 Agent Deck 的启示（补充上次）

1. "isolated boundary" ≠ "copy user home" — AVM 这次的安全回归正踩在这边界上
2. 第一次运行不该判 drift — 数据没建过就没 baseline
3. fidelity mapping 应全字段展示，把"未配"和"不支持"分开
4. secret 扫描应该是 write barrier，不是开关
