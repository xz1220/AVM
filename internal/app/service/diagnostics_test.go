package service

import (
	"context"
	"testing"

	"github.com/xz1220/agent-vm/internal/app/model"
	"github.com/xz1220/agent-vm/internal/infra/agentstore"
	"github.com/xz1220/agent-vm/internal/runtime"
)

func TestDiagnostics_Doctor_RuntimesProjected(t *testing.T) {
	t.Setenv("AVM_HOME", t.TempDir())
	driver := &fakeDriver{
		name: "fake",
		facts: runtime.Facts{
			Name:       "fake",
			Available:  true,
			BinaryPath: "/usr/bin/fake",
			Version:    "1.2.3",
			Risks: []runtime.Risk{
				{Code: "fake.risk", Message: "test risk"},
			},
		},
	}
	reg := registryWith(t, driver)
	d := NewDiagnostics(nil, reg, nil)
	rep, err := d.Doctor(context.Background())
	if err != nil {
		t.Fatalf("Doctor: %v", err)
	}
	if !rep.AVMHome.OK {
		t.Fatalf("AVMHome=%+v", rep.AVMHome)
	}
	if rep.ShellIntegration.OK {
		t.Fatalf("ShellIntegration should be false stub")
	}
	if len(rep.Runtimes) != 1 {
		t.Fatalf("Runtimes=%+v", rep.Runtimes)
	}
	rt := rep.Runtimes[0]
	if !rt.Available || rt.Binary != "/usr/bin/fake" || rt.Version != "1.2.3" {
		t.Fatalf("rt=%+v", rt)
	}
	if len(rt.Issues) != 1 {
		t.Fatalf("issues=%+v", rt.Issues)
	}
}

func TestDiagnostics_Runtimes_MatchesDoctor(t *testing.T) {
	t.Setenv("AVM_HOME", t.TempDir())
	driver := &fakeDriver{
		name: "fake",
		facts: runtime.Facts{
			Name:       "fake",
			Available:  true,
			BinaryPath: "/usr/bin/fake",
			Version:    "1.2.3",
			Risks:      []runtime.Risk{{Code: "fake.risk", Message: "test risk"}},
		},
	}
	d := NewDiagnostics(nil, registryWith(t, driver), nil)

	got, err := d.Runtimes(context.Background())
	if err != nil {
		t.Fatalf("Runtimes: %v", err)
	}
	if len(got) != 1 || got[0].Runtime != "fake" || !got[0].Available {
		t.Fatalf("Runtimes payload = %+v", got)
	}

	// Asserting Runtimes does not drift from Doctor.Runtimes, which is the
	// point of having a shared probeRuntimes helper.
	rep, err := d.Doctor(context.Background())
	if err != nil {
		t.Fatalf("Doctor: %v", err)
	}
	if len(rep.Runtimes) != len(got) {
		t.Fatalf("Doctor=%+v Runtimes=%+v", rep.Runtimes, got)
	}
	for i := range got {
		if got[i].Runtime != rep.Runtimes[i].Runtime ||
			got[i].Available != rep.Runtimes[i].Available ||
			got[i].Binary != rep.Runtimes[i].Binary ||
			got[i].Version != rep.Runtimes[i].Version {
			t.Fatalf("drift at %d: doctor=%+v runtimes=%+v",
				i, rep.Runtimes[i], got[i])
		}
	}
}

func TestDiagnostics_Status(t *testing.T) {
	t.Setenv("AVM_HOME", t.TempDir())
	repo := agentstore.New(t.TempDir())
	if err := repo.Save(&model.Agent{Identity: model.Identity{Name: "alpha"}}); err != nil {
		t.Fatalf("save: %v", err)
	}
	driver := &fakeDriver{
		name: "fake",
		facts: runtime.Facts{
			Name:      "fake",
			Available: true,
		},
	}
	log := &fakeLog{}
	_ = log.Append(model.RunRecord{Agent: "alpha"})
	_ = log.Append(model.RunRecord{Agent: "beta"})

	d := NewDiagnostics(repo, registryWith(t, driver), log)

	rep, err := d.Status(context.Background(), "")
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if len(rep.Agents) != 1 || rep.Agents[0].Name != "alpha" {
		t.Fatalf("agents=%+v", rep.Agents)
	}
	if len(rep.RecentRuns) != 2 {
		t.Fatalf("runs=%+v", rep.RecentRuns)
	}

	// Filtered to alpha.
	rep, err = d.Status(context.Background(), "alpha")
	if err != nil {
		t.Fatalf("Status alpha: %v", err)
	}
	if len(rep.Agents) != 1 || rep.Agents[0].Name != "alpha" {
		t.Fatalf("agents=%+v", rep.Agents)
	}
	if len(rep.RecentRuns) != 1 || rep.RecentRuns[0].Agent != "alpha" {
		t.Fatalf("recent=%+v", rep.RecentRuns)
	}
}
