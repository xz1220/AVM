package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/xz1220/agent-vm/internal/config"
)

func TestInitWritesOnlyHomeAVM(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	chdir(t, project)

	out, err := executeCommand("init")
	if err != nil {
		t.Fatalf("init returned error: %v", err)
	}
	if got := strings.TrimSpace(out); got != "initialized avm home" {
		t.Fatalf("unexpected init output: %q", got)
	}

	for _, path := range []string{
		filepath.Join(home, ".avm", "config.yaml"),
		filepath.Join(home, ".avm", "agents", "default.yaml"),
		filepath.Join(home, ".avm", "envs", "default.yaml"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected init artifact %s: %v", path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(project, ".avm")); !os.IsNotExist(err) {
		t.Fatalf("init wrote project .avm directory, stat err: %v", err)
	}

	avmRoot := filepath.Join(home, ".avm")
	if err := filepath.WalkDir(home, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == home {
			return nil
		}
		if path == avmRoot || strings.HasPrefix(path, avmRoot+string(os.PathSeparator)) {
			return nil
		}
		t.Fatalf("init wrote outside ~/.avm: %s", path)
		return nil
	}); err != nil {
		t.Fatalf("walk home: %v", err)
	}
}

func TestAgentCreateListShow(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	chdir(t, project)

	out, err := executeCommand(
		"agent", "create", "backend-coder",
		"--runtime", "codex",
		"--model", "gpt-5.4",
		"--reasoning", "high",
		"--skills", "git,test",
		"--mcps", "github",
		"--memory", "backend-standards",
	)
	if err != nil {
		t.Fatalf("agent create returned error: %v", err)
	}
	if got := strings.TrimSpace(out); got != "created agent backend-coder" {
		t.Fatalf("unexpected create output: %q", got)
	}

	agent, err := config.ReadAgent("backend-coder", config.ScopeGlobal, project)
	if err != nil {
		t.Fatalf("read created agent: %v", err)
	}
	if agent.Runtime.Preferred != "codex" {
		t.Fatalf("unexpected runtime: %q", agent.Runtime.Preferred)
	}
	if agent.ModelRun.Model != "gpt-5.4" || agent.ModelRun.ReasoningEffort != "high" {
		t.Fatalf("unexpected model run: %#v", agent.ModelRun)
	}
	if !reflect.DeepEqual(agent.Capabilities.Skills, []string{"git", "test"}) {
		t.Fatalf("unexpected skills: %#v", agent.Capabilities.Skills)
	}
	if !reflect.DeepEqual(agent.Capabilities.MCPs, []string{"github"}) {
		t.Fatalf("unexpected mcps: %#v", agent.Capabilities.MCPs)
	}
	if len(agent.MemoryRefs) != 1 || agent.MemoryRefs[0].ID != "backend-standards" || agent.MemoryRefs[0].Scope != string(config.ScopeProject) {
		t.Fatalf("unexpected memory refs: %#v", agent.MemoryRefs)
	}

	listOut, err := executeCommand("agent", "list")
	if err != nil {
		t.Fatalf("agent list returned error: %v", err)
	}
	if want := "NAME\tSCOPE\tVERSION\tDESCRIPTION\nbackend-coder\tglobal\t1.0.0\t\n"; listOut != want {
		t.Fatalf("unexpected list output:\n got: %q\nwant: %q", listOut, want)
	}

	showOut, err := executeCommand("agent", "show", "backend-coder")
	if err != nil {
		t.Fatalf("agent show returned error: %v", err)
	}
	for _, want := range []string{
		"name: backend-coder",
		"preferred: codex",
		"model: gpt-5.4",
		"reasoning_effort: high",
		"- git",
		"- github",
		"id: backend-standards",
	} {
		if !strings.Contains(showOut, want) {
			t.Fatalf("show output missing %q:\n%s", want, showOut)
		}
	}
}

func TestEnvCreate(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	chdir(t, project)

	out, err := executeCommand(
		"env", "create", "coding",
		"--codex", "backend-coder",
		"--claude-code", "backend-reviewer",
	)
	if err != nil {
		t.Fatalf("env create returned error: %v", err)
	}
	if got := strings.TrimSpace(out); got != "created env coding" {
		t.Fatalf("unexpected env create output: %q", got)
	}

	env, err := config.ReadEnvironment("coding")
	if err != nil {
		t.Fatalf("read created env: %v", err)
	}
	if env.RuntimeAgents["codex"].Primary != "backend-coder" {
		t.Fatalf("unexpected codex mapping: %#v", env.RuntimeAgents["codex"])
	}
	if env.RuntimeAgents["claude-code"].Primary != "backend-reviewer" {
		t.Fatalf("unexpected claude-code mapping: %#v", env.RuntimeAgents["claude-code"])
	}
	if !reflect.DeepEqual(env.Targets, []string{"codex", "claude-code"}) {
		t.Fatalf("unexpected targets: %#v", env.Targets)
	}

	data, err := os.ReadFile(config.EnvPath("coding"))
	if err != nil {
		t.Fatalf("read env yaml: %v", err)
	}
	if bytes := string(data); strings.Contains(bytes, "capabilities") || strings.Contains(bytes, "memory_layers") {
		t.Fatalf("env yaml declared unsupported fields:\n%s", bytes)
	}
}

func chdir(t *testing.T, dir string) {
	t.Helper()
	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(original); err != nil {
			t.Fatalf("restore cwd %s: %v", original, err)
		}
	})
}
