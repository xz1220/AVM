package boundary

import (
	"path/filepath"
	"testing"

	"github.com/xz1220/agent-vm/internal/config"
)

func TestResolveCodexBoundaryUsesAgentID(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	got, err := Resolve(Input{Runtime: "codex", AgentID: "agt_11111111111111111111111111111111", AgentName: "backend"})
	if err != nil {
		t.Fatalf("resolve codex boundary: %v", err)
	}
	wantRoot := config.AgentRuntimeHomeDir("agt_11111111111111111111111111111111", "codex")
	if got.Root != wantRoot {
		t.Fatalf("root = %q, want %q", got.Root, wantRoot)
	}
	if got.Env["CODEX_HOME"] != wantRoot || got.RunEnv["CODEX_HOME"] != wantRoot {
		t.Fatalf("codex env did not use root: %#v %#v", got.Env, got.RunEnv)
	}
	if got.Isolation != IsolationIsolated || got.BoundaryType != BoundaryRuntimeHome {
		t.Fatalf("unexpected isolation metadata: %#v", got)
	}
}

func TestResolveOpenCodeBoundarySeparatesShellAndRunEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	got, err := Resolve(Input{Runtime: "opencode", AgentID: "agt_22222222222222222222222222222222", AgentName: "builder"})
	if err != nil {
		t.Fatalf("resolve opencode boundary: %v", err)
	}
	configPath := filepath.Join(got.Root, "config", "opencode.json")
	if got.Env["OPENCODE_CONFIG"] != configPath {
		t.Fatalf("activate env config = %q, want %q", got.Env["OPENCODE_CONFIG"], configPath)
	}
	if _, ok := got.Env["XDG_DATA_HOME"]; ok {
		t.Fatalf("activate env should not contain XDG_DATA_HOME: %#v", got.Env)
	}
	if got.RunEnv["OPENCODE_DB"] != filepath.Join(got.Root, "data", "opencode.db") {
		t.Fatalf("run env missing OPENCODE_DB: %#v", got.RunEnv)
	}
	if got.RunEnv["XDG_DATA_HOME"] != filepath.Join(got.Root, "xdg-data") {
		t.Fatalf("run env missing XDG_DATA_HOME: %#v", got.RunEnv)
	}
	if got.Isolation != IsolationIsolated || got.BoundaryType != BoundaryProcessEnv {
		t.Fatalf("unexpected isolation metadata: %#v", got)
	}
}
