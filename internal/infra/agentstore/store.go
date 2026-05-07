// Package agentstore persists Agent definitions to disk under
// $AVM_HOME/agents/<name>.yaml. It implements AgentRepository.
package agentstore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/xz1220/agent-vm/internal/app/model"
	"github.com/xz1220/agent-vm/internal/infra/fsutil"
)

// Repository is the contract Application layer uses to load/save Agents.
// It does not know anything about runtimes — it just persists model.Agent.
type Repository interface {
	Get(name string) (*model.Agent, error)
	List() ([]model.AgentSummary, error)
	Save(agent *model.Agent) error
	Delete(name string) error
	Exists(name string) bool
	SourcePath(name string) (string, error)
}

// ErrNotFound is returned when an Agent does not exist.
var ErrNotFound = errors.New("agentstore: not found")

// ErrConflict is returned when Save would overwrite without permission.
var ErrConflict = errors.New("agentstore: already exists")

// FSRepo is the filesystem-backed Repository. Each Agent is stored as
// `<Dir>/<Identity.Name>.yaml`.
type FSRepo struct {
	Dir string
	// Overwrite, when true, allows Save to replace an existing Agent
	// file. Default (false) makes Save return ErrConflict if the file
	// already exists.
	Overwrite bool
}

// New returns an FSRepo bound to dir.
func New(dir string) *FSRepo { return &FSRepo{Dir: dir} }

func (r *FSRepo) pathFor(name string) string {
	return filepath.Join(r.Dir, name+".yaml")
}

// Get loads the Agent named `name` from disk.
func (r *FSRepo) Get(name string) (*model.Agent, error) {
	if name == "" {
		return nil, errors.New("agentstore: empty name")
	}
	p := r.pathFor(name)
	data, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var a model.Agent
	if err := yaml.Unmarshal(data, &a); err != nil {
		return nil, fmt.Errorf("agentstore: parse %s: %w", p, err)
	}
	return &a, nil
}

// List enumerates Agents in the store directory.
func (r *FSRepo) List() ([]model.AgentSummary, error) {
	entries, err := os.ReadDir(r.Dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]model.AgentSummary, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".yaml") {
			continue
		}
		base := strings.TrimSuffix(name, ".yaml")
		a, err := r.Get(base)
		if err != nil {
			// Skip unparseable files but continue.
			continue
		}
		runtimes := make([]string, 0, len(a.Runtimes))
		for _, rt := range a.Runtimes {
			runtimes = append(runtimes, rt.Runtime)
		}
		out = append(out, model.AgentSummary{
			Name:        a.Identity.Name,
			Description: a.Identity.Description,
			Runtimes:    runtimes,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Save validates and writes the Agent atomically. When r.Overwrite is
// false, an existing file at the destination causes ErrConflict.
func (r *FSRepo) Save(agent *model.Agent) error {
	if err := agent.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(r.Dir, 0o755); err != nil {
		return err
	}
	p := r.pathFor(agent.Identity.Name)
	if !r.Overwrite {
		if _, err := os.Stat(p); err == nil {
			return ErrConflict
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	data, err := yaml.Marshal(agent)
	if err != nil {
		return fmt.Errorf("agentstore: marshal: %w", err)
	}
	return fsutil.AtomicWriteFile(p, data, 0o644)
}

// Delete removes the named Agent.
func (r *FSRepo) Delete(name string) error {
	if name == "" {
		return errors.New("agentstore: empty name")
	}
	p := r.pathFor(name)
	if err := os.Remove(p); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrNotFound
		}
		return err
	}
	return nil
}

// Exists reports whether the named Agent file exists on disk.
func (r *FSRepo) Exists(name string) bool {
	if name == "" {
		return false
	}
	_, err := os.Stat(r.pathFor(name))
	return err == nil
}

// SourcePath returns the absolute on-disk path of the Agent definition.
func (r *FSRepo) SourcePath(name string) (string, error) {
	if name == "" {
		return "", errors.New("agentstore: empty name")
	}
	p := r.pathFor(name)
	if _, err := os.Stat(p); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", ErrNotFound
		}
		return "", err
	}
	return p, nil
}
