package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"github.com/xz1220/agent-vm/internal/config"
)

type createOptions struct {
	From      string
	Name      string
	Runtime   string
	Runtimes  []string
	Model     string
	Reasoning string
	Skills    []string
	MCPs      []string
	Scope     config.Scope
	Yes       bool
	NoInput   bool
}

func newCreateCommand() *cobra.Command {
	var opts createOptions
	var scope string

	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an AVM agent profile",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			parsedScope, err := parseCreateScope(scope)
			if err != nil {
				return err
			}
			opts.Scope = parsedScope
			return runCreate(cmd, opts)
		},
	}
	cmd.Flags().StringVar(&opts.Name, "name", "", "agent profile name to create")
	cmd.Flags().StringVar(&opts.From, "from", "", "copy an existing AVM agent profile")
	cmd.Flags().StringVar(&opts.Runtime, "runtime", "", "preferred runtime for the created profile")
	cmd.Flags().StringSliceVar(&opts.Runtimes, "runtimes", nil, "runtimes to support, first one is preferred")
	cmd.Flags().StringVar(&opts.Model, "model", "", "model override")
	cmd.Flags().StringVar(&opts.Reasoning, "reasoning", "", "reasoning effort override")
	cmd.Flags().StringSliceVar(&opts.Skills, "skills", nil, "skills to attach")
	cmd.Flags().StringSliceVar(&opts.MCPs, "mcps", nil, "MCP servers to attach")
	cmd.Flags().StringVar(&scope, "scope", string(config.ScopeGlobal), "profile scope")
	cmd.Flags().BoolVar(&opts.Yes, "yes", false, "accept defaults and do not prompt")
	cmd.Flags().BoolVar(&opts.NoInput, "no-input", false, "fail instead of prompting")
	return cmd
}

func runCreate(cmd *cobra.Command, opts createOptions) error {
	if err := ensureInitialized(); err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	reader := bufio.NewReader(cmd.InOrStdin())
	out := cmd.OutOrStdout()
	useTUI := createUseTUI(cmd) && !opts.Yes && !opts.NoInput
	source, err := resolveCreateSource(cmd, reader, out, cwd, opts, useTUI)
	if err != nil {
		return err
	}

	values, err := resolveCreateValues(cmd, reader, out, cwd, source, opts, useTUI)
	if err != nil {
		return err
	}
	if _, err := config.ReadAgent(values.Name, values.Scope, cwd); err == nil {
		return fmt.Errorf("agent %q already exists; choose another --name", values.Name)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}

	agent := agentFromCreateSource(source, values)
	if err := config.WriteAgent(agent, values.Scope, cwd); err != nil {
		return err
	}

	fmt.Fprintf(out, "created agent %s from %s\n", agent.Name, source.Summary())
	fmt.Fprintln(out)
	fmt.Fprintln(out, "To use it in this shell with shell integration:")
	fmt.Fprintf(out, "  avm use %s\n", agent.Name)
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Without shell integration:")
	fmt.Fprintf(out, "  eval \"$(avm activate %s)\"\n", agent.Name)
	runtimeCommands := runtimeStartCommands(runtimePreferenceList(agent.Runtime))
	if len(runtimeCommands) > 0 {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Then start a runtime:")
		for _, runtimeCommand := range runtimeCommands {
			fmt.Fprintf(out, "  %s\n", runtimeCommand)
		}
	}
	return nil
}

type createSourceKind string

const (
	createSourceProfile createSourceKind = "profile"
)

type createSource struct {
	Kind        createSourceKind
	Name        string
	Description string
	Scope       config.Scope
	Runtime     string
	Agent       *config.AgentProfile
}

func (s createSource) Summary() string {
	if s.Scope != "" {
		return fmt.Sprintf("%s profile %s", s.Scope, s.Name)
	}
	return "profile " + s.Name
}

func (s createSource) PromptLabel() string {
	label := s.Summary()
	if s.Description != "" {
		label += " - " + s.Description
	}
	return label
}

type resolvedCreateValues struct {
	Name      string
	Runtime   string
	Runtimes  []string
	Model     string
	Reasoning string
	Skills    []string
	MCPs      []string
	Scope     config.Scope
}

func resolveCreateSource(cmd *cobra.Command, reader *bufio.Reader, out io.Writer, cwd string, opts createOptions, useTUI bool) (createSource, error) {
	if opts.From != "" {
		return createSourceFromProfile(opts.From, cwd)
	}
	if opts.Yes {
		return createSourceFromProfile("default", cwd)
	}
	if opts.NoInput {
		return createSource{}, fmt.Errorf("create requires --from, --yes, or interactive input")
	}
	if useTUI {
		return promptCreateSourceTUI(cmd, cwd)
	}
	return promptCreateSource(reader, out, cwd)
}

func createSourceFromProfile(name, cwd string) (createSource, error) {
	for _, scope := range []config.Scope{config.ScopeProject, config.ScopeGlobal} {
		agent, err := config.ReadAgent(name, scope, cwd)
		if err == nil {
			return createSource{
				Kind:        createSourceProfile,
				Name:        agent.Name,
				Description: agent.Description,
				Scope:       scope,
				Runtime:     agent.Runtime.Preferred,
				Agent:       agent,
			}, nil
		}
		if err != nil && !os.IsNotExist(err) {
			return createSource{}, err
		}
	}
	return createSource{}, fmt.Errorf("profile %q not found in project or global agents", name)
}

func promptCreateSource(reader *bufio.Reader, out io.Writer, cwd string) (createSource, error) {
	sources, err := listCreateSources(cwd)
	if err != nil {
		return createSource{}, err
	}
	if len(sources) == 0 {
		return createSource{}, fmt.Errorf("no create sources found")
	}

	fmt.Fprintln(out, "Create from:")
	for i, source := range sources {
		fmt.Fprintf(out, "  %d) %s\n", i+1, source.PromptLabel())
	}
	index, err := promptSelectIndex(reader, out, "Source", 1, len(sources), 1)
	if err != nil {
		return createSource{}, err
	}
	return sources[index-1], nil
}

func promptCreateSourceTUI(cmd *cobra.Command, cwd string) (createSource, error) {
	sources, err := listCreateSources(cwd)
	if err != nil {
		return createSource{}, err
	}
	if len(sources) == 0 {
		return createSource{}, fmt.Errorf("no create sources found")
	}

	selected := 0
	options := make([]huh.Option[int], 0, len(sources))
	for i, source := range sources {
		options = append(options, huh.NewOption(source.PromptLabel(), i))
	}
	form := huh.NewForm(huh.NewGroup(
		huh.NewSelect[int]().
			Title("Create from").
			Options(options...).
			Value(&selected),
	))
	if err := runCreateTUIForm(cmd, form); err != nil {
		return createSource{}, err
	}
	if selected < 0 || selected >= len(sources) {
		return createSource{}, fmt.Errorf("invalid source selection")
	}
	return sources[selected], nil
}

func listCreateSources(cwd string) ([]createSource, error) {
	return listProfileSources(cwd)
}

func listProfileSources(cwd string) ([]createSource, error) {
	var sources []createSource
	for _, scope := range []config.Scope{config.ScopeGlobal, config.ScopeProject} {
		summaries, err := config.ListAgents(scope, cwd)
		if err != nil {
			return nil, err
		}
		for _, summary := range summaries {
			agent, err := config.ReadAgent(summary.Name, scope, cwd)
			if err != nil {
				return nil, err
			}
			sources = append(sources, createSource{
				Kind:        createSourceProfile,
				Name:        agent.Name,
				Description: agent.Description,
				Scope:       scope,
				Runtime:     agent.Runtime.Preferred,
				Agent:       agent,
			})
		}
	}
	return sources, nil
}

func resolveCreateValues(cmd *cobra.Command, reader *bufio.Reader, out io.Writer, cwd string, source createSource, opts createOptions, useTUI bool) (resolvedCreateValues, error) {
	values := defaultCreateValues(source, opts, cwd)
	if cmd.Flags().Changed("skills") {
		values.Skills = normalizeStringList(opts.Skills)
	}
	if cmd.Flags().Changed("mcps") {
		values.MCPs = normalizeStringList(opts.MCPs)
	}
	if cmd.Flags().Changed("runtimes") {
		values.Runtimes = normalizeCreateRuntimeList(opts.Runtimes)
	} else if cmd.Flags().Changed("runtime") {
		values.Runtimes = normalizeCreateRuntimeList([]string{opts.Runtime})
	}

	if !opts.Yes {
		if opts.NoInput {
			return values, fmt.Errorf("create requires --yes or interactive input")
		}
		if useTUI {
			return promptCreateValuesTUI(cmd, out, source, values)
		}
		var err error
		values.Name, err = promptString(reader, out, "Agent name", values.Name)
		if err != nil {
			return values, err
		}
		runtimesValue, err := promptString(reader, out, "Runtimes (comma separated: codex, claude-code, opencode, cline, cursor)", strings.Join(values.Runtimes, ","))
		if err != nil {
			return values, err
		}
		values.Runtimes = splitSelectionValues(runtimesValue)
		if err := finalizeCreateRuntimes(&values); err != nil {
			return values, err
		}
		values.Skills, err = promptCapabilitySelection(reader, out, "Skills", "skills", listInstalledSkillOptions, values.Skills)
		if err != nil {
			return values, err
		}
		values.MCPs, err = promptCapabilitySelection(reader, out, "MCP servers", "mcps", listInstalledMCPOptions, values.MCPs)
		if err != nil {
			return values, err
		}
		printCreatePreview(out, source, values)
		confirmed, err := promptConfirm(reader, out, fmt.Sprintf("Create agent %q for %s", values.Name, strings.Join(values.Runtimes, ",")), true)
		if err != nil {
			return values, err
		}
		if !confirmed {
			return values, fmt.Errorf("create cancelled")
		}
	}

	if err := finalizeCreateRuntimes(&values); err != nil {
		return values, err
	}
	return values, nil
}

func promptCreateValuesTUI(cmd *cobra.Command, out io.Writer, source createSource, values resolvedCreateValues) (resolvedCreateValues, error) {
	skillOptions, err := listInstalledSkillOptions()
	if err != nil {
		return values, err
	}
	mcpOptions, err := listInstalledMCPOptions()
	if err != nil {
		return values, err
	}

	fields := []huh.Field{
		huh.NewInput().
			Title("Agent name").
			Value(&values.Name).
			Validate(huh.ValidateNotEmpty()),
		huh.NewMultiSelect[string]().
			Title("Runtimes").
			Description("Use Space to select one or more runtimes. The first selected runtime in this list becomes preferred.").
			Options(runtimeTUIOptions(values.Runtimes)...).
			Value(&values.Runtimes),
	}
	if options := capabilityTUIOptions(skillOptions, values.Skills); len(options) > 0 {
		fields = append(fields, huh.NewMultiSelect[string]().
			Title("Skills").
			Description("Use Space to select installed skills for this agent.").
			Options(options...).
			Value(&values.Skills))
	}
	if options := capabilityTUIOptions(mcpOptions, values.MCPs); len(options) > 0 {
		fields = append(fields, huh.NewMultiSelect[string]().
			Title("MCP servers").
			Description("Use Space to select MCP servers for this agent.").
			Options(options...).
			Value(&values.MCPs))
	}

	if err := runCreateTUIForm(cmd, huh.NewForm(huh.NewGroup(fields...))); err != nil {
		return values, err
	}
	values.Skills = uniqueSortedCreateStrings(values.Skills)
	values.MCPs = uniqueSortedCreateStrings(values.MCPs)
	if err := finalizeCreateRuntimes(&values); err != nil {
		return values, err
	}

	printCreatePreview(out, source, values)
	confirmed := true
	form := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().
			Title(fmt.Sprintf("Create agent %q for %s?", values.Name, strings.Join(values.Runtimes, ","))).
			Affirmative("Create").
			Negative("Cancel").
			Value(&confirmed),
	))
	if err := runCreateTUIForm(cmd, form); err != nil {
		return values, err
	}
	if !confirmed {
		return values, fmt.Errorf("create cancelled")
	}
	return values, nil
}

func defaultCreateValues(source createSource, opts createOptions, cwd string) resolvedCreateValues {
	values := resolvedCreateValues{Scope: opts.Scope}
	agent := source.Agent
	values.Name = firstNonEmptyString(opts.Name, suggestCopiedAgentName(agent.Name, values.Scope, cwd))
	values.Runtime = firstNonEmptyString(opts.Runtime, agent.Runtime.Preferred, "codex")
	values.Runtimes = runtimePreferenceList(agent.Runtime)
	values.Model = firstNonEmptyString(opts.Model, agent.ModelRun.Model)
	values.Reasoning = firstNonEmptyString(opts.Reasoning, agent.ModelRun.ReasoningEffort, "medium")
	values.Skills = append([]string(nil), agent.Capabilities.Skills...)
	values.MCPs = append([]string(nil), agent.Capabilities.MCPs...)
	if len(values.Runtimes) == 0 {
		values.Runtimes = normalizeCreateRuntimeList([]string{values.Runtime})
	}
	return values
}

func agentFromCreateSource(source createSource, values resolvedCreateValues) *config.AgentProfile {
	return agentFromExistingProfile(source.Agent, values)
}

func agentFromExistingProfile(source *config.AgentProfile, values resolvedCreateValues) *config.AgentProfile {
	agent := cloneAgentProfile(source)
	agent.Name = values.Name
	agent.SourceScope = string(values.Scope)
	if agent.Identity.DisplayName == "" || agent.Identity.DisplayName == source.Name {
		agent.Identity.DisplayName = values.Name
	}
	agent.Runtime.Preferred = values.Runtime
	agent.Runtime.Fallback = runtimeFallbackFromValues(values)
	agent.ModelRun.Model = values.Model
	agent.ModelRun.ReasoningEffort = values.Reasoning
	agent.Capabilities.Skills = append([]string(nil), values.Skills...)
	agent.Capabilities.MCPs = append([]string(nil), values.MCPs...)
	return agent
}

func cloneAgentProfile(source *config.AgentProfile) *config.AgentProfile {
	if source == nil {
		return &config.AgentProfile{}
	}
	agent := *source
	agent.Runtime.Fallback = append([]string(nil), source.Runtime.Fallback...)
	agent.Identity.Tags = append([]string(nil), source.Identity.Tags...)
	agent.Instructions.References = append([]string(nil), source.Instructions.References...)
	agent.IOContract.InputModes = append([]string(nil), source.IOContract.InputModes...)
	agent.Capabilities.Skills = append([]string(nil), source.Capabilities.Skills...)
	agent.Capabilities.MCPs = append([]string(nil), source.Capabilities.MCPs...)
	agent.Capabilities.Commands = append([]string(nil), source.Capabilities.Commands...)
	agent.Capabilities.Hooks = append([]string(nil), source.Capabilities.Hooks...)
	agent.Capabilities.Toolsets = cloneStringMapCreate(source.Capabilities.Toolsets)
	agent.Permissions.Allow = append([]string(nil), source.Permissions.Allow...)
	agent.Permissions.Deny = append([]string(nil), source.Permissions.Deny...)
	agent.Permissions.AdditionalDirectories = append([]string(nil), source.Permissions.AdditionalDirectories...)
	agent.MemoryRefs = append([]config.MemoryRef(nil), source.MemoryRefs...)
	agent.LifecycleHooks.BeforeRun = append([]string(nil), source.LifecycleHooks.BeforeRun...)
	agent.LifecycleHooks.AfterRun = append([]string(nil), source.LifecycleHooks.AfterRun...)
	agent.RuntimeExtensions = cloneRuntimeExtensionsCreate(source.RuntimeExtensions)
	return &agent
}

func cloneStringMapCreate(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func cloneRuntimeExtensionsCreate(in map[string]map[string]any) map[string]map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]map[string]any, len(in))
	for runtime, values := range in {
		cloned := make(map[string]any, len(values))
		for key, value := range values {
			cloned[key] = value
		}
		out[runtime] = cloned
	}
	return out
}

func parseCreateScope(value string) (config.Scope, error) {
	if value == "" {
		return config.ScopeGlobal, nil
	}
	scope := config.Scope(value)
	switch scope {
	case config.ScopeGlobal, config.ScopeProject, config.ScopeLocal:
		return scope, nil
	default:
		return "", fmt.Errorf("invalid scope %q", value)
	}
}

func promptString(reader *bufio.Reader, out io.Writer, label, defaultValue string) (string, error) {
	if defaultValue == "" {
		fmt.Fprintf(out, "%s: ", label)
	} else {
		fmt.Fprintf(out, "%s [%s]: ", label, defaultValue)
	}
	line, err := reader.ReadString('\n')
	if err != nil && !(err == io.EOF && line != "") {
		if err == io.EOF {
			return "", fmt.Errorf("input required for %s", strings.ToLower(label))
		}
		return "", err
	}
	value := strings.TrimSpace(line)
	if value == "" {
		value = defaultValue
	}
	return value, nil
}

func promptStringList(reader *bufio.Reader, out io.Writer, label string, defaults []string) ([]string, error) {
	defaultValue := strings.Join(defaults, ",")
	if defaultValue == "" {
		fmt.Fprintf(out, "%s (comma separated, or none): ", label)
	} else {
		fmt.Fprintf(out, "%s (comma separated, none to clear) [%s]: ", label, defaultValue)
	}
	line, err := reader.ReadString('\n')
	if err != nil && !(err == io.EOF && line != "") {
		if err == io.EOF {
			return nil, fmt.Errorf("input required for %s", strings.ToLower(label))
		}
		return nil, err
	}
	value := strings.TrimSpace(line)
	if value == "" {
		return normalizeStringList(defaults), nil
	}
	if strings.EqualFold(value, "none") {
		return nil, nil
	}
	return splitSelectionValues(value), nil
}

func promptSelectIndex(reader *bufio.Reader, out io.Writer, label string, min, max, defaultValue int) (int, error) {
	fmt.Fprintf(out, "%s [%d]: ", label, defaultValue)
	line, err := reader.ReadString('\n')
	if err != nil && !(err == io.EOF && line != "") {
		if err == io.EOF {
			return 0, fmt.Errorf("input required for %s", strings.ToLower(label))
		}
		return 0, err
	}
	value := strings.TrimSpace(line)
	if value == "" {
		value = strconv.Itoa(defaultValue)
	}
	index, err := strconv.Atoi(value)
	if err != nil || index < min || index > max {
		return 0, fmt.Errorf("invalid %s selection %q", strings.ToLower(label), value)
	}
	return index, nil
}

type capabilityOption struct {
	Name        string
	Description string
	Path        string
}

func promptCapabilitySelection(reader *bufio.Reader, out io.Writer, label, kind string, listInstalled func() ([]capabilityOption, error), defaults []string) ([]string, error) {
	installed, err := listInstalled()
	if err != nil {
		return nil, err
	}
	if len(installed) == 0 && len(defaults) == 0 {
		return nil, nil
	}
	if len(installed) == 0 {
		return promptStringList(reader, out, label, defaults)
	}

	options := mergeSelectionOptions(installedNames(installed), defaults)
	selected := stringSet(defaults)
	installedSet := stringSet(installedNames(installed))
	installedByName := capabilityOptionsByName(installed)
	fmt.Fprintf(out, "%s installed in %s:\n", label, config.RegistryKindDir(kind))
	for i, option := range options {
		mark := " "
		if _, ok := selected[option]; ok {
			mark = "x"
		}
		suffix := ""
		if _, ok := installedSet[option]; !ok {
			suffix = " (referenced, not installed)"
		}
		fmt.Fprintf(out, "  [%s] %d) %s%s\n", mark, i+1, formatCapabilityOption(option, installedByName[option]), suffix)
	}
	fmt.Fprintf(out, "%s selection (numbers/names, comma separated; none to clear; Enter keeps defaults): ", label)
	line, err := reader.ReadString('\n')
	if err != nil && !(err == io.EOF && line != "") {
		if err == io.EOF {
			return nil, fmt.Errorf("input required for %s", strings.ToLower(label))
		}
		return nil, err
	}
	value := strings.TrimSpace(line)
	if value == "" {
		return normalizeStringList(defaults), nil
	}
	if strings.EqualFold(value, "none") {
		return nil, nil
	}

	var selectedValues []string
	for _, token := range splitSelectionValues(value) {
		if index, err := strconv.Atoi(token); err == nil {
			if index < 1 || index > len(options) {
				return nil, fmt.Errorf("invalid %s selection %q", strings.ToLower(label), token)
			}
			selectedValues = append(selectedValues, options[index-1])
			continue
		}
		selectedValues = append(selectedValues, token)
	}
	return uniqueSortedCreateStrings(selectedValues), nil
}

func promptConfirm(reader *bufio.Reader, out io.Writer, label string, defaultYes bool) (bool, error) {
	suffix := " [Y/n]: "
	if !defaultYes {
		suffix = " [y/N]: "
	}
	fmt.Fprint(out, label+suffix)
	line, err := reader.ReadString('\n')
	if err != nil && !(err == io.EOF && line != "") {
		if err == io.EOF {
			return false, fmt.Errorf("input required for confirmation")
		}
		return false, err
	}
	value := strings.ToLower(strings.TrimSpace(line))
	if value == "" {
		return defaultYes, nil
	}
	switch value {
	case "y", "yes":
		return true, nil
	case "n", "no":
		return false, nil
	default:
		return false, fmt.Errorf("invalid confirmation %q", value)
	}
}

func printCreatePreview(out io.Writer, source createSource, values resolvedCreateValues) {
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Preview:")
	fmt.Fprintf(out, "  source: %s\n", source.Summary())
	fmt.Fprintf(out, "  name: %s\n", values.Name)
	fmt.Fprintf(out, "  scope: %s\n", values.Scope)
	fmt.Fprintf(out, "  runtimes: %s\n", previewList(values.Runtimes))
	if values.Model != "" {
		fmt.Fprintf(out, "  model: %s\n", values.Model)
	}
	if values.Reasoning != "" {
		fmt.Fprintf(out, "  reasoning: %s\n", values.Reasoning)
	}
	fmt.Fprintf(out, "  skills: %s\n", previewList(values.Skills))
	fmt.Fprintf(out, "  mcps: %s\n", previewList(values.MCPs))
}

func previewList(values []string) string {
	values = normalizeStringList(values)
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(values, ",")
}

func createUseTUI(cmd *cobra.Command) bool {
	return isTerminalFile(cmd.InOrStdin()) && isTerminalFile(cmd.OutOrStdout())
}

func isTerminalFile(value any) bool {
	file, ok := value.(*os.File)
	if !ok || file == nil {
		return false
	}
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

func runCreateTUIForm(cmd *cobra.Command, form *huh.Form) error {
	return runTUIForm(cmd, form, "create")
}

func runTUIForm(cmd *cobra.Command, form *huh.Form, action string) error {
	err := form.
		WithInput(cmd.InOrStdin()).
		WithOutput(cmd.OutOrStdout()).
		WithTheme(huh.ThemeCharm()).
		Run()
	if errors.Is(err, huh.ErrUserAborted) {
		return fmt.Errorf("%s cancelled", action)
	}
	return err
}

func runtimeTUIOptions(selected []string) []huh.Option[string] {
	selected = normalizeCreateRuntimeList(selected)
	runtimes := append([]string(nil), selected...)
	for _, runtime := range []string{"codex", "claude-code", "opencode", "cline", "cursor"} {
		if runtime != "" && !containsCreateString(runtimes, runtime) {
			runtimes = append(runtimes, runtime)
		}
	}
	selectedSet := stringSet(selected)
	options := make([]huh.Option[string], 0, len(runtimes))
	for _, runtime := range runtimes {
		_, isSelected := selectedSet[runtime]
		options = append(options, huh.NewOption(runtime, runtime).Selected(isSelected))
	}
	return options
}

func containsCreateString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func capabilityTUIOptions(installed []capabilityOption, defaults []string) []huh.Option[string] {
	names := mergeSelectionOptions(installedNames(installed), defaults)
	if len(names) == 0 {
		return nil
	}
	installedByName := capabilityOptionsByName(installed)
	selected := stringSet(defaults)
	options := make([]huh.Option[string], 0, len(names))
	for _, name := range names {
		label := formatCapabilityOption(name, installedByName[name])
		if _, ok := installedByName[name]; !ok {
			label += " (referenced, not installed)"
		}
		_, isSelected := selected[name]
		options = append(options, huh.NewOption(label, name).Selected(isSelected))
	}
	return options
}

func finalizeCreateRuntimes(values *resolvedCreateValues) error {
	if values == nil {
		return fmt.Errorf("create values are nil")
	}
	values.Runtimes = normalizeCreateRuntimeList(values.Runtimes)
	if len(values.Runtimes) == 0 && values.Runtime != "" {
		values.Runtimes = normalizeCreateRuntimeList([]string{values.Runtime})
	}
	if len(values.Runtimes) == 0 {
		return fmt.Errorf("at least one runtime is required")
	}
	for _, runtime := range values.Runtimes {
		if !isKnownRuntime(runtime) {
			return fmt.Errorf("invalid runtime %q", runtime)
		}
	}
	values.Runtime = values.Runtimes[0]
	return nil
}

func normalizeCreateRuntimeList(values []string) []string {
	var out []string
	seen := map[string]struct{}{}
	for _, value := range values {
		for _, runtime := range splitSelectionValues(value) {
			runtime = strings.TrimSpace(runtime)
			if runtime == "" {
				continue
			}
			if _, ok := seen[runtime]; ok {
				continue
			}
			seen[runtime] = struct{}{}
			out = append(out, runtime)
		}
	}
	return out
}

func runtimePreferenceList(runtime config.RuntimePreferences) []string {
	return normalizeCreateRuntimeList(append([]string{runtime.Preferred}, runtime.Fallback...))
}

func runtimePreferencesFromValues(values resolvedCreateValues) config.RuntimePreferences {
	return config.RuntimePreferences{
		Preferred: values.Runtime,
		Fallback:  runtimeFallbackFromValues(values),
	}
}

func runtimeFallbackFromValues(values resolvedCreateValues) []string {
	if len(values.Runtimes) <= 1 {
		return nil
	}
	return append([]string(nil), values.Runtimes[1:]...)
}

func runtimeStartCommand(runtime string) string {
	switch runtime {
	case "codex":
		return "codex"
	case "claude-code":
		return "claude"
	case "opencode":
		return "opencode"
	case "cline":
		return "cline"
	case "cursor":
		return "cursor ."
	default:
		return ""
	}
}

func runtimeStartCommands(runtimes []string) []string {
	var out []string
	seen := map[string]struct{}{}
	for _, runtime := range runtimes {
		if _, ok := seen[runtime]; ok {
			continue
		}
		seen[runtime] = struct{}{}
		if command := runtimeStartCommand(runtime); command != "" {
			out = append(out, command)
		}
	}
	return out
}

func listInstalledSkills() ([]string, error) {
	options, err := listInstalledSkillOptions()
	if err != nil {
		return nil, err
	}
	return installedNames(options), nil
}

func listInstalledSkillOptions() ([]capabilityOption, error) {
	entries, err := os.ReadDir(config.RegistryKindDir("skills"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var options []capabilityOption
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		path := filepath.Join(config.RegistryKindDir("skills"), name, "SKILL.md")
		if _, err := os.Stat(path); err == nil {
			options = append(options, capabilityOption{
				Name:        name,
				Description: skillDescription(path),
				Path:        path,
			})
		}
	}
	sort.Slice(options, func(i, j int) bool {
		return options[i].Name < options[j].Name
	})
	return options, nil
}

func listInstalledMCPs() ([]string, error) {
	options, err := listInstalledMCPOptions()
	if err != nil {
		return nil, err
	}
	return installedNames(options), nil
}

func listInstalledMCPOptions() ([]capabilityOption, error) {
	entries, err := os.ReadDir(config.RegistryKindDir("mcps"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var options []capabilityOption
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := filepath.Ext(name)
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		options = append(options, capabilityOption{
			Name: strings.TrimSuffix(name, ext),
			Path: filepath.Join(config.RegistryKindDir("mcps"), name),
		})
	}
	sort.Slice(options, func(i, j int) bool {
		return options[i].Name < options[j].Name
	})
	return options, nil
}

func skillDescription(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "---") {
			continue
		}
		line = strings.TrimPrefix(line, ">")
		line = strings.TrimSpace(strings.Trim(line, "`*_"))
		if line != "" {
			return line
		}
	}
	return ""
}

func installedNames(options []capabilityOption) []string {
	names := make([]string, 0, len(options))
	for _, option := range options {
		names = append(names, option.Name)
	}
	return names
}

func capabilityOptionsByName(options []capabilityOption) map[string]capabilityOption {
	byName := make(map[string]capabilityOption, len(options))
	for _, option := range options {
		byName[option.Name] = option
	}
	return byName
}

func formatCapabilityOption(name string, option capabilityOption) string {
	if option.Description == "" {
		return name
	}
	return name + " - " + option.Description
}

func mergeSelectionOptions(installed, defaults []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, value := range normalizeStringList(installed) {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	for _, value := range normalizeStringList(defaults) {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func splitSelectionValues(value string) []string {
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\t'
	})
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field != "" {
			out = append(out, field)
		}
	}
	return out
}

func stringSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range normalizeStringList(values) {
		set[value] = struct{}{}
	}
	return set
}

func uniqueSortedCreateStrings(values []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, value := range normalizeStringList(values) {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func suggestCopiedAgentName(name string, scope config.Scope, cwd string) string {
	return suggestAvailableAgentName(name+"-copy", scope, cwd)
}

func suggestAvailableAgentName(name string, scope config.Scope, cwd string) string {
	name = safeCreateName(name)
	if name == "" {
		name = "agent"
	}
	if !agentExists(name, scope, cwd) {
		return name
	}
	for i := 2; ; i++ {
		candidate := name + "-" + strconv.Itoa(i)
		if !agentExists(candidate, scope, cwd) {
			return candidate
		}
	}
}

func agentExists(name string, scope config.Scope, cwd string) bool {
	_, err := config.ReadAgent(name, scope, cwd)
	return err == nil
}

func safeCreateName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if r == '-' || r == '_' || r == ' ' || r == '.' || r == '/' || r == ':' {
			if !lastDash && b.Len() > 0 {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}
