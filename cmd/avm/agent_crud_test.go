package main

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/xz1220/agent-vm/internal/config"
)

func TestAgentCloneEditRenameDelete(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	chdir(t, project)

	source := config.AgentProfile{
		Name:        "source-agent",
		Description: "source description",
		Runtime: config.RuntimePreferences{
			Preferred: "codex",
			Fallback:  []string{"opencode"},
		},
		Capabilities: config.CapabilityRefs{
			Skills: []string{"docs"},
			MCPs:   []string{"github"},
		},
		MemoryRefs: []config.MemoryRef{{
			ID:    "standards",
			Scope: string(config.ScopeProject),
			Path:  "/memory/standards.md",
			Mode:  "read",
		}},
	}
	if err := config.WriteAgent(&source, config.ScopeGlobal, project); err != nil {
		t.Fatalf("write source agent: %v", err)
	}

	out, err := executeCommand("agent", "clone", "source-agent", "--name", "copy-agent")
	if err != nil {
		t.Fatalf("agent clone returned error: %v\n%s", err, out)
	}
	if !strings.Contains(out, "cloned agent source-agent from global to copy-agent") {
		t.Fatalf("unexpected clone output:\n%s", out)
	}
	cloned, err := config.ReadAgent("copy-agent", config.ScopeGlobal, project)
	if err != nil {
		t.Fatalf("read cloned agent: %v", err)
	}
	if cloned.Description != source.Description || !reflect.DeepEqual(cloned.Capabilities.Skills, source.Capabilities.Skills) || len(cloned.MemoryRefs) != 1 {
		t.Fatalf("clone did not preserve source fields: %#v", cloned)
	}

	out, err = executeCommand(
		"agent", "edit", "copy-agent",
		"--description", "edited description",
		"--runtimes", "claude-code,codex",
		"--model", "gpt-5.4",
		"--reasoning", "high",
		"--skills", "test,docs",
		"--mcps", "github",
		"--system", "system prompt",
		"--memory", "edited-memory:project:/memory/edited.md:append",
	)
	if err != nil {
		t.Fatalf("agent edit returned error: %v\n%s", err, out)
	}
	if !strings.Contains(out, "updated agent copy-agent") || !strings.Contains(out, "runtime.runtimes") {
		t.Fatalf("unexpected edit output:\n%s", out)
	}
	edited, err := config.ReadAgent("copy-agent", config.ScopeGlobal, project)
	if err != nil {
		t.Fatalf("read edited agent: %v", err)
	}
	if edited.Description != "edited description" ||
		edited.Runtime.Preferred != "claude-code" ||
		!reflect.DeepEqual(edited.Runtime.Fallback, []string{"codex"}) ||
		edited.ModelRun.Model != "gpt-5.4" ||
		edited.ModelRun.ReasoningEffort != "high" ||
		edited.Instructions.System != "system prompt" ||
		len(edited.MemoryRefs) != 1 ||
		edited.MemoryRefs[0].Mode != "append" {
		t.Fatalf("unexpected edited agent: %#v", edited)
	}

	env := &config.Environment{
		Name: "coding",
		RuntimeAgents: map[string]config.RuntimeAgent{
			"codex": {Primary: "copy-agent"},
		},
		Targets: []string{"codex"},
	}
	if err := config.WriteEnvironment(env); err != nil {
		t.Fatalf("write env: %v", err)
	}

	out, err = executeCommand("agent", "rename", "copy-agent", "renamed-agent")
	if err == nil {
		t.Fatalf("expected rename reference error, output:\n%s", out)
	}
	if !strings.Contains(err.Error(), "use --update-refs") {
		t.Fatalf("unexpected rename reference error: %v", err)
	}

	out, err = executeCommand("agent", "rename", "copy-agent", "renamed-agent", "--update-refs")
	if err != nil {
		t.Fatalf("agent rename returned error: %v\n%s", err, out)
	}
	if !strings.Contains(out, "renamed agent copy-agent to renamed-agent") ||
		!strings.Contains(out, "updated 1 reference(s)") {
		t.Fatalf("unexpected rename output:\n%s", out)
	}
	if _, err := config.ReadAgent("copy-agent", config.ScopeGlobal, project); !os.IsNotExist(err) {
		t.Fatalf("old renamed agent still exists, err: %v", err)
	}
	renamedEnv, err := config.ReadEnvironment("coding")
	if err != nil {
		t.Fatalf("read env after rename: %v", err)
	}
	if renamedEnv.RuntimeAgents["codex"].Primary != "renamed-agent" {
		t.Fatalf("rename did not update env reference: %#v", renamedEnv.RuntimeAgents)
	}

	out, err = executeCommand("agent", "delete", "renamed-agent")
	if err == nil {
		t.Fatalf("expected delete reference error, output:\n%s", out)
	}
	if !strings.Contains(err.Error(), "use --force") {
		t.Fatalf("unexpected delete reference error: %v", err)
	}
	out, err = executeCommand("agent", "delete", "renamed-agent", "--force")
	if err != nil {
		t.Fatalf("agent delete returned error: %v\n%s", err, out)
	}
	if !strings.Contains(out, "deleted agent renamed-agent") ||
		!strings.Contains(out, "left 1 reference(s) unchanged") {
		t.Fatalf("unexpected delete output:\n%s", out)
	}
	if _, err := config.ReadAgent("renamed-agent", config.ScopeGlobal, project); !os.IsNotExist(err) {
		t.Fatalf("deleted agent still exists, err: %v", err)
	}
}

func TestAgentEditInteractiveBasicFields(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	chdir(t, project)

	if _, err := executeCommand("agent", "create", "interactive-agent", "--runtime", "codex"); err != nil {
		t.Fatalf("agent create returned error: %v", err)
	}

	cmd := newRootCommand()
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader("basic\nInteractive description\nInteractive Agent\nreviewer\nalpha,beta\n\n"))
	cmd.SetArgs([]string{"agent", "edit", "interactive-agent"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("interactive agent edit returned error: %v\n%s", err, out.String())
	}
	for _, want := range []string{"Fields to edit:", "Changes:", "updated agent interactive-agent"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("interactive output missing %q:\n%s", want, out.String())
		}
	}

	agent, err := config.ReadAgent("interactive-agent", config.ScopeGlobal, project)
	if err != nil {
		t.Fatalf("read edited agent: %v", err)
	}
	if agent.Description != "Interactive description" ||
		agent.Identity.DisplayName != "Interactive Agent" ||
		agent.Identity.Role != "reviewer" ||
		!reflect.DeepEqual(agent.Identity.Tags, []string{"alpha", "beta"}) {
		t.Fatalf("unexpected interactive edit result: %#v", agent)
	}
}

func TestAgentDeleteRefusesActiveAgent(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	chdir(t, project)

	out, err := executeCommand("agent", "delete", "default", "--force")
	if err == nil {
		t.Fatalf("expected active delete error, output:\n%s", out)
	}
	if !strings.Contains(err.Error(), `agent "default" is active`) {
		t.Fatalf("unexpected active delete error: %v", err)
	}
}
