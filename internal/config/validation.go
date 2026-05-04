package config

import (
	"fmt"
	"regexp"
)

var (
	nameRegex    = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
	agentIDRegex = regexp.MustCompile(`^agt_[a-f0-9]{16,64}$`)
)

func Validate(value any) error {
	switch v := value.(type) {
	case *GlobalConfig:
		return validateGlobalConfig(v, "")
	case GlobalConfig:
		return validateGlobalConfig(&v, "")
	case *AgentProfile:
		return validateAgentProfile(v, "")
	case AgentProfile:
		return validateAgentProfile(&v, "")
	case *Environment:
		return validateEnvironment(v, "")
	case Environment:
		return validateEnvironment(&v, "")
	case *ActiveRef:
		return validateActiveRef(*v, "")
	case ActiveRef:
		return validateActiveRef(v, "")
	default:
		return fmt.Errorf("unsupported config type %T", value)
	}
}

func ValidateActiveRef(ref ActiveRef) error {
	return validateActiveRef(ref, "")
}

func ValidateGlobalConfig(cfg *GlobalConfig) error {
	return validateGlobalConfig(cfg, "")
}

func ValidateAgentProfile(agent *AgentProfile) error {
	return validateAgentProfile(agent, "")
}

func ValidateEnvironment(env *Environment) error {
	return validateEnvironment(env, "")
}

func validateActiveRef(ref ActiveRef, path string) error {
	if ref.Kind != ActiveKindProfile && ref.Kind != ActiveKindEnv {
		return fieldError(path, "active.kind", "invalid value %q", ref.Kind)
	}
	if !validName(ref.Name) {
		return fieldError(path, "active.name", "invalid name %q", ref.Name)
	}
	return nil
}

func validateGlobalConfig(cfg *GlobalConfig, path string) error {
	if cfg == nil {
		return fieldError(path, "", "global config is nil")
	}
	if cfg.Version == "" {
		return fieldError(path, "version", "required")
	}
	if err := validateActiveRef(cfg.Active, path); err != nil {
		return err
	}
	if !validSourceScope(cfg.Defaults.SourceScope) {
		return fieldError(path, "defaults.source_scope", "invalid value %q", cfg.Defaults.SourceScope)
	}
	if len(cfg.Defaults.Targets) == 0 {
		return fieldError(path, "defaults.targets", "at least one target is required")
	}
	if err := validateTargets(path, "defaults.targets", cfg.Defaults.Targets); err != nil {
		return err
	}
	if !oneOf(cfg.Defaults.ConflictStrategy, "prompt", "overwrite", "skip", "fail", "rename") {
		return fieldError(path, "defaults.conflict_strategy", "invalid value %q", cfg.Defaults.ConflictStrategy)
	}
	if cfg.Settings.BackupMaxCount < 0 {
		return fieldError(path, "settings.backup_max_count", "must be >= 0")
	}
	if !oneOf(cfg.Settings.WriteMode, "managed-only", "allow-runtime-overwrite") {
		return fieldError(path, "settings.write_mode", "invalid value %q", cfg.Settings.WriteMode)
	}
	if cfg.Settings.ShellPrompt.Format == "" {
		return fieldError(path, "settings.shell_prompt.format", "required")
	}
	return nil
}

func validateAgentProfile(agent *AgentProfile, path string) error {
	if agent == nil {
		return fieldError(path, "", "agent profile is nil")
	}
	if !validName(agent.Name) {
		return fieldError(path, "name", "invalid name %q", agent.Name)
	}
	if !validAgentID(agent.ID) {
		return fieldError(path, "id", "invalid agent id %q", agent.ID)
	}
	if agent.Version == "" {
		return fieldError(path, "version", "required")
	}
	if !validSourceScope(agent.SourceScope) {
		return fieldError(path, "source_scope", "invalid value %q", agent.SourceScope)
	}
	if !isKnownTarget(agent.Runtime.Preferred) {
		return fieldError(path, "runtime.preferred", "invalid value %q", agent.Runtime.Preferred)
	}
	if !oneOf(agent.Runtime.Kind, "local", "remote") {
		return fieldError(path, "runtime.kind", "invalid value %q", agent.Runtime.Kind)
	}
	if !oneOf(agent.Runtime.Mode, "primary", "subagent", "all") {
		return fieldError(path, "runtime.mode", "invalid value %q", agent.Runtime.Mode)
	}
	for i, fallback := range agent.Runtime.Fallback {
		if !isKnownTarget(fallback) {
			return fieldError(path, fmt.Sprintf("runtime.fallback[%d]", i), "invalid value %q", fallback)
		}
	}
	if agent.ModelRun.ReasoningEffort != "" && !oneOf(agent.ModelRun.ReasoningEffort, "low", "medium", "high", "xhigh") {
		return fieldError(path, "model_run.reasoning_effort", "invalid value %q", agent.ModelRun.ReasoningEffort)
	}
	if agent.ModelRun.Verbosity != "" && !oneOf(agent.ModelRun.Verbosity, "quiet", "concise", "normal", "verbose", "low", "medium", "high") {
		return fieldError(path, "model_run.verbosity", "invalid value %q", agent.ModelRun.Verbosity)
	}
	if err := validateCapabilityNames(path, "capabilities.skills", agent.Capabilities.Skills); err != nil {
		return err
	}
	if err := validateCapabilityNames(path, "capabilities.mcps", agent.Capabilities.MCPs); err != nil {
		return err
	}
	if err := validateCapabilityNames(path, "capabilities.commands", agent.Capabilities.Commands); err != nil {
		return err
	}
	if err := validateCapabilityNames(path, "capabilities.hooks", agent.Capabilities.Hooks); err != nil {
		return err
	}
	for name := range agent.Capabilities.Toolsets {
		if !validName(name) {
			return fieldError(path, "capabilities.toolsets", "invalid name %q", name)
		}
	}
	if !oneOf(agent.Permissions.Approval, "never", "on-request", "prompt", "untrusted", "on-risky-actions") {
		return fieldError(path, "permissions.approval", "invalid value %q", agent.Permissions.Approval)
	}
	if !oneOf(agent.Permissions.Sandbox, "read-only", "workspace-write", "danger-full-access") {
		return fieldError(path, "permissions.sandbox", "invalid value %q", agent.Permissions.Sandbox)
	}
	for runtime := range agent.RuntimeExtensions {
		if !validName(runtime) {
			return fieldError(path, "runtime_extensions", "invalid runtime name %q", runtime)
		}
	}
	return nil
}

func validateEnvironment(env *Environment, path string) error {
	if env == nil {
		return fieldError(path, "", "environment is nil")
	}
	if !validName(env.Name) {
		return fieldError(path, "name", "invalid name %q", env.Name)
	}
	if env.Version == "" {
		return fieldError(path, "version", "required")
	}
	if len(env.RuntimeAgents) == 0 {
		return fieldError(path, "runtime_agents", "at least one runtime agent mapping is required")
	}
	for runtime, agent := range env.RuntimeAgents {
		if !isKnownTarget(runtime) {
			return fieldError(path, "runtime_agents", "invalid runtime %q", runtime)
		}
		if !validName(agent.Primary) {
			return fieldError(path, "runtime_agents."+runtime+".primary", "invalid name %q", agent.Primary)
		}
		for i, available := range agent.Available {
			if !validName(available) {
				return fieldError(path, fmt.Sprintf("runtime_agents.%s.available[%d]", runtime, i), "invalid name %q", available)
			}
		}
	}
	if len(env.Targets) == 0 {
		return fieldError(path, "targets", "at least one target is required")
	}
	if err := validateTargets(path, "targets", env.Targets); err != nil {
		return err
	}
	for runtime := range env.RuntimeOverrides {
		if !isKnownTarget(runtime) {
			return fieldError(path, "runtime_overrides", "invalid runtime %q", runtime)
		}
	}
	return nil
}

func validateCapabilityNames(path, field string, names []string) error {
	for i, name := range names {
		if !validName(name) {
			return fieldError(path, fmt.Sprintf("%s[%d]", field, i), "invalid name %q", name)
		}
	}
	return nil
}

func validateTargets(path, field string, targets []string) error {
	seen := make(map[string]struct{}, len(targets))
	for i, target := range targets {
		if !isKnownTarget(target) {
			return fieldError(path, fmt.Sprintf("%s[%d]", field, i), "invalid target %q", target)
		}
		if _, ok := seen[target]; ok {
			return fieldError(path, fmt.Sprintf("%s[%d]", field, i), "duplicate target %q", target)
		}
		seen[target] = struct{}{}
	}
	return nil
}

func validName(name string) bool {
	return nameRegex.MatchString(name)
}

func validAgentID(id string) bool {
	return agentIDRegex.MatchString(id)
}

func validSourceScope(scope string) bool {
	return oneOf(scope, string(ScopeGlobal), string(ScopeProject), string(ScopeLocal))
}

func isKnownTarget(target string) bool {
	_, ok := KnownTargets[target]
	return ok
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func fieldError(path, field, format string, args ...any) error {
	message := fmt.Sprintf(format, args...)
	if path != "" && field != "" {
		return fmt.Errorf("%s: %s: %s", path, field, message)
	}
	if path != "" {
		return fmt.Errorf("%s: %s", path, message)
	}
	if field != "" {
		return fmt.Errorf("%s: %s", field, message)
	}
	return fmt.Errorf("%s", message)
}
