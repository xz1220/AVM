package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xz1220/agent-vm/internal/config"
	"github.com/xz1220/agent-vm/internal/state"
)

func TestUseStatusDeactivateCommands(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	setupCodexHome(t, home)
	chdir(t, project)

	if _, err := executeCommand("init"); err != nil {
		t.Fatalf("init returned error: %v", err)
	}
	if _, err := executeCommand("agent", "create", "backend-coder", "--runtime", "codex"); err != nil {
		t.Fatalf("agent create returned error: %v", err)
	}

	useOut, err := executeCommand("use", "backend-coder")
	if err != nil {
		t.Fatalf("use returned error: %v", err)
	}
	wantUse := "active: profile:backend-coder\n" +
		"sync: completed\n" +
		"targets:\n" +
		"  codex: synced\n" +
		"warnings:\n" +
		"  none\n"
	if useOut != wantUse {
		t.Fatalf("unexpected use output:\n got: %q\nwant: %q", useOut, wantUse)
	}

	cfg, err := config.ReadGlobalConfig()
	if err != nil {
		t.Fatalf("read global config: %v", err)
	}
	if cfg.Active != (config.ActiveRef{Kind: config.ActiveKindProfile, Name: "backend-coder"}) {
		t.Fatalf("unexpected active ref: %#v", cfg.Active)
	}
	assertCurrentActive(t, "profile:backend-coder")
	codexHome := config.RuntimeHomeDir(config.ActiveRef{Kind: config.ActiveKindProfile, Name: "backend-coder"}, "codex")
	codexConfig := readFileForTest(t, filepath.Join(codexHome, "config.toml"))
	if !strings.Contains(codexConfig, "profile = \"avm-backend-coder\"") {
		t.Fatalf("codex config did not switch selector:\n%s", codexConfig)
	}
	if strings.Contains(codexConfig, "profile = \"user\"") {
		t.Fatalf("codex config kept stale user selector:\n%s", codexConfig)
	}

	statusOut, err := executeCommand("status")
	if err != nil {
		t.Fatalf("status returned error: %v", err)
	}
	rolePath := filepath.ToSlash(filepath.Join(codexHome, "agents", "backend-coder.toml"))
	configPath := filepath.ToSlash(filepath.Join(codexHome, "config.toml"))
	wantStatus := fmt.Sprintf("active: profile:backend-coder\n"+
		"runtime status:\n"+
		"  codex: synced (agent backend-coder)\n"+
		"managed paths:\n"+
		"  codex:\n"+
		"    - %s owner=avm merge=whole-file\n"+
		"    - %s owner=avm merge=whole-file\n"+
		"mapping status:\n"+
		"  codex:\n"+
		"    - active -> profiles.avm-backend-coder: native\n"+
		"    - agent.description -> agents.backend-coder.description: native\n"+
		"    - agent.instructions.developer -> %s#developer_instructions: native\n"+
		"    - agent.instructions.system -> %s#developer_instructions: rendered_as_instructions (Codex role files have developer instructions but no separate AVM system instruction field in Phase 1.)\n"+
		"    - agent.memory_refs -> %s#developer_instructions: rendered_as_instructions (Codex has no native portable memory scope in Phase 1.)\n"+
		"    - agent.model.model -> profiles.avm-backend-coder.model: native\n"+
		"    - agent.model.reasoning_effort -> profiles.avm-backend-coder.model_reasoning_effort: native\n"+
		"    - agent.model.verbosity -> %s#developer_instructions: rendered_as_instructions (Codex Phase 1 does not expose an AVM verbosity field; it is preserved as role guidance.)\n"+
		"    - agent.name -> agents.backend-coder.name: native\n"+
		"    - agent.permissions.approval -> profiles.avm-backend-coder.approval_policy: native\n"+
		"    - agent.permissions.sandbox -> profiles.avm-backend-coder.sandbox_mode: native\n"+
		"    - capabilities.skills -> %s#developer_instructions: rendered_as_instructions (Codex has no native AVM skill registry mount in Phase 1.)\n"+
		"    - project.AGENTS.md: ignored (Codex project instructions are user-owned; the Codex adapter does not overwrite AGENTS.md.)\n"+
		"warnings:\n"+
		"  none\n", rolePath, configPath, rolePath, rolePath, rolePath, rolePath, rolePath)
	if statusOut != wantStatus {
		t.Fatalf("unexpected status output:\n got: %q\nwant: %q", statusOut, wantStatus)
	}

	deactivateOut, err := executeCommand("deactivate")
	if err != nil {
		t.Fatalf("deactivate returned error: %v", err)
	}
	wantDeactivate := "active: profile:default\n" +
		"sync: completed\n" +
		"targets:\n" +
		"  codex: synced\n" +
		"warnings:\n" +
		"  none\n"
	if deactivateOut != wantDeactivate {
		t.Fatalf("unexpected deactivate output:\n got: %q\nwant: %q", deactivateOut, wantDeactivate)
	}
	assertCurrentActive(t, "profile:default")
}

func TestStatusShowsSyncStateDetails(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	setupCodexHome(t, home)
	chdir(t, project)

	if _, err := executeCommand("init"); err != nil {
		t.Fatalf("init returned error: %v", err)
	}
	if _, err := executeCommand("agent", "create", "backend-coder", "--runtime", "codex"); err != nil {
		t.Fatalf("agent create returned error: %v", err)
	}
	if _, err := executeCommand("use", "backend-coder"); err != nil {
		t.Fatalf("use returned error: %v", err)
	}

	active := config.ActiveRef{Kind: config.ActiveKindProfile, Name: "backend-coder"}
	syncState := state.NewSyncState(active)
	syncState.Runtimes["codex"] = state.RuntimeState{
		Runtime:   "codex",
		Status:    state.RuntimeStatusSynced,
		Active:    active,
		AgentName: "backend-coder",
		ManagedPaths: []state.ManagedPathState{
			{Path: "/runtime/config.toml", Owner: "avm", MergeMode: "whole-file"},
		},
		Mappings: []state.MappingState{
			{SourcePath: "model_run.model", TargetPath: "profiles.avm.model", Status: "native"},
		},
		Warnings: []string{"unsupported field capabilities.hooks"},
	}
	raw, err := json.Marshal(syncState)
	if err != nil {
		t.Fatalf("marshal sync state: %v", err)
	}
	if err := os.WriteFile(syncStatePath(), raw, 0o600); err != nil {
		t.Fatalf("write sync state: %v", err)
	}

	statusOut, err := executeCommand("status")
	if err != nil {
		t.Fatalf("status returned error: %v", err)
	}
	wantStatus := "active: profile:backend-coder\n" +
		"runtime status:\n" +
		"  codex: synced (agent backend-coder)\n" +
		"managed paths:\n" +
		"  codex:\n" +
		"    - /runtime/config.toml owner=avm merge=whole-file\n" +
		"mapping status:\n" +
		"  codex:\n" +
		"    - model_run.model -> profiles.avm.model: native\n" +
		"warnings:\n" +
		"  - codex: unsupported field capabilities.hooks\n"
	if statusOut != wantStatus {
		t.Fatalf("unexpected status output:\n got: %q\nwant: %q", statusOut, wantStatus)
	}
}

func TestUseKindEnvAndAutoPrefersProfile(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	setupCodexHome(t, home)
	chdir(t, project)

	if _, err := executeCommand("init"); err != nil {
		t.Fatalf("init returned error: %v", err)
	}
	if _, err := executeCommand("agent", "create", "coding", "--runtime", "codex"); err != nil {
		t.Fatalf("create coding agent: %v", err)
	}
	if _, err := executeCommand("agent", "create", "backend-coder", "--runtime", "codex"); err != nil {
		t.Fatalf("create backend-coder agent: %v", err)
	}
	if _, err := executeCommand("env", "create", "coding", "--codex", "backend-coder"); err != nil {
		t.Fatalf("create coding env: %v", err)
	}

	autoOut, err := executeCommand("use", "coding")
	if err != nil {
		t.Fatalf("auto use returned error: %v", err)
	}
	if !strings.HasPrefix(autoOut, "active: profile:coding\n") {
		t.Fatalf("auto use did not prefer profile:\n%s", autoOut)
	}

	envOut, err := executeCommand("use", "--kind", "env", "coding")
	if err != nil {
		t.Fatalf("env use returned error: %v", err)
	}
	wantEnv := "active: env:coding\n" +
		"sync: completed\n" +
		"targets:\n" +
		"  codex: synced\n" +
		"warnings:\n" +
		"  none\n"
	if envOut != wantEnv {
		t.Fatalf("unexpected env use output:\n got: %q\nwant: %q", envOut, wantEnv)
	}
}

func TestActivatePrintsShellExportsForIsolatedRuntimeHomes(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	sourceCodexHome := setupCodexHome(t, home)
	writeFileForTest(t, filepath.Join(sourceCodexHome, "auth.json"), "{\"auth_mode\":\"source\"}\n")
	sourceClaudeHome := filepath.Join(home, ".claude")
	writeFileForTest(t, filepath.Join(sourceClaudeHome, ".credentials.json"), "{\"type\":\"oauth\"}\n")
	writeFileForTest(t, filepath.Join(sourceClaudeHome, "config.json"), "{\"primaryApiKey\":\"source\"}\n")
	chdir(t, project)

	if _, err := executeCommand("init"); err != nil {
		t.Fatalf("init returned error: %v", err)
	}
	if _, err := executeCommand("agent", "create", "codex-agent", "--runtime", "codex"); err != nil {
		t.Fatalf("create codex agent: %v", err)
	}
	if _, err := executeCommand("agent", "create", "claude-agent", "--runtime", "claude-code"); err != nil {
		t.Fatalf("create claude agent: %v", err)
	}
	if _, err := executeCommand("agent", "create", "opencode-agent", "--runtime", "opencode"); err != nil {
		t.Fatalf("create opencode agent: %v", err)
	}
	if _, err := executeCommand("env", "create", "dual-runtime", "--codex", "codex-agent", "--claude-code", "claude-agent", "--opencode", "opencode-agent"); err != nil {
		t.Fatalf("create env: %v", err)
	}

	out, err := executeCommand("activate", "--kind", "env", "dual-runtime")
	if err != nil {
		t.Fatalf("activate returned error: %v\n%s", err, out)
	}
	active := config.ActiveRef{Kind: config.ActiveKindEnv, Name: "dual-runtime"}
	codexHome := config.RuntimeHomeDir(active, "codex")
	claudeHome := config.RuntimeHomeDir(active, "claude-code")
	opencodeHome := config.RuntimeHomeDir(active, "opencode")
	for _, want := range []string{
		"export AVM_HOME='" + filepath.Join(home, ".avm") + "'",
		"export AVM_ACTIVE='env:dual-runtime'",
		"export CODEX_HOME='" + codexHome + "'",
		"export CLAUDE_CONFIG_DIR='" + claudeHome + "'",
		"export AVM_CLAUDE_MCP_CONFIG='" + filepath.Join(claudeHome, "mcp.json") + "'",
		"export AVM_CLAUDE_AGENT='claude-agent'",
		"export OPENCODE_CONFIG='" + filepath.Join(opencodeHome, "opencode.json") + "'",
		"export OPENCODE_CONFIG_DIR='" + opencodeHome + "'",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("activate output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "sync:") || strings.Contains(out, "warnings:") {
		t.Fatalf("activate output must be eval-safe shell assignments only:\n%s", out)
	}
	assertPathExistsForTest(t, filepath.Join(codexHome, "config.toml"))
	if got := readFileForTest(t, filepath.Join(codexHome, "auth.json")); got != "{\"auth_mode\":\"source\"}\n" {
		t.Fatalf("codex auth sidecar was not copied into runtime home: %q", got)
	}
	assertPathExistsForTest(t, filepath.Join(claudeHome, "settings.json"))
	if got := readFileForTest(t, filepath.Join(claudeHome, ".credentials.json")); got != "{\"type\":\"oauth\"}\n" {
		t.Fatalf("claude credentials sidecar was not copied into runtime home: %q", got)
	}
	if got := readFileForTest(t, filepath.Join(claudeHome, "config.json")); got != "{\"primaryApiKey\":\"source\"}\n" {
		t.Fatalf("claude config auth sidecar was not copied into runtime home: %q", got)
	}
	assertPathExistsForTest(t, filepath.Join(opencodeHome, "opencode.json"))
	assertPathExistsForTest(t, filepath.Join(opencodeHome, "agents", "opencode-agent.md"))

	writeFileForTest(t, filepath.Join(codexHome, "auth.json"), "{\"auth_mode\":\"isolated-login\"}\n")
	writeFileForTest(t, filepath.Join(claudeHome, ".credentials.json"), "{\"type\":\"isolated-oauth\"}\n")
	writeFileForTest(t, filepath.Join(claudeHome, "config.json"), "{\"primaryApiKey\":\"isolated\"}\n")
	out, err = executeCommand("activate", "--kind", "env", "dual-runtime")
	if err != nil {
		t.Fatalf("second activate returned error: %v\n%s", err, out)
	}
	if got := readFileForTest(t, filepath.Join(codexHome, "auth.json")); got != "{\"auth_mode\":\"isolated-login\"}\n" {
		t.Fatalf("codex auth sidecar was not preserved across runtime home reset: %q", got)
	}
	if got := readFileForTest(t, filepath.Join(claudeHome, ".credentials.json")); got != "{\"type\":\"isolated-oauth\"}\n" {
		t.Fatalf("claude credentials sidecar was not preserved across runtime home reset: %q", got)
	}
	if got := readFileForTest(t, filepath.Join(claudeHome, "config.json")); got != "{\"primaryApiKey\":\"isolated\"}\n" {
		t.Fatalf("claude config auth sidecar was not preserved across runtime home reset: %q", got)
	}
}

func TestStatusFiltersStaleRuntimeStateAfterActivationSwitch(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	setupCodexHome(t, home)
	cursorDir := filepath.Join(project, ".cursor")
	if err := os.MkdirAll(cursorDir, 0o700); err != nil {
		t.Fatalf("create cursor dir: %v", err)
	}
	chdir(t, project)

	if _, err := executeCommand("init"); err != nil {
		t.Fatalf("init returned error: %v", err)
	}
	if _, err := executeCommand("agent", "create", "writer-agent", "--runtime", "codex"); err != nil {
		t.Fatalf("create writer-agent: %v", err)
	}
	if _, err := executeCommand("agent", "create", "cursor-agent", "--runtime", "cursor"); err != nil {
		t.Fatalf("create cursor-agent: %v", err)
	}
	if _, err := executeCommand("env", "create", "all-runtimes", "--codex", "writer-agent", "--cursor", "cursor-agent"); err != nil {
		t.Fatalf("create env: %v", err)
	}
	if out, err := executeCommand("use", "--kind", "env", "all-runtimes"); err != nil {
		t.Fatalf("use env returned error: %v\n%s", err, out)
	}

	envStatus, err := executeCommand("status")
	if err != nil {
		t.Fatalf("env status returned error: %v", err)
	}
	if !strings.Contains(envStatus, "  cursor: synced (agent cursor-agent)") {
		t.Fatalf("env status missing cursor synced state:\n%s", envStatus)
	}

	if out, err := executeCommand("use", "--kind", "profile", "writer-agent"); err != nil {
		t.Fatalf("use profile returned error: %v\n%s", err, out)
	}
	profileStatus, err := executeCommand("status")
	if err != nil {
		t.Fatalf("profile status returned error: %v", err)
	}
	if strings.Contains(profileStatus, "cursor: synced") || strings.Contains(profileStatus, "cursor-agent") {
		t.Fatalf("profile status leaked stale cursor state:\n%s", profileStatus)
	}

	codexHome := config.RuntimeHomeDir(config.ActiveRef{Kind: config.ActiveKindProfile, Name: "writer-agent"}, "codex")
	codexConfig := readFileForTest(t, filepath.Join(codexHome, "config.toml"))
	if !strings.Contains(codexConfig, "profile = \"avm-writer-agent\"") {
		t.Fatalf("codex config did not switch to writer-agent selector:\n%s", codexConfig)
	}
}

func TestUseRendersMCPRegistryDefinitions(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("GITHUB_TOKEN", "REAL_SECRET_SHOULD_NOT_APPEAR")
	setupCodexHome(t, home)
	chdir(t, project)

	if _, err := executeCommand("init"); err != nil {
		t.Fatalf("init returned error: %v", err)
	}
	writeFileForTest(t, filepath.Join(home, ".avm", "registry", "mcps", "github.yaml"), "name: github\nkind: mcp\nserver:\n  type: stdio\n  command: printf\n  args:\n    - avm-test-mcp\n  env:\n    GITHUB_TOKEN: ${GITHUB_TOKEN}\n")
	if _, err := executeCommand("agent", "create", "codex-agent", "--runtime", "codex", "--mcps", "github"); err != nil {
		t.Fatalf("create codex agent: %v", err)
	}
	if _, err := executeCommand("agent", "create", "claude-agent", "--runtime", "claude-code", "--mcps", "github"); err != nil {
		t.Fatalf("create claude agent: %v", err)
	}
	if _, err := executeCommand("env", "create", "all-runtimes", "--codex", "codex-agent", "--claude-code", "claude-agent"); err != nil {
		t.Fatalf("create env: %v", err)
	}
	if out, err := executeCommand("use", "--kind", "env", "all-runtimes"); err != nil {
		t.Fatalf("use env returned error: %v\n%s", err, out)
	}

	active := config.ActiveRef{Kind: config.ActiveKindEnv, Name: "all-runtimes"}
	codexHome := config.RuntimeHomeDir(active, "codex")
	claudeConfigDir := config.RuntimeHomeDir(active, "claude-code")
	codexConfig := readFileForTest(t, filepath.Join(codexHome, "config.toml"))
	for _, want := range []string{
		"profile = \"avm-all-runtimes\"",
		"[mcp_servers.github]",
		"command = \"printf\"",
		"args = [\"avm-test-mcp\"]",
		"GITHUB_TOKEN = \"${GITHUB_TOKEN}\"",
	} {
		if !strings.Contains(codexConfig, want) {
			t.Fatalf("codex config missing %q:\n%s", want, codexConfig)
		}
	}
	if strings.Contains(codexConfig, "REAL_SECRET_SHOULD_NOT_APPEAR") {
		t.Fatalf("codex config expanded secret:\n%s", codexConfig)
	}

	claudeMCP := readFileForTest(t, filepath.Join(claudeConfigDir, "mcp.json"))
	for _, want := range []string{`"github"`, `"command": "printf"`, `"avm-test-mcp"`, `"GITHUB_TOKEN": "${GITHUB_TOKEN}"`} {
		if !strings.Contains(claudeMCP, want) {
			t.Fatalf("claude mcp config missing %q:\n%s", want, claudeMCP)
		}
	}
	if strings.Contains(claudeMCP, "REAL_SECRET_SHOULD_NOT_APPEAR") {
		t.Fatalf("claude mcp config expanded secret:\n%s", claudeMCP)
	}
}

func TestUseActivatesSkillContentForRuntimeSkillDirs(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	setupCodexHome(t, home)
	clineDataHome := filepath.Join(home, ".cline-test")
	for _, dir := range []string{clineDataHome} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatalf("create runtime dir %s: %v", dir, err)
		}
	}
	t.Setenv("CLINE_DATA_HOME", clineDataHome)
	chdir(t, project)

	if _, err := executeCommand("init"); err != nil {
		t.Fatalf("init returned error: %v", err)
	}
	writeFileForTest(t, filepath.Join(home, ".avm", "registry", "skills", "probe-skill", "SKILL.md"), "# Probe\n\nAVM_SKILL_PROBE_MARKER_20260426\n")
	writeFileForTest(t, filepath.Join(home, ".avm", "registry", "skills", "unused-skill", "SKILL.md"), "# Unused\n\nUNUSED_SKILL_MARKER\n")
	if _, err := executeCommand("agent", "create", "codex-skill-agent", "--runtime", "codex", "--skills", "probe-skill"); err != nil {
		t.Fatalf("create codex skill agent: %v", err)
	}
	if _, err := executeCommand("agent", "create", "claude-skill-agent", "--runtime", "claude-code", "--skills", "probe-skill"); err != nil {
		t.Fatalf("create claude skill agent: %v", err)
	}
	if _, err := executeCommand("agent", "create", "cline-skill-agent", "--runtime", "cline", "--skills", "probe-skill"); err != nil {
		t.Fatalf("create cline skill agent: %v", err)
	}
	if _, err := executeCommand("env", "create", "skill-env", "--codex", "codex-skill-agent", "--claude-code", "claude-skill-agent", "--cline", "cline-skill-agent"); err != nil {
		t.Fatalf("create skill env: %v", err)
	}
	if out, err := executeCommand("use", "--kind", "env", "skill-env"); err != nil {
		t.Fatalf("use skill env returned error: %v\n%s", err, out)
	}
	skillEnv := config.ActiveRef{Kind: config.ActiveKindEnv, Name: "skill-env"}
	codexHome := config.RuntimeHomeDir(skillEnv, "codex")
	claudeConfigDir := config.RuntimeHomeDir(skillEnv, "claude-code")

	activeSkillPath := filepath.Join(home, ".avm", "active", "skills", "probe-skill", "SKILL.md")
	activeUnusedPath := filepath.Join(home, ".avm", "active", "skills", "unused-skill", "SKILL.md")
	if got := readFileForTest(t, activeSkillPath); !strings.Contains(got, "AVM_SKILL_PROBE_MARKER_20260426") {
		t.Fatalf("active skill missing marker:\n%s", got)
	}
	if _, err := os.Stat(activeUnusedPath); !os.IsNotExist(err) {
		t.Fatalf("unused skill should not be active, stat err: %v", err)
	}

	for _, path := range []string{
		filepath.Join(codexHome, "skills", "probe-skill", "SKILL.md"),
		filepath.Join(claudeConfigDir, "skills", "probe-skill", "SKILL.md"),
	} {
		got := readFileForTest(t, path)
		for _, want := range []string{`name: "probe-skill"`, "AVM_SKILL_PROBE_MARKER_20260426"} {
			if !strings.Contains(got, want) {
				t.Fatalf("runtime skill %s missing %q:\n%s", path, want, got)
			}
		}
	}

	clineRules := readFileForTest(t, filepath.Join(project, ".clinerules", "avm", "cline-skill-agent.md"))
	if !strings.Contains(clineRules, filepath.ToSlash(activeSkillPath)) {
		t.Fatalf("cline rules missing active skill path:\n%s", clineRules)
	}

	if _, err := executeCommand("agent", "create", "no-skill-agent", "--runtime", "codex"); err != nil {
		t.Fatalf("create no-skill agent: %v", err)
	}
	if out, err := executeCommand("use", "--kind", "profile", "no-skill-agent"); err != nil {
		t.Fatalf("use no-skill profile returned error: %v\n%s", err, out)
	}
	noSkillCodexHome := config.RuntimeHomeDir(config.ActiveRef{Kind: config.ActiveKindProfile, Name: "no-skill-agent"}, "codex")
	for _, path := range []string{
		activeSkillPath,
		filepath.Join(noSkillCodexHome, "skills", "probe-skill", "SKILL.md"),
	} {
		assertPathMissing(t, path)
	}
	for _, path := range []string{
		filepath.Join(codexHome, "skills", "probe-skill", "SKILL.md"),
		filepath.Join(claudeConfigDir, "skills", "probe-skill", "SKILL.md"),
	} {
		assertPathExistsForTest(t, path)
	}
}

func TestUseMissingActivationStableErrors(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	setupCodexHome(t, home)
	chdir(t, project)

	if _, err := executeCommand("init"); err != nil {
		t.Fatalf("init returned error: %v", err)
	}

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "auto", args: []string{"use", "missing"}, want: "activation \"missing\" not found as profile or env"},
		{name: "profile", args: []string{"use", "--kind", "profile", "missing"}, want: "profile \"missing\" not found"},
		{name: "env", args: []string{"use", "--kind", "env", "missing"}, want: "env \"missing\" not found"},
		{name: "invalid kind", args: []string{"use", "--kind", "team", "missing"}, want: "invalid activation kind \"team\" (want profile or env)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := executeCommand(tt.args...)
			if err == nil {
				t.Fatal("expected error")
			}
			if got := err.Error(); got != tt.want {
				t.Fatalf("unexpected error:\n got: %q\nwant: %q", got, tt.want)
			}
		})
	}
}

func assertCurrentActive(t *testing.T, want string) {
	t.Helper()
	data, err := os.ReadFile(currentActivePath())
	if err != nil {
		t.Fatalf("read current active: %v", err)
	}
	if got := strings.TrimSpace(string(data)); got != want {
		t.Fatalf("unexpected current active:\n got: %q\nwant: %q", got, want)
	}
}

func readFileForTest(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func writeFileForTest(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("create %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func assertPathExistsForTest(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected path %s: %v", path, err)
	}
}

func setupCodexHome(t *testing.T, home string) string {
	t.Helper()

	codexHome := filepath.Join(home, ".codex-test")
	if err := os.MkdirAll(codexHome, 0o700); err != nil {
		t.Fatalf("create codex home: %v", err)
	}
	t.Setenv("CODEX_HOME", codexHome)
	return codexHome
}
