package adapter

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/xz1220/agent-vm/internal/config"
)

type RenderInputOptions struct {
	ProjectRoot  string
	ActiveDir    string
	RuntimeHomes map[string]string
}

func RenderInputFromResolved(resolved *config.ResolvedActivation, runtime string, opts RenderInputOptions) (RenderInput, error) {
	if resolved == nil {
		return RenderInput{}, fmt.Errorf("resolved activation is nil")
	}
	if runtime == "" {
		return RenderInput{}, fmt.Errorf("runtime is required")
	}

	agent, ok := resolved.RuntimeAgents[runtime]
	if !ok {
		return RenderInput{}, fmt.Errorf("runtime %q has no resolved agent", runtime)
	}
	if opts.ActiveDir == "" {
		opts.ActiveDir = config.ActiveDir()
	}

	return RenderInput{
		Active:       activeRefFromConfig(resolved.Active),
		Runtime:      runtime,
		Agent:        agentFromConfig(agent),
		Capabilities: capabilitySetFromConfig(resolved.Capabilities[runtime], opts.ActiveDir),
		Memory:       portableMemoryFromConfig(resolved.Memory[runtime]),
		ProjectRoot:  opts.ProjectRoot,
		ActiveDir:    opts.ActiveDir,
		RuntimeHome:  opts.RuntimeHomes[runtime],
	}, nil
}

func RenderInputsFromResolved(resolved *config.ResolvedActivation, opts RenderInputOptions) ([]RenderInput, error) {
	if resolved == nil {
		return nil, fmt.Errorf("resolved activation is nil")
	}

	runtimes := resolvedRuntimeOrder(resolved)
	inputs := make([]RenderInput, 0, len(runtimes))
	for _, runtime := range runtimes {
		input, err := RenderInputFromResolved(resolved, runtime, opts)
		if err != nil {
			return nil, err
		}
		inputs = append(inputs, input)
	}
	return inputs, nil
}

func activeRefFromConfig(ref config.ActiveRef) ActiveRef {
	return ActiveRef{
		Kind: ref.Kind,
		Name: ref.Name,
	}
}

func agentFromConfig(agent config.AgentProfile) Agent {
	return Agent{
		Name:        agent.Name,
		Description: agent.Description,
		SourceScope: agent.SourceScope,
		Instructions: Instructions{
			System:     agent.Instructions.System,
			Developer:  agent.Instructions.Developer,
			References: append([]string(nil), agent.Instructions.References...),
		},
		Model: ModelConfig{
			Model:           agent.ModelRun.Model,
			ReasoningEffort: agent.ModelRun.ReasoningEffort,
			Verbosity:       agent.ModelRun.Verbosity,
			Temperature:     agent.ModelRun.Temperature,
		},
		Permissions: PermissionConfig{
			Approval:              agent.Permissions.Approval,
			Sandbox:               agent.Permissions.Sandbox,
			Allow:                 append([]string(nil), agent.Permissions.Allow...),
			Deny:                  append([]string(nil), agent.Permissions.Deny...),
			AdditionalDirectories: append([]string(nil), agent.Permissions.AdditionalDirectories...),
		},
		MemoryRefs: memoryRefsFromConfig(agent.MemoryRefs),
	}
}

func capabilitySetFromConfig(capabilities config.ResolvedCapabilities, activeDir string) CapabilitySet {
	return CapabilitySet{
		Skills:     skillRefs(capabilities, activeDir),
		MCPServers: mcpServers(capabilities),
		Commands:   capabilityRefs(capabilities.Commands),
		Hooks:      capabilityRefs(capabilities.Hooks),
		Toolsets:   toolsets(capabilities.Toolsets),
	}
}

func capabilityRefs(names []string) []CapabilityRef {
	refs := make([]CapabilityRef, 0, len(names))
	for _, name := range names {
		if name != "" {
			refs = append(refs, CapabilityRef{Name: name})
		}
	}
	return refs
}

func skillRefs(capabilities config.ResolvedCapabilities, activeDir string) []CapabilityRef {
	byName := make(map[string]config.ResolvedSkill, len(capabilities.SkillRefs))
	for _, skill := range capabilities.SkillRefs {
		if skill.Name != "" {
			byName[skill.Name] = skill
		}
	}

	refs := make([]CapabilityRef, 0, len(capabilities.Skills))
	seen := make(map[string]struct{}, len(capabilities.Skills))
	for _, name := range capabilities.Skills {
		if name == "" {
			continue
		}
		seen[name] = struct{}{}
		ref := CapabilityRef{Name: name}
		if _, ok := byName[name]; ok && activeDir != "" {
			ref.Path = filepath.ToSlash(filepath.Join(activeDir, "skills", name, "SKILL.md"))
		}
		refs = append(refs, ref)
	}

	var extras []string
	for name := range byName {
		if _, ok := seen[name]; !ok {
			extras = append(extras, name)
		}
	}
	sort.Strings(extras)
	for _, name := range extras {
		ref := CapabilityRef{Name: name}
		if activeDir != "" {
			ref.Path = filepath.ToSlash(filepath.Join(activeDir, "skills", name, "SKILL.md"))
		}
		refs = append(refs, ref)
	}
	return refs
}

func mcpServers(capabilities config.ResolvedCapabilities) []MCPServer {
	byName := make(map[string]config.ResolvedMCPServer, len(capabilities.MCPServers))
	for _, server := range capabilities.MCPServers {
		if server.Name != "" {
			byName[server.Name] = server
		}
	}

	servers := make([]MCPServer, 0, len(capabilities.MCPs))
	seen := make(map[string]struct{}, len(capabilities.MCPs))
	for _, name := range capabilities.MCPs {
		if name != "" {
			seen[name] = struct{}{}
			if server, ok := byName[name]; ok {
				servers = append(servers, mcpServerFromConfig(server))
				continue
			}
			servers = append(servers, MCPServer{Name: name})
		}
	}

	var extras []string
	for name := range byName {
		if _, ok := seen[name]; !ok {
			extras = append(extras, name)
		}
	}
	sort.Strings(extras)
	for _, name := range extras {
		servers = append(servers, mcpServerFromConfig(byName[name]))
	}
	return servers
}

func mcpServerFromConfig(server config.ResolvedMCPServer) MCPServer {
	return MCPServer{
		Name:    server.Name,
		Command: server.Command,
		Args:    append([]string(nil), server.Args...),
		Env:     envVars(server.Env),
		URL:     server.URL,
		Headers: envVars(server.Headers),
	}
}

func envVars(values map[string]string) []EnvVar {
	if len(values) == 0 {
		return nil
	}
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)

	vars := make([]EnvVar, 0, len(names))
	for _, name := range names {
		if name != "" {
			vars = append(vars, EnvVar{Name: name, Value: values[name]})
		}
	}
	return vars
}

func toolsets(values map[string]string) []Toolset {
	if len(values) == 0 {
		return nil
	}

	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	sort.Strings(names)

	items := make([]Toolset, 0, len(names))
	for _, name := range names {
		items = append(items, Toolset{Name: name, Mode: values[name]})
	}
	return items
}

func memoryRefsFromConfig(refs []config.MemoryRef) []MemoryRef {
	out := make([]MemoryRef, 0, len(refs))
	for _, ref := range refs {
		out = append(out, MemoryRef{
			ID:    ref.ID,
			Scope: ref.Scope,
			Path:  ref.Path,
			Mode:  ref.Mode,
		})
	}
	return out
}

func portableMemoryFromConfig(memory []config.PortableMemory) []PortableMemory {
	out := make([]PortableMemory, 0, len(memory))
	for _, item := range memory {
		out = append(out, PortableMemory{
			ID:    item.ID,
			Scope: item.Scope,
			Path:  item.Path,
			Mode:  item.Mode,
		})
	}
	return out
}

func resolvedRuntimeOrder(resolved *config.ResolvedActivation) []string {
	if resolved == nil || len(resolved.RuntimeAgents) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(resolved.RuntimeAgents))
	runtimes := make([]string, 0, len(resolved.RuntimeAgents))
	for _, runtime := range resolved.Targets {
		if _, ok := resolved.RuntimeAgents[runtime]; !ok {
			continue
		}
		if _, ok := seen[runtime]; ok {
			continue
		}
		seen[runtime] = struct{}{}
		runtimes = append(runtimes, runtime)
	}

	var remaining []string
	for runtime := range resolved.RuntimeAgents {
		if _, ok := seen[runtime]; !ok {
			remaining = append(remaining, runtime)
		}
	}
	sort.Strings(remaining)
	return append(runtimes, remaining...)
}
