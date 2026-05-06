package main

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/xz1220/agent-vm/internal/config"
)

func TestEnvListShowAndResolved(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	chdir(t, project)

	writeEnvTestAgent(t, project, config.ScopeGlobal, envTestAgent("global-coder", "codex", "global"))
	writeEnvTestAgent(t, project, config.ScopeGlobal, envTestAgent("global-reviewer", "claude-code", "review"))
	writeEnvTestAgent(t, project, config.ScopeProject, envTestAgent("project-coder", "codex", "project"))
	if err := config.WriteEnvironment(&config.Environment{
		Name: "work",
		RuntimeAgents: map[string]config.RuntimeAgent{
			"codex":       {Primary: "global-coder"},
			"claude-code": {Primary: "global-reviewer"},
		},
		Targets: []string{"codex", "claude-code"},
	}); err != nil {
		t.Fatalf("write env: %v", err)
	}
	if err := config.WriteProjectOverride(project, &config.ProjectOverride{
		Extends: "work",
		RuntimeAgents: map[string]config.RuntimeAgent{
			"codex": {Primary: "project-coder"},
		},
	}); err != nil {
		t.Fatalf("write project override: %v", err)
	}

	listOut, err := executeCommand("env", "list")
	if err != nil {
		t.Fatalf("env list returned error: %v", err)
	}
	if !strings.Contains(listOut, "NAME\tVERSION\tDESCRIPTION") || !strings.Contains(listOut, "work\t1.0.0") {
		t.Fatalf("unexpected env list output:\n%s", listOut)
	}

	showOut, err := executeCommand("env", "show", "work")
	if err != nil {
		t.Fatalf("env show returned error: %v", err)
	}
	for _, want := range []string{"name: work", "codex:", "primary: global-coder"} {
		if !strings.Contains(showOut, want) {
			t.Fatalf("env show output missing %q:\n%s", want, showOut)
		}
	}

	localOut, err := executeCommand("env", "show", "work", "--local")
	if err != nil {
		t.Fatalf("env show --local returned error: %v", err)
	}
	for _, want := range []string{"extends: work", "primary: project-coder"} {
		if !strings.Contains(localOut, want) {
			t.Fatalf("env show --local output missing %q:\n%s", want, localOut)
		}
	}

	resolvedOut, err := executeCommand("env", "show", "work", "--resolved")
	if err != nil {
		t.Fatalf("env show --resolved returned error: %v", err)
	}
	for _, want := range []string{"source_files:", config.ProjectEnvPath(project), "primary: project-coder", "global-reviewer"} {
		if !strings.Contains(resolvedOut, want) {
			t.Fatalf("env show --resolved output missing %q:\n%s", want, resolvedOut)
		}
	}
}

func TestEnvCreateRejectsExisting(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	chdir(t, project)

	if _, err := executeCommand("env", "create", "work"); err != nil {
		t.Fatalf("first env create returned error: %v", err)
	}
	out, err := executeCommand("env", "create", "work")
	if err == nil {
		t.Fatalf("expected duplicate env create error, output:\n%s", out)
	}
	if !strings.Contains(err.Error(), `env "work" already exists`) {
		t.Fatalf("unexpected duplicate env create error: %v", err)
	}

	if _, err := executeCommand("env", "create", "work", "--local", "--codex", "default"); err != nil {
		t.Fatalf("first local env create returned error: %v", err)
	}
	out, err = executeCommand("env", "create", "work", "--local", "--codex", "default")
	if err == nil {
		t.Fatalf("expected duplicate local env create error, output:\n%s", out)
	}
	if !strings.Contains(err.Error(), "local env override already exists") {
		t.Fatalf("unexpected duplicate local env create error: %v", err)
	}
}

func TestEnvCreateInteractive(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	chdir(t, project)

	cmd := newRootCommand()
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader("review\nReview env\ncodex,claude-code\ndefault\ndefault\n\n"))
	cmd.SetArgs([]string{"env", "create"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("interactive env create returned error: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "Preview:") || !strings.Contains(out.String(), "created env review") {
		t.Fatalf("unexpected interactive create output:\n%s", out.String())
	}

	env, err := config.ReadEnvironment("review")
	if err != nil {
		t.Fatalf("read created env: %v", err)
	}
	if env.Description != "Review env" ||
		!reflect.DeepEqual(env.Targets, []string{"codex", "claude-code"}) ||
		env.RuntimeAgents["codex"].Primary != "default" ||
		env.RuntimeAgents["claude-code"].Primary != "default" {
		t.Fatalf("unexpected interactive env: %#v", env)
	}
}

func TestEnvEditTargetsRequireRuntimeAgents(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	chdir(t, project)

	if err := config.WriteEnvironment(&config.Environment{
		Name: "work",
		RuntimeAgents: map[string]config.RuntimeAgent{
			"codex": {Primary: "default"},
		},
		Targets: []string{"codex"},
	}); err != nil {
		t.Fatalf("write env: %v", err)
	}

	out, err := executeCommand("env", "edit", "work", "--targets", "codex,claude-code")
	if err == nil {
		t.Fatalf("expected missing runtime agent error, output:\n%s", out)
	}
	if !strings.Contains(err.Error(), "targets.claude-code") {
		t.Fatalf("unexpected missing target error: %v", err)
	}

	out, err = executeCommand("env", "edit", "work", "--targets", "codex,claude-code", "--claude-code", "default")
	if err != nil {
		t.Fatalf("env edit with target agent returned error: %v\n%s", err, out)
	}
	env, err := config.ReadEnvironment("work")
	if err != nil {
		t.Fatalf("read edited env: %v", err)
	}
	if env.RuntimeAgents["claude-code"].Primary != "default" || !containsEnvTestString(env.Targets, "claude-code") {
		t.Fatalf("claude-code mapping was not added: %#v", env)
	}

	out, err = executeCommand("env", "edit", "work", "--targets", "codex")
	if err != nil {
		t.Fatalf("env edit pruning target returned error: %v\n%s", err, out)
	}
	env, err = config.ReadEnvironment("work")
	if err != nil {
		t.Fatalf("read pruned env: %v", err)
	}
	if _, ok := env.RuntimeAgents["claude-code"]; ok || containsEnvTestString(env.Targets, "claude-code") {
		t.Fatalf("claude-code mapping was not pruned: %#v", env)
	}
}

func TestEnvCloneEditRenameDelete(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	chdir(t, project)

	writeEnvTestAgent(t, project, config.ScopeGlobal, envTestAgent("global-coder", "codex", "global"))
	writeEnvTestAgent(t, project, config.ScopeGlobal, envTestAgent("global-reviewer", "claude-code", "review"))
	writeEnvTestAgent(t, project, config.ScopeGlobal, envTestAgent("project-coder", "codex", "project"))
	source := &config.Environment{
		Name:        "source-env",
		Description: "source description",
		RuntimeAgents: map[string]config.RuntimeAgent{
			"codex":       {Primary: "global-coder"},
			"claude-code": {Primary: "global-reviewer"},
		},
		Targets: []string{"codex", "claude-code"},
	}
	if err := config.WriteEnvironment(source); err != nil {
		t.Fatalf("write source env: %v", err)
	}

	out, err := executeCommand("env", "clone", "source-env", "--name", "copy-env")
	if err != nil {
		t.Fatalf("env clone returned error: %v\n%s", err, out)
	}
	if !strings.Contains(out, "cloned env source-env to copy-env") {
		t.Fatalf("unexpected clone output:\n%s", out)
	}
	cloned, err := config.ReadEnvironment("copy-env")
	if err != nil {
		t.Fatalf("read cloned env: %v", err)
	}
	if cloned.Description != source.Description || !reflect.DeepEqual(cloned.RuntimeAgents, source.RuntimeAgents) {
		t.Fatalf("clone did not preserve source env: %#v", cloned)
	}

	out, err = executeCommand("env", "edit", "copy-env", "--description", "edited env", "--codex", "project-coder", "--claude-code", "none")
	if err != nil {
		t.Fatalf("env edit returned error: %v\n%s", err, out)
	}
	if !strings.Contains(out, "updated env copy-env") || !strings.Contains(out, "runtime_agents") {
		t.Fatalf("unexpected edit output:\n%s", out)
	}
	edited, err := config.ReadEnvironment("copy-env")
	if err != nil {
		t.Fatalf("read edited env: %v", err)
	}
	if edited.Description != "edited env" || edited.RuntimeAgents["codex"].Primary != "project-coder" {
		t.Fatalf("unexpected edited env: %#v", edited)
	}
	if _, ok := edited.RuntimeAgents["claude-code"]; ok || containsEnvTestString(edited.Targets, "claude-code") {
		t.Fatalf("claude-code mapping was not removed: %#v", edited)
	}

	if err := config.WriteProjectOverride(project, &config.ProjectOverride{Extends: "copy-env"}); err != nil {
		t.Fatalf("write project override: %v", err)
	}
	out, err = executeCommand("env", "rename", "copy-env", "renamed-env")
	if err == nil {
		t.Fatalf("expected rename reference error, output:\n%s", out)
	}
	if !strings.Contains(err.Error(), "use --update-refs") {
		t.Fatalf("unexpected rename reference error: %v", err)
	}
	out, err = executeCommand("env", "rename", "copy-env", "renamed-env", "--update-refs")
	if err != nil {
		t.Fatalf("env rename returned error: %v\n%s", err, out)
	}
	if !strings.Contains(out, "renamed env copy-env to renamed-env") || !strings.Contains(out, "updated 1 reference(s)") {
		t.Fatalf("unexpected rename output:\n%s", out)
	}
	override, err := config.ReadProjectOverride(project)
	if err != nil {
		t.Fatalf("read override after rename: %v", err)
	}
	if override.Extends != "renamed-env" {
		t.Fatalf("override extends = %q, want renamed-env", override.Extends)
	}

	out, err = executeCommand("env", "delete", "renamed-env")
	if err == nil {
		t.Fatalf("expected delete reference error, output:\n%s", out)
	}
	if !strings.Contains(err.Error(), "use --force") {
		t.Fatalf("unexpected delete reference error: %v", err)
	}
	out, err = executeCommand("env", "delete", "renamed-env", "--force")
	if err != nil {
		t.Fatalf("env delete returned error: %v\n%s", err, out)
	}
	if !strings.Contains(out, "deleted env renamed-env") || !strings.Contains(out, "left 1 reference(s) unchanged") {
		t.Fatalf("unexpected delete output:\n%s", out)
	}
	if _, err := config.ReadEnvironment("renamed-env"); !os.IsNotExist(err) {
		t.Fatalf("deleted env still exists, err: %v", err)
	}
	out, err = executeCommand("env", "delete", "--local")
	if err != nil {
		t.Fatalf("env delete --local returned error: %v\n%s", err, out)
	}
	if !strings.Contains(out, "deleted local env override") {
		t.Fatalf("unexpected local delete output:\n%s", out)
	}
	if _, err := config.ReadProjectOverride(project); !os.IsNotExist(err) {
		t.Fatalf("local override still exists, err: %v", err)
	}
}

func TestEnvEditLocalInteractivePreservesInheritedTargets(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	chdir(t, project)

	writeEnvTestAgent(t, project, config.ScopeGlobal, envTestAgent("global-coder", "codex", "global"))
	writeEnvTestAgent(t, project, config.ScopeGlobal, envTestAgent("global-reviewer", "claude-code", "review"))
	writeEnvTestAgent(t, project, config.ScopeProject, envTestAgent("project-coder", "codex", "project"))
	if err := config.WriteEnvironment(&config.Environment{
		Name: "work",
		RuntimeAgents: map[string]config.RuntimeAgent{
			"codex":       {Primary: "global-coder"},
			"claude-code": {Primary: "global-reviewer"},
		},
		Targets: []string{"codex", "claude-code"},
	}); err != nil {
		t.Fatalf("write env: %v", err)
	}
	if err := config.WriteProjectOverride(project, &config.ProjectOverride{
		Extends: "work",
		RuntimeAgents: map[string]config.RuntimeAgent{
			"codex": {Primary: "project-coder"},
		},
	}); err != nil {
		t.Fatalf("write project override: %v", err)
	}

	cmd := newRootCommand()
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader("\n\n\n\n\n\n\n"))
	cmd.SetArgs([]string{"env", "edit", "work", "--local"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("interactive local env edit returned error: %v\n%s", err, out.String())
	}
	override, err := config.ReadProjectOverride(project)
	if err != nil {
		t.Fatalf("read edited override: %v", err)
	}
	if override.Targets != nil {
		t.Fatalf("local override targets = %#v, want inherited nil targets", override.Targets)
	}
	if override.RuntimeAgents["codex"].Primary != "project-coder" {
		t.Fatalf("local override codex changed unexpectedly: %#v", override.RuntimeAgents)
	}
}

func TestEnvEditInteractiveRuntime(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	chdir(t, project)

	writeEnvTestAgent(t, project, config.ScopeGlobal, envTestAgent("global-coder", "codex", "global"))
	writeEnvTestAgent(t, project, config.ScopeGlobal, envTestAgent("project-coder", "codex", "project"))
	if err := config.WriteEnvironment(&config.Environment{
		Name: "interactive-env",
		RuntimeAgents: map[string]config.RuntimeAgent{
			"codex": {Primary: "global-coder"},
		},
		Targets: []string{"codex"},
	}); err != nil {
		t.Fatalf("write env: %v", err)
	}

	cmd := newRootCommand()
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader("Interactive env\n\nproject-coder\n\n\n\n\n\n"))
	cmd.SetArgs([]string{"env", "edit", "interactive-env"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("interactive env edit returned error: %v\n%s", err, out.String())
	}
	for _, want := range []string{"Changes:", "updated env interactive-env"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("interactive output missing %q:\n%s", want, out.String())
		}
	}
	env, err := config.ReadEnvironment("interactive-env")
	if err != nil {
		t.Fatalf("read edited env: %v", err)
	}
	if env.Description != "Interactive env" || env.RuntimeAgents["codex"].Primary != "project-coder" {
		t.Fatalf("unexpected interactive edit result: %#v", env)
	}
}

func TestEnvDeleteRefusesActiveEnv(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	chdir(t, project)

	if err := config.WriteEnvironment(&config.Environment{
		Name: "work",
		RuntimeAgents: map[string]config.RuntimeAgent{
			"codex": {Primary: "default"},
		},
		Targets: []string{"codex"},
	}); err != nil {
		t.Fatalf("write env: %v", err)
	}
	if err := config.WriteGlobalConfig(&config.GlobalConfig{
		Active: config.ActiveRef{Kind: config.ActiveKindEnv, Name: "work"},
		Defaults: config.DefaultsConfig{
			SourceScope:      string(config.ScopeGlobal),
			Targets:          []string{"codex"},
			ConflictStrategy: "prompt",
		},
	}); err != nil {
		t.Fatalf("write global config: %v", err)
	}

	out, err := executeCommand("env", "delete", "work", "--force")
	if err == nil {
		t.Fatalf("expected active delete error, output:\n%s", out)
	}
	if !strings.Contains(err.Error(), `env "work" is active`) {
		t.Fatalf("unexpected active delete error: %v", err)
	}
}
