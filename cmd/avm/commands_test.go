package main

import (
	"bytes"
	"strings"
	"testing"
)

func executeCommand(args ...string) (string, error) {
	cmd := newRootCommand()
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs(args)

	err := cmd.Execute()
	return out.String(), err
}

func TestRegisteredCommandHelp(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "root",
			args: []string{"--help"},
			want: []string{"activate", "agent", "create", "deactivate", "env", "export", "import", "install", "init", "memory", "package", "runtime", "skill", "shell", "status", "sync", "use"},
		},
		{
			name: "create",
			args: []string{"create", "--help"},
			want: []string{"create [package]", "--name", "--from", "--from-import", "--runtime", "--runtimes", "--yes", "--no-input"},
		},
		{
			name: "package",
			args: []string{"package", "--help"},
			want: []string{"list", "show"},
		},
		{
			name: "skill",
			args: []string{"skill", "--help"},
			want: []string{"list"},
		},
		{
			name: "runtime",
			args: []string{"runtime", "--help"},
			want: []string{"list", "scan"},
		},
		{
			name: "agent",
			args: []string{"agent", "--help"},
			want: []string{"create", "list", "show"},
		},
		{
			name: "agent create",
			args: []string{"agent", "create", "--help"},
			want: []string{"create <name>", "--runtime", "--skills", "--mcps", "--memory"},
		},
		{
			name: "env create",
			args: []string{"env", "create", "--help"},
			want: []string{"create <name>", "--codex", "--claude-code", "--opencode", "--cline", "--cursor"},
		},
		{
			name: "memory import",
			args: []string{"memory", "import", "--help"},
			want: []string{"import", "--from", "--dry-run", "--format"},
		},
		{
			name: "export",
			args: []string{"export", "--help"},
			want: []string{"export <agent-or-env>", "--output", "--kind"},
		},
		{
			name: "import",
			args: []string{"import", "--help"},
			want: []string{"import <file.avm.zip>"},
		},
		{
			name: "install",
			args: []string{"install", "--help"},
			want: []string{"install <file.avm.zip>"},
		},
		{
			name: "activate",
			args: []string{"activate", "--help"},
			want: []string{"activate <profile-or-env>"},
		},
		{
			name: "use",
			args: []string{"use", "--help"},
			want: []string{"use <profile-or-env>"},
		},
		{
			name: "status",
			args: []string{"status", "--help"},
			want: []string{"Show AVM activation and runtime status"},
		},
		{
			name: "shell init",
			args: []string{"shell", "init", "--help"},
			want: []string{"init <shell>"},
		},
		{
			name: "deactivate",
			args: []string{"deactivate", "--help"},
			want: []string{"Deactivate the current AVM profile or environment"},
		},
		{
			name: "sync",
			args: []string{"sync", "--help"},
			want: []string{"Sync the current AVM active profile or environment", "--target"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := executeCommand(tt.args...)
			if err != nil {
				t.Fatalf("help returned error: %v", err)
			}
			for _, want := range tt.want {
				if !strings.Contains(got, want) {
					t.Fatalf("help output missing %q:\n%s", want, got)
				}
			}
		})
	}
}

func TestSkeletonCommandsReturnNotImplemented(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "memory import", args: []string{"memory", "import"}, want: "avm memory import: not implemented"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := executeCommand(tt.args...)
			if err == nil {
				t.Fatal("expected not implemented error")
			}
			if got := err.Error(); got != tt.want {
				t.Fatalf("unexpected error:\n got: %q\nwant: %q", got, tt.want)
			}
		})
	}
}
