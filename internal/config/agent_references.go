package config

import (
	"os"
	"sort"
	"strconv"
)

type AgentReference struct {
	Kind    string
	Name    string
	Path    string
	Runtime string
	Field   string
}

func FindAgentReferences(name, cwd string) ([]AgentReference, error) {
	if !validName(name) {
		return nil, fieldError("", "name", "invalid name %q", name)
	}

	var refs []AgentReference
	cfg, err := ReadGlobalConfig()
	if err == nil {
		if cfg.Active.Kind == ActiveKindProfile && cfg.Active.Name == name {
			refs = append(refs, AgentReference{
				Kind:  "active",
				Name:  cfg.Active.Name,
				Path:  GlobalConfigPath(),
				Field: "active",
			})
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	envs, err := ListEnvironments()
	if err != nil {
		return nil, err
	}
	for _, summary := range envs {
		env, err := ReadEnvironment(summary.Name)
		if err != nil {
			return nil, err
		}
		refs = appendRuntimeAgentReferences(refs, "env", env.Name, summary.Path, env.RuntimeAgents, name)
	}

	override, err := ReadProjectOverride(cwd)
	if err == nil {
		refs = appendRuntimeAgentReferences(refs, "project_override", override.Extends, ProjectEnvPath(cwd), override.RuntimeAgents, name)
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	sortAgentReferences(refs)
	return refs, nil
}

func UpdateAgentReferences(oldName, newName, cwd string) ([]AgentReference, error) {
	if !validName(oldName) {
		return nil, fieldError("", "old_name", "invalid name %q", oldName)
	}
	if !validName(newName) {
		return nil, fieldError("", "new_name", "invalid name %q", newName)
	}

	refs, err := FindAgentReferences(oldName, cwd)
	if err != nil {
		return nil, err
	}

	envs, err := ListEnvironments()
	if err != nil {
		return nil, err
	}
	for _, summary := range envs {
		env, err := ReadEnvironment(summary.Name)
		if err != nil {
			return nil, err
		}
		if replaceRuntimeAgentReferences(env.RuntimeAgents, oldName, newName) {
			if err := WriteEnvironment(env); err != nil {
				return nil, err
			}
		}
	}

	override, err := ReadProjectOverride(cwd)
	if err == nil {
		if replaceRuntimeAgentReferences(override.RuntimeAgents, oldName, newName) {
			if err := WriteProjectOverride(cwd, override); err != nil {
				return nil, err
			}
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	return refs, nil
}

func appendRuntimeAgentReferences(refs []AgentReference, kind, ownerName, path string, agents map[string]RuntimeAgent, name string) []AgentReference {
	for runtime, agent := range agents {
		if agent.Primary == name {
			refs = append(refs, AgentReference{
				Kind:    kind,
				Name:    ownerName,
				Path:    path,
				Runtime: runtime,
				Field:   "runtime_agents." + runtime + ".primary",
			})
		}
		for i, available := range agent.Available {
			if available == name {
				refs = append(refs, AgentReference{
					Kind:    kind,
					Name:    ownerName,
					Path:    path,
					Runtime: runtime,
					Field:   "runtime_agents." + runtime + ".available[" + strconv.Itoa(i) + "]",
				})
			}
		}
	}
	return refs
}

func replaceRuntimeAgentReferences(agents map[string]RuntimeAgent, oldName, newName string) bool {
	changed := false
	for runtime, agent := range agents {
		if agent.Primary == oldName {
			agent.Primary = newName
			changed = true
		}
		for i, available := range agent.Available {
			if available == oldName {
				agent.Available[i] = newName
				changed = true
			}
		}
		agent.Available = dedupeRuntimeAgentAvailable(agent.Available)
		agents[runtime] = agent
	}
	return changed
}

func dedupeRuntimeAgentAvailable(values []string) []string {
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

func sortAgentReferences(refs []AgentReference) {
	sort.Slice(refs, func(i, j int) bool {
		left := refs[i]
		right := refs[j]
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		if left.Name != right.Name {
			return left.Name < right.Name
		}
		if left.Runtime != right.Runtime {
			return left.Runtime < right.Runtime
		}
		if left.Field != right.Field {
			return left.Field < right.Field
		}
		return left.Path < right.Path
	})
}
