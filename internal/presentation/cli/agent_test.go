package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/xz1220/agent-vm/internal/app/model"
	"github.com/xz1220/agent-vm/internal/app/service"
	"github.com/xz1220/agent-vm/internal/infra/home"
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
	// Build a real SystemService so init/uninstall/shell tests exercise
	// the actual service path. The Layout honors $AVM_HOME via
	// home.DefaultLayout, which tests typically t.Setenv before calling
	// newTestDeps. Tests that don't touch System (most agent/run/pkg
	// cases) never hit this surface and don't care about the layout.
	layout, _ := home.DefaultLayout()
	return Deps{Services: service.Container{
		Agents:       agents,
		Run:          runner,
		Packages:     pkgs,
		Capabilities: caps,
		Diagnostics:  diag,
		System:       service.NewSystem(layout),
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

func TestAgentCreate_RequiresName(t *testing.T) {
	deps := newTestDeps(nil, nil, nil, nil, nil)
	_, _, err := runCmd(t, deps, "agent", "create")
	if err == nil {
		t.Fatalf("expected error for missing --name, got none")
	}
	se := service.AsError(err)
	if se == nil {
		t.Fatalf("expected typed *service.Error, got %T %v", err, err)
	}
	// Empty Name fails name regex validation in service.
	if se.Code != service.CodeValidation && se.Code != service.CodeAgentInvalidName {
		t.Fatalf("unexpected error code %s (%s)", se.Code, se.Message)
	}
}

func TestAgentCreate_RequiresRuntime_Envelope(t *testing.T) {
	deps := newTestDeps(nil, nil, nil, nil, nil)
	out, _, err := runCmd(t, deps, "--json", "agent", "create", "--name", "alpha")
	if err == nil {
		t.Fatal("expected MISSING_INPUT error without --runtime")
	}
	var env struct {
		Error *service.Error `json:"error"`
	}
	if jerr := json.Unmarshal([]byte(out), &env); jerr != nil {
		t.Fatalf("invalid envelope: %v\n%s", jerr, out)
	}
	if env.Error == nil || env.Error.Code != service.CodeMissingInput {
		t.Fatalf("expected MISSING_INPUT envelope, got %+v", env.Error)
	}
	if got, _ := env.Error.Details["field"].(string); got != "runtime" {
		t.Fatalf("expected details.field=runtime, got %v", env.Error.Details)
	}
}

func TestAgentCreate_NonInteractive_OK(t *testing.T) {
	agents := newFakeAgents()
	deps := newTestDeps(agents, nil, nil, nil, nil)
	out, _, err := runCmd(t, deps,
		"agent", "create",
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
	_, _, err := runCmd(t, deps, "agent", "delete", "alpha")
	if err == nil {
		t.Fatalf("expected error without --yes")
	}
}

func TestAgentDelete_NonInteractive_OK(t *testing.T) {
	agents := newFakeAgents()
	agents.put(model.Agent{Identity: model.Identity{Name: "alpha"}})
	deps := newTestDeps(agents, nil, nil, nil, nil)
	out, _, err := runCmd(t, deps, "agent", "delete", "alpha", "--yes")
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

func TestAgentDelete_JSON_EmitsNull(t *testing.T) {
	agents := newFakeAgents()
	agents.put(model.Agent{Identity: model.Identity{Name: "alpha"}})
	deps := newTestDeps(agents, nil, nil, nil, nil)
	out, _, err := runCmd(t, deps, "--json", "agent", "delete", "alpha", "--yes")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if out != "null\n" {
		t.Fatalf("expected literal \"null\\n\" in JSON mode, got %q", out)
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
		"agent", "edit", "alpha",
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

// TestAgentEdit_ReplacesSkillsList covers the expanded non-interactive
// edit flag set: --skill replaces the entire skills list.
func TestAgentEdit_ReplacesSkillsList(t *testing.T) {
	agents := newFakeAgents()
	agents.put(model.Agent{
		Identity: model.Identity{Name: "alpha"},
		Skills:   []model.CapabilityRef{{ID: "cap_old", Kind: model.CapabilityKindSkill}},
	})
	deps := newTestDeps(agents, nil, nil, nil, nil)
	_, _, err := runCmd(t, deps,
		"agent", "edit", "alpha",
		"--skill", "cap_one",
		"--skill", "cap_two",
	)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(agents.editCalls) != 1 {
		t.Fatalf("expected 1 edit call, got %d", len(agents.editCalls))
	}
	got := agents.editCalls[0]
	if got.Skills == nil {
		t.Fatalf("expected Skills to be set (nil pointer means keep-existing)")
	}
	if len(*got.Skills) != 2 || (*got.Skills)[0].ID != "cap_one" || (*got.Skills)[1].ID != "cap_two" {
		t.Fatalf("expected [cap_one cap_two], got %+v", *got.Skills)
	}
}

// TestAgentEdit_KeepsSkillsWhenFlagAbsent verifies that not passing
// --skill leaves Skills as nil so the service preserves existing value.
func TestAgentEdit_KeepsSkillsWhenFlagAbsent(t *testing.T) {
	agents := newFakeAgents()
	agents.put(model.Agent{Identity: model.Identity{Name: "alpha"}})
	deps := newTestDeps(agents, nil, nil, nil, nil)
	_, _, err := runCmd(t, deps,
		"agent", "edit", "alpha",
		"--description", "x",
	)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	got := agents.editCalls[0]
	if got.Skills != nil {
		t.Fatalf("expected Skills nil (keep-existing), got %+v", got.Skills)
	}
	if got.MCP != nil {
		t.Fatalf("expected MCP nil, got %+v", got.MCP)
	}
	if got.Runtimes != nil {
		t.Fatalf("expected Runtimes nil, got %+v", got.Runtimes)
	}
}

// TestJSONError_Envelope verifies that --json mode wraps any error in a
// stable {"error": {code, message, details}} envelope on stdout.
func TestJSONError_Envelope(t *testing.T) {
	deps := newTestDeps(nil, nil, nil, nil, nil)
	out, _, err := runCmd(t, deps, "--json", "agent", "show", "ghost")
	if err == nil {
		t.Fatalf("expected error for missing agent")
	}
	var env struct {
		Error *service.Error `json:"error"`
	}
	if jerr := json.Unmarshal([]byte(out), &env); jerr != nil {
		t.Fatalf("invalid JSON envelope: %v\noutput: %s", jerr, out)
	}
	if env.Error == nil {
		t.Fatalf("expected error envelope populated, got %s", out)
	}
	if env.Error.Code != service.CodeAgentNotFound {
		t.Fatalf("expected AGENT_NOT_FOUND, got %s", env.Error.Code)
	}
	name, _ := env.Error.Details["name"].(string)
	if name != "ghost" {
		t.Fatalf("expected details.name=ghost, got %v", env.Error.Details)
	}
}
