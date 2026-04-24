package main

import "github.com/spf13/cobra"

func newDeactivateCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "deactivate",
		Short: "Deactivate the current AVM profile or environment",
		Args:  cobra.NoArgs,
		RunE:  notImplemented,
	}
}
