package claudecode

import (
	"context"
	"encoding/json"
	"errors"
	"io"
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
	bin := filepath.Join(dir, "claude")
	script := "#!/bin/sh\necho 1.2.3\nexit 0\n"
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
	if !strings.Contains(f.Version, "1.2.3") {
		t.Fatalf("Version=%q want to contain 1.2.3", f.Version)
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
	for _, k := range []string{EnvConfigDir, EnvPluginCache, EnvTmp, EnvDebugDir} {
		if _, ok := b.Env[k]; !ok {
			t.Errorf("missing env var %q", k)
		}
	}
	if b.Env[EnvConfigDir] != want {
		t.Errorf("%s=%q want %q", EnvConfigDir, b.Env[EnvConfigDir], want)
	}
}

func TestPlan_Mappings(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("AVM_HOME", tmp)
	d := New()
	a := &model.Agent{
		Identity: model.Identity{
			Name:        "demo",
			Description: "a demo agent",
			Role:        "writer",
		},
		Instructions: model.Instructions{System: "be terse"},
		Skills:       []model.CapabilityRef{{ID: "s1", Kind: model.CapabilityKindSkill}},
		MCP:          []model.CapabilityRef{{ID: "m1", Kind: model.CapabilityKindMCP}},
		Runtimes:     []model.RuntimePref{{Runtime: Name}},
	}
	plan, err := d.Plan(context.Background(), a)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(plan.Files) < 2 {
		t.Fatalf("expected CLAUDE.md + settings.json, got %d", len(plan.Files))
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
	// settings.json must contain mcpServers entry with id "m1".
	for _, f := range plan.Files {
		if filepath.Base(f.Path) == "settings.json" {
			var raw map[string]any
			if err := json.Unmarshal(f.Contents, &raw); err != nil {
				t.Fatalf("settings.json invalid: %v", err)
			}
			servers, ok := raw["mcpServers"].(map[string]any)
			if !ok {
				t.Fatalf("settings.json missing mcpServers map: %+v", raw)
			}
			if _, ok := servers["m1"]; !ok {
				t.Fatalf("expected mcpServers[m1], got %+v", servers)
			}
		}
	}
}

func TestDiscoverGlobal_Empty(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv(EnvConfigDir, t.TempDir())
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
	cfgDir := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv(EnvConfigDir, cfgDir)
	skill := filepath.Join(cfgDir, "skills", "writer")
	if err := os.MkdirAll(skill, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skill, "SKILL.md"), []byte("---\nversion: 0.1\n---\n"), 0o600); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
	cfg := `{"mcpServers":{"alpha":{"command":"true"},"beta":{"command":"true"}}}`
	if err := os.WriteFile(filepath.Join(cfgDir, ".claude.json"), []byte(cfg), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	d := New()
	got, err := d.DiscoverGlobal(context.Background())
	if err != nil {
		t.Fatalf("DiscoverGlobal: %v", err)
	}
	skillSeen := false
	mcps := map[string]bool{}
	for _, c := range got {
		switch c.Kind {
		case model.CapabilityKindSkill:
			if c.Name == "writer" {
				skillSeen = true
			}
		case model.CapabilityKindMCP:
			mcps[c.Name] = true
		}
	}
	if !skillSeen {
		t.Errorf("expected to find writer skill")
	}
	if !mcps["alpha"] || !mcps["beta"] {
		t.Errorf("expected alpha and beta MCP, got %v", mcps)
	}
}

func TestLaunchSpec_PopulatesEnv(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "claude")
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
	for _, k := range []string{EnvConfigDir, EnvPluginCache, EnvTmp, EnvDebugDir} {
		if _, ok := spec.Env[k]; !ok {
			t.Errorf("missing env var %q in launch spec", k)
		}
	}
	if !spec.Stdin {
		t.Fatalf("expected Stdin=true")
	}
}

func TestExportGlobal_Skill(t *testing.T) {
	cfgDir := t.TempDir()
	skillDir := filepath.Join(cfgDir, "skills", "review")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := "# review skill\nbe thorough\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Setenv(EnvConfigDir, cfgDir)
	t.Setenv("HOME", t.TempDir())

	d := New()
	exp, err := d.ExportGlobal(context.Background(), model.CapabilityKindSkill, "review")
	if err != nil {
		t.Fatalf("ExportGlobal: %v", err)
	}
	if exp.Format != model.PayloadFormatSkillMD || exp.Filename != "SKILL.md" {
		t.Fatalf("bad metadata: %+v", exp)
	}
	got, _ := io.ReadAll(exp.Content)
	exp.Content.Close()
	if string(got) != body {
		t.Fatalf("body mismatch")
	}
}

func TestExportGlobal_MCP(t *testing.T) {
	cfgDir := t.TempDir()
	t.Setenv(EnvConfigDir, cfgDir)
	t.Setenv("HOME", t.TempDir())

	cfg := map[string]any{
		"mcpServers": map[string]any{
			"gh": map[string]any{
				"command":     "npx",
				"args":        []any{"-y", "@modelcontextprotocol/server-github"},
				"env":         map[string]any{"GITHUB_TOKEN": "stub"},
				"weird_extra": 42.0,
			},
		},
	}
	raw, _ := json.Marshal(cfg)
	if err := os.WriteFile(filepath.Join(cfgDir, ".claude.json"), raw, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	d := New()
	exp, err := d.ExportGlobal(context.Background(), model.CapabilityKindMCP, "gh")
	if err != nil {
		t.Fatalf("ExportGlobal: %v", err)
	}
	if exp.Format != model.PayloadFormatMCPConfigV1 {
		t.Fatalf("Format=%q", exp.Format)
	}
	out, _ := io.ReadAll(exp.Content)
	exp.Content.Close()
	var cfg2 runtime.MCPConfigV1
	if err := json.Unmarshal(out, &cfg2); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, out)
	}
	if cfg2.Command != "npx" || len(cfg2.Args) != 2 {
		t.Fatalf("bad mcp config: %+v", cfg2)
	}
	if cfg2.Env["GITHUB_TOKEN"] != "stub" {
		t.Fatalf("env: %+v", cfg2.Env)
	}
	if _, ok := cfg2.Extra["weird_extra"]; !ok {
		t.Fatalf("expected weird_extra in Extra: %+v", cfg2.Extra)
	}
}

func TestExportGlobal_NotFound(t *testing.T) {
	cfgDir := t.TempDir()
	t.Setenv(EnvConfigDir, cfgDir)
	t.Setenv("HOME", t.TempDir())

	d := New()
	_, err := d.ExportGlobal(context.Background(), model.CapabilityKindSkill, "ghost")
	if !errors.Is(err, runtime.ErrGlobalCapabilityNotFound) {
		t.Fatalf("expected ErrGlobalCapabilityNotFound, got %v", err)
	}
}
