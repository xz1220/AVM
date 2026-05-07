package service

import (
	"bytes"
	"context"
	"testing"

	"github.com/xz1220/agent-vm/internal/app/model"
	"github.com/xz1220/agent-vm/internal/infra/capstore"
	"github.com/xz1220/agent-vm/internal/runtime"
)

func TestCapabilities_Discover_AVMOnly(t *testing.T) {
	store := capstore.New(t.TempDir())
	if _, err := store.Add(model.CapabilityRecord{
		Kind: model.CapabilityKindSkill, Name: "alpha",
	}, bytes.NewReader([]byte("a"))); err != nil {
		t.Fatalf("Add: %v", err)
	}
	s := NewCapabilities(store, runtime.NewRegistry())
	got, err := s.Discover(context.Background(), model.DiscoverRequest{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(got) != 1 || got[0].Source != model.SourceAVM || got[0].Name != "alpha" {
		t.Fatalf("got %+v", got)
	}
}

func TestCapabilities_Discover_RuntimeGlobal(t *testing.T) {
	store := capstore.New(t.TempDir())
	driver := &fakeDriver{
		name: "fake",
		globals: []model.GlobalCapability{
			{Runtime: "fake", Kind: model.CapabilityKindMCP, Name: "global-x"},
		},
	}
	reg := registryWith(t, driver)
	s := NewCapabilities(store, reg)
	got, err := s.Discover(context.Background(), model.DiscoverRequest{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(got) != 1 || got[0].Source != model.SourceRuntimeGlobal {
		t.Fatalf("got %+v", got)
	}
}

func TestCapabilities_Discover_KindFilter(t *testing.T) {
	store := capstore.New(t.TempDir())
	if _, err := store.Add(model.CapabilityRecord{
		Kind: model.CapabilityKindSkill, Name: "alpha",
	}, bytes.NewReader([]byte("a"))); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if _, err := store.Add(model.CapabilityRecord{
		Kind: model.CapabilityKindMCP, Name: "beta",
	}, bytes.NewReader([]byte("b"))); err != nil {
		t.Fatalf("Add: %v", err)
	}
	s := NewCapabilities(store, runtime.NewRegistry())
	got, err := s.Discover(context.Background(), model.DiscoverRequest{
		Kinds: []model.CapabilityKind{model.CapabilityKindSkill},
	})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(got) != 1 || got[0].Name != "alpha" {
		t.Fatalf("expected only skill, got %+v", got)
	}
}

func TestCapabilities_Discover_ConflictMarker(t *testing.T) {
	store := capstore.New(t.TempDir())
	if _, err := store.Add(model.CapabilityRecord{
		Kind: model.CapabilityKindSkill, Name: "shared",
	}, bytes.NewReader([]byte("a"))); err != nil {
		t.Fatalf("Add: %v", err)
	}
	driver := &fakeDriver{
		name: "fake",
		globals: []model.GlobalCapability{
			{Runtime: "fake", Kind: model.CapabilityKindSkill, Name: "shared"},
		},
	}
	reg := registryWith(t, driver)
	s := NewCapabilities(store, reg)
	got, err := s.Discover(context.Background(), model.DiscoverRequest{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 candidates, got %d (%+v)", len(got), got)
	}
	for _, c := range got {
		if !c.Conflict {
			t.Fatalf("expected Conflict=true, got %+v", c)
		}
	}
}

func TestCapabilities_Discover_RuntimeFilter(t *testing.T) {
	store := capstore.New(t.TempDir())
	d1 := &fakeDriver{
		name: "rt1",
		globals: []model.GlobalCapability{
			{Runtime: "rt1", Kind: model.CapabilityKindSkill, Name: "one"},
		},
	}
	d2 := &fakeDriver{
		name: "rt2",
		globals: []model.GlobalCapability{
			{Runtime: "rt2", Kind: model.CapabilityKindSkill, Name: "two"},
		},
	}
	reg := registryWith(t, d1, d2)
	s := NewCapabilities(store, reg)
	got, err := s.Discover(context.Background(), model.DiscoverRequest{
		Runtimes: []string{"rt2"},
	})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(got) != 1 || got[0].Name != "two" {
		t.Fatalf("got %+v", got)
	}
}
