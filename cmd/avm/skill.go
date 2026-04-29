package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/xz1220/agent-vm/internal/config"
	"gopkg.in/yaml.v3"
)

func newSkillCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skill",
		Short: "Inspect installed AVM skills",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newSkillListCommand())
	return cmd
}

func newSkillListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List active or installed AVM skills",
		Args:  cobra.NoArgs,
		RunE:  runSkillList,
	}
	cmd.Flags().Bool("all", false, "list all installed skills instead of the active selection")
	cmd.Flags().Bool("active", false, "list skills selected by the active shell")
	return cmd
}

func runSkillList(cmd *cobra.Command, args []string) error {
	if err := ensureInitialized(); err != nil {
		return err
	}
	all, err := cmd.Flags().GetBool("all")
	if err != nil {
		return err
	}
	active, err := cmd.Flags().GetBool("active")
	if err != nil {
		return err
	}
	if all && active {
		return fmt.Errorf("--all and --active cannot be used together")
	}

	if active || (!all && os.Getenv("AVM_ACTIVE") != "") {
		return runActiveSkillList(cmd)
	}
	return runInstalledSkillList(cmd)
}

func runInstalledSkillList(cmd *cobra.Command) error {
	skills, err := listInstalledSkillOptions()
	if err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	if len(skills) == 0 {
		fmt.Fprintf(out, "no skills installed in %s\n", config.RegistryKindDir("skills"))
		return nil
	}

	fmt.Fprintln(out, "NAME\tSUMMARY\tPATH")
	for _, skill := range skills {
		fmt.Fprintf(out, "%s\t%s\t%s\n", skill.Name, skill.Description, skill.Path)
	}
	return nil
}

func runActiveSkillList(cmd *cobra.Command) error {
	skills, err := listActiveSkillOptions()
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	if len(skills) == 0 {
		fmt.Fprintf(out, "no active skills selected in %s\n", activeDirFromEnv())
		return nil
	}

	fmt.Fprintln(out, "NAME\tSUMMARY\tPATH")
	for _, skill := range skills {
		fmt.Fprintf(out, "%s\t%s\t%s\n", skill.Name, skill.Description, skill.Path)
	}
	return nil
}

type activeSkillManifest struct {
	Skills []string `yaml:"skills"`
}

func listActiveSkillOptions() ([]capabilityOption, error) {
	activeDir := activeDirFromEnv()
	raw, err := os.ReadFile(filepath.Join(activeDir, "manifest.yaml"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var manifest activeSkillManifest
	if err := yaml.Unmarshal(raw, &manifest); err != nil {
		return nil, err
	}
	var options []capabilityOption
	for _, name := range normalizeStringList(manifest.Skills) {
		path := filepath.Join(activeDir, "skills", name, "SKILL.md")
		if _, err := os.Stat(path); err != nil {
			continue
		}
		options = append(options, capabilityOption{
			Name:        name,
			Description: skillDescription(path),
			Path:        path,
		})
	}
	return options, nil
}

func activeDirFromEnv() string {
	if value := os.Getenv("AVM_ACTIVE_DIR"); value != "" {
		return value
	}
	return config.ActiveDir()
}
