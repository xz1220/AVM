package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/xz1220/agent-vm/internal/app/model"
)

func TestRun_PreviewOnly(t *testing.T) {
	runner := &fakeRunner{
		preview: &model.RunPreview{Agent: "alpha", Runtime: "codex", WritePaths: []string{"/tmp/x"}},
	}
	deps := newTestDeps(nil, nil, runner, nil, nil)
	out, _, err := runCmd(t, deps, "run", "alpha", "--preview", "--runtime", "codex", "--non-interactive")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, "Agent:") || !strings.Contains(out, "alpha") || !strings.Contains(out, "/tmp/x") {
		t.Fatalf("unexpected preview output: %q", out)
	}
}

func TestRun_Run(t *testing.T) {
	runner := &fakeRunner{result: &model.RunResult{
		Preview:  model.RunPreview{Agent: "alpha", Runtime: "codex"},
		ExitCode: 0,
	}}
	deps := newTestDeps(nil, nil, runner, nil, nil)
	out, _, err := runCmd(t, deps, "run", "alpha", "--runtime", "codex", "--non-interactive")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, "Exit:    0") {
		t.Fatalf("expected exit line in output, got: %q", out)
	}
}

func TestRun_ExitCodePropagates(t *testing.T) {
	runner := &fakeRunner{result: &model.RunResult{
		Preview:  model.RunPreview{Agent: "alpha", Runtime: "codex"},
		ExitCode: 2,
	}}
	deps := newTestDeps(nil, nil, runner, nil, nil)
	_, _, err := runCmd(t, deps, "run", "alpha", "--runtime", "codex", "--non-interactive")
	if err == nil {
		t.Fatalf("expected non-zero exit error, got none")
	}
	var ec *exitCodeError
	if !errors.As(err, &ec) {
		t.Fatalf("expected exitCodeError, got %T: %v", err, err)
	}
	if ec.ExitCode() != 2 {
		t.Fatalf("expected exit code 2, got %d", ec.ExitCode())
	}
}

func TestRun_MultiRuntime_NonInteractiveFails(t *testing.T) {
	runner := &fakeRunner{
		previewErr: errors.New("runner: agent \"alpha\" has multiple runtimes; pass --runtime"),
	}
	deps := newTestDeps(nil, nil, runner, nil, nil)
	_, _, err := runCmd(t, deps, "run", "alpha", "--non-interactive")
	if err == nil {
		t.Fatalf("expected error, got none")
	}
	if !strings.Contains(err.Error(), "multiple runtimes") {
		t.Fatalf("unexpected error: %v", err)
	}
}
