package cli

import (
	"github.com/spf13/cobra"
)

// newRuntimeCmd is the parent for runtime-introspection commands. The
// only purpose of this surface today is to give UIs a non-diagnostic
// runtime picker payload — Doctor remains the right command for human
// machine-health checks.
func newRuntimeCmd(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "runtime",
		Short: "Introspect runtimes registered with AVM",
	}
	cmd.AddCommand(newRuntimeListCmd(deps))
	return cmd
}

func newRuntimeListCmd(deps Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List runtimes with availability and version",
		Long: `List every runtime registered with AVM, with the same probe payload
'avm doctor' uses (RuntimeCheck): name, availability, binary path,
version, and any issues. UIs should consume this rather than parsing
the broader DoctorReport when they only need a runtime picker.`,
		RunE: func(c *cobra.Command, args []string) error {
			items, err := deps.Services.Diagnostics.Runtimes(c.Context())
			if err != nil {
				return err
			}
			if globalFlags(c).JSON {
				return jsonWrite(c.OutOrStdout(), items)
			}
			return RenderRuntimeList(c.OutOrStdout(), items)
		},
	}
}
