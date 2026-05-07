// Package capstore is the AVM capability store. It owns the durable
// content of skills/MCPs that Agents reference by stable ID. Package
// installs and explicit imports route through this store; runtime
// global discoveries do NOT enter here automatically.
package capstore

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/xz1220/agent-vm/internal/app/model"
	"github.com/xz1220/agent-vm/internal/infra/fsutil"
)

// Store is the contract for AVM capability persistence.
type Store interface {
	List() ([]model.CapabilityRecord, error)
	Get(id model.CapabilityID) (model.CapabilityRecord, error)
	// Add writes payload under a new ID derived from the record. If
	// the same name+kind already exists with identical content,
	// implementations may dedupe and return the existing ID.
	Add(record model.CapabilityRecord, payload io.Reader) (model.CapabilityID, error)
	// Materialize prepares the given capabilities under target dir
	// (e.g. via symlink) so a runtime boundary can reference them.
	Materialize(ids []model.CapabilityID, target string) error
	Remove(id model.CapabilityID) error
}

// ErrNotFound is returned when an ID is unknown.
var ErrNotFound = errors.New("capstore: not found")

// FSStore is the filesystem-backed Store.
//
// Layout under Dir:
//
//	<id>/manifest.yaml   — CapabilityRecord (plus content checksum)
//	<id>/payload/<name>  — capability bytes
type FSStore struct{ Dir string }

func New(dir string) *FSStore { return &FSStore{Dir: dir} }

// recordFile is what we serialize to manifest.yaml. We keep it separate
// from the model type to control YAML field names.
type recordFile struct {
	ID         string `yaml:"id"`
	Kind       string `yaml:"kind"`
	Name       string `yaml:"name"`
	Version    string `yaml:"version,omitempty"`
	Source     string `yaml:"source,omitempty"`
	Checksum   string `yaml:"checksum"`
	ImportFrom string `yaml:"import_from,omitempty"`
	// PayloadFile is the on-disk filename inside the payload/ subdir.
	PayloadFile string `yaml:"payload_file"`
}

func toRecordFile(r model.CapabilityRecord, payloadFile string) recordFile {
	return recordFile{
		ID:          string(r.ID),
		Kind:        string(r.Kind),
		Name:        r.Name,
		Version:     r.Version,
		Source:      string(r.Source),
		Checksum:    r.Checksum,
		ImportFrom:  r.ImportFrom,
		PayloadFile: payloadFile,
	}
}

func (rf recordFile) toModel() model.CapabilityRecord {
	return model.CapabilityRecord{
		ID:         model.CapabilityID(rf.ID),
		Kind:       model.CapabilityKind(rf.Kind),
		Name:       rf.Name,
		Version:    rf.Version,
		Source:     model.CapabilitySource(rf.Source),
		Checksum:   rf.Checksum,
		ImportFrom: rf.ImportFrom,
	}
}

func (s *FSStore) idDir(id model.CapabilityID) string {
	return filepath.Join(s.Dir, string(id))
}

func (s *FSStore) manifestPath(id model.CapabilityID) string {
	return filepath.Join(s.idDir(id), "manifest.yaml")
}

func (s *FSStore) payloadDir(id model.CapabilityID) string {
	return filepath.Join(s.idDir(id), "payload")
}

func deriveID(kind model.CapabilityKind, name, contentSum string) model.CapabilityID {
	h := sha256.Sum256([]byte(string(kind) + "\n" + name + "\n" + contentSum))
	return model.CapabilityID("cap_" + hex.EncodeToString(h[:])[:32])
}

// List enumerates every record under Dir.
func (s *FSStore) List() ([]model.CapabilityRecord, error) {
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]model.CapabilityRecord, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		rec, err := s.Get(model.CapabilityID(e.Name()))
		if err != nil {
			continue
		}
		out = append(out, rec)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

// Get loads a single record by ID.
func (s *FSStore) Get(id model.CapabilityID) (model.CapabilityRecord, error) {
	if id == "" {
		return model.CapabilityRecord{}, ErrNotFound
	}
	data, err := os.ReadFile(s.manifestPath(id))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return model.CapabilityRecord{}, ErrNotFound
		}
		return model.CapabilityRecord{}, err
	}
	var rf recordFile
	if err := yaml.Unmarshal(data, &rf); err != nil {
		return model.CapabilityRecord{}, fmt.Errorf("capstore: parse %s: %w", s.manifestPath(id), err)
	}
	return rf.toModel(), nil
}

// Add stores payload under a new ID derived from (kind,name,sha256(content)).
// Identical (kind,name,content) returns the existing ID.
func (s *FSStore) Add(rec model.CapabilityRecord, payload io.Reader) (model.CapabilityID, error) {
	if rec.Kind == "" || rec.Name == "" {
		return "", errors.New("capstore: kind and name are required")
	}
	if payload == nil {
		return "", errors.New("capstore: nil payload")
	}
	buf, err := io.ReadAll(payload)
	if err != nil {
		return "", err
	}
	contentSum, err := fsutil.Sha256Sum(bytes.NewReader(buf))
	if err != nil {
		return "", err
	}
	id := deriveID(rec.Kind, rec.Name, contentSum)

	// Idempotent: if it already exists with the same checksum, return it.
	if existing, err := s.Get(id); err == nil {
		if existing.Checksum == contentSum {
			return id, nil
		}
		// Same ID but different checksum should be impossible because
		// the ID embeds the checksum, but be defensive.
		return "", fmt.Errorf("capstore: id %s exists with different checksum", id)
	} else if !errors.Is(err, ErrNotFound) {
		return "", err
	}

	if err := os.MkdirAll(s.payloadDir(id), 0o755); err != nil {
		return "", err
	}
	payloadName := rec.Name
	if payloadName == "" {
		payloadName = string(id)
	}
	payloadPath := filepath.Join(s.payloadDir(id), payloadName)
	if err := fsutil.AtomicWriteFile(payloadPath, buf, 0o644); err != nil {
		return "", err
	}

	full := rec
	full.ID = id
	full.Checksum = contentSum
	rf := toRecordFile(full, payloadName)
	manifestBytes, err := yaml.Marshal(rf)
	if err != nil {
		return "", err
	}
	if err := fsutil.AtomicWriteFile(s.manifestPath(id), manifestBytes, 0o644); err != nil {
		return "", err
	}
	return id, nil
}

// Materialize creates entries under target so the runtime can read the
// capability payloads. We prefer symlinks; if the OS forbids them
// (e.g. unprivileged Windows), we fall back to copying.
func (s *FSStore) Materialize(ids []model.CapabilityID, target string) error {
	if target == "" {
		return errors.New("capstore: empty target")
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		return err
	}
	for _, id := range ids {
		rec, err := s.Get(id)
		if err != nil {
			return fmt.Errorf("capstore: %s: %w", id, err)
		}
		// Discover the payload file: read manifest to learn the name.
		var rf recordFile
		data, err := os.ReadFile(s.manifestPath(id))
		if err != nil {
			return err
		}
		if err := yaml.Unmarshal(data, &rf); err != nil {
			return err
		}
		src := filepath.Join(s.payloadDir(id), rf.PayloadFile)
		// One subdir per capability kind to give the runtime a stable
		// surface (e.g. target/skills/<name>).
		kindDir := filepath.Join(target, string(rec.Kind)+"s")
		if err := os.MkdirAll(kindDir, 0o755); err != nil {
			return err
		}
		dst := filepath.Join(kindDir, rf.PayloadFile)
		if err := fsutil.EnsureWithin(target, dst); err != nil {
			return err
		}
		// Remove any existing entry first to make Materialize idempotent.
		_ = os.Remove(dst)
		if err := os.Symlink(src, dst); err != nil {
			// Fall back to copy.
			if cerr := copyFile(src, dst); cerr != nil {
				return fmt.Errorf("capstore: materialize %s: symlink=%v copy=%v", id, err, cerr)
			}
		}
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	data, err := io.ReadAll(in)
	if err != nil {
		return err
	}
	return fsutil.AtomicWriteFile(dst, data, 0o644)
}

// Remove deletes the capability directory.
func (s *FSStore) Remove(id model.CapabilityID) error {
	if id == "" {
		return ErrNotFound
	}
	dir := s.idDir(id)
	if _, err := os.Stat(dir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrNotFound
		}
		return err
	}
	return os.RemoveAll(dir)
}
