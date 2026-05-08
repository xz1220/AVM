package service

import (
	"bytes"
	"context"
	"errors"
	"strings"
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

// TestCapabilities_Discover_MarksImported verifies that runtime-global
// candidates whose (kind,name) already lives in capstore get Imported=true.
func TestCapabilities_Discover_MarksImported(t *testing.T) {
	store := capstore.New(t.TempDir())
	if _, err := store.Add(model.CapabilityRecord{
		Kind:   model.CapabilityKindSkill,
		Name:   "shared",
		Source: model.SourceRuntimeGlobal,
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
	// Find the runtime-global candidate; it should be Imported=true.
	var rg *model.CapabilityCandidate
	for i := range got {
		if got[i].Source == model.SourceRuntimeGlobal {
			rg = &got[i]
		}
	}
	if rg == nil {
		t.Fatalf("expected a runtime-global candidate, got %+v", got)
	}
	if !rg.Imported {
		t.Fatalf("expected Imported=true on runtime-global, got %+v", rg)
	}
}

// helpers for Import / Bootstrap tests

func driverWithSkillExport(name, skillName, body string) *fakeDriver {
	return &fakeDriver{
		name: name,
		globals: []model.GlobalCapability{
			{Runtime: name, Kind: model.CapabilityKindSkill, Name: skillName, Path: "/fake/skills/" + skillName},
		},
		exports: map[string]fakeExport{
			"skill:" + skillName: {
				format:   model.PayloadFormatSkillMD,
				body:     body,
				filename: "SKILL.md",
			},
		},
	}
}

func TestCapabilities_Import_Skill_Success(t *testing.T) {
	store := capstore.New(t.TempDir())
	driver := driverWithSkillExport("fake", "hello", "# hello content\n")
	s := NewCapabilities(store, registryWith(t, driver))

	res, err := s.Import(context.Background(), model.ImportCapabilityRequest{
		Runtime: "fake", Kind: model.CapabilityKindSkill, Name: "hello",
	})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if res == nil || !res.Created {
		t.Fatalf("expected Created=true, got %+v", res)
	}
	if !strings.HasPrefix(string(res.ID), "cap_") {
		t.Fatalf("expected derived cap ID, got %s", res.ID)
	}
	rec, err := store.Get(res.ID)
	if err != nil {
		t.Fatalf("Get after Import: %v", err)
	}
	if rec.Format != model.PayloadFormatSkillMD || rec.Source != model.SourceRuntimeGlobal {
		t.Fatalf("record fields wrong: %+v", rec)
	}
}

func TestCapabilities_Import_MCP_Success(t *testing.T) {
	store := capstore.New(t.TempDir())
	driver := &fakeDriver{
		name: "fake",
		globals: []model.GlobalCapability{
			{Runtime: "fake", Kind: model.CapabilityKindMCP, Name: "gh", Path: "/fake/.config"},
		},
		exports: map[string]fakeExport{
			"mcp:gh": {
				format:   model.PayloadFormatMCPConfigV1,
				body:     `{"kind":"mcp","name":"gh","command":"npx"}`,
				filename: "mcp.json",
			},
		},
	}
	s := NewCapabilities(store, registryWith(t, driver))
	res, err := s.Import(context.Background(), model.ImportCapabilityRequest{
		Runtime: "fake", Kind: model.CapabilityKindMCP, Name: "gh",
	})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if res == nil || !res.Created {
		t.Fatalf("expected Created=true, got %+v", res)
	}
	rec, err := store.Get(res.ID)
	if err != nil {
		t.Fatalf("Get after Import: %v", err)
	}
	if rec.Kind != model.CapabilityKindMCP || rec.Format != model.PayloadFormatMCPConfigV1 {
		t.Fatalf("record fields wrong: %+v", rec)
	}
}

func TestCapabilities_Import_Dedup(t *testing.T) {
	store := capstore.New(t.TempDir())
	driver := driverWithSkillExport("fake", "hello", "# hello\n")
	s := NewCapabilities(store, registryWith(t, driver))

	first, err := s.Import(context.Background(), model.ImportCapabilityRequest{
		Runtime: "fake", Kind: model.CapabilityKindSkill, Name: "hello",
	})
	if err != nil {
		t.Fatalf("first Import: %v", err)
	}
	second, err := s.Import(context.Background(), model.ImportCapabilityRequest{
		Runtime: "fake", Kind: model.CapabilityKindSkill, Name: "hello",
	})
	if err != nil {
		t.Fatalf("second Import: %v", err)
	}
	if second.Created {
		t.Fatalf("expected Created=false on dedup, got %+v", second)
	}
	if first.ID != second.ID {
		t.Fatalf("expected same ID, got %s vs %s", first.ID, second.ID)
	}
}

func TestCapabilities_Import_ConflictCancel(t *testing.T) {
	store := capstore.New(t.TempDir())
	// First import "v1"
	driver1 := driverWithSkillExport("fake", "hello", "# hello v1\n")
	s := NewCapabilities(store, registryWith(t, driver1))
	if _, err := s.Import(context.Background(), model.ImportCapabilityRequest{
		Runtime: "fake", Kind: model.CapabilityKindSkill, Name: "hello",
	}); err != nil {
		t.Fatalf("first Import: %v", err)
	}
	// Now driver returns different content. Default OnConflict (cancel) → CONFLICT.
	driver2 := driverWithSkillExport("fake2", "hello", "# hello v2\n")
	s2 := NewCapabilities(store, registryWith(t, driver2))
	_, err := s2.Import(context.Background(), model.ImportCapabilityRequest{
		Runtime: "fake2", Kind: model.CapabilityKindSkill, Name: "hello",
	})
	if err == nil {
		t.Fatalf("expected conflict error, got nil")
	}
	se := AsError(err)
	if se == nil || se.Code != CodeCapabilityConflict {
		t.Fatalf("expected CodeCapabilityConflict, got %+v", err)
	}
	if got, _ := se.Details["name"].(string); got != "hello" {
		t.Fatalf("expected details.name=hello, got %v", se.Details)
	}
}

func TestCapabilities_Import_ConflictSkip(t *testing.T) {
	store := capstore.New(t.TempDir())
	driver1 := driverWithSkillExport("fake", "hello", "# hello v1\n")
	s := NewCapabilities(store, registryWith(t, driver1))
	first, err := s.Import(context.Background(), model.ImportCapabilityRequest{
		Runtime: "fake", Kind: model.CapabilityKindSkill, Name: "hello",
	})
	if err != nil {
		t.Fatalf("first Import: %v", err)
	}
	// Different content, but OnConflict=skip → no error, returns existing ID.
	driver2 := driverWithSkillExport("fake2", "hello", "# hello v2\n")
	s2 := NewCapabilities(store, registryWith(t, driver2))
	res, err := s2.Import(context.Background(), model.ImportCapabilityRequest{
		Runtime: "fake2", Kind: model.CapabilityKindSkill, Name: "hello",
		OnConflict: model.ResolveSkip,
	})
	if err != nil {
		t.Fatalf("Import skip: %v", err)
	}
	if res.Created || res.Replaced {
		t.Fatalf("expected Created=false and Replaced=false on skip, got %+v", res)
	}
	if res.ID != first.ID {
		t.Fatalf("expected existing ID returned, got %s want %s", res.ID, first.ID)
	}
}

func TestCapabilities_Import_ConflictOverwrite(t *testing.T) {
	store := capstore.New(t.TempDir())
	driver1 := driverWithSkillExport("fake", "hello", "# hello v1\n")
	s := NewCapabilities(store, registryWith(t, driver1))
	first, err := s.Import(context.Background(), model.ImportCapabilityRequest{
		Runtime: "fake", Kind: model.CapabilityKindSkill, Name: "hello",
	})
	if err != nil {
		t.Fatalf("first Import: %v", err)
	}
	driver2 := driverWithSkillExport("fake2", "hello", "# hello v2 different\n")
	s2 := NewCapabilities(store, registryWith(t, driver2))
	res, err := s2.Import(context.Background(), model.ImportCapabilityRequest{
		Runtime: "fake2", Kind: model.CapabilityKindSkill, Name: "hello",
		OnConflict: model.ResolveOverwrite,
	})
	if err != nil {
		t.Fatalf("Import overwrite: %v", err)
	}
	if !res.Replaced || !res.Created {
		t.Fatalf("expected Created=true, Replaced=true, got %+v", res)
	}
	if res.ID == first.ID {
		t.Fatalf("expected new ID after overwrite, got same %s", res.ID)
	}
	// Old ID should be gone.
	if _, err := store.Get(first.ID); err == nil {
		t.Fatalf("expected old record removed, still resolves")
	}
}

func TestCapabilities_Import_RuntimeNotFound(t *testing.T) {
	store := capstore.New(t.TempDir())
	s := NewCapabilities(store, runtime.NewRegistry())
	_, err := s.Import(context.Background(), model.ImportCapabilityRequest{
		Runtime: "ghost", Kind: model.CapabilityKindSkill, Name: "hello",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	se := AsError(err)
	if se == nil || se.Code != CodeRuntimeNotFound {
		t.Fatalf("expected CodeRuntimeNotFound, got %+v", err)
	}
}

func TestCapabilities_Import_GlobalNotFound(t *testing.T) {
	store := capstore.New(t.TempDir())
	driver := &fakeDriver{name: "fake"} // no globals, no exports
	s := NewCapabilities(store, registryWith(t, driver))
	_, err := s.Import(context.Background(), model.ImportCapabilityRequest{
		Runtime: "fake", Kind: model.CapabilityKindSkill, Name: "missing",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	se := AsError(err)
	if se == nil || se.Code != CodeCapabilityNotFound {
		t.Fatalf("expected CodeCapabilityNotFound, got %+v", err)
	}
}

func TestCapabilities_Import_MissingFields(t *testing.T) {
	store := capstore.New(t.TempDir())
	s := NewCapabilities(store, runtime.NewRegistry())
	cases := []model.ImportCapabilityRequest{
		{},
		{Runtime: "fake"},
		{Runtime: "fake", Kind: model.CapabilityKindSkill},
	}
	for i, c := range cases {
		_, err := s.Import(context.Background(), c)
		if err == nil {
			t.Fatalf("case %d: expected error, got nil", i)
		}
		se := AsError(err)
		if se == nil || se.Code != CodeMissingInput {
			t.Fatalf("case %d: expected CodeMissingInput, got %+v", i, err)
		}
	}
}

func TestCapabilities_Bootstrap_AllSkills(t *testing.T) {
	store := capstore.New(t.TempDir())
	driver := &fakeDriver{
		name: "fake",
		globals: []model.GlobalCapability{
			{Runtime: "fake", Kind: model.CapabilityKindSkill, Name: "alpha", Path: "/p/alpha"},
			{Runtime: "fake", Kind: model.CapabilityKindSkill, Name: "beta", Path: "/p/beta"},
		},
		exports: map[string]fakeExport{
			"skill:alpha": {format: model.PayloadFormatSkillMD, body: "# alpha\n", filename: "SKILL.md"},
			"skill:beta":  {format: model.PayloadFormatSkillMD, body: "# beta\n", filename: "SKILL.md"},
		},
	}
	s := NewCapabilities(store, registryWith(t, driver))
	res, err := s.Bootstrap(context.Background(), model.BootstrapCapabilitiesRequest{
		Runtime: "fake",
	})
	if err != nil {
		t.Fatalf("Bootstrap: %v", err)
	}
	if len(res.Imported) != 2 {
		t.Fatalf("expected 2 imported, got %d (%+v)", len(res.Imported), res)
	}
	if len(res.Skipped) != 0 {
		t.Fatalf("expected no skipped, got %+v", res.Skipped)
	}
}

func TestCapabilities_Bootstrap_PartialFailure(t *testing.T) {
	store := capstore.New(t.TempDir())
	// alpha will succeed; beta has no export → ErrGlobalCapabilityNotFound.
	driver := &fakeDriver{
		name: "fake",
		globals: []model.GlobalCapability{
			{Runtime: "fake", Kind: model.CapabilityKindSkill, Name: "alpha", Path: "/p/alpha"},
			{Runtime: "fake", Kind: model.CapabilityKindSkill, Name: "beta", Path: "/p/beta"},
		},
		exports: map[string]fakeExport{
			"skill:alpha": {format: model.PayloadFormatSkillMD, body: "# alpha\n", filename: "SKILL.md"},
			// no skill:beta
		},
	}
	s := NewCapabilities(store, registryWith(t, driver))
	res, err := s.Bootstrap(context.Background(), model.BootstrapCapabilitiesRequest{
		Runtime: "fake",
	})
	if err != nil {
		t.Fatalf("Bootstrap should not error on per-item failure: %v", err)
	}
	if len(res.Imported) != 1 || res.Imported[0].ID == "" {
		t.Fatalf("expected 1 import, got %+v", res.Imported)
	}
	if len(res.Skipped) != 1 || res.Skipped[0].Name != "beta" {
		t.Fatalf("expected 1 skip for beta, got %+v", res.Skipped)
	}
}

func TestCapabilities_Bootstrap_RuntimeNotFound(t *testing.T) {
	store := capstore.New(t.TempDir())
	s := NewCapabilities(store, runtime.NewRegistry())
	_, err := s.Bootstrap(context.Background(), model.BootstrapCapabilitiesRequest{
		Runtime: "ghost",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	se := AsError(err)
	if se == nil || se.Code != CodeRuntimeNotFound {
		t.Fatalf("expected CodeRuntimeNotFound, got %+v", err)
	}
}

func TestCapabilities_List_PureCapstore(t *testing.T) {
	store := capstore.New(t.TempDir())
	if _, err := store.Add(model.CapabilityRecord{
		Kind: model.CapabilityKindSkill, Name: "alpha",
	}, bytes.NewReader([]byte("a"))); err != nil {
		t.Fatalf("Add: %v", err)
	}
	// Driver with a global discovery — must NOT appear in List.
	driver := &fakeDriver{
		name: "fake",
		globals: []model.GlobalCapability{
			{Runtime: "fake", Kind: model.CapabilityKindMCP, Name: "global-only"},
		},
	}
	reg := registryWith(t, driver)
	s := NewCapabilities(store, reg)
	recs, err := s.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(recs) != 1 || recs[0].Name != "alpha" {
		t.Fatalf("List should only see capstore records, got %+v", recs)
	}
}

func TestCapabilities_Get_NotFound(t *testing.T) {
	store := capstore.New(t.TempDir())
	s := NewCapabilities(store, runtime.NewRegistry())
	_, err := s.Get(context.Background(), model.CapabilityID("cap_missing"))
	if err == nil {
		t.Fatal("expected CAPABILITY_NOT_FOUND error")
	}
	se := AsError(err)
	if se == nil || se.Code != CodeCapabilityNotFound {
		t.Fatalf("expected CodeCapabilityNotFound, got %+v", err)
	}
	if got, _ := se.Details["id"].(string); got != "cap_missing" {
		t.Fatalf("expected details.id=cap_missing, got %v", se.Details)
	}
}

func TestCapabilities_Get_Success(t *testing.T) {
	store := capstore.New(t.TempDir())
	id, err := store.Add(model.CapabilityRecord{
		Kind: model.CapabilityKindSkill, Name: "alpha",
	}, bytes.NewReader([]byte("a")))
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	s := NewCapabilities(store, runtime.NewRegistry())
	got, err := s.Get(context.Background(), id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil || got.ID != id || got.Name != "alpha" {
		t.Fatalf("unexpected record: %+v", got)
	}
}

// asserts the unused error import is still used; goimports-style guard.
var _ = errors.New
