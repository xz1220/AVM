package main

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRunPrintsOpenCodeProcessIsolationEnv(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	chdir(t, project)

	if _, err := executeCommand("init"); err != nil {
		t.Fatalf("init returned error: %v", err)
	}
	if _, err := executeCommand("agent", "create", "opencode-agent", "--runtime", "opencode"); err != nil {
		t.Fatalf("agent create returned error: %v", err)
	}
	if out, err := executeCommand("use", "opencode-agent"); err != nil {
		t.Fatalf("use returned error: %v\n%s", err, out)
	}

	runtimeHome := agentRuntimeHomeForTest(t, "opencode-agent", "opencode")
	activateOut, err := executeCommand("activate", "opencode-agent")
	if err != nil {
		t.Fatalf("activate returned error: %v\n%s", err, activateOut)
	}
	if strings.Contains(activateOut, "OPENCODE_DB") || strings.Contains(activateOut, "XDG_DATA_HOME") {
		t.Fatalf("activate should not export OpenCode process-only isolation env:\n%s", activateOut)
	}

	out, err := executeCommand("run", "opencode", "--print-env")
	if err != nil {
		t.Fatalf("run --print-env returned error: %v\n%s", err, out)
	}
	for _, want := range []string{
		"OPENCODE_CONFIG=" + filepath.Join(runtimeHome, "config", "opencode.json"),
		"OPENCODE_CONFIG_DIR=" + filepath.Join(runtimeHome, "config"),
		"OPENCODE_DB=" + filepath.Join(runtimeHome, "data", "opencode.db"),
		"XDG_CACHE_HOME=" + filepath.Join(runtimeHome, "xdg-cache"),
		"XDG_DATA_HOME=" + filepath.Join(runtimeHome, "xdg-data"),
		"XDG_STATE_HOME=" + filepath.Join(runtimeHome, "xdg-state"),
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("run env missing %q:\n%s", want, out)
		}
	}
	assertPathExistsForTest(t, filepath.Join(runtimeHome, "config", "opencode.json"))
	assertPathExistsForTest(t, filepath.Join(runtimeHome, "config", "agents", "opencode-agent.md"))
	assertPathExistsForTest(t, filepath.Join(runtimeHome, "data"))
	assertPathExistsForTest(t, filepath.Join(runtimeHome, "xdg-data"))
}
