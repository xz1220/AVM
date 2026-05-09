import React, {useEffect, useMemo, useState} from "react";
import {Box, Text, useInput} from "ink";
import TextInput from "ink-text-input";
import Fuse from "fuse.js";
import type {AgentDetail, AgentSummary, CapabilityRecord, CapabilityRef} from "./protocol.js";
import {ErrorText, Frame, Muted, Pill, SectionTitle, truncate, windowSlice} from "./components.js";
import {theme} from "./theme.js";

const AGENT_PAGE = 6;

export function AgentBrowser(props: {
  agents: AgentSummary[];
  detail?: AgentDetail;
  capabilities: CapabilityRecord[];
  selectedName?: string;
  loading: boolean;
  error?: string;
  onSelect: (name: string) => void;
  onCreate: () => void;
  onEdit: () => void;
  onDelete: () => void;
  onRefresh: () => void;
  onExit: () => void;
}) {
  const [query, setQuery] = useState("");
  const [searching, setSearching] = useState(false);

  const filtered = useMemo(() => {
    if (query.trim() === "") {
      return props.agents;
    }
    const fuse = new Fuse(props.agents, {
      keys: ["name", "description", "runtimes"],
      threshold: 0.35
    });
    return fuse.search(query).map((result) => result.item);
  }, [props.agents, query]);

  const selectedIndex = Math.max(0, filtered.findIndex((agent) => agent.name === props.selectedName));

  useEffect(() => {
    if (filtered.length > 0 && !filtered.some((agent) => agent.name === props.selectedName)) {
      const first = filtered[0];
      if (first) {
        props.onSelect(first.name);
      }
    }
  }, [filtered, props.selectedName, props.onSelect]);

  useInput((input, key) => {
    if (searching) {
      if (key.escape) {
        setSearching(false);
      }
      return;
    }
    if (input === "q") {
      props.onExit();
    } else if (input === "n") {
      props.onCreate();
    } else if (input === "e" && props.detail) {
      props.onEdit();
    } else if (input === "d" && props.detail) {
      props.onDelete();
    } else if (input === "r") {
      props.onRefresh();
    } else if (input === "/") {
      setSearching(true);
    } else if (key.downArrow || input === "j") {
      const next = filtered[Math.min(filtered.length - 1, selectedIndex + 1)];
      if (next) props.onSelect(next.name);
    } else if (key.upArrow || input === "k") {
      const next = filtered[Math.max(0, selectedIndex - 1)];
      if (next) props.onSelect(next.name);
    }
  });

  return (
    <Frame
      title="AVM UI"
      subtitle="Agent CRUD"
      status={props.error}
      actions={[
        ["/", "search"],
        ["n", "new"],
        ["e", "edit"],
        ["d", "delete"],
        ["r", "refresh"],
        ["q", "quit"]
      ]}
    >
      <Box flexDirection="column" flexGrow={1}>
        <Box paddingX={1} paddingY={1}>
          <Text color={searching ? theme.accent : theme.muted}>search: </Text>
          {searching ? <TextInput value={query} onChange={setQuery} /> : <Text>{query || "-"}</Text>}
        </Box>
        <Box flexGrow={1}>
          <Box width="38%" borderStyle="single" borderColor={theme.muted} flexDirection="column" paddingX={1}>
            <SectionTitle>Agents</SectionTitle>
            {props.loading ? <Muted>loading...</Muted> : null}
            {!props.loading && filtered.length === 0 ? <Muted>no agents</Muted> : null}
            {(() => {
              const window = windowSlice(filtered, Math.max(0, selectedIndex), AGENT_PAGE);
              return (
                <>
                  {window.before > 0 ? <Muted>↑ {window.before} more</Muted> : null}
                  {window.visible.map((agent) => {
                    const selected = agent.name === props.selectedName;
                    return (
                      <Box key={agent.name} flexDirection="column">
                        <Text color={selected ? theme.accent : undefined} bold={selected}>
                          {selected ? "> " : "  "}{agent.name}
                        </Text>
                        <Text color={theme.muted}>
                          {"  "}{truncate(agent.description, 42) || "-"}
                        </Text>
                        <Text color={theme.muted}>
                          {"  "}{agent.runtimes.length > 0 ? agent.runtimes.join(", ") : "no runtime"}
                        </Text>
                      </Box>
                    );
                  })}
                  {window.after > 0 ? <Muted>↓ {window.after} more</Muted> : null}
                  {filtered.length > 0
                    ? <Muted>{Math.max(1, selectedIndex + 1)}/{filtered.length}</Muted>
                    : null}
                </>
              );
            })()}
          </Box>
          <AgentDetailPanel detail={props.detail} capabilities={props.capabilities} />
        </Box>
      </Box>
    </Frame>
  );
}

function AgentDetailPanel(props: {detail?: AgentDetail; capabilities: CapabilityRecord[]}) {
  if (!props.detail) {
    return (
      <Box flexGrow={1} borderStyle="single" borderColor={theme.muted} paddingX={1}>
        <Muted>Select an Agent or press n to create one.</Muted>
      </Box>
    );
  }
  const agent = props.detail.agent;
  const byID = new Map(props.capabilities.map((capability) => [capability.id, capability]));
  return (
    <Box flexGrow={1} borderStyle="single" borderColor={theme.muted} flexDirection="column" paddingX={1}>
      <Box justifyContent="space-between">
        <SectionTitle>{agent.identity.name}</SectionTitle>
        <Text>{agent.runtimes.map((runtime) => runtime.default ? `${runtime.runtime}*` : runtime.runtime).join(", ") || "no runtime"}</Text>
      </Box>
      <Text>{agent.identity.description || "-"}</Text>
      <Text color={theme.muted}>role: {agent.identity.role || "-"}</Text>
      <Text color={theme.muted}>source: {props.detail.source_path || "-"}</Text>
      <Box marginTop={1} flexDirection="column">
        <SectionTitle>Instructions</SectionTitle>
        <Text>{truncate(agent.instructions.system, 160) || "-"}</Text>
      </Box>
      <Box marginTop={1}>
        <Box width="50%" flexDirection="column">
          <SectionTitle>Skills</SectionTitle>
          <CapabilityRefs refs={agent.skills} capabilities={byID} />
        </Box>
        <Box flexGrow={1} flexDirection="column">
          <SectionTitle>MCP</SectionTitle>
          <CapabilityRefs refs={agent.mcp} capabilities={byID} />
        </Box>
      </Box>
      <Box marginTop={1} flexDirection="column">
        <SectionTitle>Mapping</SectionTitle>
        {props.detail.mapping.length === 0 ? <Muted>no mapping summary</Muted> : props.detail.mapping.map((mapping) => (
          <Box key={mapping.runtime} flexDirection="column">
            <Text><Pill>{mapping.runtime}</Pill></Text>
            {mapping.fields.map((field) => (
              <Text key={`${mapping.runtime}-${field.field}`} color={theme.muted}>
                {field.field}: {field.status}{field.note ? ` - ${field.note}` : ""}
              </Text>
            ))}
            {mapping.warnings.map((warning) => (
              <ErrorText key={`${mapping.runtime}-${warning}`}>{warning}</ErrorText>
            ))}
          </Box>
        ))}
      </Box>
    </Box>
  );
}

function CapabilityRefs(props: {
  refs: CapabilityRef[];
  capabilities: Map<string, CapabilityRecord>;
}) {
  if (props.refs.length === 0) {
    return <Muted>none</Muted>;
  }
  return (
    <>
      {props.refs.map((ref) => {
        const record = props.capabilities.get(ref.id);
        return (
          <Box key={ref.id} flexDirection="column">
            <Text>
              {truncate(record?.name ?? ref.id, 36)}{" "}
              <Pill color={record?.source === "runtime" ? theme.warn : theme.ok}>
                {record?.source ?? ref.kind}
              </Pill>
            </Text>
            {record ? (
              <Text color={theme.muted}>
                {truncate(ref.id, 36)}{record.version ? ` v${record.version}` : ""}
              </Text>
            ) : null}
          </Box>
        );
      })}
    </>
  );
}
