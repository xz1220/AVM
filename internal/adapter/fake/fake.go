// Package fake provides a deterministic adapter implementation for tests.
package fake

import (
	"bytes"
	"fmt"
	"os"
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

	name      string
	found     bool
	version   string
	configDir string
	rendered  []*adapter.RenderPlan
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
				SourcePath: "agent.lifecycle_hooks",
				Status:     adapter.MappingIgnored,
				Reason:     "fake adapter does not execute lifecycle hooks",
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

	results := make([]adapter.RenderOperationResult, 0, len(normalized.Operations))
	for _, operation := range normalized.Operations {
		changed, err := applyRenderOperation(operation)
		if err != nil {
			return nil, err
		}
		results = append(results, adapter.RenderOperationResult{
			OperationID: operation.ID,
			Action:      operation.Action,
			Path:        operation.Path,
			Changed:     changed,
		})
	}

	a.mu.Lock()
	a.rendered = append(a.rendered, normalized)
	a.mu.Unlock()

	return &adapter.RenderResult{
		Runtime:      normalized.Runtime,
		Operations:   results,
		ManagedPaths: append([]adapter.ManagedPath(nil), normalized.ManagedPaths...),
		Mappings:     append([]adapter.FieldMapping(nil), normalized.Mappings...),
		Warnings:     append([]string(nil), normalized.Warnings...),
	}, nil
}

func applyRenderOperation(operation adapter.RenderOperation) (bool, error) {
	if operation.Path == "" {
		return false, fmt.Errorf("fake adapter render operation %q has empty path", operation.ID)
	}

	switch operation.Action {
	case adapter.OperationEnsureDir:
		return ensureDir(operation.Path)
	case adapter.OperationWriteFile:
		return writeFile(operation.Path, operation.Content)
	case adapter.OperationRemoveFile:
		return removeFile(operation.Path)
	case adapter.OperationMergeSection, adapter.OperationStructuredSet:
		return false, fmt.Errorf("fake adapter cannot apply %s operation %q at %s", operation.Action, operation.ID, operation.Path)
	default:
		return false, fmt.Errorf("fake adapter render operation %q has unsupported action %q", operation.ID, operation.Action)
	}
}

func ensureDir(path string) (bool, error) {
	info, err := os.Stat(path)
	if err == nil {
		if !info.IsDir() {
			return false, fmt.Errorf("cannot ensure directory %s: path exists and is not a directory", path)
		}
		return false, nil
	}
	if !os.IsNotExist(err) {
		return false, err
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return false, err
	}
	return true, nil
}

func writeFile(path string, content []byte) (bool, error) {
	existing, err := os.ReadFile(path)
	if err == nil && bytes.Equal(existing, content) {
		return false, nil
	}
	if err != nil && !os.IsNotExist(err) {
		return false, err
	}

	parent := filepath.Dir(path)
	if parent != "." && parent != "" {
		if err := os.MkdirAll(parent, 0o700); err != nil {
			return false, err
		}
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		return false, err
	}
	return true, nil
}

func removeFile(path string) (bool, error) {
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (a *Adapter) ManagedPaths(ctx adapter.Context, plan *adapter.RenderPlan) []adapter.ManagedPath {
	_ = ctx

	normalized := renderplan.Normalize(plan)
	if normalized == nil {
		return nil
	}
	return append([]adapter.ManagedPath(nil), normalized.ManagedPaths...)
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
