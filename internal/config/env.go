package config

import "sort"

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
