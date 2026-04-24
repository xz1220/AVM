package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRootHelp(t *testing.T) {
	cmd := newRootCommand()
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("help returned error: %v", err)
	}

	got := out.String()
	for _, want := range []string{"Agent VM", "avm", "init"} {
		if !strings.Contains(got, want) {
			t.Fatalf("help output missing %q:\n%s", want, got)
		}
	}
}

func TestVersionFlag(t *testing.T) {
	cmd := newRootCommand()
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs([]string{"--version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("version returned error: %v", err)
	}
	if got := strings.TrimSpace(out.String()); !strings.HasPrefix(got, "avm ") {
		t.Fatalf("unexpected version output: %q", got)
	}
}
