package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func newRuntimeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "runtime",
		Short: "Inspect runtime detection and import candidates",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newRuntimeListCommand())
	cmd.AddCommand(newRuntimeScanCommand())
	return cmd
}

func newRuntimeListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List detected runtimes and import candidates",
		Args:  cobra.NoArgs,
		RunE:  runRuntimeList,
	}
}

func newRuntimeScanCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "scan",
		Short: "Refresh runtime detection and import candidates",
		Args:  cobra.NoArgs,
		RunE:  runRuntimeScan,
	}
}

func runRuntimeList(cmd *cobra.Command, args []string) error {
	if err := ensureInitialized(); err != nil {
		return err
	}
	if _, err := os.Stat(initImportReportPath()); os.IsNotExist(err) {
		if err := refreshInitImportReport(); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}
	report, err := readCreateImportReport()
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "runtime import report: %s\n", initImportReportPath())
	fmt.Fprintln(out)
	fmt.Fprintln(out, "RUNTIME\tFOUND\tCANDIDATES\tCONFIG_DIR")
	for _, runtimeReport := range report.Runtimes {
		fmt.Fprintf(out, "%s\t%s\t%d\t%s\n",
			runtimeReport.Runtime,
			yesNo(runtimeReport.Found),
			len(runtimeReport.AgentCandidates),
			cleanTableCell(runtimeReport.ConfigDir),
		)
	}

	hasCandidates := false
	for _, runtimeReport := range report.Runtimes {
		if len(runtimeReport.AgentCandidates) > 0 {
			hasCandidates = true
			break
		}
	}
	if !hasCandidates {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "no runtime import candidates found")
		fmt.Fprintln(out, "run `avm runtime scan` after adding runtime configuration")
		return nil
	}

	fmt.Fprintln(out)
	fmt.Fprintln(out, "IMPORT_CANDIDATE\tSUMMARY\tCREATE")
	for _, runtimeReport := range report.Runtimes {
		for _, candidate := range runtimeReport.AgentCandidates {
			if strings.TrimSpace(candidate.Name) == "" {
				continue
			}
			ref := runtimeReport.Runtime + "/" + candidate.Name
			fmt.Fprintf(out, "%s\t%s\tavm create --from-import %s\n",
				ref,
				cleanTableCell(candidate.Description),
				shellToken(ref),
			)
		}
	}
	return nil
}

func runRuntimeScan(cmd *cobra.Command, args []string) error {
	if err := ensureInitialized(); err != nil {
		return err
	}
	if err := refreshInitImportReport(); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "runtime import report updated: %s\n", initImportReportPath())
	return nil
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func cleanTableCell(value string) string {
	value = strings.ReplaceAll(value, "\t", " ")
	value = strings.ReplaceAll(value, "\n", " ")
	return strings.TrimSpace(value)
}

func shellToken(value string) string {
	if value == "" {
		return "''"
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			continue
		}
		switch r {
		case '-', '_', '.', '/', ':':
			continue
		default:
			return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
		}
	}
	return value
}
