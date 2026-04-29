package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xz1220/agent-vm/internal/adapter"
	"github.com/xz1220/agent-vm/internal/config"
)

func TestCreateFromBuiltinPackageWithYesLazyInitializes(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	chdir(t, project)

	out, err := executeCommand("create", "backend-coder", "--name", "api-coder", "--runtime", "codex", "--yes")
	if err != nil {
		t.Fatalf("create returned error: %v\n%s", err, out)
	}
	for _, want := range []string{
		"created agent api-coder from package backend-coder",
		`eval "$(avm activate api-coder)"`,
		"codex",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("create output missing %q:\n%s", want, out)
		}
	}

	if _, err := config.ReadGlobalConfig(); err != nil {
		t.Fatalf("create should lazy initialize global config: %v", err)
	}
	agent, err := config.ReadAgent("api-coder", config.ScopeGlobal, project)
	if err != nil {
		t.Fatalf("read created agent: %v", err)
	}
	if agent.Runtime.Preferred != "codex" {
		t.Fatalf("runtime = %q, want codex", agent.Runtime.Preferred)
	}
	if !containsCreateTestString(agent.Capabilities.Skills, "git") || !containsCreateTestString(agent.Capabilities.Skills, "test") {
		t.Fatalf("unexpected skills: %#v", agent.Capabilities.Skills)
	}
}

func TestCreateWithMultipleRuntimesActivatesAll(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	chdir(t, project)

	out, err := executeCommand("create", "backend-coder", "--name", "multi-coder", "--runtimes", "codex,opencode", "--yes")
	if err != nil {
		t.Fatalf("create returned error: %v\n%s", err, out)
	}
	if !strings.Contains(out, "codex") || !strings.Contains(out, "opencode") {
		t.Fatalf("create output should mention selected runtimes:\n%s", out)
	}
	agent, err := config.ReadAgent("multi-coder", config.ScopeGlobal, project)
	if err != nil {
		t.Fatalf("read created agent: %v", err)
	}
	if agent.Runtime.Preferred != "codex" {
		t.Fatalf("preferred runtime = %q, want codex", agent.Runtime.Preferred)
	}
	if strings.Join(agent.Runtime.Fallback, ",") != "opencode" {
		t.Fatalf("fallback runtimes = %#v, want opencode", agent.Runtime.Fallback)
	}

	activateOut, err := executeCommand("activate", "multi-coder")
	if err != nil {
		t.Fatalf("activate returned error: %v\n%s", err, activateOut)
	}
	for _, want := range []string{"CODEX_HOME", "OPENCODE_CONFIG", "OPENCODE_CONFIG_DIR"} {
		if !strings.Contains(activateOut, want) {
			t.Fatalf("activate output missing %q:\n%s", want, activateOut)
		}
	}
}

func TestCreateInteractiveDefaults(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	chdir(t, project)

	cmd := newRootCommand()
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader("\n\n\n\n"))
	cmd.SetArgs([]string{"create", "backend-coder"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("interactive create returned error: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "Agent name [backend-coder]") {
		t.Fatalf("interactive output missing name prompt:\n%s", out.String())
	}
	if _, err := config.ReadAgent("backend-coder", config.ScopeGlobal, project); err != nil {
		t.Fatalf("read interactive agent: %v", err)
	}
}

func TestCreateFromExistingProfileWithYes(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	chdir(t, project)

	out, err := executeCommand("create", "--from", "default", "--name", "default-opencode", "--runtime", "opencode", "--yes")
	if err != nil {
		t.Fatalf("create from profile returned error: %v\n%s", err, out)
	}
	if !strings.Contains(out, "created agent default-opencode from global profile default") {
		t.Fatalf("unexpected create output:\n%s", out)
	}

	agent, err := config.ReadAgent("default-opencode", config.ScopeGlobal, project)
	if err != nil {
		t.Fatalf("read cloned agent: %v", err)
	}
	if agent.Runtime.Preferred != "opencode" {
		t.Fatalf("runtime = %q, want opencode", agent.Runtime.Preferred)
	}
	if agent.Identity.DisplayName != "default-opencode" {
		t.Fatalf("display name = %q, want default-opencode", agent.Identity.DisplayName)
	}
}

func TestCreateFromImportCandidateWithYes(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	chdir(t, project)

	if _, err := executeCommand("init"); err != nil {
		t.Fatalf("init returned error: %v", err)
	}
	report := initImportReport{
		Version:     initImportReportVersion,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Runtimes: []initRuntimeImportReport{
			{
				Runtime: "claude-code",
				Found:   true,
				AgentCandidates: []adapter.ImportedAgent{
					{
						Name:        "global-reviewer",
						Description: "Review existing changes",
						Instructions: adapter.Instructions{
							Developer: "Review for correctness.",
						},
					},
				},
			},
		},
	}
	if err := saveInitImportReport(initImportReportPath(), report); err != nil {
		t.Fatalf("save import report: %v", err)
	}

	out, err := executeCommand("create", "--from-import", "claude-code/global-reviewer", "--name", "reviewer-copy", "--runtime", "opencode", "--yes")
	if err != nil {
		t.Fatalf("create from import returned error: %v\n%s", err, out)
	}
	if !strings.Contains(out, "created agent reviewer-copy from claude-code import candidate global-reviewer") {
		t.Fatalf("unexpected create output:\n%s", out)
	}

	agent, err := config.ReadAgent("reviewer-copy", config.ScopeGlobal, project)
	if err != nil {
		t.Fatalf("read imported agent: %v", err)
	}
	if agent.Runtime.Preferred != "opencode" {
		t.Fatalf("runtime = %q, want opencode", agent.Runtime.Preferred)
	}
	if agent.Instructions.Developer != "Review for correctness." {
		t.Fatalf("developer instructions = %q", agent.Instructions.Developer)
	}
}

func TestCreateInteractiveSelectsInstalledSkills(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	chdir(t, project)
	writeCreateTestSkill(t, "docs")
	writeCreateTestSkill(t, "git")
	writeCreateTestSkill(t, "security")

	cmd := newRootCommand()
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader("\n\n1,3\n\n"))
	cmd.SetArgs([]string{"create", "backend-coder", "--name", "skill-picker"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("interactive create returned error: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "Skills installed in") ||
		!strings.Contains(out.String(), "docs - Use docs in test scenarios.") ||
		!strings.Contains(out.String(), "Preview:") {
		t.Fatalf("interactive output missing skill picker or preview:\n%s", out.String())
	}

	agent, err := config.ReadAgent("skill-picker", config.ScopeGlobal, project)
	if err != nil {
		t.Fatalf("read interactive agent: %v", err)
	}
	if !containsCreateTestString(agent.Capabilities.Skills, "docs") || !containsCreateTestString(agent.Capabilities.Skills, "security") {
		t.Fatalf("unexpected skills: %#v", agent.Capabilities.Skills)
	}
	if containsCreateTestString(agent.Capabilities.Skills, "git") || containsCreateTestString(agent.Capabilities.Skills, "test") {
		t.Fatalf("interactive selection should replace default skills, got %#v", agent.Capabilities.Skills)
	}
}

func TestCreateInteractiveCanStartFromExistingProfile(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	chdir(t, project)

	cmd := newRootCommand()
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader("4\nscenario-api\n\n\n"))
	cmd.SetArgs([]string{"create"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("interactive create from profile returned error: %v\n%s", err, out.String())
	}
	for _, want := range []string{"Create from:", "global profile default", "source: global profile default"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("interactive output missing %q:\n%s", want, out.String())
		}
	}

	agent, err := config.ReadAgent("scenario-api", config.ScopeGlobal, project)
	if err != nil {
		t.Fatalf("read created agent: %v", err)
	}
	if agent.Runtime.Preferred != "codex" {
		t.Fatalf("runtime = %q, want codex", agent.Runtime.Preferred)
	}
}

func TestPackageListShow(t *testing.T) {
	listOut, err := executeCommand("package", "list")
	if err != nil {
		t.Fatalf("package list returned error: %v", err)
	}
	if !strings.Contains(listOut, "backend-coder") || !strings.Contains(listOut, "reviewer") {
		t.Fatalf("package list missing builtins:\n%s", listOut)
	}

	showOut, err := executeCommand("package", "show", "backend-coder")
	if err != nil {
		t.Fatalf("package show returned error: %v", err)
	}
	for _, want := range []string{"name: backend-coder", "modes:", "default_runtime: codex"} {
		if !strings.Contains(showOut, want) {
			t.Fatalf("package show missing %q:\n%s", want, showOut)
		}
	}
}

func TestMergeSelectionOptionsKeepsInstalledFirst(t *testing.T) {
	got := mergeSelectionOptions([]string{"docs", "security"}, []string{"git", "test"})
	want := []string{"docs", "security", "git", "test"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("mergeSelectionOptions = %#v, want %#v", got, want)
	}
}

func writeCreateTestSkill(t *testing.T, name string) {
	t.Helper()
	dir := config.SkillRegistryPath(name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	body := "# " + name + "\n\nUse " + name + " in test scenarios.\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o600); err != nil {
		t.Fatalf("write skill: %v", err)
	}
}

func containsCreateTestString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
