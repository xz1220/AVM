package main

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/xz1220/agent-vm/internal/memory"
)

func newMemoryCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "memory",
		Short: "Manage AVM portable memory",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newMemoryImportCommand())
	return cmd
}

func newMemoryImportCommand() *cobra.Command {
	var from string
	var dryRun bool
	var format string

	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import portable memory candidates",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMemoryImport(cmd, args, from, dryRun, format)
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "memory source path or runtime")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview memory import without writing")
	cmd.Flags().StringVar(&format, "format", "text", "output format")
	return cmd
}

func runMemoryImport(cmd *cobra.Command, args []string, from string, dryRun bool, format string) error {
	if from == "" && !dryRun {
		return notImplemented(cmd, args)
	}
	if from == "" {
		return fmt.Errorf("%s: --from is required", cmd.CommandPath())
	}
	if !dryRun {
		return fmt.Errorf("%s: only --dry-run is implemented", cmd.CommandPath())
	}

	plan, err := memory.ImportDryRun(memory.ImportOptions{
		Source: from,
		DryRun: true,
	})
	if err != nil {
		return err
	}

	switch format {
	case "", "text":
		return memory.WriteTextReport(cmd.OutOrStdout(), plan)
	case "json":
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetEscapeHTML(false)
		encoder.SetIndent("", "  ")
		return encoder.Encode(plan)
	default:
		return fmt.Errorf("%s: unsupported --format %q", cmd.CommandPath(), format)
	}
}
