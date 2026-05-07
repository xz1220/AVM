// Package service hosts AVM's application-layer use cases. Services
// orchestrate model, runtime and infra to fulfil PRD §4 user actions.
//
// Services own product rules. They do not own runtime-specific config
// file details, and they never write runtime-managed paths directly.
package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/xz1220/agent-vm/internal/app/model"
	"github.com/xz1220/agent-vm/internal/infra/agentstore"
	"github.com/xz1220/agent-vm/internal/runtime"
)

// AgentService implements PRD §4.2 Agent CRUD.
type AgentService interface {
	Create(ctx context.Context, req model.CreateAgentRequest) (*model.Agent, error)
	List(ctx context.Context) ([]model.AgentSummary, error)
	Show(ctx context.Context, name string) (*model.AgentDetail, error)
	Edit(ctx context.Context, req model.EditAgentRequest) (*model.Agent, error)
	Delete(ctx context.Context, req model.DeleteAgentRequest) error
	Clone(ctx context.Context, name, newName string) (*model.Agent, error)
	Rename(ctx context.Context, oldName, newName string) (*model.Agent, error)
}

// Agents is the default AgentService.
type Agents struct {
	Repo     agentstore.Repository
	Runtimes runtime.Registry
}

// NewAgents constructs the default AgentService.
func NewAgents(repo agentstore.Repository, registry runtime.Registry) *Agents {
	return &Agents{Repo: repo, Runtimes: registry}
}

// withOverwrite temporarily flips r.Overwrite to v if r is an *FSRepo,
// runs fn, and then restores the previous value. We touch the
// concrete FSRepo via type assertion only — service code itself
// continues to depend on the Repository interface.
func withOverwrite(repo agentstore.Repository, v bool, fn func() error) error {
	fs, ok := repo.(*agentstore.FSRepo)
	if !ok {
		return fn()
	}
	prev := fs.Overwrite
	fs.Overwrite = v
	defer func() { fs.Overwrite = prev }()
	return fn()
}

// Create implements PRD §4.2: explicit conflict resolution; never an
// implicit overwrite.
func (s *Agents) Create(ctx context.Context, req model.CreateAgentRequest) (*model.Agent, error) {
	if s.Repo == nil {
		return nil, errors.New("agents: missing repository")
	}
	agent := &model.Agent{
		Identity: model.Identity{
			Name:        req.Name,
			Description: req.Description,
			Role:        req.Role,
		},
		Instructions: req.Instructions,
		Skills:       req.Skills,
		MCP:          req.MCP,
		Runtimes:     req.Runtimes,
	}
	if err := agent.Validate(); err != nil {
		return nil, fmt.Errorf("agents: create: %w", err)
	}

	// Source == package: package import is the responsibility of
	// PackageService.Install. Create itself does not pull from packages.
	// Callers wanting a package-derived Agent should call Install first
	// and then optionally Edit the resulting Agent.

	target := req.Name
	if s.Repo.Exists(target) {
		switch req.OnConflict {
		case model.ResolveOverwrite:
			if err := withOverwrite(s.Repo, true, func() error {
				return s.Repo.Save(agent)
			}); err != nil {
				return nil, fmt.Errorf("agents: create: %w", err)
			}
			return agent, nil
		case model.ResolveCancel:
			return nil, fmt.Errorf("agents: create: cancelled: %w", agentstore.ErrConflict)
		case model.ResolveRename:
			if req.NonInteractive {
				return nil, fmt.Errorf("agents: create: rename requires interactive mode for new name; got conflict on %q: %w", target, agentstore.ErrConflict)
			}
			// Without a presentation-supplied new name, we cannot
			// invent one safely here. Surface conflict to the caller;
			// presentation layer should reissue Create with a new
			// Name and OnConflict left blank.
			return nil, fmt.Errorf("agents: create: rename requires a new name from the caller: %w", agentstore.ErrConflict)
		case model.ResolveAsk:
			return nil, fmt.Errorf("agents: create: agent %q already exists: %w", target, agentstore.ErrConflict)
		default:
			return nil, fmt.Errorf("agents: create: unknown conflict resolution %q", req.OnConflict)
		}
	}

	if err := s.Repo.Save(agent); err != nil {
		return nil, fmt.Errorf("agents: create: %w", err)
	}
	return agent, nil
}

// List returns the projection used by `avm agent list`.
func (s *Agents) List(ctx context.Context) ([]model.AgentSummary, error) {
	if s.Repo == nil {
		return nil, errors.New("agents: missing repository")
	}
	return s.Repo.List()
}

// Show returns Agent detail enriched with per-runtime mapping summaries.
// Driver failures are reported as warnings; they do not fail Show.
func (s *Agents) Show(ctx context.Context, name string) (*model.AgentDetail, error) {
	if s.Repo == nil {
		return nil, errors.New("agents: missing repository")
	}
	agent, err := s.Repo.Get(name)
	if err != nil {
		return nil, fmt.Errorf("agents: show %q: %w", name, err)
	}
	src, err := s.Repo.SourcePath(name)
	if err != nil {
		return nil, fmt.Errorf("agents: show %q: %w", name, err)
	}
	detail := &model.AgentDetail{Agent: *agent, SourcePath: src}
	if s.Runtimes != nil {
		for _, pref := range agent.Runtimes {
			summary := model.RuntimeMappingSummary{Runtime: pref.Runtime}
			drv, derr := s.Runtimes.Resolve(pref.Runtime)
			if derr != nil {
				summary.Warnings = append(summary.Warnings, derr.Error())
				detail.Mapping = append(detail.Mapping, summary)
				continue
			}
			plan, perr := drv.Plan(ctx, agent)
			if perr != nil {
				summary.Warnings = append(summary.Warnings, perr.Error())
				detail.Mapping = append(detail.Mapping, summary)
				continue
			}
			for _, m := range plan.Mappings {
				summary.Fields = append(summary.Fields, model.FieldMappingSummary{
					Field:  m.Field,
					Status: m.Status,
					Note:   m.Note,
				})
			}
			for _, w := range plan.Warnings {
				summary.Warnings = append(summary.Warnings, w.Message)
			}
			detail.Mapping = append(detail.Mapping, summary)
		}
	}
	return detail, nil
}

// Edit applies a partial edit. Nil pointer fields keep existing values.
func (s *Agents) Edit(ctx context.Context, req model.EditAgentRequest) (*model.Agent, error) {
	if s.Repo == nil {
		return nil, errors.New("agents: missing repository")
	}
	if req.Name == "" {
		return nil, errors.New("agents: edit: empty name")
	}
	agent, err := s.Repo.Get(req.Name)
	if err != nil {
		return nil, fmt.Errorf("agents: edit %q: %w", req.Name, err)
	}
	if req.Identity != nil {
		// Preserve the original name regardless of what the request
		// carries — edit must not silently rename. Use Rename for that.
		ident := *req.Identity
		ident.Name = agent.Identity.Name
		agent.Identity = ident
	}
	if req.Instructions != nil {
		agent.Instructions = *req.Instructions
	}
	if req.Skills != nil {
		agent.Skills = *req.Skills
	}
	if req.MCP != nil {
		agent.MCP = *req.MCP
	}
	if req.Runtimes != nil {
		agent.Runtimes = *req.Runtimes
	}
	if err := agent.Validate(); err != nil {
		return nil, fmt.Errorf("agents: edit %q: %w", req.Name, err)
	}
	if err := withOverwrite(s.Repo, true, func() error { return s.Repo.Save(agent) }); err != nil {
		return nil, fmt.Errorf("agents: edit %q: %w", req.Name, err)
	}
	return agent, nil
}

// Delete removes the named Agent per PRD §4.2 (does not touch
// referenced capabilities).
func (s *Agents) Delete(ctx context.Context, req model.DeleteAgentRequest) error {
	if s.Repo == nil {
		return errors.New("agents: missing repository")
	}
	if req.Name == "" {
		return errors.New("agents: delete: empty name")
	}
	if req.NonInteractive && !req.Confirm {
		return errors.New("agents: delete: --confirm required in non-interactive mode")
	}
	if err := s.Repo.Delete(req.Name); err != nil {
		return fmt.Errorf("agents: delete %q: %w", req.Name, err)
	}
	return nil
}

// Clone duplicates an existing Agent under a new name. The new name
// must not already exist.
func (s *Agents) Clone(ctx context.Context, name, newName string) (*model.Agent, error) {
	if s.Repo == nil {
		return nil, errors.New("agents: missing repository")
	}
	if name == "" || newName == "" {
		return nil, errors.New("agents: clone: empty name")
	}
	src, err := s.Repo.Get(name)
	if err != nil {
		return nil, fmt.Errorf("agents: clone %q: %w", name, err)
	}
	if s.Repo.Exists(newName) {
		return nil, fmt.Errorf("agents: clone: %q already exists: %w", newName, agentstore.ErrConflict)
	}
	dst := *src
	dst.Identity.Name = newName
	if err := dst.Validate(); err != nil {
		return nil, fmt.Errorf("agents: clone: %w", err)
	}
	if err := s.Repo.Save(&dst); err != nil {
		return nil, fmt.Errorf("agents: clone: %w", err)
	}
	return &dst, nil
}

// Rename moves an Agent. We save the new name first; only on success do
// we delete the old. If the new name already exists we never touch the
// old file.
func (s *Agents) Rename(ctx context.Context, oldName, newName string) (*model.Agent, error) {
	if s.Repo == nil {
		return nil, errors.New("agents: missing repository")
	}
	if oldName == "" || newName == "" {
		return nil, errors.New("agents: rename: empty name")
	}
	if oldName == newName {
		return s.Repo.Get(oldName)
	}
	src, err := s.Repo.Get(oldName)
	if err != nil {
		return nil, fmt.Errorf("agents: rename %q: %w", oldName, err)
	}
	if s.Repo.Exists(newName) {
		return nil, fmt.Errorf("agents: rename: %q already exists: %w", newName, agentstore.ErrConflict)
	}
	dst := *src
	dst.Identity.Name = newName
	if err := dst.Validate(); err != nil {
		return nil, fmt.Errorf("agents: rename: %w", err)
	}
	if err := s.Repo.Save(&dst); err != nil {
		return nil, fmt.Errorf("agents: rename: save new: %w", err)
	}
	if err := s.Repo.Delete(oldName); err != nil {
		// New file is in place; surface the partial state.
		return &dst, fmt.Errorf("agents: rename: new saved but old delete failed: %w", err)
	}
	return &dst, nil
}
