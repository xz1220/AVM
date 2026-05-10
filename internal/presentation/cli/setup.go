package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/xz1220/agent-vm/internal/app/model"
	"github.com/xz1220/agent-vm/internal/app/service"
)

func newSetupCmd(deps Deps) *cobra.Command {
	var runtimes []string
	var noCapabilities bool
	var onConflict string

	cmd := &cobra.Command{
		Use:   "setup",
		Short: "Initialize AVM and import existing runtime capabilities",
		Long: `Initialize AVM and import existing runtime capabilities.

Setup is the first-run onboarding path. It lays out AVM home, detects
installed runtimes, imports runtime-global skills/MCP definitions into
the AVM capability store, and prints the next command to run.`,
		RunE: func(c *cobra.Command, args []string) error {
			resolution, err := parseSetupConflict(onConflict)
			if err != nil {
				return err
			}
			res, err := runSetup(c, deps, setupOptions{
				Runtimes:       runtimes,
				NoCapabilities: noCapabilities,
				OnConflict:     resolution,
			})
			if err != nil {
				return err
			}
			if globalFlags(c).JSON {
				return jsonWrite(c.OutOrStdout(), res)
			}
			return renderSetup(c.OutOrStdout(), res)
		},
	}
	cmd.Flags().StringSliceVar(&runtimes, "runtime", nil, "runtime to bootstrap (repeatable; default all detected runtimes)")
	cmd.Flags().BoolVar(&noCapabilities, "no-capabilities", false, "skip runtime-global capability import")
	cmd.Flags().StringVar(&onConflict, "on-conflict", "skip", "skip|overwrite|cancel when an imported capability name conflicts")
	return cmd
}

type setupOptions struct {
	Runtimes       []string
	NoCapabilities bool
	OnConflict     model.ConflictResolution
}

func parseSetupConflict(raw string) (model.ConflictResolution, error) {
	switch model.ConflictResolution(raw) {
	case "", model.ResolveSkip:
		return model.ResolveSkip, nil
	case model.ResolveOverwrite:
		return model.ResolveOverwrite, nil
	case model.ResolveCancel:
		return model.ResolveCancel, nil
	default:
		return "", service.NewError(service.CodeValidation,
			fmt.Sprintf("unsupported --on-conflict %q (want skip|overwrite|cancel)", raw),
			map[string]any{"on_conflict": raw})
	}
}

func runSetup(c *cobra.Command, deps Deps, opts setupOptions) (*model.SetupResult, error) {
	initRes, err := deps.Services.System.Init(c.Context())
	if err != nil {
		return nil, err
	}
	out := &model.SetupResult{Init: *initRes}

	runtimeChecks, err := deps.Services.Diagnostics.Runtimes(c.Context())
	if err != nil {
		return nil, err
	}
	allowRuntime := makeRuntimeFilter(opts.Runtimes)
	firstAvailable := ""
	for _, rt := range runtimeChecks {
		if !allowRuntime(rt.Runtime) {
			continue
		}
		item := model.SetupRuntimeResult{
			Runtime:   rt.Runtime,
			Available: rt.Available,
			Binary:    rt.Binary,
			Version:   rt.Version,
			Issues:    append([]string(nil), rt.Issues...),
		}
		if rt.Available {
			if firstAvailable == "" {
				firstAvailable = rt.Runtime
			}
			if !opts.NoCapabilities {
				boot, bootErr := deps.Services.Capabilities.Bootstrap(c.Context(), model.BootstrapCapabilitiesRequest{
					Runtime:    rt.Runtime,
					OnConflict: opts.OnConflict,
				})
				if bootErr != nil {
					item.Issues = append(item.Issues, bootErr.Error())
				} else if boot != nil {
					item.Imported = boot.Imported
					item.Skipped = boot.Skipped
				}
			}
		}
		out.Runtimes = append(out.Runtimes, item)
	}
	out.NextSteps = setupNextSteps(firstAvailable)
	return out, nil
}

func makeRuntimeFilter(names []string) func(string) bool {
	if len(names) == 0 {
		return func(string) bool { return true }
	}
	allowed := make(map[string]struct{}, len(names))
	for _, n := range names {
		allowed[n] = struct{}{}
	}
	return func(name string) bool {
		_, ok := allowed[name]
		return ok
	}
}

func setupNextSteps(runtime string) []string {
	if runtime == "" {
		return []string{
			"avm runtime list",
			"Install Codex, Claude Code, or OpenCode, then rerun avm setup",
		}
	}
	return []string{
		"avm-ui",
		fmt.Sprintf("avm agent create --name backend-coder --runtime %s", runtime),
		"avm run backend-coder",
	}
}

func renderSetup(w io.Writer, res *model.SetupResult) error {
	if res == nil {
		return nil
	}
	if res.Init.AlreadyExists {
		fmt.Fprintf(w, "AVM home already initialized at %s\n", res.Init.Root)
	} else {
		fmt.Fprintf(w, "Initialized AVM home at %s\n", res.Init.Root)
	}
	if len(res.Runtimes) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Runtimes:")
		for _, rt := range res.Runtimes {
			status := "not found"
			if rt.Available {
				status = "available"
			}
			parts := []string{status}
			if rt.Version != "" {
				parts = append(parts, rt.Version)
			}
			if len(rt.Imported) > 0 || len(rt.Skipped) > 0 {
				parts = append(parts, fmt.Sprintf("imported %d, skipped %d", len(rt.Imported), len(rt.Skipped)))
			}
			if len(rt.Issues) > 0 {
				parts = append(parts, fmt.Sprintf("%d issue(s)", len(rt.Issues)))
			}
			fmt.Fprintf(w, "  %s: %s\n", rt.Runtime, strings.Join(parts, "; "))
			for _, issue := range rt.Issues {
				fmt.Fprintf(w, "    - %s\n", issue)
			}
		}
	}
	if len(res.NextSteps) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Next:")
		for _, step := range res.NextSteps {
			fmt.Fprintf(w, "  %s\n", step)
		}
	}
	return nil
}
