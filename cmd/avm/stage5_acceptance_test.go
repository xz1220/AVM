package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/xz1220/agent-vm/internal/config"
	"github.com/xz1220/agent-vm/internal/state"
)

func TestStage5AcceptanceSmokeFlow(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	runtimeRoot := t.TempDir()
	binDir := filepath.Join(t.TempDir(), "bin")
	mkdirAll(t, binDir)

	t.Setenv("HOME", home)
	t.Setenv("PATH", binDir)

	codexHome := filepath.Join(runtimeRoot, "codex-home")
	claudeConfigDir := filepath.Join(runtimeRoot, "claude-config")
	clineDataHome := filepath.Join(runtimeRoot, "cline-data")
	for _, dir := range []string{
		codexHome,
		claudeConfigDir,
		clineDataHome,
		filepath.Join(project, ".cursor"),
	} {
		mkdirAll(t, dir)
	}
	t.Setenv("CODEX_HOME", codexHome)
	t.Setenv("CLAUDE_CONFIG_DIR", claudeConfigDir)
	t.Setenv("CLINE_DATA_HOME", clineDataHome)
	chdir(t, project)
	project = currentWorkingDir(t)

	initOut, err := executeCommand("init")
	if err != nil {
		t.Fatalf("init returned error: %v", err)
	}
	assertContains(t, initOut, "initialized avm home")
	assertPathExists(t, filepath.Join(home, ".avm", "config.yaml"))
	assertPathExists(t, filepath.Join(home, ".avm", "agents", "default.yaml"))
	assertPathExists(t, filepath.Join(home, ".avm", "envs", "default.yaml"))
	assertPathMissing(t, filepath.Join(project, ".avm"))
	assertNoRegularFiles(t, claudeConfigDir)
	assertManagedRuntimePathsMissing(t, acceptanceRuntimePaths{
		codexHome: codexHome,
		clineHome: clineDataHome,
		project:   project,
	})

	createAcceptanceAgent(t, "codex-agent", "codex", "--model", "gpt-5.4", "--reasoning", "medium", "--skills", "test", "--mcps", "github", "--memory", "acceptance-standards:project:/memory/acceptance-standards.md:read")
	createAcceptanceAgent(t, "claude-agent", "claude-code", "--model", "claude-sonnet", "--reasoning", "medium", "--skills", "test")
	createAcceptanceAgent(t, "cline-agent", "cline", "--model", "cline-model", "--reasoning", "medium", "--skills", "test")
	createAcceptanceAgent(t, "cursor-agent", "cursor", "--model", "cursor-model", "--reasoning", "medium", "--skills", "test")

	listOut, err := executeCommand("agent", "list")
	if err != nil {
		t.Fatalf("agent list returned error: %v", err)
	}
	for _, want := range []string{"codex-agent", "claude-agent", "cline-agent", "cursor-agent"} {
		assertContains(t, listOut, want)
	}

	showOut, err := executeCommand("agent", "show", "codex-agent")
	if err != nil {
		t.Fatalf("agent show returned error: %v", err)
	}
	for _, want := range []string{"name: codex-agent", "preferred: codex", "model: gpt-5.4", "id: acceptance-standards"} {
		assertContains(t, showOut, want)
	}

	envOut, err := executeCommand(
		"env", "create", "all-runtimes",
		"--codex", "codex-agent",
		"--claude-code", "claude-agent",
		"--cline", "cline-agent",
		"--cursor", "cursor-agent",
	)
	if err != nil {
		t.Fatalf("env create returned error: %v", err)
	}
	assertContains(t, envOut, "created env all-runtimes")

	memoryBefore := listRegularFiles(t, filepath.Join(home, ".avm", "memory"))
	memorySource := acceptanceTestdataPath(t, "memory", "acceptance-standards.md")
	memoryOut, err := executeCommand("memory", "import", "--from", memorySource, "--dry-run", "--format", "json")
	if err != nil {
		t.Fatalf("memory import dry-run returned error: %v", err)
	}
	for _, want := range []string{`"dry_run": true`, `"id": "acceptance-standards"`, `"status": "new"`} {
		assertContains(t, memoryOut, want)
	}
	memoryAfter := listRegularFiles(t, filepath.Join(home, ".avm", "memory"))
	if strings.Join(memoryAfter, "\n") != strings.Join(memoryBefore, "\n") {
		t.Fatalf("memory import dry-run wrote formal memory files:\nbefore: %#v\nafter: %#v", memoryBefore, memoryAfter)
	}
	assertManagedRuntimePathsMissing(t, acceptanceRuntimePaths{
		codexHome: codexHome,
		clineHome: clineDataHome,
		project:   project,
	})
	assertNoRegularFiles(t, claudeConfigDir)

	useOut, err := executeCommand("use", "--kind", "env", "all-runtimes")
	if err != nil {
		t.Fatalf("use returned error: %v\noutput:\n%s", err, useOut)
	}
	for _, want := range []string{
		"active: env:all-runtimes",
		"  codex: synced",
		"  claude-code: synced",
		"  cline: synced",
		"  cursor: synced",
	} {
		assertContains(t, useOut, want)
	}

	paths := acceptanceRuntimePaths{
		codexHome: codexHome,
		clineHome: clineDataHome,
		project:   project,
	}
	for _, path := range []string{
		paths.codexConfig(),
		paths.codexAgent("codex-agent"),
		paths.claudeAgent("claude-agent"),
		paths.clineRules("cline-agent"),
		paths.cursorRule("cursor-agent"),
	} {
		assertPathExists(t, path)
	}

	syncState := readAcceptanceSyncState(t)
	if syncState.LastActive != (config.ActiveRef{Kind: config.ActiveKindEnv, Name: "all-runtimes"}) {
		t.Fatalf("unexpected sync-state active: %#v", syncState.LastActive)
	}
	assertSyncedRuntime(t, syncState, "codex", "codex-agent", paths.codexConfig(), paths.codexAgent("codex-agent"))
	assertSyncedRuntime(t, syncState, "claude-code", "claude-agent", paths.claudeAgent("claude-agent"))
	assertSyncedRuntime(t, syncState, "cline", "cline-agent", paths.clineRules("cline-agent"))
	assertSyncedRuntime(t, syncState, "cursor", "cursor-agent", paths.cursorRule("cursor-agent"))

	statusOut, err := executeCommand("status")
	if err != nil {
		t.Fatalf("status returned error: %v", err)
	}
	for _, want := range []string{
		"active: env:all-runtimes",
		"  codex: synced (agent codex-agent)",
		"  claude-code: synced (agent claude-agent)",
		"  cline: synced (agent cline-agent)",
		"  cursor: synced (agent cursor-agent)",
		"managed paths:",
		"mapping status:",
	} {
		assertContains(t, statusOut, want)
	}

	deactivateOut, err := executeCommand("deactivate")
	if err != nil {
		t.Fatalf("deactivate returned error: %v\noutput:\n%s", err, deactivateOut)
	}
	assertContains(t, deactivateOut, "active: profile:default")
	assertCurrentActive(t, "profile:default")
}

func TestStage5AcceptanceSelectedGaps(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	chdir(t, project)

	if _, err := executeCommand("init"); err != nil {
		t.Fatalf("init returned error: %v", err)
	}

	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "env local not implemented",
			args: []string{"env", "create", "workspace", "--local", "--codex", "default"},
			want: "avm env create --local is not supported yet",
		},
		{
			name: "shell init not implemented",
			args: []string{"shell", "init", "zsh"},
			want: "avm shell init: not implemented",
		},
		{
			name: "sync command absent",
			args: []string{"sync"},
			want: `unknown command "sync"`,
		},
		{
			name: "export command absent",
			args: []string{"export", "default"},
			want: `unknown command "export"`,
		},
		{
			name: "import command absent",
			args: []string{"import", "bundle.avm.zip"},
			want: `unknown command "import"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := executeCommand(tt.args...)
			if err == nil {
				t.Fatal("expected error")
			}
			if got := err.Error(); !strings.Contains(got, tt.want) {
				t.Fatalf("unexpected error:\n got: %q\nwant to contain: %q", got, tt.want)
			}
		})
	}
}

type acceptanceRuntimePaths struct {
	codexHome string
	clineHome string
	project   string
}

func (p acceptanceRuntimePaths) codexConfig() string {
	return filepath.Join(p.codexHome, "config.toml")
}

func (p acceptanceRuntimePaths) codexAgent(name string) string {
	return filepath.Join(p.codexHome, "agents", name+".toml")
}

func (p acceptanceRuntimePaths) claudeAgent(name string) string {
	return filepath.Join(p.project, ".claude", "agents", name+".md")
}

func (p acceptanceRuntimePaths) clineRules(name string) string {
	return filepath.Join(p.project, ".clinerules", "avm", name+".md")
}

func (p acceptanceRuntimePaths) cursorRule(name string) string {
	return filepath.Join(p.project, ".cursor", "rules", "avm-"+name+".md")
}

func (p acceptanceRuntimePaths) clineMCPSettings() string {
	return filepath.Join(p.clineHome, "settings", "cline_mcp_settings.json")
}

func assertManagedRuntimePathsMissing(t *testing.T, paths acceptanceRuntimePaths) {
	t.Helper()
	for _, path := range []string{
		paths.codexConfig(),
		paths.codexAgent("codex-agent"),
		paths.claudeAgent("claude-agent"),
		paths.clineRules("cline-agent"),
		paths.cursorRule("cursor-agent"),
		paths.clineMCPSettings(),
	} {
		assertPathMissing(t, path)
	}
}

func createAcceptanceAgent(t *testing.T, name, runtime string, extraArgs ...string) {
	t.Helper()
	args := []string{"agent", "create", name, "--runtime", runtime}
	args = append(args, extraArgs...)
	out, err := executeCommand(args...)
	if err != nil {
		t.Fatalf("agent create %s returned error: %v", name, err)
	}
	assertContains(t, out, "created agent "+name)
}

func readAcceptanceSyncState(t *testing.T) state.SyncState {
	t.Helper()
	data, err := os.ReadFile(syncStatePath())
	if err != nil {
		t.Fatalf("read sync-state: %v", err)
	}
	var syncState state.SyncState
	if err := json.Unmarshal(data, &syncState); err != nil {
		t.Fatalf("unmarshal sync-state: %v", err)
	}
	return syncState
}

func assertSyncedRuntime(t *testing.T, syncState state.SyncState, runtime, agent string, managedPaths ...string) {
	t.Helper()
	runtimeState, ok := syncState.Runtimes[runtime]
	if !ok {
		t.Fatalf("sync-state missing runtime %q: %#v", runtime, syncState.Runtimes)
	}
	if runtimeState.Status != state.RuntimeStatusSynced {
		t.Fatalf("%s status = %q, want synced; state=%#v", runtime, runtimeState.Status, runtimeState)
	}
	if runtimeState.AgentName != agent {
		t.Fatalf("%s agent = %q, want %q", runtime, runtimeState.AgentName, agent)
	}
	if len(runtimeState.Mappings) == 0 {
		t.Fatalf("%s has no mapping state", runtime)
	}

	seen := make(map[string]bool, len(runtimeState.ManagedPaths))
	for _, managed := range runtimeState.ManagedPaths {
		seen[filepath.Clean(managed.Path)] = true
	}
	for _, path := range managedPaths {
		if !seen[filepath.Clean(path)] {
			t.Fatalf("%s managed paths missing %s: %#v", runtime, path, runtimeState.ManagedPaths)
		}
	}
}

func listRegularFiles(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	sort.Strings(files)
	return files
}

func assertPathExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected path %s: %v", path, err)
	}
}

func assertPathMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected path %s to be missing, stat err: %v", path, err)
	}
}

func assertNoRegularFiles(t *testing.T, root string) {
	t.Helper()
	if files := listRegularFiles(t, root); len(files) > 0 {
		t.Fatalf("expected no regular files under %s, found %#v", root, files)
	}
}

func assertContains(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Fatalf("output missing %q:\n%s", want, got)
	}
}

func acceptanceTestdataPath(t *testing.T, elems ...string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	parts := append([]string{filepath.Dir(file), "..", "..", "testdata", "acceptance"}, elems...)
	return filepath.Clean(filepath.Join(parts...))
}

func currentWorkingDir(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	return cwd
}

func mkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
}
