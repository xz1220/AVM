package config

type Scope string

const (
	ScopeGlobal  Scope = "global"
	ScopeProject Scope = "project"
	ScopeLocal   Scope = "local"
	ScopeUser    Scope = "user"
	ScopeTeam    Scope = "team"
)

const (
	ActiveKindProfile = "profile"
	ActiveKindEnv     = "env"
)

type TargetCapability struct {
	Level string
}

var KnownTargets = map[string]TargetCapability{
	"claude-code": {Level: "full"},
	"codex":       {Level: "full"},
	"cline":       {Level: "full"},
	"cursor":      {Level: "partial"},
}

type ActiveRef struct {
	Kind string `yaml:"kind" json:"kind"`
	Name string `yaml:"name" json:"name"`
}

type GlobalConfig struct {
	Version  string         `yaml:"version"`
	Active   ActiveRef      `yaml:"active"`
	Defaults DefaultsConfig `yaml:"defaults"`
	Settings Settings       `yaml:"settings"`
}

type DefaultsConfig struct {
	SourceScope      string   `yaml:"source_scope"`
	Targets          []string `yaml:"targets,omitempty"`
	ConflictStrategy string   `yaml:"conflict_strategy"`
}

type Settings struct {
	BackupEnabled  bool                `yaml:"backup_enabled"`
	BackupMaxCount int                 `yaml:"backup_max_count"`
	WriteMode      string              `yaml:"write_mode"`
	ShellPrompt    ShellPromptSettings `yaml:"shell_prompt"`
}

type ShellPromptSettings struct {
	Enabled bool   `yaml:"enabled"`
	Format  string `yaml:"format"`
}

type AgentProfile struct {
	Name              string                    `yaml:"name"`
	Description       string                    `yaml:"description,omitempty"`
	Version           string                    `yaml:"version"`
	SourceScope       string                    `yaml:"source_scope"`
	Runtime           RuntimePreferences        `yaml:"runtime"`
	Identity          AgentIdentity             `yaml:"identity,omitempty"`
	Instructions      Instructions              `yaml:"instructions,omitempty"`
	ModelRun          ModelRun                  `yaml:"model_run,omitempty"`
	IOContract        IOContract                `yaml:"io_contract,omitempty"`
	Capabilities      CapabilityRefs            `yaml:"capabilities,omitempty"`
	Permissions       Permissions               `yaml:"permissions"`
	MemoryRefs        []MemoryRef               `yaml:"memory_refs,omitempty"`
	LifecycleHooks    LifecycleHooks            `yaml:"lifecycle_hooks,omitempty"`
	RuntimeExtensions map[string]map[string]any `yaml:"runtime_extensions,omitempty"`
}

type RuntimePreferences struct {
	Preferred string   `yaml:"preferred"`
	Kind      string   `yaml:"kind"`
	Mode      string   `yaml:"mode"`
	Fallback  []string `yaml:"fallback,omitempty"`
}

type AgentIdentity struct {
	DisplayName string   `yaml:"display_name,omitempty"`
	Role        string   `yaml:"role,omitempty"`
	Tags        []string `yaml:"tags,omitempty"`
}

type Instructions struct {
	System     string   `yaml:"system,omitempty"`
	Developer  string   `yaml:"developer,omitempty"`
	References []string `yaml:"references,omitempty"`
}

type ModelRun struct {
	Model           string   `yaml:"model,omitempty"`
	ReasoningEffort string   `yaml:"reasoning_effort,omitempty"`
	Verbosity       string   `yaml:"verbosity,omitempty"`
	Temperature     *float64 `yaml:"temperature,omitempty"`
}

type IOContract struct {
	InputModes  []string `yaml:"input_modes,omitempty"`
	OutputStyle string   `yaml:"output_style,omitempty"`
	Language    string   `yaml:"language,omitempty"`
}

type CapabilityRefs struct {
	Skills   []string          `yaml:"skills,omitempty"`
	MCPs     []string          `yaml:"mcps,omitempty"`
	Commands []string          `yaml:"commands,omitempty"`
	Hooks    []string          `yaml:"hooks,omitempty"`
	Toolsets map[string]string `yaml:"toolsets,omitempty"`
}

type Permissions struct {
	Approval              string   `yaml:"approval"`
	Sandbox               string   `yaml:"sandbox"`
	Allow                 []string `yaml:"allow,omitempty"`
	Deny                  []string `yaml:"deny,omitempty"`
	AdditionalDirectories []string `yaml:"additional_directories,omitempty"`
}

type MemoryRef struct {
	ID    string `yaml:"id"`
	Scope string `yaml:"scope"`
	Path  string `yaml:"path"`
	Mode  string `yaml:"mode"`
}

type LifecycleHooks struct {
	BeforeRun []string `yaml:"before_run,omitempty"`
	AfterRun  []string `yaml:"after_run,omitempty"`
}

type Environment struct {
	Name             string                  `yaml:"name"`
	Description      string                  `yaml:"description,omitempty"`
	Version          string                  `yaml:"version"`
	RuntimeAgents    map[string]RuntimeAgent `yaml:"runtime_agents,omitempty"`
	Targets          []string                `yaml:"targets,omitempty"`
	RuntimeOverrides map[string]any          `yaml:"runtime_overrides,omitempty"`
}

type RuntimeAgent struct {
	Primary   string   `yaml:"primary"`
	Available []string `yaml:"available,omitempty"`
}

type PortableMemory struct {
	ID          string            `yaml:"id"`
	Scope       string            `yaml:"scope"`
	Format      string            `yaml:"format"`
	Path        string            `yaml:"path"`
	Description string            `yaml:"description,omitempty"`
	Mode        string            `yaml:"mode"`
	Tags        []string          `yaml:"tags,omitempty"`
	Origin      MemoryOrigin      `yaml:"origin,omitempty"`
	WritePolicy MemoryWritePolicy `yaml:"write_policy,omitempty"`
}

type MemoryOrigin struct {
	Type       string `yaml:"type,omitempty"`
	Runtime    string `yaml:"runtime,omitempty"`
	SourcePath string `yaml:"source_path,omitempty"`
}

type MemoryWritePolicy struct {
	AllowPush           bool `yaml:"allow_push"`
	RequireConfirmation bool `yaml:"require_confirmation"`
}

type AgentSummary struct {
	Name        string
	Description string
	Version     string
	SourceScope string
	Path        string
}

type EnvironmentSummary struct {
	Name        string
	Description string
	Version     string
	Path        string
}

type PortableMemorySummary struct {
	ID          string
	Scope       string
	Format      string
	Path        string
	Description string
}

func (c *GlobalConfig) ApplyDefaults() {
	if c.Version == "" {
		c.Version = "1"
	}
	if c.Active.Kind == "" {
		c.Active.Kind = ActiveKindProfile
	}
	if c.Active.Name == "" {
		c.Active.Name = "default"
	}
	if c.Defaults.SourceScope == "" {
		c.Defaults.SourceScope = string(ScopeGlobal)
	}
	if len(c.Defaults.Targets) == 0 {
		c.Defaults.Targets = []string{"claude-code", "codex", "cline"}
	}
	if c.Defaults.ConflictStrategy == "" {
		c.Defaults.ConflictStrategy = "prompt"
	}
	if c.Settings.BackupMaxCount == 0 {
		c.Settings.BackupMaxCount = 10
	}
	if c.Settings.WriteMode == "" {
		c.Settings.WriteMode = "managed-only"
	}
	if c.Settings.ShellPrompt.Format == "" {
		c.Settings.ShellPrompt.Format = "avm:%s"
	}
}

func (a *AgentProfile) ApplyDefaults(defaultSourceScope string) {
	if a.Version == "" {
		a.Version = "1.0.0"
	}
	if a.SourceScope == "" {
		if defaultSourceScope == "" {
			defaultSourceScope = string(ScopeGlobal)
		}
		a.SourceScope = defaultSourceScope
	}
	if a.Runtime.Kind == "" {
		a.Runtime.Kind = "local"
	}
	if a.Runtime.Mode == "" {
		a.Runtime.Mode = "primary"
	}
	if a.ModelRun.ReasoningEffort == "" {
		a.ModelRun.ReasoningEffort = "medium"
	}
	if a.ModelRun.Verbosity == "" {
		a.ModelRun.Verbosity = "normal"
	}
	if a.Permissions.Approval == "" {
		a.Permissions.Approval = "on-request"
	}
	if a.Permissions.Sandbox == "" {
		a.Permissions.Sandbox = "workspace-write"
	}
}

func (e *Environment) ApplyDefaults() {
	if e.Version == "" {
		e.Version = "1.0.0"
	}
}

func (m *PortableMemory) ApplyDefaults() {
	if m.Format == "" {
		m.Format = "markdown"
	}
	if m.Mode == "" {
		m.Mode = "read"
	}
	if m.Origin.Type == "" {
		m.Origin.Type = "file"
	}
}
