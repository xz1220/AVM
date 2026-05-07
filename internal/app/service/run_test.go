package service

import (
	"context"
	"errors"
	"testing"

	"github.com/xz1220/agent-vm/internal/app/model"
	"github.com/xz1220/agent-vm/internal/infra/agentstore"
	"github.com/xz1220/agent-vm/internal/infra/process"
	"github.com/xz1220/agent-vm/internal/runtime"
)

// fakeWriter is a stand-in for managedfile.Writer that records calls.
type fakeWriter struct {
	dryRunResult []model.DiffEntry
	dryRunErr    error
	applyErr     error
	applyCount   int
	dryRunCount  int
}

func (f *fakeWriter) DryRun(ctx context.Context, files []runtime.ManagedFile) ([]model.DiffEntry, error) {
	f.dryRunCount++
	return f.dryRunResult, f.dryRunErr
}

func (f *fakeWriter) Apply(ctx context.Context, files []runtime.ManagedFile) ([]model.DiffEntry, error) {
	f.applyCount++
	return nil, f.applyErr
}

// fakeProcess is a stand-in for process.Runner. It does not exec.
type fakeProcess struct {
	exitCode int
	err      error
	called   bool
	gotSpec  runtime.LaunchSpec
}

func (f *fakeProcess) Run(ctx context.Context, spec runtime.LaunchSpec) (process.Result, error) {
	f.called = true
	f.gotSpec = spec
	return process.Result{ExitCode: f.exitCode}, f.err
}

// fakeLog is a stand-in for runlog.Log.
type fakeLog struct {
	records []model.RunRecord
}

func (l *fakeLog) Append(rec model.RunRecord) error {
	l.records = append(l.records, rec)
	return nil
}
func (l *fakeLog) List(limit int) ([]model.RunRecord, error) {
	if limit == 0 || limit >= len(l.records) {
		return l.records, nil
	}
	return l.records[len(l.records)-limit:], nil
}

func defaultPlan() *runtime.Plan {
	return &runtime.Plan{
		Files: []runtime.ManagedFile{
			{Path: "/tmp/avm-test/managedfile", Contents: []byte("x")},
		},
		Mappings: []runtime.FieldMapping{
			{Field: "identity.name", Status: model.MappingNative},
		},
	}
}

func mkAgentInRepo(t *testing.T, repo agentstore.Repository, name string, runtimes []model.RuntimePref) {
	t.Helper()
	a := &model.Agent{
		Identity: model.Identity{Name: name},
		Runtimes: runtimes,
	}
	if err := repo.Save(a); err != nil {
		t.Fatalf("save agent: %v", err)
	}
}

func TestRunner_Preview_HappyPath(t *testing.T) {
	repo := agentstore.New(t.TempDir())
	mkAgentInRepo(t, repo, "alpha", []model.RuntimePref{{Runtime: "fake"}})
	driver := &fakeDriver{
		name: "fake",
		plan: defaultPlan(),
		bnd:  runtime.Boundary{StateDir: "/tmp/avm-state", Env: map[string]string{"FOO_HOME": "/tmp/avm-state"}},
	}
	w := &fakeWriter{dryRunResult: []model.DiffEntry{{Path: "/tmp/avm-test/managedfile", Reason: "created"}}}
	r := NewRunner(repo, registryWith(t, driver), w, &fakeProcess{}, &fakeLog{})
	pv, err := r.Preview(context.Background(), model.RunRequest{Agent: "alpha"})
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if pv.Runtime != "fake" {
		t.Fatalf("runtime=%q", pv.Runtime)
	}
	if len(pv.WritePaths) != 1 {
		t.Fatalf("WritePaths=%+v", pv.WritePaths)
	}
	if pv.Boundary.StateDir != "/tmp/avm-state" || len(pv.Boundary.EnvKeys) != 1 || pv.Boundary.EnvKeys[0] != "FOO_HOME" {
		t.Fatalf("boundary=%+v", pv.Boundary)
	}
	if len(pv.Drift) != 1 {
		t.Fatalf("drift=%+v", pv.Drift)
	}
	if w.dryRunCount != 1 || w.applyCount != 0 {
		t.Fatalf("preview should DryRun only: dryrun=%d apply=%d", w.dryRunCount, w.applyCount)
	}
}

func TestRunner_Preview_RuntimeUnknown(t *testing.T) {
	repo := agentstore.New(t.TempDir())
	mkAgentInRepo(t, repo, "alpha", []model.RuntimePref{{Runtime: "missing"}})
	r := NewRunner(repo, runtime.NewRegistry(), &fakeWriter{}, &fakeProcess{}, &fakeLog{})
	if _, err := r.Preview(context.Background(), model.RunRequest{Agent: "alpha"}); err == nil {
		t.Fatal("expected error for missing runtime")
	}
}

func TestRunner_ResolveRuntime_MultipleRequiresFlag(t *testing.T) {
	repo := agentstore.New(t.TempDir())
	mkAgentInRepo(t, repo, "alpha", []model.RuntimePref{
		{Runtime: "a"}, {Runtime: "b"},
	})
	driver := &fakeDriver{name: "a"}
	r := NewRunner(repo, registryWith(t, driver), &fakeWriter{}, &fakeProcess{}, &fakeLog{})
	_, err := r.Preview(context.Background(), model.RunRequest{Agent: "alpha", NonInteractive: true})
	if err == nil {
		t.Fatal("expected error for multi-runtime non-interactive")
	}
}

func TestRunner_ResolveRuntime_DefaultPicksOne(t *testing.T) {
	repo := agentstore.New(t.TempDir())
	mkAgentInRepo(t, repo, "alpha", []model.RuntimePref{
		{Runtime: "a"}, {Runtime: "b", Default: true},
	})
	driver := &fakeDriver{
		name: "b",
		plan: &runtime.Plan{},
	}
	// Note: only "b" registered — selecting it via Default should work
	// since we don't need "a".
	r := NewRunner(repo, registryWith(t, driver), &fakeWriter{}, &fakeProcess{}, &fakeLog{})
	pv, err := r.Preview(context.Background(), model.RunRequest{Agent: "alpha"})
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if pv.Runtime != "b" {
		t.Fatalf("runtime=%q want b", pv.Runtime)
	}
}

func TestRunner_ResolveRuntime_ExplicitOverride(t *testing.T) {
	repo := agentstore.New(t.TempDir())
	mkAgentInRepo(t, repo, "alpha", nil)
	driver := &fakeDriver{name: "explicit", plan: &runtime.Plan{}}
	r := NewRunner(repo, registryWith(t, driver), &fakeWriter{}, &fakeProcess{}, &fakeLog{})
	pv, err := r.Preview(context.Background(), model.RunRequest{Agent: "alpha", Runtime: "explicit"})
	if err != nil {
		t.Fatalf("Preview: %v", err)
	}
	if pv.Runtime != "explicit" {
		t.Fatalf("runtime=%q", pv.Runtime)
	}
}

func TestRunner_ResolveRuntime_NoRuntimes(t *testing.T) {
	repo := agentstore.New(t.TempDir())
	mkAgentInRepo(t, repo, "alpha", nil)
	r := NewRunner(repo, runtime.NewRegistry(), &fakeWriter{}, &fakeProcess{}, &fakeLog{})
	if _, err := r.Preview(context.Background(), model.RunRequest{Agent: "alpha"}); err == nil {
		t.Fatal("expected error for agent with no runtimes")
	}
}

func TestRunner_Run_AppliesAndLogs(t *testing.T) {
	repo := agentstore.New(t.TempDir())
	mkAgentInRepo(t, repo, "alpha", []model.RuntimePref{{Runtime: "fake"}})
	driver := &fakeDriver{
		name: "fake",
		plan: defaultPlan(),
		bnd:  runtime.Boundary{StateDir: "/tmp/avm-state"},
		launch: runtime.LaunchSpec{
			Bin: "/usr/bin/true",
		},
	}
	w := &fakeWriter{}
	p := &fakeProcess{exitCode: 0}
	log := &fakeLog{}
	r := NewRunner(repo, registryWith(t, driver), w, p, log)
	res, err := r.Run(context.Background(), model.RunRequest{Agent: "alpha"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !p.called {
		t.Fatal("process not called")
	}
	if w.applyCount != 1 {
		t.Fatalf("Apply called %d times", w.applyCount)
	}
	if len(log.records) != 1 || log.records[0].Agent != "alpha" {
		t.Fatalf("log=%+v", log.records)
	}
	if res.ExitCode != 0 {
		t.Fatalf("exit=%d", res.ExitCode)
	}
}

func TestRunner_Run_DriftKeepsAndApplies(t *testing.T) {
	repo := agentstore.New(t.TempDir())
	mkAgentInRepo(t, repo, "alpha", []model.RuntimePref{{Runtime: "fake"}})
	driver := &fakeDriver{
		name:   "fake",
		plan:   defaultPlan(),
		bnd:    runtime.Boundary{StateDir: "/tmp/avm-state"},
		launch: runtime.LaunchSpec{Bin: "/usr/bin/true"},
	}
	w := &fakeWriter{
		dryRunResult: []model.DiffEntry{{Path: "/tmp/avm-test/managedfile", Reason: "updated"}},
	}
	log := &fakeLog{}
	r := NewRunner(repo, registryWith(t, driver), w, &fakeProcess{}, log)
	_, err := r.Run(context.Background(), model.RunRequest{
		Agent:          "alpha",
		NonInteractive: true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(log.records) != 1 || len(log.records[0].Drift) != 1 {
		t.Fatalf("drift not recorded: %+v", log.records)
	}
}

func TestRunner_Run_ProcessErrorPropagates(t *testing.T) {
	repo := agentstore.New(t.TempDir())
	mkAgentInRepo(t, repo, "alpha", []model.RuntimePref{{Runtime: "fake"}})
	driver := &fakeDriver{
		name:   "fake",
		plan:   defaultPlan(),
		bnd:    runtime.Boundary{},
		launch: runtime.LaunchSpec{Bin: "/usr/bin/true"},
	}
	p := &fakeProcess{err: errors.New("boom")}
	log := &fakeLog{}
	r := NewRunner(repo, registryWith(t, driver), &fakeWriter{}, p, log)
	_, err := r.Run(context.Background(), model.RunRequest{Agent: "alpha"})
	if err == nil {
		t.Fatal("expected process error")
	}
	if len(log.records) != 1 {
		t.Fatalf("expected run log entry, got %+v", log.records)
	}
}
