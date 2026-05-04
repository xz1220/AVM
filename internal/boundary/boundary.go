package boundary

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/xz1220/agent-vm/internal/config"
)

type IsolationStatus string

const (
	IsolationIsolated    IsolationStatus = "isolated"
	IsolationShared      IsolationStatus = "shared"
	IsolationUnsupported IsolationStatus = "unsupported"
)

type BoundaryType string

const (
	BoundaryRuntimeHome BoundaryType = "runtime_home"
	BoundaryProcessEnv  BoundaryType = "process_env"
	BoundaryNone        BoundaryType = "none"
)

type BoundaryKey struct {
	AgentID   string `json:"agent_id,omitempty"`
	AgentName string `json:"agent_name,omitempty"`
	Runtime   string `json:"runtime,omitempty"`
}

type RuntimeBoundary struct {
	Key          BoundaryKey       `json:"key"`
	Root         string            `json:"root,omitempty"`
	Env          map[string]string `json:"env,omitempty"`
	RunEnv       map[string]string `json:"run_env,omitempty"`
	Paths        map[string]string `json:"paths,omitempty"`
	Isolation    IsolationStatus   `json:"isolation"`
	BoundaryType BoundaryType      `json:"boundary_type"`
	Warnings     []string          `json:"warnings,omitempty"`
}

type Input struct {
	Runtime   string
	AgentID   string
	AgentName string
	Overrides map[string]string
}

type Resolver interface {
	ResolveBoundary(input Input) (RuntimeBoundary, error)
}

type Registry struct {
	resolvers map[string]Resolver
}

func NewRegistry() *Registry {
	registry := &Registry{resolvers: make(map[string]Resolver)}
	registry.Register("codex", codexResolver{})
	registry.Register("claude-code", claudeResolver{})
	registry.Register("opencode", opencodeResolver{})
	return registry
}

func (r *Registry) Register(runtime string, resolver Resolver) {
	if r == nil || runtime == "" || resolver == nil {
		return
	}
	r.resolvers[runtime] = resolver
}

func (r *Registry) ResolveBoundary(input Input) (RuntimeBoundary, error) {
	if input.Runtime == "" {
		return RuntimeBoundary{}, fmt.Errorf("runtime is required")
	}
	if input.AgentID == "" {
		return RuntimeBoundary{}, fmt.Errorf("agent id is required for runtime %q boundary", input.Runtime)
	}
	if r == nil {
		r = NewRegistry()
	}
	resolver, ok := r.resolvers[input.Runtime]
	if !ok || resolver == nil {
		return unsupportedBoundary(input), nil
	}
	boundary, err := resolver.ResolveBoundary(input)
	if err != nil {
		return RuntimeBoundary{}, err
	}
	if override := input.Overrides[input.Runtime]; override != "" {
		boundary.Isolation = IsolationShared
		boundary.Warnings = append(boundary.Warnings, "runtime home override may share memory with another agent")
		boundary = normalizeBoundary(boundary)
	}
	return boundary, nil
}

func Resolve(input Input) (RuntimeBoundary, error) {
	return NewRegistry().ResolveBoundary(input)
}

type codexResolver struct{}

func (codexResolver) ResolveBoundary(input Input) (RuntimeBoundary, error) {
	root := rootForInput(input)
	return normalizeBoundary(RuntimeBoundary{
		Key:          boundaryKey(input),
		Root:         root,
		Env:          map[string]string{"CODEX_HOME": root},
		RunEnv:       map[string]string{"CODEX_HOME": root},
		Paths:        map[string]string{"home": root, "config": filepath.Join(root, "config.toml")},
		Isolation:    IsolationIsolated,
		BoundaryType: BoundaryRuntimeHome,
	}), nil
}

type claudeResolver struct{}

func (claudeResolver) ResolveBoundary(input Input) (RuntimeBoundary, error) {
	root := rootForInput(input)
	env := map[string]string{
		"CLAUDE_CONFIG_DIR":     root,
		"AVM_CLAUDE_MCP_CONFIG": filepath.Join(root, "mcp.json"),
	}
	if input.AgentName != "" {
		env["AVM_CLAUDE_AGENT"] = input.AgentName
	}
	return normalizeBoundary(RuntimeBoundary{
		Key:          boundaryKey(input),
		Root:         root,
		Env:          env,
		RunEnv:       cloneMap(env),
		Paths:        map[string]string{"home": root, "mcp_config": filepath.Join(root, "mcp.json")},
		Isolation:    IsolationIsolated,
		BoundaryType: BoundaryRuntimeHome,
	}), nil
}

type opencodeResolver struct{}

func (opencodeResolver) ResolveBoundary(input Input) (RuntimeBoundary, error) {
	root := rootForInput(input)
	configDir := filepath.Join(root, "config")
	configPath := filepath.Join(configDir, "opencode.json")
	dataDir := filepath.Join(root, "data")
	xdgData := filepath.Join(root, "xdg-data")
	xdgState := filepath.Join(root, "xdg-state")
	xdgCache := filepath.Join(root, "xdg-cache")
	env := map[string]string{
		"OPENCODE_CONFIG":     configPath,
		"OPENCODE_CONFIG_DIR": configDir,
	}
	runEnv := cloneMap(env)
	runEnv["OPENCODE_DB"] = filepath.Join(dataDir, "opencode.db")
	runEnv["XDG_DATA_HOME"] = xdgData
	runEnv["XDG_STATE_HOME"] = xdgState
	runEnv["XDG_CACHE_HOME"] = xdgCache
	return normalizeBoundary(RuntimeBoundary{
		Key:    boundaryKey(input),
		Root:   root,
		Env:    env,
		RunEnv: runEnv,
		Paths: map[string]string{
			"config_dir":  configDir,
			"config_path": configPath,
			"data_dir":    dataDir,
			"db_path":     filepath.Join(dataDir, "opencode.db"),
			"xdg_data":    xdgData,
			"xdg_state":   xdgState,
			"xdg_cache":   xdgCache,
		},
		Isolation:    IsolationIsolated,
		BoundaryType: BoundaryProcessEnv,
		Warnings:     []string{"OpenCode data/state isolation requires launching through avm run opencode"},
	}), nil
}

func unsupportedBoundary(input Input) RuntimeBoundary {
	return normalizeBoundary(RuntimeBoundary{
		Key:          boundaryKey(input),
		Isolation:    IsolationUnsupported,
		BoundaryType: BoundaryNone,
		Warnings:     []string{"runtime does not support AVM-managed memory isolation"},
	})
}

func rootForInput(input Input) string {
	if override := input.Overrides[input.Runtime]; override != "" {
		return filepath.Clean(override)
	}
	return agentRuntimeRoot(input.AgentID, input.Runtime)
}

func agentRuntimeRoot(agentID, runtime string) string {
	return config.AgentRuntimeHomeDir(agentID, runtime)
}

func boundaryKey(input Input) BoundaryKey {
	return BoundaryKey{
		AgentID:   input.AgentID,
		AgentName: input.AgentName,
		Runtime:   input.Runtime,
	}
}

func normalizeBoundary(boundary RuntimeBoundary) RuntimeBoundary {
	boundary.Root = filepath.Clean(boundary.Root)
	if boundary.Root == "." {
		boundary.Root = ""
	}
	boundary.Env = cleanMap(boundary.Env)
	boundary.RunEnv = cleanMap(boundary.RunEnv)
	if len(boundary.RunEnv) == 0 {
		boundary.RunEnv = cloneMap(boundary.Env)
	}
	boundary.Paths = cleanMap(boundary.Paths)
	boundary.Warnings = uniqueStrings(boundary.Warnings)
	return boundary
}

func cleanMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	keys := make([]string, 0, len(in))
	for key := range in {
		if key != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value := in[key]; value != "" {
			out[key] = filepath.Clean(value)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cloneMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
