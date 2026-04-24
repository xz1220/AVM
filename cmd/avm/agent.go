package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/xz1220/agent-vm/internal/config"
	"gopkg.in/yaml.v3"
)

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
		newAgentListCommand(),
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
	cmd.Flags().StringSlice("memory", nil, "portable memory refs to attach")
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
	memoryValues, err := cmd.Flags().GetStringSlice("memory")
	if err != nil {
		return err
	}
	memoryRefs, err := parseMemoryRefs(memoryValues)
	if err != nil {
		return err
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
		MemoryRefs: memoryRefs,
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

	agent, err := config.ReadAgent(args[0], scope, cwd)
	if err != nil {
		return err
	}
	encoder := yaml.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent(2)
	if err := encoder.Encode(agent); err != nil {
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

func parseMemoryRefs(values []string) ([]config.MemoryRef, error) {
	values = normalizeStringList(values)
	refs := make([]config.MemoryRef, 0, len(values))
	for _, value := range values {
		parts := strings.Split(value, ":")
		if len(parts) > 4 {
			return nil, fmt.Errorf("invalid memory ref %q", value)
		}

		id := strings.TrimSpace(parts[0])
		scope := string(config.ScopeProject)
		path := ""
		mode := "read"
		if len(parts) > 1 && strings.TrimSpace(parts[1]) != "" {
			scope = strings.TrimSpace(parts[1])
		}
		if len(parts) > 2 {
			path = strings.TrimSpace(parts[2])
		}
		if len(parts) > 3 && strings.TrimSpace(parts[3]) != "" {
			mode = strings.TrimSpace(parts[3])
		}
		if path == "" {
			path = config.MemoryPath(id, config.Scope(scope))
		}
		refs = append(refs, config.MemoryRef{
			ID:    id,
			Scope: scope,
			Path:  path,
			Mode:  mode,
		})
	}
	return refs, nil
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
