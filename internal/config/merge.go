package config

type ProjectOverride struct {
	Extends          string                  `yaml:"extends,omitempty"`
	RuntimeAgents    map[string]RuntimeAgent `yaml:"runtime_agents,omitempty"`
	Targets          []string                `yaml:"targets,omitempty"`
	RuntimeOverrides map[string]any          `yaml:"runtime_overrides,omitempty"`
}

func MergeEnvironment(base *Environment, override *ProjectOverride) *Environment {
	if base == nil {
		return nil
	}

	merged := cloneEnvironment(base)
	if override == nil {
		return merged
	}

	if len(override.RuntimeAgents) > 0 {
		if merged.RuntimeAgents == nil {
			merged.RuntimeAgents = make(map[string]RuntimeAgent, len(override.RuntimeAgents))
		}
		for runtime, agentOverride := range override.RuntimeAgents {
			agent := merged.RuntimeAgents[runtime]
			if agentOverride.Primary != "" {
				agent.Primary = agentOverride.Primary
			}
			if agentOverride.Available != nil {
				agent.Available = cloneStringSlice(agentOverride.Available)
			}
			merged.RuntimeAgents[runtime] = agent
		}
	}

	if override.Targets != nil {
		merged.Targets = cloneStringSlice(override.Targets)
	}
	if override.RuntimeOverrides != nil {
		merged.RuntimeOverrides = mergeRuntimeOverrides(merged.RuntimeOverrides, override.RuntimeOverrides)
	}

	return merged
}

func readProjectOverride(path string) (*ProjectOverride, error) {
	var override ProjectOverride
	if err := readYAML(path, &override); err != nil {
		return nil, err
	}
	if err := validateProjectOverride(&override, path); err != nil {
		return nil, err
	}
	return &override, nil
}

func validateProjectOverride(override *ProjectOverride, path string) error {
	if override == nil {
		return fieldError(path, "", "project override is nil")
	}
	if override.Extends == "" {
		return fieldError(path, "extends", "required")
	}
	if !validName(override.Extends) {
		return fieldError(path, "extends", "invalid name %q", override.Extends)
	}
	for runtime, agent := range override.RuntimeAgents {
		if !isKnownTarget(runtime) {
			return fieldError(path, "runtime_agents", "invalid runtime %q", runtime)
		}
		if agent.Primary != "" && !validName(agent.Primary) {
			return fieldError(path, "runtime_agents."+runtime+".primary", "invalid name %q", agent.Primary)
		}
		for i, available := range agent.Available {
			if !validName(available) {
				return fieldError(path, "runtime_agents."+runtime+".available", "invalid name at %d: %q", i, available)
			}
		}
	}
	if override.Targets != nil {
		if err := validateTargets(path, "targets", override.Targets); err != nil {
			return err
		}
	}
	for runtime := range override.RuntimeOverrides {
		if !isKnownTarget(runtime) {
			return fieldError(path, "runtime_overrides", "invalid runtime %q", runtime)
		}
	}
	return nil
}

func cloneEnvironment(env *Environment) *Environment {
	if env == nil {
		return nil
	}
	return &Environment{
		Name:             env.Name,
		Description:      env.Description,
		Version:          env.Version,
		RuntimeAgents:    cloneRuntimeAgents(env.RuntimeAgents),
		Targets:          cloneStringSlice(env.Targets),
		RuntimeOverrides: cloneAnyMap(env.RuntimeOverrides),
	}
}

func cloneRuntimeAgents(agents map[string]RuntimeAgent) map[string]RuntimeAgent {
	if agents == nil {
		return nil
	}
	cloned := make(map[string]RuntimeAgent, len(agents))
	for runtime, agent := range agents {
		cloned[runtime] = RuntimeAgent{
			Primary:   agent.Primary,
			Available: cloneStringSlice(agent.Available),
		}
	}
	return cloned
}

func mergeRuntimeOverrides(base, override map[string]any) map[string]any {
	if base == nil && override == nil {
		return nil
	}
	merged := cloneAnyMap(base)
	if merged == nil {
		merged = make(map[string]any, len(override))
	}
	for runtime, overrideValue := range override {
		baseMap, baseOK := merged[runtime].(map[string]any)
		overrideMap, overrideOK := overrideValue.(map[string]any)
		if baseOK && overrideOK {
			merged[runtime] = mergeAnyMap(baseMap, overrideMap)
			continue
		}
		merged[runtime] = cloneAny(overrideValue)
	}
	return merged
}

func mergeAnyMap(base, override map[string]any) map[string]any {
	merged := cloneAnyMap(base)
	if merged == nil {
		merged = make(map[string]any, len(override))
	}
	for key, overrideValue := range override {
		baseMap, baseOK := merged[key].(map[string]any)
		overrideMap, overrideOK := overrideValue.(map[string]any)
		if baseOK && overrideOK {
			merged[key] = mergeAnyMap(baseMap, overrideMap)
			continue
		}
		merged[key] = cloneAny(overrideValue)
	}
	return merged
}

func cloneAnyMap(values map[string]any) map[string]any {
	if values == nil {
		return nil
	}
	cloned := make(map[string]any, len(values))
	for key, value := range values {
		cloned[key] = cloneAny(value)
	}
	return cloned
}

func cloneAny(value any) any {
	switch v := value.(type) {
	case map[string]any:
		return cloneAnyMap(v)
	case []any:
		cloned := make([]any, len(v))
		for i, item := range v {
			cloned[i] = cloneAny(item)
		}
		return cloned
	case []string:
		return cloneStringSlice(v)
	case map[string]string:
		return cloneStringMap(v)
	default:
		return value
	}
}

func cloneStringSlice(values []string) []string {
	if values == nil {
		return nil
	}
	cloned := make([]string, len(values))
	copy(cloned, values)
	return cloned
}

func cloneStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}
