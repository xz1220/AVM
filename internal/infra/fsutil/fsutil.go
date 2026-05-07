// Package fsutil provides filesystem primitives shared across infra:
// atomic writes, backups, path safety checks, and directory scans.
package fsutil

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// EnsureWithin returns nil iff target is a subpath of root. It guards
// against zip-slip and similar path-escape attacks.
func EnsureWithin(root, target string) error {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(absRoot, absTarget)
	if err != nil {
		return err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return errors.New("fsutil: path escapes root")
	}
	return nil
}

// AtomicWriteFile writes data to path atomically: it writes a temp file
// in the same directory and then renames over the destination. The mode
// is applied after rename so it survives umask differences.
func AtomicWriteFile(path string, data []byte, mode fs.FileMode) error {
	if path == "" {
		return errors.New("fsutil: empty path")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".tmp-"+filepath.Base(path)+"-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	cleanup := func() {
		_ = os.Remove(tmp)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		cleanup()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		cleanup()
		return err
	}
	if err := f.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Chmod(tmp, mode); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		cleanup()
		return err
	}
	return nil
}

// Sha256Sum hashes r with SHA-256 and returns the lowercase hex digest.
func Sha256Sum(r io.Reader) (string, error) {
	if r == nil {
		return "", errors.New("fsutil: nil reader")
	}
	h := sha256.New()
	if _, err := io.Copy(h, r); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
