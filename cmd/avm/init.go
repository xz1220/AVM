package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/xz1220/agent-vm/internal/config"
	"github.com/xz1220/agent-vm/internal/state"
)

func newInitCommand() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize an AVM home directory",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := initAVMHome(force); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "initialized avm home")
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "rebuild default AVM config, agent, and env")
	return cmd
}

func initAVMHome(force bool) error {
	if !force {
		if _, err := os.Stat(config.GlobalConfigPath()); err == nil {
			return fmt.Errorf("avm home already initialized; use --force to rebuild defaults")
		} else if !os.IsNotExist(err) {
			return err
		}
	}

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
		cacheDir(),
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
	if err := config.WriteEnvironment(defaultEnvironment()); err != nil {
		return err
	}
	if err := state.SaveSyncState(syncStatePath(), state.NewSyncState(defaultGlobalConfig().Active)); err != nil {
		return err
	}
	return refreshInitImportReport()
}

func cacheDir() string {
	return filepath.Join(config.AvmDir(), "cache")
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
