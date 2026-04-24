package main

import "github.com/spf13/cobra"

func newStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show AVM activation and runtime status",
		Args:  cobra.NoArgs,
		RunE:  notImplemented,
	}
}
