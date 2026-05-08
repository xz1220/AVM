import React, {useCallback, useEffect, useMemo, useState} from "react";
import {useApp} from "ink";
import type {AvmClient} from "./avm-client.js";
import type {AgentDetail, AgentSummary, CapabilityRecord, RuntimeCheck} from "./protocol.js";
import {AgentBrowser} from "./browse.js";
import {AgentEditor, DeleteConfirm} from "./editor.js";

type Mode =
  | {name: "browse"}
  | {name: "create"}
  | {name: "edit"; detail: AgentDetail}
  | {name: "delete"; detail: AgentDetail};

export function App(props: {client: AvmClient}) {
  const {exit} = useApp();
  const [mode, setMode] = useState<Mode>({name: "browse"});
  const [agents, setAgents] = useState<AgentSummary[]>([]);
  const [selectedName, setSelectedName] = useState<string | undefined>();
  const [detail, setDetail] = useState<AgentDetail | undefined>();
  const [runtimes, setRuntimes] = useState<RuntimeCheck[]>([]);
  const [capabilities, setCapabilities] = useState<CapabilityRecord[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | undefined>();

  const selectedSummary = useMemo(() => {
    return agents.find((agent) => agent.name === selectedName);
  }, [agents, selectedName]);

  const refresh = useCallback(async (preferredName?: string) => {
    setLoading(true);
    setError(undefined);
    try {
      const [nextAgents, nextRuntimes, nextCapabilities] = await Promise.all([
        props.client.listAgents(),
        props.client.listRuntimes(),
        props.client.listCapabilities()
      ]);
      setAgents(nextAgents);
      const nextName = preferredName ?? selectedName ?? nextAgents[0]?.name;
      setSelectedName(nextName);
      setRuntimes(nextRuntimes);
      setCapabilities(nextCapabilities);
    } catch (nextError: unknown) {
      setError(nextError instanceof Error ? nextError.message : String(nextError));
    } finally {
      setLoading(false);
    }
  }, [props.client, selectedName]);

  useEffect(() => {
    void refresh();
  }, []);

  useEffect(() => {
    if (!selectedName) {
      setDetail(undefined);
      return;
    }
    let cancelled = false;
    props.client.showAgent(selectedName)
      .then((nextDetail) => {
        if (!cancelled) setDetail(nextDetail);
      })
      .catch((nextError: unknown) => {
        if (!cancelled) setError(nextError instanceof Error ? nextError.message : String(nextError));
      });
    return () => {
      cancelled = true;
    };
  }, [props.client, selectedName]);

  if (mode.name === "create") {
    return (
      <AgentEditor
        mode="create"
        client={props.client}
        runtimes={runtimes}
        onCancel={() => setMode({name: "browse"})}
        onSaved={(name) => {
          setMode({name: "browse"});
          void refresh(name);
        }}
      />
    );
  }

  if (mode.name === "edit") {
    return (
      <AgentEditor
        mode="edit"
        client={props.client}
        initial={mode.detail}
        runtimes={runtimes}
        onCancel={() => setMode({name: "browse"})}
        onSaved={(name) => {
          setMode({name: "browse"});
          void refresh(name);
        }}
      />
    );
  }

  if (mode.name === "delete") {
    return (
      <DeleteConfirm
        agent={mode.detail}
        client={props.client}
        onCancel={() => setMode({name: "browse"})}
        onDeleted={() => {
          setMode({name: "browse"});
          void refresh();
        }}
      />
    );
  }

  return (
    <AgentBrowser
      agents={agents}
      detail={detail}
      capabilities={capabilities}
      selectedName={selectedSummary?.name}
      loading={loading}
      error={error}
      onSelect={setSelectedName}
      onCreate={() => setMode({name: "create"})}
      onEdit={() => detail && setMode({name: "edit", detail})}
      onDelete={() => detail && setMode({name: "delete", detail})}
      onRefresh={() => void refresh()}
      onExit={exit}
    />
  );
}
