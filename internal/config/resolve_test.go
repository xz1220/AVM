package config

import (
	"reflect"
	"strings"
	"testing"
)

func TestResolveActivationProfileActive(t *testing.T) {
	_, project := setupResolveTest(t)

	agent := resolveTestAgent("backend-coder", "codex", "git", "test")
	if err := WriteAgent(&agent, ScopeGlobal, project); err != nil {
		t.Fatalf("WriteAgent returned error: %v", err)
	}
	cfg := GlobalConfig{
		Active: ActiveRef{Kind: ActiveKindProfile, Name: "backend-coder"},
		Defaults: DefaultsConfig{
			SourceScope:      string(ScopeGlobal),
			Targets:          []string{"codex"},
			ConflictStrategy: "prompt",
		},
	}
	if err := WriteGlobalConfig(&cfg); err != nil {
		t.Fatalf("WriteGlobalConfig returned error: %v", err)
	}

	resolved, err := ResolveActivation(ActiveRef{Kind: ActiveKindProfile, Name: "backend-coder"}, project)
	if err != nil {
		t.Fatalf("ResolveActivation returned error: %v", err)
	}

	if resolved.Env != nil {
		t.Fatalf("profile active should not include env, got %#v", resolved.Env)
	}
	if got := resolved.RuntimeAgents["codex"].Name; got != "backend-coder" {
		t.Fatalf("resolved codex agent = %q, want backend-coder", got)
	}
	assertStringSlice(t, resolved.Targets, []string{"codex"})
	assertStringSlice(t, resolved.Capabilities["codex"].Skills, []string{"git", "test"})
	if !containsString(resolved.SourceFiles, AgentPath("backend-coder")) {
		t.Fatalf("source files missing agent path: %#v", resolved.SourceFiles)
	}
	if !containsString(resolved.SourceFiles, GlobalConfigPath()) {
		t.Fatalf("source files missing global config path: %#v", resolved.SourceFiles)
	}
}

func TestResolveActivationEnvActive(t *testing.T) {
	_, project := setupResolveTest(t)

	agents := []AgentProfile{
		resolveTestAgent("backend-coder", "codex", "git"),
		resolveTestAgent("backend-reviewer", "claude-code", "review"),
		resolveTestAgent("backend-assistant", "cline", "assist"),
	}
	for i := range agents {
		if err := WriteAgent(&agents[i], ScopeGlobal, project); err != nil {
			t.Fatalf("WriteAgent(%s) returned error: %v", agents[i].Name, err)
		}
	}
	env := Environment{
		Name:    "coding",
		Version: "1.0.0",
		RuntimeAgents: map[string]RuntimeAgent{
			"codex":       {Primary: "backend-coder", Available: []string{"backend-coder", "backend-reviewer"}},
			"claude-code": {Primary: "backend-reviewer"},
			"cline":       {Primary: "backend-assistant"},
		},
		Targets: []string{"codex", "claude-code", "cline"},
	}
	if err := WriteEnvironment(&env); err != nil {
		t.Fatalf("WriteEnvironment returned error: %v", err)
	}

	resolved, err := ResolveActivation(ActiveRef{Kind: ActiveKindEnv, Name: "coding"}, project)
	if err != nil {
		t.Fatalf("ResolveActivation returned error: %v", err)
	}

	if resolved.Env == nil || resolved.Env.Name != "coding" {
		t.Fatalf("resolved env = %#v, want coding", resolved.Env)
	}
	if got := resolved.RuntimeAgents["codex"].Name; got != "backend-coder" {
		t.Fatalf("resolved codex agent = %q, want backend-coder", got)
	}
	if got := resolved.RuntimeAgents["claude-code"].Name; got != "backend-reviewer" {
		t.Fatalf("resolved claude-code agent = %q, want backend-reviewer", got)
	}
	if got := resolved.RuntimeAgents["cline"].Name; got != "backend-assistant" {
		t.Fatalf("resolved cline agent = %q, want backend-assistant", got)
	}
	assertStringSlice(t, resolved.Targets, []string{"codex", "claude-code", "cline"})
	assertStringSlice(t, resolved.Capabilities["claude-code"].Skills, []string{"review"})
}

func TestResolveActivationMissingProfile(t *testing.T) {
	_, project := setupResolveTest(t)

	_, err := ResolveActivation(ActiveRef{Kind: ActiveKindProfile, Name: "missing-profile"}, project)
	if err == nil {
		t.Fatal("ResolveActivation returned nil error for missing profile")
	}
	if !strings.Contains(err.Error(), `profile "missing-profile" not found`) {
		t.Fatalf("missing profile error = %v", err)
	}
}

func TestResolveActivationProjectOverridePriority(t *testing.T) {
	_, project := setupResolveTest(t)

	globalCoder := resolveTestAgent("global-coder", "codex", "global")
	if err := WriteAgent(&globalCoder, ScopeGlobal, project); err != nil {
		t.Fatalf("WriteAgent(global-coder) returned error: %v", err)
	}
	globalProjectCoder := resolveTestAgent("project-coder", "codex", "global-skill")
	globalProjectCoder.Description = "global project-coder"
	if err := WriteAgent(&globalProjectCoder, ScopeGlobal, project); err != nil {
		t.Fatalf("WriteAgent(global project-coder) returned error: %v", err)
	}
	projectCoder := resolveTestAgent("project-coder", "codex", "project-skill")
	projectCoder.Description = "project project-coder"
	if err := WriteAgent(&projectCoder, ScopeProject, project); err != nil {
		t.Fatalf("WriteAgent(project project-coder) returned error: %v", err)
	}

	env := Environment{
		Name:    "coding",
		Version: "1.0.0",
		RuntimeAgents: map[string]RuntimeAgent{
			"codex": {Primary: "global-coder"},
		},
		Targets: []string{"codex"},
	}
	if err := WriteEnvironment(&env); err != nil {
		t.Fatalf("WriteEnvironment returned error: %v", err)
	}
	writeProjectOverride(t, project, &ProjectOverride{
		Extends: "coding",
		RuntimeAgents: map[string]RuntimeAgent{
			"codex": {Primary: "project-coder"},
		},
		Targets: []string{"codex"},
	})

	resolved, err := ResolveActivation(ActiveRef{Kind: ActiveKindEnv, Name: "coding"}, project)
	if err != nil {
		t.Fatalf("ResolveActivation returned error: %v", err)
	}

	got := resolved.RuntimeAgents["codex"]
	if got.Name != "project-coder" {
		t.Fatalf("resolved codex agent = %q, want project-coder", got.Name)
	}
	if got.Description != "project project-coder" {
		t.Fatalf("resolved codex agent description = %q, want project profile", got.Description)
	}
	if got.SourceScope != string(ScopeProject) {
		t.Fatalf("resolved codex source scope = %q, want project", got.SourceScope)
	}
	assertStringSlice(t, resolved.Capabilities["codex"].Skills, []string{"project-skill"})
	if resolved.Env.RuntimeAgents["codex"].Primary != "project-coder" {
		t.Fatalf("merged env primary = %q, want project-coder", resolved.Env.RuntimeAgents["codex"].Primary)
	}
	if !containsString(resolved.SourceFiles, ProjectEnvPath(project)) {
		t.Fatalf("source files missing project override: %#v", resolved.SourceFiles)
	}
	if !containsString(resolved.SourceFiles, ProjectAgentPath(project, "project-coder")) {
		t.Fatalf("source files missing project agent: %#v", resolved.SourceFiles)
	}
}

func setupResolveTest(t *testing.T) (string, string) {
	t.Helper()
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	return home, project
}

func resolveTestAgent(name, runtime string, skills ...string) AgentProfile {
	return AgentProfile{
		Name:    name,
		Version: "1.0.0",
		Runtime: RuntimePreferences{
			Preferred: runtime,
			Kind:      "local",
			Mode:      "primary",
		},
		Capabilities: CapabilityRefs{
			Skills: skills,
		},
		Permissions: Permissions{
			Approval: "on-request",
			Sandbox:  "workspace-write",
		},
	}
}

func writeProjectOverride(t *testing.T, project string, override *ProjectOverride) {
	t.Helper()
	if err := writeYAML(ProjectEnvPath(project), override); err != nil {
		t.Fatalf("write project override: %v", err)
	}
}

func assertStringSlice(t *testing.T, got, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("slice mismatch:\nwant %#v\ngot  %#v", want, got)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
