# Adapter Module

> 最后更新：2026-05-04（runtime boundary 隔离）

`internal/adapter` translates resolved AVM activation input into
runtime-specific render plans.

## Contract

```go
type Adapter interface {
    Name() string
    Detect(ctx Context) Detection
    Plan(ctx Context, input RenderInput) (*RenderPlan, error)
    Render(ctx Context, plan *RenderPlan) (*RenderResult, error)
    ManagedPaths(ctx Context, plan *RenderPlan) []ManagedPath
}
```

`RenderInput` contains active ref, runtime, Agent projection, resolved
capabilities, project root, active dir, and runtime boundary. `RuntimeHome`
remains as a compatibility field; adapters should prefer `Boundary`.

## Mapping Status

Every adapter must report how fields map:

- `native`
- `rendered_as_instructions`
- `ignored`
- `unsupported`

Adapters no longer expose memory import capabilities and do not render AVM
memory refs.

## Supported Runtime Scope

- Codex: isolated config + role files.
- Claude Code: agent markdown + MCP file.
- OpenCode: config, agent, skills, MCP.
- Cline: rules + MCP settings.
- Cursor: conservative rules/MCP proof of concept.
