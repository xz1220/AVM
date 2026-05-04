package main

import (
	"fmt"
	"os"
	"os/exec"
	"sort"

	"github.com/spf13/cobra"
	"github.com/xz1220/agent-vm/internal/boundary"
	"github.com/xz1220/agent-vm/internal/config"
)

func newRunCommand() *cobra.Command {
	var printEnv bool
	cmd := &cobra.Command{
		Use:   "run <runtime> [args...]",
		Short: "Run a runtime with the active AVM agent boundary",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRuntime(cmd, args, printEnv)
		},
	}
	cmd.Flags().BoolVar(&printEnv, "print-env", false, "print the runtime process environment instead of executing the runtime")
	return cmd
}

func runRuntime(cmd *cobra.Command, args []string, printEnv bool) error {
	if err := ensureInitialized(); err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	cfg, err := config.ReadGlobalConfig()
	if err != nil {
		return err
	}
	resolved, err := resolveActivationRef(cfg.Active, cwd)
	if err != nil {
		return err
	}
	result, err := applyActivation(resolved, cwd)
	if result == nil {
		return err
	}
	runtime := args[0]
	target, ok := activationTargetForRuntime(result, runtime)
	if !ok {
		return fmt.Errorf("runtime %q is not active for %s", runtime, formatActiveRef(result.Active))
	}
	if target.Status != "synced" {
		if err != nil {
			return err
		}
		return fmt.Errorf("runtime %q is not synced: %s", runtime, target.Status)
	}
	runEnv := processEnvForBoundary(target.Boundary)
	if err := ensureBoundaryRunDirs(target.Boundary); err != nil {
		return err
	}
	if printEnv {
		printProcessEnv(cmd.OutOrStdout(), runEnv)
		return nil
	}
	return execRuntime(runtime, args[1:], runEnv)
}

func activationTargetForRuntime(result *activationResult, runtime string) (activationTargetResult, bool) {
	if result == nil {
		return activationTargetResult{}, false
	}
	for _, target := range result.Targets {
		if target.Runtime == runtime {
			return target, true
		}
	}
	return activationTargetResult{}, false
}

func processEnvForBoundary(runtimeBoundary boundary.RuntimeBoundary) map[string]string {
	if len(runtimeBoundary.RunEnv) > 0 {
		return cloneEnvMap(runtimeBoundary.RunEnv)
	}
	return cloneEnvMap(runtimeBoundary.Env)
}

func ensureBoundaryRunDirs(runtimeBoundary boundary.RuntimeBoundary) error {
	if runtimeBoundary.Root != "" {
		if err := os.MkdirAll(runtimeBoundary.Root, 0o700); err != nil {
			return err
		}
	}
	for _, key := range []string{"config_dir", "data_dir", "xdg_data", "xdg_state", "xdg_cache"} {
		path := runtimeBoundary.Paths[key]
		if path == "" {
			continue
		}
		if err := os.MkdirAll(path, 0o700); err != nil {
			return err
		}
	}
	return nil
}

func execRuntime(runtime string, args []string, env map[string]string) error {
	binary := runtimeBinary(runtime)
	command := exec.Command(binary, args...)
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	command.Env = mergedProcessEnv(env)
	return command.Run()
}

func runtimeBinary(runtime string) string {
	switch runtime {
	case "claude-code":
		return "claude"
	default:
		return runtime
	}
}

func mergedProcessEnv(env map[string]string) []string {
	merged := os.Environ()
	for _, name := range sortedEnvNames(env) {
		merged = append(merged, name+"="+env[name])
	}
	return merged
}

func printProcessEnv(out interface{ Write([]byte) (int, error) }, env map[string]string) {
	for _, name := range sortedEnvNames(env) {
		fmt.Fprintf(out, "%s=%s\n", name, env[name])
	}
}

func sortedEnvNames(env map[string]string) []string {
	names := make([]string, 0, len(env))
	for name := range env {
		if name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func cloneEnvMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
