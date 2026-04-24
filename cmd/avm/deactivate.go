package main

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/xz1220/agent-vm/internal/config"
)

func newDeactivateCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "deactivate",
		Short: "Deactivate the current AVM profile or environment",
		Args:  cobra.NoArgs,
		RunE:  runDeactivate,
	}
}

func runDeactivate(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	resolved, err := resolveActivationRef(config.ActiveRef{
		Kind: config.ActiveKindProfile,
		Name: "default",
	}, cwd)
	if err != nil {
		return err
	}
	result, err := applyActivation(resolved, cwd)
	if err != nil {
		return err
	}
	printActivationResult(cmd.OutOrStdout(), result)
	return nil
}
