package sync

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/xz1220/agent-vm/internal/adapter"
	"github.com/xz1220/agent-vm/internal/backup"
	"github.com/xz1220/agent-vm/internal/config"
	"github.com/xz1220/agent-vm/internal/state"
)

type StaticAdapterRegistry map[string]adapter.Adapter

func (r StaticAdapterRegistry) Get(runtime string) (adapter.Adapter, bool) {
	adp, ok := r[runtime]
	return adp, ok && adp != nil
}

type Syncer struct {
	Registry AdapterRegistry
	Now      func() time.Time
}

func NewSyncer(registry AdapterRegistry) *Syncer {
	return &Syncer{Registry: registry}
}

func (s *Syncer) SyncActivation(ctx context.Context, resolved *config.ResolvedActivation, opts Options) (*Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if resolved == nil {
		return nil, fmt.Errorf("resolved activation is nil")
	}
	if s == nil || s.Registry == nil {
		return nil, fmt.Errorf("adapter registry is required")
	}

	opts = defaultOptions(opts)
	now := s.now()
	inputs, err := adapter.RenderInputsFromResolved(resolved, adapter.RenderInputOptions{
		ProjectRoot: opts.ProjectRoot,
		ActiveDir:   opts.ActiveDir,
	})
	if err != nil {
		return nil, err
	}
	inputs, missingTargets := filterInputs(inputs, opts.Targets)

	if !opts.DryRun {
		if err := rebuildActive(resolved, opts.ActiveDir, now); err != nil {
			return nil, err
		}
	}

	syncState, err := state.LoadSyncStateOrNew(opts.StatePath, resolved.Active)
	if err != nil {
		return nil, err
	}
	syncState.LastActive = resolved.Active
	syncState.UpdatedAt = now
	if syncState.Runtimes == nil {
		syncState.Runtimes = make(map[string]state.RuntimeState)
	}

	result := &Result{
		Active: resolved.Active,
		DryRun: opts.DryRun,
	}
	for _, warning := range resolved.Warnings {
		if warning != "" {
			result.Warnings = append(result.Warnings, warning)
		}
	}

	for _, runtime := range missingTargets {
		targetResult := TargetResult{
			Runtime:  runtime,
			Status:   TargetStatusSkipped,
			Active:   resolved.Active,
			Warnings: []string{"target has no resolved agent"},
		}
		result.Targets = append(result.Targets, targetResult)
		syncState.Runtimes[runtime] = runtimeStateFromTarget(targetResult, nil, syncState.Runtimes[runtime], now)
	}

	for _, input := range inputs {
		prior := syncState.Runtimes[input.Runtime]
		targetResult := s.renderTarget(ctx, input, resolved.Active, prior, opts, now)
		result.Targets = append(result.Targets, targetResult)
		syncState.Runtimes[input.Runtime] = runtimeStateFromTarget(targetResult, targetResult.ManagedPaths, prior, now)
	}

	if !opts.DryRun {
		if err := state.SaveSyncState(opts.StatePath, syncState); err != nil {
			return result, err
		}
	}

	if err := resultError(result); err != nil {
		return result, err
	}
	return result, nil
}

func (s *Syncer) renderTarget(ctx context.Context, input adapter.RenderInput, active config.ActiveRef, prior state.RuntimeState, opts Options, now time.Time) TargetResult {
	targetResult := TargetResult{
		Runtime:   input.Runtime,
		Status:    TargetStatusFailed,
		Active:    active,
		AgentName: input.Agent.Name,
	}

	adp, ok := s.Registry.Get(input.Runtime)
	if !ok || adp == nil {
		targetResult.Status = TargetStatusSkipped
		targetResult.Warnings = append(targetResult.Warnings, "adapter not registered")
		return targetResult
	}

	detection := adp.Detect(ctx)
	targetResult.Warnings = append(targetResult.Warnings, detection.Warnings...)
	if !detection.Found {
		targetResult.Status = TargetStatusSkipped
		targetResult.Warnings = append(targetResult.Warnings, "runtime not found")
		return targetResult
	}

	plan, err := adp.Plan(ctx, input)
	if err != nil {
		targetResult.Error = err.Error()
		return targetResult
	}
	if plan == nil {
		targetResult.Error = "adapter returned nil render plan"
		return targetResult
	}
	targetResult.Plan = plan
	targetResult.Mappings = append([]adapter.FieldMapping(nil), plan.Mappings...)
	targetResult.Warnings = append(targetResult.Warnings, plan.Warnings...)

	managedPaths := adp.ManagedPaths(ctx, plan)
	if len(managedPaths) == 0 && plan != nil {
		managedPaths = append([]adapter.ManagedPath(nil), plan.ManagedPaths...)
	}
	targetResult.ManagedPaths = managedPaths

	conflicts, err := DetectConflicts(input.Runtime, managedPaths, prior)
	if err != nil {
		targetResult.Error = err.Error()
		return targetResult
	}
	if err := conflictError(conflicts); err != nil {
		targetResult.Error = err.Error()
		return targetResult
	}

	if opts.DryRun {
		targetResult.Status = TargetStatusSynced
		return targetResult
	}

	if _, err := backup.BackupManagedPaths(input.Runtime, managedPaths, opts.BackupDir, now); err != nil {
		targetResult.Error = err.Error()
		return targetResult
	}

	renderResult, err := adp.Render(ctx, plan)
	if err != nil {
		targetResult.Error = err.Error()
		return targetResult
	}
	if renderResult == nil {
		targetResult.Error = "adapter returned nil render result"
		return targetResult
	}
	targetResult.RenderResult = renderResult
	if len(renderResult.ManagedPaths) > 0 {
		targetResult.ManagedPaths = append([]adapter.ManagedPath(nil), renderResult.ManagedPaths...)
	}
	if len(renderResult.Mappings) > 0 {
		targetResult.Mappings = append([]adapter.FieldMapping(nil), renderResult.Mappings...)
	}
	targetResult.Warnings = append(targetResult.Warnings, renderResult.Warnings...)
	targetResult.Status = TargetStatusSynced
	return targetResult
}

func runtimeStateFromTarget(target TargetResult, managedPaths []adapter.ManagedPath, prior state.RuntimeState, now time.Time) state.RuntimeState {
	runtimeState := state.RuntimeState{
		Runtime:   target.Runtime,
		Status:    state.RuntimeStatus(target.Status),
		Active:    target.Active,
		AgentName: target.AgentName,
		Mappings:  state.MappingStates(target.Mappings),
		Warnings:  append([]string(nil), target.Warnings...),
		Error:     target.Error,
		UpdatedAt: now.UTC(),
	}

	if len(managedPaths) > 0 && target.Status == TargetStatusSynced && target.RenderResult != nil {
		hashed, err := ManagedPathStatesWithHashes(managedPaths)
		if err == nil {
			runtimeState.ManagedPaths = hashed
			return runtimeState
		}
		runtimeState.Warnings = append(runtimeState.Warnings, "failed to hash managed paths: "+err.Error())
	}
	runtimeState.ManagedPaths = managedPathStatesWithPriorHashes(managedPaths, prior.ManagedPaths)
	return runtimeState
}

func managedPathStatesWithPriorHashes(paths []adapter.ManagedPath, prior []state.ManagedPathState) []state.ManagedPathState {
	states := state.ManagedPathStates(paths)
	if len(states) == 0 || len(prior) == 0 {
		return states
	}

	priorByPath := make(map[string]state.ManagedPathState, len(prior))
	for _, item := range prior {
		if item.Path != "" {
			priorByPath[item.Path] = item
		}
	}
	for i := range states {
		priorState, ok := priorByPath[states[i].Path]
		if !ok {
			continue
		}
		states[i].FileHash = priorState.FileHash
		states[i].ManagedHash = priorState.ManagedHash
	}
	return states
}

func defaultOptions(opts Options) Options {
	if opts.ActiveDir == "" {
		opts.ActiveDir = config.ActiveDir()
	}
	if opts.StatePath == "" {
		opts.StatePath = state.SyncStatePath()
	}
	if opts.BackupDir == "" {
		opts.BackupDir = config.BackupDir()
	}
	return opts
}

func (s *Syncer) now() time.Time {
	if s != nil && s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func filterInputs(inputs []adapter.RenderInput, targets []string) ([]adapter.RenderInput, []string) {
	if len(targets) == 0 {
		return inputs, nil
	}

	allowed := make(map[string]struct{}, len(targets))
	for _, target := range targets {
		if target != "" {
			allowed[target] = struct{}{}
		}
	}

	filtered := make([]adapter.RenderInput, 0, len(inputs))
	found := make(map[string]struct{}, len(inputs))
	for _, input := range inputs {
		if _, ok := allowed[input.Runtime]; ok {
			filtered = append(filtered, input)
			found[input.Runtime] = struct{}{}
		}
	}

	missing := make([]string, 0)
	for target := range allowed {
		if _, ok := found[target]; !ok {
			missing = append(missing, target)
		}
	}
	sort.Strings(missing)
	return filtered, missing
}

func resultError(result *Result) error {
	if result == nil {
		return nil
	}

	failed := make([]string, 0)
	for _, target := range result.Targets {
		if target.Status == TargetStatusFailed {
			if target.Error != "" {
				failed = append(failed, target.Runtime+": "+target.Error)
			} else {
				failed = append(failed, target.Runtime)
			}
		}
	}
	if len(failed) == 0 {
		return nil
	}
	sort.Strings(failed)
	return fmt.Errorf("sync activation failed for %s", strings.Join(failed, "; "))
}
