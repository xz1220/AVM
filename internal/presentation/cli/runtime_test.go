package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/xz1220/agent-vm/internal/app/model"
)

func TestRuntimeList_Empty(t *testing.T) {
	diag := &fakeDiagnostics{runtimes: []model.RuntimeCheck{}}
	deps := newTestDeps(nil, nil, nil, nil, diag)
	out, _, err := runCmd(t, deps, "runtime", "list")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, "(no runtimes registered)") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestRuntimeList_Human(t *testing.T) {
	diag := &fakeDiagnostics{runtimes: []model.RuntimeCheck{
		{Runtime: "codex", Available: true, Binary: "/usr/bin/codex", Version: "1.2.3"},
		{Runtime: "claudecode", Available: false, Issues: []string{"binary not found"}},
	}}
	deps := newTestDeps(nil, nil, nil, nil, diag)
	out, _, err := runCmd(t, deps, "runtime", "list")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	for _, want := range []string{"codex", "yes", "/usr/bin/codex", "1.2.3", "claudecode", "no", "binary not found"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output: %s", want, out)
		}
	}
}

func TestRuntimeList_JSON(t *testing.T) {
	diag := &fakeDiagnostics{runtimes: []model.RuntimeCheck{
		{Runtime: "codex", Available: true, Binary: "/usr/bin/codex", Version: "1.0"},
	}}
	deps := newTestDeps(nil, nil, nil, nil, diag)
	out, _, err := runCmd(t, deps, "--json", "runtime", "list")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	var got []model.RuntimeCheck
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(got) != 1 || got[0].Runtime != "codex" || !got[0].Available {
		t.Fatalf("unexpected JSON: %+v", got)
	}
}
