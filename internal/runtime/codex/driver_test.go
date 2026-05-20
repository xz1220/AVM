package codex

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"

	"github.com/xz1220/agent-vm/internal/app/model"
	"github.com/xz1220/agent-vm/internal/infra/capstore"
	"github.com/xz1220/agent-vm/internal/runtime"
)

func TestName(t *testing.T) {
	if got := New(nil).Name(); got != Name {
		t.Fatalf("Name=%q want %q", got, Name)
	}
}

func TestFacts_BinaryMissing(t *testing.T) {
	t.Setenv("PATH", "/nonexistent")
	d := New(nil)
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
	d := New(nil)
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
	d := New(nil)
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
	if v := b.Env["HOME"]; v != want {
		t.Fatalf("env[HOME]=%q want %q", v, want)
	}
}

func TestBoundary_RejectsEmptyName(t *testing.T) {
	d := New(nil)
	if _, err := d.Boundary(context.Background(), &model.Agent{}); err == nil {
		t.Fatalf("expected error for empty agent name")
	}
}

func TestPlan_FieldMappings(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("AVM_HOME", tmp)
	t.Setenv("HOME", t.TempDir()) // keep auth.json read deterministic

	caps := capstore.New(t.TempDir())
	skillID := addSkillCap(t, caps, "s1", "# s1\n")
	mcpID := addMCPCap(t, caps, "m1", runtime.MCPConfigV1{Command: "true"})

	d := New(caps)
	a := &model.Agent{
		Identity: model.Identity{
			Name:        "demo",
			Description: "desc",
			Role:        "reviewer",
		},
		Instructions: model.Instructions{System: "be helpful"},
		Skills:       []model.CapabilityRef{{ID: skillID, Kind: model.CapabilityKindSkill}},
		MCP:          []model.CapabilityRef{{ID: mcpID, Kind: model.CapabilityKindMCP}},
		Runtimes:     []model.RuntimePref{{Runtime: Name, Default: true}},
	}
	plan, err := d.Plan(context.Background(), a)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	if len(plan.Files) < 3 {
		t.Fatalf("expected at least AGENTS.md + config.toml + skill, got %d files", len(plan.Files))
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

func TestPlan_WritesAgentsMDUnderWorkspace(t *testing.T) {
	tmp := t.TempDir()
	callerCwd := t.TempDir()
	t.Setenv("AVM_HOME", tmp)
	t.Setenv("HOME", t.TempDir())

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })
	if err := os.Chdir(callerCwd); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	resolvedCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd after Chdir: %v", err)
	}

	d := New(nil)
	a := &model.Agent{
		Identity:     model.Identity{Name: "demo"},
		Instructions: model.Instructions{System: "stay focused"},
	}
	plan, err := d.Plan(context.Background(), a)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	bnd, _ := d.Boundary(context.Background(), a)
	wantPath := filepath.Join(bnd.StateDir, workspaceDirName, "AGENTS.md")
	var agentsFile *runtime.ManagedFile
	for i := range plan.Files {
		if plan.Files[i].Path == wantPath {
			agentsFile = &plan.Files[i]
			break
		}
	}
	if agentsFile == nil {
		t.Fatalf("AGENTS.md missing at %s; files=%+v", wantPath, plan.Files)
	}
	contents := string(agentsFile.Contents)
	if !strings.Contains(contents, "The user-authorized working directory is `"+resolvedCwd+"`") {
		t.Fatalf("AGENTS.md missing user cwd %q\n--- AGENTS.md ---\n%s", resolvedCwd, contents)
	}
}

func TestPlan_RejectsInvalidName(t *testing.T) {
	d := New(nil)
	if _, err := d.Plan(context.Background(), &model.Agent{Identity: model.Identity{Name: "BAD NAME"}}); err == nil {
		t.Fatalf("expected validation error")
	}
}

func TestDiscoverGlobal_MissingDirIsEmpty(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv(EnvHome, t.TempDir())
	d := New(nil)
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
	d := New(nil)
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
	d := New(nil)
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

func TestDiscoverGlobal_MCPNestedEnvNotSeparate(t *testing.T) {
	codexHome := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv(EnvHome, codexHome)
	cfg := `[mcp_servers.gh]
command = "echo"
args = ["x"]

[mcp_servers.gh.env]
GITHUB_TOKEN = "secret"
`
	if err := os.WriteFile(filepath.Join(codexHome, "config.toml"), []byte(cfg), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	d := New(nil)
	got, err := d.DiscoverGlobal(context.Background())
	if err != nil {
		t.Fatalf("DiscoverGlobal: %v", err)
	}
	mcps := []string{}
	for _, c := range got {
		if c.Kind == model.CapabilityKindMCP {
			mcps = append(mcps, c.Name)
		}
	}
	if len(mcps) != 1 || mcps[0] != "gh" {
		t.Fatalf("expected exactly one MCP named gh, got %v", mcps)
	}
}

func TestLaunchSpec_NoBinary(t *testing.T) {
	t.Setenv("PATH", "/nonexistent")
	t.Setenv("AVM_HOME", t.TempDir())
	d := New(nil)
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
	d := New(nil)
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
	if _, ok := spec.Env["HOME"]; !ok {
		t.Fatalf("expected HOME in env, got %+v", spec.Env)
	}
}

// Codex on npm/nvm installs is a `#!/usr/bin/env node` shebang, so the
// spawned child needs PATH (and other parent vars) to resolve `node`.
// Runtime-owned env vars are still forced to the boundary so user-global
// Codex state does not leak into the Agent run.
func TestLaunchSpec_InheritsParentEnvAndOverridesBoundaryVars(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "codex")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake bin: %v", err)
	}
	avmHome := t.TempDir()
	t.Setenv("PATH", dir)
	t.Setenv("AVM_HOME", avmHome)
	t.Setenv(EnvHome, "/should/be/overridden") // simulate user shell already setting CODEX_HOME
	t.Setenv("HOME", "/user/home/should/not/leak")

	d := New(nil)
	a := &model.Agent{Identity: model.Identity{Name: "demo"}}
	spec, err := d.LaunchSpec(context.Background(), a, &runtime.Plan{})
	if err != nil {
		t.Fatalf("LaunchSpec: %v", err)
	}
	// PATH must be inherited so codex's `#!/usr/bin/env node` shebang works.
	if got := spec.Env["PATH"]; got != dir {
		t.Errorf("expected PATH=%q (inherited from parent), got %q", dir, got)
	}
	// CODEX_HOME must be overridden to the boundary, not the parent value.
	wantBoundary := filepath.Join(avmHome, "boundaries", Name, "demo")
	if got := spec.Env[EnvHome]; got != wantBoundary {
		t.Errorf("expected %s=%q (boundary override), got %q", EnvHome, wantBoundary, got)
	}
	// HOME must be isolated too because Codex scans ~/.agents/skills in
	// addition to CODEX_HOME/skills.
	if got := spec.Env["HOME"]; got != wantBoundary {
		t.Errorf("expected HOME=%q (boundary override), got %q", wantBoundary, got)
	}
}

// Codex uses --cd as the discovery root for project-local config, project
// skills, repo skills, and AGENTS.md. LaunchSpec must therefore pin --cd to
// the AVM-owned workspace and expose the caller cwd only through --add-dir.
func TestLaunchSpec_CdPinnedToWorkspaceAndUserCwdAsAddDir(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "codex")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake bin: %v", err)
	}
	avmHome := t.TempDir()
	callerCwd := t.TempDir()
	t.Setenv("PATH", dir)
	t.Setenv("AVM_HOME", avmHome)

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })
	if err := os.Chdir(callerCwd); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	// Resolve symlinks so the comparison matches what os.Getwd returns
	// inside the driver on platforms (macOS) where TempDir lives under
	// /var → /private/var.
	resolvedCwd, err := filepath.EvalSymlinks(callerCwd)
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}

	d := New(nil)
	a := &model.Agent{Identity: model.Identity{Name: "demo"}}
	spec, err := d.LaunchSpec(context.Background(), a, &runtime.Plan{})
	if err != nil {
		t.Fatalf("LaunchSpec: %v", err)
	}

	wantWorkspace := filepath.Join(avmHome, "boundaries", Name, "demo", workspaceDirName)
	if spec.Workdir != wantWorkspace {
		t.Errorf("Workdir=%q want %q", spec.Workdir, wantWorkspace)
	}
	if got, ok := argValue(spec.Args, "--cd"); !ok || got != wantWorkspace {
		t.Fatalf("Args=%v want --cd %s", spec.Args, wantWorkspace)
	}
	if got, ok := argValue(spec.Args, "--add-dir"); !ok || got != resolvedCwd {
		t.Fatalf("Args=%v want --add-dir %s", spec.Args, resolvedCwd)
	}
}

func TestLaunchSpec_AddDirOmittedWhenCallerCwdInsideBoundary(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "codex")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake bin: %v", err)
	}
	avmHome := t.TempDir()
	t.Setenv("PATH", dir)
	t.Setenv("AVM_HOME", avmHome)

	boundary := filepath.Join(avmHome, "boundaries", Name, "demo")
	workspace := filepath.Join(boundary, workspaceDirName)
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("MkdirAll workspace: %v", err)
	}
	boundarySubdir := filepath.Join(boundary, "memories", "scratch")
	if err := os.MkdirAll(boundarySubdir, 0o755); err != nil {
		t.Fatalf("MkdirAll boundary subdir: %v", err)
	}
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })
	if err := os.Chdir(boundarySubdir); err != nil {
		t.Fatalf("Chdir boundary subdir: %v", err)
	}

	d := New(nil)
	a := &model.Agent{Identity: model.Identity{Name: "demo"}}
	spec, err := d.LaunchSpec(context.Background(), a, &runtime.Plan{})
	if err != nil {
		t.Fatalf("LaunchSpec: %v", err)
	}
	if got, ok := argValue(spec.Args, "--cd"); !ok || got != workspace {
		t.Fatalf("Args=%v want --cd %s", spec.Args, workspace)
	}
	if _, ok := argValue(spec.Args, "--add-dir"); ok {
		t.Fatalf("Args=%v should omit --add-dir when caller cwd is inside boundary", spec.Args)
	}
}

func TestExportGlobal_Skill(t *testing.T) {
	codexHome := t.TempDir()
	skillDir := filepath.Join(codexHome, "skills", "hello")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	body := "# hello content\nthis is a skill\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}
	t.Setenv(EnvHome, codexHome)
	t.Setenv("HOME", t.TempDir()) // suppress ~/.agents/skills scan

	d := New(nil)
	exported, err := d.ExportGlobal(context.Background(), model.CapabilityKindSkill, "hello")
	if err != nil {
		t.Fatalf("ExportGlobal: %v", err)
	}
	if exported.Format != model.PayloadFormatSkillMD {
		t.Fatalf("Format=%q want %q", exported.Format, model.PayloadFormatSkillMD)
	}
	if exported.Filename != "SKILL.md" {
		t.Fatalf("Filename=%q want SKILL.md", exported.Filename)
	}
	got, err := io.ReadAll(exported.Content)
	exported.Content.Close()
	if err != nil {
		t.Fatalf("read content: %v", err)
	}
	if string(got) != body {
		t.Fatalf("body=%q want %q", got, body)
	}
}

func TestExportGlobal_MCP(t *testing.T) {
	codexHome := t.TempDir()
	tomlBody := `
[mcp_servers.gh]
command = "npx"
args = ["-y", "@modelcontextprotocol/server-github"]
unknown_field = "kept-in-extra"

[mcp_servers.gh.env]
GITHUB_TOKEN = "secret-redacted"
`
	if err := os.WriteFile(filepath.Join(codexHome, "config.toml"), []byte(tomlBody), 0o644); err != nil {
		t.Fatalf("write config.toml: %v", err)
	}
	t.Setenv(EnvHome, codexHome)
	t.Setenv("HOME", t.TempDir())

	d := New(nil)
	exported, err := d.ExportGlobal(context.Background(), model.CapabilityKindMCP, "gh")
	if err != nil {
		t.Fatalf("ExportGlobal: %v", err)
	}
	if exported.Format != model.PayloadFormatMCPConfigV1 {
		t.Fatalf("Format=%q want %q", exported.Format, model.PayloadFormatMCPConfigV1)
	}
	if exported.Filename != "mcp.json" {
		t.Fatalf("Filename=%q want mcp.json", exported.Filename)
	}
	raw, err := io.ReadAll(exported.Content)
	exported.Content.Close()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var got runtime.MCPConfigV1
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, raw)
	}
	if got.Kind != string(model.CapabilityKindMCP) || got.Name != "gh" {
		t.Fatalf("kind/name wrong: %+v", got)
	}
	if got.Command != "npx" {
		t.Fatalf("Command=%q want npx", got.Command)
	}
	if len(got.Args) != 2 || got.Args[1] != "@modelcontextprotocol/server-github" {
		t.Fatalf("Args=%v", got.Args)
	}
	if got.Env["GITHUB_TOKEN"] != "secret-redacted" {
		t.Fatalf("Env=%v", got.Env)
	}
	if got.Extra["unknown_field"] != "kept-in-extra" {
		t.Fatalf("expected unknown_field in Extra, got %+v", got.Extra)
	}
}

func TestExportGlobal_NotFound(t *testing.T) {
	codexHome := t.TempDir()
	t.Setenv(EnvHome, codexHome)
	t.Setenv("HOME", t.TempDir())

	d := New(nil)
	_, err := d.ExportGlobal(context.Background(), model.CapabilityKindSkill, "ghost")
	if !errors.Is(err, runtime.ErrGlobalCapabilityNotFound) {
		t.Fatalf("expected ErrGlobalCapabilityNotFound, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Plan-level materialization tests (skills, MCP, auth.json)
// ---------------------------------------------------------------------------

func addSkillCap(t *testing.T, caps capstore.Store, name, body string) model.CapabilityID {
	t.Helper()
	rec := model.CapabilityRecord{
		Kind:   model.CapabilityKindSkill,
		Name:   name,
		Format: model.PayloadFormatSkillMD,
	}
	id, err := caps.Add(rec, strings.NewReader(body))
	if err != nil {
		t.Fatalf("add skill cap: %v", err)
	}
	return id
}

func addMCPCap(t *testing.T, caps capstore.Store, name string, cfg runtime.MCPConfigV1) model.CapabilityID {
	t.Helper()
	if cfg.Name == "" {
		cfg.Name = name
	}
	if cfg.Kind == "" {
		cfg.Kind = string(model.CapabilityKindMCP)
	}
	body, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal mcp cfg: %v", err)
	}
	rec := model.CapabilityRecord{
		Kind:   model.CapabilityKindMCP,
		Name:   name,
		Format: model.PayloadFormatMCPConfigV1,
	}
	id, err := caps.Add(rec, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("add mcp cap: %v", err)
	}
	return id
}

func TestPlan_SkillsMaterialize(t *testing.T) {
	t.Setenv("AVM_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())

	caps := capstore.New(t.TempDir())
	body := "# hello\nbody bytes\n"
	id := addSkillCap(t, caps, "hello", body)

	d := New(caps)
	a := &model.Agent{
		Identity: model.Identity{Name: "demo"},
		Skills:   []model.CapabilityRef{{ID: id, Kind: model.CapabilityKindSkill}},
	}
	plan, err := d.Plan(context.Background(), a)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	bnd, _ := d.Boundary(context.Background(), a)
	wantPath := filepath.Join(bnd.StateDir, "skills", "hello", "SKILL.md")
	var found *runtime.ManagedFile
	for i := range plan.Files {
		if plan.Files[i].Path == wantPath {
			found = &plan.Files[i]
			break
		}
	}
	if found == nil {
		t.Fatalf("expected skill ManagedFile at %s; got %+v", wantPath, plan.Files)
	}
	if string(found.Contents) != body {
		t.Fatalf("skill bytes mismatch: got %q want %q", found.Contents, body)
	}
	if found.Mode != 0o644 {
		t.Fatalf("Mode=%o want 0o644", found.Mode)
	}
}

func TestPlan_MCPRendersFullSection(t *testing.T) {
	t.Setenv("AVM_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())

	caps := capstore.New(t.TempDir())
	id := addMCPCap(t, caps, "github", runtime.MCPConfigV1{
		Command: "npx",
		Args:    []string{"-y", "@modelcontextprotocol/server-github"},
		Env:     map[string]string{"GITHUB_TOKEN": "secret"},
	})

	d := New(caps)
	a := &model.Agent{
		Identity: model.Identity{Name: "demo"},
		MCP:      []model.CapabilityRef{{ID: id, Kind: model.CapabilityKindMCP}},
	}
	plan, err := d.Plan(context.Background(), a)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	bnd, _ := d.Boundary(context.Background(), a)
	cfgPath := filepath.Join(bnd.StateDir, "config.toml")
	var cfgFile *runtime.ManagedFile
	for i := range plan.Files {
		if plan.Files[i].Path == cfgPath {
			cfgFile = &plan.Files[i]
			break
		}
	}
	if cfgFile == nil {
		t.Fatalf("config.toml missing from plan: %+v", plan.Files)
	}
	cfg := string(cfgFile.Contents)
	for _, want := range []string{
		`[mcp_servers."github"]`,
		`command = "npx"`,
		`"-y"`,
		`"@modelcontextprotocol/server-github"`,
		`"GITHUB_TOKEN" = "secret"`,
	} {
		if !strings.Contains(cfg, want) {
			t.Errorf("config.toml missing %q\n--- config.toml ---\n%s", want, cfg)
		}
	}
}

func parseBoundaryConfig(t *testing.T, raw string) map[string]any {
	t.Helper()
	var doc map[string]any
	if _, err := toml.Decode(raw, &doc); err != nil {
		t.Fatalf("parse rendered config.toml: %v\n--- raw ---\n%s", err, raw)
	}
	return doc
}

func TestRenderConfigTOML_PinsEmptyProjectRootMarkersAtTopLevel(t *testing.T) {
	doc := parseBoundaryConfig(t, renderConfigTOML(nil))
	v, ok := doc["project_root_markers"]
	if !ok {
		t.Fatalf("project_root_markers must be a top-level key; got keys=%v", topLevelKeys(doc))
	}
	arr, ok := v.([]any)
	if !ok {
		t.Fatalf("project_root_markers must be an array, got %T (%v)", v, v)
	}
	if len(arr) != 0 {
		t.Errorf("project_root_markers must be empty, got %v", arr)
	}
}

func TestRenderConfigTOML_PinsDefaultPermissionsAtTopLevel(t *testing.T) {
	doc := parseBoundaryConfig(t, renderConfigTOML(nil))
	v, ok := doc["default_permissions"]
	if !ok {
		t.Fatalf("default_permissions must be a top-level key; got keys=%v", topLevelKeys(doc))
	}
	got, ok := v.(string)
	if !ok {
		t.Fatalf("default_permissions must be a string, got %T (%v)", v, v)
	}
	if got != ":workspace" {
		t.Errorf("default_permissions=%q want \":workspace\"", got)
	}
}

func TestRenderConfigTOML_TopLevelKeysNotNestedUnderSections(t *testing.T) {
	doc := parseBoundaryConfig(t, renderConfigTOML(nil))
	for sectionName, val := range doc {
		section, ok := val.(map[string]any)
		if !ok {
			continue
		}
		for _, forbidden := range []string{"project_root_markers", "default_permissions"} {
			if _, nested := section[forbidden]; nested {
				t.Errorf("%q must be top-level, found nested under [%s]", forbidden, sectionName)
			}
		}
	}
}

func topLevelKeys(doc map[string]any) []string {
	keys := make([]string, 0, len(doc))
	for k := range doc {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func TestLaunchSpec_DoesNotLeakUserDotCodexDiscovery(t *testing.T) {
	fakeHome := t.TempDir()
	leakyCfg := []byte(`model = "leaked-model"
notify = ["mail"]
approval_policy = "never"
`)
	if err := os.MkdirAll(filepath.Join(fakeHome, ".codex", "skills", "leaky"), 0o755); err != nil {
		t.Fatalf("mkdir leaky skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(fakeHome, ".codex", "config.toml"), leakyCfg, 0o600); err != nil {
		t.Fatalf("write leaky config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(fakeHome, ".codex", "skills", "leaky", "SKILL.md"),
		[]byte("---\nname: leaky\n---\n"), 0o600); err != nil {
		t.Fatalf("write leaky skill: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(fakeHome, ".agents", "skills", "leaky2"), 0o755); err != nil {
		t.Fatalf("mkdir leaky2: %v", err)
	}
	if err := os.WriteFile(filepath.Join(fakeHome, ".agents", "skills", "leaky2", "SKILL.md"),
		[]byte("---\nname: leaky2\n---\n"), 0o600); err != nil {
		t.Fatalf("write leaky2 skill: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(fakeHome, ".git"), 0o755); err != nil {
		t.Fatalf("mkdir .git marker: %v", err)
	}

	binDir := t.TempDir()
	bin := filepath.Join(binDir, "codex")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write fake bin: %v", err)
	}

	avmHome := filepath.Join(fakeHome, ".avm")
	t.Setenv("PATH", binDir)
	t.Setenv("HOME", fakeHome)
	t.Setenv("AVM_HOME", avmHome)

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })
	if err := os.Chdir(fakeHome); err != nil {
		t.Fatalf("Chdir to fakeHome: %v", err)
	}

	d := New(capstore.New(t.TempDir()))
	a := &model.Agent{Identity: model.Identity{Name: "mbti"}}
	plan, err := d.Plan(context.Background(), a)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	spec, err := d.LaunchSpec(context.Background(), a, plan)
	if err != nil {
		t.Fatalf("LaunchSpec: %v", err)
	}

	wantBoundary := filepath.Join(avmHome, "boundaries", Name, "mbti")
	wantWorkspace := filepath.Join(wantBoundary, workspaceDirName)
	resolvedFakeHome, err := filepath.EvalSymlinks(fakeHome)
	if err != nil {
		t.Fatalf("EvalSymlinks fakeHome: %v", err)
	}

	if got, ok := argValue(spec.Args, "--cd"); !ok || got != wantWorkspace {
		t.Fatalf("Args=%v want --cd %s", spec.Args, wantWorkspace)
	}
	if got, ok := argValue(spec.Args, "--add-dir"); !ok || got != resolvedFakeHome {
		t.Fatalf("Args=%v want --add-dir %s", spec.Args, resolvedFakeHome)
	}
	if spec.Env[EnvHome] != wantBoundary {
		t.Errorf("%s=%q want %q", EnvHome, spec.Env[EnvHome], wantBoundary)
	}
	if spec.Env["HOME"] != wantBoundary {
		t.Errorf("HOME=%q want %q", spec.Env["HOME"], wantBoundary)
	}

	agentsPath := filepath.Join(wantWorkspace, "AGENTS.md")
	cfgPath := filepath.Join(wantBoundary, "config.toml")
	var cfg string
	foundAgents := false
	for _, f := range plan.Files {
		switch f.Path {
		case agentsPath:
			foundAgents = true
		case cfgPath:
			cfg = string(f.Contents)
		}
	}
	if !foundAgents {
		t.Errorf("workspace AGENTS.md not in Plan.Files at %s", agentsPath)
	}
	if cfg == "" {
		t.Fatalf("boundary config.toml not in Plan.Files")
	}
	doc := parseBoundaryConfig(t, cfg)
	if v, ok := doc["project_root_markers"].([]any); !ok || len(v) != 0 {
		t.Errorf("project_root_markers top-level key missing or non-empty; doc=%+v", doc)
	}
	if v, ok := doc["default_permissions"].(string); !ok || v != ":workspace" {
		t.Errorf("default_permissions top-level key missing or not :workspace; doc=%+v", doc)
	}
}

func TestPlan_MCPDuplicateName(t *testing.T) {
	t.Setenv("AVM_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())

	caps := capstore.New(t.TempDir())
	id1 := addMCPCap(t, caps, "github", runtime.MCPConfigV1{Command: "v1"})
	id2 := addMCPCap(t, caps, "github", runtime.MCPConfigV1{Command: "v2"})
	if id1 == id2 {
		t.Fatalf("expected distinct IDs for distinct content")
	}

	d := New(caps)
	a := &model.Agent{
		Identity: model.Identity{Name: "demo"},
		MCP: []model.CapabilityRef{
			{ID: id1, Kind: model.CapabilityKindMCP},
			{ID: id2, Kind: model.CapabilityKindMCP},
		},
	}
	if _, err := d.Plan(context.Background(), a); err == nil {
		t.Fatalf("expected duplicate-name error")
	} else if !strings.Contains(err.Error(), "github") {
		t.Fatalf("error should reference duplicate name; got %v", err)
	}
}

func TestPlan_AuthJSON_Present(t *testing.T) {
	t.Setenv("AVM_HOME", t.TempDir())
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0o755); err != nil {
		t.Fatalf("mkdir .codex: %v", err)
	}
	auth := []byte(`{"token":"redacted"}`)
	if err := os.WriteFile(filepath.Join(home, ".codex", "auth.json"), auth, 0o600); err != nil {
		t.Fatalf("write auth.json: %v", err)
	}

	d := New(nil)
	a := &model.Agent{Identity: model.Identity{Name: "demo"}}
	plan, err := d.Plan(context.Background(), a)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	bnd, _ := d.Boundary(context.Background(), a)
	wantPath := filepath.Join(bnd.StateDir, "auth.json")
	var got *runtime.ManagedFile
	for i := range plan.Files {
		if plan.Files[i].Path == wantPath {
			got = &plan.Files[i]
			break
		}
	}
	if got == nil {
		t.Fatalf("expected auth.json in plan.Files at %s", wantPath)
	}
	if !bytes.Equal(got.Contents, auth) {
		t.Fatalf("auth bytes mismatch: %q vs %q", got.Contents, auth)
	}
	if got.Mode != 0o600 {
		t.Fatalf("Mode=%o want 0o600", got.Mode)
	}
}

func TestPlan_AuthJSON_Missing(t *testing.T) {
	t.Setenv("AVM_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir()) // empty home, no ~/.codex

	d := New(nil)
	a := &model.Agent{Identity: model.Identity{Name: "demo"}}
	plan, err := d.Plan(context.Background(), a)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	bnd, _ := d.Boundary(context.Background(), a)
	wantPath := filepath.Join(bnd.StateDir, "auth.json")
	for _, f := range plan.Files {
		if f.Path == wantPath {
			t.Fatalf("auth.json should NOT be in plan when source missing; found %s", f.Path)
		}
	}
}

func argValue(args []string, flag string) (string, bool) {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == flag {
			return args[i+1], true
		}
	}
	return "", false
}
