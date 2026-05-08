package model

// CapabilityKind enumerates the kinds of capabilities AVM understands.
// Future kinds (commands, hooks, toolsets) are added here.
type CapabilityKind string

const (
	CapabilityKindSkill CapabilityKind = "skill"
	CapabilityKindMCP   CapabilityKind = "mcp"
)

// CapabilityID is the stable identity of a capability inside the AVM
// capability store. Agents reference capabilities by ID; package names,
// import sources, and checksums are audit metadata, not identity.
type CapabilityID string

// CapabilitySource describes where a discovered capability came from.
// AVM must show this to users at create/edit time so users can tell
// AVM-managed capabilities apart from runtime global discoveries.
type CapabilitySource string

const (
	SourceAVM           CapabilitySource = "avm"     // managed by AVM capability store
	SourcePackage       CapabilitySource = "package" // installed via AVM package
	SourceRuntimeGlobal CapabilitySource = "runtime" // discovered in runtime global dir
)

// PayloadFormat is the contract between the writer of a capability and
// any reader / runtime materializer about how to interpret the bytes
// stored in capstore. Format also drives the on-disk filename inside
// the capstore payload directory (e.g. SkillMD → "SKILL.md").
//
// Empty Format keeps backward compatibility with pre-import records
// and means "use the capability Name as the filename".
const (
	PayloadFormatSkillMD     = "skill_md"      // a single SKILL.md file (skills)
	PayloadFormatMCPConfigV1 = "mcp_config_v1" // canonical AVM MCP JSON (see runtime drivers)
)

// CapabilityRecord is an entry in the AVM capability store.
type CapabilityRecord struct {
	ID         CapabilityID     `json:"id"`
	Kind       CapabilityKind   `json:"kind"`
	Name       string           `json:"name"`
	Version    string           `json:"version,omitempty"`
	Source     CapabilitySource `json:"source"`
	Checksum   string           `json:"checksum,omitempty"`
	ImportFrom string           `json:"import_from,omitempty"` // optional audit: package name or runtime path
	// Format declares how the payload bytes should be interpreted.
	// See PayloadFormat* constants. Empty = legacy "filename = Name".
	Format string `json:"format,omitempty"`
}

// GlobalCapability is what a runtime reports during global discovery.
// It does not enter Agent references directly — users must explicitly
// import it into the AVM capability store first.
type GlobalCapability struct {
	Runtime string         `json:"runtime"`
	Kind    CapabilityKind `json:"kind"`
	Name    string         `json:"name"`
	Path    string         `json:"path,omitempty"`
	Version string         `json:"version,omitempty"`
}

// CapabilityCandidate is what AVM presents to users at create/edit time.
// It unifies AVM-managed records with runtime global discoveries and
// must always carry a Source so the UI can explain provenance.
type CapabilityCandidate struct {
	Kind     CapabilityKind    `json:"kind"`
	Name     string            `json:"name"`
	Source   CapabilitySource  `json:"source"`
	Record   *CapabilityRecord `json:"record,omitempty"` // set when Source != SourceRuntimeGlobal
	Global   *GlobalCapability `json:"global,omitempty"` // set when Source == SourceRuntimeGlobal
	Conflict bool              `json:"conflict,omitempty"`
	// Imported is set on runtime-global candidates when an AVM-managed
	// record with the same (kind, name) already exists in capstore.
	// UI can use this to suppress redundant import prompts.
	Imported bool `json:"imported,omitempty"`
}
