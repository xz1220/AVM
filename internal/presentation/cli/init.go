package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func newInitCmd(deps Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Initialize ~/.avm",
		RunE: func(c *cobra.Command, args []string) error {
			res, err := deps.Services.System.Init(c.Context())
			if err != nil {
				return err
			}
			if res.AlreadyExists {
				fmt.Fprintf(c.OutOrStdout(), "already initialized at %s\n", res.Root)
				fmt.Fprintln(c.OutOrStdout(), "\nNext:\n  avm setup")
				return nil
			}
			fmt.Fprintln(c.OutOrStdout(), "Created:")
			for _, p := range res.CreatedPaths {
				fmt.Fprintf(c.OutOrStdout(), "  %s\n", p)
			}
			fmt.Fprintln(c.OutOrStdout(), "\nNext:\n  avm setup")
			return nil
		},
	}
}

func newDoctorCmd(deps Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose AVM and runtime state",
		RunE: func(c *cobra.Command, args []string) error {
			g := globalFlags(c)
			rep, err := deps.Services.Diagnostics.Doctor(c.Context())
			if err != nil {
				return err
			}
			if g.JSON {
				return jsonWrite(c.OutOrStdout(), rep)
			}
			return RenderDoctor(c.OutOrStdout(), rep)
		},
	}
}

func newStatusCmd(deps Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "status [agent]",
		Short: "Show AVM status",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			g := globalFlags(c)
			agent := ""
			if len(args) > 0 {
				agent = args[0]
			}
			rep, err := deps.Services.Diagnostics.Status(c.Context(), agent)
			if err != nil {
				return err
			}
			if g.JSON {
				return jsonWrite(c.OutOrStdout(), rep)
			}
			return RenderStatus(c.OutOrStdout(), rep)
		},
	}
}

func newUninstallCmd(deps Deps) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Uninstall AVM (binary / shell integration / data)",
		RunE: func(c *cobra.Command, args []string) error {
			binPath, _ := os.Executable()
			if binPath == "" {
				binPath = os.Args[0]
			}
			if !yes {
				root, err := deps.Services.System.HomeRoot(c.Context())
				if err != nil {
					return err
				}
				fmt.Fprintln(c.OutOrStdout(), "Would remove:")
				fmt.Fprintf(c.OutOrStdout(), "  binary:        %s\n", binPath)
				fmt.Fprintf(c.OutOrStdout(), "  AVM home:      %s\n", root)
				fmt.Fprintln(c.OutOrStdout(), "Re-run with --yes to apply.")
				return nil
			}
			res, err := deps.Services.System.UninstallHome(c.Context())
			if err != nil {
				return err
			}
			if res.Removed {
				fmt.Fprintf(c.OutOrStdout(), "Removed %s\n", res.Root)
			} else {
				fmt.Fprintf(c.OutOrStdout(), "AVM home %s not present; nothing to remove.\n", res.Root)
			}
			if err := os.Remove(binPath); err != nil {
				if os.IsPermission(err) {
					fmt.Fprintf(c.OutOrStdout(), "Could not remove binary at %s (permission denied); remove manually.\n", binPath)
					return nil
				}
				if errors.Is(err, os.ErrNotExist) {
					return nil
				}
				return fmt.Errorf("remove binary: %w", err)
			}
			fmt.Fprintf(c.OutOrStdout(), "Removed %s\n", binPath)
			return nil
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "actually delete files (otherwise dry run)")
	return cmd
}

func newShellCmd(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "shell",
		Short: "Manage AVM shell integration",
	}
	cmd.AddCommand(newShellInstallCmd(deps))
	cmd.AddCommand(newShellUninstallCmd(deps))
	return cmd
}

// detectShell returns the shell name (bash|zsh|fish) inferred from $SHELL.
func detectShell() (string, error) {
	sh := os.Getenv("SHELL")
	if sh == "" {
		return "", errors.New("shell: $SHELL is empty; pass --shell")
	}
	base := filepath.Base(sh)
	switch base {
	case "bash", "zsh", "fish":
		return base, nil
	}
	for _, k := range []string{"bash", "zsh", "fish"} {
		if strings.HasPrefix(base, k) {
			return k, nil
		}
	}
	return "", fmt.Errorf("shell: unsupported shell %q", base)
}

func newShellInstallCmd(deps Deps) *cobra.Command {
	var shellOverride string
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install shell integration",
		RunE: func(c *cobra.Command, args []string) error {
			shell := shellOverride
			if shell == "" {
				s, err := detectShell()
				if err != nil {
					return err
				}
				shell = s
			}
			path, err := deps.Services.System.CompletionPath(c.Context(), shell)
			if err != nil {
				return err
			}
			root := c.Root()
			switch shell {
			case "bash":
				if err := root.GenBashCompletionFile(path); err != nil {
					return err
				}
			case "zsh":
				if err := root.GenZshCompletionFile(path); err != nil {
					return err
				}
			case "fish":
				if err := root.GenFishCompletionFile(path, true); err != nil {
					return err
				}
			default:
				return fmt.Errorf("shell: unsupported %q", shell)
			}
			fmt.Fprintf(c.OutOrStdout(), "Wrote %s\n", path)
			fmt.Fprintf(c.OutOrStdout(), "Add this line to your %s rc file:\n", shell)
			fmt.Fprintf(c.OutOrStdout(), "  source %s\n", path)
			return nil
		},
	}
	cmd.Flags().StringVar(&shellOverride, "shell", "", "shell name (bash|zsh|fish)")
	return cmd
}

func newShellUninstallCmd(deps Deps) *cobra.Command {
	var shellOverride string
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Uninstall shell integration",
		RunE: func(c *cobra.Command, args []string) error {
			shell := shellOverride
			if shell == "" {
				s, err := detectShell()
				if err != nil {
					return err
				}
				shell = s
			}
			// Compute the path through the service, then delete via the
			// service. CLI never touches infra paths directly.
			path, err := deps.Services.System.CompletionPath(c.Context(), shell)
			if err != nil {
				return err
			}
			if _, statErr := os.Stat(path); errors.Is(statErr, os.ErrNotExist) {
				fmt.Fprintf(c.OutOrStdout(), "(nothing to remove at %s)\n", path)
				return nil
			}
			if err := deps.Services.System.RemoveCompletion(c.Context(), shell); err != nil {
				return err
			}
			fmt.Fprintf(c.OutOrStdout(), "Removed %s\n", path)
			fmt.Fprintf(c.OutOrStdout(), "Remove this line from your %s rc file if present:\n", shell)
			fmt.Fprintf(c.OutOrStdout(), "  source %s\n", path)
			return nil
		},
	}
	cmd.Flags().StringVar(&shellOverride, "shell", "", "shell name (bash|zsh|fish)")
	return cmd
}
