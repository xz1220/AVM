package opencode

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xz1220/agent-vm/internal/app/model"
	"github.com/xz1220/agent-vm/internal/runtime"
)

func TestName(t *testing.T) {
	if got := New().Name(); got != Name {
		t.Fatalf("Name=%q want %q", got, Name)
	}
}

func TestFacts_BinaryMissing(t *testing.T) {
	t.Setenv("PATH", "/nonexistent")
	d := New()
	f, err := d.Facts(context.Background())
	if err != nil {
		t.Fatalf("Facts unexpected error: %v", err)
	}
	if f.Available {
		t.Fatalf("expected Available=false, got %+v", f)
	}
}

func TestFacts_BinaryPresent(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "openclaw")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\necho 0.5.0\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake bin: %v", err)
	}
	t.Setenv("PATH", dir)
	d := New()
	f, err := d.Facts(context.Background())
	if err != nil {
		t.Fatalf("Facts: %v", err)
	}
	if !f.Available {
		t.Fatalf("expected Available=true, got %+v", f)
	}
	if !strings.Contains(f.Version, "0.5.0") {
		t.Fatalf("Version=%q want to contain 0.5.0", f.Version)
	}
}

func TestBoundary_AllEnvVars(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("AVM_HOME", tmp)
	d := New()
	a := &model.Agent{Identity: model.Identity{Name: "demo"}}
	b, err := d.Boundary(context.Background(), a)
	if err != nil {
		t.Fatalf("Boundary: %v", err)
	}
	want := filepath.Join(tmp, "boundaries", Name, "demo")
	if b.StateDir != want {
		t.Fatalf("StateDir=%q want %q", b.StateDir, want)
	}
	for _, k := range []string{EnvStateDir, EnvConfigPath, EnvAgentDir} {
		if _, ok := b.Env[k]; !ok {
			t.Errorf("missing env var %q", k)
		}
	}
	if b.Env[EnvStateDir] != want {
		t.Errorf("%s=%q want %q", EnvStateDir, b.Env[EnvStateDir], want)
	}
}

func TestPlan_Mappings(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("AVM_HOME", tmp)
	d := New()
	a := &model.Agent{
		Identity: model.Identity{
			Name:        "demo",
			Description: "demo",
			Role:        "qa",
		},
		Instructions: model.Instructions{System: "be precise"},
		Skills:       []model.CapabilityRef{{ID: "s1", Kind: model.CapabilityKindSkill}},
		MCP:          []model.CapabilityRef{{ID: "m1", Kind: model.CapabilityKindMCP}},
		Runtimes:     []model.RuntimePref{{Runtime: Name}},
	}
	plan, err := d.Plan(context.Background(), a)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(plan.Files) < 2 {
		t.Fatalf("expected at least AGENTS.md + openclaw.json, got %d", len(plan.Files))
	}
	got := map[string]model.MappingStatus{}
	for _, m := range plan.Mappings {
		got[m.Field] = m.Status
	}
	want := map[string]model.MappingStatus{
		"identity.name":        model.MappingNative,
		"identity.description": model.MappingNative,
		"identity.role":        model.MappingRenderedAsInstructions,
		"instructions":         model.MappingNative,
		"skills":               model.MappingNative,
		"mcp":                  model.MappingNative,
		"runtimes":             model.MappingIgnored,
	}
	for f, s := range want {
		if got[f] != s {
			t.Errorf("field %q status=%q want %q", f, got[f], s)
		}
	}
	// Verify openclaw.json is well-formed and includes the MCP server name.
	for _, f := range plan.Files {
		if filepath.Base(f.Path) == "openclaw.json" {
			var raw map[string]any
			if err := json.Unmarshal(f.Contents, &raw); err != nil {
				t.Fatalf("openclaw.json invalid: %v", err)
			}
			mcp, _ := raw["mcp"].(map[string]any)
			servers, _ := mcp["servers"].(map[string]any)
			if _, ok := servers["m1"]; !ok {
				t.Errorf("openclaw.json missing servers[m1], got %+v", servers)
			}
		}
	}
}

func TestDiscoverGlobal_Empty(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv(EnvStateDir, t.TempDir())
	d := New()
	got, err := d.DiscoverGlobal(context.Background())
	if err != nil {
		t.Fatalf("DiscoverGlobal: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0, got %d", len(got))
	}
}

func TestDiscoverGlobal_FindsSkillAndMCP(t *testing.T) {
	state := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv(EnvStateDir, state)
	skill := filepath.Join(state, "workspace", "skills", "linter")
	if err := os.MkdirAll(skill, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("---\nversion: 9\n---\n"), 0o600); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
	cfg := `{"mcp":{"servers":{"alpha":{"command":"true"}}}}`
	if err := os.WriteFile(filepath.Join(state, "openclaw.json"), []byte(cfg), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	d := New()
	got, err := d.DiscoverGlobal(context.Background())
	if err != nil {
		t.Fatalf("DiscoverGlobal: %v", err)
	}
	skillSeen := false
	mcpSeen := false
	for _, c := range got {
		if c.Kind == model.CapabilityKindSkill && c.Name == "linter" {
			skillSeen = true
		}
		if c.Kind == model.CapabilityKindMCP && c.Name == "alpha" {
			mcpSeen = true
		}
	}
	if !skillSeen {
		t.Errorf("expected linter skill")
	}
	if !mcpSeen {
		t.Errorf("expected alpha MCP")
	}
}

func TestLaunchSpec(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "openclaw")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake bin: %v", err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("AVM_HOME", t.TempDir())
	d := New()
	a := &model.Agent{Identity: model.Identity{Name: "demo"}}
	spec, err := d.LaunchSpec(context.Background(), a, &runtime.Plan{})
	if err != nil {
		t.Fatalf("LaunchSpec: %v", err)
	}
	if spec.Bin != bin {
		t.Fatalf("Bin=%q want %q", spec.Bin, bin)
	}
	if !spec.Stdin {
		t.Fatalf("expected Stdin=true")
	}
	if len(spec.Args) == 0 {
		t.Fatalf("expected default agent args")
	}
	if _, ok := spec.Env[EnvStateDir]; !ok {
		t.Fatalf("missing %s in env", EnvStateDir)
	}
}
