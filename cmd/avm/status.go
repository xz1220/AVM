package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/spf13/cobra"
	"github.com/xz1220/agent-vm/internal/config"
	"github.com/xz1220/agent-vm/internal/state"
)

func newStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show AVM activation and runtime status",
		Args:  cobra.NoArgs,
		RunE:  runStatus,
	}
}

func runStatus(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	cfg, err := config.ReadGlobalConfig()
	if err != nil {
		if os.IsNotExist(err) {
			printMissingConfigStatus(cmd.OutOrStdout())
			return nil
		}
		return err
	}

	syncState, err := readSyncState()
	if err != nil {
		if os.IsNotExist(err) {
			targets, warnings := statusTargetsFromActive(cfg.Active, cwd)
			warnings = append(warnings, "sync-state not found")
			printStatusWithoutSyncState(cmd.OutOrStdout(), cfg.Active, targets, warnings)
			return nil
		}
		targets, warnings := statusTargetsFromActive(cfg.Active, cwd)
		warnings = append(warnings, fmt.Sprintf("sync-state unreadable: %v", err))
		printStatusWithoutSyncState(cmd.OutOrStdout(), cfg.Active, targets, warnings)
		return nil
	}

	printStatusWithSyncState(cmd.OutOrStdout(), cfg.Active, syncState)
	return nil
}

func readSyncState() (*state.SyncState, error) {
	data, err := os.ReadFile(syncStatePath())
	if err != nil {
		return nil, err
	}
	var syncState state.SyncState
	if err := json.Unmarshal(data, &syncState); err != nil {
		return nil, err
	}
	return &syncState, nil
}

func printMissingConfigStatus(out io.Writer) {
	fmt.Fprintln(out, "active: none")
	fmt.Fprintln(out, "runtime status:")
	fmt.Fprintln(out, "  none")
	fmt.Fprintln(out, "managed paths:")
	fmt.Fprintln(out, "  none")
	fmt.Fprintln(out, "mapping status:")
	fmt.Fprintln(out, "  none")
	fmt.Fprintln(out, "warnings:")
	fmt.Fprintln(out, "  - config not found; run avm init")
}

func printStatusWithoutSyncState(out io.Writer, active config.ActiveRef, targets []string, warnings []string) {
	fmt.Fprintf(out, "active: %s\n", formatActiveRef(active))
	fmt.Fprintln(out, "runtime status:")
	printTargetLines(out, targets, "unknown")
	fmt.Fprintln(out, "managed paths:")
	printTargetLines(out, targets, "none")
	fmt.Fprintln(out, "mapping status:")
	printTargetLines(out, targets, "none")
	printWarnings(out, warnings)
}

func printStatusWithSyncState(out io.Writer, active config.ActiveRef, syncState *state.SyncState) {
	runtimes := syncStateRuntimeOrder(syncState)
	warnings := make([]string, 0)
	if formatActiveRef(syncState.LastActive) != "none" && syncState.LastActive != active {
		warnings = append(warnings, fmt.Sprintf("sync-state active %s differs from config active %s", formatActiveRef(syncState.LastActive), formatActiveRef(active)))
	}

	fmt.Fprintf(out, "active: %s\n", formatActiveRef(active))
	fmt.Fprintln(out, "runtime status:")
	if len(runtimes) == 0 {
		fmt.Fprintln(out, "  none")
	} else {
		for _, runtime := range runtimes {
			runtimeState := syncState.Runtimes[runtime]
			status := string(runtimeState.Status)
			if status == "" {
				status = "unknown"
			}
			if runtimeState.AgentName == "" {
				fmt.Fprintf(out, "  %s: %s\n", runtime, status)
			} else {
				fmt.Fprintf(out, "  %s: %s (agent %s)\n", runtime, status, runtimeState.AgentName)
			}
			if runtimeState.Error != "" {
				warnings = append(warnings, fmt.Sprintf("%s: %s", runtime, runtimeState.Error))
			}
			for _, warning := range runtimeState.Warnings {
				warnings = append(warnings, fmt.Sprintf("%s: %s", runtime, warning))
			}
		}
	}

	fmt.Fprintln(out, "managed paths:")
	if len(runtimes) == 0 {
		fmt.Fprintln(out, "  none")
	} else {
		for _, runtime := range runtimes {
			printManagedPaths(out, runtime, syncState.Runtimes[runtime].ManagedPaths)
		}
	}

	fmt.Fprintln(out, "mapping status:")
	if len(runtimes) == 0 {
		fmt.Fprintln(out, "  none")
	} else {
		for _, runtime := range runtimes {
			printMappings(out, runtime, syncState.Runtimes[runtime].Mappings)
		}
	}
	printWarnings(out, warnings)
}

func statusTargetsFromActive(active config.ActiveRef, cwd string) ([]string, []string) {
	resolved, err := config.ResolveActivation(active, cwd)
	if err != nil {
		return nil, []string{fmt.Sprintf("active could not be resolved: %v", err)}
	}
	targets := append([]string(nil), resolved.Targets...)
	if len(targets) == 0 {
		targets = runtimeAgentKeys(resolved.RuntimeAgents)
	}
	return targets, append([]string(nil), resolved.Warnings...)
}

func printTargetLines(out io.Writer, targets []string, value string) {
	if len(targets) == 0 {
		fmt.Fprintln(out, "  none")
		return
	}
	for _, target := range targets {
		fmt.Fprintf(out, "  %s: %s\n", target, value)
	}
}

func printWarnings(out io.Writer, warnings []string) {
	fmt.Fprintln(out, "warnings:")
	if len(warnings) == 0 {
		fmt.Fprintln(out, "  none")
		return
	}
	for _, warning := range warnings {
		fmt.Fprintf(out, "  - %s\n", warning)
	}
}

func printManagedPaths(out io.Writer, runtime string, paths []state.ManagedPathState) {
	if len(paths) == 0 {
		fmt.Fprintf(out, "  %s: none\n", runtime)
		return
	}
	sort.Slice(paths, func(i, j int) bool {
		if paths[i].Path != paths[j].Path {
			return paths[i].Path < paths[j].Path
		}
		if paths[i].Owner != paths[j].Owner {
			return paths[i].Owner < paths[j].Owner
		}
		return paths[i].MergeMode < paths[j].MergeMode
	})
	fmt.Fprintf(out, "  %s:\n", runtime)
	for _, path := range paths {
		fmt.Fprintf(out, "    - %s owner=%s merge=%s\n", path.Path, path.Owner, path.MergeMode)
	}
}

func printMappings(out io.Writer, runtime string, mappings []state.MappingState) {
	if len(mappings) == 0 {
		fmt.Fprintf(out, "  %s: none\n", runtime)
		return
	}
	sort.Slice(mappings, func(i, j int) bool {
		if mappings[i].SourcePath != mappings[j].SourcePath {
			return mappings[i].SourcePath < mappings[j].SourcePath
		}
		if mappings[i].TargetPath != mappings[j].TargetPath {
			return mappings[i].TargetPath < mappings[j].TargetPath
		}
		if mappings[i].Status != mappings[j].Status {
			return mappings[i].Status < mappings[j].Status
		}
		return mappings[i].Reason < mappings[j].Reason
	})
	fmt.Fprintf(out, "  %s:\n", runtime)
	for _, mapping := range mappings {
		if mapping.TargetPath == "" {
			fmt.Fprintf(out, "    - %s: %s", mapping.SourcePath, mapping.Status)
		} else {
			fmt.Fprintf(out, "    - %s -> %s: %s", mapping.SourcePath, mapping.TargetPath, mapping.Status)
		}
		if mapping.Reason != "" {
			fmt.Fprintf(out, " (%s)", mapping.Reason)
		}
		fmt.Fprintln(out)
	}
}

func syncStateRuntimeOrder(syncState *state.SyncState) []string {
	if syncState == nil || len(syncState.Runtimes) == 0 {
		return nil
	}
	runtimes := make([]string, 0, len(syncState.Runtimes))
	for runtime := range syncState.Runtimes {
		runtimes = append(runtimes, runtime)
	}
	sort.Strings(runtimes)
	return runtimes
}
