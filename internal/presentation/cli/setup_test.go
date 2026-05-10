package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/xz1220/agent-vm/internal/app/model"
)

func TestSetup_InitializesAndBootstrapsAvailableRuntimes(t *testing.T) {
	t.Setenv("AVM_HOME", t.TempDir())
	caps := &fakeCaps{
		bootRes: &model.BootstrapCapabilitiesResult{
			Imported: []model.ImportCapabilityResult{{ID: "cap_one", Created: true}},
		},
	}
	diag := &fakeDiagnostics{runtimes: []model.RuntimeCheck{
		{Runtime: "codex", Available: true, Binary: "/usr/bin/codex", Version: "1.0"},
		{Runtime: "claude-code", Available: false, Issues: []string{"binary not found"}},
	}}

	out, _, err := runCmd(t, newTestDeps(nil, nil, nil, caps, diag), "setup")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, "Initialized AVM home") ||
		!strings.Contains(out, "codex: available; 1.0; imported 1, skipped 0") ||
		!strings.Contains(out, "claude-code: not found") ||
		!strings.Contains(out, "avm-ui") {
		t.Fatalf("unexpected output:\n%s", out)
	}
	if len(caps.bootReqs) != 1 {
		t.Fatalf("expected one bootstrap call, got %+v", caps.bootReqs)
	}
	if caps.bootReqs[0].Runtime != "codex" || caps.bootReqs[0].OnConflict != model.ResolveSkip {
		t.Fatalf("unexpected bootstrap request: %+v", caps.bootReqs[0])
	}
}

func TestSetup_CanSkipCapabilityBootstrap(t *testing.T) {
	t.Setenv("AVM_HOME", t.TempDir())
	caps := &fakeCaps{}
	diag := &fakeDiagnostics{runtimes: []model.RuntimeCheck{
		{Runtime: "codex", Available: true},
	}}

	_, _, err := runCmd(t, newTestDeps(nil, nil, nil, caps, diag), "setup", "--no-capabilities")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(caps.bootReqs) != 0 {
		t.Fatalf("expected no bootstrap calls, got %+v", caps.bootReqs)
	}
}

func TestSetup_JSON(t *testing.T) {
	t.Setenv("AVM_HOME", t.TempDir())
	diag := &fakeDiagnostics{runtimes: []model.RuntimeCheck{
		{Runtime: "codex", Available: true},
	}}

	out, _, err := runCmd(t, newTestDeps(nil, nil, nil, nil, diag), "--json", "setup")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	var got model.SetupResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if got.Init.Root == "" || len(got.Runtimes) != 1 || got.Runtimes[0].Runtime != "codex" {
		t.Fatalf("unexpected setup result: %+v", got)
	}
}

func TestRootVersion(t *testing.T) {
	deps := newTestDeps(nil, nil, nil, nil, nil)
	deps.Build = BuildInfo{Version: "1.2.3", Commit: "abc123", Date: "2026-05-10T00:00:00Z"}

	out, _, err := runCmd(t, deps, "--version")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	want := "avm 1.2.3 (abc123, 2026-05-10T00:00:00Z)"
	if strings.TrimSpace(out) != want {
		t.Fatalf("version output = %q, want %q", strings.TrimSpace(out), want)
	}
}
