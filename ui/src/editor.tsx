import React, {useEffect, useMemo, useState} from "react";
import {Box, Text, useInput} from "ink";
import TextInput from "ink-text-input";
import Fuse from "fuse.js";
import type {AvmClient} from "./avm-client.js";
import type {AgentDetail, AgentWriteInput, CapabilityCandidate, CapabilityKind, CapabilityRef, RuntimeCheck, RuntimePref} from "./protocol.js";
import {capabilityRefID} from "./mock-capabilities.js";
import {ErrorText, Frame, Muted, Pill, SectionTitle, truncate, windowSlice} from "./components.js";
import {theme} from "./theme.js";

type EditorMode = "create" | "edit";
type Step = "basic" | "runtime" | "instructions" | "skills" | "mcp" | "review";

type Draft = {
  name: string;
  description: string;
  role: string;
  system: string;
  runtimes: RuntimePref[];
  skills: CapabilityRef[];
  mcp: CapabilityRef[];
};

const steps: Step[] = ["basic", "runtime", "instructions", "skills", "mcp", "review"];

export function AgentEditor(props: {
  mode: EditorMode;
  client: AvmClient;
  initial?: AgentDetail;
  runtimes: RuntimeCheck[];
  onCancel: () => void;
  onSaved: (name: string) => void;
}) {
  const [draft, setDraft] = useState<Draft>(() => initialDraft(props.mode, props.initial));
  const [step, setStep] = useState<Step>("basic");
  const [status, setStatus] = useState<string | undefined>();
  const [saving, setSaving] = useState(false);
  const [capabilities, setCapabilities] = useState<CapabilityCandidate[]>([]);

  const selectedRuntimeNames = draft.runtimes.map((runtime) => runtime.runtime);
  const toggleCapability = async (
    kind: CapabilityKind,
    selected: CapabilityRef[],
    setSelected: (refs: CapabilityRef[]) => void,
    candidate: CapabilityCandidate
  ) => {
    const recordID = candidate.record?.id;
    if (recordID) {
      const exists = selected.some((ref) => ref.id === recordID);
      setSelected(exists
        ? selected.filter((ref) => ref.id !== recordID)
        : [...selected, {id: recordID, kind}]
      );
      return;
    }
    if (!candidate.global) return;

    setStatus(`importing ${candidate.name} from ${candidate.global.runtime}`);
    try {
      const imported = await props.client.importCapability({
        runtime: candidate.global.runtime,
        kind,
        name: candidate.name
      });
      const exists = selected.some((ref) => ref.id === imported.id);
      setSelected(exists
        ? selected.filter((ref) => ref.id !== imported.id)
        : [...selected, {id: imported.id, kind}]
      );
      setCapabilities((current) => upsertImportedCandidate(current, candidate, imported.id, imported.source));
      setStatus(undefined);
    } catch (error: unknown) {
      setStatus(error instanceof Error ? error.message : String(error));
    }
  };

  useEffect(() => {
    let cancelled = false;
    props.client.discoverCapabilities({
      kinds: ["skill", "mcp"],
      runtimes: selectedRuntimeNames
    }).then((candidates) => {
      if (!cancelled) setCapabilities(candidates);
    }).catch((error: unknown) => {
      if (!cancelled) setStatus(error instanceof Error ? error.message : String(error));
    });
    return () => {
      cancelled = true;
    };
  }, [props.client, selectedRuntimeNames.join(",")]);

  const stepIndex = steps.indexOf(step);
  const next = () => setStep(steps[Math.min(steps.length - 1, stepIndex + 1)] ?? "review");
  const previous = () => setStep(steps[Math.max(0, stepIndex - 1)] ?? "basic");

  useInput((_input, key) => {
    if (key.escape) {
      props.onCancel();
    }
  });

  const save = async () => {
    const validation = validateDraft(draft);
    if (validation) {
      setStatus(validation);
      return;
    }
    setSaving(true);
    setStatus(undefined);
    try {
      const input = toAgentInput(draft);
      const saved = props.mode === "create"
        ? await props.client.createAgent(input)
        : await props.client.editAgent(props.initial?.agent.identity.name ?? draft.name, input);
      props.onSaved(saved.identity.name);
    } catch (error: unknown) {
      setStatus(error instanceof Error ? error.message : String(error));
    } finally {
      setSaving(false);
    }
  };

  return (
    <Frame
      title={props.mode === "create" ? "Create Agent" : `Edit ${draft.name}`}
      subtitle="Agent CRUD"
      status={status}
      actions={[
        ["enter", "accept"],
        ["left/right", "prev/next"],
        ["esc", "cancel"]
      ]}
    >
      <Box flexGrow={1}>
        <Box width={22} borderStyle="single" borderColor={theme.muted} flexDirection="column" paddingX={1}>
          {steps.map((item, index) => (
            <Text key={item} color={item === step ? theme.accent : undefined} bold={item === step}>
              {index + 1}. {labelForStep(item)}
            </Text>
          ))}
        </Box>
        <Box flexGrow={1} borderStyle="single" borderColor={theme.muted} paddingX={1} flexDirection="column">
          {step === "basic" ? <BasicStep mode={props.mode} draft={draft} setDraft={setDraft} onNext={next} /> : null}
          {step === "runtime" ? <RuntimeStep draft={draft} setDraft={setDraft} runtimes={props.runtimes} onNext={next} onPrev={previous} /> : null}
          {step === "instructions" ? <InstructionsStep draft={draft} setDraft={setDraft} onNext={next} /> : null}
          {step === "skills" ? (
            <CapabilityStep
              kind="skill"
              selected={draft.skills}
              candidates={capabilities.filter((candidate) => candidate.kind === "skill")}
              onToggle={(candidate) => toggleCapability("skill", draft.skills, (skills) => setDraft({...draft, skills}), candidate)}
              onNext={next}
              onPrev={previous}
            />
          ) : null}
          {step === "mcp" ? (
            <CapabilityStep
              kind="mcp"
              selected={draft.mcp}
              candidates={capabilities.filter((candidate) => candidate.kind === "mcp")}
              onToggle={(candidate) => toggleCapability("mcp", draft.mcp, (mcp) => setDraft({...draft, mcp}), candidate)}
              onNext={next}
              onPrev={previous}
            />
          ) : null}
          {step === "review" ? <ReviewStep draft={draft} saving={saving} onSave={save} onPrev={previous} /> : null}
        </Box>
      </Box>
    </Frame>
  );
}

function BasicStep(props: {
  mode: EditorMode;
  draft: Draft;
  setDraft: (draft: Draft) => void;
  onNext: () => void;
}) {
  const fields = props.mode === "create" ? ["name", "description", "role"] as const : ["description", "role"] as const;
  const [index, setIndex] = useState(0);
  const field = fields[index] ?? fields[0];
  const value = props.draft[field] ?? "";

  return (
    <Box flexDirection="column">
      <SectionTitle>Basic</SectionTitle>
      {props.mode === "edit" ? <Muted>name: {props.draft.name}</Muted> : null}
      <FieldLine active={field === "name"} label="name" value={props.draft.name} hidden={props.mode === "edit"} />
      <FieldLine active={field === "description"} label="description" value={props.draft.description} />
      <FieldLine active={field === "role"} label="role" value={props.draft.role} />
      <Box marginTop={1}>
        <Text color={theme.accent}>{field}: </Text>
        <TextInput
          value={value}
          onChange={(nextValue) => props.setDraft({...props.draft, [field]: nextValue})}
          onSubmit={() => {
            if (index < fields.length - 1) {
              setIndex(index + 1);
            } else {
              props.onNext();
            }
          }}
        />
      </Box>
    </Box>
  );
}

function FieldLine(props: {active: boolean; label: string; value: string; hidden?: boolean}) {
  if (props.hidden) return null;
  return (
    <Text color={props.active ? theme.accent : undefined}>
      {props.active ? "> " : "  "}{props.label}: {props.value || "-"}
    </Text>
  );
}

function RuntimeStep(props: {
  draft: Draft;
  setDraft: (draft: Draft) => void;
  runtimes: RuntimeCheck[];
  onNext: () => void;
  onPrev: () => void;
}) {
  const [cursor, setCursor] = useState(0);
  const options = props.runtimes.length > 0 ? props.runtimes : [{runtime: "codex", available: true, issues: []}];

  useInput((input, key) => {
    if (key.downArrow || input === "j") {
      setCursor(Math.min(options.length - 1, cursor + 1));
    } else if (key.upArrow || input === "k") {
      setCursor(Math.max(0, cursor - 1));
    } else if (input === " ") {
      const option = options[cursor];
      if (!option) return;
      const exists = props.draft.runtimes.some((runtime) => runtime.runtime === option.runtime);
      const next = exists
        ? props.draft.runtimes.filter((runtime) => runtime.runtime !== option.runtime)
        : [...props.draft.runtimes, {runtime: option.runtime, default: props.draft.runtimes.length === 0}];
      props.setDraft({...props.draft, runtimes: normalizeDefaults(next)});
    } else if (input === "d") {
      const option = options[cursor];
      if (!option) return;
      props.setDraft({
        ...props.draft,
        runtimes: props.draft.runtimes.map((runtime) => ({
          runtime: runtime.runtime,
          default: runtime.runtime === option.runtime
        }))
      });
    } else if (key.return || key.rightArrow) {
      props.onNext();
    } else if (key.leftArrow) {
      props.onPrev();
    }
  });

  return (
    <Box flexDirection="column">
      <SectionTitle>Runtime</SectionTitle>
      <Muted>space toggles, d marks default</Muted>
      {options.map((option, index) => {
        const selected = props.draft.runtimes.find((runtime) => runtime.runtime === option.runtime);
        return (
          <Text key={option.runtime} color={index === cursor ? theme.accent : undefined}>
            {index === cursor ? "> " : "  "}[{selected ? "x" : " "}] {option.runtime}
            {selected?.default ? " default" : ""}
            {option.available ? "" : " unavailable"}
          </Text>
        );
      })}
    </Box>
  );
}

function InstructionsStep(props: {
  draft: Draft;
  setDraft: (draft: Draft) => void;
  onNext: () => void;
}) {
  return (
    <Box flexDirection="column">
      <SectionTitle>Instructions</SectionTitle>
      <Muted>system instructions</Muted>
      <Box marginTop={1}>
        <Text color={theme.accent}>system: </Text>
        <TextInput
          value={props.draft.system}
          onChange={(system) => props.setDraft({...props.draft, system})}
          onSubmit={props.onNext}
        />
      </Box>
    </Box>
  );
}

const CAPABILITY_PAGE = 12;

function CapabilityStep(props: {
  kind: CapabilityKind;
  selected: CapabilityRef[];
  candidates: CapabilityCandidate[];
  onToggle: (candidate: CapabilityCandidate) => void;
  onNext: () => void;
  onPrev: () => void;
}) {
  const [query, setQuery] = useState("");
  const [cursor, setCursor] = useState(0);
  const filtered = useMemo(() => {
    if (query.trim() === "") {
      return props.candidates;
    }
    const fuse = new Fuse(props.candidates, {
      keys: ["name", "source", "record.id", "global.runtime"],
      threshold: 0.35
    });
    return fuse.search(query).map((result) => result.item);
  }, [props.candidates, query]);

  const selectedIDs = new Set(props.selected.map((ref) => ref.id));

  useInput((input, key) => {
    if (key.downArrow) {
      setCursor(Math.min(filtered.length - 1, cursor + 1));
    } else if (key.upArrow) {
      setCursor(Math.max(0, cursor - 1));
    } else if (key.backspace || key.delete) {
      setQuery(query.slice(0, -1));
      setCursor(0);
    } else if (key.tab) {
      const candidate = filtered[cursor];
      if (!candidate) return;
      props.onToggle(candidate);
    } else if (key.return) {
      props.onNext();
    } else if (key.rightArrow) {
      props.onNext();
    } else if (key.leftArrow) {
      props.onPrev();
    } else if (input.length === 1 && !key.ctrl && !key.meta) {
      setQuery(`${query}${input}`);
      setCursor(0);
    }
  });

  const {visible, start, before, after} = windowSlice(filtered, cursor, CAPABILITY_PAGE);

  return (
    <Box flexDirection="column">
      <SectionTitle>{props.kind === "skill" ? "Skills" : "MCP"}</SectionTitle>
      <Box>
        <Text color={theme.accent}>filter: </Text>
        <Text>{query || "-"}</Text>
      </Box>
      <Muted>{props.selected.length} selected · type to filter · tab toggles · enter / → next · ← prev</Muted>
      {before > 0 ? <Muted>↑ {before} more</Muted> : null}
      {visible.map((candidate, offset) => {
        const index = start + offset;
        const id = capabilityRefID(candidate);
        const selected = selectedIDs.has(id);
        return (
          <Text key={id} color={index === cursor ? theme.accent : undefined}>
            {index === cursor ? "> " : "  "}[{selected ? "x" : " "}] {candidate.name}{" "}
            <Pill color={candidate.source === "runtime" ? theme.warn : theme.ok}>
              {candidate.source}{candidate.global?.runtime ? `:${candidate.global.runtime}` : ""}
            </Pill>
            {candidate.conflict ? " conflict" : ""}
          </Text>
        );
      })}
      {after > 0 ? <Muted>↓ {after} more</Muted> : null}
      {filtered.length === 0
        ? <Muted>no candidates</Muted>
        : <Muted>{cursor + 1}/{filtered.length}</Muted>}
    </Box>
  );
}

function upsertImportedCandidate(
  candidates: CapabilityCandidate[],
  candidate: CapabilityCandidate,
  id: string,
  importFrom?: string
): CapabilityCandidate[] {
  const withoutSameGlobal = candidates.filter((item) => {
    return !(item.kind === candidate.kind &&
      item.name === candidate.name &&
      item.global?.runtime === candidate.global?.runtime);
  });
  if (withoutSameGlobal.some((item) => item.record?.id === id)) {
    return withoutSameGlobal;
  }
  return [{
    kind: candidate.kind,
    name: candidate.name,
    source: "runtime",
    record: {
      id,
      kind: candidate.kind,
      name: candidate.name,
      version: candidate.global?.version,
      source: "runtime",
      import_from: importFrom
    }
  }, ...candidates];
}

function ReviewStep(props: {
  draft: Draft;
  saving: boolean;
  onSave: () => void;
  onPrev: () => void;
}) {
  useInput((input, key) => {
    if (key.leftArrow) {
      props.onPrev();
    } else if (key.return || input === "s") {
      void props.onSave();
    }
  });

  const validation = validateDraft(props.draft);
  return (
    <Box flexDirection="column">
      <SectionTitle>Review</SectionTitle>
      <Text>name: {props.draft.name || "-"}</Text>
      <Text>description: {props.draft.description || "-"}</Text>
      <Text>role: {props.draft.role || "-"}</Text>
      <Text>runtime: {props.draft.runtimes.map((runtime) => runtime.default ? `${runtime.runtime}*` : runtime.runtime).join(", ") || "-"}</Text>
      <Text>skills: {props.draft.skills.length}</Text>
      <Text>mcp: {props.draft.mcp.length}</Text>
      <Text>system: {truncate(props.draft.system, 120) || "-"}</Text>
      <Box marginTop={1}>
        {validation ? <ErrorText>{validation}</ErrorText> : <Text color={theme.ok}>{props.saving ? "saving..." : "press enter to save"}</Text>}
      </Box>
    </Box>
  );
}

export function DeleteConfirm(props: {
  agent: AgentDetail;
  client: AvmClient;
  onCancel: () => void;
  onDeleted: () => void;
}) {
  const [choice, setChoice] = useState<"cancel" | "delete">("cancel");
  const [status, setStatus] = useState<string | undefined>();
  const [submitting, setSubmitting] = useState(false);
  const agent = props.agent.agent;

  useInput((input, key) => {
    if (key.escape) {
      props.onCancel();
    } else if (key.leftArrow || key.rightArrow || key.tab) {
      setChoice(choice === "cancel" ? "delete" : "cancel");
    } else if (key.return) {
      if (choice === "cancel") {
        props.onCancel();
        return;
      }
      setSubmitting(true);
      props.client.deleteAgent(agent.identity.name)
        .then(props.onDeleted)
        .catch((error: unknown) => setStatus(error instanceof Error ? error.message : String(error)))
        .finally(() => setSubmitting(false));
    }
  });

  return (
    <Frame
      title="Delete Agent"
      subtitle={agent.identity.name}
      status={status}
      actions={[
        ["left/right", "choose"],
        ["enter", "confirm"],
        ["esc", "cancel"]
      ]}
    >
      <Box flexGrow={1} justifyContent="center" alignItems="center" flexDirection="column">
        <Box borderStyle="single" borderColor={theme.danger} paddingX={2} paddingY={1} flexDirection="column" width={70}>
          <Text color={theme.danger} bold>Delete {agent.identity.name}?</Text>
          <Text>{agent.identity.description || "-"}</Text>
          <Text color={theme.muted}>runtimes: {agent.runtimes.map((runtime) => runtime.runtime).join(", ") || "-"}</Text>
          <Text color={theme.muted}>skills: {agent.skills.length}  mcp: {agent.mcp.length}</Text>
          <Box marginTop={1}>
            <Text color={choice === "cancel" ? theme.accent : undefined}>[ Cancel ]</Text>
            <Text>  </Text>
            <Text color={choice === "delete" ? theme.danger : undefined}>[ Delete ]</Text>
          </Box>
          {submitting ? <Muted>deleting...</Muted> : null}
        </Box>
      </Box>
    </Frame>
  );
}

function labelForStep(step: Step): string {
  switch (step) {
    case "basic": return "Basic";
    case "runtime": return "Runtime";
    case "instructions": return "Instructions";
    case "skills": return "Skills";
    case "mcp": return "MCP";
    case "review": return "Review";
  }
}

function initialDraft(mode: EditorMode, detail?: AgentDetail): Draft {
  if (mode === "edit" && detail) {
    const agent = detail.agent;
    return {
      name: agent.identity.name,
      description: agent.identity.description ?? "",
      role: agent.identity.role ?? "",
      system: agent.instructions.system ?? "",
      runtimes: agent.runtimes,
      skills: agent.skills,
      mcp: agent.mcp
    };
  }
  return {
    name: "",
    description: "",
    role: "",
    system: "",
    runtimes: [],
    skills: [],
    mcp: []
  };
}

function toAgentInput(draft: Draft): AgentWriteInput {
  return {
    name: draft.name.trim(),
    description: draft.description,
    role: draft.role,
    system: draft.system,
    runtimes: normalizeDefaults(draft.runtimes),
    skills: draft.skills,
    mcp: draft.mcp
  };
}

function validateDraft(draft: Draft): string | undefined {
  if (!/^[a-z][a-z0-9-]{0,62}$/.test(draft.name.trim())) {
    return "name must match [a-z][a-z0-9-]{0,62}";
  }
  if (draft.runtimes.length === 0) {
    return "select at least one runtime";
  }
  return undefined;
}

function normalizeDefaults(runtimes: RuntimePref[]): RuntimePref[] {
  if (runtimes.length === 0) {
    return [];
  }
  const hasDefault = runtimes.some((runtime) => runtime.default);
  if (hasDefault) {
    return runtimes.map((runtime, index) => ({
      runtime: runtime.runtime,
      default: runtime.default && index === runtimes.findIndex((item) => item.default)
    }));
  }
  return runtimes.map((runtime, index) => ({
    runtime: runtime.runtime,
    default: index === 0
  }));
}
