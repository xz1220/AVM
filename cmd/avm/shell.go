package main

import "github.com/spf13/cobra"

func newShellCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "shell",
		Short: "Manage AVM shell integration",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newShellInitCommand())
	return cmd
}

func newShellInitCommand() *cobra.Command {
	return &cobra.Command{
		Use:       "init <shell>",
		Short:     "Print AVM shell integration",
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{"bash", "zsh", "fish"},
		RunE:      notImplemented,
	}
}
