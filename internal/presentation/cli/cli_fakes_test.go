package cli

import (
	"context"

	"github.com/xz1220/agent-vm/internal/app/model"
	"github.com/xz1220/agent-vm/internal/app/service"
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
	// Mirror service-level name validation so CLI tests cover the same
	// error paths users see (typed *service.Error with a stable Code).
	probe := &model.Agent{Identity: model.Identity{Name: req.Name}}
	if err := probe.Validate(); err != nil {
		code := service.CodeAgentInvalidName
		if req.Name == "" {
			code = service.CodeValidation
		}
		return nil, service.NewError(code, err.Error(), map[string]any{"name": req.Name})
	}
	if len(req.Runtimes) == 0 {
		return nil, service.MissingInputError("runtime",
			"at least one runtime preference is required")
	}
	if _, exists := f.agents[req.Name]; exists {
		switch req.OnConflict {
		case model.ResolveOverwrite:
			// fall through
		default:
			return nil, service.AgentConflictError(req.Name,
				"agent \""+req.Name+"\" already exists")
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
		return nil, service.AgentNotFoundError(name, nil)
	}
	return d, nil
}

func (f *fakeAgents) Edit(ctx context.Context, req model.EditAgentRequest) (*model.Agent, error) {
	f.editCalls = append(f.editCalls, req)
	d, ok := f.agents[req.Name]
	if !ok {
		return nil, service.AgentNotFoundError(req.Name, nil)
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
	if !req.Confirm {
		return service.MissingInputError("confirm", "set --yes to confirm")
	}
	if _, ok := f.agents[req.Name]; !ok {
		return service.AgentNotFoundError(req.Name, nil)
	}
	delete(f.agents, req.Name)
	f.deleted = append(f.deleted, req.Name)
	return nil
}

func (f *fakeAgents) Clone(ctx context.Context, name, newName string) (*model.Agent, error) {
	d, ok := f.agents[name]
	if !ok {
		return nil, service.AgentNotFoundError(name, nil)
	}
	if _, exists := f.agents[newName]; exists {
		return nil, service.AgentConflictError(newName, "agent \""+newName+"\" already exists")
	}
	clone := d.Agent
	clone.Identity.Name = newName
	f.put(clone)
	return &clone, nil
}

func (f *fakeAgents) Rename(ctx context.Context, oldName, newName string) (*model.Agent, error) {
	d, ok := f.agents[oldName]
	if !ok {
		return nil, service.AgentNotFoundError(oldName, nil)
	}
	if _, exists := f.agents[newName]; exists {
		return nil, service.AgentConflictError(newName, "agent \""+newName+"\" already exists")
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
	cands      []model.CapabilityCandidate
	err        error
	importRes  *model.ImportCapabilityResult
	importErr  error
	importReqs []model.ImportCapabilityRequest
	bootRes    *model.BootstrapCapabilitiesResult
	bootErr    error
	bootReqs   []model.BootstrapCapabilitiesRequest
	records    []model.CapabilityRecord
	listErr    error
	getErr     error
	getResp    *model.CapabilityRecord
	getReqs    []model.CapabilityID
}

func (f *fakeCaps) Discover(ctx context.Context, req model.DiscoverRequest) ([]model.CapabilityCandidate, error) {
	return f.cands, f.err
}
func (f *fakeCaps) Import(ctx context.Context, req model.ImportCapabilityRequest) (*model.ImportCapabilityResult, error) {
	f.importReqs = append(f.importReqs, req)
	if f.importErr != nil {
		return nil, f.importErr
	}
	if f.importRes != nil {
		return f.importRes, nil
	}
	return &model.ImportCapabilityResult{
		ID:      model.CapabilityID("cap_fake_" + req.Name),
		Created: true,
		Source:  req.Runtime + ":/fake/" + req.Name,
	}, nil
}
func (f *fakeCaps) Bootstrap(ctx context.Context, req model.BootstrapCapabilitiesRequest) (*model.BootstrapCapabilitiesResult, error) {
	f.bootReqs = append(f.bootReqs, req)
	if f.bootErr != nil {
		return nil, f.bootErr
	}
	if f.bootRes != nil {
		return f.bootRes, nil
	}
	return &model.BootstrapCapabilitiesResult{}, nil
}
func (f *fakeCaps) List(ctx context.Context) ([]model.CapabilityRecord, error) {
	return f.records, f.listErr
}
func (f *fakeCaps) Get(ctx context.Context, id model.CapabilityID) (*model.CapabilityRecord, error) {
	f.getReqs = append(f.getReqs, id)
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.getResp != nil {
		return f.getResp, nil
	}
	for i := range f.records {
		if f.records[i].ID == id {
			rec := f.records[i]
			return &rec, nil
		}
	}
	return nil, service.NewError(service.CodeCapabilityNotFound,
		"capability \""+string(id)+"\" not found",
		map[string]any{"id": string(id)})
}

// fakeDiagnostics is a test-only DiagnosticsService.
type fakeDiagnostics struct {
	doctor   *model.DoctorReport
	status   *model.StatusReport
	runtimes []model.RuntimeCheck
	err      error
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

func (f *fakeDiagnostics) Runtimes(ctx context.Context) ([]model.RuntimeCheck, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.runtimes != nil {
		return f.runtimes, nil
	}
	// Default mirrors the Doctor fake so tests that don't set runtimes
	// explicitly still have a sensible payload.
	if f.doctor != nil {
		return f.doctor.Runtimes, nil
	}
	return []model.RuntimeCheck{
		{Runtime: "codex", Available: true, Binary: "/usr/bin/codex", Version: "1.0"},
		{Runtime: "claudecode", Available: false, Issues: []string{"binary not found"}},
	}, nil
}
