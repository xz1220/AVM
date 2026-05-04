package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnvironmentYAMLRejectsCapabilities(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)

	writeEnvTestYAML(t, EnvPath("bad-capabilities"), `name: bad-capabilities
version: 1.0.0
runtime_agents:
  codex:
    primary: default
targets:
  - codex
capabilities:
  skills:
    - git
`)
	if _, err := ReadEnvironment("bad-capabilities"); err == nil || !strings.Contains(err.Error(), "capabilities") {
		t.Fatalf("expected capabilities decode error for global env, got %v", err)
	}

	writeEnvTestYAML(t, ProjectEnvPath(project), `extends: backend-dev
capabilities:
  skills:
    - git
`)
	if _, err := ReadProjectOverride(project); err == nil || !strings.Contains(err.Error(), "capabilities") {
		t.Fatalf("expected capabilities decode error for project env override, got %v", err)
	}
}

func writeEnvTestYAML(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
