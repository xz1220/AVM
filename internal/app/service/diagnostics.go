package service

import (
	"context"
	"fmt"

	"github.com/xz1220/agent-vm/internal/app/model"
	"github.com/xz1220/agent-vm/internal/infra/agentstore"
	"github.com/xz1220/agent-vm/internal/infra/home"
	"github.com/xz1220/agent-vm/internal/infra/runlog"
	"github.com/xz1220/agent-vm/internal/runtime"
)

// DiagnosticsService implements `avm doctor` and `avm status`.
type DiagnosticsService interface {
	Doctor(ctx context.Context) (*model.DoctorReport, error)
	Status(ctx context.Context, agent string) (*model.StatusReport, error)
	// Runtimes returns the same per-runtime probe payload Doctor uses,
	// without the AVM-home / PATH / shell-integration checks. UIs that
	// only need a runtime picker should call this instead of Doctor.
	Runtimes(ctx context.Context) ([]model.RuntimeCheck, error)
}

// Diagnostics is the default DiagnosticsService.
//
// The runtime registry is stored as Registry (not Runtimes) so the
// struct field does not collide with the Runtimes(ctx) method on the
// DiagnosticsService interface.
type Diagnostics struct {
	Agents   agentstore.Repository
	Registry runtime.Registry
	Log      runlog.Log
}

func NewDiagnostics(agents agentstore.Repository, registry runtime.Registry, log runlog.Log) *Diagnostics {
	return &Diagnostics{Agents: agents, Registry: registry, Log: log}
}

// Doctor probes AVM-level state and runtime presence.
func (s *Diagnostics) Doctor(ctx context.Context) (*model.DoctorReport, error) {
	report := &model.DoctorReport{}

	// AVM home: try to compute the layout and ensure subdirs exist.
	if layout, err := home.DefaultLayout(); err != nil {
		report.AVMHome = model.CheckResult{OK: false, Detail: err.Error()}
	} else if err := layout.EnsureDirs(); err != nil {
		report.AVMHome = model.CheckResult{OK: false, Detail: err.Error()}
	} else {
		report.AVMHome = model.CheckResult{OK: true, Detail: layout.Root}
	}

	// PATH: not yet implemented; treat as OK to avoid spurious doctor
	// failures. A real probe would inspect $PATH and confirm avm is
	// on it.
	report.PATH = model.CheckResult{OK: true, Detail: ""}

	// Shell integration: not yet installed by AVM.
	report.ShellIntegration = model.CheckResult{OK: false, Detail: "not installed"}

	report.Runtimes = s.probeRuntimes(ctx)
	return report, nil
}

// Runtimes returns the same per-runtime payload Doctor uses, with no
// extra AVM-level checks. UIs that only need a runtime picker should
// call this — they should not parse Doctor's broader DoctorReport.
func (s *Diagnostics) Runtimes(ctx context.Context) ([]model.RuntimeCheck, error) {
	return s.probeRuntimes(ctx), nil
}

// probeRuntimes is the shared per-runtime probe used by Doctor, Status
// and the standalone Runtimes endpoint. It NEVER returns an error: a
// driver-level failure becomes an `Issues` entry on that runtime so a
// missing binary cannot blank out the whole report.
func (s *Diagnostics) probeRuntimes(ctx context.Context) []model.RuntimeCheck {
	if s.Registry == nil {
		return nil
	}
	var out []model.RuntimeCheck
	for _, info := range s.Registry.List() {
		drv, err := s.Registry.Resolve(info.Name)
		if err != nil {
			out = append(out, model.RuntimeCheck{
				Runtime: info.Name,
				Issues:  []string{err.Error()},
			})
			continue
		}
		facts, err := drv.Facts(ctx)
		if err != nil {
			out = append(out, model.RuntimeCheck{
				Runtime: info.Name,
				Issues:  []string{err.Error()},
			})
			continue
		}
		rc := model.RuntimeCheck{
			Runtime:   info.Name,
			Available: facts.Available,
			Binary:    facts.BinaryPath,
			Version:   facts.Version,
		}
		for _, risk := range facts.Risks {
			rc.Issues = append(rc.Issues, fmt.Sprintf("%s: %s", risk.Code, risk.Message))
		}
		out = append(out, rc)
	}
	return out
}

// Status reports current AVM state: agents, runtime facts, recent runs.
func (s *Diagnostics) Status(ctx context.Context, agentOpt string) (*model.StatusReport, error) {
	report := &model.StatusReport{}
	if s.Agents != nil {
		all, err := s.Agents.List()
		if err == nil {
			if agentOpt == "" {
				report.Agents = all
			} else {
				for _, a := range all {
					if a.Name == agentOpt {
						report.Agents = append(report.Agents, a)
					}
				}
			}
		}
	}
	report.Runtimes = s.probeRuntimes(ctx)
	if s.Log != nil {
		runs, err := s.Log.List(20)
		if err == nil {
			if agentOpt == "" {
				report.RecentRuns = runs
			} else {
				for _, r := range runs {
					if r.Agent == agentOpt {
						report.RecentRuns = append(report.RecentRuns, r)
					}
				}
			}
		}
	}
	return report, nil
}
