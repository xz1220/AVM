package runlog

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/xz1220/agent-vm/internal/app/model"
)

func mkRec(name string, ts time.Time) model.RunRecord {
	return model.RunRecord{
		Agent:     name,
		Runtime:   "codex",
		StartedAt: ts,
		EndedAt:   ts.Add(time.Second),
		ExitCode:  0,
	}
}

func TestAppendList(t *testing.T) {
	l := New(t.TempDir())
	got, err := l.List(0)
	if err != nil {
		t.Fatalf("List on empty: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty list, got %d", len(got))
	}

	now := time.Now().UTC().Truncate(time.Second)
	for i := 0; i < 5; i++ {
		rec := mkRec("a", now.Add(time.Duration(i)*time.Second))
		if err := l.Append(rec); err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
	}

	all, err := l.List(0)
	if err != nil {
		t.Fatalf("List all: %v", err)
	}
	if len(all) != 5 {
		t.Fatalf("expected 5, got %d", len(all))
	}

	last3, err := l.List(3)
	if err != nil {
		t.Fatalf("List 3: %v", err)
	}
	if len(last3) != 3 {
		t.Fatalf("expected 3, got %d", len(last3))
	}
	// The last 3 should be the latest entries.
	if !last3[2].StartedAt.Equal(all[4].StartedAt) {
		t.Fatalf("order wrong: %+v", last3)
	}

	bigger, err := l.List(99)
	if err != nil {
		t.Fatalf("List 99: %v", err)
	}
	if len(bigger) != 5 {
		t.Fatalf("expected 5, got %d", len(bigger))
	}
}

func TestList_NegativeLimit(t *testing.T) {
	l := New(t.TempDir())
	if _, err := l.List(-1); err == nil {
		t.Fatal("expected error for negative limit")
	}
}

func TestList_SkipsMalformedLines(t *testing.T) {
	dir := t.TempDir()
	l := New(dir)
	if err := l.Append(mkRec("a", time.Now())); err != nil {
		t.Fatal(err)
	}
	// Inject a garbage line.
	f, err := os.OpenFile(filepath.Join(dir, FileName), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("not json\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if err := l.Append(mkRec("b", time.Now())); err != nil {
		t.Fatal(err)
	}

	got, err := l.List(0)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 valid entries, got %d", len(got))
	}
	if got[0].Agent != "a" || got[1].Agent != "b" {
		t.Fatalf("unexpected order: %+v", got)
	}
}
