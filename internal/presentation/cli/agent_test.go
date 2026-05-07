package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/xz1220/agent-vm/internal/app/model"
	"github.com/xz1220/agent-vm/internal/app/service"
)

func newTestDeps(agents *fakeAgents, pkgs *fakePackages, runner *fakeRunner, caps *fakeCaps, diag *fakeDiagnostics) Deps {
	if agents == nil {
		agents = newFakeAgents()
	}
	if pkgs == nil {
		pkgs = &fakePackages{}
	}
	if runner == nil {
		runner = &fakeRunner{}
	}
	if caps == nil {
		caps = &fakeCaps{}
	}
	if diag == nil {
		diag = &fakeDiagnostics{}
	}
	return Deps{Services: service.Container{
		Agents:       agents,
		Run:          runner,
		Packages:     pkgs,
		Capabilities: caps,
		Diagnostics:  diag,
	}}
}

func runCmd(t *testing.T, deps Deps, args ...string) (string, string, error) {
	t.Helper()
	root := NewRoot(deps)
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), errOut.String(), err
}

func TestAgentList_Empty(t *testing.T) {
	deps := newTestDeps(nil, nil, nil, nil, nil)
	out, _, err := runCmd(t, deps, "agent", "list")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, "(no agents)") {
		t.Fatalf("expected '(no agents)' in output, got: %q", out)
	}
}

func TestAgentList_NonEmpty(t *testing.T) {
	agents := newFakeAgents()
	agents.put(model.Agent{
		Identity: model.Identity{Name: "alpha", Description: "primary"},
		Runtimes: []model.RuntimePref{{Runtime: "codex"}},
	})
	deps := newTestDeps(agents, nil, nil, nil, nil)
	out, _, err := runCmd(t, deps, "agent", "list")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, "alpha") || !strings.Contains(out, "primary") || !strings.Contains(out, "codex") {
		t.Fatalf("unexpected output: %q", out)
	}
	if !strings.Contains(out, "NAME") {
		t.Fatalf("expected header NAME in output, got: %q", out)
	}
}

func TestAgentList_JSON(t *testing.T) {
	agents := newFakeAgents()
	agents.put(model.Agent{
		Identity: model.Identity{Name: "alpha", Description: "primary"},
		Runtimes: []model.RuntimePref{{Runtime: "codex"}},
	})
	deps := newTestDeps(agents, nil, nil, nil, nil)
	out, _, err := runCmd(t, deps, "--json", "agent", "list")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	var got []model.AgentSummary
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(got) != 1 || got[0].Name != "alpha" || got[0].Runtimes[0] != "codex" {
		t.Fatalf("unexpected JSON: %+v", got)
	}
}

func TestAgentShow(t *testing.T) {
	agents := newFakeAgents()
	agents.put(model.Agent{
		Identity:     model.Identity{Name: "alpha", Description: "desc"},
		Instructions: model.Instructions{System: "be helpful"},
		Skills:       []model.CapabilityRef{{ID: "skill-1", Kind: model.CapabilityKindSkill}},
		Runtimes:     []model.RuntimePref{{Runtime: "codex", Default: true}},
	})
	deps := newTestDeps(agents, nil, nil, nil, nil)
	out, _, err := runCmd(t, deps, "agent", "show", "alpha")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	for _, want := range []string{"alpha", "desc", "be helpful", "skill-1", "codex", "(default)"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in output, got: %q", want, out)
		}
	}
}

func TestAgentCreate_NonInteractive_RequiresName(t *testing.T) {
	deps := newTestDeps(nil, nil, nil, nil, nil)
	_, _, err := runCmd(t, deps, "--non-interactive", "agent", "create")
	if err == nil {
		t.Fatalf("expected error for missing --name, got none")
	}
	if !strings.Contains(err.Error(), "non-interactive create requires") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAgentCreate_NonInteractive_OK(t *testing.T) {
	agents := newFakeAgents()
	deps := newTestDeps(agents, nil, nil, nil, nil)
	out, _, err := runCmd(t, deps,
		"--non-interactive", "agent", "create",
		"--name", "alpha",
		"--description", "test agent",
		"--runtime", "codex",
	)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, `Created agent "alpha"`) {
		t.Fatalf("expected created message, got: %q", out)
	}
	if len(agents.createReqs) != 1 {
		t.Fatalf("expected 1 create call, got %d", len(agents.createReqs))
	}
	got := agents.createReqs[0]
	if got.Name != "alpha" || len(got.Runtimes) != 1 || got.Runtimes[0].Runtime != "codex" {
		t.Fatalf("unexpected create request: %+v", got)
	}
}

func TestAgentDelete_NonInteractive_RequiresYes(t *testing.T) {
	agents := newFakeAgents()
	agents.put(model.Agent{Identity: model.Identity{Name: "alpha"}})
	deps := newTestDeps(agents, nil, nil, nil, nil)
	_, _, err := runCmd(t, deps, "--non-interactive", "agent", "delete", "alpha")
	if err == nil {
		t.Fatalf("expected error without --yes")
	}
}

func TestAgentDelete_NonInteractive_OK(t *testing.T) {
	agents := newFakeAgents()
	agents.put(model.Agent{Identity: model.Identity{Name: "alpha"}})
	deps := newTestDeps(agents, nil, nil, nil, nil)
	out, _, err := runCmd(t, deps, "--non-interactive", "agent", "delete", "alpha", "--yes")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, `Deleted agent "alpha"`) {
		t.Fatalf("unexpected output: %q", out)
	}
	if len(agents.deleted) != 1 || agents.deleted[0] != "alpha" {
		t.Fatalf("expected agent deleted, got %v", agents.deleted)
	}
}

func TestAgentClone(t *testing.T) {
	agents := newFakeAgents()
	agents.put(model.Agent{Identity: model.Identity{Name: "alpha"}})
	deps := newTestDeps(agents, nil, nil, nil, nil)
	out, _, err := runCmd(t, deps, "agent", "clone", "alpha", "--name", "beta")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, `Cloned "alpha" -> "beta"`) {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestAgentRename(t *testing.T) {
	agents := newFakeAgents()
	agents.put(model.Agent{Identity: model.Identity{Name: "alpha"}})
	deps := newTestDeps(agents, nil, nil, nil, nil)
	out, _, err := runCmd(t, deps, "agent", "rename", "alpha", "beta")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, `Renamed "alpha" -> "beta"`) {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestAgentEdit_NonInteractive(t *testing.T) {
	agents := newFakeAgents()
	agents.put(model.Agent{Identity: model.Identity{Name: "alpha"}})
	deps := newTestDeps(agents, nil, nil, nil, nil)
	out, _, err := runCmd(t, deps,
		"--non-interactive", "agent", "edit", "alpha",
		"--description", "updated",
	)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, `Edited agent "alpha"`) {
		t.Fatalf("unexpected output: %q", out)
	}
	if len(agents.editCalls) != 1 || agents.editCalls[0].Identity == nil ||
		agents.editCalls[0].Identity.Description != "updated" {
		t.Fatalf("expected edit with description, got %+v", agents.editCalls)
	}
}
