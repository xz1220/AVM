// Package packageio reads and writes .avm.zip packages: zip layout,
// manifest parsing, checksum verification, and path safety.
//
// Package layout (inside the zip):
//
//	manifest.yaml           — model.PackageManifest (root)
//	agents/<name>.yaml      — referenced from manifest.Agents[*].Path
//	capabilities/<...>      — referenced from manifest.Capabilities[*].Path
package packageio

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/xz1220/agent-vm/internal/app/model"
	"github.com/xz1220/agent-vm/internal/infra/fsutil"
)

// ManifestName is the well-known filename inside the package zip.
const ManifestName = "manifest.yaml"

// IO is the contract Application layer uses for package files.
type IO interface {
	Read(path string) (*model.PackageManifest, ReadHandle, error)
	Write(manifest *model.PackageManifest, payload io.Reader, dst string) error
	Verify(path string) error
}

// ReadHandle exposes lazy access to package contents after Read returns
// the manifest. Caller must Close.
type ReadHandle interface {
	Open(name string) (io.ReadCloser, error)
	Files() []string
	Close() error
}

// Default is the in-process default implementation.
type Default struct{}

func New() *Default { return &Default{} }

// Read opens the zip, parses the manifest, and returns a ReadHandle for
// lazy access to package members. Caller must Close the handle.
func (Default) Read(p string) (*model.PackageManifest, ReadHandle, error) {
	if p == "" {
		return nil, nil, errors.New("packageio: empty path")
	}
	zr, err := zip.OpenReader(p)
	if err != nil {
		return nil, nil, err
	}
	manifest, err := readManifest(&zr.Reader)
	if err != nil {
		_ = zr.Close()
		return nil, nil, err
	}
	h := &readHandle{zr: zr}
	for _, f := range zr.File {
		h.names = append(h.names, f.Name)
	}
	return manifest, h, nil
}

// Write serializes manifest and copies payload (a zip stream) into dst.
// payload may be nil — callers can write a manifest-only package, but
// typical use is to pass a *bytes.Reader containing the payload zip
// stream produced earlier.
//
// To keep packageio focused, the simpler contract is: payload, if
// non-nil, must be a zip archive whose entries are merged into the
// output zip alongside the manifest. We intentionally do not allow
// "manifest-only" without payload because that would lose all referenced
// files.
func (Default) Write(manifest *model.PackageManifest, payload io.Reader, dst string) error {
	if manifest == nil {
		return errors.New("packageio: nil manifest")
	}
	if dst == "" {
		return errors.New("packageio: empty dst")
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	manifestBytes, err := yaml.Marshal(manifest)
	if err != nil {
		return fmt.Errorf("packageio: marshal manifest: %w", err)
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	// Write manifest first.
	mf, err := zw.Create(ManifestName)
	if err != nil {
		return err
	}
	if _, err := mf.Write(manifestBytes); err != nil {
		return err
	}

	// Merge payload entries if provided.
	if payload != nil {
		payloadBytes, err := io.ReadAll(payload)
		if err != nil {
			return err
		}
		zr, err := zip.NewReader(bytes.NewReader(payloadBytes), int64(len(payloadBytes)))
		if err != nil {
			return fmt.Errorf("packageio: payload not a zip: %w", err)
		}
		for _, f := range zr.File {
			if f.Name == ManifestName {
				continue // never let payload override our manifest
			}
			if !path.IsAbs(f.Name) && !strings.Contains(f.Name, "..") {
				w, err := zw.Create(f.Name)
				if err != nil {
					return err
				}
				rc, err := f.Open()
				if err != nil {
					return err
				}
				if _, err := io.Copy(w, rc); err != nil {
					_ = rc.Close()
					return err
				}
				_ = rc.Close()
			}
		}
	}

	if err := zw.Close(); err != nil {
		return err
	}
	return fsutil.AtomicWriteFile(dst, buf.Bytes(), 0o644)
}

// Verify checks SHA-256 for every PackageCapBlob.Path in the manifest
// and ensures every member name is a safe relative path.
func (Default) Verify(p string) error {
	if p == "" {
		return errors.New("packageio: empty path")
	}
	zr, err := zip.OpenReader(p)
	if err != nil {
		return err
	}
	defer zr.Close()

	manifest, err := readManifest(&zr.Reader)
	if err != nil {
		return err
	}

	// Path safety on every member relative to a virtual root.
	root := "/__pkg__"
	for _, f := range zr.File {
		joined := filepath.Join(root, f.Name)
		if err := fsutil.EnsureWithin(root, joined); err != nil {
			return fmt.Errorf("packageio: unsafe member %q: %w", f.Name, err)
		}
	}

	for _, blob := range manifest.Capabilities {
		if blob.Path == "" {
			return fmt.Errorf("packageio: capability %q has empty path", blob.Name)
		}
		f, err := openByName(&zr.Reader, blob.Path)
		if err != nil {
			return fmt.Errorf("packageio: capability %q: %w", blob.Name, err)
		}
		sum, err := fsutil.Sha256Sum(f)
		_ = f.Close()
		if err != nil {
			return err
		}
		if !strings.EqualFold(sum, blob.Checksum) {
			return fmt.Errorf("packageio: capability %q checksum mismatch: got %s want %s", blob.Name, sum, blob.Checksum)
		}
	}
	return nil
}

func readManifest(zr *zip.Reader) (*model.PackageManifest, error) {
	rc, err := openByName(zr, ManifestName)
	if err != nil {
		return nil, fmt.Errorf("packageio: manifest: %w", err)
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}
	var m model.PackageManifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("packageio: parse manifest: %w", err)
	}
	return &m, nil
}

func openByName(zr *zip.Reader, name string) (io.ReadCloser, error) {
	for _, f := range zr.File {
		if f.Name == name {
			return f.Open()
		}
	}
	return nil, fmt.Errorf("not found in package: %s", name)
}

type readHandle struct {
	zr    *zip.ReadCloser
	names []string
}

func (h *readHandle) Open(name string) (io.ReadCloser, error) {
	if h == nil || h.zr == nil {
		return nil, errors.New("packageio: handle closed")
	}
	return openByName(&h.zr.Reader, name)
}

func (h *readHandle) Files() []string {
	if h == nil {
		return nil
	}
	out := make([]string, len(h.names))
	copy(out, h.names)
	return out
}

func (h *readHandle) Close() error {
	if h == nil || h.zr == nil {
		return nil
	}
	err := h.zr.Close()
	h.zr = nil
	return err
}
