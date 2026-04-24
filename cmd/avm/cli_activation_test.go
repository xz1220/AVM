package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/xz1220/agent-vm/internal/config"
	"github.com/xz1220/agent-vm/internal/state"
)

func TestUseStatusDeactivateCommands(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	chdir(t, project)

	if _, err := executeCommand("init"); err != nil {
		t.Fatalf("init returned error: %v", err)
	}
	if _, err := executeCommand("agent", "create", "backend-coder", "--runtime", "codex"); err != nil {
		t.Fatalf("agent create returned error: %v", err)
	}

	useOut, err := executeCommand("use", "backend-coder")
	if err != nil {
		t.Fatalf("use returned error: %v", err)
	}
	wantUse := "active: profile:backend-coder\n" +
		"sync: unavailable\n" +
		"targets:\n" +
		"  claude-code: skipped\n" +
		"  codex: skipped\n" +
		"  cline: skipped\n" +
		"warnings:\n" +
		"  - sync API unavailable; active config updated without runtime render\n"
	if useOut != wantUse {
		t.Fatalf("unexpected use output:\n got: %q\nwant: %q", useOut, wantUse)
	}

	cfg, err := config.ReadGlobalConfig()
	if err != nil {
		t.Fatalf("read global config: %v", err)
	}
	if cfg.Active != (config.ActiveRef{Kind: config.ActiveKindProfile, Name: "backend-coder"}) {
		t.Fatalf("unexpected active ref: %#v", cfg.Active)
	}
	assertCurrentActive(t, "profile:backend-coder")

	statusOut, err := executeCommand("status")
	if err != nil {
		t.Fatalf("status returned error: %v", err)
	}
	wantStatus := "active: profile:backend-coder\n" +
		"runtime status:\n" +
		"  claude-code: unknown\n" +
		"  codex: unknown\n" +
		"  cline: unknown\n" +
		"managed paths:\n" +
		"  claude-code: none\n" +
		"  codex: none\n" +
		"  cline: none\n" +
		"mapping status:\n" +
		"  claude-code: none\n" +
		"  codex: none\n" +
		"  cline: none\n" +
		"warnings:\n" +
		"  - sync-state not found\n"
	if statusOut != wantStatus {
		t.Fatalf("unexpected status output:\n got: %q\nwant: %q", statusOut, wantStatus)
	}

	deactivateOut, err := executeCommand("deactivate")
	if err != nil {
		t.Fatalf("deactivate returned error: %v", err)
	}
	wantDeactivate := "active: profile:default\n" +
		"sync: unavailable\n" +
		"targets:\n" +
		"  claude-code: skipped\n" +
		"  codex: skipped\n" +
		"  cline: skipped\n" +
		"warnings:\n" +
		"  - sync API unavailable; active config updated without runtime render\n"
	if deactivateOut != wantDeactivate {
		t.Fatalf("unexpected deactivate output:\n got: %q\nwant: %q", deactivateOut, wantDeactivate)
	}
	assertCurrentActive(t, "profile:default")
}

func TestStatusShowsSyncStateDetails(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	chdir(t, project)

	if _, err := executeCommand("init"); err != nil {
		t.Fatalf("init returned error: %v", err)
	}
	if _, err := executeCommand("agent", "create", "backend-coder", "--runtime", "codex"); err != nil {
		t.Fatalf("agent create returned error: %v", err)
	}
	if _, err := executeCommand("use", "backend-coder"); err != nil {
		t.Fatalf("use returned error: %v", err)
	}

	active := config.ActiveRef{Kind: config.ActiveKindProfile, Name: "backend-coder"}
	syncState := state.NewSyncState(active)
	syncState.Runtimes["codex"] = state.RuntimeState{
		Runtime:   "codex",
		Status:    state.RuntimeStatusSynced,
		Active:    active,
		AgentName: "backend-coder",
		ManagedPaths: []state.ManagedPathState{
			{Path: "/runtime/config.toml", Owner: "avm", MergeMode: "whole-file"},
		},
		Mappings: []state.MappingState{
			{SourcePath: "model_run.model", TargetPath: "profiles.avm.model", Status: "native"},
		},
		Warnings: []string{"unsupported field capabilities.hooks"},
	}
	raw, err := json.Marshal(syncState)
	if err != nil {
		t.Fatalf("marshal sync state: %v", err)
	}
	if err := os.WriteFile(syncStatePath(), raw, 0o600); err != nil {
		t.Fatalf("write sync state: %v", err)
	}

	statusOut, err := executeCommand("status")
	if err != nil {
		t.Fatalf("status returned error: %v", err)
	}
	wantStatus := "active: profile:backend-coder\n" +
		"runtime status:\n" +
		"  codex: synced (agent backend-coder)\n" +
		"managed paths:\n" +
		"  codex:\n" +
		"    - /runtime/config.toml owner=avm merge=whole-file\n" +
		"mapping status:\n" +
		"  codex:\n" +
		"    - model_run.model -> profiles.avm.model: native\n" +
		"warnings:\n" +
		"  - codex: unsupported field capabilities.hooks\n"
	if statusOut != wantStatus {
		t.Fatalf("unexpected status output:\n got: %q\nwant: %q", statusOut, wantStatus)
	}
}

func TestUseKindEnvAndAutoPrefersProfile(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	chdir(t, project)

	if _, err := executeCommand("init"); err != nil {
		t.Fatalf("init returned error: %v", err)
	}
	if _, err := executeCommand("agent", "create", "coding", "--runtime", "codex"); err != nil {
		t.Fatalf("create coding agent: %v", err)
	}
	if _, err := executeCommand("agent", "create", "backend-coder", "--runtime", "codex"); err != nil {
		t.Fatalf("create backend-coder agent: %v", err)
	}
	if _, err := executeCommand("env", "create", "coding", "--codex", "backend-coder"); err != nil {
		t.Fatalf("create coding env: %v", err)
	}

	autoOut, err := executeCommand("use", "coding")
	if err != nil {
		t.Fatalf("auto use returned error: %v", err)
	}
	if !strings.HasPrefix(autoOut, "active: profile:coding\n") {
		t.Fatalf("auto use did not prefer profile:\n%s", autoOut)
	}

	envOut, err := executeCommand("use", "--kind", "env", "coding")
	if err != nil {
		t.Fatalf("env use returned error: %v", err)
	}
	wantEnv := "active: env:coding\n" +
		"sync: unavailable\n" +
		"targets:\n" +
		"  codex: skipped\n" +
		"warnings:\n" +
		"  - sync API unavailable; active config updated without runtime render\n"
	if envOut != wantEnv {
		t.Fatalf("unexpected env use output:\n got: %q\nwant: %q", envOut, wantEnv)
	}
}

func TestUseMissingActivationStableErrors(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	chdir(t, project)

	if _, err := executeCommand("init"); err != nil {
		t.Fatalf("init returned error: %v", err)
	}

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "auto", args: []string{"use", "missing"}, want: "activation \"missing\" not found as profile or env"},
		{name: "profile", args: []string{"use", "--kind", "profile", "missing"}, want: "profile \"missing\" not found"},
		{name: "env", args: []string{"use", "--kind", "env", "missing"}, want: "env \"missing\" not found"},
		{name: "invalid kind", args: []string{"use", "--kind", "team", "missing"}, want: "invalid activation kind \"team\" (want profile or env)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := executeCommand(tt.args...)
			if err == nil {
				t.Fatal("expected error")
			}
			if got := err.Error(); got != tt.want {
				t.Fatalf("unexpected error:\n got: %q\nwant: %q", got, tt.want)
			}
		})
	}
}

func assertCurrentActive(t *testing.T, want string) {
	t.Helper()
	data, err := os.ReadFile(currentActivePath())
	if err != nil {
		t.Fatalf("read current active: %v", err)
	}
	if got := strings.TrimSpace(string(data)); got != want {
		t.Fatalf("unexpected current active:\n got: %q\nwant: %q", got, want)
	}
}
