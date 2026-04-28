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
		newMemoryCommand(),
		newExportCommand(),
		newImportCommand(),
		newActivateCommand(),
		newUseCommand(),
		newStatusCommand(),
		newShellCommand(),
		newDeactivateCommand(),
		newSyncCommand(),
	)
}

func notImplemented(cmd *cobra.Command, args []string) error {
	return fmt.Errorf("%s: not implemented", cmd.CommandPath())
}
