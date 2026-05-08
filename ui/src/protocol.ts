import {z} from "zod";

export const CapabilityKindSchema = z.enum(["skill", "mcp"]);
export type CapabilityKind = z.infer<typeof CapabilityKindSchema>;

export const CapabilitySourceSchema = z.enum(["avm", "package", "runtime"]);
export type CapabilitySource = z.infer<typeof CapabilitySourceSchema>;

export const CapabilityRefSchema = z.object({
  id: z.string(),
  kind: CapabilityKindSchema
});
export type CapabilityRef = z.infer<typeof CapabilityRefSchema>;

export const RuntimePrefSchema = z.object({
  runtime: z.string(),
  default: z.boolean().optional()
});
export type RuntimePref = z.infer<typeof RuntimePrefSchema>;

export const InstructionsSchema = z.object({
  system: z.string().optional(),
  files: z.array(z.string()).optional(),
  inline: z.string().optional()
});
export type Instructions = z.infer<typeof InstructionsSchema>;

export const AgentSchema = z.object({
  identity: z.object({
    name: z.string(),
    description: z.string().optional(),
    role: z.string().optional()
  }),
  instructions: InstructionsSchema.optional().default({}),
  skills: z.array(CapabilityRefSchema).optional().default([]),
  mcp: z.array(CapabilityRefSchema).optional().default([]),
  runtimes: z.array(RuntimePrefSchema).optional().default([])
});
export type Agent = z.infer<typeof AgentSchema>;

export const AgentSummarySchema = z.object({
  name: z.string(),
  description: z.string().optional(),
  runtimes: z.array(z.string()).optional().default([])
});
export type AgentSummary = z.infer<typeof AgentSummarySchema>;

export const FieldMappingSummarySchema = z.object({
  field: z.string(),
  status: z.string(),
  note: z.string().optional()
});

export const RuntimeMappingSummarySchema = z.object({
  runtime: z.string(),
  fields: z.array(FieldMappingSummarySchema).optional().default([]),
  warnings: z.array(z.string()).optional().default([])
});
export type RuntimeMappingSummary = z.infer<typeof RuntimeMappingSummarySchema>;

export const AgentDetailSchema = z.object({
  agent: AgentSchema,
  source_path: z.string().optional(),
  mapping: z.array(RuntimeMappingSummarySchema).optional().default([])
});
export type AgentDetail = z.infer<typeof AgentDetailSchema>;

export const CapabilityRecordSchema = z.object({
  id: z.string(),
  kind: CapabilityKindSchema,
  name: z.string(),
  version: z.string().optional(),
  source: CapabilitySourceSchema,
  checksum: z.string().optional(),
  import_from: z.string().optional(),
  format: z.string().optional()
});
export type CapabilityRecord = z.infer<typeof CapabilityRecordSchema>;

export const GlobalCapabilitySchema = z.object({
  runtime: z.string(),
  kind: CapabilityKindSchema,
  name: z.string(),
  path: z.string().optional(),
  version: z.string().optional()
});
export type GlobalCapability = z.infer<typeof GlobalCapabilitySchema>;

export const CapabilityCandidateSchema = z.object({
  kind: CapabilityKindSchema,
  name: z.string(),
  source: CapabilitySourceSchema,
  record: CapabilityRecordSchema.optional(),
  global: GlobalCapabilitySchema.optional(),
  conflict: z.boolean().optional(),
  imported: z.boolean().optional()
});
export type CapabilityCandidate = z.infer<typeof CapabilityCandidateSchema>;

export const ImportCapabilityResultSchema = z.object({
  id: z.string(),
  created: z.boolean().optional().default(false),
  replaced: z.boolean().optional(),
  source: z.string().optional()
});
export type ImportCapabilityResult = z.infer<typeof ImportCapabilityResultSchema>;

export const RuntimeCheckSchema = z.object({
  runtime: z.string(),
  available: z.boolean(),
  binary: z.string().optional(),
  version: z.string().optional(),
  issues: z.array(z.string()).optional().default([])
});
export type RuntimeCheck = z.infer<typeof RuntimeCheckSchema>;

export const DoctorReportSchema = z.object({
  runtimes: z.array(RuntimeCheckSchema).optional().default([])
});

export const AvmErrorSchema = z.object({
  code: z.string(),
  message: z.string(),
  details: z.record(z.string(), z.unknown()).optional()
});
export type AvmErrorPayload = z.infer<typeof AvmErrorSchema>;

export const AvmErrorEnvelopeSchema = z.object({
  error: AvmErrorSchema
});

export type AgentWriteInput = {
  name: string;
  description: string;
  role: string;
  system: string;
  skills: CapabilityRef[];
  mcp: CapabilityRef[];
  runtimes: RuntimePref[];
};
