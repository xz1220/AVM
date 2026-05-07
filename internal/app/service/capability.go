package service

import (
	"context"
	"errors"
	"sort"

	"github.com/xz1220/agent-vm/internal/app/model"
	"github.com/xz1220/agent-vm/internal/infra/capstore"
	"github.com/xz1220/agent-vm/internal/runtime"
)

// CapabilityService unifies AVM-managed and runtime-global capability
// candidates per PRD §4.2. The list MUST be live: every call must
// reflect runtime-global state at the moment the user sees it.
type CapabilityService interface {
	Discover(ctx context.Context, req model.DiscoverRequest) ([]model.CapabilityCandidate, error)
}

// Capabilities is the default CapabilityService.
type Capabilities struct {
	Store    capstore.Store
	Runtimes runtime.Registry
}

func NewCapabilities(store capstore.Store, registry runtime.Registry) *Capabilities {
	return &Capabilities{Store: store, Runtimes: registry}
}

// Discover returns the live unified candidate list. Same-(kind,name)
// across multiple sources is flagged with Conflict=true so the UI
// can present them together.
func (s *Capabilities) Discover(ctx context.Context, req model.DiscoverRequest) ([]model.CapabilityCandidate, error) {
	if s.Store == nil {
		return nil, errors.New("capabilities: missing store")
	}

	kindFilter := makeKindFilter(req.Kinds)
	rtFilter := makeStringFilter(req.Runtimes)

	var out []model.CapabilityCandidate

	// AVM-managed records.
	recs, err := s.Store.List()
	if err != nil {
		return nil, err
	}
	for i := range recs {
		r := recs[i]
		if !kindFilter(r.Kind) {
			continue
		}
		// Source defaults to SourceAVM when the record didn't carry an
		// explicit source — discovery surface always reports avm here.
		src := r.Source
		if src == "" {
			src = model.SourceAVM
		}
		out = append(out, model.CapabilityCandidate{
			Kind:   r.Kind,
			Name:   r.Name,
			Source: src,
			Record: &recs[i],
		})
	}

	// Runtime-global discoveries.
	if s.Runtimes != nil {
		for _, info := range s.Runtimes.List() {
			if !rtFilter(info.Name) {
				continue
			}
			drv, err := s.Runtimes.Resolve(info.Name)
			if err != nil {
				continue
			}
			globals, err := drv.DiscoverGlobal(ctx)
			if err != nil {
				// Tolerate per-runtime discovery failures: they are
				// often "binary not installed" and should not block
				// other sources.
				continue
			}
			for i := range globals {
				g := globals[i]
				if !kindFilter(g.Kind) {
					continue
				}
				out = append(out, model.CapabilityCandidate{
					Kind:   g.Kind,
					Name:   g.Name,
					Source: model.SourceRuntimeGlobal,
					Global: &globals[i],
				})
			}
		}
	}

	markConflicts(out)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].Source < out[j].Source
	})
	return out, nil
}

func makeKindFilter(kinds []model.CapabilityKind) func(model.CapabilityKind) bool {
	if len(kinds) == 0 {
		return func(model.CapabilityKind) bool { return true }
	}
	allowed := make(map[model.CapabilityKind]struct{}, len(kinds))
	for _, k := range kinds {
		allowed[k] = struct{}{}
	}
	return func(k model.CapabilityKind) bool {
		_, ok := allowed[k]
		return ok
	}
}

func makeStringFilter(values []string) func(string) bool {
	if len(values) == 0 {
		return func(string) bool { return true }
	}
	allowed := make(map[string]struct{}, len(values))
	for _, v := range values {
		allowed[v] = struct{}{}
	}
	return func(v string) bool {
		_, ok := allowed[v]
		return ok
	}
}

// markConflicts marks every candidate that shares (kind,name) with at
// least one other candidate from a different source.
func markConflicts(cands []model.CapabilityCandidate) {
	if len(cands) < 2 {
		return
	}
	type key struct {
		kind model.CapabilityKind
		name string
	}
	seen := map[key]map[model.CapabilitySource]int{}
	for _, c := range cands {
		k := key{c.Kind, c.Name}
		if seen[k] == nil {
			seen[k] = map[model.CapabilitySource]int{}
		}
		seen[k][c.Source]++
	}
	for i := range cands {
		k := key{cands[i].Kind, cands[i].Name}
		if len(seen[k]) > 1 {
			cands[i].Conflict = true
		}
	}
}
