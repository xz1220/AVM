package config

import (
	"os"
	"sort"
)

type ResolvedActivation struct {
	Active        ActiveRef                       `yaml:"active" json:"active"`
	Env           *Environment                    `yaml:"env,omitempty" json:"env,omitempty"`
	RuntimeAgents map[string]AgentProfile         `yaml:"runtime_agents" json:"runtime_agents"`
	Capabilities  map[string]ResolvedCapabilities `yaml:"capabilities,omitempty" json:"capabilities,omitempty"`
	Targets       []string                        `yaml:"targets" json:"targets"`
	SourceFiles   []string                        `yaml:"source_files,omitempty" json:"source_files,omitempty"`
	Warnings      []string                        `yaml:"warnings,omitempty" json:"warnings,omitempty"`
}

type ResolvedCapabilities struct {
	Skills     []string            `yaml:"skills,omitempty" json:"skills,omitempty"`
	SkillRefs  []ResolvedSkill     `yaml:"skill_refs,omitempty" json:"skill_refs,omitempty"`
	MCPs       []string            `yaml:"mcps,omitempty" json:"mcps,omitempty"`
	MCPServers []ResolvedMCPServer `yaml:"mcp_servers,omitempty" json:"mcp_servers,omitempty"`
	Commands   []string            `yaml:"commands,omitempty" json:"commands,omitempty"`
	Hooks      []string            `yaml:"hooks,omitempty" json:"hooks,omitempty"`
	Toolsets   map[string]string   `yaml:"toolsets,omitempty" json:"toolsets,omitempty"`
}

type ResolvedMCPServer struct {
	Name       string            `yaml:"name" json:"name"`
	Type       string            `yaml:"type,omitempty" json:"type,omitempty"`
	Command    string            `yaml:"command,omitempty" json:"command,omitempty"`
	Args       []string          `yaml:"args,omitempty" json:"args,omitempty"`
	Env        map[string]string `yaml:"env,omitempty" json:"env,omitempty"`
	URL        string            `yaml:"url,omitempty" json:"url,omitempty"`
	Headers    map[string]string `yaml:"headers,omitempty" json:"headers,omitempty"`
	SourcePath string            `yaml:"source_path,omitempty" json:"source_path,omitempty"`
}

type ResolvedSkill struct {
	Name       string `yaml:"name" json:"name"`
	SourceDir  string `yaml:"source_dir" json:"source_dir"`
	SourcePath string `yaml:"source_path" json:"source_path"`
}

func ResolveActivation(ref ActiveRef, cwd string) (*ResolvedActivation, error) {
	if err := validateActiveRef(ref, ""); err != nil {
		return nil, err
	}

	switch ref.Kind {
	case ActiveKindProfile:
		return resolveProfileActivation(ref, cwd)
	case ActiveKindEnv:
		return resolveEnvironmentActivation(ref, cwd)
	default:
		return nil, fieldError("", "active.kind", "invalid value %q", ref.Kind)
	}
}

func resolveProfileActivation(ref ActiveRef, cwd string) (*ResolvedActivation, error) {
	agent, agentPath, err := readAgentPreferProject(ref.Name, cwd)
	if err != nil {
		return nil, err
	}

	targets, targetSources, err := targetsForProfile(agent)
	if err != nil {
		return nil, err
	}

	agents := make(map[string]AgentProfile, len(targets))
	for _, runtime := range targets {
		agents[runtime] = *agent
	}

	capabilities, capabilitySources, warnings := capabilitiesForAgents(agents)

	return &ResolvedActivation{
		Active:        ref,
		RuntimeAgents: agents,
		Capabilities:  capabilities,
		Targets:       targets,
		SourceFiles:   uniqueStrings(append(append([]string{agentPath}, targetSources...), capabilitySources...)),
		Warnings:      warnings,
	}, nil
}

func resolveEnvironmentActivation(ref ActiveRef, cwd string) (*ResolvedActivation, error) {
	base, err := ReadEnvironment(ref.Name)
	if err != nil {
		return nil, err
	}

	env := cloneEnvironment(base)
	sourceFiles := []string{EnvPath(ref.Name)}
	validatePath := EnvPath(ref.Name)

	projectOverridePath := ProjectEnvPath(cwd)
	override, err := readProjectOverride(projectOverridePath)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
	} else {
		if override.Extends != ref.Name {
			return nil, fieldError(projectOverridePath, "extends", "expected %q, got %q", ref.Name, override.Extends)
		}
		env = MergeEnvironment(base, override)
		sourceFiles = append(sourceFiles, projectOverridePath)
		validatePath = projectOverridePath
	}

	if err := validateEnvironment(env, validatePath); err != nil {
		return nil, err
	}

	runtimes := sortedRuntimeKeys(env.RuntimeAgents)
	agents := make(map[string]AgentProfile, len(runtimes))
	for _, runtime := range runtimes {
		profileName := env.RuntimeAgents[runtime].Primary
		agent, agentPath, err := readAgentPreferProject(profileName, cwd)
		if err != nil {
			return nil, fieldError("", "runtime_agents."+runtime+".primary", "%v", err)
		}
		agents[runtime] = *agent
		sourceFiles = append(sourceFiles, agentPath)
	}

	capabilities, capabilitySources, warnings := capabilitiesForAgents(agents)

	return &ResolvedActivation{
		Active:        ref,
		Env:           env,
		RuntimeAgents: agents,
		Capabilities:  capabilities,
		Targets:       cloneStringSlice(env.Targets),
		SourceFiles:   uniqueStrings(append(sourceFiles, capabilitySources...)),
		Warnings:      warnings,
	}, nil
}

func readAgentPreferProject(name, cwd string) (*AgentProfile, string, error) {
	projectPath := ProjectAgentPath(cwd, name)
	agent, err := ReadAgent(name, ScopeProject, cwd)
	if err == nil {
		return agent, projectPath, nil
	}
	if !os.IsNotExist(err) {
		return nil, "", err
	}

	globalPath := AgentPath(name)
	agent, err = ReadAgent(name, ScopeGlobal, cwd)
	if err == nil {
		return agent, globalPath, nil
	}
	if os.IsNotExist(err) {
		return nil, "", fieldError("", "agent", "profile %q not found in %s or %s", name, projectPath, globalPath)
	}
	return nil, "", err
}

func targetsForProfile(agent *AgentProfile) ([]string, []string, error) {
	if agent == nil {
		return nil, nil, fieldError("", "agent", "profile is nil")
	}
	targets := uniqueStrings(append([]string{agent.Runtime.Preferred}, agent.Runtime.Fallback...))
	if len(targets) == 0 {
		return nil, nil, fieldError("", "runtime.preferred", "required")
	}
	return targets, nil, nil
}

func capabilitiesForAgents(agents map[string]AgentProfile) (map[string]ResolvedCapabilities, []string, []string) {
	if len(agents) == 0 {
		return nil, nil, nil
	}
	capabilities := make(map[string]ResolvedCapabilities, len(agents))
	var sourceFiles []string
	var warnings []string
	for runtime, agent := range agents {
		resolved, sources, capabilityWarnings := capabilitiesForAgent(agent)
		capabilities[runtime] = resolved
		sourceFiles = append(sourceFiles, sources...)
		for _, warning := range capabilityWarnings {
			if warning != "" {
				warnings = append(warnings, runtime+": "+warning)
			}
		}
	}
	return capabilities, uniqueStrings(sourceFiles), uniqueStrings(warnings)
}

func capabilitiesForAgent(agent AgentProfile) (ResolvedCapabilities, []string, []string) {
	resolved := ResolvedCapabilities{
		Skills:   cloneStringSlice(agent.Capabilities.Skills),
		MCPs:     cloneStringSlice(agent.Capabilities.MCPs),
		Commands: cloneStringSlice(agent.Capabilities.Commands),
		Hooks:    cloneStringSlice(agent.Capabilities.Hooks),
		Toolsets: cloneStringMap(agent.Capabilities.Toolsets),
	}

	var sourceFiles []string
	var warnings []string
	for _, name := range resolved.Skills {
		skill, path, err := resolvedSkill(name)
		if err != nil {
			if !os.IsNotExist(err) {
				warnings = append(warnings, "skill registry "+name+" could not be read: "+err.Error())
			}
			continue
		}
		resolved.SkillRefs = append(resolved.SkillRefs, skill)
		sourceFiles = append(sourceFiles, path)
	}
	for _, name := range resolved.MCPs {
		server, path, err := resolvedMCPServer(name)
		if err != nil {
			if !os.IsNotExist(err) {
				warnings = append(warnings, "mcp registry "+name+" could not be read: "+err.Error())
			}
			resolved.MCPServers = append(resolved.MCPServers, ResolvedMCPServer{Name: name})
			continue
		}
		resolved.MCPServers = append(resolved.MCPServers, server)
		sourceFiles = append(sourceFiles, path)
	}
	return resolved, sourceFiles, warnings
}

func sortedRuntimeKeys(agents map[string]RuntimeAgent) []string {
	keys := make([]string, 0, len(agents))
	for runtime := range agents {
		keys = append(keys, runtime)
	}
	sort.Strings(keys)
	return keys
}

func uniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	unique := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		unique = append(unique, value)
	}
	return unique
}
