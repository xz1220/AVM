package cli

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/xz1220/agent-vm/internal/app/model"
	"github.com/xz1220/agent-vm/internal/app/service"
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

// ----------------------------------------------------------------------------
// avm agent create
// ----------------------------------------------------------------------------

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
		Long: `Create a new Agent. All inputs come from flags; the CLI never prompts.

Required:  --name and at least one --runtime
Conflicts: pass --on-conflict {overwrite|cancel|rename|skip} — default behavior
           is to fail with AGENT_CONFLICT so the caller decides explicitly.`,
		RunE: func(c *cobra.Command, args []string) error {
			req := buildCreateRequest(f)
			a, err := deps.Services.Agents.Create(c.Context(), req)
			if err != nil {
				return err
			}
			g := globalFlags(c)
			if g.JSON {
				return jsonWrite(c.OutOrStdout(), a)
			}
			fmt.Fprintf(c.OutOrStdout(), "Created agent %q\n", a.Identity.Name)
			return nil
		},
	}
	cmd.Flags().StringVar(&f.name, "name", "", "Agent name (required)")
	cmd.Flags().StringVar(&f.description, "description", "", "human-readable description")
	cmd.Flags().StringVar(&f.role, "role", "", "Agent role tag")
	cmd.Flags().StringVar(&f.system, "system", "", "system instructions")
	cmd.Flags().StringSliceVar(&f.skills, "skill", nil, "capability ID for a skill (repeatable)")
	cmd.Flags().StringSliceVar(&f.mcps, "mcp", nil, "capability ID for an MCP server (repeatable)")
	cmd.Flags().StringSliceVar(&f.runtimes, "runtime", nil, "runtime preference (repeatable; required)")
	cmd.Flags().StringVar(&f.defaultRT, "default-runtime", "", "default runtime when multiple are configured")
	cmd.Flags().StringVar(&f.onConflict, "on-conflict", "", "ask|skip|overwrite|cancel|rename (default ask: fail on conflict)")
	cmd.Flags().StringVar(&f.source, "source", "blank", "blank|default")
	return cmd
}

func buildCreateRequest(f agentCreateFlags) model.CreateAgentRequest {
	prefs := make([]model.RuntimePref, 0, len(f.runtimes))
	for _, r := range f.runtimes {
		prefs = append(prefs, model.RuntimePref{Runtime: r, Default: r == f.defaultRT})
	}
	src := model.CreateSource(f.source)
	if src == "" {
		src = model.CreateSourceBlank
	}
	return model.CreateAgentRequest{
		Name:         f.name,
		Description:  f.description,
		Role:         f.role,
		Instructions: model.Instructions{System: f.system},
		Skills:       refsFromIDs(f.skills, model.CapabilityKindSkill),
		MCP:          refsFromIDs(f.mcps, model.CapabilityKindMCP),
		Runtimes:     prefs,
		Source:       src,
		OnConflict:   model.ConflictResolution(f.onConflict),
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

// ----------------------------------------------------------------------------
// avm agent list / show
// ----------------------------------------------------------------------------

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

// ----------------------------------------------------------------------------
// avm agent edit
//
// Single non-interactive path. Each list flag (--skill, --mcp, --runtime)
// REPLACES the existing list when any value is passed; absent flags keep
// the existing list. Use `avm agent show --json` to read current state,
// then re-issue with the full new lists.
// ----------------------------------------------------------------------------

type agentEditFlags struct {
	description    string
	role           string
	system         string
	skills         []string
	mcps           []string
	runtimes       []string
	defaultRT      string
	skillsSet      bool
	mcpsSet        bool
	runtimesSet    bool
	descriptionSet bool
	roleSet        bool
	systemSet      bool
}

func newAgentEditCmd(deps Deps) *cobra.Command {
	f := agentEditFlags{}
	cmd := &cobra.Command{
		Use:   "edit <name>",
		Short: "Edit an Agent",
		Long: `Edit an Agent's fields. All input comes from flags.

  --description / --role / --system  replace those scalar fields
  --skill <id>     repeatable; replaces the entire skills list
  --mcp <id>       repeatable; replaces the entire MCP list
  --runtime <name> repeatable; replaces the runtime preference list
  --default-runtime <name>  applied to the new --runtime list

Tip: read current state with  avm agent show <name> --json  before editing.`,
		Args: cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			// Detect which flags were actually passed so absent fields
			// stay nil (keep-existing semantics).
			cmd := c
			f.descriptionSet = cmd.Flags().Changed("description")
			f.roleSet = cmd.Flags().Changed("role")
			f.systemSet = cmd.Flags().Changed("system")
			f.skillsSet = cmd.Flags().Changed("skill")
			f.mcpsSet = cmd.Flags().Changed("mcp")
			f.runtimesSet = cmd.Flags().Changed("runtime") || cmd.Flags().Changed("default-runtime")

			req := buildEditRequest(args[0], f)
			a, err := deps.Services.Agents.Edit(c.Context(), req)
			if err != nil {
				return err
			}
			g := globalFlags(c)
			if g.JSON {
				return jsonWrite(c.OutOrStdout(), a)
			}
			fmt.Fprintf(c.OutOrStdout(), "Edited agent %q\n", a.Identity.Name)
			return nil
		},
	}
	cmd.Flags().StringVar(&f.description, "description", "", "set description")
	cmd.Flags().StringVar(&f.role, "role", "", "set role")
	cmd.Flags().StringVar(&f.system, "system", "", "set system instructions")
	cmd.Flags().StringSliceVar(&f.skills, "skill", nil, "replace skills list (capability IDs)")
	cmd.Flags().StringSliceVar(&f.mcps, "mcp", nil, "replace MCP list (capability IDs)")
	cmd.Flags().StringSliceVar(&f.runtimes, "runtime", nil, "replace runtime preferences")
	cmd.Flags().StringVar(&f.defaultRT, "default-runtime", "", "default runtime within --runtime list")
	return cmd
}

func buildEditRequest(name string, f agentEditFlags) model.EditAgentRequest {
	req := model.EditAgentRequest{Name: name}

	// Identity is replaced if any of description/role were touched.
	// We intentionally do not let edit rename — Rename owns that.
	if f.descriptionSet || f.roleSet {
		req.Identity = &model.Identity{
			Name:        name,
			Description: f.description,
			Role:        f.role,
		}
	}
	if f.systemSet {
		req.Instructions = &model.Instructions{System: f.system}
	}
	if f.skillsSet {
		s := refsFromIDs(f.skills, model.CapabilityKindSkill)
		req.Skills = &s
	}
	if f.mcpsSet {
		m := refsFromIDs(f.mcps, model.CapabilityKindMCP)
		req.MCP = &m
	}
	if f.runtimesSet {
		prefs := make([]model.RuntimePref, 0, len(f.runtimes))
		for _, r := range f.runtimes {
			prefs = append(prefs, model.RuntimePref{Runtime: r, Default: r == f.defaultRT})
		}
		req.Runtimes = &prefs
	}
	return req
}

// ----------------------------------------------------------------------------
// avm agent delete / clone / rename
// ----------------------------------------------------------------------------

func newAgentDeleteCmd(deps Deps) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete an Agent",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			err := deps.Services.Agents.Delete(c.Context(), model.DeleteAgentRequest{
				Name:    args[0],
				Confirm: yes,
			})
			if err != nil {
				return err
			}
			if globalFlags(c).JSON {
				// Protocol contract: agent delete success -> JSON `null`.
				return jsonWrite(c.OutOrStdout(), nil)
			}
			fmt.Fprintf(c.OutOrStdout(), "Deleted agent %q\n", args[0])
			return nil
		},
	}
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm deletion (required)")
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

// ----------------------------------------------------------------------------
// helpers (kept here so the only "shared" surface is jsonWrite)
// ----------------------------------------------------------------------------

// jsonWrite is a small helper to render any value as JSON.
func jsonWrite(w io.Writer, v any) error {
	return render.JSON(w, v)
}

// _ keeps service imported even if no other site references it directly
// in this file; the JSON error renderer in root.go uses it.
var _ = service.AsError
