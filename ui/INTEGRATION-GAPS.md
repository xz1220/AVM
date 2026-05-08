# AVM UI Integration Gaps

This file records UI-facing protocol gaps found while building `avm-ui`.
Items are removed once the Go CLI publishes a stable JSON command for
them. The TUI mocks the missing surface behind `AvmClient` so UI work
can proceed in parallel with backend work.

## Resolved (already shipped)

These were listed earlier as missing but are now provided by the Go CLI:

- `avm capability discover --json [--kind ...] [--runtime ...]` →
  `[]CapabilityCandidate`. Implemented at
  `internal/presentation/cli/capability.go`.
- `avm capability import --runtime ... --kind ... --name ... --json` →
  `ImportCapabilityResult`. Conflict path returns `CAPABILITY_CONFLICT`
  envelope; UI re-issues with `--on-conflict skip|overwrite|cancel`.
- `avm capability list --json` and `avm capability show <id> --json` →
  capstore-only resolution path so UIs can render names/sources for the
  capability IDs an Agent already references without paying the cost of
  a full runtime probe.
- `avm runtime list --json` → `[]RuntimeCheck`. Use this for the
  runtime picker in create/edit; do not parse `avm doctor`.
- `agent create`/`edit` enforce at least one runtime in the application
  layer. UIs can rely on `MISSING_INPUT` with `details.field="runtime"`
  if a draft is submitted with zero runtimes.
- `agent delete --json` returns the literal JSON value `null` (no human
  success line). UIs can `JSON.parse(stdout)` uniformly.

## Open

(none — Phase 1 protocol surface complete.)

## Deferred to Phase 2

These are recorded as future work, not blockers:

- **Agent draft preview / validate** before save. Phase 1 UI should do
  `agent create` followed by `agent show` if it needs the runtime
  mapping; this keeps the public command surface small.
- **JSON stdin** for `agent create`/`agent edit` (e.g. `--input -`).
  Worth doing once `Instructions.Inline` and `Instructions.Files` enter
  the UI; the public DTO should be a separate `AgentDraftV*` type, not
  the internal `CreateAgentRequest` struct, to keep service evolution
  cheap.
- **`Instructions.Inline` / `Instructions.Files` flags** for
  `agent create`/`edit`. Will land together with JSON stdin.
