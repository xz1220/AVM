package main

import (
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"
	"github.com/xz1220/agent-vm/internal/config"
)

func newEnvCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "env",
		Short: "Manage AVM runtime environments",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newEnvCreateCommand())
	return cmd
}

func newEnvCreateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Create an AVM runtime environment",
		Args:  validateEnvCreateArgs,
		RunE:  runEnvCreate,
	}
	cmd.Flags().Bool("local", false, "create the environment for the current workspace")
	cmd.Flags().String("codex", "", "agent profile for Codex")
	cmd.Flags().String("claude-code", "", "agent profile for Claude Code")
	cmd.Flags().String("cline", "", "agent profile for Cline")
	cmd.Flags().String("cursor", "", "agent profile for Cursor")
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
	return cobra.ExactArgs(1)(cmd, args)
}

func runEnvCreate(cmd *cobra.Command, args []string) error {
	local, err := cmd.Flags().GetBool("local")
	if err != nil {
		return err
	}

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	runtimeAgents, targets, err := envCreateRuntimeAgents(cmd, !local)
	if err != nil {
		return err
	}
	if err := validateRuntimeAgentProfiles(runtimeAgents, cwd); err != nil {
		return err
	}

	if local {
		extends, err := envCreateLocalExtends(args)
		if err != nil {
			return err
		}
		if _, err := config.ReadEnvironment(extends); err != nil {
			return err
		}
		override := &config.ProjectOverride{
			Extends:       extends,
			RuntimeAgents: runtimeAgents,
		}
		if err := config.WriteProjectOverride(cwd, override); err != nil {
			return err
		}

		fmt.Fprintf(cmd.OutOrStdout(), "created local env override %s\n", extends)
		return nil
	}

	env := &config.Environment{
		Name:          args[0],
		RuntimeAgents: runtimeAgents,
		Targets:       targets,
	}
	if err := config.WriteEnvironment(env); err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "created env %s\n", env.Name)
	return nil
}

func envCreateRuntimeAgents(cmd *cobra.Command, withDefault bool) (map[string]config.RuntimeAgent, []string, error) {
	runtimeAgents := make(map[string]config.RuntimeAgent)
	targets := make([]string, 0, len(envCreateRuntimeOrder))
	for _, runtime := range envCreateRuntimeOrder {
		profile, err := cmd.Flags().GetString(runtime)
		if err != nil {
			return nil, nil, err
		}
		if profile == "" {
			continue
		}
		runtimeAgents[runtime] = config.RuntimeAgent{Primary: profile}
		targets = append(targets, runtime)
	}
	if withDefault && len(runtimeAgents) == 0 {
		runtimeAgents["codex"] = config.RuntimeAgent{Primary: "default"}
		targets = append(targets, "codex")
	}
	return runtimeAgents, targets, nil
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

var envCreateRuntimeOrder = []string{"codex", "claude-code", "cline", "cursor"}
