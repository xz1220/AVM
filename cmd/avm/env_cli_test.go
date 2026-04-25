package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/xz1220/agent-vm/internal/config"
)

func TestEnvCreateFailsForMissingAgentProfile(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	chdir(t, project)

	_, err := executeCommand("env", "create", "coding", "--codex", "missing-profile")
	if err == nil {
		t.Fatal("env create returned nil error for missing profile")
	}
	if got := err.Error(); !strings.Contains(got, "runtime_agents.codex.primary") || !strings.Contains(got, `profile "missing-profile" not found`) {
		t.Fatalf("unexpected missing profile error: %v", err)
	}
	if _, statErr := os.Stat(config.EnvPath("coding")); !os.IsNotExist(statErr) {
		t.Fatalf("env create wrote global env despite missing profile, stat err: %v", statErr)
	}
}

func TestEnvCreateLocalWritesProjectOverrideAndResolves(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	chdir(t, project)

	writeEnvTestAgent(t, project, config.ScopeGlobal, envTestAgent("global-coder", "codex", "global"))
	writeEnvTestAgent(t, project, config.ScopeGlobal, envTestAgent("global-reviewer", "claude-code", "review"))
	writeEnvTestAgent(t, project, config.ScopeProject, envTestAgent("project-coder", "codex", "project"))

	base := config.Environment{
		Name: "backend-dev",
		RuntimeAgents: map[string]config.RuntimeAgent{
			"codex":       {Primary: "global-coder"},
			"claude-code": {Primary: "global-reviewer"},
		},
		Targets: []string{"codex", "claude-code"},
	}
	if err := config.WriteEnvironment(&base); err != nil {
		t.Fatalf("WriteEnvironment returned error: %v", err)
	}
	beforeGlobal, err := os.ReadFile(config.EnvPath("backend-dev"))
	if err != nil {
		t.Fatalf("read global env before local create: %v", err)
	}

	out, err := executeCommand("env", "create", "backend-dev", "--local", "--codex", "project-coder")
	if err != nil {
		t.Fatalf("env create --local returned error: %v", err)
	}
	if got := strings.TrimSpace(out); got != "created local env override backend-dev" {
		t.Fatalf("unexpected env create --local output: %q", got)
	}

	afterGlobal, err := os.ReadFile(config.EnvPath("backend-dev"))
	if err != nil {
		t.Fatalf("read global env after local create: %v", err)
	}
	if !reflect.DeepEqual(beforeGlobal, afterGlobal) {
		t.Fatalf("local env create changed global env:\nbefore:\n%s\nafter:\n%s", beforeGlobal, afterGlobal)
	}

	override, err := config.ReadProjectOverride(project)
	if err != nil {
		t.Fatalf("ReadProjectOverride returned error: %v", err)
	}
	if override.Extends != "backend-dev" {
		t.Fatalf("override extends = %q, want backend-dev", override.Extends)
	}
	if got := override.RuntimeAgents["codex"].Primary; got != "project-coder" {
		t.Fatalf("override codex primary = %q, want project-coder", got)
	}
	if override.Targets != nil {
		t.Fatalf("local override should preserve base targets by omitting targets, got %#v", override.Targets)
	}

	resolved, err := config.ResolveActivation(config.ActiveRef{Kind: config.ActiveKindEnv, Name: "backend-dev"}, project)
	if err != nil {
		t.Fatalf("ResolveActivation returned error: %v", err)
	}
	if got := resolved.RuntimeAgents["codex"].Name; got != "project-coder" {
		t.Fatalf("resolved codex agent = %q, want project-coder", got)
	}
	if got := resolved.RuntimeAgents["claude-code"].Name; got != "global-reviewer" {
		t.Fatalf("resolved claude-code agent = %q, want global-reviewer", got)
	}
	if !reflect.DeepEqual(resolved.Targets, []string{"codex", "claude-code"}) {
		t.Fatalf("resolved targets = %#v, want base targets", resolved.Targets)
	}
	if !containsEnvTestString(resolved.SourceFiles, config.ProjectEnvPath(project)) {
		t.Fatalf("source files missing project override: %#v", resolved.SourceFiles)
	}

	entries, err := os.ReadDir(filepath.Join(home, ".avm", "envs"))
	if err != nil {
		t.Fatalf("read global env dir: %v", err)
	}
	if len(entries) != 1 || entries[0].Name() != "backend-dev.yaml" {
		t.Fatalf("local env create should not add global env files, got %#v", entries)
	}
}

func TestEnvCreateLocalUsesActiveEnvWhenNameOmitted(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	chdir(t, project)

	writeEnvTestAgent(t, project, config.ScopeGlobal, envTestAgent("global-coder", "codex", "global"))
	writeEnvTestAgent(t, project, config.ScopeProject, envTestAgent("project-coder", "codex", "project"))
	if err := config.WriteEnvironment(&config.Environment{
		Name: "backend-dev",
		RuntimeAgents: map[string]config.RuntimeAgent{
			"codex": {Primary: "global-coder"},
		},
		Targets: []string{"codex"},
	}); err != nil {
		t.Fatalf("WriteEnvironment returned error: %v", err)
	}
	if err := config.WriteGlobalConfig(&config.GlobalConfig{
		Active: config.ActiveRef{Kind: config.ActiveKindEnv, Name: "backend-dev"},
		Defaults: config.DefaultsConfig{
			SourceScope:      string(config.ScopeGlobal),
			Targets:          []string{"codex"},
			ConflictStrategy: "prompt",
		},
	}); err != nil {
		t.Fatalf("WriteGlobalConfig returned error: %v", err)
	}

	if _, err := executeCommand("env", "create", "--local", "--codex", "project-coder"); err != nil {
		t.Fatalf("env create --local without name returned error: %v", err)
	}

	override, err := config.ReadProjectOverride(project)
	if err != nil {
		t.Fatalf("ReadProjectOverride returned error: %v", err)
	}
	if override.Extends != "backend-dev" {
		t.Fatalf("override extends = %q, want active env backend-dev", override.Extends)
	}
}

func writeEnvTestAgent(t *testing.T, project string, scope config.Scope, agent config.AgentProfile) {
	t.Helper()
	if err := config.WriteAgent(&agent, scope, project); err != nil {
		t.Fatalf("WriteAgent(%s) returned error: %v", agent.Name, err)
	}
}

func envTestAgent(name, runtime string, skills ...string) config.AgentProfile {
	return config.AgentProfile{
		Name: name,
		Runtime: config.RuntimePreferences{
			Preferred: runtime,
		},
		Capabilities: config.CapabilityRefs{
			Skills: skills,
		},
	}
}

func containsEnvTestString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
