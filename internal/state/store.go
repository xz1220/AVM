package state

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"github.com/xz1220/agent-vm/internal/config"
)

const SyncStateFilename = "sync-state.json"

func SyncStatePath() string {
	return filepath.Join(config.StateDir(), SyncStateFilename)
}

func LoadSyncState(path string) (SyncState, error) {
	if path == "" {
		path = SyncStatePath()
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return SyncState{}, err
	}

	var syncState SyncState
	if err := json.Unmarshal(raw, &syncState); err != nil {
		return SyncState{}, err
	}
	if syncState.Runtimes == nil {
		syncState.Runtimes = make(map[string]RuntimeState)
	}
	return syncState, nil
}

func SaveSyncState(path string, syncState SyncState) error {
	if path == "" {
		path = SyncStatePath()
	}
	if syncState.Version == "" {
		syncState.Version = StateVersion
	}
	if syncState.Runtimes == nil {
		syncState.Runtimes = make(map[string]RuntimeState)
	}

	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return err
	}

	raw, err := json.MarshalIndent(syncState, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')

	tmp, err := os.CreateTemp(parent, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

func LoadSyncStateOrNew(path string, active config.ActiveRef) (SyncState, error) {
	syncState, err := LoadSyncState(path)
	if err == nil {
		if syncState.Version == "" {
			syncState.Version = StateVersion
		}
		if syncState.Runtimes == nil {
			syncState.Runtimes = make(map[string]RuntimeState)
		}
		return syncState, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return NewSyncState(active), nil
	}
	return SyncState{}, err
}
