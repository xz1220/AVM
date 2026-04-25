package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/xz1220/agent-vm/internal/packageio"
)

func newImportCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "import <file.avm.zip>",
		Short: "Import an AVM package",
		Args:  cobra.ExactArgs(1),
		RunE:  runImport,
	}
}

func runImport(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	result, err := packageio.ImportPackage(packageio.ImportOptions{
		PackagePath: args[0],
		CWD:         cwd,
	})
	if err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "imported %s %s: added %d, skipped %d\n", result.Manifest.Kind, result.Manifest.Name, len(result.Added), len(result.Skipped))
	return nil
}
