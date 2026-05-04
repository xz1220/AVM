package adapter

import (
	"context"

	"github.com/xz1220/agent-vm/internal/boundary"
)

// Context keeps the adapter interface close to the design docs while using the
// standard library context type.
type Context = context.Context

// Adapter translates an AVM render input into a runtime-specific render plan
// and applies that plan to runtime-managed paths.
type Adapter interface {
	Name() string
	Detect(ctx Context) Detection
	Plan(ctx Context, input RenderInput) (*RenderPlan, error)
	Render(ctx Context, plan *RenderPlan) (*RenderResult, error)
	ManagedPaths(ctx Context, plan *RenderPlan) []ManagedPath
}

// Detection describes whether a runtime is available and where its config
// files live.
type Detection struct {
	Runtime   string   `json:"runtime"`
	Found     bool     `json:"found"`
	Version   string   `json:"version,omitempty"`
	ConfigDir string   `json:"config_dir,omitempty"`
	Warnings  []string `json:"warnings,omitempty"`
}

// RenderInput is the runtime-independent input an adapter receives after the
// config layer has resolved the active profile or environment.
type RenderInput struct {
	Active       ActiveRef                `json:"active"`
	Runtime      string                   `json:"runtime"`
	Agent        Agent                    `json:"agent"`
	Capabilities CapabilitySet            `json:"capabilities"`
	ProjectRoot  string                   `json:"project_root,omitempty"`
	ActiveDir    string                   `json:"active_dir,omitempty"`
	Boundary     boundary.RuntimeBoundary `json:"boundary,omitempty"`
	RuntimeHome  string                   `json:"runtime_home,omitempty"`
}

// ActiveRef identifies the active AVM object that produced a render input.
type ActiveRef struct {
	Kind string `json:"kind"` // profile | env
	Name string `json:"name"`
}

// Agent is the minimal adapter-facing projection of a config AgentProfile.
// It intentionally does not duplicate the full config schema.
type Agent struct {
	ID           string           `json:"id,omitempty"`
	Name         string           `json:"name"`
	Description  string           `json:"description,omitempty"`
	SourceScope  string           `json:"source_scope,omitempty"`
	Instructions Instructions     `json:"instructions,omitempty"`
	Model        ModelConfig      `json:"model,omitempty"`
	Permissions  PermissionConfig `json:"permissions,omitempty"`
}

// Instructions contains the instruction fields adapters need to map natively
// or render into runtime-specific guidance files.
type Instructions struct {
	System     string   `json:"system,omitempty"`
	Developer  string   `json:"developer,omitempty"`
	References []string `json:"references,omitempty"`
}

// ModelConfig is the adapter-facing subset of model runtime preferences.
type ModelConfig struct {
	Model           string   `json:"model,omitempty"`
	ReasoningEffort string   `json:"reasoning_effort,omitempty"`
	Verbosity       string   `json:"verbosity,omitempty"`
	Temperature     *float64 `json:"temperature,omitempty"`
}

// PermissionConfig is the adapter-facing subset of execution permissions.
type PermissionConfig struct {
	Approval              string   `json:"approval,omitempty"`
	Sandbox               string   `json:"sandbox,omitempty"`
	Allow                 []string `json:"allow,omitempty"`
	Deny                  []string `json:"deny,omitempty"`
	AdditionalDirectories []string `json:"additional_directories,omitempty"`
}

// CapabilitySet is the resolved capability payload an adapter can render.
type CapabilitySet struct {
	Skills     []CapabilityRef `json:"skills,omitempty"`
	MCPServers []MCPServer     `json:"mcp_servers,omitempty"`
	Commands   []CapabilityRef `json:"commands,omitempty"`
	Hooks      []CapabilityRef `json:"hooks,omitempty"`
	Toolsets   []Toolset       `json:"toolsets,omitempty"`
}

// CapabilityRef points to an already-resolved active capability.
type CapabilityRef struct {
	Name string `json:"name"`
	Path string `json:"path,omitempty"`
}

// MCPServer is a typed, portable MCP server definition.
type MCPServer struct {
	Name    string   `json:"name"`
	Command string   `json:"command,omitempty"`
	Args    []string `json:"args,omitempty"`
	Env     []EnvVar `json:"env,omitempty"`
	URL     string   `json:"url,omitempty"`
	Headers []EnvVar `json:"headers,omitempty"`
}

// EnvVar preserves environment variable references without expanding secrets.
type EnvVar struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Toolset describes a named runtime tool group and its desired mode.
type Toolset struct {
	Name string `json:"name"`
	Mode string `json:"mode"`
}

// RenderPlan is a deterministic description of the runtime writes an adapter
// intends to make.
type RenderPlan struct {
	Runtime      string            `json:"runtime"`
	Active       ActiveRef         `json:"active"`
	AgentName    string            `json:"agent_name"`
	ManagedPaths []ManagedPath     `json:"managed_paths,omitempty"`
	Operations   []RenderOperation `json:"operations,omitempty"`
	Mappings     []FieldMapping    `json:"mappings,omitempty"`
	Warnings     []string          `json:"warnings,omitempty"`
}

// RenderOperation is one file-system operation in a render plan.
type RenderOperation struct {
	ID          string       `json:"id,omitempty"`
	Action      RenderAction `json:"action"`
	Path        string       `json:"path"`
	Content     []byte       `json:"content,omitempty"`
	Description string       `json:"description,omitempty"`
	Required    bool         `json:"required"`
}

type RenderAction string

const (
	OperationWriteFile     RenderAction = "write_file"
	OperationEnsureDir     RenderAction = "ensure_dir"
	OperationMergeSection  RenderAction = "merge_section"
	OperationRemoveFile    RenderAction = "remove_file"
	OperationStructuredSet RenderAction = "structured_set"
)

// ManagedPath declares the runtime path ownership boundary used by sync,
// conflict detection, and backup.
type ManagedPath struct {
	Path        string    `json:"path"`
	Owner       string    `json:"owner"` // avm | shared-section
	Description string    `json:"description,omitempty"`
	Required    bool      `json:"required"`
	MergeMode   MergeMode `json:"merge_mode"`
}

type MergeMode string

const (
	MergeModeWholeFile         MergeMode = "whole-file"
	MergeModeMarkedBlock       MergeMode = "marked-block"
	MergeModeStructuredSection MergeMode = "structured-section"
)

// FieldMapping reports how one AVM field maps into one runtime field or file.
type FieldMapping struct {
	SourcePath string        `json:"source_path"`
	TargetPath string        `json:"target_path,omitempty"`
	Status     MappingStatus `json:"status"`
	Reason     string        `json:"reason,omitempty"`
}

type MappingStatus string

const (
	MappingNative                 MappingStatus = "native"
	MappingRenderedAsInstructions MappingStatus = "rendered_as_instructions"
	MappingIgnored                MappingStatus = "ignored"
	MappingUnsupported            MappingStatus = "unsupported"
)

// Valid reports whether s is one of the adapter contract mapping statuses.
func (s MappingStatus) Valid() bool {
	switch s {
	case MappingNative, MappingRenderedAsInstructions, MappingIgnored, MappingUnsupported:
		return true
	default:
		return false
	}
}

// RenderResult reports the result of applying a render plan.
type RenderResult struct {
	Runtime      string                  `json:"runtime"`
	Operations   []RenderOperationResult `json:"operations,omitempty"`
	ManagedPaths []ManagedPath           `json:"managed_paths,omitempty"`
	Mappings     []FieldMapping          `json:"mappings,omitempty"`
	Warnings     []string                `json:"warnings,omitempty"`
}

type RenderOperationResult struct {
	OperationID string       `json:"operation_id,omitempty"`
	Action      RenderAction `json:"action"`
	Path        string       `json:"path"`
	Changed     bool         `json:"changed"`
}
