package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"github.com/xz1220/agent-vm/internal/config"
)

type envCreateOptions struct {
	Description string
	Targets     []string
	Codex       string
	ClaudeCode  string
	OpenCode    string
	Cline       string
	Cursor      string
	Local       bool
	Yes         bool
	NoInput     bool
}

type resolvedEnvCreateValues struct {
	Name          string
	Description   string
	Targets       []string
	RuntimeAgents map[string]config.RuntimeAgent
}

func newEnvCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "env",
		Short: "Manage AVM runtime environments",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		newEnvCreateCommand(),
		newEnvListCommand(),
		newEnvShowCommand(),
		newEnvCloneCommand(),
		newEnvEditCommand(),
		newEnvRenameCommand(),
		newEnvDeleteCommand(),
	)
	return cmd
}

func newEnvCreateCommand() *cobra.Command {
	var opts envCreateOptions
	cmd := &cobra.Command{
		Use:   "create [name]",
		Short: "Create an AVM runtime environment",
		Args:  validateEnvCreateArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEnvCreate(cmd, args, opts)
		},
	}
	cmd.Flags().BoolVar(&opts.Local, "local", false, "create the environment for the current workspace")
	cmd.Flags().StringVar(&opts.Description, "description", "", "environment description")
	cmd.Flags().StringSliceVar(&opts.Targets, "targets", nil, "target runtimes for this environment")
	addEnvRuntimeCreateFlags(cmd, &opts)
	cmd.Flags().BoolVar(&opts.Yes, "yes", false, "accept defaults and do not prompt")
	cmd.Flags().BoolVar(&opts.NoInput, "no-input", false, "fail instead of prompting")
	return cmd
}

func validateEnvCreateArgs(cmd *cobra.Command, args []string) error {
	local, err := cmd.Flags().GetBool("local")
	if err != nil {
		return err
	}
	if local {
		if len(args) > 1 {
			return fmt.Errorf("accepts at most 1 arg(s), received %d", len(args))
		}
		return nil
	}
	if len(args) > 1 {
		return fmt.Errorf("accepts at most 1 arg(s), received %d", len(args))
	}
	return nil
}

func runEnvCreate(cmd *cobra.Command, args []string, opts envCreateOptions) error {
	if err := ensureInitialized(); err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	reader := bufio.NewReader(cmd.InOrStdin())
	out := cmd.OutOrStdout()

	if opts.Local {
		return runEnvCreateLocal(cmd, reader, out, args, opts, cwd)
	}
	return runEnvCreateGlobal(cmd, reader, out, args, opts, cwd)
}

func runEnvCreateGlobal(cmd *cobra.Command, reader *bufio.Reader, out io.Writer, args []string, opts envCreateOptions, cwd string) error {
	values, err := resolveEnvCreateValues(cmd, reader, out, args, opts, cwd)
	if err != nil {
		return err
	}
	if exists, err := config.EnvironmentExists(values.Name); err != nil {
		return err
	} else if exists {
		return fmt.Errorf("env %q already exists", values.Name)
	}

	env := &config.Environment{
		Name:          values.Name,
		Description:   values.Description,
		RuntimeAgents: values.RuntimeAgents,
		Targets:       values.Targets,
	}
	if err := validateEnvironmentRuntimeCoverage(env.Targets, env.RuntimeAgents); err != nil {
		return err
	}
	if err := config.WriteEnvironment(env); err != nil {
		return err
	}

	fmt.Fprintf(out, "created env %s\n", env.Name)
	return nil
}

func runEnvCreateLocal(cmd *cobra.Command, reader *bufio.Reader, out io.Writer, args []string, opts envCreateOptions, cwd string) error {
	values, err := resolveLocalEnvCreateValues(cmd, reader, out, args, opts, cwd)
	if err != nil {
		return err
	}
	if _, err := config.ReadEnvironment(values.Name); err != nil {
		return err
	}
	if exists, err := config.ProjectOverrideExists(cwd); err != nil {
		return err
	} else if exists {
		return fmt.Errorf("local env override already exists at %s", config.ProjectEnvPath(cwd))
	}
	override := &config.ProjectOverride{
		Extends:       values.Name,
		RuntimeAgents: values.RuntimeAgents,
		Targets:       values.Targets,
	}
	if err := config.WriteProjectOverride(cwd, override); err != nil {
		return err
	}

	fmt.Fprintf(out, "created local env override %s\n", values.Name)
	return nil
}

func addEnvRuntimeCreateFlags(cmd *cobra.Command, opts *envCreateOptions) {
	cmd.Flags().StringVar(&opts.Codex, "codex", "", "agent profile for Codex")
	cmd.Flags().StringVar(&opts.ClaudeCode, "claude-code", "", "agent profile for Claude Code")
	cmd.Flags().StringVar(&opts.OpenCode, "opencode", "", "agent profile for OpenCode")
	cmd.Flags().StringVar(&opts.Cline, "cline", "", "agent profile for Cline")
	cmd.Flags().StringVar(&opts.Cursor, "cursor", "", "agent profile for Cursor")
}

func resolveEnvCreateValues(cmd *cobra.Command, reader *bufio.Reader, out io.Writer, args []string, opts envCreateOptions, cwd string) (resolvedEnvCreateValues, error) {
	values := defaultEnvCreateValues(args, opts, true)
	if shouldPromptEnvCreate(cmd, args, opts) {
		if createUseTUI(cmd) {
			return promptEnvCreateTUI(cmd, out, values, cwd)
		}
		return promptEnvCreate(reader, out, values, cwd)
	}
	if values.Name == "" {
		return values, fmt.Errorf("env create requires a name when --yes or --no-input is used")
	}
	if cmd.Flags().Changed("targets") {
		if err := validateRuntimeValueFlagsWithinTargets(cmd, values.Targets, envCreateRuntimeValues(opts)); err != nil {
			return values, err
		}
	}
	if err := finalizeEnvCreateValues(&values, cwd); err != nil {
		return values, err
	}
	return values, nil
}

func resolveLocalEnvCreateValues(cmd *cobra.Command, reader *bufio.Reader, out io.Writer, args []string, opts envCreateOptions, cwd string) (resolvedEnvCreateValues, error) {
	values := defaultEnvCreateValues(args, opts, false)
	if values.Name == "" {
		values.Name = activeEnvNameForCreate()
	}
	if shouldPromptEnvCreate(cmd, args, opts) {
		if createUseTUI(cmd) {
			return promptLocalEnvCreateTUI(cmd, out, values, cwd)
		}
		return promptLocalEnvCreate(reader, out, values, cwd)
	}
	if values.Name == "" {
		extends, err := envCreateLocalExtends(args)
		if err != nil {
			return values, err
		}
		values.Name = extends
	}
	if err := validateRuntimeAgentProfiles(values.RuntimeAgents, cwd); err != nil {
		return values, err
	}
	return values, nil
}

func defaultEnvCreateValues(args []string, opts envCreateOptions, withDefault bool) resolvedEnvCreateValues {
	values := resolvedEnvCreateValues{
		Description:   opts.Description,
		RuntimeAgents: envCreateRuntimeAgentsFromOptions(opts),
	}
	if len(args) > 0 {
		values.Name = args[0]
	}
	if len(opts.Targets) > 0 {
		values.Targets = normalizeCreateRuntimeList(opts.Targets)
	} else if withDefault {
		values.Targets = envTargetsFromRuntimeAgents(values.RuntimeAgents)
	}
	if withDefault && len(values.RuntimeAgents) == 0 && len(values.Targets) == 0 {
		values.RuntimeAgents["codex"] = config.RuntimeAgent{Primary: "default"}
		values.Targets = []string{"codex"}
	}
	return values
}

func envCreateRuntimeAgentsFromOptions(opts envCreateOptions) map[string]config.RuntimeAgent {
	runtimeAgents := make(map[string]config.RuntimeAgent)
	for runtime, profile := range envCreateRuntimeValues(opts) {
		profile = strings.TrimSpace(profile)
		if profile == "" {
			continue
		}
		runtimeAgents[runtime] = config.RuntimeAgent{Primary: profile}
	}
	return runtimeAgents
}

func envCreateRuntimeValues(opts envCreateOptions) map[string]string {
	return map[string]string{
		"codex":       opts.Codex,
		"claude-code": opts.ClaudeCode,
		"opencode":    opts.OpenCode,
		"cline":       opts.Cline,
		"cursor":      opts.Cursor,
	}
}

func shouldPromptEnvCreate(cmd *cobra.Command, args []string, opts envCreateOptions) bool {
	if opts.Yes || opts.NoInput {
		return false
	}
	if len(args) == 0 {
		if opts.Local && envCreateHasFieldFlags(cmd) {
			return false
		}
		return true
	}
	return createUseTUI(cmd) && !envCreateHasFieldFlags(cmd)
}

func envCreateHasFieldFlags(cmd *cobra.Command) bool {
	for _, name := range []string{"description", "targets", "codex", "claude-code", "opencode", "cline", "cursor"} {
		if cmd.Flags().Changed(name) {
			return true
		}
	}
	return false
}

func promptEnvCreate(reader *bufio.Reader, out io.Writer, values resolvedEnvCreateValues, cwd string) (resolvedEnvCreateValues, error) {
	var err error
	values.Name, err = promptString(reader, out, "Environment name", values.Name)
	if err != nil {
		return values, err
	}
	values.Description, err = promptString(reader, out, "Description", values.Description)
	if err != nil {
		return values, err
	}
	targetValue, err := promptString(reader, out, "Targets (comma separated: codex, claude-code, opencode, cline, cursor)", strings.Join(values.Targets, ","))
	if err != nil {
		return values, err
	}
	values.Targets, err = normalizeEnvTargets(splitSelectionValues(targetValue))
	if err != nil {
		return values, err
	}
	if values.RuntimeAgents == nil {
		values.RuntimeAgents = map[string]config.RuntimeAgent{}
	}
	for _, runtime := range values.Targets {
		current := values.RuntimeAgents[runtime].Primary
		if current == "" {
			current = "default"
		}
		value, err := promptString(reader, out, runtime+" agent", current)
		if err != nil {
			return values, err
		}
		value = strings.TrimSpace(value)
		if value == "" {
			return values, fmt.Errorf("runtime_agents.%s.primary: required", runtime)
		}
		values.RuntimeAgents[runtime] = config.RuntimeAgent{Primary: value}
	}
	pruneRuntimeAgentsOutsideTargets(values.RuntimeAgents, values.Targets)
	if err := finalizeEnvCreateValues(&values, cwd); err != nil {
		return values, err
	}
	printEnvCreatePreview(out, values, false)
	confirmed, err := promptConfirm(reader, out, fmt.Sprintf("Create env %q", values.Name), true)
	if err != nil {
		return values, err
	}
	if !confirmed {
		return values, fmt.Errorf("env create cancelled")
	}
	return values, nil
}

func promptEnvCreateTUI(cmd *cobra.Command, out io.Writer, values resolvedEnvCreateValues, cwd string) (resolvedEnvCreateValues, error) {
	targets := strings.Join(values.Targets, ",")
	runtimeValues := envRuntimeInputValues(values.RuntimeAgents)
	codex := defaultEnvCreateAgentValue(runtimeValues["codex"])
	claudeCode := runtimeValues["claude-code"]
	openCode := runtimeValues["opencode"]
	cline := runtimeValues["cline"]
	cursor := runtimeValues["cursor"]
	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Environment name").Value(&values.Name).Validate(huh.ValidateNotEmpty()),
		huh.NewInput().Title("Description").Value(&values.Description),
		huh.NewInput().Title("Targets").Description("Comma separated").Value(&targets),
		huh.NewInput().Title("Codex agent").Description("Required when codex is targeted").Value(&codex),
		huh.NewInput().Title("Claude Code agent").Description("Required when claude-code is targeted").Value(&claudeCode),
		huh.NewInput().Title("OpenCode agent").Description("Required when opencode is targeted").Value(&openCode),
		huh.NewInput().Title("Cline agent").Description("Required when cline is targeted").Value(&cline),
		huh.NewInput().Title("Cursor agent").Description("Required when cursor is targeted").Value(&cursor),
	))
	if err := runTUIForm(cmd, form, "env create"); err != nil {
		return values, err
	}
	values.Targets = splitSelectionValues(targets)
	values.RuntimeAgents = runtimeAgentsFromInputValues(map[string]string{
		"codex":       codex,
		"claude-code": claudeCode,
		"opencode":    openCode,
		"cline":       cline,
		"cursor":      cursor,
	})
	pruneRuntimeAgentsOutsideTargets(values.RuntimeAgents, values.Targets)
	if err := finalizeEnvCreateValues(&values, cwd); err != nil {
		return values, err
	}
	printEnvCreatePreview(out, values, false)
	confirmed := true
	form = huh.NewForm(huh.NewGroup(
		huh.NewConfirm().
			Title(fmt.Sprintf("Create env %q?", values.Name)).
			Affirmative("Create").
			Negative("Cancel").
			Value(&confirmed),
	))
	if err := runTUIForm(cmd, form, "env create"); err != nil {
		return values, err
	}
	if !confirmed {
		return values, fmt.Errorf("env create cancelled")
	}
	return values, nil
}

func promptLocalEnvCreate(reader *bufio.Reader, out io.Writer, values resolvedEnvCreateValues, cwd string) (resolvedEnvCreateValues, error) {
	var err error
	values.Name, err = promptString(reader, out, "Extends env", values.Name)
	if err != nil {
		return values, err
	}
	targets := ""
	if values.Targets != nil {
		targets = strings.Join(values.Targets, ",")
	}
	targetValue, err := promptString(reader, out, "Override targets (comma separated, blank to inherit)", targets)
	if err != nil {
		return values, err
	}
	if strings.TrimSpace(targetValue) == "" {
		values.Targets = nil
	} else {
		values.Targets, err = normalizeEnvTargets(splitSelectionValues(targetValue))
		if err != nil {
			return values, err
		}
	}
	if values.RuntimeAgents == nil {
		values.RuntimeAgents = map[string]config.RuntimeAgent{}
	}
	if err := promptEnvRuntimeAgents(reader, out, values.RuntimeAgents, nil); err != nil {
		return values, err
	}
	if err := validateRuntimeAgentProfiles(values.RuntimeAgents, cwd); err != nil {
		return values, err
	}
	printEnvCreatePreview(out, values, true)
	confirmed, err := promptConfirm(reader, out, fmt.Sprintf("Create local env override %q", values.Name), true)
	if err != nil {
		return values, err
	}
	if !confirmed {
		return values, fmt.Errorf("env create cancelled")
	}
	return values, nil
}

func promptLocalEnvCreateTUI(cmd *cobra.Command, out io.Writer, values resolvedEnvCreateValues, cwd string) (resolvedEnvCreateValues, error) {
	targets := strings.Join(values.Targets, ",")
	runtimeValues := envRuntimeInputValues(values.RuntimeAgents)
	codex := runtimeValues["codex"]
	claudeCode := runtimeValues["claude-code"]
	openCode := runtimeValues["opencode"]
	cline := runtimeValues["cline"]
	cursor := runtimeValues["cursor"]
	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Extends env").Value(&values.Name).Validate(huh.ValidateNotEmpty()),
		huh.NewInput().Title("Override targets").Description("Comma separated; leave blank to inherit").Value(&targets),
		huh.NewInput().Title("Codex agent").Description("Leave blank to inherit").Value(&codex),
		huh.NewInput().Title("Claude Code agent").Description("Leave blank to inherit").Value(&claudeCode),
		huh.NewInput().Title("OpenCode agent").Description("Leave blank to inherit").Value(&openCode),
		huh.NewInput().Title("Cline agent").Description("Leave blank to inherit").Value(&cline),
		huh.NewInput().Title("Cursor agent").Description("Leave blank to inherit").Value(&cursor),
	))
	if err := runTUIForm(cmd, form, "env create"); err != nil {
		return values, err
	}
	if strings.TrimSpace(targets) == "" {
		values.Targets = nil
	} else {
		parsed, err := normalizeEnvTargets(splitSelectionValues(targets))
		if err != nil {
			return values, err
		}
		values.Targets = parsed
	}
	values.RuntimeAgents = runtimeAgentsFromInputValues(map[string]string{
		"codex":       codex,
		"claude-code": claudeCode,
		"opencode":    openCode,
		"cline":       cline,
		"cursor":      cursor,
	})
	if err := validateRuntimeAgentProfiles(values.RuntimeAgents, cwd); err != nil {
		return values, err
	}
	printEnvCreatePreview(out, values, true)
	confirmed := true
	form = huh.NewForm(huh.NewGroup(
		huh.NewConfirm().
			Title(fmt.Sprintf("Create local env override %q?", values.Name)).
			Affirmative("Create").
			Negative("Cancel").
			Value(&confirmed),
	))
	if err := runTUIForm(cmd, form, "env create"); err != nil {
		return values, err
	}
	if !confirmed {
		return values, fmt.Errorf("env create cancelled")
	}
	return values, nil
}

func finalizeEnvCreateValues(values *resolvedEnvCreateValues, cwd string) error {
	if values == nil {
		return fmt.Errorf("env create values are nil")
	}
	values.Name = strings.TrimSpace(values.Name)
	if values.Name == "" {
		return fmt.Errorf("env create requires a name")
	}
	var err error
	values.Targets, err = normalizeEnvTargets(values.Targets)
	if err != nil {
		return err
	}
	if values.RuntimeAgents == nil {
		values.RuntimeAgents = map[string]config.RuntimeAgent{}
	}
	pruneRuntimeAgentsOutsideTargets(values.RuntimeAgents, values.Targets)
	if err := validateEnvironmentRuntimeCoverage(values.Targets, values.RuntimeAgents); err != nil {
		return err
	}
	return validateRuntimeAgentProfiles(values.RuntimeAgents, cwd)
}

func defaultEnvCreateAgentValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "default"
	}
	return value
}

func activeEnvNameForCreate() string {
	cfg, err := config.ReadGlobalConfig()
	if err != nil || cfg.Active.Kind != config.ActiveKindEnv {
		return ""
	}
	return cfg.Active.Name
}

func envTargetsFromRuntimeAgents(agents map[string]config.RuntimeAgent) []string {
	targets := make([]string, 0, len(agents))
	for _, runtime := range envCreateRuntimeOrder {
		if agent, ok := agents[runtime]; ok && agent.Primary != "" {
			targets = append(targets, runtime)
		}
	}
	return targets
}

func printEnvCreatePreview(out io.Writer, values resolvedEnvCreateValues, local bool) {
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Preview:")
	if local {
		fmt.Fprintf(out, "  extends: %s\n", values.Name)
		if values.Targets == nil {
			fmt.Fprintln(out, "  targets: inherit")
		} else {
			fmt.Fprintf(out, "  targets: %s\n", previewList(values.Targets))
		}
	} else {
		fmt.Fprintf(out, "  name: %s\n", values.Name)
		fmt.Fprintf(out, "  targets: %s\n", previewList(values.Targets))
	}
	if values.Description != "" {
		fmt.Fprintf(out, "  description: %s\n", values.Description)
	}
	fmt.Fprintf(out, "  runtime_agents: %s\n", previewList(runtimeAgentSummary(values.RuntimeAgents)))
}

func envCreateLocalExtends(args []string) (string, error) {
	if len(args) > 0 {
		return args[0], nil
	}

	cfg, err := config.ReadGlobalConfig()
	if err != nil {
		return "", err
	}
	if cfg.Active.Kind != config.ActiveKindEnv {
		return "", fmt.Errorf("--local env override requires active env or an explicit env name")
	}
	return cfg.Active.Name, nil
}

func validateRuntimeAgentProfiles(runtimeAgents map[string]config.RuntimeAgent, cwd string) error {
	runtimes := make([]string, 0, len(runtimeAgents))
	for runtime := range runtimeAgents {
		runtimes = append(runtimes, runtime)
	}
	sort.Strings(runtimes)

	for _, runtime := range runtimes {
		agent := runtimeAgents[runtime]
		if agent.Primary != "" {
			if err := validateRuntimeAgentProfile(agent.Primary, cwd); err != nil {
				return fmt.Errorf("runtime_agents.%s.primary: %w", runtime, err)
			}
		}
		for i, available := range agent.Available {
			if err := validateRuntimeAgentProfile(available, cwd); err != nil {
				return fmt.Errorf("runtime_agents.%s.available[%d]: %w", runtime, i, err)
			}
		}
	}
	return nil
}

func validateRuntimeAgentProfile(name, cwd string) error {
	if _, err := config.ReadAgent(name, config.ScopeProject, cwd); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}

	if _, err := config.ReadAgent(name, config.ScopeGlobal, cwd); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}

	return fmt.Errorf("profile %q not found in %s or %s", name, config.ProjectAgentPath(cwd, name), config.AgentPath(name))
}

var envCreateRuntimeOrder = []string{"codex", "claude-code", "opencode", "cline", "cursor"}
