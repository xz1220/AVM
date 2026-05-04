package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/xz1220/agent-vm/internal/adapter"
	"github.com/xz1220/agent-vm/internal/boundary"
	"github.com/xz1220/agent-vm/internal/config"
	avmruntime "github.com/xz1220/agent-vm/internal/runtime"
	"gopkg.in/yaml.v3"
)

type agentMappingPreviewRegistry interface {
	Get(runtime string) (adapter.Adapter, bool)
}

type agentMappingPreview struct {
	Runtime      string                    `yaml:"runtime"`
	Agent        string                    `yaml:"agent"`
	ManagedPaths []agentPreviewManagedPath `yaml:"managed_paths"`
	Warnings     []string                  `yaml:"warnings"`
	Mappings     agentPreviewMappingGroups `yaml:"mappings"`
}

type agentPreviewManagedPath struct {
	Path        string `yaml:"path"`
	Owner       string `yaml:"owner"`
	MergeMode   string `yaml:"merge_mode"`
	Required    bool   `yaml:"required"`
	Description string `yaml:"description,omitempty"`
}

type agentPreviewMappingGroups struct {
	Native                 []agentPreviewMapping `yaml:"native"`
	RenderedAsInstructions []agentPreviewMapping `yaml:"rendered_as_instructions"`
	Ignored                []agentPreviewMapping `yaml:"ignored"`
	Unsupported            []agentPreviewMapping `yaml:"unsupported"`
}

type agentPreviewMapping struct {
	Source string `yaml:"source"`
	Target string `yaml:"target,omitempty"`
	Reason string `yaml:"reason,omitempty"`
}

func newAgentCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent",
		Short: "Manage AVM agent profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		newAgentCreateCommand(),
		newAgentCloneCommand(),
		newAgentEditCommand(),
		newAgentListCommand(),
		newAgentRenameCommand(),
		newAgentDeleteCommand(),
		newAgentShowCommand(),
	)
	return cmd
}

func newAgentCreateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create an AVM agent profile",
		Args:  cobra.ExactArgs(1),
		RunE:  runAgentCreate,
	}
	cmd.Flags().String("runtime", "", "preferred runtime for this agent profile")
	cmd.Flags().String("scope", "", "profile scope")
	cmd.Flags().String("model", "", "model override")
	cmd.Flags().String("reasoning", "", "reasoning effort override")
	cmd.Flags().StringSlice("skills", nil, "skills to attach")
	cmd.Flags().StringSlice("mcps", nil, "MCP servers to attach")
	return cmd
}

func newAgentListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List AVM agent profiles",
		Args:  cobra.NoArgs,
		RunE:  runAgentList,
	}
	cmd.Flags().String("scope", string(config.ScopeGlobal), "profile scope")
	return cmd
}

func newAgentShowCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show <name>",
		Short: "Show an AVM agent profile",
		Args:  cobra.ExactArgs(1),
		RunE:  runAgentShow,
	}
	cmd.Flags().String("runtime", "", "runtime mapping to inspect")
	cmd.Flags().String("scope", string(config.ScopeGlobal), "profile scope")
	return cmd
}

func runAgentCreate(cmd *cobra.Command, args []string) error {
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

	runtime, err := cmd.Flags().GetString("runtime")
	if err != nil {
		return err
	}
	if runtime == "" {
		runtime = "codex"
	}
	if !isKnownRuntime(runtime) {
		return fmt.Errorf("invalid runtime %q", runtime)
	}

	model, err := cmd.Flags().GetString("model")
	if err != nil {
		return err
	}
	reasoning, err := cmd.Flags().GetString("reasoning")
	if err != nil {
		return err
	}
	skills, err := cmd.Flags().GetStringSlice("skills")
	if err != nil {
		return err
	}
	mcps, err := cmd.Flags().GetStringSlice("mcps")
	if err != nil {
		return err
	}
	if exists, err := config.AgentExists(args[0], scope, cwd); err != nil {
		return err
	} else if exists {
		return fmt.Errorf("agent %q already exists", args[0])
	}

	agent := &config.AgentProfile{
		Name:        args[0],
		SourceScope: string(scope),
		Runtime: config.RuntimePreferences{
			Preferred: runtime,
		},
		ModelRun: config.ModelRun{
			Model:           model,
			ReasoningEffort: reasoning,
		},
		Capabilities: config.CapabilityRefs{
			Skills: normalizeStringList(skills),
			MCPs:   normalizeStringList(mcps),
		},
	}
	if err := config.WriteAgent(agent, scope, cwd); err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "created agent %s\n", agent.Name)
	return nil
}

func runAgentList(cmd *cobra.Command, args []string) error {
	scope, err := scopeFromFlag(cmd)
	if err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	agents, err := config.ListAgents(scope, cwd)
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "NAME\tSCOPE\tVERSION\tDESCRIPTION")
	for _, agent := range agents {
		fmt.Fprintf(out, "%s\t%s\t%s\t%s\n", agent.Name, agent.SourceScope, agent.Version, agent.Description)
	}
	return nil
}

func runAgentShow(cmd *cobra.Command, args []string) error {
	scope, err := scopeFromFlag(cmd)
	if err != nil {
		return err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	runtime, err := cmd.Flags().GetString("runtime")
	if err != nil {
		return err
	}
	if runtime != "" && !isKnownRuntime(runtime) {
		return fmt.Errorf("invalid runtime %q", runtime)
	}

	agent, err := readAgentForShow(args[0], scope, cwd)
	if err != nil {
		return err
	}
	if runtime != "" {
		preview, err := buildAgentMappingPreview(context.Background(), agent, runtime, cwd, avmruntime.NewRegistry())
		if err != nil {
			return err
		}
		return encodeYAML(cmd, preview)
	}
	return encodeYAML(cmd, agent)
}

func readAgentForShow(name string, scope config.Scope, cwd string) (*config.AgentProfile, error) {
	agent, err := config.ReadAgent(name, scope, cwd)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("profile %q not found", name)
		}
		return nil, err
	}
	return agent, nil
}

func buildAgentMappingPreview(ctx context.Context, agent *config.AgentProfile, runtime, cwd string, registry agentMappingPreviewRegistry) (*agentMappingPreview, error) {
	if agent == nil {
		return nil, fmt.Errorf("agent profile is nil")
	}
	if registry == nil {
		return nil, fmt.Errorf("runtime %q adapter not registered", runtime)
	}

	adp, ok := registry.Get(runtime)
	if !ok || adp == nil {
		return nil, fmt.Errorf("runtime %q adapter not registered", runtime)
	}

	runtimeBoundary, err := boundary.Resolve(boundary.Input{
		Runtime:   runtime,
		AgentID:   agent.ID,
		AgentName: agent.Name,
	})
	if err != nil {
		return nil, fmt.Errorf("runtime %q boundary failed: %w", runtime, err)
	}
	resolved := resolvedActivationForAgentPreview(agent, runtime)
	input, err := adapter.RenderInputFromResolved(resolved, runtime, adapter.RenderInputOptions{
		ProjectRoot: cwd,
		ActiveDir:   config.ActiveDir(),
		Boundaries: map[string]boundary.RuntimeBoundary{
			runtime: runtimeBoundary,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("runtime %q mapping input failed: %w", runtime, err)
	}

	plan, err := adp.Plan(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("runtime %q mapping preview failed: %w", runtime, err)
	}
	if plan == nil {
		return nil, fmt.Errorf("runtime %q mapping preview failed: adapter returned nil render plan", runtime)
	}

	managedPaths := adp.ManagedPaths(ctx, plan)
	if len(managedPaths) == 0 {
		managedPaths = append([]adapter.ManagedPath(nil), plan.ManagedPaths...)
	}
	return mappingPreviewFromPlan(runtime, input.Agent.Name, managedPaths, plan.Mappings, plan.Warnings), nil
}

func resolvedActivationForAgentPreview(agent *config.AgentProfile, runtime string) *config.ResolvedActivation {
	profile := *agent
	return &config.ResolvedActivation{
		Active: config.ActiveRef{
			Kind: config.ActiveKindProfile,
			Name: profile.Name,
		},
		RuntimeAgents: map[string]config.AgentProfile{
			runtime: profile,
		},
		Capabilities: map[string]config.ResolvedCapabilities{
			runtime: resolvedCapabilitiesForAgentPreview(profile),
		},
		Targets: []string{runtime},
	}
}

func resolvedCapabilitiesForAgentPreview(agent config.AgentProfile) config.ResolvedCapabilities {
	toolsets := make(map[string]string, len(agent.Capabilities.Toolsets))
	for name, mode := range agent.Capabilities.Toolsets {
		toolsets[name] = mode
	}
	if len(toolsets) == 0 {
		toolsets = nil
	}

	return config.ResolvedCapabilities{
		Skills:   append([]string(nil), agent.Capabilities.Skills...),
		MCPs:     append([]string(nil), agent.Capabilities.MCPs...),
		Commands: append([]string(nil), agent.Capabilities.Commands...),
		Hooks:    append([]string(nil), agent.Capabilities.Hooks...),
		Toolsets: toolsets,
	}
}

func mappingPreviewFromPlan(runtime, agentName string, managedPaths []adapter.ManagedPath, mappings []adapter.FieldMapping, warnings []string) *agentMappingPreview {
	return &agentMappingPreview{
		Runtime:      runtime,
		Agent:        agentName,
		ManagedPaths: previewManagedPaths(managedPaths),
		Warnings:     uniqueSortedStrings(warnings),
		Mappings:     previewMappingGroups(mappings),
	}
}

func previewManagedPaths(paths []adapter.ManagedPath) []agentPreviewManagedPath {
	out := make([]agentPreviewManagedPath, 0, len(paths))
	for _, path := range paths {
		out = append(out, agentPreviewManagedPath{
			Path:        path.Path,
			Owner:       path.Owner,
			MergeMode:   string(path.MergeMode),
			Required:    path.Required,
			Description: path.Description,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		left := out[i]
		right := out[j]
		if left.Path != right.Path {
			return left.Path < right.Path
		}
		if left.Owner != right.Owner {
			return left.Owner < right.Owner
		}
		return left.MergeMode < right.MergeMode
	})
	return out
}

func previewMappingGroups(mappings []adapter.FieldMapping) agentPreviewMappingGroups {
	sorted := append([]adapter.FieldMapping(nil), mappings...)
	sort.SliceStable(sorted, func(i, j int) bool {
		left := sorted[i]
		right := sorted[j]
		if left.SourcePath != right.SourcePath {
			return left.SourcePath < right.SourcePath
		}
		if left.TargetPath != right.TargetPath {
			return left.TargetPath < right.TargetPath
		}
		if left.Status != right.Status {
			return left.Status < right.Status
		}
		return left.Reason < right.Reason
	})

	groups := agentPreviewMappingGroups{
		Native:                 []agentPreviewMapping{},
		RenderedAsInstructions: []agentPreviewMapping{},
		Ignored:                []agentPreviewMapping{},
		Unsupported:            []agentPreviewMapping{},
	}
	for _, mapping := range sorted {
		item := agentPreviewMapping{
			Source: mapping.SourcePath,
			Target: mapping.TargetPath,
			Reason: mapping.Reason,
		}
		switch mapping.Status {
		case adapter.MappingNative:
			groups.Native = append(groups.Native, item)
		case adapter.MappingRenderedAsInstructions:
			groups.RenderedAsInstructions = append(groups.RenderedAsInstructions, item)
		case adapter.MappingIgnored:
			groups.Ignored = append(groups.Ignored, item)
		case adapter.MappingUnsupported:
			groups.Unsupported = append(groups.Unsupported, item)
		default:
			item.Reason = firstNonEmptyString(item.Reason, "adapter returned unknown mapping status "+string(mapping.Status))
			groups.Unsupported = append(groups.Unsupported, item)
		}
	}
	return groups
}

func uniqueSortedStrings(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}

	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func encodeYAML(cmd *cobra.Command, value any) error {
	encoder := yaml.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent(2)
	if err := encoder.Encode(value); err != nil {
		_ = encoder.Close()
		return err
	}
	return encoder.Close()
}

func scopeFromFlag(cmd *cobra.Command) (config.Scope, error) {
	value, err := cmd.Flags().GetString("scope")
	if err != nil {
		return "", err
	}
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

func normalizeStringList(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func isKnownRuntime(runtime string) bool {
	_, ok := config.KnownTargets[runtime]
	return ok
}
