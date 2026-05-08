# AVM CLI Protocol

This document is the **stable contract** between the AVM Go CLI and any
external consumer (the TS/JS UI in `ui/`, shell scripts, CI). The Go CLI
is plumbing only — it never prompts. All input comes from flags or
stdin; all output goes to stdout (success) or stderr (errors, human mode).

> **Versioning rule:** changing a field name in JSON output, removing
> an error code, or changing exit-code semantics is a breaking change.
> Adding new fields, codes, or commands is non-breaking.

## 1. Output modes

The root persistent flag `--json` switches between two output modes.
Any subcommand inherits it.

| Mode | Default | Stream | Format |
| --- | --- | --- | --- |
| Human | yes | stdout | column-aligned text, ASCII only |
| JSON | `--json` | stdout | indented JSON; errors envelope on failure |

Non-zero exit code still means "the command failed". JSON mode does
not change that — it only changes how the failure is described.

## 2. Exit codes

| Code | Meaning |
| --- | --- |
| `0` | Success |
| non-zero | Failure. Specific code is the runtime process exit code for `avm run`; for other commands it is `1`. |

`avm run <agent>` propagates the runtime's own exit code so shell
scripts can branch on it (`avm run x; echo $?`).

## 3. Success output (JSON mode)

Every command emits the corresponding `internal/app/model` value as a
JSON object. Field names are `snake_case`. Empty optional fields are
omitted (`omitempty`).

> **Empty list payloads:** when a command's success type is a slice
> (`[]AgentSummary`, `[]CapabilityRecord`, `[]RuntimeCheck`, ...) and
> there are zero items, the JSON value may be either `[]` or `null`.
> Existing stores generally emit `[]`; missing/uninitialized backing
> directories may surface Go's nil slice as `null`. Consumers must handle
> both as an empty list.

| Command | Success payload |
| --- | --- |
| `avm agent create` | `Agent` |
| `avm agent list` | `[]AgentSummary` |
| `avm agent show <name>` | `AgentDetail` |
| `avm agent edit <name>` | `Agent` |
| `avm agent delete <name>` | literal `null` |
| `avm agent clone <name> --name <new>` | `Agent` |
| `avm agent rename <old> <new>` | `Agent` |
| `avm run <agent> --preview` | `RunPreview` |
| `avm run <agent>` | `RunResult` |
| `avm package list` | `[]PackageSummary` |
| `avm package show <name>` | `PackageDetail` |
| `avm package install <file>` | `InstallResult` |
| `avm package uninstall <name>` | literal `null` |
| `avm package export <agent>` | `ExportResult` |
| `avm package inspect <file>` | `PackageDetail` |
| `avm capability discover` | `[]CapabilityCandidate` |
| `avm capability list` | `[]CapabilityRecord` |
| `avm capability show <id>` | `CapabilityRecord` |
| `avm capability import` | `ImportCapabilityResult` |
| `avm capability bootstrap` | `BootstrapCapabilitiesResult` |
| `avm runtime list` | `[]RuntimeCheck` |
| `avm doctor` | `DoctorReport` |
| `avm status [agent]` | `StatusReport` |
| `avm init` | `InitResult` (currently human-only; JSON support TBD) |
| `avm uninstall --yes` | `UninstallResult` (TBD as above) |

The model types live in `internal/app/model/`. JSON tags on the Go
structs are the source of truth for field names. See:

- `agent.go` — `Agent`, `Identity`, `Instructions`, `CapabilityRef`, `RuntimePref`, `AgentSummary`, `AgentDetail`, `RuntimeMappingSummary`, `FieldMappingSummary`
- `capability.go` — `CapabilityID`, `CapabilityKind`, `CapabilitySource`, `CapabilityRecord`, `GlobalCapability`, `CapabilityCandidate`
- `run.go` — `RunRequest`, `RunPreview`, `RunResult`, `RunRecord`, `BoundarySummary`, `MappingStatus`, `DriftPolicy`, `DiffEntry`, `Warning`
- `package.go` — `PackageManifest`, `PackageSummary`, `PackageDetail`, `InstallRequest`/`Result`, `ExportRequest`/`Result`, `ConflictResolution`
- `requests.go` — `ImportCapabilityRequest`/`Result`, `BootstrapCapabilitiesRequest`/`Result`, `SkippedCapability`
- `diagnostics.go` — `DoctorReport`, `StatusReport`, `RuntimeCheck`, `CheckResult`
- `system.go` — `InitResult`, `UninstallResult`

## 4. Error envelope (JSON mode)

On failure, JSON mode writes a single envelope to stdout and exits
non-zero:

```json
{
  "error": {
    "code": "AGENT_CONFLICT",
    "message": "agent \"alpha\" already exists",
    "details": {
      "name": "alpha"
    }
  }
}
```

| Field | Type | Meaning |
| --- | --- | --- |
| `code` | string (constant) | machine-readable identifier; see §5 |
| `message` | string | human-readable, may include the offending value |
| `details` | object \| omitted | structured context, code-specific shape |

Human mode prints `avm: <message>` to stderr instead.

## 5. Error codes

| Code | Used by | `details` shape | Meaning |
| --- | --- | --- | --- |
| `AGENT_CONFLICT` | `agent create`, `agent clone`, `agent rename`, `package install` | `{"name": string}` | An Agent with that name already exists; caller must pick a different name or pass `--on-conflict overwrite`. |
| `AGENT_NOT_FOUND` | `agent show`/`edit`/`delete`/`clone`/`rename`, `run`, `package export`, `package uninstall` | `{"name": string}` | The named Agent does not exist. |
| `AGENT_INVALID_NAME` | `agent create`/`edit`/`clone`/`rename` (when name is non-empty but malformed) | `{"name": string}` | Name failed regex `^[a-z][a-z0-9-]{0,62}$`. |
| `RUNTIME_NOT_FOUND` | `run`, `agent show` mapping render | `{"runtime": string}` | Runtime name not registered in the driver registry. |
| `RUNTIME_AMBIGUOUS` | `run` | `{"agent": string, "runtimes": [string]}` | Agent has multiple runtimes and no `--runtime` was supplied. UI: prompt the user to pick one of `details.runtimes` and re-issue with `--runtime`. |
| `RUNTIME_MISSING` | `run` | `{"agent": string}` | Agent has no runtimes configured at all. Caller can pass `--runtime`. |
| `RUNTIME_BINARY_MISSING` | `run` (driver returned process error) | `{"runtime": string, "agent": string}` | Runtime binary not on PATH or failed to launch. |
| `RUNTIME_PLAN_FAILURE` | `run`, `agent show` | `{"runtime": string, "agent": string}` | Driver's `Plan` or `LaunchSpec` returned an error. |
| `DRIFT_DETECTED` | `run` | `{"agent": string, "runtime": string, "entries": [DiffEntry]}` | Managed config drifted from AVM Agent definition and `--drift` was unset. UI: present `entries` to user, prompt for `keep`/`merge`/`discard`, re-issue with `--drift=<choice>`. |
| `PACKAGE_NOT_FOUND` | reserved for future installed-package registry | `{"name": string}` | Named installed package does not exist. |
| `PACKAGE_INVALID_MANIFEST` | `package install`/`inspect` | `{"file": string}` or `{"path": string}` | Manifest could not be parsed or required fields are missing. |
| `PACKAGE_CHECKSUM_MISMATCH` | reserved | `{"file": string, "want": string, "got": string}` | Capability blob checksum did not match manifest. |
| `CAPABILITY_NOT_FOUND` | `package export`, `capability import` | `{"id": string}` for export; `{"runtime": string, "kind": string, "name": string}` for import | The referenced capability ID / (runtime,kind,name) is unknown. |
| `CAPABILITY_CONFLICT` | `capability import` | `{"kind": string, "name": string, "existing_id": string, "existing_checksum": string}` | A different-content capability with the same `(kind,name)` already lives in capstore. UI: present existing record and prompt for `--on-conflict skip|overwrite`. Same `(kind,name)` across multiple discovery sources is also reflected by the `Conflict: true` flag on `Discover` results. |
| `MISSING_INPUT` | many; common for missing `--name`, `--runtime`, `--yes`, `--shell` | `{"field": string, "hint": string}` | A required input was absent. UI: surface `hint` and re-issue with the field set. `agent create`/`edit` raise this with `field="runtime"` when an Agent would have zero runtime preferences. |
| `VALIDATION` | several; e.g. unknown `--on-conflict` value | varies | Generic validation failure not fitting a narrower code. |
| `IO_FAILURE` | many | varies | Underlying filesystem / zip / network IO failed. |
| `INTERNAL_ERROR` | catch-all | varies | An error the CLI/service does not recognise; treat as bug. |

UI policy hints:
- `AGENT_CONFLICT`: prompt rename / overwrite / cancel; re-issue with `--on-conflict`.
- `RUNTIME_AMBIGUOUS`: select from `details.runtimes`.
- `DRIFT_DETECTED`: present `details.entries` (each is `{path, field, reason}`), prompt for policy.
- `MISSING_INPUT`: surface `details.hint` to the user as guidance for missing input.
- All other codes: render `code` + `message` to the user; no automatic retry.

## 6. Common request patterns

### Agent CRUD

```
avm agent create --name alpha --runtime codex                         # required: --name + at least one --runtime
avm agent create ... --on-conflict overwrite                          # opt into overwriting an existing agent
avm agent edit alpha --description "new desc" --skill cap_x --skill cap_y
                                                                      # any list flag (--skill/--mcp/--runtime) replaces
                                                                      # the whole list. Absent flags keep existing values.
avm agent show alpha --json                                           # read current state for diff/edit flows
avm agent delete alpha --yes                                          # --yes is required (no implicit confirm)
```

### Run flow (UI two-step)

```
# Step 1: ask Preview
avm run <agent> --preview --json
  → success: render plan
  → AGENT_NOT_FOUND / RUNTIME_AMBIGUOUS / DRIFT_DETECTED → prompt + re-issue

# Step 2: launch
avm run <agent> --runtime <r> --drift keep
  → stdout (JSON mode): RunResult; exit code = runtime exit
```

### Package install flow

```
avm package inspect <file> --json     # show what will be written
avm package install <file>            # default fails on AGENT_CONFLICT
avm package install <file> --on-conflict {rename|skip|overwrite|cancel}
```

### Capability resolve flow (UI: render IDs an Agent already references)

```
# Cheap capstore-only read. Does not call into runtime drivers, so it is
# safe for hot UI paths like "show me the names behind these IDs".
avm capability list --json                      # []CapabilityRecord
avm capability show <id> --json                 # CapabilityRecord
  → CAPABILITY_NOT_FOUND { "id": <id> } if unknown
```

### Runtime picker flow (UI: pick a runtime when creating an Agent)

```
# Non-diagnostic runtime listing. Same per-runtime payload Doctor uses
# (RuntimeCheck), without home/PATH/shell-integration noise.
avm runtime list --json                         # []RuntimeCheck
```

### Capability discover / import flow

```
# 1. See every capability AVM can find — AVM-managed records plus
#    runtime-global discoveries. Imported=true on a runtime-global
#    candidate means "already in capstore, no need to import again".
avm capability discover --json
avm capability discover --runtime codex --kind skill

# 2. Import a single runtime-global capability into capstore.
avm capability import --runtime codex --kind skill --name hello --json
  → success: ImportCapabilityResult { id, created, replaced, source }
  → CAPABILITY_NOT_FOUND if the runtime doesn't expose this (kind,name)
  → CAPABILITY_CONFLICT if (kind,name) already exists with different content
    → re-issue with --on-conflict {skip|overwrite}

# 3. First-install bootstrap: import every runtime-global capability
#    a runtime exposes. Per-item failures land in `skipped` and never
#    abort the run.
avm capability bootstrap --runtime codex --json
  → BootstrapCapabilitiesResult { imported: [...], skipped: [...] }
```

## 7. Stability guarantees

| Surface | Stability |
| --- | --- |
| Command names + flag names | Stable. Removing or renaming = breaking. |
| Error codes (table in §5) | Stable. Removing or renaming = breaking. Adding new codes = non-breaking. |
| JSON field names on `internal/app/model` types | Stable. |
| Error `details` shape per code | Best-effort stable. Adding new keys = non-breaking; removing is breaking. |
| Exit codes | `0` = success, non-zero = failure. `avm run` propagates runtime exit code. |
| Human-mode output text | NOT stable. Scripts must use `--json`. |

## 8. Where the contract is enforced

- JSON schemas: implicit, derived from Go struct tags in `internal/app/model/`.
- Error code constants: `internal/app/service/errors.go` `Code*`.
- CLI error envelope wrapper: `internal/presentation/cli/root.go::renderError`.
- Test for JSON envelope (general): `internal/presentation/cli/agent_test.go::TestJSONError_Envelope`.
- Test for CAPABILITY_CONFLICT envelope: `internal/presentation/cli/capability_test.go::TestCapabilityImport_ConflictEnvelope`.

If you change any of these, update this document in the same PR.
