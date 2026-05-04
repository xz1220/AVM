package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"reflect"
	"strconv"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"github.com/xz1220/agent-vm/internal/config"
)

type agentEditOptions struct {
	Description           string
	DisplayName           string
	Role                  string
	Tags                  []string
	Runtime               string
	Runtimes              []string
	RuntimeKind           string
	RuntimeMode           string
	Model                 string
	Reasoning             string
	Verbosity             string
	Temperature           string
	Skills                []string
	MCPs                  []string
	Commands              []string
	Hooks                 []string
	System                string
	Developer             string
	References            []string
	Approval              string
	Sandbox               string
	Allow                 []string
	Deny                  []string
	AdditionalDirectories []string
	Yes                   bool
	NoInput               bool
}

func newAgentCloneCommand() *cobra.Command {
	var fromScope string
	cmd := &cobra.Command{
		Use:   "clone <source>",
		Short: "Clone an AVM agent profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAgentClone(cmd, args, fromScope)
		},
	}
	cmd.Flags().String("name", "", "new agent profile name")
	cmd.Flags().String("scope", string(config.ScopeGlobal), "destination profile scope")
	cmd.Flags().StringVar(&fromScope, "from-scope", "", "source profile scope")
	return cmd
}

func newAgentEditCommand() *cobra.Command {
	var opts agentEditOptions
	cmd := &cobra.Command{
		Use:     "edit <name>",
		Aliases: []string{"update"},
		Short:   "Edit an AVM agent profile",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAgentEdit(cmd, args, opts)
		},
	}
	cmd.Flags().String("scope", string(config.ScopeGlobal), "profile scope")
	cmd.Flags().StringVar(&opts.Description, "description", "", "agent description")
	cmd.Flags().StringVar(&opts.DisplayName, "display-name", "", "agent display name")
	cmd.Flags().StringVar(&opts.Role, "role", "", "agent role")
	cmd.Flags().StringSliceVar(&opts.Tags, "tags", nil, "agent tags")
	cmd.Flags().StringVar(&opts.Runtime, "runtime", "", "preferred runtime")
	cmd.Flags().StringSliceVar(&opts.Runtimes, "runtimes", nil, "runtimes to support, first one is preferred")
	cmd.Flags().StringVar(&opts.RuntimeKind, "runtime-kind", "", "runtime kind")
	cmd.Flags().StringVar(&opts.RuntimeMode, "runtime-mode", "", "runtime mode")
	cmd.Flags().StringVar(&opts.Model, "model", "", "model override")
	cmd.Flags().StringVar(&opts.Reasoning, "reasoning", "", "reasoning effort override")
	cmd.Flags().StringVar(&opts.Verbosity, "verbosity", "", "verbosity override")
	cmd.Flags().StringVar(&opts.Temperature, "temperature", "", "temperature override, or none to clear")
	cmd.Flags().StringSliceVar(&opts.Skills, "skills", nil, "skills to attach")
	cmd.Flags().StringSliceVar(&opts.MCPs, "mcps", nil, "MCP servers to attach")
	cmd.Flags().StringSliceVar(&opts.Commands, "commands", nil, "commands to attach")
	cmd.Flags().StringSliceVar(&opts.Hooks, "hooks", nil, "hooks to attach")
	cmd.Flags().StringVar(&opts.System, "system", "", "system instructions")
	cmd.Flags().StringVar(&opts.Developer, "developer", "", "developer instructions")
	cmd.Flags().StringSliceVar(&opts.References, "references", nil, "instruction references")
	cmd.Flags().StringVar(&opts.Approval, "approval", "", "approval policy")
	cmd.Flags().StringVar(&opts.Sandbox, "sandbox", "", "sandbox policy")
	cmd.Flags().StringSliceVar(&opts.Allow, "allow", nil, "allowed permission patterns")
	cmd.Flags().StringSliceVar(&opts.Deny, "deny", nil, "denied permission patterns")
	cmd.Flags().StringSliceVar(&opts.AdditionalDirectories, "additional-directories", nil, "additional writable directories")
	cmd.Flags().BoolVar(&opts.Yes, "yes", false, "apply flag-provided changes and do not prompt")
	cmd.Flags().BoolVar(&opts.NoInput, "no-input", false, "fail instead of prompting")
	return cmd
}

func newAgentRenameCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rename <old-name> <new-name>",
		Short: "Rename an AVM agent profile",
		Args:  cobra.ExactArgs(2),
		RunE:  runAgentRename,
	}
	cmd.Flags().String("scope", string(config.ScopeGlobal), "profile scope")
	cmd.Flags().Bool("update-refs", false, "update environment references to the new agent name")
	return cmd
}

func newAgentDeleteCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete an AVM agent profile",
		Args:  cobra.ExactArgs(1),
		RunE:  runAgentDelete,
	}
	cmd.Flags().String("scope", string(config.ScopeGlobal), "profile scope")
	cmd.Flags().Bool("force", false, "delete even if non-active environments reference the agent")
	return cmd
}

func runAgentClone(cmd *cobra.Command, args []string, fromScopeValue string) error {
	if err := ensureInitialized(); err != nil {
		return err
	}
	scope, err := scopeFromFlag(cmd)
	if err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	name, err := cmd.Flags().GetString("name")
	if err != nil {
		return err
	}
	if name == "" {
		return fmt.Errorf("agent clone requires --name")
	}
	if exists, err := config.AgentExists(name, scope, cwd); err != nil {
		return err
	} else if exists {
		return fmt.Errorf("agent %q already exists", name)
	}

	source, sourceScope, err := readAgentForClone(args[0], fromScopeValue, cwd)
	if err != nil {
		return err
	}
	agent := cloneAgentProfile(source)
	agent.ID = ""
	agent.Name = name
	agent.SourceScope = string(scope)
	if agent.Identity.DisplayName == "" || agent.Identity.DisplayName == source.Name {
		agent.Identity.DisplayName = name
	}
	if err := config.WriteAgent(agent, scope, cwd); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "cloned agent %s from %s to %s\n", source.Name, sourceScope, agent.Name)
	return nil
}

func readAgentForClone(name, scopeValue, cwd string) (*config.AgentProfile, config.Scope, error) {
	if scopeValue != "" {
		scope, err := parseCreateScope(scopeValue)
		if err != nil {
			return nil, "", err
		}
		agent, err := config.ReadAgent(name, scope, cwd)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, "", fmt.Errorf("profile %q not found in %s scope", name, scope)
			}
			return nil, "", err
		}
		return agent, scope, nil
	}
	source, err := createSourceFromProfile(name, cwd)
	if err != nil {
		return nil, "", err
	}
	return source.Agent, source.Scope, nil
}

func runAgentEdit(cmd *cobra.Command, args []string, opts agentEditOptions) error {
	if err := ensureInitialized(); err != nil {
		return err
	}
	scope, err := scopeFromFlag(cmd)
	if err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	agent, err := readAgentForShow(args[0], scope, cwd)
	if err != nil {
		return err
	}
	before := cloneAgentProfile(agent)

	if agentEditHasFlagChanges(cmd) {
		if err := applyAgentEditFlags(cmd, agent, opts); err != nil {
			return err
		}
		return writeEditedAgent(cmd, before, agent, scope, cwd)
	}
	if opts.NoInput || opts.Yes {
		return fmt.Errorf("agent edit requires field flags or interactive input")
	}

	reader := bufio.NewReader(cmd.InOrStdin())
	if createUseTUI(cmd) {
		if err := promptAgentEditTUI(cmd, agent); err != nil {
			return err
		}
	} else if err := promptAgentEdit(reader, cmd.OutOrStdout(), agent); err != nil {
		return err
	}
	return writeEditedAgent(cmd, before, agent, scope, cwd)
}

func applyAgentEditFlags(cmd *cobra.Command, agent *config.AgentProfile, opts agentEditOptions) error {
	if cmd.Flags().Changed("description") {
		agent.Description = opts.Description
	}
	if cmd.Flags().Changed("display-name") {
		agent.Identity.DisplayName = opts.DisplayName
	}
	if cmd.Flags().Changed("role") {
		agent.Identity.Role = opts.Role
	}
	if cmd.Flags().Changed("tags") {
		agent.Identity.Tags = normalizeStringList(opts.Tags)
	}
	if cmd.Flags().Changed("runtimes") {
		if err := setAgentRuntimes(agent, opts.Runtimes); err != nil {
			return err
		}
	} else if cmd.Flags().Changed("runtime") {
		if opts.Runtime == "" {
			return fmt.Errorf("runtime cannot be empty")
		}
		if !isKnownRuntime(opts.Runtime) {
			return fmt.Errorf("invalid runtime %q", opts.Runtime)
		}
		agent.Runtime.Preferred = opts.Runtime
		agent.Runtime.Fallback = removeStringValue(agent.Runtime.Fallback, opts.Runtime)
	}
	if cmd.Flags().Changed("runtime-kind") {
		agent.Runtime.Kind = opts.RuntimeKind
	}
	if cmd.Flags().Changed("runtime-mode") {
		agent.Runtime.Mode = opts.RuntimeMode
	}
	if cmd.Flags().Changed("model") {
		agent.ModelRun.Model = opts.Model
	}
	if cmd.Flags().Changed("reasoning") {
		agent.ModelRun.ReasoningEffort = opts.Reasoning
	}
	if cmd.Flags().Changed("verbosity") {
		agent.ModelRun.Verbosity = opts.Verbosity
	}
	if cmd.Flags().Changed("temperature") {
		temperature, err := parseAgentTemperature(opts.Temperature)
		if err != nil {
			return err
		}
		agent.ModelRun.Temperature = temperature
	}
	if cmd.Flags().Changed("skills") {
		agent.Capabilities.Skills = normalizeStringList(opts.Skills)
	}
	if cmd.Flags().Changed("mcps") {
		agent.Capabilities.MCPs = normalizeStringList(opts.MCPs)
	}
	if cmd.Flags().Changed("commands") {
		agent.Capabilities.Commands = normalizeStringList(opts.Commands)
	}
	if cmd.Flags().Changed("hooks") {
		agent.Capabilities.Hooks = normalizeStringList(opts.Hooks)
	}
	if cmd.Flags().Changed("system") {
		agent.Instructions.System = opts.System
	}
	if cmd.Flags().Changed("developer") {
		agent.Instructions.Developer = opts.Developer
	}
	if cmd.Flags().Changed("references") {
		agent.Instructions.References = normalizeStringList(opts.References)
	}
	if cmd.Flags().Changed("approval") {
		agent.Permissions.Approval = opts.Approval
	}
	if cmd.Flags().Changed("sandbox") {
		agent.Permissions.Sandbox = opts.Sandbox
	}
	if cmd.Flags().Changed("allow") {
		agent.Permissions.Allow = normalizeStringList(opts.Allow)
	}
	if cmd.Flags().Changed("deny") {
		agent.Permissions.Deny = normalizeStringList(opts.Deny)
	}
	if cmd.Flags().Changed("additional-directories") {
		agent.Permissions.AdditionalDirectories = normalizeStringList(opts.AdditionalDirectories)
	}
	return nil
}

func promptAgentEdit(reader *bufio.Reader, out io.Writer, agent *config.AgentProfile) error {
	groups, err := promptAgentEditGroups(reader, out)
	if err != nil {
		return err
	}
	before := cloneAgentProfile(agent)
	for _, group := range groups {
		if err := promptAgentEditGroup(reader, out, agent, group); err != nil {
			return err
		}
	}
	printAgentEditPreview(out, before, agent)
	confirmed, err := promptConfirm(reader, out, fmt.Sprintf("Apply changes to agent %q", agent.Name), true)
	if err != nil {
		return err
	}
	if !confirmed {
		return fmt.Errorf("agent edit cancelled")
	}
	return nil
}

func promptAgentEditTUI(cmd *cobra.Command, agent *config.AgentProfile) error {
	selected := []string{"basic", "runtime", "model", "capabilities"}
	options := make([]huh.Option[string], 0, len(agentEditGroups))
	for _, group := range agentEditGroups {
		options = append(options, huh.NewOption(group.Label, group.Key).Selected(containsCreateString(selected, group.Key)))
	}
	form := huh.NewForm(huh.NewGroup(
		huh.NewMultiSelect[string]().
			Title("Fields to edit").
			Options(options...).
			Value(&selected),
	))
	if err := runTUIForm(cmd, form, "agent edit"); err != nil {
		return err
	}
	selected = normalizeStringList(selected)
	if len(selected) == 0 {
		return fmt.Errorf("agent edit requires at least one field group")
	}

	before := cloneAgentProfile(agent)
	for _, group := range selected {
		if err := promptAgentEditGroupTUI(cmd, agent, group); err != nil {
			return err
		}
	}
	printAgentEditPreview(cmd.OutOrStdout(), before, agent)
	confirmed := true
	form = huh.NewForm(huh.NewGroup(
		huh.NewConfirm().
			Title(fmt.Sprintf("Apply changes to agent %q?", agent.Name)).
			Affirmative("Apply").
			Negative("Cancel").
			Value(&confirmed),
	))
	if err := runTUIForm(cmd, form, "agent edit"); err != nil {
		return err
	}
	if !confirmed {
		return fmt.Errorf("agent edit cancelled")
	}
	return nil
}

func promptAgentEditGroup(reader *bufio.Reader, out io.Writer, agent *config.AgentProfile, group string) error {
	var err error
	switch group {
	case "basic":
		agent.Description, err = promptString(reader, out, "Description", agent.Description)
		if err != nil {
			return err
		}
		agent.Identity.DisplayName, err = promptString(reader, out, "Display name", agent.Identity.DisplayName)
		if err != nil {
			return err
		}
		agent.Identity.Role, err = promptString(reader, out, "Role", agent.Identity.Role)
		if err != nil {
			return err
		}
		agent.Identity.Tags, err = promptStringList(reader, out, "Tags", agent.Identity.Tags)
		return err
	case "runtime":
		runtimes, err := promptString(reader, out, "Runtimes (comma separated: codex, claude-code, opencode, cline, cursor)", strings.Join(runtimePreferenceList(agent.Runtime), ","))
		if err != nil {
			return err
		}
		if err := setAgentRuntimes(agent, splitSelectionValues(runtimes)); err != nil {
			return err
		}
		agent.Runtime.Kind, err = promptString(reader, out, "Runtime kind", agent.Runtime.Kind)
		if err != nil {
			return err
		}
		agent.Runtime.Mode, err = promptString(reader, out, "Runtime mode", agent.Runtime.Mode)
		return err
	case "model":
		agent.ModelRun.Model, err = promptString(reader, out, "Model", agent.ModelRun.Model)
		if err != nil {
			return err
		}
		agent.ModelRun.ReasoningEffort, err = promptString(reader, out, "Reasoning", agent.ModelRun.ReasoningEffort)
		if err != nil {
			return err
		}
		agent.ModelRun.Verbosity, err = promptString(reader, out, "Verbosity", agent.ModelRun.Verbosity)
		if err != nil {
			return err
		}
		temperature, err := promptString(reader, out, "Temperature (none to clear)", formatAgentTemperature(agent.ModelRun.Temperature))
		if err != nil {
			return err
		}
		agent.ModelRun.Temperature, err = parseAgentTemperature(temperature)
		return err
	case "capabilities":
		agent.Capabilities.Skills, err = promptCapabilitySelection(reader, out, "Skills", "skills", listInstalledSkillOptions, agent.Capabilities.Skills)
		if err != nil {
			return err
		}
		agent.Capabilities.MCPs, err = promptCapabilitySelection(reader, out, "MCP servers", "mcps", listInstalledMCPOptions, agent.Capabilities.MCPs)
		if err != nil {
			return err
		}
		agent.Capabilities.Commands, err = promptStringList(reader, out, "Commands", agent.Capabilities.Commands)
		if err != nil {
			return err
		}
		agent.Capabilities.Hooks, err = promptStringList(reader, out, "Hooks", agent.Capabilities.Hooks)
		return err
	case "instructions":
		agent.Instructions.System, err = promptString(reader, out, "System instructions", agent.Instructions.System)
		if err != nil {
			return err
		}
		agent.Instructions.Developer, err = promptString(reader, out, "Developer instructions", agent.Instructions.Developer)
		if err != nil {
			return err
		}
		agent.Instructions.References, err = promptStringList(reader, out, "Instruction references", agent.Instructions.References)
		return err
	case "permissions":
		agent.Permissions.Approval, err = promptString(reader, out, "Approval", agent.Permissions.Approval)
		if err != nil {
			return err
		}
		agent.Permissions.Sandbox, err = promptString(reader, out, "Sandbox", agent.Permissions.Sandbox)
		if err != nil {
			return err
		}
		agent.Permissions.Allow, err = promptStringList(reader, out, "Allow", agent.Permissions.Allow)
		if err != nil {
			return err
		}
		agent.Permissions.Deny, err = promptStringList(reader, out, "Deny", agent.Permissions.Deny)
		if err != nil {
			return err
		}
		agent.Permissions.AdditionalDirectories, err = promptStringList(reader, out, "Additional directories", agent.Permissions.AdditionalDirectories)
		return err
	default:
		return fmt.Errorf("invalid edit group %q", group)
	}
}

func promptAgentEditGroupTUI(cmd *cobra.Command, agent *config.AgentProfile, group string) error {
	switch group {
	case "basic":
		tags := strings.Join(agent.Identity.Tags, ",")
		form := huh.NewForm(huh.NewGroup(
			huh.NewInput().Title("Description").Value(&agent.Description),
			huh.NewInput().Title("Display name").Value(&agent.Identity.DisplayName),
			huh.NewInput().Title("Role").Value(&agent.Identity.Role),
			huh.NewInput().Title("Tags").Description("Comma separated").Value(&tags),
		))
		if err := runTUIForm(cmd, form, "agent edit"); err != nil {
			return err
		}
		agent.Identity.Tags = splitSelectionValues(tags)
		return nil
	case "runtime":
		runtimes := runtimePreferenceList(agent.Runtime)
		form := huh.NewForm(huh.NewGroup(
			huh.NewMultiSelect[string]().Title("Runtimes").Options(runtimeTUIOptions(runtimes)...).Value(&runtimes),
			huh.NewInput().Title("Runtime kind").Value(&agent.Runtime.Kind),
			huh.NewInput().Title("Runtime mode").Value(&agent.Runtime.Mode),
		))
		if err := runTUIForm(cmd, form, "agent edit"); err != nil {
			return err
		}
		return setAgentRuntimes(agent, runtimes)
	case "model":
		temperature := formatAgentTemperature(agent.ModelRun.Temperature)
		form := huh.NewForm(huh.NewGroup(
			huh.NewInput().Title("Model").Value(&agent.ModelRun.Model),
			huh.NewInput().Title("Reasoning").Value(&agent.ModelRun.ReasoningEffort),
			huh.NewInput().Title("Verbosity").Value(&agent.ModelRun.Verbosity),
			huh.NewInput().Title("Temperature").Description("Use none to clear").Value(&temperature),
		))
		if err := runTUIForm(cmd, form, "agent edit"); err != nil {
			return err
		}
		parsed, err := parseAgentTemperature(temperature)
		if err != nil {
			return err
		}
		agent.ModelRun.Temperature = parsed
		return nil
	case "capabilities":
		skills := append([]string(nil), agent.Capabilities.Skills...)
		mcps := append([]string(nil), agent.Capabilities.MCPs...)
		commands := strings.Join(agent.Capabilities.Commands, ",")
		hooks := strings.Join(agent.Capabilities.Hooks, ",")
		fields := []huh.Field{}
		if options, err := listInstalledSkillOptions(); err != nil {
			return err
		} else if tuiOptions := capabilityTUIOptions(options, skills); len(tuiOptions) > 0 {
			fields = append(fields, huh.NewMultiSelect[string]().Title("Skills").Options(tuiOptions...).Value(&skills))
		}
		if options, err := listInstalledMCPOptions(); err != nil {
			return err
		} else if tuiOptions := capabilityTUIOptions(options, mcps); len(tuiOptions) > 0 {
			fields = append(fields, huh.NewMultiSelect[string]().Title("MCP servers").Options(tuiOptions...).Value(&mcps))
		}
		fields = append(fields,
			huh.NewInput().Title("Commands").Description("Comma separated").Value(&commands),
			huh.NewInput().Title("Hooks").Description("Comma separated").Value(&hooks),
		)
		if err := runTUIForm(cmd, huh.NewForm(huh.NewGroup(fields...)), "agent edit"); err != nil {
			return err
		}
		agent.Capabilities.Skills = uniqueSortedCreateStrings(skills)
		agent.Capabilities.MCPs = uniqueSortedCreateStrings(mcps)
		agent.Capabilities.Commands = splitSelectionValues(commands)
		agent.Capabilities.Hooks = splitSelectionValues(hooks)
		return nil
	case "instructions":
		references := strings.Join(agent.Instructions.References, ",")
		form := huh.NewForm(huh.NewGroup(
			huh.NewInput().Title("System instructions").Value(&agent.Instructions.System),
			huh.NewInput().Title("Developer instructions").Value(&agent.Instructions.Developer),
			huh.NewInput().Title("Instruction references").Description("Comma separated").Value(&references),
		))
		if err := runTUIForm(cmd, form, "agent edit"); err != nil {
			return err
		}
		agent.Instructions.References = splitSelectionValues(references)
		return nil
	case "permissions":
		allow := strings.Join(agent.Permissions.Allow, ",")
		deny := strings.Join(agent.Permissions.Deny, ",")
		dirs := strings.Join(agent.Permissions.AdditionalDirectories, ",")
		form := huh.NewForm(huh.NewGroup(
			huh.NewInput().Title("Approval").Value(&agent.Permissions.Approval),
			huh.NewInput().Title("Sandbox").Value(&agent.Permissions.Sandbox),
			huh.NewInput().Title("Allow").Description("Comma separated").Value(&allow),
			huh.NewInput().Title("Deny").Description("Comma separated").Value(&deny),
			huh.NewInput().Title("Additional directories").Description("Comma separated").Value(&dirs),
		))
		if err := runTUIForm(cmd, form, "agent edit"); err != nil {
			return err
		}
		agent.Permissions.Allow = splitSelectionValues(allow)
		agent.Permissions.Deny = splitSelectionValues(deny)
		agent.Permissions.AdditionalDirectories = splitSelectionValues(dirs)
		return nil
	default:
		return fmt.Errorf("invalid edit group %q", group)
	}
}

func writeEditedAgent(cmd *cobra.Command, before, agent *config.AgentProfile, scope config.Scope, cwd string) error {
	if err := config.WriteAgent(agent, scope, cwd); err != nil {
		return err
	}
	diffs := agentChangeSummary(before, agent)
	out := cmd.OutOrStdout()
	if len(diffs) == 0 {
		fmt.Fprintf(out, "agent %s unchanged\n", agent.Name)
		return nil
	}
	fmt.Fprintf(out, "updated agent %s\n", agent.Name)
	for _, diff := range diffs {
		fmt.Fprintf(out, "  %s\n", diff)
	}
	return nil
}

func runAgentRename(cmd *cobra.Command, args []string) error {
	if err := ensureInitialized(); err != nil {
		return err
	}
	scope, err := scopeFromFlag(cmd)
	if err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	updateRefs, err := cmd.Flags().GetBool("update-refs")
	if err != nil {
		return err
	}

	oldName := args[0]
	newName := args[1]
	if _, err := readAgentForShow(oldName, scope, cwd); err != nil {
		return err
	}
	if exists, err := config.AgentExists(newName, scope, cwd); err != nil {
		return err
	} else if exists {
		return fmt.Errorf("agent %q already exists", newName)
	}

	refs, err := config.FindAgentReferences(oldName, cwd)
	if err != nil {
		return err
	}
	if activeAgentReferenced(refs) {
		return fmt.Errorf("agent %q is active; activate another profile or deactivate before renaming", oldName)
	}
	if len(refs) > 0 && !updateRefs {
		return fmt.Errorf("agent %q is referenced; use --update-refs to rename references:\n%s", oldName, formatAgentReferences(refs))
	}

	if err := config.RenameAgent(oldName, newName, scope, cwd); err != nil {
		return err
	}
	if updateRefs {
		if _, err := config.UpdateAgentReferences(oldName, newName, cwd); err != nil {
			return err
		}
	}
	fmt.Fprintf(cmd.OutOrStdout(), "renamed agent %s to %s\n", oldName, newName)
	if updateRefs && len(refs) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "updated %d reference(s)\n", len(refs))
	}
	return nil
}

func runAgentDelete(cmd *cobra.Command, args []string) error {
	if err := ensureInitialized(); err != nil {
		return err
	}
	scope, err := scopeFromFlag(cmd)
	if err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	force, err := cmd.Flags().GetBool("force")
	if err != nil {
		return err
	}

	name := args[0]
	if _, err := readAgentForShow(name, scope, cwd); err != nil {
		return err
	}
	refs, err := config.FindAgentReferences(name, cwd)
	if err != nil {
		return err
	}
	if activeAgentReferenced(refs) {
		return fmt.Errorf("agent %q is active; activate another profile or deactivate before deleting", name)
	}
	if len(refs) > 0 && !force {
		return fmt.Errorf("agent %q is referenced; use --force to delete anyway:\n%s", name, formatAgentReferences(refs))
	}
	if err := config.DeleteAgent(name, scope, cwd); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("profile %q not found", name)
		}
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "deleted agent %s\n", name)
	if force && len(refs) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "left %d reference(s) unchanged\n", len(refs))
	}
	return nil
}

type agentEditGroup struct {
	Key   string
	Label string
}

var agentEditGroups = []agentEditGroup{
	{Key: "basic", Label: "Basic"},
	{Key: "runtime", Label: "Runtime"},
	{Key: "model", Label: "Model"},
	{Key: "capabilities", Label: "Capabilities"},
	{Key: "instructions", Label: "Instructions"},
	{Key: "permissions", Label: "Permissions"},
}

func promptAgentEditGroups(reader *bufio.Reader, out io.Writer) ([]string, error) {
	fmt.Fprintln(out, "Fields to edit:")
	for i, group := range agentEditGroups {
		fmt.Fprintf(out, "  %d) %s\n", i+1, group.Label)
	}
	value, err := promptString(reader, out, "Selection (numbers/names, comma separated; all)", "basic,runtime,model,capabilities")
	if err != nil {
		return nil, err
	}
	return parseAgentEditGroupSelection(value)
}

func parseAgentEditGroupSelection(value string) ([]string, error) {
	value = strings.TrimSpace(value)
	if strings.EqualFold(value, "all") {
		out := make([]string, 0, len(agentEditGroups))
		for _, group := range agentEditGroups {
			out = append(out, group.Key)
		}
		return out, nil
	}

	byName := make(map[string]string, len(agentEditGroups))
	for i, group := range agentEditGroups {
		byName[group.Key] = group.Key
		byName[strings.ToLower(group.Label)] = group.Key
		byName[strconv.Itoa(i+1)] = group.Key
	}

	seen := map[string]struct{}{}
	var out []string
	for _, token := range splitSelectionValues(value) {
		key, ok := byName[strings.ToLower(token)]
		if !ok {
			return nil, fmt.Errorf("invalid edit group %q", token)
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("agent edit requires at least one field group")
	}
	return out, nil
}

func agentEditHasFlagChanges(cmd *cobra.Command) bool {
	for _, name := range []string{
		"description", "display-name", "role", "tags",
		"runtime", "runtimes", "runtime-kind", "runtime-mode",
		"model", "reasoning", "verbosity", "temperature",
		"skills", "mcps", "commands", "hooks",
		"system", "developer", "references",
		"approval", "sandbox", "allow", "deny", "additional-directories",
	} {
		if cmd.Flags().Changed(name) {
			return true
		}
	}
	return false
}

func setAgentRuntimes(agent *config.AgentProfile, values []string) error {
	runtimes := normalizeCreateRuntimeList(values)
	if len(runtimes) == 0 {
		return fmt.Errorf("at least one runtime is required")
	}
	for _, runtime := range runtimes {
		if !isKnownRuntime(runtime) {
			return fmt.Errorf("invalid runtime %q", runtime)
		}
	}
	agent.Runtime.Preferred = runtimes[0]
	if len(runtimes) > 1 {
		agent.Runtime.Fallback = append([]string(nil), runtimes[1:]...)
	} else {
		agent.Runtime.Fallback = nil
	}
	return nil
}

func removeStringValue(values []string, value string) []string {
	var out []string
	for _, current := range values {
		if current != value {
			out = append(out, current)
		}
	}
	return out
}

func parseAgentTemperature(value string) (*float64, error) {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "none") {
		return nil, nil
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid temperature %q", value)
	}
	return &parsed, nil
}

func formatAgentTemperature(value *float64) string {
	if value == nil {
		return ""
	}
	return strconv.FormatFloat(*value, 'f', -1, 64)
}

func printAgentEditPreview(out io.Writer, before, after *config.AgentProfile) {
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Changes:")
	diffs := agentChangeSummary(before, after)
	if len(diffs) == 0 {
		fmt.Fprintln(out, "  none")
		return
	}
	for _, diff := range diffs {
		fmt.Fprintf(out, "  %s\n", diff)
	}
}

func agentChangeSummary(before, after *config.AgentProfile) []string {
	var diffs []string
	addDiff := func(field, oldValue, newValue string) {
		if oldValue != newValue {
			diffs = append(diffs, fmt.Sprintf("%s: %s -> %s", field, previewScalar(oldValue), previewScalar(newValue)))
		}
	}
	addListDiff := func(field string, oldValue, newValue []string) {
		oldValue = normalizeStringList(oldValue)
		newValue = normalizeStringList(newValue)
		if !reflect.DeepEqual(oldValue, newValue) {
			diffs = append(diffs, fmt.Sprintf("%s: %s -> %s", field, previewList(oldValue), previewList(newValue)))
		}
	}

	addDiff("description", before.Description, after.Description)
	addDiff("identity.display_name", before.Identity.DisplayName, after.Identity.DisplayName)
	addDiff("identity.role", before.Identity.Role, after.Identity.Role)
	addListDiff("identity.tags", before.Identity.Tags, after.Identity.Tags)
	addListDiff("runtime.runtimes", runtimePreferenceList(before.Runtime), runtimePreferenceList(after.Runtime))
	addDiff("runtime.kind", before.Runtime.Kind, after.Runtime.Kind)
	addDiff("runtime.mode", before.Runtime.Mode, after.Runtime.Mode)
	addDiff("model_run.model", before.ModelRun.Model, after.ModelRun.Model)
	addDiff("model_run.reasoning_effort", before.ModelRun.ReasoningEffort, after.ModelRun.ReasoningEffort)
	addDiff("model_run.verbosity", before.ModelRun.Verbosity, after.ModelRun.Verbosity)
	addDiff("model_run.temperature", formatAgentTemperature(before.ModelRun.Temperature), formatAgentTemperature(after.ModelRun.Temperature))
	addListDiff("capabilities.skills", before.Capabilities.Skills, after.Capabilities.Skills)
	addListDiff("capabilities.mcps", before.Capabilities.MCPs, after.Capabilities.MCPs)
	addListDiff("capabilities.commands", before.Capabilities.Commands, after.Capabilities.Commands)
	addListDiff("capabilities.hooks", before.Capabilities.Hooks, after.Capabilities.Hooks)
	addDiff("instructions.system", before.Instructions.System, after.Instructions.System)
	addDiff("instructions.developer", before.Instructions.Developer, after.Instructions.Developer)
	addListDiff("instructions.references", before.Instructions.References, after.Instructions.References)
	addDiff("permissions.approval", before.Permissions.Approval, after.Permissions.Approval)
	addDiff("permissions.sandbox", before.Permissions.Sandbox, after.Permissions.Sandbox)
	addListDiff("permissions.allow", before.Permissions.Allow, after.Permissions.Allow)
	addListDiff("permissions.deny", before.Permissions.Deny, after.Permissions.Deny)
	addListDiff("permissions.additional_directories", before.Permissions.AdditionalDirectories, after.Permissions.AdditionalDirectories)
	return diffs
}

func previewScalar(value string) string {
	if strings.TrimSpace(value) == "" {
		return "none"
	}
	return value
}

func activeAgentReferenced(refs []config.AgentReference) bool {
	for _, ref := range refs {
		if ref.Kind == "active" {
			return true
		}
	}
	return false
}

func formatAgentReferences(refs []config.AgentReference) string {
	lines := make([]string, 0, len(refs))
	for _, ref := range refs {
		lines = append(lines, "  "+formatAgentReference(ref))
	}
	return strings.Join(lines, "\n")
}

func formatAgentReference(ref config.AgentReference) string {
	switch ref.Kind {
	case "active":
		return fmt.Sprintf("%s: %s", ref.Path, ref.Field)
	case "env":
		return fmt.Sprintf("%s env %s: %s", ref.Path, ref.Name, ref.Field)
	case "project_override":
		return fmt.Sprintf("%s project override %s: %s", ref.Path, ref.Name, ref.Field)
	default:
		return fmt.Sprintf("%s %s %s: %s", ref.Path, ref.Kind, ref.Name, ref.Field)
	}
}
