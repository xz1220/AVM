package cli

import (
	"context"
	"errors"

	"github.com/xz1220/agent-vm/internal/app/model"
	"github.com/xz1220/agent-vm/internal/infra/agentstore"
)

// fakeAgents is a test-only AgentService implementing the methods the
// CLI calls against the service container.
type fakeAgents struct {
	agents     map[string]*model.AgentDetail
	createErr  error
	listErr    error
	deleted    []string
	editCalls  []model.EditAgentRequest
	createReqs []model.CreateAgentRequest
}

func newFakeAgents() *fakeAgents {
	return &fakeAgents{agents: map[string]*model.AgentDetail{}}
}

func (f *fakeAgents) put(a model.Agent) {
	f.agents[a.Identity.Name] = &model.AgentDetail{Agent: a, SourcePath: "/tmp/" + a.Identity.Name + ".yaml"}
}

func (f *fakeAgents) Create(ctx context.Context, req model.CreateAgentRequest) (*model.Agent, error) {
	f.createReqs = append(f.createReqs, req)
	if f.createErr != nil {
		return nil, f.createErr
	}
	if _, exists := f.agents[req.Name]; exists {
		switch req.OnConflict {
		case model.ResolveOverwrite:
			// fall through
		default:
			return nil, agentstore.ErrConflict
		}
	}
	a := &model.Agent{
		Identity:     model.Identity{Name: req.Name, Description: req.Description, Role: req.Role},
		Instructions: req.Instructions,
		Skills:       req.Skills,
		MCP:          req.MCP,
		Runtimes:     req.Runtimes,
	}
	f.put(*a)
	return a, nil
}

func (f *fakeAgents) List(ctx context.Context) ([]model.AgentSummary, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := make([]model.AgentSummary, 0, len(f.agents))
	for _, d := range f.agents {
		rts := make([]string, 0, len(d.Agent.Runtimes))
		for _, r := range d.Agent.Runtimes {
			rts = append(rts, r.Runtime)
		}
		out = append(out, model.AgentSummary{
			Name:        d.Agent.Identity.Name,
			Description: d.Agent.Identity.Description,
			Runtimes:    rts,
		})
	}
	return out, nil
}

func (f *fakeAgents) Show(ctx context.Context, name string) (*model.AgentDetail, error) {
	d, ok := f.agents[name]
	if !ok {
		return nil, agentstore.ErrNotFound
	}
	return d, nil
}

func (f *fakeAgents) Edit(ctx context.Context, req model.EditAgentRequest) (*model.Agent, error) {
	f.editCalls = append(f.editCalls, req)
	d, ok := f.agents[req.Name]
	if !ok {
		return nil, agentstore.ErrNotFound
	}
	if req.Identity != nil {
		ident := *req.Identity
		ident.Name = d.Agent.Identity.Name
		d.Agent.Identity = ident
	}
	if req.Instructions != nil {
		d.Agent.Instructions = *req.Instructions
	}
	if req.Skills != nil {
		d.Agent.Skills = *req.Skills
	}
	if req.MCP != nil {
		d.Agent.MCP = *req.MCP
	}
	if req.Runtimes != nil {
		d.Agent.Runtimes = *req.Runtimes
	}
	return &d.Agent, nil
}

func (f *fakeAgents) Delete(ctx context.Context, req model.DeleteAgentRequest) error {
	if req.NonInteractive && !req.Confirm {
		return errors.New("confirm required")
	}
	if _, ok := f.agents[req.Name]; !ok {
		return agentstore.ErrNotFound
	}
	delete(f.agents, req.Name)
	f.deleted = append(f.deleted, req.Name)
	return nil
}

func (f *fakeAgents) Clone(ctx context.Context, name, newName string) (*model.Agent, error) {
	d, ok := f.agents[name]
	if !ok {
		return nil, agentstore.ErrNotFound
	}
	if _, exists := f.agents[newName]; exists {
		return nil, agentstore.ErrConflict
	}
	clone := d.Agent
	clone.Identity.Name = newName
	f.put(clone)
	return &clone, nil
}

func (f *fakeAgents) Rename(ctx context.Context, oldName, newName string) (*model.Agent, error) {
	d, ok := f.agents[oldName]
	if !ok {
		return nil, agentstore.ErrNotFound
	}
	if _, exists := f.agents[newName]; exists {
		return nil, agentstore.ErrConflict
	}
	clone := d.Agent
	clone.Identity.Name = newName
	delete(f.agents, oldName)
	f.put(clone)
	return &clone, nil
}

// fakeRunner is a test-only RunService.
type fakeRunner struct {
	preview      *model.RunPreview
	previewErr   error
	previewErrs  []error // queue, popped per call (overrides previewErr)
	previewCount int
	result       *model.RunResult
	runErr       error
	lastReq      model.RunRequest
}

func (f *fakeRunner) Preview(ctx context.Context, req model.RunRequest) (*model.RunPreview, error) {
	f.lastReq = req
	if f.previewCount < len(f.previewErrs) {
		err := f.previewErrs[f.previewCount]
		f.previewCount++
		if err != nil {
			return nil, err
		}
	} else if f.previewErr != nil {
		return nil, f.previewErr
	}
	if f.preview != nil {
		return f.preview, nil
	}
	return &model.RunPreview{Agent: req.Agent, Runtime: req.Runtime}, nil
}

func (f *fakeRunner) Run(ctx context.Context, req model.RunRequest) (*model.RunResult, error) {
	f.lastReq = req
	if f.runErr != nil {
		return nil, f.runErr
	}
	if f.result != nil {
		return f.result, nil
	}
	pv := &model.RunPreview{Agent: req.Agent, Runtime: req.Runtime}
	return &model.RunResult{Preview: *pv}, nil
}

// fakePackages is a test-only PackageService.
type fakePackages struct {
	listResp    []model.PackageSummary
	listErr     error
	showErr     error
	inspectResp *model.PackageDetail
	inspectErr  error
	installRes  *model.InstallResult
	installErr  error
	installReqs []model.InstallRequest
	uninstalled []string
	exportPath  string
	exportErr   error
	exportCalls []model.ExportRequest
}

func (f *fakePackages) List(ctx context.Context) ([]model.PackageSummary, error) {
	return f.listResp, f.listErr
}
func (f *fakePackages) Show(ctx context.Context, name string) (*model.PackageDetail, error) {
	if f.showErr != nil {
		return nil, f.showErr
	}
	return nil, nil
}
func (f *fakePackages) Install(ctx context.Context, req model.InstallRequest) (*model.InstallResult, error) {
	f.installReqs = append(f.installReqs, req)
	if f.installErr != nil {
		return nil, f.installErr
	}
	return f.installRes, nil
}
func (f *fakePackages) Uninstall(ctx context.Context, name string) error {
	f.uninstalled = append(f.uninstalled, name)
	return nil
}
func (f *fakePackages) Export(ctx context.Context, req model.ExportRequest) (*model.ExportResult, error) {
	f.exportCalls = append(f.exportCalls, req)
	if f.exportErr != nil {
		return nil, f.exportErr
	}
	return &model.ExportResult{Path: f.exportPath}, nil
}
func (f *fakePackages) Inspect(ctx context.Context, file string) (*model.PackageDetail, error) {
	if f.inspectErr != nil {
		return nil, f.inspectErr
	}
	if f.inspectResp != nil {
		return f.inspectResp, nil
	}
	return &model.PackageDetail{Manifest: model.PackageManifest{Name: "demo"}, Source: file}, nil
}

// fakeCaps is a test-only CapabilityService.
type fakeCaps struct {
	cands []model.CapabilityCandidate
	err   error
}

func (f *fakeCaps) Discover(ctx context.Context, req model.DiscoverRequest) ([]model.CapabilityCandidate, error) {
	return f.cands, f.err
}

// fakeDiagnostics is a test-only DiagnosticsService.
type fakeDiagnostics struct {
	doctor *model.DoctorReport
	status *model.StatusReport
	err    error
}

func (f *fakeDiagnostics) Doctor(ctx context.Context) (*model.DoctorReport, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.doctor != nil {
		return f.doctor, nil
	}
	return &model.DoctorReport{
		AVMHome:          model.CheckResult{OK: true, Detail: "/tmp/.avm"},
		PATH:             model.CheckResult{OK: true},
		ShellIntegration: model.CheckResult{OK: false, Detail: "not installed"},
		Runtimes: []model.RuntimeCheck{
			{Runtime: "codex", Available: true, Binary: "/usr/bin/codex", Version: "1.0"},
			{Runtime: "claudecode", Available: false, Issues: []string{"binary not found"}},
		},
	}, nil
}

func (f *fakeDiagnostics) Status(ctx context.Context, agent string) (*model.StatusReport, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.status != nil {
		return f.status, nil
	}
	return &model.StatusReport{}, nil
}
