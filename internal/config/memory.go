package config

import "sort"

func ReadPortableMemory(id string, scope Scope) (*PortableMemory, error) {
	if !validName(id) {
		return nil, fieldError("", "id", "invalid id %q", id)
	}
	if !validMemoryScope(string(scope)) {
		return nil, fieldError("", "scope", "invalid value %q", scope)
	}
	path := MemoryPath(id, scope)

	var memory PortableMemory
	if err := readYAML(path, &memory); err != nil {
		return nil, err
	}
	memory.ApplyDefaults()
	if memory.ID != id {
		return nil, fieldError(path, "id", "expected %q, got %q", id, memory.ID)
	}
	if memory.Scope != string(scope) {
		return nil, fieldError(path, "scope", "expected %q, got %q", scope, memory.Scope)
	}
	if err := validatePortableMemory(&memory, path); err != nil {
		return nil, err
	}
	return &memory, nil
}

func WritePortableMemory(memory *PortableMemory) error {
	if memory == nil {
		return fieldError("", "", "portable memory is nil")
	}
	memory.ApplyDefaults()
	if err := validatePortableMemory(memory, ""); err != nil {
		return err
	}
	path := MemoryPath(memory.ID, Scope(memory.Scope))
	if err := validatePortableMemory(memory, path); err != nil {
		return err
	}
	return writeYAML(path, memory)
}

func ListPortableMemory(scope Scope) ([]PortableMemorySummary, error) {
	scopes := []Scope{scope}
	if scope == "" {
		scopes = []Scope{ScopeUser, ScopeProject, ScopeLocal, ScopeTeam}
	} else if !validMemoryScope(string(scope)) {
		return nil, fieldError("", "scope", "invalid value %q", scope)
	}

	var summaries []PortableMemorySummary
	for _, currentScope := range scopes {
		paths, err := listYAMLFiles(MemoryScopeDir(currentScope))
		if err != nil {
			return nil, err
		}
		for _, path := range paths {
			var memory PortableMemory
			if err := readYAML(path, &memory); err != nil {
				return nil, err
			}
			memory.ApplyDefaults()
			if err := validatePortableMemory(&memory, path); err != nil {
				return nil, err
			}
			summaries = append(summaries, PortableMemorySummary{
				ID:          memory.ID,
				Scope:       memory.Scope,
				Format:      memory.Format,
				Path:        path,
				Description: memory.Description,
			})
		}
	}
	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].Scope == summaries[j].Scope {
			return summaries[i].ID < summaries[j].ID
		}
		return summaries[i].Scope < summaries[j].Scope
	})
	return summaries, nil
}
