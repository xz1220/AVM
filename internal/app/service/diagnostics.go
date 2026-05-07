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
}

// Diagnostics is the default DiagnosticsService.
type Diagnostics struct {
	Agents   agentstore.Repository
	Runtimes runtime.Registry
	Log      runlog.Log
}

func NewDiagnostics(agents agentstore.Repository, registry runtime.Registry, log runlog.Log) *Diagnostics {
	return &Diagnostics{Agents: agents, Runtimes: registry, Log: log}
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

	// Per-runtime probing.
	if s.Runtimes != nil {
		for _, info := range s.Runtimes.List() {
			drv, err := s.Runtimes.Resolve(info.Name)
			if err != nil {
				report.Runtimes = append(report.Runtimes, model.RuntimeCheck{
					Runtime: info.Name,
					Issues:  []string{err.Error()},
				})
				continue
			}
			facts, err := drv.Facts(ctx)
			if err != nil {
				report.Runtimes = append(report.Runtimes, model.RuntimeCheck{
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
			report.Runtimes = append(report.Runtimes, rc)
		}
	}
	return report, nil
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
	if s.Runtimes != nil {
		for _, info := range s.Runtimes.List() {
			drv, err := s.Runtimes.Resolve(info.Name)
			if err != nil {
				report.Runtimes = append(report.Runtimes, model.RuntimeCheck{
					Runtime: info.Name,
					Issues:  []string{err.Error()},
				})
				continue
			}
			facts, err := drv.Facts(ctx)
			if err != nil {
				report.Runtimes = append(report.Runtimes, model.RuntimeCheck{
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
			report.Runtimes = append(report.Runtimes, rc)
		}
	}
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
