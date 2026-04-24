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
			want: []string{"agent", "deactivate", "env", "init", "shell", "status", "use"},
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
			want: []string{"create <name>", "--codex", "--claude-code", "--cline", "--cursor"},
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
		{name: "agent create", args: []string{"agent", "create", "backend-coder"}, want: "avm agent create: not implemented"},
		{name: "agent list", args: []string{"agent", "list"}, want: "avm agent list: not implemented"},
		{name: "agent show", args: []string{"agent", "show", "backend-coder"}, want: "avm agent show: not implemented"},
		{name: "env create", args: []string{"env", "create", "backend-dev"}, want: "avm env create: not implemented"},
		{name: "use", args: []string{"use", "backend-coder"}, want: "avm use: not implemented"},
		{name: "status", args: []string{"status"}, want: "avm status: not implemented"},
		{name: "shell init", args: []string{"shell", "init", "zsh"}, want: "avm shell init: not implemented"},
		{name: "deactivate", args: []string{"deactivate"}, want: "avm deactivate: not implemented"},
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
