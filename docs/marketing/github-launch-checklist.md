# GitHub Launch Checklist

Use this checklist before making the repository public or announcing it.

## Repository Metadata

Recommended description:

```text
Local control plane for portable AI coding agent profiles across Codex, Claude Code, OpenClaw, and Hermes Agent.
```

Recommended topics:

```text
ai-agents, coding-agents, codex, claude-code, openclaw, hermes-agent, mcp, agent-profiles, cli, golang, developer-tools
```

Recommended website:

```text
https://github.com/xz1220/Agent-VM#readme
```

## Required Before Public Launch

- [ ] Choose and add an open-source license.
- [ ] Make the repository public.
- [ ] Add repository description and topics.
- [ ] Enable GitHub private vulnerability reporting if available.
- [ ] Add a social preview image.
- [ ] Keep all localized README files aligned on implemented vs planned behavior.
- [ ] Tag the first preview release after `avm use`, `status`, and `deactivate`
      are implemented.

## First Public Release Bar

Do not present AVM as installable until these work end to end:

- [ ] `avm init`
- [ ] `avm agent create/list/show`
- [ ] `avm use <profile>`
- [ ] `avm status`
- [ ] `avm deactivate`
- [ ] one concrete runtime adapter path, preferably Codex or Claude Code
- [ ] clear backup/conflict behavior for managed runtime files

## Demo Assets

Current repo assets:

- `DESIGN.md` — AVM visual source of truth, inspired by Cursor-like developer tooling.
- `assets/avm-hero.svg` — README hero and social-preview source.
- `assets/avm-before-after.svg` — README problem/solution visual.

Create a short terminal recording that shows:

1. `avm init`
2. creating `backend-coder`
3. attaching one skill and one MCP ref
4. `avm use backend-coder`
5. `avm status` showing native, rendered, unsupported, and ignored mappings
6. the generated runtime config diff

Keep the recording under 60 seconds. The viewer should understand the product
without reading the full README.

## Launch Message

Short version:

```text
I am building Agent VM: a local control plane for AI coding agent profiles.

Instead of syncing scattered prompt files and MCP configs by hand, AVM makes the
agent itself a portable object: role, tools, permissions, model settings, and
runtime choice. The first target is developers who use multiple coding agents such
as Codex, Claude Code, OpenClaw, and Hermes Agent.
```

Call to action:

```text
If you manage more than one AI coding tool, I would love feedback on the Agent
Profile model and the runtime adapter roadmap.
```
