package main

import "github.com/spf13/cobra"

func newUseCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "use <profile-or-env>",
		Short: "Activate an AVM agent profile or environment",
		Args:  cobra.ExactArgs(1),
		RunE:  notImplemented,
	}
}
