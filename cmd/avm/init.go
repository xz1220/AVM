package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/xz1220/agent-vm/internal/config"
)

func newInitCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize an AVM home directory",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := initAVMHome(); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "initialized avm home")
			return nil
		},
	}
	return cmd
}

func initAVMHome() error {
	dirs := []string{
		config.AvmDir(),
		config.AgentsDir(),
		config.EnvsDir(),
		config.RegistryDir(),
		config.RegistryKindDir("skills"),
		config.RegistryKindDir("mcps"),
		config.MemoryDir(),
		config.MemoryScopeDir(config.ScopeUser),
		config.MemoryScopeDir(config.ScopeProject),
		config.MemoryScopeDir(config.ScopeLocal),
		config.MemoryScopeDir(config.ScopeTeam),
		config.ActiveDir(),
		config.StateDir(),
		config.BackupDir(),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}

	if err := config.WriteGlobalConfig(defaultGlobalConfig()); err != nil {
		return err
	}
	if err := config.WriteAgent(defaultAgentProfile(), config.ScopeGlobal, ""); err != nil {
		return err
	}
	return config.WriteEnvironment(defaultEnvironment())
}

func defaultGlobalConfig() *config.GlobalConfig {
	cfg := &config.GlobalConfig{
		Version: "1",
		Active: config.ActiveRef{
			Kind: config.ActiveKindProfile,
			Name: "default",
		},
		Defaults: config.DefaultsConfig{
			SourceScope:      string(config.ScopeGlobal),
			Targets:          []string{"claude-code", "codex", "cline"},
			ConflictStrategy: "prompt",
		},
		Settings: config.Settings{
			BackupEnabled:  true,
			BackupMaxCount: 10,
			WriteMode:      "managed-only",
			ShellPrompt: config.ShellPromptSettings{
				Enabled: true,
				Format:  "avm:%s",
			},
		},
	}
	cfg.ApplyDefaults()
	return cfg
}

func defaultAgentProfile() *config.AgentProfile {
	agent := &config.AgentProfile{
		Name: "default",
		Runtime: config.RuntimePreferences{
			Preferred: "codex",
		},
	}
	agent.ApplyDefaults(string(config.ScopeGlobal))
	return agent
}

func defaultEnvironment() *config.Environment {
	env := &config.Environment{
		Name: "default",
		RuntimeAgents: map[string]config.RuntimeAgent{
			"claude-code": {Primary: "default"},
			"codex":       {Primary: "default"},
			"cline":       {Primary: "default"},
		},
		Targets: []string{"claude-code", "codex", "cline"},
	}
	env.ApplyDefaults()
	return env
}
