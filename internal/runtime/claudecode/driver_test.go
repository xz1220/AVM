package claudecode

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	d := New(nil)
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
	for _, k := range []string{"HOME", EnvConfigDir, EnvPluginCache, EnvTmp, EnvDebugDir} {
		if _, ok := b.Env[k]; !ok {
			t.Errorf("missing env var %q", k)
		}
	}
	if b.Env[EnvConfigDir] != want {
		t.Errorf("%s=%q want %q", EnvConfigDir, b.Env[EnvConfigDir], want)
	}
	// HOME isolation is what makes ~/.claude.json (oauthAccount, projects,
	// onboarding state, ...) per-Agent; it must point at the boundary.
	if b.Env["HOME"] != want {
		t.Errorf("HOME=%q want %q", b.Env["HOME"], want)
	}
}

func TestPlan_Mappings(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("AVM_HOME", tmp)
	t.Setenv("HOME", t.TempDir()) // keep readUserCredentials a no-op

	store := capstore.New(t.TempDir())
	skillID, err := store.Add(model.CapabilityRecord{
		Kind: model.CapabilityKindSkill, Name: "writer",
		Format: model.PayloadFormatSkillMD,
	}, bytes.NewReader([]byte("# writer skill\n")))
	if err != nil {
		t.Fatalf("Add skill: %v", err)
	}
	mcpBody, _ := json.Marshal(runtime.MCPConfigV1{
		Kind:    string(model.CapabilityKindMCP),
		Name:    "gh",
		Command: "npx",
		Args:    []string{"-y", "@modelcontextprotocol/server-github"},
		Env:     map[string]string{"GITHUB_TOKEN": "stub"},
	})
	mcpID, err := store.Add(model.CapabilityRecord{
		Kind: model.CapabilityKindMCP, Name: "gh",
		Format: model.PayloadFormatMCPConfigV1,
	}, bytes.NewReader(mcpBody))
	if err != nil {
		t.Fatalf("Add mcp: %v", err)
	}

	d := New(store)
	a := &model.Agent{
		Identity: model.Identity{
			Name:        "demo",
			Description: "a demo agent",
			Role:        "writer",
		},
		Instructions: model.Instructions{System: "be terse"},
		Skills:       []model.CapabilityRef{{ID: skillID, Kind: model.CapabilityKindSkill}},
		MCP:          []model.CapabilityRef{{ID: mcpID, Kind: model.CapabilityKindMCP}},
		Runtimes:     []model.RuntimePref{{Runtime: Name}},
	}
	plan, err := d.Plan(context.Background(), a)
	if err != nil {
		t.Fatalf("Plan: %v", err)
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

	files := map[string]runtime.ManagedFile{}
	for _, f := range plan.Files {
		files[f.Path] = f
	}
	bnd := filepath.Join(tmp, "boundaries", Name, "demo")
	if _, ok := files[filepath.Join(bnd, "CLAUDE.md")]; !ok {
		t.Fatalf("missing CLAUDE.md in plan files: %+v", files)
	}
	settingsPath := filepath.Join(bnd, "settings.json")
	settings, ok := files[settingsPath]
	if !ok {
		t.Fatalf("missing settings.json in plan files: %+v", files)
	}
	skillPath := filepath.Join(bnd, "skills", "writer", "SKILL.md")
	if _, ok := files[skillPath]; !ok {
		t.Fatalf("expected skill materialized at %s, got files=%+v", skillPath, files)
	}

	var raw map[string]any
	if err := json.Unmarshal(settings.Contents, &raw); err != nil {
		t.Fatalf("settings.json invalid: %v", err)
	}
	servers, ok := raw["mcpServers"].(map[string]any)
	if !ok {
		t.Fatalf("settings.json missing mcpServers map: %+v", raw)
	}
	// Key must be the capability name, not the cap_xxx ID.
	if _, byID := servers[string(mcpID)]; byID {
		t.Fatalf("mcpServers should not be keyed by cap ID, got %+v", servers)
	}
	gh, ok := servers["gh"].(map[string]any)
	if !ok {
		t.Fatalf("expected mcpServers[gh] map, got %+v", servers)
	}
	if gh["command"] != "npx" {
		t.Errorf("command=%v want npx", gh["command"])
	}
	args, ok := gh["args"].([]any)
	if !ok || len(args) != 2 {
		t.Errorf("args=%+v want length 2", gh["args"])
	}
	env, ok := gh["env"].(map[string]any)
	if !ok || env["GITHUB_TOKEN"] != "stub" {
		t.Errorf("env=%+v", gh["env"])
	}
}

func TestDiscoverGlobal_Empty(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv(EnvConfigDir, t.TempDir())
	d := New(nil)
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
	d := New(nil)
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
	t.Setenv("AVM_CLAUDE_TEST_INHERIT", "ok")
	d := New(nil)
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
	// Parent process environment must be inherited so Node-based
	// installs of `claude` can resolve `node` via PATH.
	if got := spec.Env["AVM_CLAUDE_TEST_INHERIT"]; got != "ok" {
		t.Errorf("expected parent env to be inherited (AVM_CLAUDE_TEST_INHERIT=ok), got %q", got)
	}
	if got := spec.Env["PATH"]; got == "" {
		t.Errorf("expected PATH inherited from parent, got empty")
	}
	// Boundary env must still win over any inherited copy.
	bnd := filepath.Join(os.Getenv("AVM_HOME"), "boundaries", Name, "demo")
	if spec.Env[EnvConfigDir] != bnd {
		t.Errorf("%s=%q want %q", EnvConfigDir, spec.Env[EnvConfigDir], bnd)
	}
	if !spec.Stdin {
		t.Fatalf("expected Stdin=true")
	}
}

func TestPlan_CopiesUserCredentials(t *testing.T) {
	avmHome := t.TempDir()
	t.Setenv("AVM_HOME", avmHome)

	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	credBody := []byte(`{"token":"sekret"}`)
	if err := os.WriteFile(filepath.Join(home, ".claude", ".credentials.json"), credBody, 0o600); err != nil {
		t.Fatalf("write creds: %v", err)
	}

	d := New(nil)
	plan, err := d.Plan(context.Background(), &model.Agent{
		Identity: model.Identity{Name: "demo"},
		Runtimes: []model.RuntimePref{{Runtime: Name}},
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	wantPath := filepath.Join(avmHome, "boundaries", Name, "demo", ".credentials.json")
	var found bool
	for _, f := range plan.Files {
		if f.Path == wantPath {
			found = true
			if string(f.Contents) != string(credBody) {
				t.Errorf("credentials body mismatch: %s vs %s", f.Contents, credBody)
			}
			if f.Mode != 0o600 {
				t.Errorf("creds mode=%v want 0o600", f.Mode)
			}
			break
		}
	}
	if !found {
		t.Fatalf("expected boundary credentials copy at %s, got files=%+v", wantPath, plan.Files)
	}
}

func TestPlan_CopiesUserSettingsLocal(t *testing.T) {
	avmHome := t.TempDir()
	t.Setenv("AVM_HOME", avmHome)

	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := []byte(`{"permissions":{"allow":["Bash(gh auth *)"]}}`)
	if err := os.WriteFile(filepath.Join(home, ".claude", "settings.local.json"), body, 0o600); err != nil {
		t.Fatalf("write settings.local: %v", err)
	}

	d := New(nil)
	plan, err := d.Plan(context.Background(), &model.Agent{
		Identity: model.Identity{Name: "demo"},
		Runtimes: []model.RuntimePref{{Runtime: Name}},
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	wantPath := filepath.Join(avmHome, "boundaries", Name, "demo", "settings.local.json")
	for _, f := range plan.Files {
		if f.Path == wantPath {
			if string(f.Contents) != string(body) {
				t.Errorf("settings.local body mismatch: %s", f.Contents)
			}
			return
		}
	}
	t.Fatalf("expected settings.local copy at %s, got files=%+v", wantPath, plan.Files)
}

func TestPlan_MergesUserSettings(t *testing.T) {
	avmHome := t.TempDir()
	t.Setenv("AVM_HOME", avmHome)

	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	userSettings := map[string]any{
		"model":       "opus[1m]",
		"theme":       "dark",
		"effortLevel": "xhigh",
		"permissions": map[string]any{"defaultMode": "auto"},
		// Stale mcpServers from user config; AVM must overwrite with its own.
		"mcpServers": map[string]any{"old": map[string]any{"command": "should-be-replaced"}},
	}
	rawSettings, _ := json.Marshal(userSettings)
	if err := os.WriteFile(filepath.Join(home, ".claude", "settings.json"), rawSettings, 0o600); err != nil {
		t.Fatalf("write user settings: %v", err)
	}

	store := capstore.New(t.TempDir())
	mcpBody, _ := json.Marshal(runtime.MCPConfigV1{
		Kind: string(model.CapabilityKindMCP), Name: "gh", Command: "npx",
	})
	mcpID, err := store.Add(model.CapabilityRecord{
		Kind: model.CapabilityKindMCP, Name: "gh",
		Format: model.PayloadFormatMCPConfigV1,
	}, bytes.NewReader(mcpBody))
	if err != nil {
		t.Fatalf("Add mcp: %v", err)
	}

	d := New(store)
	plan, err := d.Plan(context.Background(), &model.Agent{
		Identity: model.Identity{Name: "demo"},
		MCP:      []model.CapabilityRef{{ID: mcpID, Kind: model.CapabilityKindMCP}},
		Runtimes: []model.RuntimePref{{Runtime: Name}},
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	settingsPath := filepath.Join(avmHome, "boundaries", Name, "demo", "settings.json")
	var rendered []byte
	for _, f := range plan.Files {
		if f.Path == settingsPath {
			rendered = f.Contents
			break
		}
	}
	if rendered == nil {
		t.Fatalf("missing settings.json in plan files")
	}
	var got map[string]any
	if err := json.Unmarshal(rendered, &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, rendered)
	}
	// User-owned keys preserved.
	for _, k := range []string{"model", "theme", "effortLevel", "permissions"} {
		if _, ok := got[k]; !ok {
			t.Errorf("merged settings.json missing user key %q: %+v", k, got)
		}
	}
	if got["model"] != "opus[1m]" {
		t.Errorf("model=%v want opus[1m]", got["model"])
	}
	// AVM owns mcpServers — old user entry must be gone.
	servers, ok := got["mcpServers"].(map[string]any)
	if !ok {
		t.Fatalf("mcpServers not a map: %+v", got["mcpServers"])
	}
	if _, leaked := servers["old"]; leaked {
		t.Errorf("user's stale mcpServers leaked into rendered settings: %+v", servers)
	}
	if _, ok := servers["gh"]; !ok {
		t.Errorf("expected mcpServers[gh], got %+v", servers)
	}
}

func TestPlan_DropsLeakedMCPWhenAgentHasNone(t *testing.T) {
	avmHome := t.TempDir()
	t.Setenv("AVM_HOME", avmHome)

	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	user := map[string]any{
		"model":      "sonnet",
		"mcpServers": map[string]any{"corp": map[string]any{"command": "true"}},
	}
	raw, _ := json.Marshal(user)
	if err := os.WriteFile(filepath.Join(home, ".claude", "settings.json"), raw, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	d := New(nil)
	plan, err := d.Plan(context.Background(), &model.Agent{
		Identity: model.Identity{Name: "demo"},
		Runtimes: []model.RuntimePref{{Runtime: Name}},
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	settingsPath := filepath.Join(avmHome, "boundaries", Name, "demo", "settings.json")
	for _, f := range plan.Files {
		if f.Path != settingsPath {
			continue
		}
		var got map[string]any
		if err := json.Unmarshal(f.Contents, &got); err != nil {
			t.Fatalf("invalid JSON: %v", err)
		}
		if got["model"] != "sonnet" {
			t.Errorf("user model not preserved: %+v", got)
		}
		if _, leaked := got["mcpServers"]; leaked {
			t.Errorf("user mcpServers should be dropped when Agent has no MCP refs: %+v", got)
		}
		return
	}
	t.Fatalf("settings.json missing")
}

func TestPlan_BadUserSettings_ProducesWarning(t *testing.T) {
	avmHome := t.TempDir()
	t.Setenv("AVM_HOME", avmHome)

	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(home, ".claude", "settings.json"), []byte("{not valid json"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	d := New(nil)
	plan, err := d.Plan(context.Background(), &model.Agent{
		Identity: model.Identity{Name: "demo"},
		Runtimes: []model.RuntimePref{{Runtime: Name}},
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	var sawWarning bool
	for _, w := range plan.Warnings {
		if w.Code == "claude.settings-parse-failed" {
			sawWarning = true
			break
		}
	}
	if !sawWarning {
		t.Fatalf("expected claude.settings-parse-failed warning, got %+v", plan.Warnings)
	}
}

func TestPlan_CredentialsAbsent_NoWarning(t *testing.T) {
	t.Setenv("AVM_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir()) // no ~/.claude/.credentials.json present

	d := New(nil)
	plan, err := d.Plan(context.Background(), &model.Agent{
		Identity: model.Identity{Name: "demo"},
		Runtimes: []model.RuntimePref{{Runtime: Name}},
	})
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}
	for _, w := range plan.Warnings {
		if w.Code == "claude.creds-read-failed" {
			t.Fatalf("missing creds should be silent, got warning %+v", w)
		}
	}
}

func TestDiscoverGlobal_FollowsSymlinkSkills(t *testing.T) {
	// Real-world layout: ~/.claude/skills/<name> is a symlink into
	// ~/.agents/skills/<name>, where the actual SKILL.md lives. Earlier
	// versions of scanSkillDir filtered with DirEntry.IsDir(), which
	// returns false for symlinks, silently dropping every linked skill.
	cfgDir := t.TempDir()
	t.Setenv(EnvConfigDir, cfgDir)
	t.Setenv("HOME", t.TempDir())

	realRoot := t.TempDir()
	realSkill := filepath.Join(realRoot, "lark-doc")
	if err := os.MkdirAll(realSkill, 0o755); err != nil {
		t.Fatalf("mkdir real skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(realSkill, "SKILL.md"), []byte("---\nversion: 1.0\n---\n"), 0o600); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}

	skillsRoot := filepath.Join(cfgDir, "skills")
	if err := os.MkdirAll(skillsRoot, 0o755); err != nil {
		t.Fatalf("mkdir skills root: %v", err)
	}
	link := filepath.Join(skillsRoot, "lark-doc")
	if err := os.Symlink(realSkill, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	d := New(nil)
	got, err := d.DiscoverGlobal(context.Background())
	if err != nil {
		t.Fatalf("DiscoverGlobal: %v", err)
	}
	var found bool
	for _, c := range got {
		if c.Kind == model.CapabilityKindSkill && c.Name == "lark-doc" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected to find symlinked skill lark-doc, got %+v", got)
	}
}

func TestDiscoverGlobal_FindsPluginSkillsAndManagedSettings(t *testing.T) {
	cfgDir := t.TempDir()
	t.Setenv(EnvConfigDir, cfgDir)
	t.Setenv("HOME", t.TempDir())

	// Plugin-bundled skill.
	pluginSkill := filepath.Join(cfgDir, "plugins", "lark", "skills", "lark-doc")
	if err := os.MkdirAll(pluginSkill, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pluginSkill, "SKILL.md"), []byte("---\nversion: 2.0\n---\n"), 0o600); err != nil {
		t.Fatalf("write SKILL.md: %v", err)
	}

	// managed-settings.json with an MCP server an enterprise admin pushed.
	managed := `{"mcpServers":{"corp-vault":{"command":"true"}}}`
	if err := os.WriteFile(filepath.Join(cfgDir, "managed-settings.json"), []byte(managed), 0o600); err != nil {
		t.Fatalf("write managed-settings: %v", err)
	}

	d := New(nil)
	got, err := d.DiscoverGlobal(context.Background())
	if err != nil {
		t.Fatalf("DiscoverGlobal: %v", err)
	}
	skill, mcp := false, false
	for _, c := range got {
		if c.Kind == model.CapabilityKindSkill && c.Name == "lark-doc" {
			skill = true
		}
		if c.Kind == model.CapabilityKindMCP && c.Name == "corp-vault" {
			mcp = true
		}
	}
	if !skill {
		t.Errorf("expected plugin-bundled skill lark-doc, got %+v", got)
	}
	if !mcp {
		t.Errorf("expected managed-settings.json MCP corp-vault, got %+v", got)
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

	d := New(nil)
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

	d := New(nil)
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

	d := New(nil)
	_, err := d.ExportGlobal(context.Background(), model.CapabilityKindSkill, "ghost")
	if !errors.Is(err, runtime.ErrGlobalCapabilityNotFound) {
		t.Fatalf("expected ErrGlobalCapabilityNotFound, got %v", err)
	}
}
