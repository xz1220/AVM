package main

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/xz1220/agent-vm/internal/config"
)

func newActivateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "activate <profile-or-env>",
		Short: "Print shell exports for an AVM agent profile or environment",
		Args:  cobra.ExactArgs(1),
		RunE:  runActivate,
	}
	cmd.Flags().String("kind", "", "activation kind (profile or env)")
	return cmd
}

func runActivate(cmd *cobra.Command, args []string) error {
	if err := ensureInitialized(); err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	resolved, err := resolveActivationFromCommand(cmd, args[0], cwd)
	if err != nil {
		return err
	}
	result, err := applyActivation(resolved, cwd)
	if err != nil {
		return err
	}
	printShellActivation(cmd.OutOrStdout(), result)
	return nil
}

func printShellActivation(out io.Writer, result *activationResult) {
	if result == nil {
		return
	}
	writeShellExport(out, "AVM_HOME", config.AvmDir())
	writeShellExport(out, "AVM_ACTIVE", formatActiveRef(result.Active))
	writeShellExport(out, "AVM_ACTIVE_DIR", config.ActiveDir())
	writeShellExport(out, "AVM_STATE_DIR", config.StateDir())
	for _, target := range result.Targets {
		if target.Status != "synced" {
			continue
		}
		writeShellEnv(out, target.Boundary.Env)
	}
}

func writeShellEnv(out io.Writer, env map[string]string) {
	if len(env) == 0 {
		return
	}
	names := make([]string, 0, len(env))
	for name := range env {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		writeShellExport(out, name, env[name])
	}
}

func writeShellExport(out io.Writer, name, value string) {
	if name == "" {
		return
	}
	fmt.Fprintf(out, "export %s=%s\n", name, shellQuote(value))
}

func shellQuote(value string) string {
	if value == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
