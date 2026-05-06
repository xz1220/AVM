package config

import (
	"fmt"
	"os"
	"sort"
)

func ReadEnvironment(name string) (*Environment, error) {
	if !validName(name) {
		return nil, fieldError("", "name", "invalid name %q", name)
	}
	path := EnvPath(name)

	var env Environment
	if err := readYAML(path, &env); err != nil {
		return nil, err
	}
	env.ApplyDefaults()
	if env.Name != name {
		return nil, fieldError(path, "name", "expected %q, got %q", name, env.Name)
	}
	if err := validateEnvironment(&env, path); err != nil {
		return nil, err
	}
	return &env, nil
}

func WriteEnvironment(env *Environment) error {
	if env == nil {
		return fieldError("", "", "environment is nil")
	}
	env.ApplyDefaults()
	if err := validateEnvironment(env, ""); err != nil {
		return err
	}
	path := EnvPath(env.Name)
	if err := validateEnvironment(env, path); err != nil {
		return err
	}
	return writeYAML(path, env)
}

func EnvironmentExists(name string) (bool, error) {
	if _, err := ReadEnvironment(name); err == nil {
		return true, nil
	} else if os.IsNotExist(err) {
		return false, nil
	} else {
		return false, err
	}
}

func DeleteEnvironment(name string) error {
	if !validName(name) {
		return fieldError("", "name", "invalid name %q", name)
	}
	return os.Remove(EnvPath(name))
}

func RenameEnvironment(oldName, newName string) error {
	if oldName == newName {
		return fmt.Errorf("old and new environment names are the same")
	}
	env, err := ReadEnvironment(oldName)
	if err != nil {
		return err
	}
	if exists, err := EnvironmentExists(newName); err != nil {
		return err
	} else if exists {
		return fmt.Errorf("environment %q already exists", newName)
	}

	renamed := cloneEnvironment(env)
	renamed.Name = newName
	if err := WriteEnvironment(renamed); err != nil {
		return err
	}
	return DeleteEnvironment(oldName)
}

func ReadProjectOverride(cwd string) (*ProjectOverride, error) {
	return readProjectOverride(ProjectEnvPath(cwd))
}

func WriteProjectOverride(cwd string, override *ProjectOverride) error {
	if override == nil {
		return fieldError("", "", "project override is nil")
	}
	path := ProjectEnvPath(cwd)
	if err := validateProjectOverride(override, path); err != nil {
		return err
	}
	return writeYAML(path, override)
}

func ProjectOverrideExists(cwd string) (bool, error) {
	if _, err := ReadProjectOverride(cwd); err == nil {
		return true, nil
	} else if os.IsNotExist(err) {
		return false, nil
	} else {
		return false, err
	}
}

func DeleteProjectOverride(cwd string) error {
	return os.Remove(ProjectEnvPath(cwd))
}

func ListEnvironments() ([]EnvironmentSummary, error) {
	paths, err := listYAMLFiles(EnvsDir())
	if err != nil {
		return nil, err
	}

	summaries := make([]EnvironmentSummary, 0, len(paths))
	for _, path := range paths {
		var env Environment
		if err := readYAML(path, &env); err != nil {
			return nil, err
		}
		env.ApplyDefaults()
		if err := validateEnvironment(&env, path); err != nil {
			return nil, err
		}
		summaries = append(summaries, EnvironmentSummary{
			Name:        env.Name,
			Description: env.Description,
			Version:     env.Version,
			Path:        path,
		})
	}
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].Name < summaries[j].Name
	})
	return summaries, nil
}
