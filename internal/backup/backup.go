package backup

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/xz1220/agent-vm/internal/adapter"
	"github.com/xz1220/agent-vm/internal/config"
)

type Snapshot struct {
	Path      string
	BackedUp  []string
	CreatedAt time.Time
}

func BackupManagedPaths(runtime string, paths []adapter.ManagedPath, backupRoot string, now time.Time) (*Snapshot, error) {
	if backupRoot == "" {
		backupRoot = config.BackupDir()
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	existing := existingManagedPaths(paths)
	if len(existing) == 0 {
		return nil, nil
	}

	snapshotDir := filepath.Join(backupRoot, now.UTC().Format("20060102T150405.000000000Z"), sanitizePathPart(runtime))
	if err := os.MkdirAll(snapshotDir, 0o700); err != nil {
		return nil, err
	}

	snapshot := &Snapshot{
		Path:      snapshotDir,
		CreatedAt: now.UTC(),
	}
	for _, managedPath := range existing {
		target := filepath.Join(snapshotDir, backupRelativePath(managedPath.Path))
		if err := copyPath(managedPath.Path, target); err != nil {
			return nil, err
		}
		snapshot.BackedUp = append(snapshot.BackedUp, managedPath.Path)
	}
	return snapshot, nil
}

func existingManagedPaths(paths []adapter.ManagedPath) []adapter.ManagedPath {
	existing := make([]adapter.ManagedPath, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, managedPath := range paths {
		if managedPath.Path == "" {
			continue
		}
		clean := filepath.Clean(managedPath.Path)
		if _, ok := seen[clean]; ok {
			continue
		}
		if _, err := os.Lstat(clean); err == nil {
			managedPath.Path = clean
			existing = append(existing, managedPath)
			seen[clean] = struct{}{}
		}
	}
	return existing
}

func backupRelativePath(path string) string {
	path = filepath.Clean(path)
	if volume := filepath.VolumeName(path); volume != "" {
		path = strings.TrimPrefix(path, volume)
	}
	path = strings.TrimLeft(path, string(filepath.Separator))
	if path == "." || path == "" {
		return "_"
	}
	return path
}

func copyPath(source, target string) error {
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}

	switch {
	case info.Mode()&os.ModeSymlink != 0:
		link, err := os.Readlink(source)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		return os.Symlink(link, target)
	case info.IsDir():
		return copyDir(source, target, info.Mode().Perm())
	case info.Mode().IsRegular():
		return copyFile(source, target, info.Mode().Perm())
	default:
		return fmt.Errorf("cannot backup unsupported path type %s", source)
	}
}

func copyDir(source, target string, mode os.FileMode) error {
	if err := os.MkdirAll(target, mode); err != nil {
		return err
	}
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := copyPath(filepath.Join(source, entry.Name()), filepath.Join(target, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(source, target string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return err
	}

	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func sanitizePathPart(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "_"
	}
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_")
	return replacer.Replace(value)
}
