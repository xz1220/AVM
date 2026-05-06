package config

import (
	"os"
	"sort"
)

type EnvironmentReference struct {
	Kind  string
	Name  string
	Path  string
	Field string
}

func FindEnvironmentReferences(name, cwd string) ([]EnvironmentReference, error) {
	if !validName(name) {
		return nil, fieldError("", "name", "invalid name %q", name)
	}

	var refs []EnvironmentReference
	cfg, err := ReadGlobalConfig()
	if err == nil {
		if cfg.Active.Kind == ActiveKindEnv && cfg.Active.Name == name {
			refs = append(refs, EnvironmentReference{
				Kind:  "active",
				Name:  cfg.Active.Name,
				Path:  GlobalConfigPath(),
				Field: "active",
			})
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	override, err := ReadProjectOverride(cwd)
	if err == nil {
		if override.Extends == name {
			refs = append(refs, EnvironmentReference{
				Kind:  "project_override",
				Name:  override.Extends,
				Path:  ProjectEnvPath(cwd),
				Field: "extends",
			})
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	sortEnvironmentReferences(refs)
	return refs, nil
}

func UpdateEnvironmentReferences(oldName, newName, cwd string) ([]EnvironmentReference, error) {
	if !validName(oldName) {
		return nil, fieldError("", "old_name", "invalid name %q", oldName)
	}
	if !validName(newName) {
		return nil, fieldError("", "new_name", "invalid name %q", newName)
	}

	refs, err := FindEnvironmentReferences(oldName, cwd)
	if err != nil {
		return nil, err
	}

	override, err := ReadProjectOverride(cwd)
	if err == nil {
		if override.Extends == oldName {
			override.Extends = newName
			if err := WriteProjectOverride(cwd, override); err != nil {
				return nil, err
			}
		}
	} else if !os.IsNotExist(err) {
		return nil, err
	}

	return refs, nil
}

func sortEnvironmentReferences(refs []EnvironmentReference) {
	sort.Slice(refs, func(i, j int) bool {
		left := refs[i]
		right := refs[j]
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		if left.Name != right.Name {
			return left.Name < right.Name
		}
		if left.Field != right.Field {
			return left.Field < right.Field
		}
		return left.Path < right.Path
	})
}
