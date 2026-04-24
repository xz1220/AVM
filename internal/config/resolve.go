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
	Memory        map[string][]PortableMemory     `yaml:"memory,omitempty" json:"memory,omitempty"`
	Targets       []string                        `yaml:"targets" json:"targets"`
	SourceFiles   []string                        `yaml:"source_files,omitempty" json:"source_files,omitempty"`
	Warnings      []string                        `yaml:"warnings,omitempty" json:"warnings,omitempty"`
}

type ResolvedCapabilities struct {
	Skills   []string          `yaml:"skills,omitempty" json:"skills,omitempty"`
	MCPs     []string          `yaml:"mcps,omitempty" json:"mcps,omitempty"`
	Commands []string          `yaml:"commands,omitempty" json:"commands,omitempty"`
	Hooks    []string          `yaml:"hooks,omitempty" json:"hooks,omitempty"`
	Toolsets map[string]string `yaml:"toolsets,omitempty" json:"toolsets,omitempty"`
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

	runtime := agent.Runtime.Preferred
	agents := map[string]AgentProfile{
		runtime: *agent,
	}

	return &ResolvedActivation{
		Active:        ref,
		RuntimeAgents: agents,
		Capabilities:  capabilitiesForAgents(agents),
		Targets:       targets,
		SourceFiles:   uniqueStrings(append([]string{agentPath}, targetSources...)),
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

	return &ResolvedActivation{
		Active:        ref,
		Env:           env,
		RuntimeAgents: agents,
		Capabilities:  capabilitiesForAgents(agents),
		Targets:       cloneStringSlice(env.Targets),
		SourceFiles:   uniqueStrings(sourceFiles),
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
	cfg, err := ReadGlobalConfig()
	if err == nil {
		if len(cfg.Defaults.Targets) > 0 {
			return cloneStringSlice(cfg.Defaults.Targets), []string{GlobalConfigPath()}, nil
		}
		return []string{agent.Runtime.Preferred}, []string{GlobalConfigPath()}, nil
	}
	if !os.IsNotExist(err) {
		return nil, nil, err
	}
	return []string{agent.Runtime.Preferred}, nil, nil
}

func capabilitiesForAgents(agents map[string]AgentProfile) map[string]ResolvedCapabilities {
	if len(agents) == 0 {
		return nil
	}
	capabilities := make(map[string]ResolvedCapabilities, len(agents))
	for runtime, agent := range agents {
		capabilities[runtime] = capabilitiesForAgent(agent)
	}
	return capabilities
}

func capabilitiesForAgent(agent AgentProfile) ResolvedCapabilities {
	return ResolvedCapabilities{
		Skills:   cloneStringSlice(agent.Capabilities.Skills),
		MCPs:     cloneStringSlice(agent.Capabilities.MCPs),
		Commands: cloneStringSlice(agent.Capabilities.Commands),
		Hooks:    cloneStringSlice(agent.Capabilities.Hooks),
		Toolsets: cloneStringMap(agent.Capabilities.Toolsets),
	}
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
