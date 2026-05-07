package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/xz1220/agent-vm/internal/app/model"
	"github.com/xz1220/agent-vm/internal/infra/agentstore"
	"github.com/xz1220/agent-vm/internal/presentation/render"
)

func newAgentCmd(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Manage AVM Agents (PRD §4.2)",
	}
	cmd.AddCommand(newAgentCreateCmd(deps))
	cmd.AddCommand(newAgentListCmd(deps))
	cmd.AddCommand(newAgentShowCmd(deps))
	cmd.AddCommand(newAgentEditCmd(deps))
	cmd.AddCommand(newAgentDeleteCmd(deps))
	cmd.AddCommand(newAgentCloneCmd(deps))
	cmd.AddCommand(newAgentRenameCmd(deps))
	return cmd
}

// agentCreateFlags exposes the non-interactive subset of the create
// flow. The interactive flow (PRD §4.2) is the canonical one and is
// preferred; flags are a scripted-mode escape hatch.
type agentCreateFlags struct {
	name        string
	description string
	role        string
	system      string
	skills      []string
	mcps        []string
	runtimes    []string
	defaultRT   string
	onConflict  string
	source      string
}

func newAgentCreateCmd(deps Deps) *cobra.Command {
	f := agentCreateFlags{}
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create a new Agent",
		RunE: func(c *cobra.Command, args []string) error {
			g := globalFlags(c)
			if isInteractive(g) {
				return runAgentCreateInteractive(c.Context(), c.OutOrStdout(), deps, f, g)
			}
			return runAgentCreateNonInteractive(c.Context(), c.OutOrStdout(), deps, f, g)
		},
	}
	cmd.Flags().StringVar(&f.name, "name", "", "Agent name (required in non-interactive mode)")
	cmd.Flags().StringVar(&f.description, "description", "", "human-readable description")
	cmd.Flags().StringVar(&f.role, "role", "", "Agent role tag")
	cmd.Flags().StringVar(&f.system, "system", "", "system instructions")
	cmd.Flags().StringSliceVar(&f.skills, "skill", nil, "capability ID for a skill (repeatable)")
	cmd.Flags().StringSliceVar(&f.mcps, "mcp", nil, "capability ID for an MCP server (repeatable)")
	cmd.Flags().StringSliceVar(&f.runtimes, "runtime", nil, "runtime preference (repeatable)")
	cmd.Flags().StringVar(&f.defaultRT, "default-runtime", "", "default runtime when multiple are configured")
	cmd.Flags().StringVar(&f.onConflict, "on-conflict", "", "ask|skip|overwrite|cancel|rename")
	cmd.Flags().StringVar(&f.source, "source", "blank", "blank|default|package")
	return cmd
}

func runAgentCreateNonInteractive(ctx context.Context, out io.Writer, deps Deps, f agentCreateFlags, g globals) error {
	if f.name == "" {
		return newMissingInputErr("non-interactive create requires --name and at least one --runtime")
	}
	if len(f.runtimes) == 0 {
		return newMissingInputErr("non-interactive create requires --name and at least one --runtime")
	}
	req := buildCreateRequest(f, true)
	a, err := deps.Services.Agents.Create(ctx, req)
	if err != nil {
		return err
	}
	if g.JSON {
		return jsonWrite(out, a)
	}
	fmt.Fprintf(out, "Created agent %q\n", a.Identity.Name)
	return nil
}

func runAgentCreateInteractive(ctx context.Context, out io.Writer, deps Deps, f agentCreateFlags, g globals) error {
	// Step 1: source
	source := f.source
	if source == "" {
		s, err := promptSelect("Source for the new Agent", []string{"blank", "default", "package"})
		if err != nil {
			return err
		}
		source = s
	}
	if model.CreateSource(source) == model.CreateSourcePackage {
		return errors.New("agent create: package source not yet wired through CLI; install the package first with 'avm package install'")
	}

	// Step 2: name (loop until valid + non-existing)
	name := f.name
	for {
		if name == "" {
			if err := promptInput("Agent name (lowercase, [a-z0-9-])", &name); err != nil {
				return err
			}
		}
		if name == "" {
			return errors.New("agent create: empty name")
		}
		probe := &model.Agent{Identity: model.Identity{Name: name}}
		if err := probe.Validate(); err != nil {
			fmt.Fprintf(out, "invalid name: %v\n", err)
			name = ""
			continue
		}
		// Check existence; tolerate the case where Repo is unavailable.
		if exists, err := agentExists(deps, name); err == nil && exists {
			fmt.Fprintf(out, "agent %q already exists; pick another name\n", name)
			name = ""
			continue
		}
		break
	}

	// Step 3: description
	desc := f.description
	if desc == "" {
		_ = promptInput("Description (optional)", &desc)
	}
	// Step 4: instructions
	system := f.system
	if system == "" {
		_ = promptInput("System instructions (optional, single line)", &system)
	}

	// Step 5/6: skills/mcp from CapabilityService.Discover
	skills, err := pickCapabilities(ctx, deps, model.CapabilityKindSkill, "Select skills")
	if err != nil {
		return err
	}
	mcps, err := pickCapabilities(ctx, deps, model.CapabilityKindMCP, "Select MCP servers")
	if err != nil {
		return err
	}

	// Step 7: runtime preferences
	runtimeNames := registeredRuntimes(deps)
	rts, err := promptMultiSelect("Runtime preferences", runtimeNames)
	if err != nil {
		return err
	}
	if len(rts) == 0 {
		return errors.New("agent create: select at least one runtime")
	}
	prefs := make([]model.RuntimePref, 0, len(rts))
	for i, r := range rts {
		prefs = append(prefs, model.RuntimePref{Runtime: r, Default: i == 0 && len(rts) > 1})
	}

	// Step 8: preview
	previewAgent := &model.Agent{
		Identity:     model.Identity{Name: name, Description: desc, Role: f.role},
		Instructions: model.Instructions{System: system},
		Skills:       skills,
		MCP:          mcps,
		Runtimes:     prefs,
	}
	fmt.Fprintln(out, "--- preview ---")
	_ = RenderAgentDetail(out, &model.AgentDetail{Agent: *previewAgent})

	// Step 9: confirm
	ok, err := promptConfirm("Create this Agent?")
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("agent create: cancelled")
	}

	// Issue Create. Loop on conflict in interactive mode.
	req := model.CreateAgentRequest{
		Name:         name,
		Description:  desc,
		Role:         f.role,
		Instructions: model.Instructions{System: system},
		Skills:       skills,
		MCP:          mcps,
		Runtimes:     prefs,
		Source:       model.CreateSource(source),
	}
	for {
		a, err := deps.Services.Agents.Create(ctx, req)
		if err == nil {
			fmt.Fprintf(out, "Created agent %q\n", a.Identity.Name)
			return nil
		}
		if !errors.Is(err, agentstore.ErrConflict) {
			return err
		}
		choice, perr := promptSelect(fmt.Sprintf("Agent %q exists; resolve?", req.Name),
			[]string{"rename", "overwrite", "cancel"})
		if perr != nil {
			return perr
		}
		switch choice {
		case "overwrite":
			req.OnConflict = model.ResolveOverwrite
		case "cancel":
			return errors.New("agent create: cancelled")
		case "rename":
			var nn string
			if perr := promptInput("New name", &nn); perr != nil {
				return perr
			}
			if nn == "" {
				return errors.New("agent create: empty new name")
			}
			req.Name = nn
			req.OnConflict = ""
		}
	}
}

// pickCapabilities runs Discover for a single kind and lets the user
// pick by display label "kind/name (source)". The result is mapped
// back to CapabilityRef using the underlying Record/Global.
func pickCapabilities(ctx context.Context, deps Deps, kind model.CapabilityKind, title string) ([]model.CapabilityRef, error) {
	if deps.Services.Capabilities == nil {
		return nil, nil
	}
	cands, err := deps.Services.Capabilities.Discover(ctx, model.DiscoverRequest{Kinds: []model.CapabilityKind{kind}})
	if err != nil {
		return nil, err
	}
	if len(cands) == 0 {
		return nil, nil
	}
	labels := make([]string, 0, len(cands))
	idx := map[string]int{}
	for i, c := range cands {
		conflict := ""
		if c.Conflict {
			conflict = " [conflict]"
		}
		label := fmt.Sprintf("%s (%s)%s", c.Name, c.Source, conflict)
		labels = append(labels, label)
		idx[label] = i
	}
	picked, err := promptMultiSelect(title, labels)
	if err != nil {
		return nil, err
	}
	out := make([]model.CapabilityRef, 0, len(picked))
	for _, p := range picked {
		c := cands[idx[p]]
		switch c.Source {
		case model.SourceAVM, model.SourcePackage:
			if c.Record != nil {
				out = append(out, model.CapabilityRef{ID: c.Record.ID, Kind: c.Kind})
			}
		case model.SourceRuntimeGlobal:
			// Runtime-global caps can't yet be referenced directly by ID.
			// Fall back to using the name as a placeholder ID; the
			// service-layer rewrite during package install patches IDs.
			if c.Global != nil {
				out = append(out, model.CapabilityRef{ID: model.CapabilityID(c.Global.Name), Kind: c.Kind})
			}
		}
	}
	return out, nil
}

func registeredRuntimes(deps Deps) []string {
	// We don't have a runtime registry on Deps directly. The only way to
	// surface registered runtimes here without leaking infra is to ask
	// the diagnostics service. Doctor returns per-runtime entries even
	// when unavailable, which is exactly what we need for the picker.
	if deps.Services.Diagnostics == nil {
		return nil
	}
	rep, err := deps.Services.Diagnostics.Doctor(context.Background())
	if err != nil || rep == nil {
		return nil
	}
	out := make([]string, 0, len(rep.Runtimes))
	for _, r := range rep.Runtimes {
		out = append(out, r.Runtime)
	}
	return out
}

func agentExists(deps Deps, name string) (bool, error) {
	if deps.Services.Agents == nil {
		return false, errors.New("no agents service")
	}
	_, err := deps.Services.Agents.Show(context.Background(), name)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, agentstore.ErrNotFound) {
		return false, nil
	}
	// Wrapped/text fallback for older Show paths.
	if strings.Contains(err.Error(), "not found") {
		return false, nil
	}
	return false, nil
}

func buildCreateRequest(f agentCreateFlags, nonInteractive bool) model.CreateAgentRequest {
	prefs := make([]model.RuntimePref, 0, len(f.runtimes))
	for _, r := range f.runtimes {
		prefs = append(prefs, model.RuntimePref{Runtime: r, Default: r == f.defaultRT})
	}
	skills := refsFromIDs(f.skills, model.CapabilityKindSkill)
	mcps := refsFromIDs(f.mcps, model.CapabilityKindMCP)
	src := model.CreateSource(f.source)
	if src == "" {
		src = model.CreateSourceBlank
	}
	resolution := model.ConflictResolution(f.onConflict)
	return model.CreateAgentRequest{
		Name:           f.name,
		Description:    f.description,
		Role:           f.role,
		Instructions:   model.Instructions{System: f.system},
		Skills:         skills,
		MCP:            mcps,
		Runtimes:       prefs,
		Source:         src,
		OnConflict:     resolution,
		NonInteractive: nonInteractive,
	}
}

func refsFromIDs(ids []string, kind model.CapabilityKind) []model.CapabilityRef {
	if len(ids) == 0 {
		return nil
	}
	out := make([]model.CapabilityRef, 0, len(ids))
	for _, id := range ids {
		out = append(out, model.CapabilityRef{ID: model.CapabilityID(id), Kind: kind})
	}
	return out
}

func newAgentListCmd(deps Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List Agents",
		RunE: func(c *cobra.Command, args []string) error {
			g := globalFlags(c)
			summaries, err := deps.Services.Agents.List(c.Context())
			if err != nil {
				return err
			}
			if g.JSON {
				return jsonWrite(c.OutOrStdout(), summaries)
			}
			return RenderAgentList(c.OutOrStdout(), summaries)
		},
	}
}

func newAgentShowCmd(deps Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "show <name>",
		Short: "Show one Agent",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			g := globalFlags(c)
			detail, err := deps.Services.Agents.Show(c.Context(), args[0])
			if err != nil {
				return err
			}
			if g.JSON {
				return jsonWrite(c.OutOrStdout(), detail)
			}
			return RenderAgentDetail(c.OutOrStdout(), detail)
		},
	}
}

func newAgentEditCmd(deps Deps) *cobra.Command {
	var (
		description string
		role        string
		system      string
	)
	cmd := &cobra.Command{
		Use:   "edit <name>",
		Short: "Edit an Agent",
		Long: `Edit an Agent's basic fields.

The non-interactive flag set is intentionally minimal — supported flags are
--description, --role, and --system. Editing skills, MCP, or runtime prefs
non-interactively is not yet supported; use the interactive flow.`,
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			g := globalFlags(c)
			if isInteractive(g) {
				return runAgentEditInteractive(c.Context(), c.OutOrStdout(), deps, args[0])
			}
			req := model.EditAgentRequest{Name: args[0], NonInteractive: true}
			if description != "" || role != "" {
				req.Identity = &model.Identity{Name: args[0], Description: description, Role: role}
			}
			if system != "" {
				req.Instructions = &model.Instructions{System: system}
			}
			a, err := deps.Services.Agents.Edit(c.Context(), req)
			if err != nil {
				return err
			}
			fmt.Fprintf(c.OutOrStdout(), "Edited agent %q\n", a.Identity.Name)
			return nil
		},
	}
	cmd.Flags().StringVar(&description, "description", "", "set description")
	cmd.Flags().StringVar(&role, "role", "", "set role")
	cmd.Flags().StringVar(&system, "system", "", "set system instructions")
	return cmd
}

func runAgentEditInteractive(ctx context.Context, out io.Writer, deps Deps, name string) error {
	detail, err := deps.Services.Agents.Show(ctx, name)
	if err != nil {
		return err
	}
	cur := detail.Agent
	desc := cur.Identity.Description
	role := cur.Identity.Role
	system := cur.Instructions.System
	if err := promptInput(fmt.Sprintf("Description [%s]", desc), &desc); err != nil {
		return err
	}
	if err := promptInput(fmt.Sprintf("Role [%s]", role), &role); err != nil {
		return err
	}
	if err := promptInput(fmt.Sprintf("System instructions [%s]", oneLine(system)), &system); err != nil {
		return err
	}
	ok, err := promptConfirm("Apply changes?")
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("agent edit: cancelled")
	}
	req := model.EditAgentRequest{
		Name:         name,
		Identity:     &model.Identity{Name: name, Description: desc, Role: role},
		Instructions: &model.Instructions{System: system},
	}
	a, err := deps.Services.Agents.Edit(ctx, req)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "Edited agent %q\n", a.Identity.Name)
	return nil
}

func newAgentDeleteCmd(deps Deps) *cobra.Command {
	var confirm bool
	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete an Agent",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			g := globalFlags(c)
			if isInteractive(g) && !confirm {
				detail, err := deps.Services.Agents.Show(c.Context(), args[0])
				if err == nil && detail != nil {
					fmt.Fprintln(c.OutOrStdout(), "About to delete:")
					fmt.Fprintf(c.OutOrStdout(), "  name:     %s\n", detail.Agent.Identity.Name)
					fmt.Fprintf(c.OutOrStdout(), "  runtimes: %d\n", len(detail.Agent.Runtimes))
					fmt.Fprintf(c.OutOrStdout(), "  skills:   %d\n", len(detail.Agent.Skills))
					fmt.Fprintf(c.OutOrStdout(), "  mcp:      %d\n", len(detail.Agent.MCP))
				}
				ok, err := promptConfirm("Delete this Agent?")
				if err != nil {
					return err
				}
				if !ok {
					return errors.New("agent delete: cancelled")
				}
				confirm = true
			}
			err := deps.Services.Agents.Delete(c.Context(), model.DeleteAgentRequest{
				Name:           args[0],
				Confirm:        confirm,
				NonInteractive: !isInteractive(g),
			})
			if err != nil {
				return err
			}
			fmt.Fprintf(c.OutOrStdout(), "Deleted agent %q\n", args[0])
			return nil
		},
	}
	cmd.Flags().BoolVar(&confirm, "yes", false, "confirm deletion (required in non-interactive mode)")
	return cmd
}

func newAgentCloneCmd(deps Deps) *cobra.Command {
	var newName string
	cmd := &cobra.Command{
		Use:   "clone <name>",
		Short: "Clone an Agent",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			a, err := deps.Services.Agents.Clone(c.Context(), args[0], newName)
			if err != nil {
				return err
			}
			fmt.Fprintf(c.OutOrStdout(), "Cloned %q -> %q\n", args[0], a.Identity.Name)
			return nil
		},
	}
	cmd.Flags().StringVar(&newName, "name", "", "new Agent name")
	_ = cmd.MarkFlagRequired("name")
	return cmd
}

func newAgentRenameCmd(deps Deps) *cobra.Command {
	return &cobra.Command{
		Use:   "rename <old> <new>",
		Short: "Rename an Agent",
		Args:  cobra.ExactArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			a, err := deps.Services.Agents.Rename(c.Context(), args[0], args[1])
			if err != nil {
				return err
			}
			fmt.Fprintf(c.OutOrStdout(), "Renamed %q -> %q\n", args[0], a.Identity.Name)
			return nil
		},
	}
}

// jsonWrite is a small helper to render any value as JSON.
func jsonWrite(w io.Writer, v any) error {
	return render.JSON(w, v)
}
