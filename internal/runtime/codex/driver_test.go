package codex

import (
	"context"
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
		t.Fatalf("expected Available=false when binary missing, got %+v", f)
	}
	if f.Name != Name {
		t.Fatalf("Name=%q want %q", f.Name, Name)
	}
}

func TestFacts_BinaryPresent(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "codex")
	// Cheap fake: prints a version on --version, otherwise exits 0.
	script := "#!/bin/sh\necho codex 0.0.0-test\nexit 0\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
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
	if f.BinaryPath != bin {
		t.Fatalf("BinaryPath=%q want %q", f.BinaryPath, bin)
	}
	if !strings.Contains(f.Version, "0.0.0-test") {
		t.Fatalf("Version=%q want to contain 0.0.0-test", f.Version)
	}
	if len(f.Capabilities) == 0 {
		t.Fatalf("expected non-empty Capabilities")
	}
	if len(f.Risks) == 0 {
		t.Fatalf("expected non-empty Risks")
	}
}

func TestBoundary_UsesAVMHome(t *testing.T) {
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
	if v := b.Env[EnvHome]; v != want {
		t.Fatalf("env[%s]=%q want %q", EnvHome, v, want)
	}
}

func TestBoundary_RejectsEmptyName(t *testing.T) {
	d := New()
	if _, err := d.Boundary(context.Background(), &model.Agent{}); err == nil {
		t.Fatalf("expected error for empty agent name")
	}
}

func TestPlan_FieldMappings(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("AVM_HOME", tmp)
	d := New()
	a := &model.Agent{
		Identity: model.Identity{
			Name:        "demo",
			Description: "desc",
			Role:        "reviewer",
		},
		Instructions: model.Instructions{System: "be helpful"},
		Skills:       []model.CapabilityRef{{ID: "s1", Kind: model.CapabilityKindSkill}},
		MCP:          []model.CapabilityRef{{ID: "m1", Kind: model.CapabilityKindMCP}},
		Runtimes:     []model.RuntimePref{{Runtime: Name, Default: true}},
	}
	plan, err := d.Plan(context.Background(), a)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(plan.Files) < 2 {
		t.Fatalf("expected at least AGENTS.md + config.toml, got %d files", len(plan.Files))
	}
	got := map[string]model.MappingStatus{}
	for _, m := range plan.Mappings {
		got[m.Field] = m.Status
	}
	cases := map[string]model.MappingStatus{
		"identity.name":        model.MappingNative,
		"identity.description": model.MappingNative,
		"identity.role":        model.MappingRenderedAsInstructions,
		"instructions":         model.MappingNative,
		"skills":               model.MappingNative,
		"mcp":                  model.MappingNative,
		"runtimes":             model.MappingIgnored,
	}
	for field, want := range cases {
		if got[field] != want {
			t.Errorf("field %q status=%q want %q", field, got[field], want)
		}
	}
	// Each managed file must be inside the boundary StateDir.
	bnd, _ := d.Boundary(context.Background(), a)
	for _, f := range plan.Files {
		if !strings.HasPrefix(f.Path, bnd.StateDir) {
			t.Errorf("managed file %q not inside boundary %q", f.Path, bnd.StateDir)
		}
	}
}

func TestPlan_RejectsInvalidName(t *testing.T) {
	d := New()
	if _, err := d.Plan(context.Background(), &model.Agent{Identity: model.Identity{Name: "BAD NAME"}}); err == nil {
		t.Fatalf("expected validation error")
	}
}

func TestDiscoverGlobal_MissingDirIsEmpty(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv(EnvHome, t.TempDir())
	d := New()
	got, err := d.DiscoverGlobal(context.Background())
	if err != nil {
		t.Fatalf("DiscoverGlobal: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 capabilities, got %d", len(got))
	}
}

func TestDiscoverGlobal_FindsSkills(t *testing.T) {
	codexHome := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv(EnvHome, codexHome)
	skill := filepath.Join(codexHome, "skills", "demo")
	if err := os.MkdirAll(skill, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	manifest := "---\nversion: 1.2.3\n---\nbody"
	if err := os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte(manifest), 0o600); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
	d := New()
	got, err := d.DiscoverGlobal(context.Background())
	if err != nil {
		t.Fatalf("DiscoverGlobal: %v", err)
	}
	found := false
	for _, c := range got {
		if c.Kind == model.CapabilityKindSkill && c.Name == "demo" {
			found = true
			if c.Version != "1.2.3" {
				t.Errorf("Version=%q want 1.2.3", c.Version)
			}
			if c.Runtime != Name {
				t.Errorf("Runtime=%q want %q", c.Runtime, Name)
			}
		}
	}
	if !found {
		t.Fatalf("expected to find demo skill in %+v", got)
	}
}

func TestDiscoverGlobal_FindsMCP(t *testing.T) {
	codexHome := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv(EnvHome, codexHome)
	cfg := `[mcp_servers.foo]
command = "true"

[mcp_servers."bar-baz"]
command = "true"
`
	if err := os.WriteFile(filepath.Join(codexHome, "config.toml"), []byte(cfg), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	d := New()
	got, err := d.DiscoverGlobal(context.Background())
	if err != nil {
		t.Fatalf("DiscoverGlobal: %v", err)
	}
	names := map[string]bool{}
	for _, c := range got {
		if c.Kind == model.CapabilityKindMCP {
			names[c.Name] = true
		}
	}
	if !names["foo"] || !names["bar-baz"] {
		t.Fatalf("expected foo and bar-baz, got %v", names)
	}
}

func TestLaunchSpec_NoBinary(t *testing.T) {
	t.Setenv("PATH", "/nonexistent")
	t.Setenv("AVM_HOME", t.TempDir())
	d := New()
	a := &model.Agent{Identity: model.Identity{Name: "demo"}}
	if _, err := d.LaunchSpec(context.Background(), a, &runtime.Plan{}); err == nil {
		t.Fatalf("expected error when binary missing")
	}
}

func TestLaunchSpec_PopulatesEnv(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "codex")
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
		t.Fatalf("expected Stdin=true (interactive)")
	}
	if _, ok := spec.Env[EnvHome]; !ok {
		t.Fatalf("expected %s in env, got %+v", EnvHome, spec.Env)
	}
}
