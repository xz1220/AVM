package service

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/xz1220/agent-vm/internal/app/model"
	"github.com/xz1220/agent-vm/internal/infra/agentstore"
	"github.com/xz1220/agent-vm/internal/runtime"
)

// fakeDriver is a minimal runtime.Driver used by service tests. It does
// not call into any real binary.
type fakeDriver struct {
	name      string
	plan      *runtime.Plan
	planErr   error
	bnd       runtime.Boundary
	bndErr    error
	launch    runtime.LaunchSpec
	launchErr error
	facts     runtime.Facts
	factsErr  error
	globals   []model.GlobalCapability
	// exports keys "<kind>:<name>" to canned ExportGlobal payloads.
	// The string body is wrapped in io.NopCloser and returned verbatim.
	exports map[string]fakeExport
}

type fakeExport struct {
	format   string
	body     string
	filename string
	err      error
}

func (f *fakeDriver) Name() string { return f.name }
func (f *fakeDriver) Facts(ctx context.Context) (runtime.Facts, error) {
	return f.facts, f.factsErr
}
func (f *fakeDriver) DiscoverGlobal(ctx context.Context) ([]model.GlobalCapability, error) {
	return f.globals, nil
}
func (f *fakeDriver) ExportGlobal(ctx context.Context, kind model.CapabilityKind, name string) (runtime.Exported, error) {
	if f.exports == nil {
		return runtime.Exported{}, runtime.ErrGlobalCapabilityNotFound
	}
	e, ok := f.exports[string(kind)+":"+name]
	if !ok {
		return runtime.Exported{}, runtime.ErrGlobalCapabilityNotFound
	}
	if e.err != nil {
		return runtime.Exported{}, e.err
	}
	// Find the matching GlobalCapability (if any) so the result carries
	// the same Path the discovery surface advertised.
	var matched model.GlobalCapability
	for _, g := range f.globals {
		if g.Kind == kind && g.Name == name {
			matched = g
			break
		}
	}
	return runtime.Exported{
		Capability: matched,
		Format:     e.format,
		Content:    io.NopCloser(strings.NewReader(e.body)),
		Filename:   e.filename,
	}, nil
}
func (f *fakeDriver) Plan(ctx context.Context, _ *model.Agent) (*runtime.Plan, error) {
	return f.plan, f.planErr
}
func (f *fakeDriver) Boundary(ctx context.Context, _ *model.Agent) (runtime.Boundary, error) {
	return f.bnd, f.bndErr
}
func (f *fakeDriver) LaunchSpec(ctx context.Context, _ *model.Agent, _ *runtime.Plan) (runtime.LaunchSpec, error) {
	return f.launch, f.launchErr
}

func registryWith(t *testing.T, drivers ...runtime.Driver) runtime.Registry {
	t.Helper()
	r := runtime.NewRegistry()
	for _, d := range drivers {
		if err := r.Register(d); err != nil {
			t.Fatalf("register %s: %v", d.Name(), err)
		}
	}
	return r
}

func TestAgents_List_NilRepo(t *testing.T) {
	s := &Agents{}
	if _, err := s.List(context.Background()); err == nil {
		t.Fatal("expected error with nil repo")
	}
}

func TestAgents_Create_AndList(t *testing.T) {
	repo := agentstore.New(t.TempDir())
	s := NewAgents(repo, runtime.NewRegistry())
	_, err := s.Create(context.Background(), model.CreateAgentRequest{
		Name:        "alpha",
		Description: "first",
		Runtimes:    []model.RuntimePref{{Runtime: "fake"}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	got, err := s.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 || got[0].Name != "alpha" {
		t.Fatalf("List = %+v", got)
	}
}

func TestAgents_Create_ConflictAsk(t *testing.T) {
	repo := agentstore.New(t.TempDir())
	s := NewAgents(repo, runtime.NewRegistry())
	req := model.CreateAgentRequest{
		Name:     "alpha",
		Runtimes: []model.RuntimePref{{Runtime: "fake"}},
	}
	if _, err := s.Create(context.Background(), req); err != nil {
		t.Fatalf("Create #1: %v", err)
	}
	_, err := s.Create(context.Background(), req)
	if err == nil {
		t.Fatal("expected conflict error")
	}
	if !errors.Is(err, agentstore.ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}

func TestAgents_Create_RequiresRuntime(t *testing.T) {
	repo := agentstore.New(t.TempDir())
	s := NewAgents(repo, runtime.NewRegistry())
	_, err := s.Create(context.Background(), model.CreateAgentRequest{Name: "alpha"})
	if err == nil {
		t.Fatal("expected MISSING_INPUT for empty runtimes")
	}
	se, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *service.Error, got %T", err)
	}
	if se.Code != CodeMissingInput {
		t.Fatalf("code=%s want %s", se.Code, CodeMissingInput)
	}
	if got, _ := se.Details["field"].(string); got != "runtime" {
		t.Fatalf("details.field=%v want runtime", se.Details["field"])
	}
	if repo.Exists("alpha") {
		t.Fatal("agent should not be persisted on validation failure")
	}
}

func TestAgents_Create_Overwrite(t *testing.T) {
	repo := agentstore.New(t.TempDir())
	s := NewAgents(repo, runtime.NewRegistry())
	if _, err := s.Create(context.Background(), model.CreateAgentRequest{
		Name:        "alpha",
		Description: "v1",
		Runtimes:    []model.RuntimePref{{Runtime: "fake"}},
	}); err != nil {
		t.Fatalf("Create #1: %v", err)
	}
	a, err := s.Create(context.Background(), model.CreateAgentRequest{
		Name:        "alpha",
		Description: "v2",
		Runtimes:    []model.RuntimePref{{Runtime: "fake"}},
		OnConflict:  model.ResolveOverwrite,
	})
	if err != nil {
		t.Fatalf("Create overwrite: %v", err)
	}
	if a.Identity.Description != "v2" {
		t.Fatalf("description=%q want v2", a.Identity.Description)
	}
	got, err := repo.Get("alpha")
	if err != nil || got.Identity.Description != "v2" {
		t.Fatalf("on disk = %+v err=%v", got, err)
	}
}

func TestAgents_Create_InvalidName(t *testing.T) {
	repo := agentstore.New(t.TempDir())
	s := NewAgents(repo, runtime.NewRegistry())
	if _, err := s.Create(context.Background(), model.CreateAgentRequest{Name: "BAD NAME"}); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestAgents_Show_RuntimeMappingProjection(t *testing.T) {
	repo := agentstore.New(t.TempDir())
	driver := &fakeDriver{
		name: "fake",
		plan: &runtime.Plan{
			Mappings: []runtime.FieldMapping{
				{Field: "identity.name", Status: model.MappingNative},
				{Field: "skills", Status: model.MappingRenderedAsInstructions, Note: "fallback"},
			},
			Warnings: []model.Warning{{Code: "x", Message: "be careful"}},
		},
	}
	reg := registryWith(t, driver)
	s := NewAgents(repo, reg)
	if _, err := s.Create(context.Background(), model.CreateAgentRequest{
		Name:     "alpha",
		Runtimes: []model.RuntimePref{{Runtime: "fake"}},
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	detail, err := s.Show(context.Background(), "alpha")
	if err != nil {
		t.Fatalf("Show: %v", err)
	}
	if detail.SourcePath == "" {
		t.Fatal("expected non-empty SourcePath")
	}
	if len(detail.Mapping) != 1 || detail.Mapping[0].Runtime != "fake" {
		t.Fatalf("mapping=%+v", detail.Mapping)
	}
	if len(detail.Mapping[0].Fields) != 2 {
		t.Fatalf("fields=%+v", detail.Mapping[0].Fields)
	}
	if len(detail.Mapping[0].Warnings) != 1 || detail.Mapping[0].Warnings[0] != "be careful" {
		t.Fatalf("warnings=%+v", detail.Mapping[0].Warnings)
	}
}

func TestAgents_Show_DriverFailureBecomesWarning(t *testing.T) {
	repo := agentstore.New(t.TempDir())
	driver := &fakeDriver{name: "fake", planErr: errors.New("boom")}
	reg := registryWith(t, driver)
	s := NewAgents(repo, reg)
	if _, err := s.Create(context.Background(), model.CreateAgentRequest{
		Name:     "alpha",
		Runtimes: []model.RuntimePref{{Runtime: "fake"}},
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	detail, err := s.Show(context.Background(), "alpha")
	if err != nil {
		t.Fatalf("Show should not fail on driver error, got %v", err)
	}
	if len(detail.Mapping) != 1 || len(detail.Mapping[0].Warnings) == 0 {
		t.Fatalf("expected warning recorded, got %+v", detail.Mapping)
	}
}

func TestAgents_Edit(t *testing.T) {
	repo := agentstore.New(t.TempDir())
	s := NewAgents(repo, runtime.NewRegistry())
	if _, err := s.Create(context.Background(), model.CreateAgentRequest{
		Name:        "alpha",
		Description: "v1",
		Runtimes:    []model.RuntimePref{{Runtime: "fake"}},
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	desc := "v2"
	a, err := s.Edit(context.Background(), model.EditAgentRequest{
		Name:     "alpha",
		Identity: &model.Identity{Description: desc, Role: "tester"},
	})
	if err != nil {
		t.Fatalf("Edit: %v", err)
	}
	if a.Identity.Name != "alpha" || a.Identity.Description != desc || a.Identity.Role != "tester" {
		t.Fatalf("got %+v", a.Identity)
	}
}

func TestAgents_Edit_CannotDropAllRuntimes(t *testing.T) {
	repo := agentstore.New(t.TempDir())
	s := NewAgents(repo, runtime.NewRegistry())
	if _, err := s.Create(context.Background(), model.CreateAgentRequest{
		Name:     "alpha",
		Runtimes: []model.RuntimePref{{Runtime: "fake"}},
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	empty := []model.RuntimePref{}
	_, err := s.Edit(context.Background(), model.EditAgentRequest{
		Name:     "alpha",
		Runtimes: &empty,
	})
	if err == nil {
		t.Fatal("expected error when edit replaces runtimes with empty list")
	}
	se, ok := err.(*Error)
	if !ok || se.Code != CodeMissingInput {
		t.Fatalf("expected MISSING_INPUT, got %T %v", err, err)
	}
}

func TestAgents_Edit_NonexistentAgent(t *testing.T) {
	repo := agentstore.New(t.TempDir())
	s := NewAgents(repo, runtime.NewRegistry())
	_, err := s.Edit(context.Background(), model.EditAgentRequest{Name: "ghost"})
	if !errors.Is(err, agentstore.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestAgents_Delete_NonInteractiveRequiresConfirm(t *testing.T) {
	repo := agentstore.New(t.TempDir())
	s := NewAgents(repo, runtime.NewRegistry())
	if _, err := s.Create(context.Background(), model.CreateAgentRequest{
		Name:     "alpha",
		Runtimes: []model.RuntimePref{{Runtime: "fake"}},
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	err := s.Delete(context.Background(), model.DeleteAgentRequest{
		Name: "alpha",
	})
	if err == nil {
		t.Fatal("expected error without Confirm=true")
	}
	if err := s.Delete(context.Background(), model.DeleteAgentRequest{
		Name:    "alpha",
		Confirm: true,
	}); err != nil {
		t.Fatalf("Delete with confirm: %v", err)
	}
	if repo.Exists("alpha") {
		t.Fatal("agent still on disk after delete")
	}
}

func TestAgents_Clone(t *testing.T) {
	repo := agentstore.New(t.TempDir())
	s := NewAgents(repo, runtime.NewRegistry())
	if _, err := s.Create(context.Background(), model.CreateAgentRequest{
		Name:        "alpha",
		Description: "v1",
		Runtimes:    []model.RuntimePref{{Runtime: "fake"}},
	}); err != nil {
		t.Fatalf("create: %v", err)
	}
	clone, err := s.Clone(context.Background(), "alpha", "beta")
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}
	if clone.Identity.Name != "beta" || clone.Identity.Description != "v1" {
		t.Fatalf("clone = %+v", clone.Identity)
	}
	// Cloning to existing name fails.
	if _, err := s.Clone(context.Background(), "alpha", "beta"); !errors.Is(err, agentstore.ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}

func TestAgents_Rename(t *testing.T) {
	repo := agentstore.New(t.TempDir())
	s := NewAgents(repo, runtime.NewRegistry())
	if _, err := s.Create(context.Background(), model.CreateAgentRequest{
		Name:     "alpha",
		Runtimes: []model.RuntimePref{{Runtime: "fake"}},
	}); err != nil {
		t.Fatalf("create alpha: %v", err)
	}
	if _, err := s.Create(context.Background(), model.CreateAgentRequest{
		Name:     "beta",
		Runtimes: []model.RuntimePref{{Runtime: "fake"}},
	}); err != nil {
		t.Fatalf("create beta: %v", err)
	}
	// Renaming to existing fails without touching old.
	if _, err := s.Rename(context.Background(), "alpha", "beta"); !errors.Is(err, agentstore.ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
	if !repo.Exists("alpha") {
		t.Fatal("alpha was deleted on conflict")
	}
	// Renaming to fresh name succeeds.
	if _, err := s.Rename(context.Background(), "alpha", "gamma"); err != nil {
		t.Fatalf("Rename: %v", err)
	}
	if repo.Exists("alpha") || !repo.Exists("gamma") {
		t.Fatalf("post-rename state: alpha=%v gamma=%v", repo.Exists("alpha"), repo.Exists("gamma"))
	}
}
