package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xz1220/agent-vm/internal/config"
	"github.com/xz1220/agent-vm/internal/state"
)

func TestInitCreatesBaseDirsAndInitialState(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	out, err := executeCommand("init")
	if err != nil {
		t.Fatalf("init returned error: %v", err)
	}
	if out != "initialized avm home\n" {
		t.Fatalf("unexpected init output: %q", out)
	}

	for _, dir := range []string{
		config.AgentsDir(),
		config.EnvsDir(),
		config.RegistryDir(),
		config.RegistryKindDir("skills"),
		config.RegistryKindDir("mcps"),
		config.ActiveDir(),
		config.StateDir(),
		config.BackupDir(),
		cacheDir(),
	} {
		assertDirExists(t, dir)
	}
	assertFileExists(t, config.GlobalConfigPath())
	assertFileExists(t, config.AgentPath("default"))
	assertFileExists(t, config.EnvPath("default"))

	syncState, err := state.LoadSyncState(syncStatePath())
	if err != nil {
		t.Fatalf("load initial sync state: %v", err)
	}
	if syncState.Version != state.StateVersion {
		t.Fatalf("unexpected sync state version: %q", syncState.Version)
	}
	if syncState.LastActive != (config.ActiveRef{Kind: config.ActiveKindProfile, Name: "default"}) {
		t.Fatalf("unexpected initial active: %#v", syncState.LastActive)
	}
	if len(syncState.Runtimes) != 0 {
		t.Fatalf("initial sync state should not contain runtime state: %#v", syncState.Runtimes)
	}
}

func TestInitRepeatIsNoopAndForcePreservesExtraFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if _, err := executeCommand("init"); err != nil {
		t.Fatalf("initial init returned error: %v", err)
	}

	extraPath := filepath.Join(config.AvmDir(), "notes.txt")
	if err := os.WriteFile(extraPath, []byte("keep me\n"), 0o600); err != nil {
		t.Fatalf("write extra file: %v", err)
	}

	repeatOut, err := executeCommand("init")
	if err != nil {
		t.Fatalf("repeat init returned error: %v", err)
	}
	if repeatOut != "avm home already initialized\n" {
		t.Fatalf("unexpected repeat init output: %q", repeatOut)
	}
	assertFileContains(t, extraPath, "keep me")

	if err := config.UpdateActive(config.ActiveRef{Kind: config.ActiveKindProfile, Name: "custom"}); err != nil {
		t.Fatalf("set custom active: %v", err)
	}

	out, err := executeCommand("init", "--force")
	if err != nil {
		t.Fatalf("forced init returned error: %v", err)
	}
	if out != "initialized avm home\n" {
		t.Fatalf("unexpected forced init output: %q", out)
	}
	assertFileContains(t, extraPath, "keep me")

	cfg, err := config.ReadGlobalConfig()
	if err != nil {
		t.Fatalf("read global config after force: %v", err)
	}
	if cfg.Active != (config.ActiveRef{Kind: config.ActiveKindProfile, Name: "default"}) {
		t.Fatalf("forced init did not restore default active: %#v", cfg.Active)
	}

	syncState, err := state.LoadSyncState(syncStatePath())
	if err != nil {
		t.Fatalf("load sync state after force: %v", err)
	}
	if syncState.LastActive != cfg.Active {
		t.Fatalf("sync state active = %#v, want %#v", syncState.LastActive, cfg.Active)
	}
}

func assertDirExists(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("expected directory %s to exist: %v", path, err)
	}
	if !info.IsDir() {
		t.Fatalf("expected %s to be a directory", path)
	}
}

func assertFileExists(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("expected file %s to exist: %v", path, err)
	}
	if info.IsDir() {
		t.Fatalf("expected %s to be a file", path)
	}
}

func assertFileContains(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !strings.Contains(string(data), want) {
		t.Fatalf("%s did not contain %q:\n%s", path, want, string(data))
	}
}
