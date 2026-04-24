package memory

import (
	"fmt"
	"io"
	"text/tabwriter"
)

func WriteTextReport(w io.Writer, plan *MemoryImportPlan) error {
	if plan == nil {
		return fmt.Errorf("memory import plan is nil")
	}

	label := plan.Runtime
	if label == "" {
		label = "file"
	}
	if _, err := fmt.Fprintf(w, "Memory import dry-run: %s\n", label); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Source: %s\n\n", plan.Source); err != nil {
		return err
	}

	if _, err := fmt.Fprintln(w, "Status counts:"); err != nil {
		return err
	}
	for _, count := range plan.StatusCounts {
		if _, err := fmt.Fprintf(w, "  %s: %d\n", count.Status, count.Count); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintln(w, "\nDiffs:"); err != nil {
		return err
	}
	if len(plan.Diffs) == 0 {
		if _, err := fmt.Fprintln(w, "  none"); err != nil {
			return err
		}
	} else {
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		if _, err := fmt.Fprintln(tw, "  STATUS\tID\tSCOPE\tTARGET\tPREVIEW"); err != nil {
			return err
		}
		for _, diff := range plan.Diffs {
			scope := ""
			for _, candidate := range plan.Candidates {
				if candidate.ID == diff.MemoryID {
					scope = candidate.Scope
					break
				}
			}
			if _, err := fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%s\n", diff.Status, diff.MemoryID, scope, diff.TargetPath, diff.Preview); err != nil {
				return err
			}
		}
		if err := tw.Flush(); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintln(w, "\nWarnings:"); err != nil {
		return err
	}
	if len(plan.Warnings) == 0 {
		if _, err := fmt.Fprintln(w, "  none"); err != nil {
			return err
		}
	} else {
		for _, warning := range plan.Warnings {
			if _, err := fmt.Fprintf(w, "  %s\n", warning); err != nil {
				return err
			}
		}
	}

	_, err := fmt.Fprintln(w, "\nNo files written.")
	return err
}
