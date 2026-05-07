package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/xz1220/agent-vm/internal/app/model"
	"github.com/xz1220/agent-vm/internal/app/service"
	"github.com/xz1220/agent-vm/internal/infra/agentstore"
)

func newPackageCmd(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "package",
		Short: "Manage AVM packages (PRD §4.5)",
	}
	cmd.AddCommand(newPackageListCmd(deps))
	cmd.AddCommand(newPackageShowCmd(deps))
	cmd.AddCommand(newPackageInstallCmd(deps))
	cmd.AddCommand(newPackageUninstallCmd(deps))
	cmd.AddCommand(newPackageExportCmd(deps))
	cmd.AddCommand(newPackageInspectCmd(deps))
	return cmd
}

func newPackageListCmd(deps Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List installed packages",
		RunE: func(c *cobra.Command, args []string) error {
			g := globalFlags(c)
			pkgs, err := deps.Services.Packages.List(c.Context())
			if err != nil {
				return err
			}
			if g.JSON {
				return jsonWrite(c.OutOrStdout(), pkgs)
			}
			return RenderPackageList(c.OutOrStdout(), pkgs)
		},
	}
}

func newPackageShowCmd(deps Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "show <name>",
		Short: "Show an installed package",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			g := globalFlags(c)
			detail, err := deps.Services.Packages.Show(c.Context(), args[0])
			if err != nil {
				if errors.Is(err, service.ErrPackageRegistryNotSupported) {
					fmt.Fprintln(c.OutOrStdout(), "(installed-package registry not yet implemented; use 'avm package inspect <file>')")
					return nil
				}
				return err
			}
			if g.JSON {
				return jsonWrite(c.OutOrStdout(), detail)
			}
			return RenderPackageDetail(c.OutOrStdout(), detail)
		},
	}
}

func newPackageInstallCmd(deps Deps) *cobra.Command {
	var (
		resolution string
	)
	cmd := &cobra.Command{
		Use:   "install <package-or-file>",
		Short: "Install a package",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			g := globalFlags(c)

			// Inspect first so we can show what will be written.
			if detail, ierr := deps.Services.Packages.Inspect(c.Context(), args[0]); ierr == nil {
				fmt.Fprintln(c.OutOrStdout(), "Package contents:")
				fmt.Fprintf(c.OutOrStdout(), "  name:    %s\n", detail.Manifest.Name)
				fmt.Fprintf(c.OutOrStdout(), "  version: %s\n", detail.Manifest.Version)
				fmt.Fprintf(c.OutOrStdout(), "  agents:  %d\n", len(detail.Manifest.Agents))
				fmt.Fprintf(c.OutOrStdout(), "  caps:    %d\n", len(detail.Manifest.Capabilities))
				if isInteractive(g) {
					ok, perr := promptConfirm("Proceed with install?")
					if perr != nil {
						return perr
					}
					if !ok {
						return errors.New("install: cancelled")
					}
				}
			}

			req := model.InstallRequest{
				Source:         args[0],
				Resolution:     model.ConflictResolution(resolution),
				NonInteractive: !isInteractive(g),
			}

			// Loop on conflict in interactive mode.
			for {
				res, err := deps.Services.Packages.Install(c.Context(), req)
				if err == nil {
					if g.JSON {
						return jsonWrite(c.OutOrStdout(), res)
					}
					return renderInstallResult(c.OutOrStdout(), res)
				}
				if !isInteractive(g) || !errors.Is(err, agentstore.ErrConflict) {
					return err
				}
				// Interactive prompt for resolution.
				name := extractConflictName(err.Error())
				choice, perr := promptSelect(fmt.Sprintf("Conflict on %q. Resolve?", name),
					[]string{"rename", "skip", "overwrite", "cancel"})
				if perr != nil {
					return perr
				}
				if choice == "cancel" {
					return errors.New("install: cancelled")
				}
				req.Resolution = model.ConflictResolution(choice)
			}
		},
	}
	cmd.Flags().StringVar(&resolution, "on-conflict", "", "rename|skip|overwrite|cancel")
	return cmd
}

func renderInstallResult(w io.Writer, r *model.InstallResult) error {
	if r == nil {
		return nil
	}
	fmt.Fprintf(w, "Installed package %q\n", r.Manifest.Name)
	if len(r.InstalledAgents) > 0 {
		fmt.Fprintf(w, "  agents:  %s\n", strings.Join(r.InstalledAgents, ", "))
	}
	if len(r.ImportedCaps) > 0 {
		ids := make([]string, 0, len(r.ImportedCaps))
		for _, id := range r.ImportedCaps {
			ids = append(ids, string(id))
		}
		fmt.Fprintf(w, "  caps:    %s\n", strings.Join(ids, ", "))
	}
	if len(r.Skipped) > 0 {
		fmt.Fprintf(w, "  skipped: %s\n", strings.Join(r.Skipped, ", "))
	}
	if len(r.Renamed) > 0 {
		fmt.Fprintln(w, "  renamed:")
		for old, n := range r.Renamed {
			fmt.Fprintf(w, "    %s -> %s\n", old, n)
		}
	}
	return nil
}

// extractConflictName tries to pull the offending agent name out of the
// service error string, which is of the form "...agent %q exists...".
func extractConflictName(s string) string {
	if i := strings.Index(s, "agent \""); i >= 0 {
		rest := s[i+len("agent \""):]
		if j := strings.Index(rest, "\""); j > 0 {
			return rest[:j]
		}
	}
	return ""
}

func newPackageUninstallCmd(deps Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall <name>",
		Short: "Uninstall a package (deletes the imported Agent)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			g := globalFlags(c)
			if isInteractive(g) {
				ok, perr := promptConfirm(fmt.Sprintf("Uninstall %q?", args[0]))
				if perr != nil {
					return perr
				}
				if !ok {
					return errors.New("uninstall: cancelled")
				}
			}
			if err := deps.Services.Packages.Uninstall(c.Context(), args[0]); err != nil {
				return err
			}
			fmt.Fprintf(c.OutOrStdout(), "Uninstalled %q\n", args[0])
			return nil
		},
	}
}

func newPackageExportCmd(deps Deps) *cobra.Command {
	var (
		out        string
		withSkills bool
		withMCP    bool
	)
	cmd := &cobra.Command{
		Use:   "export <agent>",
		Short: "Export an Agent as a package",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			res, err := deps.Services.Packages.Export(c.Context(), model.ExportRequest{
				Agent:         args[0],
				IncludeSkills: withSkills,
				IncludeMCP:    withMCP,
				OutputPath:    out,
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(c.OutOrStdout(), "Wrote %s\n", res.Path)
			return nil
		},
	}
	cmd.Flags().StringVarP(&out, "output", "o", "", "output file")
	cmd.Flags().BoolVar(&withSkills, "with-skills", true, "embed referenced skills")
	cmd.Flags().BoolVar(&withMCP, "with-mcp", true, "embed referenced MCP servers")
	return cmd
}

func newPackageInspectCmd(deps Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "inspect <file.avm.zip>",
		Short: "Inspect a package file without installing",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			g := globalFlags(c)
			detail, err := deps.Services.Packages.Inspect(c.Context(), args[0])
			if err != nil {
				return err
			}
			if g.JSON {
				return jsonWrite(c.OutOrStdout(), detail)
			}
			return RenderPackageDetail(c.OutOrStdout(), detail)
		},
	}
}
