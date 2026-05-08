package runtime

import (
	"context"
	"errors"
	"io"

	"github.com/xz1220/agent-vm/internal/app/model"
)

// Driver is the per-runtime aggregate entry point. The application
// layer only depends on this interface and Registry; how a driver
// internally splits facts/adapter/boundary/launcher is its own detail.
type Driver interface {
	// Name returns the canonical runtime name (codex, claude-code, opencode...).
	Name() string

	// Facts probes runtime presence/version/capabilities. Cheap and
	// safe to call multiple times; drivers may cache internally.
	Facts(ctx context.Context) (Facts, error)

	// DiscoverGlobal returns capabilities found in the runtime's own
	// global directory (e.g. ~/.codex/skills). These are NOT AVM
	// managed; application layer must show their source clearly.
	DiscoverGlobal(ctx context.Context) ([]model.GlobalCapability, error)

	// ExportGlobal serializes a single runtime-global capability into
	// AVM canonical form so the application layer can copy it into the
	// AVM capability store. The returned Content stream is owned by
	// the caller and MUST be closed.
	//
	// Drivers translate runtime-native shapes into a canonical form:
	//   - skills (file-based, all runtimes): bytes of SKILL.md →
	//     Format = model.PayloadFormatSkillMD, Filename = "SKILL.md"
	//   - mcp (config-fragment): an AVM mcp_config_v1 JSON document
	//     with at least {kind,name,command,args,env,extra} →
	//     Format = model.PayloadFormatMCPConfigV1, Filename = "mcp.json"
	//
	// If the named (kind,name) is not present in this runtime's
	// global discovery, drivers must return ErrGlobalCapabilityNotFound.
	ExportGlobal(ctx context.Context, kind model.CapabilityKind, name string) (Exported, error)

	// Plan renders the Agent into the runtime's managed config.
	Plan(ctx context.Context, agent *model.Agent) (*Plan, error)

	// Boundary returns the per-Agent×runtime isolation boundary.
	Boundary(ctx context.Context, agent *model.Agent) (Boundary, error)

	// LaunchSpec describes how to spawn the runtime for this Agent.
	LaunchSpec(ctx context.Context, agent *model.Agent, plan *Plan) (LaunchSpec, error)
}

// Exported is the result of Driver.ExportGlobal — a canonical
// representation of one runtime-global capability the application
// layer can stream straight into capstore.Add.
type Exported struct {
	// Capability carries the original metadata DiscoverGlobal would
	// have surfaced (Path, Version, Runtime).
	Capability model.GlobalCapability
	// Format names the on-disk shape of Content. See model.PayloadFormat*.
	Format string
	// Content is the canonical bytes. Caller closes.
	Content io.ReadCloser
	// Filename is the suggested payload filename inside capstore
	// (e.g. "SKILL.md" or "mcp.json").
	Filename string
}

// ErrGlobalCapabilityNotFound is returned by Driver.ExportGlobal when
// the requested (kind,name) is not present in the runtime's global
// surface. Callers can errors.Is against this sentinel.
var ErrGlobalCapabilityNotFound = errors.New("runtime: global capability not found")

// MCPConfigV1 is the canonical AVM MCP capability content. Drivers
// translate runtime-native MCP definitions into this shape when
// implementing ExportGlobal; capstore stores the JSON bytes verbatim.
//
// The Extra map preserves runtime-specific fields the canonical schema
// doesn't yet cover, so we don't lose information across an import →
// export cycle. v2 may promote stable Extra keys to first-class fields.
type MCPConfigV1 struct {
	Kind    string            `json:"kind"` // always "mcp"
	Name    string            `json:"name"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	Extra   map[string]any    `json:"extra,omitempty"`
}
