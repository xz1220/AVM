package state

import (
	"time"

	"github.com/xz1220/agent-vm/internal/adapter"
	"github.com/xz1220/agent-vm/internal/config"
)

const StateVersion = "1"

type RuntimeStatus string

const (
	RuntimeStatusSynced  RuntimeStatus = "synced"
	RuntimeStatusSkipped RuntimeStatus = "skipped"
	RuntimeStatusFailed  RuntimeStatus = "failed"
	RuntimeStatusPartial RuntimeStatus = "partial"
)

type SyncState struct {
	Version    string                  `json:"version"`
	LastActive config.ActiveRef        `json:"last_active"`
	Runtimes   map[string]RuntimeState `json:"runtimes"`
	UpdatedAt  time.Time               `json:"updated_at"`
}

type RuntimeState struct {
	Runtime      string             `json:"runtime"`
	Status       RuntimeStatus      `json:"status"`
	Active       config.ActiveRef   `json:"active"`
	AgentName    string             `json:"agent_name"`
	RuntimeHome  string             `json:"runtime_home,omitempty"`
	ManagedPaths []ManagedPathState `json:"managed_paths,omitempty"`
	Mappings     []MappingState     `json:"mappings,omitempty"`
	Warnings     []string           `json:"warnings,omitempty"`
	Error        string             `json:"error,omitempty"`
	UpdatedAt    time.Time          `json:"updated_at"`
}

type ManagedPathState struct {
	Path        string `json:"path"`
	Owner       string `json:"owner"`
	MergeMode   string `json:"merge_mode"`
	FileHash    string `json:"file_hash,omitempty"`
	ManagedHash string `json:"managed_hash,omitempty"`
}

type MappingState struct {
	SourcePath string `json:"source_path"`
	TargetPath string `json:"target_path,omitempty"`
	Status     string `json:"status"`
	Reason     string `json:"reason,omitempty"`
}

func NewSyncState(active config.ActiveRef) SyncState {
	return SyncState{
		Version:    StateVersion,
		LastActive: active,
		Runtimes:   make(map[string]RuntimeState),
		UpdatedAt:  time.Now().UTC(),
	}
}

func ManagedPathStates(paths []adapter.ManagedPath) []ManagedPathState {
	states := make([]ManagedPathState, 0, len(paths))
	for _, path := range paths {
		states = append(states, ManagedPathState{
			Path:      path.Path,
			Owner:     path.Owner,
			MergeMode: string(path.MergeMode),
		})
	}
	return states
}

func MappingStates(mappings []adapter.FieldMapping) []MappingState {
	states := make([]MappingState, 0, len(mappings))
	for _, mapping := range mappings {
		states = append(states, MappingState{
			SourcePath: mapping.SourcePath,
			TargetPath: mapping.TargetPath,
			Status:     string(mapping.Status),
			Reason:     mapping.Reason,
		})
	}
	return states
}
