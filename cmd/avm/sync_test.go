package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xz1220/agent-vm/internal/config"
	"github.com/xz1220/agent-vm/internal/state"
)

func TestSyncCommandUsesCurrentConfigActive(t *testing.T) {
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
	if err := os.WriteFile(currentActivePath(), []byte("profile:stale\n"), 0o600); err != nil {
		t.Fatalf("write stale current-active: %v", err)
	}

	out, err := executeCommand("sync")
	if err != nil {
		t.Fatalf("sync returned error: %v", err)
	}
	want := "active: profile:backend-coder\n" +
		"sync: completed\n" +
		"targets:\n" +
		"  codex: synced\n" +
		"warnings:\n" +
		"  none\n"
	if out != want {
		t.Fatalf("unexpected sync output:\n got: %q\nwant: %q", out, want)
	}

	cfg, err := config.ReadGlobalConfig()
	if err != nil {
		t.Fatalf("read global config: %v", err)
	}
	if cfg.Active != (config.ActiveRef{Kind: config.ActiveKindProfile, Name: "backend-coder"}) {
		t.Fatalf("sync changed active config: %#v", cfg.Active)
	}
	assertCurrentActive(t, "profile:backend-coder")
}

func TestSyncCommandReplacesExternalChangeInsideIsolatedRuntimeHome(t *testing.T) {
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

	codexHome := agentRuntimeHomeForTest(t, "backend-coder", "codex")
	rolePath := filepath.Join(codexHome, "agents", "backend-coder.toml")
	if err := os.WriteFile(rolePath, []byte("external change\n"), 0o600); err != nil {
		t.Fatalf("modify codex role: %v", err)
	}

	out, err := executeCommand("sync", "--target", "codex")
	if err != nil {
		t.Fatalf("sync returned error: %v\n%s", err, out)
	}
	if !strings.Contains(out, "sync: completed\n") || !strings.Contains(out, "  codex: synced\n") {
		t.Fatalf("sync output did not expose synced status:\n%s", out)
	}
	assertFileContains(t, rolePath, "developer_instructions")

	syncState, err := state.LoadSyncState(syncStatePath())
	if err != nil {
		t.Fatalf("load sync state: %v", err)
	}
	runtimeState := syncState.Runtimes["codex"]
	if runtimeState.Status != state.RuntimeStatusSynced {
		t.Fatalf("codex status = %q, want synced", runtimeState.Status)
	}
	if runtimeState.Error != "" {
		t.Fatalf("codex error = %q, want empty", runtimeState.Error)
	}

	cfg, err := config.ReadGlobalConfig()
	if err != nil {
		t.Fatalf("read global config: %v", err)
	}
	if cfg.Active != (config.ActiveRef{Kind: config.ActiveKindProfile, Name: "backend-coder"}) {
		t.Fatalf("sync conflict changed active config: %#v", cfg.Active)
	}
	assertCurrentActive(t, "profile:backend-coder")
}
