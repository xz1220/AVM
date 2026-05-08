package cli

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/xz1220/agent-vm/internal/app/model"
	"github.com/xz1220/agent-vm/internal/app/service"
)

func TestCapabilityDiscover_Empty(t *testing.T) {
	deps := newTestDeps(nil, nil, nil, &fakeCaps{}, nil)
	out, _, err := runCmd(t, deps, "capability", "discover")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, "(no capabilities discovered)") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestCapabilityDiscover_Human(t *testing.T) {
	caps := &fakeCaps{cands: []model.CapabilityCandidate{
		{
			Kind:   model.CapabilityKindSkill,
			Name:   "alpha",
			Source: model.SourceAVM,
			Record: &model.CapabilityRecord{ID: "cap_demo"},
		},
		{
			Kind:     model.CapabilityKindSkill,
			Name:     "alpha",
			Source:   model.SourceRuntimeGlobal,
			Global:   &model.GlobalCapability{Path: "/home/x/.codex/skills/alpha"},
			Imported: true,
			Conflict: false,
		},
	}}
	deps := newTestDeps(nil, nil, nil, caps, nil)
	out, _, err := runCmd(t, deps, "capability", "discover")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	for _, want := range []string{"alpha", "cap_demo", "imported", "/home/x/.codex/skills/alpha"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output: %s", want, out)
		}
	}
}

func TestCapabilityDiscover_JSON(t *testing.T) {
	caps := &fakeCaps{cands: []model.CapabilityCandidate{
		{Kind: model.CapabilityKindMCP, Name: "gh", Source: model.SourceAVM,
			Record: &model.CapabilityRecord{ID: "cap_x"}},
	}}
	deps := newTestDeps(nil, nil, nil, caps, nil)
	out, _, err := runCmd(t, deps, "--json", "capability", "discover")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	var got []model.CapabilityCandidate
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(got) != 1 || got[0].Name != "gh" {
		t.Fatalf("unexpected JSON: %+v", got)
	}
}

func TestCapabilityImport_Success(t *testing.T) {
	caps := &fakeCaps{}
	deps := newTestDeps(nil, nil, nil, caps, nil)
	out, _, err := runCmd(t, deps,
		"capability", "import",
		"--runtime", "codex",
		"--kind", "skill",
		"--name", "hello",
	)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, "imported cap_fake_hello") {
		t.Fatalf("unexpected output: %s", out)
	}
	if len(caps.importReqs) != 1 ||
		caps.importReqs[0].Runtime != "codex" ||
		caps.importReqs[0].Name != "hello" {
		t.Fatalf("unexpected import request: %+v", caps.importReqs)
	}
}

func TestCapabilityImport_JSONReplaced(t *testing.T) {
	caps := &fakeCaps{
		importRes: &model.ImportCapabilityResult{
			ID:       model.CapabilityID("cap_new"),
			Created:  true,
			Replaced: true,
			Source:   "codex:/home/x/.codex/skills/hello",
		},
	}
	deps := newTestDeps(nil, nil, nil, caps, nil)
	out, _, err := runCmd(t, deps, "--json",
		"capability", "import",
		"--runtime", "codex", "--kind", "skill", "--name", "hello",
		"--on-conflict", "overwrite",
	)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	var got model.ImportCapabilityResult
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if got.ID != "cap_new" || !got.Replaced {
		t.Fatalf("unexpected result: %+v", got)
	}
	if caps.importReqs[0].OnConflict != model.ResolveOverwrite {
		t.Fatalf("OnConflict not propagated: %+v", caps.importReqs[0])
	}
}

func TestCapabilityImport_JSONEmitsCreatedFalse(t *testing.T) {
	caps := &fakeCaps{
		importRes: &model.ImportCapabilityResult{
			ID:      model.CapabilityID("cap_existing"),
			Created: false,
			Source:  "codex:/home/x/.codex/skills/hello",
		},
	}
	deps := newTestDeps(nil, nil, nil, caps, nil)
	out, _, err := runCmd(t, deps, "--json",
		"capability", "import",
		"--runtime", "codex", "--kind", "skill", "--name", "hello",
	)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	got, ok := raw["created"].(bool)
	if !ok || got {
		t.Fatalf("expected created=false to be emitted, got %v in %s", raw["created"], out)
	}
}

func TestCapabilityImport_ConflictEnvelope(t *testing.T) {
	caps := &fakeCaps{
		importErr: service.NewError(service.CodeCapabilityConflict,
			"already imported with different content",
			map[string]any{
				"kind":              "skill",
				"name":              "hello",
				"existing_id":       "cap_old",
				"existing_checksum": "deadbeef",
			}),
	}
	deps := newTestDeps(nil, nil, nil, caps, nil)
	out, _, err := runCmd(t, deps, "--json",
		"capability", "import",
		"--runtime", "codex", "--kind", "skill", "--name", "hello",
	)
	if err == nil {
		t.Fatal("expected error")
	}
	var env struct {
		Error *service.Error `json:"error"`
	}
	if jerr := json.Unmarshal([]byte(out), &env); jerr != nil {
		t.Fatalf("invalid envelope: %v\n%s", jerr, out)
	}
	if env.Error == nil || env.Error.Code != service.CodeCapabilityConflict {
		t.Fatalf("expected CAPABILITY_CONFLICT envelope, got %+v", env.Error)
	}
	if got, _ := env.Error.Details["name"].(string); got != "hello" {
		t.Fatalf("expected details.name=hello, got %v", env.Error.Details)
	}
}

func TestCapabilityBootstrap_HumanSummary(t *testing.T) {
	caps := &fakeCaps{
		bootRes: &model.BootstrapCapabilitiesResult{
			Imported: []model.ImportCapabilityResult{
				{ID: "cap_a", Created: true, Source: "codex:/p/a"},
				{ID: "cap_b", Created: true, Source: "codex:/p/b"},
			},
			Skipped: []model.SkippedCapability{
				{Kind: model.CapabilityKindSkill, Name: "boom", Reason: "export failed"},
			},
		},
	}
	deps := newTestDeps(nil, nil, nil, caps, nil)
	out, _, err := runCmd(t, deps,
		"capability", "bootstrap",
		"--runtime", "codex",
	)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	for _, want := range []string{"Imported 2 capabilities", "cap_a", "cap_b", "Skipped 1", "boom", "export failed"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output: %s", want, out)
		}
	}
}

func TestCapabilityBootstrap_PropagatesFlags(t *testing.T) {
	caps := &fakeCaps{}
	deps := newTestDeps(nil, nil, nil, caps, nil)
	_, _, err := runCmd(t, deps,
		"capability", "bootstrap",
		"--runtime", "codex",
		"--kind", "skill",
		"--on-conflict", "skip",
	)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if len(caps.bootReqs) != 1 {
		t.Fatalf("expected 1 bootstrap call, got %d", len(caps.bootReqs))
	}
	got := caps.bootReqs[0]
	if got.Runtime != "codex" || got.OnConflict != model.ResolveSkip {
		t.Fatalf("unexpected request: %+v", got)
	}
	if len(got.Kinds) != 1 || got.Kinds[0] != model.CapabilityKindSkill {
		t.Fatalf("expected kind=skill, got %+v", got.Kinds)
	}
}

func TestCapabilityList_Empty(t *testing.T) {
	deps := newTestDeps(nil, nil, nil, &fakeCaps{}, nil)
	out, _, err := runCmd(t, deps, "capability", "list")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, "(capability store empty)") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestCapabilityList_JSON(t *testing.T) {
	caps := &fakeCaps{records: []model.CapabilityRecord{
		{ID: "cap_a", Kind: model.CapabilityKindSkill, Name: "alpha", Source: model.SourceAVM},
		{ID: "cap_b", Kind: model.CapabilityKindMCP, Name: "gh", Source: model.SourcePackage,
			Version: "1.2", ImportFrom: "pkg-x"},
	}}
	deps := newTestDeps(nil, nil, nil, caps, nil)
	out, _, err := runCmd(t, deps, "--json", "capability", "list")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	var got []model.CapabilityRecord
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if len(got) != 2 || got[0].ID != "cap_a" || got[1].ID != "cap_b" {
		t.Fatalf("unexpected records: %+v", got)
	}
}

func TestCapabilityShow_Found(t *testing.T) {
	caps := &fakeCaps{records: []model.CapabilityRecord{
		{ID: "cap_a", Kind: model.CapabilityKindSkill, Name: "alpha",
			Source: model.SourceAVM, Version: "v1", Checksum: "deadbeef"},
	}}
	deps := newTestDeps(nil, nil, nil, caps, nil)
	out, _, err := runCmd(t, deps, "capability", "show", "cap_a")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	for _, want := range []string{"cap_a", "alpha", "skill", "v1", "deadbeef"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in output: %s", want, out)
		}
	}
}

func TestCapabilityShow_NotFound_Envelope(t *testing.T) {
	deps := newTestDeps(nil, nil, nil, &fakeCaps{}, nil)
	out, _, err := runCmd(t, deps, "--json", "capability", "show", "cap_missing")
	if err == nil {
		t.Fatal("expected CAPABILITY_NOT_FOUND error")
	}
	var env struct {
		Error *service.Error `json:"error"`
	}
	if jerr := json.Unmarshal([]byte(out), &env); jerr != nil {
		t.Fatalf("invalid envelope: %v\n%s", jerr, out)
	}
	if env.Error == nil || env.Error.Code != service.CodeCapabilityNotFound {
		t.Fatalf("expected CAPABILITY_NOT_FOUND envelope, got %+v", env.Error)
	}
	if got, _ := env.Error.Details["id"].(string); got != "cap_missing" {
		t.Fatalf("expected details.id=cap_missing, got %v", env.Error.Details)
	}
}

// guard against the import being lost during edits
var _ = errors.New
