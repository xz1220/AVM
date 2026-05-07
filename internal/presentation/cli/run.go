package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/xz1220/agent-vm/internal/app/model"
)

func newRunCmd(deps Deps) *cobra.Command {
	var (
		runtime string
		preview bool
		drift   string
	)
	cmd := &cobra.Command{
		Use:   "run <agent>",
		Short: "Run an Agent through a runtime (PRD §4.4)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			g := globalFlags(c)
			req := model.RunRequest{
				Agent:          args[0],
				Runtime:        runtime,
				NonInteractive: !isInteractive(g),
				DriftPolicy:    model.DriftPolicy(drift),
			}
			return runAgent(c.Context(), c.OutOrStdout(), c.ErrOrStderr(), deps, req, preview, g)
		},
	}
	cmd.Flags().StringVar(&runtime, "runtime", "", "target runtime")
	cmd.Flags().BoolVar(&preview, "preview", false, "show plan, do not launch")
	cmd.Flags().StringVar(&drift, "drift", "", "ask|merge|discard|keep (default keep in non-interactive)")
	return cmd
}

func runAgent(ctx context.Context, out, errw io.Writer, deps Deps, req model.RunRequest, previewOnly bool, g globals) error {
	pv, err := deps.Services.Run.Preview(ctx, req)
	if err != nil {
		// Multi-runtime ambiguity → prompt in interactive mode.
		if isInteractive(g) && req.Runtime == "" && strings.Contains(err.Error(), "multiple runtimes; pass --runtime") {
			rt, perr := chooseRuntimeForAgent(ctx, deps, req.Agent)
			if perr != nil {
				return perr
			}
			req.Runtime = rt
			pv, err = deps.Services.Run.Preview(ctx, req)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}

	if previewOnly {
		if g.JSON {
			return jsonWrite(out, pv)
		}
		return RenderRunPreview(out, pv)
	}

	// Drift gate.
	if len(pv.Drift) > 0 && req.DriftPolicy == model.DriftAsk && isInteractive(g) {
		fmt.Fprintln(out, "Drift detected:")
		for _, d := range pv.Drift {
			fmt.Fprintf(out, "  - %s [%s] %s\n", d.Path, d.Field, d.Reason)
		}
		choice, perr := promptSelect("How should AVM handle the drift?",
			[]string{"keep", "merge", "discard", "cancel"})
		if perr != nil {
			return perr
		}
		if choice == "cancel" {
			return errors.New("run: cancelled at drift prompt")
		}
		req.DriftPolicy = model.DriftPolicy(choice)
	}

	res, err := deps.Services.Run.Run(ctx, req)
	if err != nil {
		return err
	}
	if g.JSON {
		if err := jsonWrite(out, res); err != nil {
			return err
		}
	} else if err := RenderRunResult(out, res); err != nil {
		return err
	}
	if res != nil && res.ExitCode != 0 {
		// Propagate the runtime exit code so scripts can branch on it.
		// cobra picks up an error to set non-zero process exit; signal
		// via a typed exit-code error.
		return &exitCodeError{code: res.ExitCode}
	}
	return nil
}

// chooseRuntimeForAgent uses Show to fetch the configured runtimes for
// an agent and prompts the user to pick one. We deliberately don't
// fall back to the registered set here: PRD §4.4 says the prompt is for
// the Agent's preferences, not arbitrary runtimes.
func chooseRuntimeForAgent(ctx context.Context, deps Deps, agent string) (string, error) {
	detail, err := deps.Services.Agents.Show(ctx, agent)
	if err != nil {
		return "", err
	}
	options := make([]string, 0, len(detail.Agent.Runtimes))
	for _, r := range detail.Agent.Runtimes {
		options = append(options, r.Runtime)
	}
	if len(options) == 0 {
		return "", fmt.Errorf("agent %q has no configured runtimes", agent)
	}
	return promptSelect("Pick a runtime for "+agent, options)
}

// exitCodeError carries a non-zero exit code through cobra so main.go
// can map it to os.Exit. We also implement os-level signaling here for
// callers that bypass main's errno mapping.
type exitCodeError struct{ code int }

func (e *exitCodeError) Error() string {
	return fmt.Sprintf("run: exit code %d", e.code)
}

// ExitCode lets main.go translate to os.Exit if it chooses.
func (e *exitCodeError) ExitCode() int { return e.code }
