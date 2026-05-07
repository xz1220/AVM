package agentstore

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/xz1220/agent-vm/internal/app/model"
)

func makeAgent(name string) *model.Agent {
	return &model.Agent{
		Identity: model.Identity{Name: name, Description: "desc-" + name},
		Runtimes: []model.RuntimePref{{Runtime: "codex", Default: true}},
	}
}

func TestSaveGetExistsSourcePath(t *testing.T) {
	dir := t.TempDir()
	r := New(dir)

	if r.Exists("foo") {
		t.Fatal("Exists should be false initially")
	}

	a := makeAgent("foo")
	if err := r.Save(a); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if !r.Exists("foo") {
		t.Fatal("Exists should be true after save")
	}

	got, err := r.Get("foo")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Identity.Name != "foo" {
		t.Fatalf("got name %q", got.Identity.Name)
	}

	src, err := r.SourcePath("foo")
	if err != nil {
		t.Fatalf("SourcePath: %v", err)
	}
	if src != filepath.Join(dir, "foo.yaml") {
		t.Fatalf("source path: %q", src)
	}
}

func TestSave_ConflictWithoutOverwrite(t *testing.T) {
	r := New(t.TempDir())
	if err := r.Save(makeAgent("dup")); err != nil {
		t.Fatalf("first save: %v", err)
	}
	err := r.Save(makeAgent("dup"))
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}

func TestSave_OverwriteTrue(t *testing.T) {
	r := &FSRepo{Dir: t.TempDir(), Overwrite: true}
	a := makeAgent("dup")
	if err := r.Save(a); err != nil {
		t.Fatalf("first save: %v", err)
	}
	a.Identity.Description = "updated"
	if err := r.Save(a); err != nil {
		t.Fatalf("second save: %v", err)
	}
	got, _ := r.Get("dup")
	if got.Identity.Description != "updated" {
		t.Fatalf("description not updated: %q", got.Identity.Description)
	}
}

func TestSave_InvalidName(t *testing.T) {
	r := New(t.TempDir())
	a := makeAgent("Bad Name")
	if err := r.Save(a); err == nil {
		t.Fatal("expected validation error for bad name")
	}
}

func TestGet_NotFound(t *testing.T) {
	r := New(t.TempDir())
	_, err := r.Get("missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestList(t *testing.T) {
	r := New(t.TempDir())
	if err := r.Save(makeAgent("zeta")); err != nil {
		t.Fatal(err)
	}
	if err := r.Save(makeAgent("alpha")); err != nil {
		t.Fatal(err)
	}
	// stray non-yaml file should be ignored
	if err := os.WriteFile(filepath.Join(r.Dir, "README.md"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := r.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(got))
	}
	if got[0].Name != "alpha" || got[1].Name != "zeta" {
		t.Fatalf("unsorted: %+v", got)
	}
	if len(got[0].Runtimes) != 1 || got[0].Runtimes[0] != "codex" {
		t.Fatalf("runtimes projection: %+v", got[0].Runtimes)
	}
}

func TestList_MissingDir(t *testing.T) {
	r := New(filepath.Join(t.TempDir(), "does-not-exist"))
	got, err := r.List()
	if err != nil {
		t.Fatalf("List should tolerate missing dir, got %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty list, got %d", len(got))
	}
}

func TestDelete(t *testing.T) {
	r := New(t.TempDir())
	if err := r.Save(makeAgent("foo")); err != nil {
		t.Fatal(err)
	}
	if err := r.Delete("foo"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if r.Exists("foo") {
		t.Fatal("still exists")
	}
	if err := r.Delete("foo"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}
