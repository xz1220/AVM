package main

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/xz1220/agent-vm/internal/config"
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
	return &cobra.Command{
		Use:   "list",
		Short: "List installed AVM skills",
		Args:  cobra.NoArgs,
		RunE:  runSkillList,
	}
}

func runSkillList(cmd *cobra.Command, args []string) error {
	if err := ensureInitialized(); err != nil {
		return err
	}
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
