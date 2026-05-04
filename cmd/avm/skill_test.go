package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xz1220/agent-vm/internal/config"
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

func TestSkillListShowsActiveSkillsInActivatedShell(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	chdir(t, project)
	writeCreateTestSkill(t, "docs")
	writeCreateTestSkill(t, "security")

	if out, err := executeCommand("create", "--from", "default", "--name", "docs-agent", "--skills", "docs", "--runtime", "codex", "--yes"); err != nil {
		t.Fatalf("create returned error: %v\n%s", err, out)
	}
	if out, err := executeCommand("activate", "docs-agent"); err != nil {
		t.Fatalf("activate returned error: %v\n%s", err, out)
	}
	assertFileExistsSkillTest(t, filepath.Join(config.ActiveDir(), "skills", "docs", "SKILL.md"))
	assertFileMissingSkillTest(t, filepath.Join(config.ActiveDir(), "skills", "security", "SKILL.md"))
	runtimeHome := agentRuntimeHomeForTest(t, "docs-agent", "codex")
	assertFileExistsSkillTest(t, filepath.Join(runtimeHome, "skills", "docs", "SKILL.md"))
	assertFileMissingSkillTest(t, filepath.Join(runtimeHome, "skills", "security", "SKILL.md"))

	t.Setenv("AVM_ACTIVE", "profile:docs-agent")
	t.Setenv("AVM_ACTIVE_DIR", config.ActiveDir())
	activeOut, err := executeCommand("skill", "list")
	if err != nil {
		t.Fatalf("skill list returned error: %v\n%s", err, activeOut)
	}
	if !strings.Contains(activeOut, "docs") {
		t.Fatalf("active skill list missing docs:\n%s", activeOut)
	}
	if strings.Contains(activeOut, "security") {
		t.Fatalf("active skill list should not include unselected skill:\n%s", activeOut)
	}

	allOut, err := executeCommand("skill", "list", "--all")
	if err != nil {
		t.Fatalf("skill list --all returned error: %v\n%s", err, allOut)
	}
	if !strings.Contains(allOut, "docs") || !strings.Contains(allOut, "security") {
		t.Fatalf("skill list --all should include registry skills:\n%s", allOut)
	}
}

func assertFileExistsSkillTest(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file %s to exist: %v", path, err)
	}
}

func assertFileMissingSkillTest(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected file %s to be missing, got %v", path, err)
	}
}
