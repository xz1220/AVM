package memory

import (
	"os"
	"path/filepath"
	"testing"
)

func TestImportDryRunMarkdownNewDoesNotWriteHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	source := filepath.Join("..", "..", "testdata", "memory", "backend-standards.md")
	plan, err := ImportDryRun(ImportOptions{Source: source, DryRun: true})
	if err != nil {
		t.Fatalf("ImportDryRun returned error: %v", err)
	}

	if plan.Source != filepath.Clean(source) || !plan.DryRun {
		t.Fatalf("unexpected plan metadata: %#v", plan)
	}
	if len(plan.Candidates) != 1 {
		t.Fatalf("expected one candidate, got %d", len(plan.Candidates))
	}
	candidate := plan.Candidates[0]
	if candidate.ID != "backend-standards" || candidate.Scope != "project" || candidate.Format != "markdown" {
		t.Fatalf("unexpected candidate: %#v", candidate)
	}
	if len(plan.Diffs) != 1 || plan.Diffs[0].Status != DiffStatusNew {
		t.Fatalf("unexpected diffs: %#v", plan.Diffs)
	}
	assertAllStatusCountsPresent(t, plan)
	if _, err := os.Stat(filepath.Join(home, ".avm")); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote AVM home or returned unexpected stat error: %v", err)
	}
}

func TestImportDryRunYAMLInput(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	source := filepath.Join("..", "..", "testdata", "memory", "yaml-standards.yaml")
	plan, err := ImportDryRun(ImportOptions{Source: source, DryRun: true})
	if err != nil {
		t.Fatalf("ImportDryRun returned error: %v", err)
	}

	if len(plan.Candidates) != 1 {
		t.Fatalf("expected one candidate, got %d", len(plan.Candidates))
	}
	candidate := plan.Candidates[0]
	if candidate.ID != "yaml-standards" || candidate.Format != "yaml" {
		t.Fatalf("unexpected yaml candidate: %#v", candidate)
	}
	if got := plan.Diffs[0].Preview; got != "rules:" {
		t.Fatalf("unexpected preview: %q", got)
	}
}

func TestImportDryRunDiffStatuses(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	sourceDir := t.TempDir()
	writeFile(t, filepath.Join(sourceDir, "changed-memory.md"), "new content\n")
	writeFile(t, filepath.Join(sourceDir, "skipped-memory.md"), "same content\n")
	writeFile(t, filepath.Join(sourceDir, "conflict-memory.md"), "candidate content\n")

	memoryDir := filepath.Join(home, ".avm", "memory", "project")
	writeFile(t, filepath.Join(memoryDir, "changed-memory.md"), "old content\n")
	writeFile(t, filepath.Join(memoryDir, "skipped-memory.md"), "same content\n")
	writeFile(t, filepath.Join(memoryDir, "conflict-memory.yaml"), "id: conflict-memory\nscope: project\nformat: yaml\npath: "+filepath.Join(memoryDir, "conflict-memory.yaml")+"\nmode: read\n")

	tests := []struct {
		name   string
		source string
		want   DiffStatus
	}{
		{name: "changed", source: filepath.Join(sourceDir, "changed-memory.md"), want: DiffStatusChanged},
		{name: "skipped", source: filepath.Join(sourceDir, "skipped-memory.md"), want: DiffStatusSkipped},
		{name: "conflict", source: filepath.Join(sourceDir, "conflict-memory.md"), want: DiffStatusConflict},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan, err := ImportDryRun(ImportOptions{Source: tt.source, DryRun: true})
			if err != nil {
				t.Fatalf("ImportDryRun returned error: %v", err)
			}
			if len(plan.Diffs) != 1 || plan.Diffs[0].Status != tt.want {
				t.Fatalf("unexpected diffs: %#v", plan.Diffs)
			}
		})
	}
}

func TestImportDryRunUnsupportedExtensionIsSkipped(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	source := filepath.Join(t.TempDir(), "memory.txt")
	writeFile(t, source, "plain text")

	plan, err := ImportDryRun(ImportOptions{Source: source, DryRun: true})
	if err != nil {
		t.Fatalf("ImportDryRun returned error: %v", err)
	}
	if len(plan.Candidates) != 0 {
		t.Fatalf("expected no candidates, got %#v", plan.Candidates)
	}
	if len(plan.Diffs) != 1 || plan.Diffs[0].Status != DiffStatusSkipped {
		t.Fatalf("unexpected diffs: %#v", plan.Diffs)
	}
}

func TestDiffStatusesAreConstrained(t *testing.T) {
	for _, status := range DiffStatuses() {
		if !status.Valid() {
			t.Fatalf("allowed status is invalid: %q", status)
		}
	}
	if DiffStatus("updated").Valid() {
		t.Fatal("unexpected valid status")
	}
}

func assertAllStatusCountsPresent(t *testing.T, plan *MemoryImportPlan) {
	t.Helper()
	if len(plan.StatusCounts) != len(DiffStatuses()) {
		t.Fatalf("missing status counts: %#v", plan.StatusCounts)
	}
	for i, status := range DiffStatuses() {
		if plan.StatusCounts[i].Status != status {
			t.Fatalf("status count order mismatch: %#v", plan.StatusCounts)
		}
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
}
