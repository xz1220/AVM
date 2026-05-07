// Package cli is the AVM presentation layer. It owns command and flag
// parsing, interactive UX, and rendering of structured results from
// the application services. It does not own product rules.
package cli

import (
	"github.com/spf13/cobra"

	"github.com/xz1220/agent-vm/internal/app/service"
)

// Deps is the wiring presentation needs from the composition root.
type Deps struct {
	Services service.Container
}

// global persistent flag names.
const (
	flagJSON           = "json"
	flagNonInteractive = "non-interactive"
)

// NewRoot builds the cobra tree for `avm`.
func NewRoot(deps Deps) *cobra.Command {
	root := &cobra.Command{
		Use:           "avm",
		Short:         "Agent VM - local config manager for AI coding agents",
		SilenceUsage:  true,
		SilenceErrors: false,
	}

	root.PersistentFlags().Bool(flagJSON, false, "render results as JSON for programmatic consumers")
	root.PersistentFlags().Bool(flagNonInteractive, false, "fail instead of prompting; required for scripted use")

	root.AddCommand(newAgentCmd(deps))
	root.AddCommand(newRunCmd(deps))
	root.AddCommand(newPackageCmd(deps))
	root.AddCommand(newInitCmd(deps))
	root.AddCommand(newDoctorCmd(deps))
	root.AddCommand(newStatusCmd(deps))
	root.AddCommand(newUninstallCmd(deps))
	root.AddCommand(newShellCmd(deps))

	return root
}

// globalFlags collects the values of root persistent flags.
type globals struct {
	JSON           bool
	NonInteractive bool
}

func globalFlags(cmd *cobra.Command) globals {
	root := cmd
	for root.HasParent() {
		root = root.Parent()
	}
	g := globals{}
	if f := root.PersistentFlags().Lookup(flagJSON); f != nil {
		g.JSON = f.Value.String() == "true"
	}
	if f := root.PersistentFlags().Lookup(flagNonInteractive); f != nil {
		g.NonInteractive = f.Value.String() == "true"
	}
	return g
}
