package state

import (
	"path/filepath"
	"testing"

	"github.com/xz1220/agent-vm/internal/config"
)

func TestSyncStateRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "sync-state.json")
	original := NewSyncState(config.ActiveRef{Kind: config.ActiveKindEnv, Name: "coding"})
	original.Runtimes["fake"] = RuntimeState{
		Runtime:   "fake",
		Status:    RuntimeStatusSynced,
		Active:    original.LastActive,
		AgentName: "backend",
		ManagedPaths: []ManagedPathState{
			{
				Path:        "/tmp/fake",
				Owner:       "avm",
				MergeMode:   "whole-file",
				FileHash:    "sha256:file",
				ManagedHash: "sha256:managed",
			},
		},
		Mappings: []MappingState{
			{SourcePath: "agent.name", TargetPath: "/tmp/fake#name", Status: "native"},
		},
	}

	if err := SaveSyncState(path, original); err != nil {
		t.Fatalf("save sync state: %v", err)
	}

	decoded, err := LoadSyncState(path)
	if err != nil {
		t.Fatalf("load sync state: %v", err)
	}
	if decoded.Version != StateVersion {
		t.Fatalf("decoded version = %q, want %q", decoded.Version, StateVersion)
	}
	if decoded.Runtimes["fake"].ManagedPaths[0].ManagedHash != "sha256:managed" {
		t.Fatalf("decoded managed hash mismatch: %#v", decoded.Runtimes["fake"].ManagedPaths)
	}
}
