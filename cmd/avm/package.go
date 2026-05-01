package main

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"
	"github.com/xz1220/agent-vm/internal/packageio"
)

func newPackageCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "package",
		Short: "Manage AVM packages",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newPackageInspectCommand(), newPackageInstallCommand(), newPackageExportCommand())
	return cmd
}

func newPackageInspectCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "inspect <file.avm.zip>",
		Short: "Inspect an AVM package file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			result, err := packageio.InspectPackage(packageio.InspectOptions{PackagePath: args[0]})
			if err != nil {
				return err
			}
			printPackageInspect(cmd, result)
			return nil
		},
	}
}

func printPackageInspect(cmd *cobra.Command, result *packageio.InspectResult) {
	out := cmd.OutOrStdout()
	manifest := result.Manifest
	fmt.Fprintf(out, "package: %s %s\n", manifest.Kind, manifest.Name)
	fmt.Fprintf(out, "version: %s\n", manifest.Version)
	printStringSection(out, "agents", manifest.Agents)
	printStringSection(out, "envs", manifest.Envs)
	printStringSection(out, "skills", manifest.Capabilities.Skills)
	printStringSection(out, "mcps", manifest.Capabilities.MCPs)
	printStringSection(out, "commands", manifest.Capabilities.Commands)
	printStringSection(out, "hooks", manifest.Capabilities.Hooks)
	printStringSection(out, "toolsets", manifest.Capabilities.Toolsets)
	if len(manifest.MemoryRefs) > 0 {
		fmt.Fprintln(out, "memory refs:")
		for _, ref := range manifest.MemoryRefs {
			fmt.Fprintf(out, "  %s/%s\n", ref.Scope, ref.ID)
		}
	}
	printStringSection(out, "files", result.Files)
}

func printStringSection(out io.Writer, label string, values []string) {
	if len(values) == 0 {
		return
	}
	fmt.Fprintf(out, "%s:\n", label)
	for _, value := range values {
		fmt.Fprintf(out, "  %s\n", value)
	}
}
