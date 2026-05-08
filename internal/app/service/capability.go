package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
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
	// Import copies one runtime-global capability into the AVM
	// capability store. See PRD §4.2 / docs/rewrite-architecture-proposal §7.
	Import(ctx context.Context, req model.ImportCapabilityRequest) (*model.ImportCapabilityResult, error)
	// Bootstrap imports every runtime-global capability the named
	// runtime currently exposes. Single failures land in
	// BootstrapCapabilitiesResult.Skipped and never abort the run.
	Bootstrap(ctx context.Context, req model.BootstrapCapabilitiesRequest) (*model.BootstrapCapabilitiesResult, error)

	// List returns every record currently held in the AVM capability
	// store. Unlike Discover, this never calls into runtime drivers;
	// it is meant for hot UI paths that resolve Agent capability IDs
	// to display metadata (name, source, version, import_from).
	List(ctx context.Context) ([]model.CapabilityRecord, error)
	// Get returns a single record by ID. Returns a typed
	// CAPABILITY_NOT_FOUND error if the ID is unknown.
	Get(ctx context.Context, id model.CapabilityID) (*model.CapabilityRecord, error)
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
		return nil, WrapError(CodeIOFailure, err,
			"list capability store: "+err.Error(), nil)
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
	markImported(out)
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

// Import implements PRD §4.2 runtime-global → capstore copy. See the
// CapabilityService interface for the contract.
//
// capstore is content-addressed (ID embeds the content checksum), so
// identical content + identical name dedupe automatically. Different
// content with the same (kind,name) does NOT collide in capstore — it
// would just produce two separate IDs. The conflict semantics PRD §4.2
// expects ("rename / replace / cancel") therefore live at the service
// layer: we explicitly look up an existing (kind,name) record and
// compare its checksum to the new export before writing.
func (s *Capabilities) Import(ctx context.Context, req model.ImportCapabilityRequest) (*model.ImportCapabilityResult, error) {
	if s.Store == nil {
		return nil, errors.New("capabilities: missing store")
	}
	if req.Runtime == "" {
		return nil, MissingInputError("runtime", "runtime name is required")
	}
	if req.Kind == "" {
		return nil, MissingInputError("kind", "capability kind is required (skill|mcp)")
	}
	if req.Name == "" {
		return nil, MissingInputError("name", "capability name is required")
	}
	if s.Runtimes == nil {
		return nil, errors.New("capabilities: missing runtime registry")
	}

	drv, err := s.Runtimes.Resolve(req.Runtime)
	if err != nil {
		if errors.Is(err, runtime.ErrUnknownRuntime) {
			return nil, NewError(CodeRuntimeNotFound,
				fmt.Sprintf("runtime %q not registered", req.Runtime),
				map[string]any{"runtime": req.Runtime})
		}
		return nil, WrapError(CodeIOFailure, err, err.Error(), nil)
	}

	exported, err := drv.ExportGlobal(ctx, req.Kind, req.Name)
	if err != nil {
		if errors.Is(err, runtime.ErrGlobalCapabilityNotFound) {
			return nil, NewError(CodeCapabilityNotFound,
				fmt.Sprintf("runtime %q has no %s named %q",
					req.Runtime, req.Kind, req.Name),
				map[string]any{"runtime": req.Runtime, "kind": string(req.Kind), "name": req.Name})
		}
		return nil, WrapError(CodeIOFailure, err,
			fmt.Sprintf("export %s/%s from %s: %v", req.Kind, req.Name, req.Runtime, err), nil)
	}
	defer func() {
		if exported.Content != nil {
			_ = exported.Content.Close()
		}
	}()

	body, err := io.ReadAll(exported.Content)
	if err != nil {
		return nil, WrapError(CodeIOFailure, err,
			"read exported content: "+err.Error(), nil)
	}
	contentSum := sha256Hex(body)
	source := req.Runtime + ":" + exported.Capability.Path
	rec := model.CapabilityRecord{
		Kind:       req.Kind,
		Name:       req.Name,
		Version:    exported.Capability.Version,
		Source:     model.SourceRuntimeGlobal,
		ImportFrom: source,
		Format:     exported.Format,
	}

	// (kind,name) collision check: does the store already hold this
	// logical capability?
	existingID, existingChecksum, _ := s.findByKindName(req.Kind, req.Name)
	if existingID != "" {
		if existingChecksum == contentSum {
			// Same content → dedup hit. Don't even call Add; capstore
			// would derive the same ID and return it idempotently, but
			// we already know the answer.
			return &model.ImportCapabilityResult{
				ID:      existingID,
				Created: false,
				Source:  source,
			}, nil
		}
		// Same name, different content → apply OnConflict.
		switch req.OnConflict {
		case model.ResolveSkip:
			return &model.ImportCapabilityResult{
				ID:      existingID,
				Created: false,
				Source:  source,
			}, nil
		case model.ResolveOverwrite:
			if rmErr := s.Store.Remove(existingID); rmErr != nil {
				return nil, WrapError(CodeIOFailure, rmErr,
					"remove existing before overwrite: "+rmErr.Error(),
					map[string]any{"id": string(existingID)})
			}
			newID, addErr := s.Store.Add(rec, bytes.NewReader(body))
			if addErr != nil {
				return nil, WrapError(CodeIOFailure, addErr,
					"add capability after overwrite: "+addErr.Error(), nil)
			}
			return &model.ImportCapabilityResult{
				ID:       newID,
				Created:  true,
				Replaced: true,
				Source:   source,
			}, nil
		default:
			// ResolveCancel / ResolveAsk / empty → surface conflict.
			return nil, NewError(CodeCapabilityConflict,
				fmt.Sprintf("%s %q already imported with different content; pass --on-conflict skip|overwrite",
					req.Kind, req.Name),
				map[string]any{
					"kind":              string(req.Kind),
					"name":              req.Name,
					"existing_id":       string(existingID),
					"existing_checksum": existingChecksum,
				})
		}
	}

	// Fresh add.
	id, addErr := s.Store.Add(rec, bytes.NewReader(body))
	if addErr != nil {
		return nil, WrapError(CodeIOFailure, addErr,
			"add capability: "+addErr.Error(), nil)
	}
	return &model.ImportCapabilityResult{
		ID:      id,
		Created: true,
		Source:  source,
	}, nil
}

// sha256Hex computes the lowercase-hex SHA-256 of body. We use the
// service-local helper rather than depending on capstore's private
// fsutil.Sha256Sum so the service stays free of infra-internal imports.
func sha256Hex(body []byte) string {
	h := sha256.Sum256(body)
	return hex.EncodeToString(h[:])
}

// findByKindName scans the capstore for a record matching (kind,name)
// and returns its ID + checksum. Empty ID means no match.
func (s *Capabilities) findByKindName(kind model.CapabilityKind, name string) (model.CapabilityID, string, error) {
	recs, err := s.Store.List()
	if err != nil {
		return "", "", err
	}
	for _, r := range recs {
		if r.Kind == kind && r.Name == name {
			return r.ID, r.Checksum, nil
		}
	}
	return "", "", nil
}

// Bootstrap implements PRD §4.2 first-install batch import.
func (s *Capabilities) Bootstrap(ctx context.Context, req model.BootstrapCapabilitiesRequest) (*model.BootstrapCapabilitiesResult, error) {
	if s.Store == nil {
		return nil, errors.New("capabilities: missing store")
	}
	if req.Runtime == "" {
		return nil, MissingInputError("runtime", "runtime name is required")
	}
	if s.Runtimes == nil {
		return nil, errors.New("capabilities: missing runtime registry")
	}
	if _, err := s.Runtimes.Resolve(req.Runtime); err != nil {
		if errors.Is(err, runtime.ErrUnknownRuntime) {
			return nil, NewError(CodeRuntimeNotFound,
				fmt.Sprintf("runtime %q not registered", req.Runtime),
				map[string]any{"runtime": req.Runtime})
		}
		return nil, WrapError(CodeIOFailure, err, err.Error(), nil)
	}

	cands, err := s.Discover(ctx, model.DiscoverRequest{
		Runtimes: []string{req.Runtime},
		Kinds:    req.Kinds,
	})
	if err != nil {
		return nil, err
	}

	out := &model.BootstrapCapabilitiesResult{}
	for _, c := range cands {
		if c.Source != model.SourceRuntimeGlobal {
			continue
		}
		single := model.ImportCapabilityRequest{
			Runtime:    req.Runtime,
			Kind:       c.Kind,
			Name:       c.Name,
			OnConflict: req.OnConflict,
		}
		res, importErr := s.Import(ctx, single)
		if importErr != nil {
			out.Skipped = append(out.Skipped, model.SkippedCapability{
				Kind:   c.Kind,
				Name:   c.Name,
				Reason: importErr.Error(),
			})
			continue
		}
		out.Imported = append(out.Imported, *res)
	}
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

// markImported flips Imported=true on runtime-global discovery
// candidates whose (kind,name) already appears as a stored record
// (i.e. some other candidate in the slice has Record != nil). Lets
// the UI suppress redundant import prompts.
func markImported(cands []model.CapabilityCandidate) {
	if len(cands) < 2 {
		return
	}
	type key struct {
		kind model.CapabilityKind
		name string
	}
	inStore := map[key]struct{}{}
	for _, c := range cands {
		// Record != nil means the candidate originates from capstore.
		if c.Record != nil {
			inStore[key{c.Kind, c.Name}] = struct{}{}
		}
	}
	for i := range cands {
		// Only flag the runtime-global discovery copy (Global != nil).
		if cands[i].Global == nil {
			continue
		}
		if _, ok := inStore[key{cands[i].Kind, cands[i].Name}]; ok {
			cands[i].Imported = true
		}
	}
}

// List returns capstore records sorted by ID (capstore.List ordering).
// It does not consult runtime drivers, so it is safe for hot UI paths.
func (s *Capabilities) List(ctx context.Context) ([]model.CapabilityRecord, error) {
	if s.Store == nil {
		return nil, errors.New("capabilities: missing store")
	}
	recs, err := s.Store.List()
	if err != nil {
		return nil, WrapError(CodeIOFailure, err,
			"list capability store: "+err.Error(), nil)
	}
	return recs, nil
}

// Get returns a single capstore record by ID.
func (s *Capabilities) Get(ctx context.Context, id model.CapabilityID) (*model.CapabilityRecord, error) {
	if s.Store == nil {
		return nil, errors.New("capabilities: missing store")
	}
	if id == "" {
		return nil, MissingInputError("id", "capability id is required")
	}
	rec, err := s.Store.Get(id)
	if err != nil {
		if errors.Is(err, capstore.ErrNotFound) {
			return nil, NewError(CodeCapabilityNotFound,
				fmt.Sprintf("capability %q not found", id),
				map[string]any{"id": string(id)})
		}
		return nil, WrapError(CodeIOFailure, err,
			"get capability: "+err.Error(),
			map[string]any{"id": string(id)})
	}
	return &rec, nil
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
