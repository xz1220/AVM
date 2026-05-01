package main

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"github.com/xz1220/agent-vm/internal/packageio"
)

var httpClient packageio.HTTPClient

func newPackageInstallCommand() *cobra.Command {
	var dryRun bool
	var checksum string
	cmd := &cobra.Command{
		Use:   "install <source>",
		Short: "Install an AVM package",
		Long: `Install an AVM package from a local file, URL, or GitHub Release.

Sources:
  local file:       avm package install backend-coder.avm.zip
  HTTP/HTTPS URL:   avm package install https://example.com/pkg.avm.zip
  GitHub Release:   avm package install github:owner/repo@v1.0.0
  GitHub (latest):  avm package install github:owner/repo`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPackageInstall(cmd, args[0], dryRun, checksum)
		},
	}
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview package install without writing files")
	cmd.Flags().StringVar(&checksum, "checksum", "", "verify checksum (format: sha256:<hex>)")
	return cmd
}

func runPackageInstall(cmd *cobra.Command, source string, dryRun bool, checksum string) error {
	packagePath := source
	kind, resolved := packageio.ResolveSource(source)

	if kind == packageio.SourceGitHub {
		url, err := packageio.GitHubReleaseURL(resolved)
		if err != nil {
			return err
		}
		resolved = url
		kind = packageio.SourceHTTP
	}

	if kind == packageio.SourceHTTP {
		fmt.Fprintf(cmd.OutOrStdout(), "downloading %s\n", resolved)
		downloaded, cleanup, err := packageio.DownloadPackage(packageio.DownloadOptions{
			URL:      resolved,
			Dir:      os.TempDir(),
			Checksum: checksum,
			Client:   httpClient,
		})
		if err != nil {
			return err
		}
		defer cleanup()
		packagePath = downloaded
	} else if checksum != "" {
		if err := packageio.VerifyChecksum(packagePath, checksum); err != nil {
			return err
		}
	}

	result, err := installPackageFromPath(packagePath, dryRun)
	if err != nil {
		return err
	}
	if dryRun {
		printInstallDryRun(cmd, result)
		return nil
	}
	fmt.Fprintf(cmd.OutOrStdout(), "installed %s %s: added %d, skipped %d\n", result.Manifest.Kind, result.Manifest.Name, len(result.Added), len(result.Skipped))
	return nil
}

func installPackageFromPath(packagePath string, dryRun bool) (*packageio.ImportResult, error) {
	if !dryRun {
		if err := ensureInitialized(); err != nil {
			return nil, err
		}
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	return packageio.ImportPackage(packageio.ImportOptions{
		PackagePath: packagePath,
		CWD:         cwd,
		DryRun:      dryRun,
	})
}

func printInstallDryRun(cmd *cobra.Command, result *packageio.ImportResult) {
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "install plan for %s %s: add %d, skip %d, conflict %d\n", result.Manifest.Kind, result.Manifest.Name, len(result.Added), len(result.Skipped), len(result.Conflicts))
	printImportActions(out, "would add", result.Added)
	printImportActions(out, "would skip", result.Skipped)
	printImportActions(out, "conflicts", result.Conflicts)
}

func printImportActions(out io.Writer, label string, actions []packageio.ImportAction) {
	if len(actions) == 0 {
		return
	}
	fmt.Fprintf(out, "%s:\n", label)
	for _, action := range actions {
		fmt.Fprintf(out, "  %s -> %s\n", action.PackagePath, action.TargetPath)
	}
}
