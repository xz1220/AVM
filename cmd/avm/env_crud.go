package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"reflect"
	"sort"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"github.com/xz1220/agent-vm/internal/config"
)

type envEditOptions struct {
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

type envResolvedView struct {
	Name             string                           `yaml:"name"`
	Description      string                           `yaml:"description,omitempty"`
	Version          string                           `yaml:"version"`
	RuntimeAgents    map[string]config.RuntimeAgent   `yaml:"runtime_agents,omitempty"`
	Targets          []string                         `yaml:"targets,omitempty"`
	RuntimeOverrides map[string]any                   `yaml:"runtime_overrides,omitempty"`
	SourceFiles      []string                         `yaml:"source_files,omitempty"`
	Warnings         []string                         `yaml:"warnings,omitempty"`
	Agents           map[string]envResolvedAgentBrief `yaml:"agents,omitempty"`
}

type envResolvedAgentBrief struct {
	Name        string `yaml:"name"`
	Runtime     string `yaml:"runtime"`
	SourceScope string `yaml:"source_scope"`
}

func newEnvListCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List AVM runtime environments",
		Args:  cobra.NoArgs,
		RunE:  runEnvList,
	}
}

func newEnvShowCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <name>",
		Short: "Show an AVM runtime environment",
		Args:  validateEnvShowArgs,
		RunE:  runEnvShow,
	}
	cmd.Flags().Bool("resolved", false, "show merged environment for the current workspace")
	cmd.Flags().Bool("local", false, "show the current workspace environment override")
	return cmd
}

func newEnvCloneCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "clone <source>",
		Short: "Clone an AVM runtime environment",
		Args:  cobra.ExactArgs(1),
		RunE:  runEnvClone,
	}
	cmd.Flags().String("name", "", "new environment name")
	return cmd
}

func newEnvEditCommand() *cobra.Command {
	var opts envEditOptions
	cmd := &cobra.Command{
		Use:     "edit <name>",
		Aliases: []string{"update"},
		Short:   "Edit an AVM runtime environment",
		Args:    validateEnvEditArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEnvEdit(cmd, args, opts)
		},
	}
	cmd.Flags().BoolVar(&opts.Local, "local", false, "edit the current workspace environment override")
	cmd.Flags().StringVar(&opts.Description, "description", "", "environment description")
	cmd.Flags().StringSliceVar(&opts.Targets, "targets", nil, "target runtimes for this environment")
	addEnvRuntimeEditFlags(cmd, &opts)
	cmd.Flags().BoolVar(&opts.Yes, "yes", false, "apply flag-provided changes and do not prompt")
	cmd.Flags().BoolVar(&opts.NoInput, "no-input", false, "fail instead of prompting")
	return cmd
}

func newEnvRenameCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "rename <old-name> <new-name>",
		Short: "Rename an AVM runtime environment",
		Args:  cobra.ExactArgs(2),
		RunE:  runEnvRename,
	}
	cmd.Flags().Bool("update-refs", false, "update current workspace environment references to the new name")
	return cmd
}

func newEnvDeleteCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete an AVM runtime environment",
		Args:  validateEnvDeleteArgs,
		RunE:  runEnvDelete,
	}
	cmd.Flags().Bool("force", false, "delete even if the current workspace references the environment")
	cmd.Flags().Bool("local", false, "delete the current workspace environment override")
	return cmd
}

func addEnvRuntimeEditFlags(cmd *cobra.Command, opts *envEditOptions) {
	cmd.Flags().StringVar(&opts.Codex, "codex", "", "agent profile for Codex, or none to remove")
	cmd.Flags().StringVar(&opts.ClaudeCode, "claude-code", "", "agent profile for Claude Code, or none to remove")
	cmd.Flags().StringVar(&opts.OpenCode, "opencode", "", "agent profile for OpenCode, or none to remove")
	cmd.Flags().StringVar(&opts.Cline, "cline", "", "agent profile for Cline, or none to remove")
	cmd.Flags().StringVar(&opts.Cursor, "cursor", "", "agent profile for Cursor, or none to remove")
}

func validateEnvShowArgs(cmd *cobra.Command, args []string) error {
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
	return cobra.ExactArgs(1)(cmd, args)
}

func validateEnvEditArgs(cmd *cobra.Command, args []string) error {
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
	return cobra.ExactArgs(1)(cmd, args)
}

func validateEnvDeleteArgs(cmd *cobra.Command, args []string) error {
	local, err := cmd.Flags().GetBool("local")
	if err != nil {
		return err
	}
	if local {
		if len(args) > 0 {
			return fmt.Errorf("accepts 0 arg(s), received %d", len(args))
		}
		return nil
	}
	return cobra.ExactArgs(1)(cmd, args)
}

func runEnvList(cmd *cobra.Command, args []string) error {
	envs, err := config.ListEnvironments()
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "NAME\tVERSION\tDESCRIPTION")
	for _, env := range envs {
		fmt.Fprintf(out, "%s\t%s\t%s\n", env.Name, env.Version, env.Description)
	}
	return nil
}

func runEnvShow(cmd *cobra.Command, args []string) error {
	local, err := cmd.Flags().GetBool("local")
	if err != nil {
		return err
	}
	resolved, err := cmd.Flags().GetBool("resolved")
	if err != nil {
		return err
	}
	if local && resolved {
		return fmt.Errorf("--local cannot be combined with --resolved")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	if local {
		override, err := readLocalOverrideForShow(args, cwd)
		if err != nil {
			return err
		}
		return encodeYAML(cmd, override)
	}

	name := args[0]
	if resolved {
		return showResolvedEnvironment(cmd, name, cwd)
	}
	env, err := readEnvForShow(name)
	if err != nil {
		return err
	}
	return encodeYAML(cmd, env)
}

func runEnvClone(cmd *cobra.Command, args []string) error {
	if err := ensureInitialized(); err != nil {
		return err
	}
	name, err := cmd.Flags().GetString("name")
	if err != nil {
		return err
	}
	if name == "" {
		return fmt.Errorf("env clone requires --name")
	}
	if exists, err := config.EnvironmentExists(name); err != nil {
		return err
	} else if exists {
		return fmt.Errorf("env %q already exists", name)
	}
	source, err := readEnvForShow(args[0])
	if err != nil {
		return err
	}
	env := cloneEnvironmentForCommand(source)
	env.Name = name
	if err := config.WriteEnvironment(env); err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "cloned env %s to %s\n", source.Name, env.Name)
	return nil
}

func runEnvEdit(cmd *cobra.Command, args []string, opts envEditOptions) error {
	if err := ensureInitialized(); err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	if opts.Local {
		return runEnvEditLocal(cmd, args, opts, cwd)
	}
	return runEnvEditGlobal(cmd, args[0], opts, cwd)
}

func runEnvEditGlobal(cmd *cobra.Command, name string, opts envEditOptions, cwd string) error {
	env, err := readEnvForShow(name)
	if err != nil {
		return err
	}
	before := cloneEnvironmentForCommand(env)

	if envEditHasFlagChanges(cmd) {
		if err := applyEnvEditFlags(cmd, env, opts, cwd); err != nil {
			return err
		}
		return writeEditedEnvironment(cmd, before, env)
	}
	if opts.NoInput || opts.Yes {
		return fmt.Errorf("env edit requires field flags or interactive input")
	}

	reader := bufio.NewReader(cmd.InOrStdin())
	if createUseTUI(cmd) {
		if err := promptEnvEditTUI(cmd, env, cwd); err != nil {
			return err
		}
	} else if err := promptEnvEdit(reader, cmd.OutOrStdout(), env, cwd); err != nil {
		return err
	}
	return writeEditedEnvironment(cmd, before, env)
}

func runEnvEditLocal(cmd *cobra.Command, args []string, opts envEditOptions, cwd string) error {
	if cmd.Flags().Changed("description") {
		return fmt.Errorf("--description cannot be used with --local")
	}
	extends, override, err := localOverrideForEdit(args, cwd)
	if err != nil {
		return err
	}
	if _, err := config.ReadEnvironment(extends); err != nil {
		return err
	}
	before := cloneProjectOverrideForCommand(override)

	if envEditHasFlagChanges(cmd) {
		if err := applyProjectOverrideEditFlags(cmd, override, opts, cwd); err != nil {
			return err
		}
		return writeEditedProjectOverride(cmd, before, override, cwd)
	}
	if opts.NoInput || opts.Yes {
		return fmt.Errorf("env edit --local requires field flags or interactive input")
	}

	reader := bufio.NewReader(cmd.InOrStdin())
	if createUseTUI(cmd) {
		if err := promptProjectOverrideEditTUI(cmd, override, cwd); err != nil {
			return err
		}
	} else if err := promptProjectOverrideEdit(reader, cmd.OutOrStdout(), override, cwd); err != nil {
		return err
	}
	return writeEditedProjectOverride(cmd, before, override, cwd)
}

func runEnvRename(cmd *cobra.Command, args []string) error {
	if err := ensureInitialized(); err != nil {
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
	if _, err := readEnvForShow(oldName); err != nil {
		return err
	}
	if exists, err := config.EnvironmentExists(newName); err != nil {
		return err
	} else if exists {
		return fmt.Errorf("env %q already exists", newName)
	}
	refs, err := config.FindEnvironmentReferences(oldName, cwd)
	if err != nil {
		return err
	}
	if activeEnvironmentReferenced(refs) {
		return fmt.Errorf("env %q is active; activate another profile or environment before renaming", oldName)
	}
	if len(refs) > 0 && !updateRefs {
		return fmt.Errorf("env %q is referenced; use --update-refs to rename references:\n%s", oldName, formatEnvironmentReferences(refs))
	}

	if err := config.RenameEnvironment(oldName, newName); err != nil {
		return err
	}
	if updateRefs {
		if _, err := config.UpdateEnvironmentReferences(oldName, newName, cwd); err != nil {
			return err
		}
	}
	fmt.Fprintf(cmd.OutOrStdout(), "renamed env %s to %s\n", oldName, newName)
	if updateRefs && len(refs) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "updated %d reference(s)\n", len(refs))
	}
	return nil
}

func runEnvDelete(cmd *cobra.Command, args []string) error {
	if err := ensureInitialized(); err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	local, err := cmd.Flags().GetBool("local")
	if err != nil {
		return err
	}
	if local {
		return runEnvDeleteLocal(cmd, cwd)
	}

	force, err := cmd.Flags().GetBool("force")
	if err != nil {
		return err
	}
	name := args[0]
	if _, err := readEnvForShow(name); err != nil {
		return err
	}
	refs, err := config.FindEnvironmentReferences(name, cwd)
	if err != nil {
		return err
	}
	if activeEnvironmentReferenced(refs) {
		return fmt.Errorf("env %q is active; activate another profile or environment before deleting", name)
	}
	if len(refs) > 0 && !force {
		return fmt.Errorf("env %q is referenced; use --force to delete anyway:\n%s", name, formatEnvironmentReferences(refs))
	}
	if err := config.DeleteEnvironment(name); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("env %q not found", name)
		}
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "deleted env %s\n", name)
	if force && len(refs) > 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "left %d reference(s) unchanged\n", len(refs))
	}
	return nil
}

func runEnvDeleteLocal(cmd *cobra.Command, cwd string) error {
	if err := config.DeleteProjectOverride(cwd); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("local env override not found at %s", config.ProjectEnvPath(cwd))
		}
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "deleted local env override\n")
	return nil
}

func readEnvForShow(name string) (*config.Environment, error) {
	env, err := config.ReadEnvironment(name)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("env %q not found", name)
		}
		return nil, err
	}
	return env, nil
}

func readLocalOverrideForShow(args []string, cwd string) (*config.ProjectOverride, error) {
	override, err := config.ReadProjectOverride(cwd)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("local env override not found at %s", config.ProjectEnvPath(cwd))
		}
		return nil, err
	}
	if len(args) > 0 && override.Extends != args[0] {
		return nil, fmt.Errorf("local env override extends %q, not %q", override.Extends, args[0])
	}
	return override, nil
}

func showResolvedEnvironment(cmd *cobra.Command, name, cwd string) error {
	resolved, err := config.ResolveActivation(config.ActiveRef{Kind: config.ActiveKindEnv, Name: name}, cwd)
	if err != nil {
		return err
	}
	view := envResolvedView{
		Name:             resolved.Env.Name,
		Description:      resolved.Env.Description,
		Version:          resolved.Env.Version,
		RuntimeAgents:    resolved.Env.RuntimeAgents,
		Targets:          resolved.Env.Targets,
		RuntimeOverrides: resolved.Env.RuntimeOverrides,
		SourceFiles:      resolved.SourceFiles,
		Warnings:         resolved.Warnings,
		Agents:           resolvedAgentBriefs(resolved.RuntimeAgents),
	}
	return encodeYAML(cmd, view)
}

func resolvedAgentBriefs(agents map[string]config.AgentProfile) map[string]envResolvedAgentBrief {
	if len(agents) == 0 {
		return nil
	}
	out := make(map[string]envResolvedAgentBrief, len(agents))
	for runtime, agent := range agents {
		out[runtime] = envResolvedAgentBrief{
			Name:        agent.Name,
			Runtime:     agent.Runtime.Preferred,
			SourceScope: agent.SourceScope,
		}
	}
	return out
}

func applyEnvEditFlags(cmd *cobra.Command, env *config.Environment, opts envEditOptions, cwd string) error {
	if cmd.Flags().Changed("description") {
		env.Description = opts.Description
	}
	targetsChanged := cmd.Flags().Changed("targets")
	if cmd.Flags().Changed("targets") {
		targets, err := normalizeEnvTargets(opts.Targets)
		if err != nil {
			return err
		}
		env.Targets = targets
		if err := validateRuntimeFlagsWithinTargets(cmd, env.Targets, opts); err != nil {
			return err
		}
		pruneRuntimeAgentsOutsideTargets(env.RuntimeAgents, env.Targets)
	}
	if err := applyEnvRuntimeFlags(cmd, env.RuntimeAgents, &env.Targets, opts); err != nil {
		return err
	}
	if targetsChanged {
		pruneRuntimeAgentsOutsideTargets(env.RuntimeAgents, env.Targets)
	}
	if err := validateEnvironmentRuntimeCoverage(env.Targets, env.RuntimeAgents); err != nil {
		return err
	}
	return validateRuntimeAgentProfiles(env.RuntimeAgents, cwd)
}

func applyProjectOverrideEditFlags(cmd *cobra.Command, override *config.ProjectOverride, opts envEditOptions, cwd string) error {
	if cmd.Flags().Changed("targets") {
		targets, err := normalizeEnvTargets(opts.Targets)
		if err != nil {
			return err
		}
		override.Targets = targets
	}
	if override.RuntimeAgents == nil {
		override.RuntimeAgents = map[string]config.RuntimeAgent{}
	}
	if err := applyEnvRuntimeFlags(cmd, override.RuntimeAgents, nil, opts); err != nil {
		return err
	}
	return validateRuntimeAgentProfiles(override.RuntimeAgents, cwd)
}

func validateRuntimeFlagsWithinTargets(cmd *cobra.Command, targets []string, opts envEditOptions) error {
	return validateRuntimeValueFlagsWithinTargets(cmd, targets, envRuntimeFlagValues(opts))
}

func validateRuntimeValueFlagsWithinTargets(cmd *cobra.Command, targets []string, values map[string]string) error {
	targetSet := stringSet(targets)
	for runtime, value := range values {
		if !cmd.Flags().Changed(runtime) {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(value), "none") {
			continue
		}
		if _, ok := targetSet[runtime]; !ok {
			return fmt.Errorf("--%s cannot be set because %s is not in --targets", runtime, runtime)
		}
	}
	return nil
}

func validateEnvironmentRuntimeCoverage(targets []string, runtimeAgents map[string]config.RuntimeAgent) error {
	for _, runtime := range normalizeStringList(targets) {
		agent, ok := runtimeAgents[runtime]
		if !ok || strings.TrimSpace(agent.Primary) == "" {
			return fmt.Errorf("targets.%s: missing runtime agent mapping; set --%s <agent> or remove %s from --targets", runtime, runtime, runtime)
		}
	}
	return nil
}

func pruneRuntimeAgentsOutsideTargets(runtimeAgents map[string]config.RuntimeAgent, targets []string) {
	if len(runtimeAgents) == 0 {
		return
	}
	targetSet := stringSet(targets)
	for runtime := range runtimeAgents {
		if _, ok := targetSet[runtime]; !ok {
			delete(runtimeAgents, runtime)
		}
	}
}

func applyEnvRuntimeFlags(cmd *cobra.Command, runtimeAgents map[string]config.RuntimeAgent, targets *[]string, opts envEditOptions) error {
	if runtimeAgents == nil {
		return fmt.Errorf("runtime agent map is nil")
	}
	for runtime, value := range envRuntimeFlagValues(opts) {
		if !cmd.Flags().Changed(runtime) {
			continue
		}
		value = strings.TrimSpace(value)
		if value == "" {
			return fmt.Errorf("--%s requires an agent profile name or none", runtime)
		}
		if strings.EqualFold(value, "none") {
			delete(runtimeAgents, runtime)
			if targets != nil {
				*targets = removeStringValue(*targets, runtime)
			}
			continue
		}
		runtimeAgents[runtime] = config.RuntimeAgent{Primary: value}
		if targets != nil {
			*targets = appendTargetIfMissing(*targets, runtime)
		}
	}
	return nil
}

func promptEnvEdit(reader *bufio.Reader, out io.Writer, env *config.Environment, cwd string) error {
	before := cloneEnvironmentForCommand(env)
	var err error
	env.Description, err = promptString(reader, out, "Description", env.Description)
	if err != nil {
		return err
	}
	targetValue, err := promptString(reader, out, "Targets (comma separated: codex, claude-code, opencode, cline, cursor)", strings.Join(env.Targets, ","))
	if err != nil {
		return err
	}
	env.Targets, err = normalizeEnvTargets(splitSelectionValues(targetValue))
	if err != nil {
		return err
	}
	if err := promptEnvRuntimeAgents(reader, out, env.RuntimeAgents, &env.Targets); err != nil {
		return err
	}
	pruneRuntimeAgentsOutsideTargets(env.RuntimeAgents, env.Targets)
	if err := validateEnvironmentRuntimeCoverage(env.Targets, env.RuntimeAgents); err != nil {
		return err
	}
	if err := validateRuntimeAgentProfiles(env.RuntimeAgents, cwd); err != nil {
		return err
	}
	printEnvEditPreview(out, before, env)
	confirmed, err := promptConfirm(reader, out, fmt.Sprintf("Apply changes to env %q", env.Name), true)
	if err != nil {
		return err
	}
	if !confirmed {
		return fmt.Errorf("env edit cancelled")
	}
	return nil
}

func promptEnvEditTUI(cmd *cobra.Command, env *config.Environment, cwd string) error {
	before := cloneEnvironmentForCommand(env)
	targets := strings.Join(env.Targets, ",")
	values := envRuntimeInputValues(env.RuntimeAgents)
	codex := values["codex"]
	claudeCode := values["claude-code"]
	openCode := values["opencode"]
	cline := values["cline"]
	cursor := values["cursor"]
	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Description").Value(&env.Description),
		huh.NewInput().Title("Targets").Description("Comma separated").Value(&targets),
		huh.NewInput().Title("Codex agent").Description("Use none to remove").Value(&codex),
		huh.NewInput().Title("Claude Code agent").Description("Use none to remove").Value(&claudeCode),
		huh.NewInput().Title("OpenCode agent").Description("Use none to remove").Value(&openCode),
		huh.NewInput().Title("Cline agent").Description("Use none to remove").Value(&cline),
		huh.NewInput().Title("Cursor agent").Description("Use none to remove").Value(&cursor),
	))
	if err := runTUIForm(cmd, form, "env edit"); err != nil {
		return err
	}
	values["codex"] = codex
	values["claude-code"] = claudeCode
	values["opencode"] = openCode
	values["cline"] = cline
	values["cursor"] = cursor
	var err error
	env.Targets, err = normalizeEnvTargets(splitSelectionValues(targets))
	if err != nil {
		return err
	}
	env.RuntimeAgents = runtimeAgentsFromInputValues(values)
	pruneRuntimeAgentsOutsideTargets(env.RuntimeAgents, env.Targets)
	if err := validateEnvironmentRuntimeCoverage(env.Targets, env.RuntimeAgents); err != nil {
		return err
	}
	if err := validateRuntimeAgentProfiles(env.RuntimeAgents, cwd); err != nil {
		return err
	}
	printEnvEditPreview(cmd.OutOrStdout(), before, env)
	confirmed := true
	form = huh.NewForm(huh.NewGroup(
		huh.NewConfirm().
			Title(fmt.Sprintf("Apply changes to env %q?", env.Name)).
			Affirmative("Apply").
			Negative("Cancel").
			Value(&confirmed),
	))
	if err := runTUIForm(cmd, form, "env edit"); err != nil {
		return err
	}
	if !confirmed {
		return fmt.Errorf("env edit cancelled")
	}
	return nil
}

func promptProjectOverrideEdit(reader *bufio.Reader, out io.Writer, override *config.ProjectOverride, cwd string) error {
	before := cloneProjectOverrideForCommand(override)
	targets := ""
	if override.Targets != nil {
		targets = strings.Join(override.Targets, ",")
	}
	targetValue, err := promptString(reader, out, "Override targets (comma separated, none to clear)", targets)
	if err != nil {
		return err
	}
	if strings.EqualFold(targetValue, "none") {
		override.Targets = nil
	} else if strings.TrimSpace(targetValue) != "" {
		override.Targets, err = normalizeEnvTargets(splitSelectionValues(targetValue))
		if err != nil {
			return err
		}
	}
	if override.RuntimeAgents == nil {
		override.RuntimeAgents = map[string]config.RuntimeAgent{}
	}
	if err := promptEnvRuntimeAgents(reader, out, override.RuntimeAgents, nil); err != nil {
		return err
	}
	if err := validateRuntimeAgentProfiles(override.RuntimeAgents, cwd); err != nil {
		return err
	}
	printProjectOverrideEditPreview(out, before, override)
	confirmed, err := promptConfirm(reader, out, fmt.Sprintf("Apply local override changes for env %q", override.Extends), true)
	if err != nil {
		return err
	}
	if !confirmed {
		return fmt.Errorf("env edit cancelled")
	}
	return nil
}

func promptProjectOverrideEditTUI(cmd *cobra.Command, override *config.ProjectOverride, cwd string) error {
	before := cloneProjectOverrideForCommand(override)
	targets := strings.Join(override.Targets, ",")
	values := envRuntimeInputValues(override.RuntimeAgents)
	codex := values["codex"]
	claudeCode := values["claude-code"]
	openCode := values["opencode"]
	cline := values["cline"]
	cursor := values["cursor"]
	form := huh.NewForm(huh.NewGroup(
		huh.NewInput().Title("Override targets").Description("Comma separated; leave blank to inherit").Value(&targets),
		huh.NewInput().Title("Codex agent").Description("Leave blank to inherit, use none to remove override").Value(&codex),
		huh.NewInput().Title("Claude Code agent").Description("Leave blank to inherit, use none to remove override").Value(&claudeCode),
		huh.NewInput().Title("OpenCode agent").Description("Leave blank to inherit, use none to remove override").Value(&openCode),
		huh.NewInput().Title("Cline agent").Description("Leave blank to inherit, use none to remove override").Value(&cline),
		huh.NewInput().Title("Cursor agent").Description("Leave blank to inherit, use none to remove override").Value(&cursor),
	))
	if err := runTUIForm(cmd, form, "env edit"); err != nil {
		return err
	}
	values["codex"] = codex
	values["claude-code"] = claudeCode
	values["opencode"] = openCode
	values["cline"] = cline
	values["cursor"] = cursor
	if strings.TrimSpace(targets) == "" {
		override.Targets = nil
	} else {
		parsed, err := normalizeEnvTargets(splitSelectionValues(targets))
		if err != nil {
			return err
		}
		override.Targets = parsed
	}
	override.RuntimeAgents = runtimeAgentsFromInputValues(values)
	if err := validateRuntimeAgentProfiles(override.RuntimeAgents, cwd); err != nil {
		return err
	}
	printProjectOverrideEditPreview(cmd.OutOrStdout(), before, override)
	confirmed := true
	form = huh.NewForm(huh.NewGroup(
		huh.NewConfirm().
			Title(fmt.Sprintf("Apply local override changes for env %q?", override.Extends)).
			Affirmative("Apply").
			Negative("Cancel").
			Value(&confirmed),
	))
	if err := runTUIForm(cmd, form, "env edit"); err != nil {
		return err
	}
	if !confirmed {
		return fmt.Errorf("env edit cancelled")
	}
	return nil
}

func promptEnvRuntimeAgents(reader *bufio.Reader, out io.Writer, agents map[string]config.RuntimeAgent, targets *[]string) error {
	if agents == nil {
		return fmt.Errorf("runtime agent map is nil")
	}
	for _, runtime := range envCreateRuntimeOrder {
		current := agents[runtime].Primary
		value, err := promptString(reader, out, runtime+" agent (none to remove)", current)
		if err != nil {
			return err
		}
		value = strings.TrimSpace(value)
		if value == "" || strings.EqualFold(value, "none") {
			delete(agents, runtime)
			if targets != nil {
				*targets = removeStringValue(*targets, runtime)
			}
			continue
		}
		agents[runtime] = config.RuntimeAgent{Primary: value}
		if targets != nil {
			*targets = appendTargetIfMissing(*targets, runtime)
		}
	}
	return nil
}

func writeEditedEnvironment(cmd *cobra.Command, before, env *config.Environment) error {
	if err := config.WriteEnvironment(env); err != nil {
		return err
	}
	diffs := envChangeSummary(before, env)
	out := cmd.OutOrStdout()
	if len(diffs) == 0 {
		fmt.Fprintf(out, "env %s unchanged\n", env.Name)
		return nil
	}
	fmt.Fprintf(out, "updated env %s\n", env.Name)
	for _, diff := range diffs {
		fmt.Fprintf(out, "  %s\n", diff)
	}
	return nil
}

func writeEditedProjectOverride(cmd *cobra.Command, before, override *config.ProjectOverride, cwd string) error {
	if err := config.WriteProjectOverride(cwd, override); err != nil {
		return err
	}
	diffs := projectOverrideChangeSummary(before, override)
	out := cmd.OutOrStdout()
	if len(diffs) == 0 {
		fmt.Fprintf(out, "local env override %s unchanged\n", override.Extends)
		return nil
	}
	fmt.Fprintf(out, "updated local env override %s\n", override.Extends)
	for _, diff := range diffs {
		fmt.Fprintf(out, "  %s\n", diff)
	}
	return nil
}

func localOverrideForEdit(args []string, cwd string) (string, *config.ProjectOverride, error) {
	override, err := config.ReadProjectOverride(cwd)
	if err == nil {
		if len(args) > 0 && override.Extends != args[0] {
			return "", nil, fmt.Errorf("local env override extends %q, not %q", override.Extends, args[0])
		}
		return override.Extends, override, nil
	}
	if err != nil && !os.IsNotExist(err) {
		return "", nil, err
	}

	extends, err := envCreateLocalExtends(args)
	if err != nil {
		return "", nil, err
	}
	return extends, &config.ProjectOverride{Extends: extends}, nil
}

func envEditHasFlagChanges(cmd *cobra.Command) bool {
	for _, name := range []string{"description", "targets", "codex", "claude-code", "opencode", "cline", "cursor"} {
		if cmd.Flags().Changed(name) {
			return true
		}
	}
	return false
}

func envRuntimeFlagValues(opts envEditOptions) map[string]string {
	return map[string]string{
		"codex":       opts.Codex,
		"claude-code": opts.ClaudeCode,
		"opencode":    opts.OpenCode,
		"cline":       opts.Cline,
		"cursor":      opts.Cursor,
	}
}

func normalizeEnvTargets(values []string) ([]string, error) {
	targets := normalizeCreateRuntimeList(values)
	if len(targets) == 0 {
		return nil, fmt.Errorf("at least one target is required")
	}
	for _, runtime := range targets {
		if !isKnownRuntime(runtime) {
			return nil, fmt.Errorf("invalid runtime %q", runtime)
		}
	}
	return targets, nil
}

func appendTargetIfMissing(targets []string, runtime string) []string {
	if runtime == "" || containsCreateString(targets, runtime) {
		return targets
	}
	return append(targets, runtime)
}

func envRuntimeInputValues(agents map[string]config.RuntimeAgent) map[string]string {
	out := make(map[string]string, len(envCreateRuntimeOrder))
	for _, runtime := range envCreateRuntimeOrder {
		out[runtime] = agents[runtime].Primary
	}
	return out
}

func runtimeAgentsFromInputValues(values map[string]string) map[string]config.RuntimeAgent {
	agents := map[string]config.RuntimeAgent{}
	for _, runtime := range envCreateRuntimeOrder {
		value := strings.TrimSpace(values[runtime])
		if value == "" || strings.EqualFold(value, "none") {
			continue
		}
		agents[runtime] = config.RuntimeAgent{Primary: value}
	}
	return agents
}

func cloneEnvironmentForCommand(env *config.Environment) *config.Environment {
	if env == nil {
		return &config.Environment{}
	}
	return &config.Environment{
		Name:             env.Name,
		Description:      env.Description,
		Version:          env.Version,
		RuntimeAgents:    cloneRuntimeAgentsForCommand(env.RuntimeAgents),
		Targets:          append([]string(nil), env.Targets...),
		RuntimeOverrides: cloneAnyMapForEnvCommand(env.RuntimeOverrides),
	}
}

func cloneProjectOverrideForCommand(override *config.ProjectOverride) *config.ProjectOverride {
	if override == nil {
		return &config.ProjectOverride{}
	}
	return &config.ProjectOverride{
		Extends:          override.Extends,
		RuntimeAgents:    cloneRuntimeAgentsForCommand(override.RuntimeAgents),
		Targets:          append([]string(nil), override.Targets...),
		RuntimeOverrides: cloneAnyMapForEnvCommand(override.RuntimeOverrides),
	}
}

func cloneRuntimeAgentsForCommand(agents map[string]config.RuntimeAgent) map[string]config.RuntimeAgent {
	if agents == nil {
		return nil
	}
	cloned := make(map[string]config.RuntimeAgent, len(agents))
	for runtime, agent := range agents {
		cloned[runtime] = config.RuntimeAgent{
			Primary:   agent.Primary,
			Available: append([]string(nil), agent.Available...),
		}
	}
	return cloned
}

func cloneAnyMapForEnvCommand(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = cloneAnyForEnvCommand(value)
	}
	return out
}

func cloneAnyForEnvCommand(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneAnyMapForEnvCommand(typed)
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = cloneAnyForEnvCommand(item)
		}
		return out
	default:
		return typed
	}
}

func printEnvEditPreview(out io.Writer, before, after *config.Environment) {
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Changes:")
	diffs := envChangeSummary(before, after)
	if len(diffs) == 0 {
		fmt.Fprintln(out, "  none")
		return
	}
	for _, diff := range diffs {
		fmt.Fprintf(out, "  %s\n", diff)
	}
}

func printProjectOverrideEditPreview(out io.Writer, before, after *config.ProjectOverride) {
	fmt.Fprintln(out)
	fmt.Fprintln(out, "Changes:")
	diffs := projectOverrideChangeSummary(before, after)
	if len(diffs) == 0 {
		fmt.Fprintln(out, "  none")
		return
	}
	for _, diff := range diffs {
		fmt.Fprintf(out, "  %s\n", diff)
	}
}

func envChangeSummary(before, after *config.Environment) []string {
	var diffs []string
	if before.Description != after.Description {
		diffs = append(diffs, fmt.Sprintf("description: %s -> %s", previewScalar(before.Description), previewScalar(after.Description)))
	}
	if !reflect.DeepEqual(normalizeStringList(before.Targets), normalizeStringList(after.Targets)) {
		diffs = append(diffs, fmt.Sprintf("targets: %s -> %s", previewList(before.Targets), previewList(after.Targets)))
	}
	if !reflect.DeepEqual(runtimeAgentSummary(before.RuntimeAgents), runtimeAgentSummary(after.RuntimeAgents)) {
		diffs = append(diffs, fmt.Sprintf("runtime_agents: %s -> %s", previewList(runtimeAgentSummary(before.RuntimeAgents)), previewList(runtimeAgentSummary(after.RuntimeAgents))))
	}
	return diffs
}

func projectOverrideChangeSummary(before, after *config.ProjectOverride) []string {
	var diffs []string
	if before.Extends != after.Extends {
		diffs = append(diffs, fmt.Sprintf("extends: %s -> %s", previewScalar(before.Extends), previewScalar(after.Extends)))
	}
	if !reflect.DeepEqual(normalizeStringList(before.Targets), normalizeStringList(after.Targets)) {
		diffs = append(diffs, fmt.Sprintf("targets: %s -> %s", previewList(before.Targets), previewList(after.Targets)))
	}
	if !reflect.DeepEqual(runtimeAgentSummary(before.RuntimeAgents), runtimeAgentSummary(after.RuntimeAgents)) {
		diffs = append(diffs, fmt.Sprintf("runtime_agents: %s -> %s", previewList(runtimeAgentSummary(before.RuntimeAgents)), previewList(runtimeAgentSummary(after.RuntimeAgents))))
	}
	return diffs
}

func runtimeAgentSummary(agents map[string]config.RuntimeAgent) []string {
	if len(agents) == 0 {
		return nil
	}
	keys := make([]string, 0, len(agents))
	for runtime := range agents {
		keys = append(keys, runtime)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, runtime := range keys {
		agent := agents[runtime]
		value := runtime + "=" + agent.Primary
		if len(agent.Available) > 0 {
			value += "[" + strings.Join(agent.Available, ",") + "]"
		}
		out = append(out, value)
	}
	return out
}

func activeEnvironmentReferenced(refs []config.EnvironmentReference) bool {
	for _, ref := range refs {
		if ref.Kind == "active" {
			return true
		}
	}
	return false
}

func formatEnvironmentReferences(refs []config.EnvironmentReference) string {
	lines := make([]string, 0, len(refs))
	for _, ref := range refs {
		lines = append(lines, "  "+formatEnvironmentReference(ref))
	}
	return strings.Join(lines, "\n")
}

func formatEnvironmentReference(ref config.EnvironmentReference) string {
	switch ref.Kind {
	case "active":
		return fmt.Sprintf("%s: %s", ref.Path, ref.Field)
	case "project_override":
		return fmt.Sprintf("%s project override %s: %s", ref.Path, ref.Name, ref.Field)
	default:
		return fmt.Sprintf("%s %s %s: %s", ref.Path, ref.Kind, ref.Name, ref.Field)
	}
}
