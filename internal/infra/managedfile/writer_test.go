package managedfile

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/xz1220/agent-vm/internal/runtime"
)

func TestApply_CreatesAndUpdates(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.toml")
	b := filepath.Join(dir, "sub", "b.json")

	files := []runtime.ManagedFile{
		{Path: a, Mode: 0o644, Contents: []byte("v1")},
		{Path: b, Mode: 0o600, Contents: []byte("hello")},
	}

	w := New()
	diffs, err := w.Apply(context.Background(), files)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(diffs) != 2 {
		t.Fatalf("expected 2 created entries, got %d: %+v", len(diffs), diffs)
	}
	for _, d := range diffs {
		if d.Reason != "created" {
			t.Fatalf("expected created, got %+v", d)
		}
	}

	// Re-applying same contents → no diffs.
	diffs, err = w.Apply(context.Background(), files)
	if err != nil {
		t.Fatalf("re-apply: %v", err)
	}
	if len(diffs) != 0 {
		t.Fatalf("expected no diffs on no-op apply, got %+v", diffs)
	}

	// Mutate one file.
	files[0].Contents = []byte("v2")
	diffs, err = w.Apply(context.Background(), files)
	if err != nil {
		t.Fatalf("mutated apply: %v", err)
	}
	if len(diffs) != 1 || diffs[0].Reason != "updated" {
		t.Fatalf("expected one updated diff, got %+v", diffs)
	}
	got, _ := os.ReadFile(a)
	if !bytes.Equal(got, []byte("v2")) {
		t.Fatalf("file contents not updated")
	}

	// Permissions on b.
	st, err := os.Stat(b)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("mode: %o", st.Mode().Perm())
	}
}

func TestApply_DefaultMode(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.txt")
	w := New()
	if _, err := w.Apply(context.Background(), []runtime.ManagedFile{{Path: p, Contents: []byte("x")}}); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o644 {
		t.Fatalf("default mode: %o", st.Mode().Perm())
	}
}

func TestDryRun_DoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.txt")
	w := New()
	diffs, err := w.DryRun(context.Background(), []runtime.ManagedFile{{Path: p, Contents: []byte("x")}})
	if err != nil {
		t.Fatal(err)
	}
	if len(diffs) != 1 || diffs[0].Reason != "created" {
		t.Fatalf("expected created diff, got %+v", diffs)
	}
	if _, err := os.Stat(p); err == nil {
		t.Fatal("DryRun must not write the file")
	}
}

func TestDryRun_ExistingMatch(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.txt")
	if err := os.WriteFile(p, []byte("same"), 0o644); err != nil {
		t.Fatal(err)
	}
	w := New()
	diffs, err := w.DryRun(context.Background(), []runtime.ManagedFile{{Path: p, Contents: []byte("same")}})
	if err != nil {
		t.Fatal(err)
	}
	if len(diffs) != 0 {
		t.Fatalf("expected no diff for matching contents, got %+v", diffs)
	}
}

func TestApply_EmptyPath(t *testing.T) {
	w := New()
	if _, err := w.Apply(context.Background(), []runtime.ManagedFile{{Contents: []byte("x")}}); err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestApply_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w := New()
	_, err := w.Apply(ctx, []runtime.ManagedFile{{Path: "x", Contents: []byte("x")}})
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
}
