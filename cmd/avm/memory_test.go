package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMemoryImportDryRunTextOutput(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	source := filepath.Join("..", "..", "testdata", "memory", "backend-standards.md")
	got, err := executeCommand("memory", "import", "--from", source, "--dry-run")
	if err != nil {
		t.Fatalf("memory import dry-run returned error: %v", err)
	}

	for _, want := range []string{
		"Memory import dry-run: file",
		"Status counts:",
		"new:",
		"changed:",
		"conflict:",
		"skipped:",
		"No files written.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "not implemented") {
		t.Fatalf("dry-run unexpectedly returned skeleton output:\n%s", got)
	}
}

func TestMemoryImportDryRunJSONOutputIsStable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	source := filepath.Join("..", "..", "testdata", "memory", "backend-standards.md")
	before, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("failed to read source fixture: %v", err)
	}
	first, err := executeCommand("memory", "import", "--from", source, "--dry-run", "--format", "json")
	if err != nil {
		t.Fatalf("first memory import dry-run returned error: %v", err)
	}
	second, err := executeCommand("memory", "import", "--from", source, "--dry-run", "--format", "json")
	if err != nil {
		t.Fatalf("second memory import dry-run returned error: %v", err)
	}
	if first != second {
		t.Fatalf("json output is not stable:\nfirst:\n%s\nsecond:\n%s", first, second)
	}

	for _, want := range []string{
		`"dry_run": true`,
		`"status": "new"`,
		`"status": "changed"`,
		`"status": "conflict"`,
		`"status": "skipped"`,
	} {
		if !strings.Contains(first, want) {
			t.Fatalf("json output missing %q:\n%s", want, first)
		}
	}
	if _, err := os.Stat(filepath.Join(home, ".avm", "memory")); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote AVM memory dir or returned unexpected stat error: %v", err)
	}
	after, err := os.ReadFile(source)
	if err != nil {
		t.Fatalf("failed to reread source fixture: %v", err)
	}
	if string(before) != string(after) {
		t.Fatal("dry-run modified the source memory file")
	}
}
