package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xz1220/agent-vm/internal/config"
)

func TestPackageAgentExportImportWithReferencedMetadata(t *testing.T) {
	sourceHome := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", sourceHome)
	chdir(t, project)

	writePackageIOFile(t, filepath.Join(sourceHome, ".avm", "registry", "skills", "git", "SKILL.md"), "# Git\n")
	writePackageIOFile(t, filepath.Join(sourceHome, ".avm", "registry", "skills", "git", "meta.yaml"), "name: git\nkind: skill\n")
	writePackageIOFile(t, filepath.Join(sourceHome, ".avm", "registry", "mcps", "github.yaml"), "name: github\nkind: mcp\nserver:\n  env:\n    GITHUB_TOKEN: ${GITHUB_TOKEN}\n")
	writePackageIOFile(t, filepath.Join(sourceHome, ".avm", "memory", "project", "backend-standards.md"), "Prefer small changes.\n")
	if err := config.WritePortableMemory(&config.PortableMemory{
		ID:     "backend-standards",
		Scope:  string(config.ScopeProject),
		Format: "markdown",
		Path:   "~/.avm/memory/project/backend-standards.md",
		Mode:   "read",
	}); err != nil {
		t.Fatalf("write memory metadata: %v", err)
	}
	if err := config.WriteAgent(&config.AgentProfile{
		Name:        "backend-coder",
		SourceScope: string(config.ScopeGlobal),
		Runtime: config.RuntimePreferences{
			Preferred: "codex",
		},
		Capabilities: config.CapabilityRefs{
			Skills: []string{"git"},
			MCPs:   []string{"github"},
		},
		MemoryRefs: []config.MemoryRef{{
			ID:    "backend-standards",
			Scope: string(config.ScopeProject),
			Path:  "~/.avm/memory/project/backend-standards.md",
			Mode:  "read",
		}},
	}, config.ScopeGlobal, project); err != nil {
		t.Fatalf("write agent: %v", err)
	}

	packagePath := filepath.Join(t.TempDir(), "backend-coder.avm.zip")
	if out, err := executeCommand("export", "backend-coder", "--output", packagePath); err != nil {
		t.Fatalf("export returned error: %v\n%s", err, out)
	}

	names := zipEntryNames(t, packagePath)
	for _, want := range []string{
		"manifest.yaml",
		"agents/backend-coder.yaml",
		"memory/project/backend-standards.yaml",
		"memory/project/backend-standards.md",
		"registry/mcps/github.yaml",
		"registry/skills/git/SKILL.md",
		"registry/skills/git/meta.yaml",
	} {
		if !names[want] {
			t.Fatalf("package missing %s; entries: %#v", want, names)
		}
	}

	targetHome := t.TempDir()
	t.Setenv("HOME", targetHome)
	out, err := executeCommand("import", packagePath)
	if err != nil {
		t.Fatalf("import returned error: %v\n%s", err, out)
	}
	if !strings.Contains(out, "imported agent backend-coder: added") {
		t.Fatalf("unexpected import output: %q", out)
	}
	if _, err := config.ReadAgent("backend-coder", config.ScopeGlobal, project); err != nil {
		t.Fatalf("read imported agent: %v", err)
	}
	if _, err := config.ReadPortableMemory("backend-standards", config.ScopeProject); err != nil {
		t.Fatalf("read imported memory metadata: %v", err)
	}
	for _, path := range []string{
		filepath.Join(targetHome, ".avm", "memory", "project", "backend-standards.md"),
		filepath.Join(targetHome, ".avm", "registry", "skills", "git", "SKILL.md"),
		filepath.Join(targetHome, ".avm", "registry", "mcps", "github.yaml"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected imported file %s: %v", path, err)
		}
	}
	if _, err := os.Stat(config.GlobalConfigPath()); !os.IsNotExist(err) {
		t.Fatalf("import should not activate package or write global config, stat err: %v", err)
	}

	out, err = executeCommand("import", packagePath)
	if err != nil {
		t.Fatalf("second import returned error: %v\n%s", err, out)
	}
	if !strings.Contains(out, "added 0, skipped") {
		t.Fatalf("same-content import did not skip: %q", out)
	}
}

func TestPackageEnvExportImportIncludesReferencedAgents(t *testing.T) {
	sourceHome := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", sourceHome)
	chdir(t, project)

	writeTestAgent(t, project, "backend-coder", "codex")
	writeTestAgent(t, project, "backend-reviewer", "claude-code")
	if err := config.WriteEnvironment(&config.Environment{
		Name: "coding",
		RuntimeAgents: map[string]config.RuntimeAgent{
			"codex": {
				Primary:   "backend-coder",
				Available: []string{"backend-coder", "backend-reviewer"},
			},
			"claude-code": {
				Primary: "backend-reviewer",
			},
		},
		Targets: []string{"codex", "claude-code"},
	}); err != nil {
		t.Fatalf("write env: %v", err)
	}

	packagePath := filepath.Join(t.TempDir(), "coding.avm.zip")
	if out, err := executeCommand("export", "coding", "--output", packagePath); err != nil {
		t.Fatalf("env export returned error: %v\n%s", err, out)
	}
	names := zipEntryNames(t, packagePath)
	for _, want := range []string{
		"envs/coding.yaml",
		"agents/backend-coder.yaml",
		"agents/backend-reviewer.yaml",
	} {
		if !names[want] {
			t.Fatalf("env package missing %s; entries: %#v", want, names)
		}
	}

	targetHome := t.TempDir()
	t.Setenv("HOME", targetHome)
	if out, err := executeCommand("import", packagePath); err != nil {
		t.Fatalf("env import returned error: %v\n%s", err, out)
	}
	if _, err := config.ReadEnvironment("coding"); err != nil {
		t.Fatalf("read imported env: %v", err)
	}
	if _, err := config.ReadAgent("backend-reviewer", config.ScopeGlobal, project); err != nil {
		t.Fatalf("read imported referenced agent: %v", err)
	}
	if _, err := os.Stat(config.GlobalConfigPath()); !os.IsNotExist(err) {
		t.Fatalf("import should not activate env or write global config, stat err: %v", err)
	}
}

func TestPackageImportDifferentContentConflict(t *testing.T) {
	sourceHome := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", sourceHome)
	chdir(t, project)
	writeTestAgent(t, project, "backend-coder", "codex")

	packagePath := filepath.Join(t.TempDir(), "backend-coder.avm.zip")
	if out, err := executeCommand("export", "backend-coder", "--output", packagePath); err != nil {
		t.Fatalf("export returned error: %v\n%s", err, out)
	}

	targetHome := t.TempDir()
	t.Setenv("HOME", targetHome)
	writeTestAgent(t, project, "backend-coder", "cline")
	out, err := executeCommand("import", packagePath)
	if err == nil {
		t.Fatalf("expected conflict, got nil error and output %q", out)
	}
	if got, want := err.Error(), "package import conflict: agents/backend-coder.yaml already exists with different content"; got != want {
		t.Fatalf("unexpected conflict error:\n got: %q\nwant: %q", got, want)
	}
}

func writeTestAgent(t *testing.T, project, name, runtime string) {
	t.Helper()
	if err := config.WriteAgent(&config.AgentProfile{
		Name:        name,
		SourceScope: string(config.ScopeGlobal),
		Runtime: config.RuntimePreferences{
			Preferred: runtime,
		},
	}, config.ScopeGlobal, project); err != nil {
		t.Fatalf("write agent %s: %v", name, err)
	}
}

func writePackageIOFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func zipEntryNames(t *testing.T, packagePath string) map[string]bool {
	t.Helper()
	reader, err := zip.OpenReader(packagePath)
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	defer reader.Close()
	names := make(map[string]bool, len(reader.File))
	for _, file := range reader.File {
		names[file.Name] = true
	}
	return names
}
