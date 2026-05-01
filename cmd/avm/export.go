package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/xz1220/agent-vm/internal/packageio"
)

func newPackageExportCommand() *cobra.Command {
	var output string
	var kind string

	cmd := &cobra.Command{
		Use:   "export <agent>",
		Short: "Export an AVM agent package",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPackageExport(cmd, args[0], output, kind)
		},
	}
	cmd.Flags().StringVar(&output, "output", "", "output .avm.zip file")
	cmd.Flags().StringVar(&kind, "kind", "", "export kind: agent (default) or env")
	return cmd
}

func runPackageExport(cmd *cobra.Command, name, output, kind string) error {
	if output == "" {
		return fmt.Errorf("%s: --output is required", cmd.CommandPath())
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	result, err := packageio.ExportPackage(packageio.ExportOptions{
		Name:       name,
		Kind:       kind,
		OutputPath: output,
		CWD:        cwd,
	})
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "exported %s %s to %s\n", result.Manifest.Kind, result.Manifest.Name, result.Output)
	for _, warning := range result.Warnings {
		fmt.Fprintf(out, "warning: %s\n", warning)
	}
	return nil
}
