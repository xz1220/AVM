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

// validationError translates a model.Agent.Validate failure into a
// typed Error. The validator only ever rejects the name regex today,
// so we map to CodeAgentInvalidName when the agent has a Name set;
// otherwise CodeValidation.
func validationError(name string, err error) *Error {
	if name != "" {
		return WrapError(CodeAgentInvalidName, err,
			err.Error(),
			map[string]any{"name": name},
		)
	}
	return WrapError(CodeValidation, err, err.Error(), nil)
}

func requireRuntimePrefs(prefs []model.RuntimePref, hint string) error {
	if len(prefs) == 0 {
		return MissingInputError("runtime", hint)
	}
	return nil
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
		return nil, validationError(req.Name, err)
	}
	if err := requireRuntimePrefs(agent.Runtimes, "at least one runtime preference is required"); err != nil {
		return nil, err
	}

	target := req.Name
	if s.Repo.Exists(target) {
		switch req.OnConflict {
		case model.ResolveOverwrite:
			if err := withOverwrite(s.Repo, true, func() error {
				return s.Repo.Save(agent)
			}); err != nil {
				return nil, WrapError(CodeIOFailure, err,
					fmt.Sprintf("save agent %q: %v", target, err),
					map[string]any{"name": target},
				)
			}
			return agent, nil
		case model.ResolveCancel:
			return nil, AgentConflictError(target,
				fmt.Sprintf("create cancelled: agent %q already exists", target))
		case model.ResolveRename:
			// Service cannot invent a fresh name safely; the caller
			// (UI/CLI) must reissue Create with a new Name and
			// OnConflict left blank.
			return nil, AgentConflictError(target,
				fmt.Sprintf("rename requested but no new name supplied for agent %q", target))
		case model.ResolveAsk:
			return nil, AgentConflictError(target,
				fmt.Sprintf("agent %q already exists", target))
		default:
			return nil, NewError(CodeValidation,
				fmt.Sprintf("unknown conflict resolution %q", req.OnConflict),
				map[string]any{"on_conflict": string(req.OnConflict)},
			)
		}
	}

	if err := s.Repo.Save(agent); err != nil {
		// Race condition: a parallel writer created the file between
		// Exists and Save. Surface as conflict if that's what we got.
		if errors.Is(err, agentstore.ErrConflict) {
			return nil, AgentConflictError(target,
				fmt.Sprintf("agent %q already exists (concurrent create)", target))
		}
		return nil, WrapError(CodeIOFailure, err,
			fmt.Sprintf("save agent %q: %v", target, err),
			map[string]any{"name": target},
		)
	}
	return agent, nil
}

// List returns the projection used by `avm agent list`.
func (s *Agents) List(ctx context.Context) ([]model.AgentSummary, error) {
	if s.Repo == nil {
		return nil, errors.New("agents: missing repository")
	}
	out, err := s.Repo.List()
	if err != nil {
		return nil, WrapError(CodeIOFailure, err, "list agents: "+err.Error(), nil)
	}
	return out, nil
}

// Show returns Agent detail enriched with per-runtime mapping summaries.
// Driver failures are reported as warnings; they do not fail Show.
func (s *Agents) Show(ctx context.Context, name string) (*model.AgentDetail, error) {
	if s.Repo == nil {
		return nil, errors.New("agents: missing repository")
	}
	agent, err := s.Repo.Get(name)
	if err != nil {
		if errors.Is(err, agentstore.ErrNotFound) {
			return nil, AgentNotFoundError(name, err)
		}
		return nil, WrapError(CodeIOFailure, err, "load agent: "+err.Error(),
			map[string]any{"name": name})
	}
	src, err := s.Repo.SourcePath(name)
	if err != nil {
		return nil, WrapError(CodeIOFailure, err, "agent source path: "+err.Error(),
			map[string]any{"name": name})
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
		return nil, MissingInputError("name", "agent name is required for edit")
	}
	agent, err := s.Repo.Get(req.Name)
	if err != nil {
		if errors.Is(err, agentstore.ErrNotFound) {
			return nil, AgentNotFoundError(req.Name, err)
		}
		return nil, WrapError(CodeIOFailure, err, "load agent: "+err.Error(),
			map[string]any{"name": req.Name})
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
		return nil, validationError(req.Name, err)
	}
	if err := requireRuntimePrefs(agent.Runtimes, "at least one runtime preference is required"); err != nil {
		return nil, err
	}
	if err := withOverwrite(s.Repo, true, func() error { return s.Repo.Save(agent) }); err != nil {
		return nil, WrapError(CodeIOFailure, err, "save agent: "+err.Error(),
			map[string]any{"name": req.Name})
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
		return MissingInputError("name", "agent name is required for delete")
	}
	if !req.Confirm {
		return MissingInputError("confirm", "set Confirm=true (CLI: --yes) to acknowledge deletion")
	}
	if err := s.Repo.Delete(req.Name); err != nil {
		if errors.Is(err, agentstore.ErrNotFound) {
			return AgentNotFoundError(req.Name, err)
		}
		return WrapError(CodeIOFailure, err, "delete agent: "+err.Error(),
			map[string]any{"name": req.Name})
	}
	return nil
}

// Clone duplicates an existing Agent under a new name. The new name
// must not already exist.
func (s *Agents) Clone(ctx context.Context, name, newName string) (*model.Agent, error) {
	if s.Repo == nil {
		return nil, errors.New("agents: missing repository")
	}
	if name == "" {
		return nil, MissingInputError("name", "source agent name is required")
	}
	if newName == "" {
		return nil, MissingInputError("new_name", "target agent name is required")
	}
	src, err := s.Repo.Get(name)
	if err != nil {
		if errors.Is(err, agentstore.ErrNotFound) {
			return nil, AgentNotFoundError(name, err)
		}
		return nil, WrapError(CodeIOFailure, err, "load agent: "+err.Error(),
			map[string]any{"name": name})
	}
	if s.Repo.Exists(newName) {
		return nil, AgentConflictError(newName,
			fmt.Sprintf("clone target %q already exists", newName))
	}
	dst := *src
	dst.Identity.Name = newName
	if err := dst.Validate(); err != nil {
		return nil, validationError(newName, err)
	}
	if err := requireRuntimePrefs(dst.Runtimes, "source agent has no runtime preferences; edit it before cloning"); err != nil {
		return nil, err
	}
	if err := s.Repo.Save(&dst); err != nil {
		return nil, WrapError(CodeIOFailure, err, "save clone: "+err.Error(),
			map[string]any{"name": newName})
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
	if oldName == "" {
		return nil, MissingInputError("old_name", "source agent name is required")
	}
	if newName == "" {
		return nil, MissingInputError("new_name", "target agent name is required")
	}
	if oldName == newName {
		return s.Repo.Get(oldName)
	}
	src, err := s.Repo.Get(oldName)
	if err != nil {
		if errors.Is(err, agentstore.ErrNotFound) {
			return nil, AgentNotFoundError(oldName, err)
		}
		return nil, WrapError(CodeIOFailure, err, "load agent: "+err.Error(),
			map[string]any{"name": oldName})
	}
	if s.Repo.Exists(newName) {
		return nil, AgentConflictError(newName,
			fmt.Sprintf("rename target %q already exists", newName))
	}
	dst := *src
	dst.Identity.Name = newName
	if err := dst.Validate(); err != nil {
		return nil, validationError(newName, err)
	}
	if err := requireRuntimePrefs(dst.Runtimes, "source agent has no runtime preferences; edit it before renaming"); err != nil {
		return nil, err
	}
	if err := s.Repo.Save(&dst); err != nil {
		return nil, WrapError(CodeIOFailure, err, "save renamed agent: "+err.Error(),
			map[string]any{"name": newName})
	}
	if err := s.Repo.Delete(oldName); err != nil {
		// New file is in place; surface the partial state.
		return &dst, WrapError(CodeIOFailure, err,
			"new agent saved but old delete failed: "+err.Error(),
			map[string]any{"old": oldName, "new": newName})
	}
	return &dst, nil
}
