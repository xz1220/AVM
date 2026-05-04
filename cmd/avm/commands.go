package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func addCommands(root *cobra.Command) {
	root.AddCommand(
		newInitCommand(),
		newCreateCommand(),
		newPackageCommand(),
		newSkillCommand(),
		newAgentCommand(),
		newEnvCommand(),
		newActivateCommand(),
		newUseCommand(),
		newStatusCommand(),
		newShellCommand(),
		newDeactivateCommand(),
		newSyncCommand(),
		newRunCommand(),
	)
}

func notImplemented(cmd *cobra.Command, args []string) error {
	return fmt.Errorf("%s: not implemented", cmd.CommandPath())
}
