import React from "react";
import {Box, Text, useStdout} from "ink";
import {theme} from "./theme.js";

export function Frame(props: {
  title: string;
  subtitle?: string;
  status?: string;
  children: React.ReactNode;
  actions: Array<[string, string]>;
}) {
  const {stdout} = useStdout();
  const height = Math.max(stdout.rows ?? 28, 24);

  return (
    <Box flexDirection="column" minHeight={height - 1}>
      <Box justifyContent="space-between" borderStyle="single" borderColor={theme.accent} paddingX={1}>
        <Text bold color={theme.accent}>{props.title}</Text>
        <Text color={theme.muted}>{props.subtitle ?? ""}</Text>
      </Box>
      <Box flexGrow={1}>{props.children}</Box>
      {props.status ? (
        <Box paddingX={1}>
          <Text color={theme.warn}>{props.status}</Text>
        </Box>
      ) : null}
      <ActionBar actions={props.actions} />
    </Box>
  );
}

export function ActionBar(props: {actions: Array<[string, string]>}) {
  return (
    <Box borderStyle="single" borderColor={theme.muted} paddingX={1}>
      {props.actions.map(([key, label], index) => (
        <Box key={`${key}-${label}`} marginRight={index === props.actions.length - 1 ? 0 : 2}>
          <Text color={theme.accent}>{key}</Text>
          <Text color={theme.muted}> {label}</Text>
        </Box>
      ))}
    </Box>
  );
}

export function SectionTitle(props: {children: React.ReactNode}) {
  return <Text bold color={theme.accent}>{props.children}</Text>;
}

export function Muted(props: {children: React.ReactNode}) {
  return <Text color={theme.muted}>{props.children}</Text>;
}

export function ErrorText(props: {children: React.ReactNode}) {
  return <Text color={theme.danger}>{props.children}</Text>;
}

export function Pill(props: {children: React.ReactNode; color?: string}) {
  return (
    <Text color={props.color ?? theme.accent}>
      [{props.children}]
    </Text>
  );
}

export function truncate(value: string | undefined, max: number): string {
  const text = value ?? "";
  if (text.length <= max) {
    return text;
  }
  return `${text.slice(0, Math.max(0, max - 3))}...`;
}

export function windowSlice<T>(items: T[], cursor: number, page: number): {
  visible: T[];
  start: number;
  before: number;
  after: number;
} {
  if (items.length <= page) {
    return {visible: items, start: 0, before: 0, after: 0};
  }
  const half = Math.floor(page / 2);
  const start = Math.max(0, Math.min(items.length - page, cursor - half));
  const visible = items.slice(start, start + page);
  return {
    visible,
    start,
    before: start,
    after: items.length - (start + visible.length)
  };
}
