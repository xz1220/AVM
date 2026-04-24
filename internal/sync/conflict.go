package sync

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/xz1220/agent-vm/internal/adapter"
	"github.com/xz1220/agent-vm/internal/state"
)

type Conflict struct {
	Path    string
	Runtime string
	Reason  string
}

func DetectConflicts(runtime string, paths []adapter.ManagedPath, runtimeState state.RuntimeState) ([]Conflict, error) {
	previous := make(map[string]state.ManagedPathState, len(runtimeState.ManagedPaths))
	for _, managedPath := range runtimeState.ManagedPaths {
		if managedPath.Path != "" {
			previous[filepath.Clean(managedPath.Path)] = managedPath
		}
	}

	conflicts := make([]Conflict, 0)
	for _, managedPath := range paths {
		if managedPath.Path == "" {
			continue
		}
		cleanPath := filepath.Clean(managedPath.Path)
		prior, ok := previous[cleanPath]
		if !ok {
			continue
		}
		if _, err := os.Lstat(cleanPath); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}

		fileHash, managedHash, err := hashManagedPath(managedPath)
		if err != nil {
			return nil, err
		}
		expected := prior.ManagedHash
		actual := managedHash
		if expected == "" || actual == "" {
			expected = prior.FileHash
			actual = fileHash
		}
		if expected != "" && actual != "" && expected != actual {
			conflicts = append(conflicts, Conflict{
				Path:    cleanPath,
				Runtime: runtime,
				Reason:  "managed path was modified outside AVM",
			})
		}
	}
	return conflicts, nil
}

func ManagedPathStatesWithHashes(paths []adapter.ManagedPath) ([]state.ManagedPathState, error) {
	states := make([]state.ManagedPathState, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, managedPath := range paths {
		if managedPath.Path == "" {
			continue
		}
		cleanPath := filepath.Clean(managedPath.Path)
		if _, ok := seen[cleanPath]; ok {
			continue
		}
		seen[cleanPath] = struct{}{}

		fileHash, managedHash, err := hashManagedPath(managedPath)
		if err != nil {
			if os.IsNotExist(err) {
				fileHash = ""
				managedHash = ""
			} else {
				return nil, err
			}
		}
		states = append(states, state.ManagedPathState{
			Path:        cleanPath,
			Owner:       managedPath.Owner,
			MergeMode:   string(managedPath.MergeMode),
			FileHash:    fileHash,
			ManagedHash: managedHash,
		})
	}
	return states, nil
}

func conflictError(conflicts []Conflict) error {
	if len(conflicts) == 0 {
		return nil
	}
	parts := make([]string, 0, len(conflicts))
	for _, conflict := range conflicts {
		parts = append(parts, fmt.Sprintf("%s: %s", conflict.Path, conflict.Reason))
	}
	sort.Strings(parts)
	return fmt.Errorf("conflict detected: %s", strings.Join(parts, "; "))
}

func hashManagedPath(managedPath adapter.ManagedPath) (string, string, error) {
	fileHash, content, err := hashPath(managedPath.Path)
	if err != nil {
		return "", "", err
	}

	switch managedPath.MergeMode {
	case adapter.MergeModeMarkedBlock:
		block, ok := avmManagedBlock(content)
		if ok {
			return fileHash, sha256String(block), nil
		}
		return fileHash, fileHash, nil
	case adapter.MergeModeStructuredSection:
		return fileHash, fileHash, nil
	default:
		return fileHash, fileHash, nil
	}
}

func hashPath(path string) (string, []byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", nil, err
	}
	if info.IsDir() {
		raw, err := hashDir(path)
		if err != nil {
			return "", nil, err
		}
		return sha256Bytes(raw), raw, nil
	}
	if info.Mode()&os.ModeSymlink != 0 {
		link, err := os.Readlink(path)
		if err != nil {
			return "", nil, err
		}
		raw := []byte("symlink " + link)
		return sha256Bytes(raw), raw, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", nil, err
	}
	return sha256Bytes(raw), raw, nil
}

func hashDir(root string) ([]byte, error) {
	var builder strings.Builder
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			builder.WriteString("symlink ")
			builder.WriteString(rel)
			builder.WriteByte(0)
			builder.WriteString(link)
			builder.WriteByte(0)
		case entry.IsDir():
			builder.WriteString("dir ")
			builder.WriteString(rel)
			builder.WriteByte(0)
		case info.Mode().IsRegular():
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			builder.WriteString("file ")
			builder.WriteString(rel)
			builder.WriteByte(0)
			builder.WriteString(sha256Bytes(raw))
			builder.WriteByte(0)
		}
		return nil
	})
	return []byte(builder.String()), err
}

func avmManagedBlock(content []byte) ([]byte, bool) {
	text := string(content)
	begin := strings.Index(text, "BEGIN AVM MANAGED")
	if begin < 0 {
		return nil, false
	}
	beginLineEnd := strings.IndexByte(text[begin:], '\n')
	if beginLineEnd < 0 {
		return nil, false
	}
	start := begin + beginLineEnd + 1
	end := strings.Index(text[start:], "END AVM MANAGED")
	if end < 0 {
		return nil, false
	}
	return []byte(text[start : start+end]), true
}

func sha256Bytes(raw []byte) string {
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func sha256String(value []byte) string {
	return sha256Bytes(value)
}
