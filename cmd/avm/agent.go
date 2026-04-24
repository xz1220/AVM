package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func addCommands(root *cobra.Command) {
	root.AddCommand(
		newInitCommand(),
		newAgentCommand(),
		newEnvCommand(),
		newUseCommand(),
		newStatusCommand(),
		newShellCommand(),
		newDeactivateCommand(),
	)
}

func notImplemented(cmd *cobra.Command, args []string) error {
	return fmt.Errorf("%s: not implemented", cmd.CommandPath())
}

func newAgentCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Manage AVM agent profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		newAgentCreateCommand(),
		newAgentListCommand(),
		newAgentShowCommand(),
	)
	return cmd
}

func newAgentCreateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create an AVM agent profile",
		Args:  cobra.ExactArgs(1),
		RunE:  notImplemented,
	}
	cmd.Flags().String("runtime", "", "preferred runtime for this agent profile")
	cmd.Flags().String("scope", "", "profile scope")
	cmd.Flags().String("model", "", "model override")
	cmd.Flags().String("reasoning", "", "reasoning effort override")
	cmd.Flags().StringSlice("skills", nil, "skills to attach")
	cmd.Flags().StringSlice("mcps", nil, "MCP servers to attach")
	cmd.Flags().StringSlice("memory", nil, "portable memory refs to attach")
	return cmd
}

func newAgentListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List AVM agent profiles",
		Args:  cobra.NoArgs,
		RunE:  notImplemented,
	}
}

func newAgentShowCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <name>",
		Short: "Show an AVM agent profile",
		Args:  cobra.ExactArgs(1),
		RunE:  notImplemented,
	}
	cmd.Flags().String("runtime", "", "runtime mapping to inspect")
	return cmd
}
