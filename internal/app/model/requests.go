package model

// CreateAgentRequest captures the user's intent to create an Agent.
// Source describes blank/default vs existing-package origin (see PRD §4.2).
type CreateAgentRequest struct {
	Name         string             `json:"name"`
	Description  string             `json:"description,omitempty"`
	Role         string             `json:"role,omitempty"`
	Instructions Instructions       `json:"instructions,omitempty"`
	Skills       []CapabilityRef    `json:"skills,omitempty"`
	MCP          []CapabilityRef    `json:"mcp,omitempty"`
	Runtimes     []RuntimePref      `json:"runtimes,omitempty"`
	Source       CreateSource       `json:"source,omitempty"`
	OnConflict   ConflictResolution `json:"on_conflict,omitempty"`
}

// CreateSource enumerates create origins per PRD §4.2.
type CreateSource string

const (
	CreateSourceBlank   CreateSource = "blank"
	CreateSourceDefault CreateSource = "default"
	CreateSourcePackage CreateSource = "package"
)

// EditAgentRequest captures partial-edit intent. Nil pointers mean
// "keep existing"; non-nil pointers replace the field.
type EditAgentRequest struct {
	Name         string           `json:"name"`
	Identity     *Identity        `json:"identity,omitempty"`
	Instructions *Instructions    `json:"instructions,omitempty"`
	Skills       *[]CapabilityRef `json:"skills,omitempty"`
	MCP          *[]CapabilityRef `json:"mcp,omitempty"`
	Runtimes     *[]RuntimePref   `json:"runtimes,omitempty"`
}

// DeleteAgentRequest controls deletion behavior.
type DeleteAgentRequest struct {
	Name    string `json:"name"`
	Confirm bool   `json:"confirm,omitempty"`
}

// DiscoverRequest scopes a capability discovery call.
type DiscoverRequest struct {
	Kinds    []CapabilityKind `json:"kinds,omitempty"`    // empty = all
	Runtimes []string         `json:"runtimes,omitempty"` // empty = all detected
}

// ImportCapabilityRequest captures the user's intent to import one
// runtime-global capability into the AVM capability store.
type ImportCapabilityRequest struct {
	Runtime    string             `json:"runtime"`
	Kind       CapabilityKind     `json:"kind"`
	Name       string             `json:"name"`
	OnConflict ConflictResolution `json:"on_conflict,omitempty"` // skip|overwrite|cancel; empty = cancel
}

// ImportCapabilityResult describes the outcome of a single import.
type ImportCapabilityResult struct {
	ID       CapabilityID `json:"id"`
	Created  bool         `json:"created"`            // false = dedup or skipped
	Replaced bool         `json:"replaced,omitempty"` // true = OnConflict=overwrite kicked in
	Source   string       `json:"source,omitempty"`   // "<runtime>:<original-path>"
}

// BootstrapCapabilitiesRequest asks AVM to import every runtime-global
// capability the named runtime currently exposes via DiscoverGlobal.
type BootstrapCapabilitiesRequest struct {
	Runtime    string             `json:"runtime"`
	Kinds      []CapabilityKind   `json:"kinds,omitempty"`       // empty = all (skill + mcp)
	OnConflict ConflictResolution `json:"on_conflict,omitempty"` // applied uniformly
}

// BootstrapCapabilitiesResult aggregates per-candidate outcomes. Single
// failures land in Skipped and never abort the whole bootstrap.
type BootstrapCapabilitiesResult struct {
	Imported []ImportCapabilityResult `json:"imported,omitempty"`
	Skipped  []SkippedCapability      `json:"skipped,omitempty"`
}

// SkippedCapability records a (kind,name) that bootstrap did not import,
// with a human-readable reason.
type SkippedCapability struct {
	Kind   CapabilityKind `json:"kind"`
	Name   string         `json:"name"`
	Reason string         `json:"reason"`
}
