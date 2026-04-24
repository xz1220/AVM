package main

import (
	"os"

	"github.com/spf13/cobra"
)

func newUseCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "use <profile-or-env>",
		Short: "Activate an AVM agent profile or environment",
		Args:  cobra.ExactArgs(1),
		RunE:  runUse,
	}
	cmd.Flags().String("kind", "", "activation kind (profile or env)")
	return cmd
}

func runUse(cmd *cobra.Command, args []string) error {
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
	printActivationResult(cmd.OutOrStdout(), result)
	return nil
}
