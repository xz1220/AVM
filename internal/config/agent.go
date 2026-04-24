package config

import "sort"

func ReadAgent(name string, scope Scope, cwd string) (*AgentProfile, error) {
	if !validName(name) {
		return nil, fieldError("", "name", "invalid name %q", name)
	}
	path, err := agentPathForScope(name, scope, cwd)
	if err != nil {
		return nil, err
	}

	var agent AgentProfile
	if err := readYAML(path, &agent); err != nil {
		return nil, err
	}
	agent.ApplyDefaults(defaultSourceScopeForAgent(scope))
	if agent.Name != name {
		return nil, fieldError(path, "name", "expected %q, got %q", name, agent.Name)
	}
	if err := validateAgentProfile(&agent, path); err != nil {
		return nil, err
	}
	return &agent, nil
}

func WriteAgent(agent *AgentProfile, scope Scope, cwd string) error {
	if agent == nil {
		return fieldError("", "", "agent profile is nil")
	}
	agent.ApplyDefaults(defaultSourceScopeForAgent(scope))
	if err := validateAgentProfile(agent, ""); err != nil {
		return err
	}
	path, err := agentPathForScope(agent.Name, scope, cwd)
	if err != nil {
		return err
	}
	if err := validateAgentProfile(agent, path); err != nil {
		return err
	}
	return writeYAML(path, agent)
}

func ListAgents(scope Scope, cwd string) ([]AgentSummary, error) {
	dir, err := agentDirForScope(scope, cwd)
	if err != nil {
		return nil, err
	}
	paths, err := listYAMLFiles(dir)
	if err != nil {
		return nil, err
	}

	summaries := make([]AgentSummary, 0, len(paths))
	for _, path := range paths {
		var agent AgentProfile
		if err := readYAML(path, &agent); err != nil {
			return nil, err
		}
		agent.ApplyDefaults(defaultSourceScopeForAgent(scope))
		if err := validateAgentProfile(&agent, path); err != nil {
			return nil, err
		}
		summaries = append(summaries, AgentSummary{
			Name:        agent.Name,
			Description: agent.Description,
			Version:     agent.Version,
			SourceScope: agent.SourceScope,
			Path:        path,
		})
	}
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].Name < summaries[j].Name
	})
	return summaries, nil
}

func defaultSourceScopeForAgent(scope Scope) string {
	switch scope {
	case ScopeProject, ScopeLocal:
		return string(scope)
	default:
		return string(ScopeGlobal)
	}
}
