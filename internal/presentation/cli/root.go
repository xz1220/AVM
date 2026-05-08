// Package cli is the AVM presentation layer. It owns command and flag
// parsing, structured output rendering, and translation of typed
// service errors into human or JSON form.
//
// This package does NOT prompt the user. AVM Go CLI is plumbing only:
// every command takes its inputs via flags or stdin and emits results
// to stdout. Interactive UX (wizards, prompts) lives in a separate
// TS/JS frontend (see ../../ui/) that shells out to this binary.
package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/xz1220/agent-vm/internal/app/service"
)

// Deps is the wiring presentation needs from the composition root.
type Deps struct {
	Services service.Container
}

// Root persistent-flag names. Constants live here so subcommands can
// look them up without typo risk.
const (
	flagJSON = "json"
)

// NewRoot builds the cobra tree for `avm`.
func NewRoot(deps Deps) *cobra.Command {
	root := &cobra.Command{
		Use:   "avm",
		Short: "Agent VM - local config manager for AI coding agents",
		// We render errors ourselves (human or JSON) via the runE
		// wrapper below, so silence cobra's defaults to avoid duplicate
		// output and unhelpful usage dumps on application errors.
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.PersistentFlags().Bool(flagJSON, false, "render results and errors as JSON for programmatic consumers")

	root.AddCommand(newAgentCmd(deps))
	root.AddCommand(newRunCmd(deps))
	root.AddCommand(newPackageCmd(deps))
	root.AddCommand(newCapabilityCmd(deps))
	root.AddCommand(newRuntimeCmd(deps))
	root.AddCommand(newInitCmd(deps))
	root.AddCommand(newDoctorCmd(deps))
	root.AddCommand(newStatusCmd(deps))
	root.AddCommand(newUninstallCmd(deps))
	root.AddCommand(newShellCmd(deps))

	wrapAllRunE(root)
	return root
}

// globals collects values of root persistent flags. Only --json today;
// kept as a struct so adding more globals later doesn't require
// touching every call site.
type globals struct {
	JSON bool
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
	return g
}

// ----------------------------------------------------------------------------
// Error rendering
//
// Every command's RunE return value is funnelled through renderError so
// human and JSON modes share one rendering policy:
//
//   --json off (default): "avm: <message>\n" to stderr; cobra exit code 1.
//   --json on:            {"error": <service.Error>} to stdout; exit 1.
// ----------------------------------------------------------------------------

// wrapAllRunE walks the command tree and replaces each RunE with a
// version that pipes through renderError. The original behavior is
// preserved on success; errors get unified rendering.
func wrapAllRunE(cmd *cobra.Command) {
	for _, sub := range cmd.Commands() {
		if sub.RunE != nil {
			orig := sub.RunE
			sub.RunE = func(c *cobra.Command, args []string) error {
				return renderError(c, orig(c, args))
			}
		}
		wrapAllRunE(sub)
	}
}

// errorEnvelope is the shape we serialise on --json. Keeping the
// "error" key constant gives TS/UI a stable discriminator: a successful
// command outputs the model, a failed one outputs {"error": ...}.
type errorEnvelope struct {
	Error *service.Error `json:"error"`
}

func renderError(c *cobra.Command, err error) error {
	if err == nil {
		return nil
	}
	g := globalFlags(c)
	se := service.AsError(err)
	if se == nil {
		// Wrap unknown errors so JSON mode still produces a valid
		// envelope. Code goes to INTERNAL_ERROR by default.
		se = service.NewError(service.CodeInternal, err.Error(), nil)
	}
	if g.JSON {
		enc := json.NewEncoder(c.OutOrStdout())
		enc.SetIndent("", "  ")
		_ = enc.Encode(errorEnvelope{Error: se})
	} else {
		fmt.Fprintf(c.ErrOrStderr(), "avm: %s\n", se.Message)
	}
	return err
}
