// Package fake provides a deterministic adapter implementation for tests.
package fake

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/xz1220/agent-vm/internal/adapter"
	"github.com/xz1220/agent-vm/internal/adapter/renderplan"
)

const defaultName = "fake"

// Adapter is a configurable fake runtime adapter for sync and contract tests.
type Adapter struct {
	mu sync.Mutex

	name       string
	found      bool
	version    string
	configDir  string
	imported   *adapter.ImportResult
	memoryPlan *adapter.MemoryImportPlan
	rendered   []*adapter.RenderPlan
}

type Option func(*Adapter)

func New(opts ...Option) *Adapter {
	a := &Adapter{
		name:      defaultName,
		found:     true,
		version:   "fake-1",
		configDir: "/tmp/avm-fake",
	}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

func WithName(name string) Option {
	return func(a *Adapter) {
		if name != "" {
			a.name = name
		}
	}
}

func WithFound(found bool) Option {
	return func(a *Adapter) {
		a.found = found
	}
}

func WithVersion(version string) Option {
	return func(a *Adapter) {
		a.version = version
	}
}

func WithConfigDir(configDir string) Option {
	return func(a *Adapter) {
		a.configDir = configDir
	}
}

func WithImportResult(result *adapter.ImportResult) Option {
	return func(a *Adapter) {
		a.imported = cloneImportResult(result)
	}
}

func WithMemoryImportPlan(plan *adapter.MemoryImportPlan) Option {
	return func(a *Adapter) {
		a.memoryPlan = cloneMemoryImportPlan(plan)
	}
}

func (a *Adapter) Name() string {
	return a.name
}

func (a *Adapter) Detect(ctx adapter.Context) adapter.Detection {
	_ = ctx

	return adapter.Detection{
		Runtime:   a.name,
		Found:     a.found,
		Version:   a.version,
		ConfigDir: a.configDir,
	}
}

func (a *Adapter) Import(ctx adapter.Context) (*adapter.ImportResult, error) {
	_ = ctx

	a.mu.Lock()
	defer a.mu.Unlock()

	if a.imported != nil {
		return cloneImportResult(a.imported), nil
	}

	return &adapter.ImportResult{Runtime: a.name}, nil
}

func (a *Adapter) Plan(ctx adapter.Context, input adapter.RenderInput) (*adapter.RenderPlan, error) {
	_ = ctx

	runtime := input.Runtime
	if runtime == "" {
		runtime = a.name
	}

	agentName := input.Agent.Name
	if agentName == "" {
		agentName = input.Active.Name
	}
	if agentName == "" {
		agentName = "unnamed"
	}

	targetPath := fakeTargetPath(input.ProjectRoot, runtime, agentName)
	plan := &adapter.RenderPlan{
		Runtime:   runtime,
		Active:    input.Active,
		AgentName: agentName,
		ManagedPaths: []adapter.ManagedPath{
			{
				Path:        targetPath,
				Owner:       "avm",
				Description: "fake rendered agent profile",
				Required:    true,
				MergeMode:   adapter.MergeModeWholeFile,
			},
		},
		Operations: []adapter.RenderOperation{
			{
				ID:          "write-agent",
				Action:      adapter.OperationWriteFile,
				Path:        targetPath,
				Content:     []byte(renderedContent(runtime, agentName, input)),
				Description: "write fake rendered agent profile",
				Required:    true,
			},
		},
		Mappings: []adapter.FieldMapping{
			{
				SourcePath: "agent.name",
				TargetPath: targetPath + "#name",
				Status:     adapter.MappingNative,
			},
			{
				SourcePath: "agent.description",
				TargetPath: targetPath + "#description",
				Status:     adapter.MappingNative,
			},
			{
				SourcePath: "agent.instructions.developer",
				TargetPath: targetPath + "#instructions",
				Status:     adapter.MappingRenderedAsInstructions,
			},
			{
				SourcePath: "agent.memory_refs",
				TargetPath: targetPath + "#memory",
				Status:     adapter.MappingRenderedAsInstructions,
			},
			{
				SourcePath: "agent.lifecycle_hooks",
				Status:     adapter.MappingIgnored,
				Reason:     "fake adapter does not execute lifecycle hooks",
			},
			{
				SourcePath: "runtime.native_memory_write",
				Status:     adapter.MappingUnsupported,
				Reason:     "fake adapter only supports memory import planning",
			},
		},
	}

	return renderplan.Normalize(plan), nil
}

func (a *Adapter) Render(ctx adapter.Context, plan *adapter.RenderPlan) (*adapter.RenderResult, error) {
	_ = ctx

	normalized := renderplan.Normalize(plan)
	if normalized == nil {
		return nil, fmt.Errorf("fake adapter render plan is nil")
	}

	a.mu.Lock()
	a.rendered = append(a.rendered, normalized)
	a.mu.Unlock()

	results := make([]adapter.RenderOperationResult, 0, len(normalized.Operations))
	for _, operation := range normalized.Operations {
		results = append(results, adapter.RenderOperationResult{
			OperationID: operation.ID,
			Action:      operation.Action,
			Path:        operation.Path,
			Changed:     true,
		})
	}

	return &adapter.RenderResult{
		Runtime:      normalized.Runtime,
		Operations:   results,
		ManagedPaths: append([]adapter.ManagedPath(nil), normalized.ManagedPaths...),
		Mappings:     append([]adapter.FieldMapping(nil), normalized.Mappings...),
		Warnings:     append([]string(nil), normalized.Warnings...),
	}, nil
}

func (a *Adapter) ManagedPaths(ctx adapter.Context, plan *adapter.RenderPlan) []adapter.ManagedPath {
	_ = ctx

	normalized := renderplan.Normalize(plan)
	if normalized == nil {
		return nil
	}
	return append([]adapter.ManagedPath(nil), normalized.ManagedPaths...)
}

func (a *Adapter) ImportMemory(ctx adapter.Context, opts adapter.MemoryImportOptions) (*adapter.MemoryImportPlan, error) {
	_ = ctx

	a.mu.Lock()
	defer a.mu.Unlock()

	if a.memoryPlan != nil {
		return cloneMemoryImportPlan(a.memoryPlan), nil
	}

	runtime := opts.Runtime
	if runtime == "" {
		runtime = a.name
	}

	return &adapter.MemoryImportPlan{
		Runtime: runtime,
		Source:  opts.Source,
	}, nil
}

func (a *Adapter) RenderedPlans() []*adapter.RenderPlan {
	a.mu.Lock()
	defer a.mu.Unlock()

	plans := make([]*adapter.RenderPlan, 0, len(a.rendered))
	for _, plan := range a.rendered {
		plans = append(plans, renderplan.Normalize(plan))
	}
	return plans
}

func fakeTargetPath(projectRoot, runtime, agentName string) string {
	if projectRoot == "" {
		projectRoot = "."
	}
	return filepath.ToSlash(filepath.Join(projectRoot, ".avm-fake", runtime, agentName+".rendered"))
}

func renderedContent(runtime, agentName string, input adapter.RenderInput) string {
	var builder strings.Builder
	writeLine := func(format string, args ...any) {
		builder.WriteString(fmt.Sprintf(format, args...))
		builder.WriteByte('\n')
	}

	writeLine("runtime: %s", runtime)
	writeLine("agent: %s", agentName)
	writeLine("description: %s", input.Agent.Description)
	writeLine("model: %s", input.Agent.Model.Model)
	writeLine("reasoning_effort: %s", input.Agent.Model.ReasoningEffort)
	writeLine("approval: %s", input.Agent.Permissions.Approval)
	writeLine("sandbox: %s", input.Agent.Permissions.Sandbox)
	writeLine("system:")
	writeLine("%s", input.Agent.Instructions.System)
	writeLine("developer:")
	writeLine("%s", input.Agent.Instructions.Developer)

	for _, name := range sortedCapabilityNames(input.Capabilities.Skills) {
		writeLine("skill: %s", name)
	}
	for _, name := range sortedMCPNames(input.Capabilities.MCPServers) {
		writeLine("mcp: %s", name)
	}
	for _, id := range sortedMemoryIDs(input.Memory) {
		writeLine("memory: %s", id)
	}

	return builder.String()
}

func sortedCapabilityNames(refs []adapter.CapabilityRef) []string {
	names := make([]string, 0, len(refs))
	for _, ref := range refs {
		names = append(names, ref.Name)
	}
	sort.Strings(names)
	return names
}

func sortedMCPNames(servers []adapter.MCPServer) []string {
	names := make([]string, 0, len(servers))
	for _, server := range servers {
		names = append(names, server.Name)
	}
	sort.Strings(names)
	return names
}

func sortedMemoryIDs(memory []adapter.PortableMemory) []string {
	ids := make([]string, 0, len(memory))
	for _, item := range memory {
		ids = append(ids, item.ID)
	}
	sort.Strings(ids)
	return ids
}

func cloneImportResult(result *adapter.ImportResult) *adapter.ImportResult {
	if result == nil {
		return nil
	}

	cloned := *result
	cloned.Agents = append([]adapter.ImportedAgent(nil), result.Agents...)
	for i := range cloned.Agents {
		cloned.Agents[i].Instructions.References = append([]string(nil), result.Agents[i].Instructions.References...)
		cloned.Agents[i].Mappings = append([]adapter.FieldMapping(nil), result.Agents[i].Mappings...)
	}
	cloned.Warnings = append([]string(nil), result.Warnings...)
	return &cloned
}

func cloneMemoryImportPlan(plan *adapter.MemoryImportPlan) *adapter.MemoryImportPlan {
	if plan == nil {
		return nil
	}

	cloned := *plan
	cloned.Candidates = append([]adapter.PortableMemory(nil), plan.Candidates...)
	cloned.Diffs = append([]adapter.MemoryDiff(nil), plan.Diffs...)
	cloned.Warnings = append([]string(nil), plan.Warnings...)
	return &cloned
}
