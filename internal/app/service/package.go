package service

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/xz1220/agent-vm/internal/app/model"
	"github.com/xz1220/agent-vm/internal/infra/agentstore"
	"github.com/xz1220/agent-vm/internal/infra/capstore"
	"github.com/xz1220/agent-vm/internal/infra/packageio"
)

// PackageService implements PRD §4.5 package install/export/inspect.
type PackageService interface {
	List(ctx context.Context) ([]model.PackageSummary, error)
	Show(ctx context.Context, name string) (*model.PackageDetail, error)
	Install(ctx context.Context, req model.InstallRequest) (*model.InstallResult, error)
	Uninstall(ctx context.Context, name string) error
	Export(ctx context.Context, req model.ExportRequest) (*model.ExportResult, error)
	Inspect(ctx context.Context, file string) (*model.PackageDetail, error)
}

// ErrPackageRegistryNotSupported is returned by List/Show until the
// installed-package registry is implemented. PRD §4.5 lists list/show
// as targets but does not pin them to this milestone.
var ErrPackageRegistryNotSupported = errors.New("packages: installed-package registry not yet supported; use 'avm package inspect <file>'")

// Packages is the default PackageService.
type Packages struct {
	Agents agentstore.Repository
	Caps   capstore.Store
	IO     packageio.IO
}

func NewPackages(agents agentstore.Repository, caps capstore.Store, io packageio.IO) *Packages {
	return &Packages{Agents: agents, Caps: caps, IO: io}
}

// capKey is the (kind, name) lookup key used to rewrite Agent
// CapabilityRefs to the locally-imported store IDs after install.
type capKey struct {
	kind model.CapabilityKind
	name string
}

// List is not yet implemented (no installed-package registry).
func (s *Packages) List(ctx context.Context) ([]model.PackageSummary, error) {
	return nil, nil
}

// Show is not yet implemented (no installed-package registry).
func (s *Packages) Show(ctx context.Context, name string) (*model.PackageDetail, error) {
	return nil, ErrPackageRegistryNotSupported
}

// Inspect reads the manifest and file list from a package on disk.
func (s *Packages) Inspect(ctx context.Context, file string) (*model.PackageDetail, error) {
	if s.IO == nil {
		return nil, errors.New("packages: missing IO")
	}
	if file == "" {
		return nil, MissingInputError("file", "package file path is required")
	}
	manifest, h, err := s.IO.Read(file)
	if err != nil {
		return nil, WrapError(CodePackageInvalidManifest, err,
			"read package: "+err.Error(),
			map[string]any{"file": file})
	}
	defer h.Close()
	return &model.PackageDetail{
		Manifest: *manifest,
		Files:    h.Files(),
		Source:   file,
	}, nil
}

// Uninstall removes the named Agent. Per PRD §4.2 deletion does not
// touch referenced capabilities — the same rule applies to package
// uninstall (capstore is content-addressed and shared across agents).
func (s *Packages) Uninstall(ctx context.Context, name string) error {
	if s.Agents == nil {
		return errors.New("packages: missing agents repo")
	}
	if name == "" {
		return MissingInputError("name", "agent name is required for uninstall")
	}
	if err := s.Agents.Delete(name); err != nil {
		if errors.Is(err, agentstore.ErrNotFound) {
			return AgentNotFoundError(name, err)
		}
		return WrapError(CodeIOFailure, err,
			"uninstall agent: "+err.Error(),
			map[string]any{"name": name})
	}
	return nil
}

// Install reads a package and imports its capabilities and agents.
func (s *Packages) Install(ctx context.Context, req model.InstallRequest) (*model.InstallResult, error) {
	if s.IO == nil || s.Agents == nil || s.Caps == nil {
		return nil, errors.New("packages: missing dependencies")
	}
	if req.Source == "" {
		return nil, MissingInputError("source", "package file path or registry slug is required")
	}
	manifest, h, err := s.IO.Read(req.Source)
	if err != nil {
		return nil, WrapError(CodePackageInvalidManifest, err,
			"read package: "+err.Error(),
			map[string]any{"source": req.Source})
	}
	defer h.Close()

	provenance := manifest.Name
	if manifest.Version != "" {
		provenance = manifest.Name + "@" + manifest.Version
	}

	result := &model.InstallResult{
		Manifest: *manifest,
		Renamed:  map[string]string{},
	}

	// Import capabilities first; remember (kind,name) → store ID so
	// Agent CapabilityRefs can be rewritten to point at the AVM IDs.
	capIDs := map[capKey]model.CapabilityID{}
	for _, blob := range manifest.Capabilities {
		rc, err := h.Open(blob.Path)
		if err != nil {
			return result, WrapError(CodePackageInvalidManifest, err,
				fmt.Sprintf("open capability blob %q: %v", blob.Path, err),
				map[string]any{"path": blob.Path})
		}
		payload, rerr := io.ReadAll(rc)
		_ = rc.Close()
		if rerr != nil {
			return result, WrapError(CodeIOFailure, rerr,
				fmt.Sprintf("read capability %q: %v", blob.Path, rerr),
				map[string]any{"path": blob.Path})
		}
		rec := model.CapabilityRecord{
			Kind:       blob.Kind,
			Name:       blob.Name,
			Source:     model.SourcePackage,
			ImportFrom: provenance,
		}
		id, err := s.Caps.Add(rec, bytes.NewReader(payload))
		if err != nil {
			return result, WrapError(CodeIOFailure, err,
				fmt.Sprintf("import capability %q: %v", blob.Name, err),
				map[string]any{"name": blob.Name, "kind": string(blob.Kind)})
		}
		capIDs[capKey{blob.Kind, blob.Name}] = id
		result.ImportedCaps = append(result.ImportedCaps, id)
	}

	// Import agents.
	for _, ar := range manifest.Agents {
		rc, err := h.Open(ar.Path)
		if err != nil {
			return result, WrapError(CodePackageInvalidManifest, err,
				fmt.Sprintf("open agent %q: %v", ar.Path, err),
				map[string]any{"path": ar.Path})
		}
		data, rerr := io.ReadAll(rc)
		_ = rc.Close()
		if rerr != nil {
			return result, WrapError(CodeIOFailure, rerr,
				fmt.Sprintf("read agent %q: %v", ar.Path, rerr),
				map[string]any{"path": ar.Path})
		}
		var agent model.Agent
		if uerr := yaml.Unmarshal(data, &agent); uerr != nil {
			return result, WrapError(CodePackageInvalidManifest, uerr,
				fmt.Sprintf("parse agent %q: %v", ar.Path, uerr),
				map[string]any{"path": ar.Path})
		}
		// Rewrite CapabilityRef IDs to local store IDs when we just
		// imported a matching (kind, name). Package authors typically
		// reference capabilities by name in the YAML they ship; we
		// look up by name and substitute the local content-addressed ID.
		rewriteCapRefs(agent.Skills, capIDs)
		rewriteCapRefs(agent.MCP, capIDs)

		// Determine target Agent name and conflict handling.
		target := agent.Identity.Name
		if target == "" {
			target = ar.Name
			agent.Identity.Name = target
		}

		exists := s.Agents.Exists(target)
		if exists {
			switch req.Resolution {
			case model.ResolveAsk:
				return result, AgentConflictError(target,
					fmt.Sprintf("agent %q exists; pass --on-conflict {rename|skip|overwrite|cancel}", target))
			case model.ResolveCancel:
				return result, AgentConflictError(target,
					fmt.Sprintf("install cancelled: agent %q already exists", target))
			case model.ResolveSkip:
				result.Skipped = append(result.Skipped, target)
				continue
			case model.ResolveOverwrite:
				if err := requireRuntimePrefs(agent.Runtimes,
					fmt.Sprintf("package agent %q has no runtime preferences", target)); err != nil {
					return result, err
				}
				if err := withOverwrite(s.Agents, true, func() error { return s.Agents.Save(&agent) }); err != nil {
					return result, WrapError(CodeIOFailure, err,
						fmt.Sprintf("save agent %q (overwrite): %v", target, err),
						map[string]any{"name": target})
				}
				result.InstalledAgents = append(result.InstalledAgents, target)
				continue
			case model.ResolveRename:
				newName := nextAvailableName(s.Agents, target)
				agent.Identity.Name = newName
				if err := agent.Validate(); err != nil {
					return result, validationError(newName, err)
				}
				if err := requireRuntimePrefs(agent.Runtimes,
					fmt.Sprintf("package agent %q has no runtime preferences", target)); err != nil {
					return result, err
				}
				if err := s.Agents.Save(&agent); err != nil {
					return result, WrapError(CodeIOFailure, err,
						fmt.Sprintf("save renamed %q: %v", newName, err),
						map[string]any{"old": target, "new": newName})
				}
				result.Renamed[target] = newName
				result.InstalledAgents = append(result.InstalledAgents, newName)
				continue
			default:
				return result, NewError(CodeValidation,
					fmt.Sprintf("unknown conflict resolution %q", req.Resolution),
					map[string]any{"resolution": string(req.Resolution)})
			}
		}

		if err := agent.Validate(); err != nil {
			return result, validationError(target, err)
		}
		if err := requireRuntimePrefs(agent.Runtimes,
			fmt.Sprintf("package agent %q has no runtime preferences", target)); err != nil {
			return result, err
		}
		if err := s.Agents.Save(&agent); err != nil {
			return result, WrapError(CodeIOFailure, err,
				fmt.Sprintf("save agent %q: %v", target, err),
				map[string]any{"name": target})
		}
		result.InstalledAgents = append(result.InstalledAgents, target)
	}

	return result, nil
}

// Export builds an .avm.zip from an Agent definition and (optionally)
// its referenced capability payloads.
func (s *Packages) Export(ctx context.Context, req model.ExportRequest) (*model.ExportResult, error) {
	if s.IO == nil || s.Agents == nil || s.Caps == nil {
		return nil, errors.New("packages: missing dependencies")
	}
	if req.Agent == "" {
		return nil, MissingInputError("agent", "agent name is required for export")
	}
	agent, err := s.Agents.Get(req.Agent)
	if err != nil {
		if errors.Is(err, agentstore.ErrNotFound) {
			return nil, AgentNotFoundError(req.Agent, err)
		}
		return nil, WrapError(CodeIOFailure, err,
			"load agent: "+err.Error(),
			map[string]any{"agent": req.Agent})
	}

	agentYAML, err := yaml.Marshal(agent)
	if err != nil {
		return nil, WrapError(CodeIOFailure, err,
			"marshal agent: "+err.Error(),
			map[string]any{"agent": req.Agent})
	}

	// Build a payload zip with the agent file and (optionally) caps.
	var payloadBuf bytes.Buffer
	zw := zip.NewWriter(&payloadBuf)

	agentPath := "agents/" + agent.Identity.Name + ".yaml"
	if w, werr := zw.Create(agentPath); werr != nil {
		return nil, WrapError(CodeIOFailure, werr, "zip create: "+werr.Error(), nil)
	} else if _, werr := w.Write(agentYAML); werr != nil {
		return nil, WrapError(CodeIOFailure, werr, "zip write: "+werr.Error(), nil)
	}

	manifest := &model.PackageManifest{
		SchemaVersion: "1",
		Name:          agent.Identity.Name,
		Version:       "0.0.0+" + time.Now().UTC().Format("20060102"),
		Description:   agent.Identity.Description,
		CreatedAt:     time.Now().UTC(),
		Agents: []model.PackageAgentRef{
			{Name: agent.Identity.Name, Path: agentPath},
		},
	}

	// Include skills/MCP if requested. Dedup by ID.
	included := map[model.CapabilityID]struct{}{}
	gather := func(refs []model.CapabilityRef, want bool) error {
		if !want {
			return nil
		}
		for _, ref := range refs {
			if ref.ID == "" {
				continue
			}
			if _, dup := included[ref.ID]; dup {
				continue
			}
			rec, err := s.Caps.Get(ref.ID)
			if err != nil {
				return WrapError(CodeCapabilityNotFound, err,
					fmt.Sprintf("capability %s: %v", ref.ID, err),
					map[string]any{"id": string(ref.ID)})
			}
			payload, name, err := readCapPayload(s.Caps, ref.ID, rec.Kind)
			if err != nil {
				return WrapError(CodeIOFailure, err,
					fmt.Sprintf("read capability payload %s: %v", ref.ID, err),
					map[string]any{"id": string(ref.ID)})
			}
			capPath := "capabilities/" + string(rec.Kind) + "/" + name
			cw, werr := zw.Create(capPath)
			if werr != nil {
				return WrapError(CodeIOFailure, werr, "zip create: "+werr.Error(), nil)
			}
			if _, werr := cw.Write(payload); werr != nil {
				return WrapError(CodeIOFailure, werr, "zip write: "+werr.Error(), nil)
			}
			sum := sha256.Sum256(payload)
			manifest.Capabilities = append(manifest.Capabilities, model.PackageCapBlob{
				Kind:     rec.Kind,
				Name:     rec.Name,
				Path:     capPath,
				Checksum: hex.EncodeToString(sum[:]),
			})
			included[ref.ID] = struct{}{}
		}
		return nil
	}
	if err := gather(agent.Skills, req.IncludeSkills); err != nil {
		return nil, err
	}
	if err := gather(agent.MCP, req.IncludeMCP); err != nil {
		return nil, err
	}

	if err := zw.Close(); err != nil {
		return nil, WrapError(CodeIOFailure, err, "zip close: "+err.Error(), nil)
	}

	dst := req.OutputPath
	if dst == "" {
		dst = agent.Identity.Name + ".avm.zip"
	}
	if err := s.IO.Write(manifest, bytes.NewReader(payloadBuf.Bytes()), dst); err != nil {
		return nil, WrapError(CodeIOFailure, err,
			"write package: "+err.Error(),
			map[string]any{"path": dst})
	}
	return &model.ExportResult{Manifest: *manifest, Path: dst}, nil
}

// rewriteCapRefs replaces a CapabilityRef.ID with the imported store
// ID when (kind, name-matching-Ref.ID) matches a recently imported
// capability. Package authors typically reference caps by name — once
// imported, those references must point at the local store.
func rewriteCapRefs(refs []model.CapabilityRef, capIDs map[capKey]model.CapabilityID) {
	for i := range refs {
		if id, ok := capIDs[capKey{refs[i].Kind, string(refs[i].ID)}]; ok {
			refs[i].ID = id
		}
	}
}

// readCapPayload loads the raw bytes for a capability ID via
// Materialize into a temp dir, since capstore intentionally hides its
// on-disk layout. Returns (data, payload-file-name).
func readCapPayload(store capstore.Store, id model.CapabilityID, kind model.CapabilityKind) ([]byte, string, error) {
	tmp, err := os.MkdirTemp("", "avm-export-*")
	if err != nil {
		return nil, "", err
	}
	defer os.RemoveAll(tmp)
	if err := store.Materialize([]model.CapabilityID{id}, tmp); err != nil {
		return nil, "", err
	}
	kindDir := filepath.Join(tmp, string(kind)+"s")
	entries, err := os.ReadDir(kindDir)
	if err != nil {
		return nil, "", err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(kindDir, e.Name()))
		if err != nil {
			return nil, "", err
		}
		return data, e.Name(), nil
	}
	return nil, "", fmt.Errorf("packages: export: empty payload for %s", id)
}

// nextAvailableName walks name-1, name-2, ... until an unused slot.
func nextAvailableName(repo agentstore.Repository, base string) string {
	for i := 1; i < 1000; i++ {
		candidate := fmt.Sprintf("%s-%d", base, i)
		if !repo.Exists(candidate) {
			return candidate
		}
	}
	// Pathological fallback; conflict will surface to the user via Save.
	return base + "-x"
}
