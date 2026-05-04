package config

import (
	"os"
	"path/filepath"
)

func AvmDir() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(string(os.PathSeparator), ".avm")
	}
	return filepath.Join(home, ".avm")
}

func GlobalConfigPath() string {
	return filepath.Join(AvmDir(), "config.yaml")
}

func AgentsDir() string {
	return filepath.Join(AvmDir(), "agents")
}

func AgentPath(name string) string {
	return filepath.Join(AgentsDir(), name+".yaml")
}

func EnvsDir() string {
	return filepath.Join(AvmDir(), "envs")
}

func EnvPath(name string) string {
	return filepath.Join(EnvsDir(), name+".yaml")
}

func RegistryDir() string {
	return filepath.Join(AvmDir(), "registry")
}

func RegistryKindDir(kind string) string {
	return filepath.Join(RegistryDir(), kind)
}

func ActiveDir() string {
	return filepath.Join(AvmDir(), "active")
}

func RuntimeHomesDir() string {
	return filepath.Join(AvmDir(), "runtime-homes")
}

func AgentRuntimeHomeDir(agentID, runtime string) string {
	return filepath.Join(RuntimeHomesDir(), "agents", agentID, RuntimeHomeRuntimeName(runtime))
}

func RuntimeHomeRuntimeName(runtime string) string {
	switch runtime {
	case "claude-code":
		return "claude"
	default:
		return runtime
	}
}

func StateDir() string {
	return filepath.Join(AvmDir(), "state")
}

func BackupDir() string {
	return filepath.Join(AvmDir(), "backup")
}

func ProjectAvmDir(cwd string) string {
	return filepath.Join(absPath(cwd), ".avm")
}

func ProjectEnvPath(cwd string) string {
	return filepath.Join(ProjectAvmDir(cwd), "env.yaml")
}

func ProjectAgentsDir(cwd string) string {
	return filepath.Join(ProjectAvmDir(cwd), "agents")
}

func ProjectAgentPath(cwd, name string) string {
	return filepath.Join(ProjectAgentsDir(cwd), name+".yaml")
}

func agentPathForScope(name string, scope Scope, cwd string) (string, error) {
	switch scope {
	case "", ScopeGlobal:
		return AgentPath(name), nil
	case ScopeProject, ScopeLocal:
		return ProjectAgentPath(cwd, name), nil
	default:
		return "", fieldError("", "scope", "invalid value %q", scope)
	}
}

func agentDirForScope(scope Scope, cwd string) (string, error) {
	switch scope {
	case "", ScopeGlobal:
		return AgentsDir(), nil
	case ScopeProject, ScopeLocal:
		return ProjectAgentsDir(cwd), nil
	default:
		return "", fieldError("", "scope", "invalid value %q", scope)
	}
}

func absPath(path string) string {
	if path == "" {
		path = "."
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return abs
}
