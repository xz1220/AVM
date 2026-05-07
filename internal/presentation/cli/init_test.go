package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xz1220/agent-vm/internal/app/model"
)

func TestDoctor_Human(t *testing.T) {
	deps := newTestDeps(nil, nil, nil, nil, nil)
	out, _, err := runCmd(t, deps, "doctor")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	for _, want := range []string{"AVM home:", "PATH:", "Shell integration:", "codex", "claudecode"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output: %q", want, out)
		}
	}
}

func TestDoctor_JSON(t *testing.T) {
	deps := newTestDeps(nil, nil, nil, nil, nil)
	out, _, err := runCmd(t, deps, "--json", "doctor")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	var rep model.DoctorReport
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if !rep.AVMHome.OK || rep.AVMHome.Detail != "/tmp/.avm" {
		t.Fatalf("unexpected AVMHome: %+v", rep.AVMHome)
	}
	if len(rep.Runtimes) < 2 {
		t.Fatalf("expected runtimes in JSON, got %+v", rep.Runtimes)
	}
}

func TestStatus_Empty(t *testing.T) {
	deps := newTestDeps(nil, nil, nil, nil, nil)
	out, _, err := runCmd(t, deps, "status")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, "Agents:") || !strings.Contains(out, "Runtimes:") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestInit_DryAndExisting(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("AVM_HOME", filepath.Join(tmp, "home"))

	deps := newTestDeps(nil, nil, nil, nil, nil)
	out, _, err := runCmd(t, deps, "init")
	if err != nil {
		t.Fatalf("init failed: %v", err)
	}
	if !strings.Contains(out, "Created:") {
		t.Fatalf("expected Created: in output, got %q", out)
	}

	// Re-running should report already initialised.
	deps2 := newTestDeps(nil, nil, nil, nil, nil)
	out2, _, err := runCmd(t, deps2, "init")
	if err != nil {
		t.Fatalf("init re-run failed: %v", err)
	}
	if !strings.Contains(out2, "already initialized at") {
		t.Fatalf("expected already initialised, got: %q", out2)
	}
}

func TestUninstall_DryRun(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("AVM_HOME", filepath.Join(tmp, "home"))
	if err := os.MkdirAll(filepath.Join(tmp, "home"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	deps := newTestDeps(nil, nil, nil, nil, nil)
	out, _, err := runCmd(t, deps, "uninstall")
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if !strings.Contains(out, "Would remove:") {
		t.Fatalf("unexpected output: %q", out)
	}
	if _, err := os.Stat(filepath.Join(tmp, "home")); err != nil {
		t.Fatalf("home should still exist after dry-run, got %v", err)
	}
}
