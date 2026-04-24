package main

import "github.com/spf13/cobra"

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
		RunE:  notImplemented,
	}
	cmd.Flags().Bool("local", false, "create the environment for the current workspace")
	cmd.Flags().String("codex", "", "agent profile for Codex")
	cmd.Flags().String("claude-code", "", "agent profile for Claude Code")
	cmd.Flags().String("cline", "", "agent profile for Cline")
	cmd.Flags().String("cursor", "", "agent profile for Cursor")
	return cmd
}
