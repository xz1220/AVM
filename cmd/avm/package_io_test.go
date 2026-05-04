package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xz1220/agent-vm/internal/config"
	"github.com/xz1220/agent-vm/internal/packageio"
)

func TestPackageAgentExportImportWithReferencedMetadata(t *testing.T) {
	sourceHome := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", sourceHome)
	chdir(t, project)

	writePackageIOFile(t, filepath.Join(sourceHome, ".avm", "registry", "skills", "git", "SKILL.md"), "# Git\n")
	writePackageIOFile(t, filepath.Join(sourceHome, ".avm", "registry", "skills", "git", "meta.yaml"), "name: git\nkind: skill\n")
	writePackageIOFile(t, filepath.Join(sourceHome, ".avm", "registry", "mcps", "github.yaml"), "name: github\nkind: mcp\nserver:\n  env:\n    GITHUB_TOKEN: ${GITHUB_TOKEN}\n")
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
	}, config.ScopeGlobal, project); err != nil {
		t.Fatalf("write agent: %v", err)
	}

	packagePath := filepath.Join(t.TempDir(), "backend-coder.avm.zip")
	if out, err := executeCommand("package", "export", "backend-coder", "--output", packagePath); err != nil {
		t.Fatalf("export returned error: %v\n%s", err, out)
	}

	inspectOut, err := executeCommand("package", "inspect", packagePath)
	if err != nil {
		t.Fatalf("package inspect returned error: %v\n%s", err, inspectOut)
	}
	for _, want := range []string{
		"package: agent backend-coder",
		"agents:",
		"  backend-coder",
		"skills:",
		"  git",
		"mcps:",
		"  github",
		"files:",
		"  agents/backend-coder.yaml",
	} {
		if !strings.Contains(inspectOut, want) {
			t.Fatalf("package inspect output missing %q:\n%s", want, inspectOut)
		}
	}

	names := zipEntryNames(t, packagePath)
	for _, want := range []string{
		"manifest.yaml",
		"agents/backend-coder.yaml",
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
	dryRunOut, err := executeCommand("package", "install", "--dry-run", packagePath)
	if err != nil {
		t.Fatalf("install dry-run returned error: %v\n%s", err, dryRunOut)
	}
	for _, want := range []string{
		"install plan for agent backend-coder: add",
		"would add:",
		"agents/backend-coder.yaml",
		"conflict 0",
	} {
		if !strings.Contains(dryRunOut, want) {
			t.Fatalf("install dry-run output missing %q:\n%s", want, dryRunOut)
		}
	}
	if _, err := config.ReadAgent("backend-coder", config.ScopeGlobal, project); !os.IsNotExist(err) {
		t.Fatalf("dry-run should not write agent, err = %v", err)
	}
	if _, err := config.ReadGlobalConfig(); !os.IsNotExist(err) {
		t.Fatalf("dry-run should not lazy initialize global config, err = %v", err)
	}

	out, err := executeCommand("package", "install", packagePath)
	if err != nil {
		t.Fatalf("import returned error: %v\n%s", err, out)
	}
	if !strings.Contains(out, "installed agent backend-coder: added") {
		t.Fatalf("unexpected import output: %q", out)
	}
	if _, err := config.ReadAgent("backend-coder", config.ScopeGlobal, project); err != nil {
		t.Fatalf("read imported agent: %v", err)
	}
	for _, path := range []string{
		filepath.Join(targetHome, ".avm", "registry", "skills", "git", "SKILL.md"),
		filepath.Join(targetHome, ".avm", "registry", "mcps", "github.yaml"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected imported file %s: %v", path, err)
		}
	}
	cfg, err := config.ReadGlobalConfig()
	if err != nil {
		t.Fatalf("import should lazy initialize global config: %v", err)
	}
	if cfg.Active != (config.ActiveRef{Kind: config.ActiveKindProfile, Name: "default"}) {
		t.Fatalf("import should not activate package, active = %#v", cfg.Active)
	}

	out, err = executeCommand("package", "install", packagePath)
	if err != nil {
		t.Fatalf("second install returned error: %v\n%s", err, out)
	}
	if !strings.Contains(out, "added 0, skipped") {
		t.Fatalf("same-content install did not skip: %q", out)
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
	if out, err := executeCommand("package", "export", "coding", "--kind", "env", "--output", packagePath); err != nil {
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
	if out, err := executeCommand("package", "install", packagePath); err != nil {
		t.Fatalf("env install returned error: %v\n%s", err, out)
	}
	if _, err := config.ReadEnvironment("coding"); err != nil {
		t.Fatalf("read imported env: %v", err)
	}
	if _, err := config.ReadAgent("backend-reviewer", config.ScopeGlobal, project); err != nil {
		t.Fatalf("read imported referenced agent: %v", err)
	}
	cfg, err := config.ReadGlobalConfig()
	if err != nil {
		t.Fatalf("import should lazy initialize global config: %v", err)
	}
	if cfg.Active != (config.ActiveRef{Kind: config.ActiveKindProfile, Name: "default"}) {
		t.Fatalf("import should not activate env, active = %#v", cfg.Active)
	}
}

func TestPackageExportDefaultDoesNotExportEnv(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	chdir(t, project)

	writeTestAgent(t, project, "backend-coder", "codex")
	if err := config.WriteEnvironment(&config.Environment{
		Name: "coding",
		RuntimeAgents: map[string]config.RuntimeAgent{
			"codex": {
				Primary: "backend-coder",
			},
		},
		Targets: []string{"codex"},
	}); err != nil {
		t.Fatalf("write env: %v", err)
	}

	packagePath := filepath.Join(t.TempDir(), "coding.avm.zip")
	out, err := executeCommand("package", "export", "coding", "--output", packagePath)
	if err == nil {
		t.Fatalf("expected default export to reject env, got nil error and output %q", out)
	}
	if got, want := err.Error(), `package export target "coding" not found as agent`; got != want {
		t.Fatalf("unexpected export error:\n got: %q\nwant: %q", got, want)
	}
}

func TestPackageInstall(t *testing.T) {
	sourceHome := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", sourceHome)
	chdir(t, project)
	writeTestAgent(t, project, "backend-coder", "codex")

	packagePath := filepath.Join(t.TempDir(), "backend-coder.avm.zip")
	if out, err := executeCommand("package", "export", "backend-coder", "--output", packagePath); err != nil {
		t.Fatalf("export returned error: %v\n%s", err, out)
	}

	targetHome := t.TempDir()
	t.Setenv("HOME", targetHome)
	out, err := executeCommand("package", "install", packagePath)
	if err != nil {
		t.Fatalf("install returned error: %v\n%s", err, out)
	}
	if !strings.Contains(out, "installed agent backend-coder: added") {
		t.Fatalf("unexpected install output: %q", out)
	}
	if _, err := config.ReadAgent("backend-coder", config.ScopeGlobal, project); err != nil {
		t.Fatalf("read installed agent: %v", err)
	}
}

func TestPackageInstallRegeneratesConflictingAgentID(t *testing.T) {
	sourceHome := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", sourceHome)
	chdir(t, project)
	writeTestAgent(t, project, "backend-coder", "codex")
	sourceAgent, err := config.ReadAgent("backend-coder", config.ScopeGlobal, project)
	if err != nil {
		t.Fatalf("read source agent: %v", err)
	}

	packagePath := filepath.Join(t.TempDir(), "backend-coder.avm.zip")
	if out, err := executeCommand("package", "export", "backend-coder", "--output", packagePath); err != nil {
		t.Fatalf("export returned error: %v\n%s", err, out)
	}

	targetHome := t.TempDir()
	t.Setenv("HOME", targetHome)
	if err := config.WriteAgent(&config.AgentProfile{
		Name:        "existing-agent",
		ID:          sourceAgent.ID,
		SourceScope: string(config.ScopeGlobal),
		Runtime: config.RuntimePreferences{
			Preferred: "codex",
		},
	}, config.ScopeGlobal, project); err != nil {
		t.Fatalf("write existing agent: %v", err)
	}

	out, err := executeCommand("package", "install", packagePath)
	if err != nil {
		t.Fatalf("install returned error: %v\n%s", err, out)
	}
	if !strings.Contains(out, "installed agent backend-coder: added") {
		t.Fatalf("unexpected install output: %q", out)
	}
	installed, err := config.ReadAgent("backend-coder", config.ScopeGlobal, project)
	if err != nil {
		t.Fatalf("read installed agent: %v", err)
	}
	if installed.ID == sourceAgent.ID {
		t.Fatalf("installed agent reused conflicting id %q", installed.ID)
	}
}

func TestPackageImportDifferentContentConflict(t *testing.T) {
	sourceHome := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", sourceHome)
	chdir(t, project)
	writeTestAgent(t, project, "backend-coder", "codex")

	packagePath := filepath.Join(t.TempDir(), "backend-coder.avm.zip")
	if out, err := executeCommand("package", "export", "backend-coder", "--output", packagePath); err != nil {
		t.Fatalf("export returned error: %v\n%s", err, out)
	}

	targetHome := t.TempDir()
	t.Setenv("HOME", targetHome)
	writeTestAgent(t, project, "backend-coder", "cline")

	dryRunOut, err := executeCommand("package", "install", "--dry-run", packagePath)
	if err != nil {
		t.Fatalf("conflict dry-run returned error: %v\n%s", err, dryRunOut)
	}
	for _, want := range []string{
		"install plan for agent backend-coder: add 0, skip 0, conflict 1",
		"conflicts:",
		"agents/backend-coder.yaml",
	} {
		if !strings.Contains(dryRunOut, want) {
			t.Fatalf("conflict dry-run output missing %q:\n%s", want, dryRunOut)
		}
	}
	agent, err := config.ReadAgent("backend-coder", config.ScopeGlobal, project)
	if err != nil {
		t.Fatalf("read dry-run target agent: %v", err)
	}
	if agent.Runtime.Preferred != "cline" {
		t.Fatalf("dry-run should not overwrite target agent, runtime = %q", agent.Runtime.Preferred)
	}

	out, err := executeCommand("package", "install", packagePath)
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

func TestPackageInstallFromURL(t *testing.T) {
	sourceHome := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", sourceHome)
	chdir(t, project)

	writeTestAgent(t, project, "backend-coder", "codex")

	packagePath := filepath.Join(t.TempDir(), "backend-coder.avm.zip")
	if out, err := executeCommand("package", "export", "backend-coder", "--output", packagePath); err != nil {
		t.Fatalf("export returned error: %v\n%s", err, out)
	}

	zipData, err := os.ReadFile(packagePath)
	if err != nil {
		t.Fatalf("read package: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(zipData)
	}))
	defer srv.Close()

	targetHome := t.TempDir()
	t.Setenv("HOME", targetHome)
	httpClient = srv.Client()
	defer func() { httpClient = nil }()

	out, err := executeCommand("package", "install", srv.URL+"/backend-coder.avm.zip")
	if err != nil {
		t.Fatalf("install from URL returned error: %v\n%s", err, out)
	}
	if !strings.Contains(out, "downloading") {
		t.Fatalf("output missing download message:\n%s", out)
	}
	if !strings.Contains(out, "installed agent backend-coder: added") {
		t.Fatalf("unexpected install output:\n%s", out)
	}
	if _, err := config.ReadAgent("backend-coder", config.ScopeGlobal, project); err != nil {
		t.Fatalf("read installed agent: %v", err)
	}
}

func TestPackageInstallFromURLWithChecksum(t *testing.T) {
	sourceHome := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", sourceHome)
	chdir(t, project)

	writeTestAgent(t, project, "backend-coder", "codex")

	packagePath := filepath.Join(t.TempDir(), "backend-coder.avm.zip")
	if out, err := executeCommand("package", "export", "backend-coder", "--output", packagePath); err != nil {
		t.Fatalf("export returned error: %v\n%s", err, out)
	}

	zipData, err := os.ReadFile(packagePath)
	if err != nil {
		t.Fatalf("read package: %v", err)
	}
	h := sha256.Sum256(zipData)
	correctChecksum := "sha256:" + hex.EncodeToString(h[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(zipData)
	}))
	defer srv.Close()

	httpClient = srv.Client()
	defer func() { httpClient = nil }()

	targetHome := t.TempDir()
	t.Setenv("HOME", targetHome)
	out, err := executeCommand("package", "install", "--checksum", correctChecksum, srv.URL+"/pkg.avm.zip")
	if err != nil {
		t.Fatalf("install with correct checksum returned error: %v\n%s", err, out)
	}
	if !strings.Contains(out, "installed agent backend-coder") {
		t.Fatalf("unexpected output:\n%s", out)
	}

	t.Setenv("HOME", t.TempDir())
	_, err = executeCommand("package", "install", "--checksum", "sha256:0000000000000000000000000000000000000000000000000000000000000000", srv.URL+"/pkg.avm.zip")
	if err == nil {
		t.Fatal("expected checksum mismatch error")
	}
	if !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPackageInstallFromURLNotFound(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()

	home := t.TempDir()
	t.Setenv("HOME", home)
	chdir(t, t.TempDir())

	httpClient = srv.Client()
	defer func() { httpClient = nil }()

	_, err := executeCommand("package", "install", srv.URL+"/missing.avm.zip")
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if !strings.Contains(err.Error(), "HTTP 404") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func setTestHTTPClient(c packageio.HTTPClient) func() {
	httpClient = c
	return func() { httpClient = nil }
}
