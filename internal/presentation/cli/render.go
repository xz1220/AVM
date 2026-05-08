package cli

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/xz1220/agent-vm/internal/app/model"
	"github.com/xz1220/agent-vm/internal/presentation/render"
)

// statusIcon renders a tri-state mapping status as ASCII (no emoji).
func statusIcon(s model.MappingStatus) string {
	switch s {
	case model.MappingNative:
		return "OK"
	case model.MappingRenderedAsInstructions:
		return "INS"
	case model.MappingIgnored:
		return "--"
	case model.MappingUnsupported:
		return "NO"
	default:
		return "?"
	}
}

// RenderAgentList renders agents as a column-aligned table.
func RenderAgentList(w io.Writer, items []model.AgentSummary) error {
	if len(items) == 0 {
		return render.Linef(w, "(no agents)")
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tDESCRIPTION\tRUNTIMES")
	for _, it := range items {
		runtimes := strings.Join(it.Runtimes, ",")
		if runtimes == "" {
			runtimes = "-"
		}
		desc := it.Description
		if desc == "" {
			desc = "-"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\n", it.Name, desc, runtimes)
	}
	return tw.Flush()
}

// RenderAgentDetail renders the human-readable show view.
func RenderAgentDetail(w io.Writer, d *model.AgentDetail) error {
	if d == nil {
		return render.Linef(w, "(no agent)")
	}
	a := d.Agent
	if _, err := fmt.Fprintf(w, "Name:        %s\n", a.Identity.Name); err != nil {
		return err
	}
	if a.Identity.Description != "" {
		fmt.Fprintf(w, "Description: %s\n", a.Identity.Description)
	}
	if a.Identity.Role != "" {
		fmt.Fprintf(w, "Role:        %s\n", a.Identity.Role)
	}
	if d.SourcePath != "" {
		fmt.Fprintf(w, "Source:      %s\n", d.SourcePath)
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "Instructions:")
	if a.Instructions.System != "" {
		fmt.Fprintf(w, "  system: %s\n", oneLine(a.Instructions.System))
	}
	if a.Instructions.Inline != "" {
		fmt.Fprintf(w, "  inline: %s\n", oneLine(a.Instructions.Inline))
	}
	if len(a.Instructions.Files) > 0 {
		fmt.Fprintf(w, "  files:  %s\n", strings.Join(a.Instructions.Files, ", "))
	}
	if a.Instructions.System == "" && a.Instructions.Inline == "" && len(a.Instructions.Files) == 0 {
		fmt.Fprintln(w, "  (none)")
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "Skills:")
	if len(a.Skills) == 0 {
		fmt.Fprintln(w, "  (none)")
	} else {
		for _, s := range a.Skills {
			fmt.Fprintf(w, "  - %s (%s)\n", s.ID, s.Kind)
		}
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "MCP:")
	if len(a.MCP) == 0 {
		fmt.Fprintln(w, "  (none)")
	} else {
		for _, m := range a.MCP {
			fmt.Fprintf(w, "  - %s (%s)\n", m.ID, m.Kind)
		}
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "Runtimes:")
	if len(a.Runtimes) == 0 {
		fmt.Fprintln(w, "  (none)")
	} else {
		for _, r := range a.Runtimes {
			marker := ""
			if r.Default {
				marker = " (default)"
			}
			fmt.Fprintf(w, "  - %s%s\n", r.Runtime, marker)
		}
	}

	if len(d.Mapping) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Runtime mapping:")
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "  RUNTIME\tFIELD\tSTATUS\tNOTE")
		for _, m := range d.Mapping {
			if len(m.Fields) == 0 && len(m.Warnings) == 0 {
				fmt.Fprintf(tw, "  %s\t-\t-\t-\n", m.Runtime)
				continue
			}
			for _, f := range m.Fields {
				fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\n", m.Runtime, f.Field, statusIcon(f.Status), f.Note)
			}
			for _, wn := range m.Warnings {
				fmt.Fprintf(tw, "  %s\t!\twarn\t%s\n", m.Runtime, wn)
			}
		}
		tw.Flush()
	}
	return nil
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 120 {
		return s[:117] + "..."
	}
	return s
}

// RenderRunPreview renders preview as a human-readable block.
func RenderRunPreview(w io.Writer, p *model.RunPreview) error {
	if p == nil {
		return render.Linef(w, "(no preview)")
	}
	fmt.Fprintf(w, "Agent:    %s\n", p.Agent)
	fmt.Fprintf(w, "Runtime:  %s\n", p.Runtime)
	if p.Boundary.StateDir != "" {
		fmt.Fprintf(w, "Boundary: %s\n", p.Boundary.StateDir)
	}
	if len(p.Boundary.EnvKeys) > 0 {
		fmt.Fprintf(w, "Env:      %s\n", strings.Join(p.Boundary.EnvKeys, ", "))
	}
	if len(p.WritePaths) > 0 {
		fmt.Fprintln(w, "Will write:")
		for _, wp := range p.WritePaths {
			fmt.Fprintf(w, "  - %s\n", wp)
		}
	}
	if len(p.Mapping) > 0 {
		fmt.Fprintln(w, "Mapping:")
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "  FIELD\tSTATUS\tNOTE")
		for _, m := range p.Mapping {
			fmt.Fprintf(tw, "  %s\t%s\t%s\n", m.Field, statusIcon(m.Status), m.Note)
		}
		tw.Flush()
	}
	if len(p.Drift) > 0 {
		fmt.Fprintln(w, "Drift:")
		for _, d := range p.Drift {
			fmt.Fprintf(w, "  - %s [%s] %s\n", d.Path, d.Field, d.Reason)
		}
	}
	if len(p.Warnings) > 0 {
		fmt.Fprintln(w, "Warnings:")
		for _, wn := range p.Warnings {
			fmt.Fprintf(w, "  - %s: %s\n", wn.Code, wn.Message)
		}
	}
	return nil
}

// RenderRunResult renders the final outcome.
func RenderRunResult(w io.Writer, r *model.RunResult) error {
	if r == nil {
		return render.Linef(w, "(no result)")
	}
	if err := RenderRunPreview(w, &r.Preview); err != nil {
		return err
	}
	fmt.Fprintln(w)
	fmt.Fprintf(w, "Started: %s\n", r.StartedAt.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(w, "Ended:   %s\n", r.EndedAt.Format("2006-01-02 15:04:05"))
	fmt.Fprintf(w, "Exit:    %d\n", r.ExitCode)
	return nil
}

// RenderPackageList renders installed packages.
func RenderPackageList(w io.Writer, items []model.PackageSummary) error {
	if len(items) == 0 {
		return render.Linef(w, "(installed-package registry not yet implemented; use 'avm package inspect <file>')")
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tVERSION\tDESCRIPTION")
	for _, it := range items {
		desc := it.Description
		if desc == "" {
			desc = "-"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\n", it.Name, it.Version, desc)
	}
	return tw.Flush()
}

// RenderPackageDetail renders manifest + file list.
func RenderPackageDetail(w io.Writer, d *model.PackageDetail) error {
	if d == nil {
		return render.Linef(w, "(no package)")
	}
	fmt.Fprintf(w, "Name:        %s\n", d.Manifest.Name)
	if d.Manifest.Version != "" {
		fmt.Fprintf(w, "Version:     %s\n", d.Manifest.Version)
	}
	if d.Manifest.Description != "" {
		fmt.Fprintf(w, "Description: %s\n", d.Manifest.Description)
	}
	if d.Manifest.Author != "" {
		fmt.Fprintf(w, "Author:      %s\n", d.Manifest.Author)
	}
	if d.Source != "" {
		fmt.Fprintf(w, "Source:      %s\n", d.Source)
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "Agents:")
	if len(d.Manifest.Agents) == 0 {
		fmt.Fprintln(w, "  (none)")
	} else {
		for _, a := range d.Manifest.Agents {
			fmt.Fprintf(w, "  - %s (%s)\n", a.Name, a.Path)
		}
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "Capabilities:")
	if len(d.Manifest.Capabilities) == 0 {
		fmt.Fprintln(w, "  (none)")
	} else {
		for _, c := range d.Manifest.Capabilities {
			fmt.Fprintf(w, "  - %s/%s -> %s\n", c.Kind, c.Name, c.Path)
		}
	}

	if len(d.Files) > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Files:")
		for _, f := range d.Files {
			fmt.Fprintf(w, "  - %s\n", f)
		}
	}
	return nil
}

// RenderDoctor renders the doctor report as a human-readable block.
func RenderDoctor(w io.Writer, r *model.DoctorReport) error {
	if r == nil {
		return render.Linef(w, "(no report)")
	}
	fmt.Fprintf(w, "AVM home:          %s\n", checkLine(r.AVMHome))
	fmt.Fprintf(w, "PATH:              %s\n", checkLine(r.PATH))
	fmt.Fprintf(w, "Shell integration: %s\n", checkLine(r.ShellIntegration))
	if len(r.Runtimes) == 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Runtimes: (none registered)")
		return nil
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Runtimes:")
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  NAME\tAVAILABLE\tBINARY\tVERSION")
	for _, rc := range r.Runtimes {
		avail := "no"
		if rc.Available {
			avail = "yes"
		}
		bin := rc.Binary
		if bin == "" {
			bin = "-"
		}
		ver := rc.Version
		if ver == "" {
			ver = "-"
		}
		fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\n", rc.Runtime, avail, bin, ver)
	}
	tw.Flush()
	for _, rc := range r.Runtimes {
		if len(rc.Issues) == 0 {
			continue
		}
		fmt.Fprintf(w, "  %s issues:\n", rc.Runtime)
		for _, iss := range rc.Issues {
			fmt.Fprintf(w, "    - %s\n", iss)
		}
	}
	return nil
}

// RenderRuntimeList renders a list of runtime probes as a human table.
// Same payload `avm doctor` uses, minus the surrounding doctor checks.
func RenderRuntimeList(w io.Writer, items []model.RuntimeCheck) error {
	if len(items) == 0 {
		return render.Linef(w, "(no runtimes registered)")
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tAVAILABLE\tBINARY\tVERSION")
	for _, rc := range items {
		avail := "no"
		if rc.Available {
			avail = "yes"
		}
		bin := rc.Binary
		if bin == "" {
			bin = "-"
		}
		ver := rc.Version
		if ver == "" {
			ver = "-"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", rc.Runtime, avail, bin, ver)
	}
	tw.Flush()
	for _, rc := range items {
		if len(rc.Issues) == 0 {
			continue
		}
		fmt.Fprintf(w, "%s issues:\n", rc.Runtime)
		for _, iss := range rc.Issues {
			fmt.Fprintf(w, "  - %s\n", iss)
		}
	}
	return nil
}

func checkLine(c model.CheckResult) string {
	tag := "FAIL"
	if c.OK {
		tag = "OK"
	}
	if c.Detail != "" {
		return fmt.Sprintf("%s (%s)", tag, c.Detail)
	}
	return tag
}

// RenderCapabilityList renders a unified capability candidate list.
func RenderCapabilityList(w io.Writer, items []model.CapabilityCandidate) error {
	if len(items) == 0 {
		return render.Linef(w, "(no capabilities discovered)")
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "KIND\tNAME\tSOURCE\tSTATUS\tID/PATH")
	for _, c := range items {
		status := "-"
		switch {
		case c.Imported && c.Conflict:
			status = "imported,conflict"
		case c.Imported:
			status = "imported"
		case c.Conflict:
			status = "conflict"
		}
		idOrPath := "-"
		switch {
		case c.Record != nil:
			idOrPath = string(c.Record.ID)
		case c.Global != nil && c.Global.Path != "":
			idOrPath = c.Global.Path
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			c.Kind, c.Name, c.Source, status, idOrPath)
	}
	return tw.Flush()
}

// RenderCapabilityRecords renders capstore-only records as a column table.
func RenderCapabilityRecords(w io.Writer, items []model.CapabilityRecord) error {
	if len(items) == 0 {
		return render.Linef(w, "(capability store empty)")
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tKIND\tNAME\tSOURCE\tVERSION\tIMPORT_FROM")
	for _, r := range items {
		ver := r.Version
		if ver == "" {
			ver = "-"
		}
		src := string(r.Source)
		if src == "" {
			src = "-"
		}
		from := r.ImportFrom
		if from == "" {
			from = "-"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			r.ID, r.Kind, r.Name, src, ver, from)
	}
	return tw.Flush()
}

// RenderCapabilityRecord renders a single record as a key:value block.
func RenderCapabilityRecord(w io.Writer, r *model.CapabilityRecord) error {
	if r == nil {
		return render.Linef(w, "(no capability)")
	}
	fmt.Fprintf(w, "ID:          %s\n", r.ID)
	fmt.Fprintf(w, "Kind:        %s\n", r.Kind)
	fmt.Fprintf(w, "Name:        %s\n", r.Name)
	if r.Version != "" {
		fmt.Fprintf(w, "Version:     %s\n", r.Version)
	}
	if r.Source != "" {
		fmt.Fprintf(w, "Source:      %s\n", r.Source)
	}
	if r.Checksum != "" {
		fmt.Fprintf(w, "Checksum:    %s\n", r.Checksum)
	}
	if r.ImportFrom != "" {
		fmt.Fprintf(w, "Import from: %s\n", r.ImportFrom)
	}
	if r.Format != "" {
		fmt.Fprintf(w, "Format:      %s\n", r.Format)
	}
	return nil
}

// RenderImportResult renders the outcome of a single capability import.
func RenderImportResult(w io.Writer, r *model.ImportCapabilityResult) error {
	if r == nil {
		return render.Linef(w, "(no result)")
	}
	verb := "imported"
	switch {
	case r.Replaced:
		verb = "replaced"
	case !r.Created:
		verb = "deduped (already in capstore)"
	}
	if _, err := fmt.Fprintf(w, "%s %s\n", verb, r.ID); err != nil {
		return err
	}
	if r.Source != "" {
		fmt.Fprintf(w, "  source: %s\n", r.Source)
	}
	return nil
}

// RenderBootstrapResult renders a bootstrap summary.
func RenderBootstrapResult(w io.Writer, r *model.BootstrapCapabilitiesResult) error {
	if r == nil {
		return render.Linef(w, "(no result)")
	}
	if len(r.Imported) == 0 && len(r.Skipped) == 0 {
		return render.Linef(w, "(nothing to import)")
	}
	if len(r.Imported) > 0 {
		fmt.Fprintf(w, "Imported %d capabilit%s:\n",
			len(r.Imported), pluralY(len(r.Imported)))
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "  ID\tCREATED\tREPLACED\tSOURCE")
		for _, it := range r.Imported {
			fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\n",
				it.ID, yesNo(it.Created), yesNo(it.Replaced), it.Source)
		}
		tw.Flush()
	}
	if len(r.Skipped) > 0 {
		fmt.Fprintf(w, "Skipped %d:\n", len(r.Skipped))
		for _, sk := range r.Skipped {
			fmt.Fprintf(w, "  - %s/%s: %s\n", sk.Kind, sk.Name, sk.Reason)
		}
	}
	return nil
}

func pluralY(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// RenderStatus renders the status report.
func RenderStatus(w io.Writer, r *model.StatusReport) error {
	if r == nil {
		return render.Linef(w, "(no status)")
	}
	fmt.Fprintln(w, "Agents:")
	if len(r.Agents) == 0 {
		fmt.Fprintln(w, "  (none)")
	} else {
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "  NAME\tDESCRIPTION\tRUNTIMES")
		for _, a := range r.Agents {
			desc := a.Description
			if desc == "" {
				desc = "-"
			}
			rts := strings.Join(a.Runtimes, ",")
			if rts == "" {
				rts = "-"
			}
			fmt.Fprintf(tw, "  %s\t%s\t%s\n", a.Name, desc, rts)
		}
		tw.Flush()
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "Runtimes:")
	if len(r.Runtimes) == 0 {
		fmt.Fprintln(w, "  (none)")
	} else {
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "  NAME\tAVAILABLE\tBINARY\tVERSION")
		for _, rc := range r.Runtimes {
			avail := "no"
			if rc.Available {
				avail = "yes"
			}
			bin := rc.Binary
			if bin == "" {
				bin = "-"
			}
			ver := rc.Version
			if ver == "" {
				ver = "-"
			}
			fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\n", rc.Runtime, avail, bin, ver)
		}
		tw.Flush()
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "Recent runs:")
	if len(r.RecentRuns) == 0 {
		fmt.Fprintln(w, "  (none)")
		return nil
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "  STARTED\tAGENT\tRUNTIME\tEXIT")
	for _, run := range r.RecentRuns {
		fmt.Fprintf(tw, "  %s\t%s\t%s\t%d\n",
			run.StartedAt.Format("2006-01-02 15:04:05"),
			run.Agent, run.Runtime, run.ExitCode)
	}
	return tw.Flush()
}
