import {spawn} from "node:child_process";
import {AgentDetailSchema, AgentSchema, AgentSummarySchema, AvmErrorEnvelopeSchema, CapabilityCandidateSchema, CapabilityRecordSchema, ImportCapabilityResultSchema, RuntimeCheckSchema, type Agent, type AgentDetail, type AgentSummary, type AgentWriteInput, type CapabilityCandidate, type CapabilityKind, type CapabilityRecord, type ImportCapabilityResult, type RuntimeCheck} from "./protocol.js";
import {mockCapabilityCandidates} from "./mock-capabilities.js";
import {z} from "zod";

const AgentSummaryListSchema = nullableArray(AgentSummarySchema);
const RuntimeCheckListSchema = nullableArray(RuntimeCheckSchema);
const CapabilityCandidateListSchema = nullableArray(CapabilityCandidateSchema);
const CapabilityRecordListSchema = nullableArray(CapabilityRecordSchema);

export class AvmCommandError extends Error {
  readonly code: string;
  readonly details?: Record<string, unknown>;
  readonly stderr: string;

  constructor(input: {code: string; message: string; details?: Record<string, unknown>; stderr?: string}) {
    super(input.message);
    this.name = "AvmCommandError";
    this.code = input.code;
    this.details = input.details;
    this.stderr = input.stderr ?? "";
  }
}

export class AvmClient {
  constructor(readonly binary: string) {}

  async listAgents(): Promise<AgentSummary[]> {
    return this.runJson(["agent", "list"], AgentSummaryListSchema);
  }

  async showAgent(name: string): Promise<AgentDetail> {
    return this.runJson(["agent", "show", name], AgentDetailSchema);
  }

  async createAgent(input: AgentWriteInput): Promise<Agent> {
    return this.runJson(["agent", "create", ...agentCreateArgs(input)], AgentSchema);
  }

  async editAgent(name: string, input: AgentWriteInput): Promise<Agent> {
    return this.runJson(["agent", "edit", name, ...agentEditArgs(input)], AgentSchema);
  }

  async deleteAgent(name: string): Promise<void> {
    await this.runVoid(["agent", "delete", name, "--yes"]);
  }

  async listRuntimes(): Promise<RuntimeCheck[]> {
    try {
      return await this.runJson(["runtime", "list"], RuntimeCheckListSchema);
    } catch {
      // Keep the TUI usable with older avm binaries while the Go protocol rolls forward.
    }
    return [
      {runtime: "codex", available: true, issues: []},
      {runtime: "claude-code", available: true, issues: []},
      {runtime: "opencode", available: true, issues: []}
    ];
  }

  async discoverCapabilities(input: {
    kinds?: CapabilityKind[];
    runtimes?: string[];
  }): Promise<CapabilityCandidate[]> {
    try {
      return await this.runJson([
        "capability", "discover",
        ...repeatFlag("--kind", input.kinds ?? []),
        ...repeatFlag("--runtime", input.runtimes ?? [])
      ], CapabilityCandidateListSchema);
    } catch (error: unknown) {
      if (!isLegacySurfaceError(error)) {
        throw error;
      }
    }
    return z.array(CapabilityCandidateSchema).parse(mockCapabilityCandidates(input));
  }

  async listCapabilities(): Promise<CapabilityRecord[]> {
    return this.runJson(["capability", "list"], CapabilityRecordListSchema);
  }

  async showCapability(id: string): Promise<CapabilityRecord> {
    return this.runJson(["capability", "show", id], CapabilityRecordSchema);
  }

  async importCapability(input: {
    runtime: string;
    kind: CapabilityKind;
    name: string;
  }): Promise<ImportCapabilityResult> {
    return this.runJson([
      "capability", "import",
      "--runtime", input.runtime,
      "--kind", input.kind,
      "--name", input.name
    ], ImportCapabilityResultSchema);
  }

  private async runVoid(args: string[]): Promise<void> {
    await run(this.binary, ["--json", ...args], undefined);
  }

  private async runJson<T>(args: string[], schema: z.ZodType<T>): Promise<T> {
    const stdout = await run(this.binary, ["--json", ...args], schema);
    return stdout;
  }
}

function agentCreateArgs(input: AgentWriteInput): string[] {
  return [
    "--name", input.name,
    ...optionalStringFlag("--description", input.description),
    ...optionalStringFlag("--role", input.role),
    ...optionalStringFlag("--system", input.system),
    ...runtimeArgs(input),
    ...capabilityArgs("--skill", input.skills),
    ...capabilityArgs("--mcp", input.mcp)
  ];
}

function agentEditArgs(input: AgentWriteInput): string[] {
  return [
    "--description", input.description,
    "--role", input.role,
    "--system", input.system,
    ...runtimeArgs(input),
    ...capabilityArgs("--skill", input.skills),
    ...capabilityArgs("--mcp", input.mcp)
  ];
}

function optionalStringFlag(flag: string, value: string): string[] {
  return value.trim() === "" ? [] : [flag, value];
}

function runtimeArgs(input: AgentWriteInput): string[] {
  const args = input.runtimes.flatMap((pref) => ["--runtime", pref.runtime]);
  const defaultRuntime = input.runtimes.find((pref) => pref.default)?.runtime;
  return defaultRuntime ? [...args, "--default-runtime", defaultRuntime] : args;
}

function capabilityArgs(flag: string, refs: {id: string}[]): string[] {
  return refs.flatMap((ref) => [flag, ref.id]);
}

function repeatFlag(flag: string, values: readonly string[]): string[] {
  return values.flatMap((value) => [flag, value]);
}

function nullableArray<T>(schema: z.ZodType<T>) {
  return z.array(schema).nullable().transform((items) => items ?? []);
}

function isLegacySurfaceError(error: unknown): boolean {
  return error instanceof AvmCommandError &&
    (error.code === "SPAWN_FAILED" || error.code.startsWith("EXIT_"));
}

async function run<T>(binary: string, args: string[], schema: z.ZodType<T> | undefined): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    const child = spawn(binary, args, {
      stdio: ["ignore", "pipe", "pipe"]
    });

    let stdout = "";
    let stderr = "";
    child.stdout.on("data", (chunk) => {
      stdout += String(chunk);
    });
    child.stderr.on("data", (chunk) => {
      stderr += String(chunk);
    });
    child.on("error", (error) => {
      reject(new AvmCommandError({
        code: "SPAWN_FAILED",
        message: `failed to spawn ${binary}: ${error.message}`,
        stderr
      }));
    });
    child.on("close", (code) => {
      if (code !== 0) {
        reject(parseError(stdout, stderr, code ?? 1));
        return;
      }
      if (!schema) {
        resolve(undefined as T);
        return;
      }
      try {
        resolve(schema.parse(JSON.parse(stdout)));
      } catch (error) {
        reject(new AvmCommandError({
          code: "INVALID_JSON",
          message: `invalid JSON from avm: ${error instanceof Error ? error.message : String(error)}`,
          stderr
        }));
      }
    });
  });
}

function parseError(stdout: string, stderr: string, exitCode: number): AvmCommandError {
  try {
    const env = AvmErrorEnvelopeSchema.parse(JSON.parse(stdout));
    return new AvmCommandError({
      code: env.error.code,
      message: env.error.message,
      details: env.error.details,
      stderr
    });
  } catch {
    return new AvmCommandError({
      code: `EXIT_${exitCode}`,
      message: stderr.trim() || stdout.trim() || `avm exited with code ${exitCode}`,
      stderr
    });
  }
}
