package capstore

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xz1220/agent-vm/internal/app/model"
)

func sampleRec(name string) model.CapabilityRecord {
	return model.CapabilityRecord{
		Kind:   model.CapabilityKindSkill,
		Name:   name,
		Source: model.SourceAVM,
	}
}

func TestAddGetIsIdempotent(t *testing.T) {
	s := New(t.TempDir())
	rec := sampleRec("hello")
	payload := []byte("# hello world\n")

	id1, err := s.Add(rec, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if !strings.HasPrefix(string(id1), "cap_") || len(id1) != len("cap_")+32 {
		t.Fatalf("bad id: %q", id1)
	}

	id2, err := s.Add(rec, bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("second Add: %v", err)
	}
	if id1 != id2 {
		t.Fatalf("expected idempotent, got %s vs %s", id1, id2)
	}

	got, err := s.Get(id1)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "hello" || got.Kind != model.CapabilityKindSkill {
		t.Fatalf("bad record: %+v", got)
	}
	if got.Checksum == "" {
		t.Fatalf("checksum not populated")
	}
}

func TestAdd_DifferentContentDifferentID(t *testing.T) {
	s := New(t.TempDir())
	rec := sampleRec("hello")
	id1, err := s.Add(rec, bytes.NewReader([]byte("v1")))
	if err != nil {
		t.Fatal(err)
	}
	id2, err := s.Add(rec, bytes.NewReader([]byte("v2")))
	if err != nil {
		t.Fatal(err)
	}
	if id1 == id2 {
		t.Fatalf("expected different IDs for different content")
	}
}

func TestAdd_RequiresKindAndName(t *testing.T) {
	s := New(t.TempDir())
	if _, err := s.Add(model.CapabilityRecord{}, bytes.NewReader([]byte("x"))); err == nil {
		t.Fatal("expected error for missing kind/name")
	}
}

func TestGet_NotFound(t *testing.T) {
	s := New(t.TempDir())
	_, err := s.Get("cap_does_not_exist")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestList(t *testing.T) {
	s := New(t.TempDir())
	if _, err := s.Add(sampleRec("a"), bytes.NewReader([]byte("x"))); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Add(sampleRec("b"), bytes.NewReader([]byte("y"))); err != nil {
		t.Fatal(err)
	}
	got, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 records, got %d", len(got))
	}
}

func TestMaterializeAndRemove(t *testing.T) {
	dir := t.TempDir()
	s := New(dir)
	id, err := s.Add(sampleRec("hello"), bytes.NewReader([]byte("payload-bytes")))
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "boundary")
	if err := s.Materialize([]model.CapabilityID{id}, target); err != nil {
		t.Fatalf("Materialize: %v", err)
	}
	link := filepath.Join(target, "skills", "hello")
	data, err := os.ReadFile(link)
	if err != nil {
		t.Fatalf("read materialized: %v", err)
	}
	if string(data) != "payload-bytes" {
		t.Fatalf("contents: %q", data)
	}
	// Idempotent.
	if err := s.Materialize([]model.CapabilityID{id}, target); err != nil {
		t.Fatalf("Materialize again: %v", err)
	}

	if err := s.Remove(id); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if _, err := s.Get(id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound after Remove, got %v", err)
	}
}

func TestMaterialize_UnknownID(t *testing.T) {
	s := New(t.TempDir())
	err := s.Materialize([]model.CapabilityID{"cap_missing"}, t.TempDir())
	if err == nil {
		t.Fatal("expected error for unknown id")
	}
}
