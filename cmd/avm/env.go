package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/xz1220/agent-vm/internal/config"
)

func newEnvCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "env",
		Short: "Manage AVM runtime environments",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newEnvCreateCommand())
	return cmd
}

func newEnvCreateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create an AVM runtime environment",
		Args:  cobra.ExactArgs(1),
		RunE:  runEnvCreate,
	}
	cmd.Flags().Bool("local", false, "create the environment for the current workspace")
	cmd.Flags().String("codex", "", "agent profile for Codex")
	cmd.Flags().String("claude-code", "", "agent profile for Claude Code")
	cmd.Flags().String("cline", "", "agent profile for Cline")
	cmd.Flags().String("cursor", "", "agent profile for Cursor")
	return cmd
}

func runEnvCreate(cmd *cobra.Command, args []string) error {
	local, err := cmd.Flags().GetBool("local")
	if err != nil {
		return err
	}
	if local {
		return fmt.Errorf("avm env create --local is not supported yet")
	}

	runtimeAgents := make(map[string]config.RuntimeAgent)
	targets := make([]string, 0, len(config.KnownTargets))
	for _, runtime := range []string{"codex", "claude-code", "cline", "cursor"} {
		profile, err := cmd.Flags().GetString(runtime)
		if err != nil {
			return err
		}
		if profile == "" {
			continue
		}
		runtimeAgents[runtime] = config.RuntimeAgent{Primary: profile}
		targets = append(targets, runtime)
	}
	if len(runtimeAgents) == 0 {
		runtimeAgents["codex"] = config.RuntimeAgent{Primary: "default"}
		targets = append(targets, "codex")
	}

	env := &config.Environment{
		Name:          args[0],
		RuntimeAgents: runtimeAgents,
		Targets:       targets,
	}
	if err := config.WriteEnvironment(env); err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "created env %s\n", env.Name)
	return nil
}
