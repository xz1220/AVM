package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	avmadapter "github.com/xz1220/agent-vm/internal/adapter"
	"github.com/xz1220/agent-vm/internal/config"
)

func TestAgentShowRuntimeMappingPreviewCodex(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	setupCodexHome(t, home)
	chdir(t, project)

	if _, err := executeCommand(
		"agent", "create", "backend-coder",
		"--runtime", "codex",
		"--model", "gpt-5.4",
		"--skills", "git,test",
		"--mcps", "github",
	); err != nil {
		t.Fatalf("agent create returned error: %v", err)
	}

	showOut, err := executeCommand("agent", "show", "backend-coder")
	if err != nil {
		t.Fatalf("agent show returned error: %v", err)
	}
	for _, want := range []string{"name: backend-coder", "preferred: codex", "model: gpt-5.4"} {
		if !strings.Contains(showOut, want) {
			t.Fatalf("show output missing %q:\n%s", want, showOut)
		}
	}
	if strings.Contains(showOut, "managed_paths:") || strings.Contains(showOut, "rendered_as_instructions:") {
		t.Fatalf("show without --runtime should remain profile YAML:\n%s", showOut)
	}

	previewOut, err := executeCommand("agent", "show", "backend-coder", "--runtime", "codex")
	if err != nil {
		t.Fatalf("agent show --runtime codex returned error: %v", err)
	}
	codexHome := agentRuntimeHomeForTest(t, "backend-coder", "codex")
	rolePath := filepath.ToSlash(filepath.Join(codexHome, "agents", "backend-coder.toml"))
	configPath := filepath.ToSlash(filepath.Join(codexHome, "config.toml"))
	for _, want := range []string{
		"runtime: codex",
		"agent: backend-coder",
		"managed_paths:",
		configPath,
		rolePath,
		"warnings:",
		"mcp server \"github\" was not rendered because command or URL is missing",
		"native:",
		"source: agent.name",
		"rendered_as_instructions:",
		"source: capabilities.skills",
		"ignored:",
		"source: project.AGENTS.md",
		"unsupported:",
		"source: capabilities.mcp_servers.github",
	} {
		if !strings.Contains(previewOut, want) {
			t.Fatalf("preview output missing %q:\n%s", want, previewOut)
		}
	}

	assertPathDoesNotExist(t, filepath.Join(codexHome, "config.toml"))
	assertPathDoesNotExist(t, filepath.Join(codexHome, "agents", "backend-coder.toml"))
}

func TestAgentShowRuntimeMappingPreviewCursor(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	chdir(t, project)

	if _, err := executeCommand(
		"agent", "create", "backend-coder",
		"--runtime", "codex",
		"--model", "gpt-5.4",
		"--skills", "git",
	); err != nil {
		t.Fatalf("agent create returned error: %v", err)
	}

	previewOut, err := executeCommand("agent", "show", "backend-coder", "--runtime", "cursor")
	if err != nil {
		t.Fatalf("agent show --runtime cursor returned error: %v", err)
	}
	rulePath := filepath.ToSlash(filepath.Join(project, ".cursor", "rules", "avm-backend-coder.md"))
	for _, want := range []string{
		"runtime: cursor",
		"agent: backend-coder",
		rulePath,
		"warnings:",
		"cursor adapter is partial in Phase 1; only AVM-owned Cursor rules and MCP server entries are rendered",
		"rendered_as_instructions:",
		"source: agent.instructions.system",
		"ignored:",
		"source: active",
		"unsupported:",
		"source: agent.profile",
		"source: agent.model.model",
		"source: capabilities.skills",
	} {
		if !strings.Contains(previewOut, want) {
			t.Fatalf("preview output missing %q:\n%s", want, previewOut)
		}
	}

	assertPathDoesNotExist(t, filepath.Join(project, ".cursor"))
}

func TestAgentShowRuntimeMappingPreviewStableErrors(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	chdir(t, project)

	if _, err := executeCommand("agent", "show", "missing", "--runtime", "unknown"); err == nil || err.Error() != `invalid runtime "unknown"` {
		t.Fatalf("unexpected invalid runtime error: %v", err)
	}
	if _, err := executeCommand("agent", "show", "missing", "--runtime", "codex"); err == nil || err.Error() != `profile "missing" not found` {
		t.Fatalf("unexpected missing profile error: %v", err)
	}

	agent := &config.AgentProfile{
		Name: "backend-coder",
		Runtime: config.RuntimePreferences{
			Preferred: "codex",
		},
	}
	agent.ApplyDefaults(string(config.ScopeGlobal))

	_, err := buildAgentMappingPreview(context.Background(), agent, "codex", project, previewPlanErrorRegistry{})
	if err == nil {
		t.Fatal("expected adapter plan error")
	}
	if got, want := err.Error(), `runtime "codex" mapping preview failed: boom`; got != want {
		t.Fatalf("unexpected adapter plan error:\n got: %q\nwant: %q", got, want)
	}
}

func assertPathDoesNotExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected %s to not exist, stat err: %v", path, err)
	}
}

type previewPlanErrorRegistry struct{}

func (previewPlanErrorRegistry) Get(runtime string) (avmadapter.Adapter, bool) {
	if runtime != "codex" {
		return nil, false
	}
	return previewPlanErrorAdapter{}, true
}

type previewPlanErrorAdapter struct{}

func (previewPlanErrorAdapter) Name() string {
	return "codex"
}

func (previewPlanErrorAdapter) Detect(ctx avmadapter.Context) avmadapter.Detection {
	_ = ctx
	return avmadapter.Detection{Runtime: "codex", Found: true}
}

func (previewPlanErrorAdapter) Plan(ctx avmadapter.Context, input avmadapter.RenderInput) (*avmadapter.RenderPlan, error) {
	_, _ = ctx, input
	return nil, errors.New("boom")
}

func (previewPlanErrorAdapter) Render(ctx avmadapter.Context, plan *avmadapter.RenderPlan) (*avmadapter.RenderResult, error) {
	_, _ = ctx, plan
	return nil, nil
}

func (previewPlanErrorAdapter) ManagedPaths(ctx avmadapter.Context, plan *avmadapter.RenderPlan) []avmadapter.ManagedPath {
	_, _ = ctx, plan
	return nil
}
