// Package managedfile applies runtime.Plan files to disk. It is the
// only place AVM writes runtime-managed config; no other layer pokes
// at runtime config paths directly.
package managedfile

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"

	"github.com/xz1220/agent-vm/internal/app/model"
	"github.com/xz1220/agent-vm/internal/infra/fsutil"
	"github.com/xz1220/agent-vm/internal/runtime"
)

// Writer applies a runtime Plan and reports drift between desired and
// existing files.
type Writer interface {
	Apply(ctx context.Context, files []runtime.ManagedFile) ([]model.DiffEntry, error)
	DryRun(ctx context.Context, files []runtime.ManagedFile) ([]model.DiffEntry, error)
}

// Default is an atomic-write-backed implementation.
type Default struct{}

func New() *Default { return &Default{} }

// Apply writes each ManagedFile atomically and reports a DiffEntry for
// every file that was created or whose contents changed.
func (Default) Apply(ctx context.Context, files []runtime.ManagedFile) ([]model.DiffEntry, error) {
	var diffs []model.DiffEntry
	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return diffs, err
		}
		if f.Path == "" {
			return diffs, errors.New("managedfile: empty path")
		}
		reason, ok, err := classify(f)
		if err != nil {
			return diffs, err
		}
		mode := f.Mode
		if mode == 0 {
			mode = 0o644
		}
		if err := fsutil.AtomicWriteFile(f.Path, f.Contents, mode); err != nil {
			return diffs, err
		}
		if ok {
			diffs = append(diffs, model.DiffEntry{Path: f.Path, Reason: reason})
		}
	}
	return diffs, nil
}

// DryRun reports what Apply would change but does not write anything.
func (Default) DryRun(ctx context.Context, files []runtime.ManagedFile) ([]model.DiffEntry, error) {
	var diffs []model.DiffEntry
	for _, f := range files {
		if err := ctx.Err(); err != nil {
			return diffs, err
		}
		if f.Path == "" {
			return diffs, errors.New("managedfile: empty path")
		}
		reason, ok, err := classify(f)
		if err != nil {
			return diffs, err
		}
		if ok {
			diffs = append(diffs, model.DiffEntry{Path: f.Path, Reason: reason})
		}
	}
	return diffs, nil
}

// classify reports whether the on-disk contents would change. The
// boolean is true when a DiffEntry should be emitted (created/updated);
// it is false when the file already matches the desired contents.
func classify(f runtime.ManagedFile) (string, bool, error) {
	existing, err := os.ReadFile(f.Path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "created", true, nil
		}
		return "", false, err
	}
	if bytes.Equal(existing, f.Contents) {
		return "", false, nil
	}
	return "updated", true, nil
}
