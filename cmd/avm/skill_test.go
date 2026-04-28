package main

import (
	"strings"
	"testing"
)

func TestSkillListShowsInstalledSkills(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	chdir(t, project)
	writeCreateTestSkill(t, "docs")
	writeCreateTestSkill(t, "security")

	out, err := executeCommand("skill", "list")
	if err != nil {
		t.Fatalf("skill list returned error: %v\n%s", err, out)
	}
	for _, want := range []string{"NAME\tSUMMARY\tPATH", "docs", "Use docs in test scenarios.", "security", "SKILL.md"} {
		if !strings.Contains(out, want) {
			t.Fatalf("skill list output missing %q:\n%s", want, out)
		}
	}
}
