package sync

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	runtimeHomes := runtimeHomesForActivation(resolved.Active, resolved.Targets, opts.RuntimeHomes)
	inputs, err := adapter.RenderInputsFromResolved(resolved, adapter.RenderInputOptions{
		ProjectRoot:  opts.ProjectRoot,
		ActiveDir:    opts.ActiveDir,
		RuntimeHomes: runtimeHomes,
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
	priorRuntimes := cloneRuntimeStates(syncState.Runtimes)

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
		targetResult = normalizeTargetResult(targetResult)
		result.Targets = append(result.Targets, targetResult)
		syncState.Runtimes[runtime] = runtimeStateFromTarget(targetResult, nil, syncState.Runtimes[runtime], now)
	}

	for _, input := range inputs {
		prior := syncState.Runtimes[input.Runtime]
		targetResult := s.renderTarget(ctx, input, resolved.Active, prior, opts, now)
		targetResult = normalizeTargetResult(targetResult)
		result.Targets = append(result.Targets, targetResult)
		syncState.Runtimes[input.Runtime] = runtimeStateFromTarget(targetResult, targetResult.ManagedPaths, prior, now)
	}
	result.Warnings = uniqueNonEmptyStrings(result.Warnings)
	syncErr := resultError(result)

	if !opts.DryRun {
		if syncErr == nil {
			if err := cleanupStaleRuntimeSkills(priorRuntimes, syncState.Runtimes, resolved.Active); err != nil {
				return result, err
			}
		}
		if err := state.SaveSyncState(opts.StatePath, syncState); err != nil {
			return result, err
		}
		if opts.UpdateActive {
			if err := config.UpdateActive(resolved.Active); err != nil {
				return result, err
			}
		}
	}

	if syncErr != nil {
		return result, syncErr
	}
	return result, nil
}

func (s *Syncer) renderTarget(ctx context.Context, input adapter.RenderInput, active config.ActiveRef, prior state.RuntimeState, opts Options, now time.Time) TargetResult {
	targetResult := TargetResult{
		Runtime:     input.Runtime,
		Status:      TargetStatusFailed,
		Active:      active,
		AgentName:   input.Agent.Name,
		RuntimeHome: input.RuntimeHome,
	}

	adp, ok := s.Registry.Get(input.Runtime)
	if !ok || adp == nil {
		targetResult.Status = TargetStatusSkipped
		targetResult.Warnings = append(targetResult.Warnings, "adapter not registered")
		return targetResult
	}

	if input.RuntimeHome != "" && !opts.DryRun {
		if err := os.MkdirAll(input.RuntimeHome, 0o700); err != nil {
			targetResult.Error = err.Error()
			return targetResult
		}
	}

	detection := adp.Detect(ctx)
	targetResult.Warnings = append(targetResult.Warnings, detection.Warnings...)
	if !detection.Found {
		if input.RuntimeHome != "" {
			targetResult.Warnings = append(targetResult.Warnings, "runtime binary not found; rendering isolated runtime home")
		} else {
			targetResult.Status = TargetStatusSkipped
			targetResult.Warnings = append(targetResult.Warnings, "runtime not found")
			return targetResult
		}
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

	if input.RuntimeHome == "" {
		conflicts, err := DetectConflicts(input.Runtime, managedPaths, prior)
		if err != nil {
			targetResult.Error = err.Error()
			return targetResult
		}
		if err := conflictError(conflicts); err != nil {
			targetResult.Error = err.Error()
			return targetResult
		}
	}

	if opts.DryRun {
		targetResult.Status = TargetStatusSynced
		return targetResult
	}

	if input.RuntimeHome != "" {
		sidecars, err := captureRuntimeHomeSidecars(input.Runtime, input.RuntimeHome)
		if err != nil {
			targetResult.Error = err.Error()
			return targetResult
		}
		if err := resetRuntimeHome(input.RuntimeHome); err != nil {
			targetResult.Error = err.Error()
			return targetResult
		}
		if err := restoreRuntimeHomeSidecars(input.RuntimeHome, sidecars); err != nil {
			targetResult.Error = err.Error()
			return targetResult
		}
	} else {
		if _, err := backup.BackupManagedPaths(input.Runtime, managedPaths, opts.BackupDir, now); err != nil {
			targetResult.Error = err.Error()
			return targetResult
		}
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
		targetResult.ManagedPaths = managedPathsAfterRender(renderResult.ManagedPaths, renderResult.Operations)
	}
	if len(renderResult.Mappings) > 0 {
		targetResult.Mappings = append([]adapter.FieldMapping(nil), renderResult.Mappings...)
	}
	targetResult.Warnings = append(targetResult.Warnings, renderResult.Warnings...)
	targetResult.Status = TargetStatusSynced
	return targetResult
}

func normalizeTargetResult(result TargetResult) TargetResult {
	result.Warnings = uniqueNonEmptyStrings(result.Warnings)
	if result.RenderResult != nil {
		result.RenderResult.Warnings = uniqueNonEmptyStrings(result.RenderResult.Warnings)
	}
	return result
}

func uniqueNonEmptyStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func runtimeStateFromTarget(target TargetResult, managedPaths []adapter.ManagedPath, prior state.RuntimeState, now time.Time) state.RuntimeState {
	runtimeState := state.RuntimeState{
		Runtime:     target.Runtime,
		Status:      state.RuntimeStatus(target.Status),
		Active:      target.Active,
		AgentName:   target.AgentName,
		RuntimeHome: target.RuntimeHome,
		Mappings:    state.MappingStates(target.Mappings),
		Warnings:    append([]string(nil), target.Warnings...),
		Error:       target.Error,
		UpdatedAt:   now.UTC(),
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

func managedPathsAfterRender(paths []adapter.ManagedPath, operations []adapter.RenderOperationResult) []adapter.ManagedPath {
	removed := make(map[string]struct{})
	for _, operation := range operations {
		if operation.Action != adapter.OperationRemoveFile || operation.Path == "" {
			continue
		}
		removed[filepath.Clean(operation.Path)] = struct{}{}
	}
	if len(removed) == 0 {
		return append([]adapter.ManagedPath(nil), paths...)
	}

	filtered := make([]adapter.ManagedPath, 0, len(paths))
	for _, path := range paths {
		if _, ok := removed[filepath.Clean(path.Path)]; ok {
			continue
		}
		filtered = append(filtered, path)
	}
	return filtered
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

func cloneRuntimeStates(in map[string]state.RuntimeState) map[string]state.RuntimeState {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]state.RuntimeState, len(in))
	for runtime, runtimeState := range in {
		runtimeState.ManagedPaths = append([]state.ManagedPathState(nil), runtimeState.ManagedPaths...)
		runtimeState.Mappings = append([]state.MappingState(nil), runtimeState.Mappings...)
		runtimeState.Warnings = append([]string(nil), runtimeState.Warnings...)
		out[runtime] = runtimeState
	}
	return out
}

func cleanupStaleRuntimeSkills(prior, current map[string]state.RuntimeState, active config.ActiveRef) error {
	if len(prior) == 0 {
		return nil
	}

	currentPaths := make(map[string]struct{})
	for _, runtimeState := range current {
		if runtimeState.Active != active {
			continue
		}
		for _, managedPath := range runtimeState.ManagedPaths {
			if managedPath.Path != "" {
				currentPaths[filepath.Clean(managedPath.Path)] = struct{}{}
			}
		}
	}

	stale := make(map[string]struct{})
	for _, runtimeState := range prior {
		if runtimeState.RuntimeHome != "" {
			continue
		}
		for _, managedPath := range runtimeState.ManagedPaths {
			if managedPath.Path == "" {
				continue
			}
			path := filepath.Clean(managedPath.Path)
			if _, ok := currentPaths[path]; ok {
				continue
			}
			if isRuntimeSkillManagedPath(path) && runtimeSkillFileAVMManaged(path) {
				stale[path] = struct{}{}
			}
		}
	}

	paths := make([]string, 0, len(stale))
	for path := range stale {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		if err := removeRuntimeSkillFileAndEmptyParent(path); err != nil {
			return err
		}
	}
	return nil
}

func isRuntimeSkillManagedPath(path string) bool {
	clean := filepath.Clean(path)
	if filepath.Base(clean) != "SKILL.md" {
		return false
	}
	skillDir := filepath.Dir(clean)
	if skillDir == "." || skillDir == string(filepath.Separator) {
		return false
	}
	return filepath.Base(filepath.Dir(skillDir)) == "skills"
}

func runtimeSkillFileAVMManaged(path string) bool {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	name := filepath.Base(filepath.Dir(filepath.Clean(path)))
	return runtimeSkillContentAVMManaged(raw, name)
}

func runtimeSkillContentAVMManaged(raw []byte, name string) bool {
	start, end := skillFrontmatterSpan(raw)
	if start != 0 || end <= 0 {
		return false
	}
	frontmatter := string(raw[start:end])
	if strings.Contains(frontmatter, "avm_managed: true") || strings.Contains(frontmatter, "avm_managed: \"true\"") {
		return true
	}
	if name != "" && strings.Contains(frontmatter, `description: "AVM skill `+name+`."`) {
		return true
	}
	return false
}

func skillFrontmatterSpan(raw []byte) (int, int) {
	trimmed := bytes.TrimLeft(raw, "\ufeff\n\r\t ")
	if len(trimmed) != len(raw) {
		return -1, -1
	}
	if !bytes.HasPrefix(raw, []byte("---\n")) && !bytes.HasPrefix(raw, []byte("---\r\n")) {
		return -1, -1
	}
	lineStart := 3
	if len(raw) > 3 && raw[3] == '\r' {
		lineStart = 5
	} else if len(raw) > 3 && raw[3] == '\n' {
		lineStart = 4
	}
	for lineStart < len(raw) {
		lineEnd := lineStart
		for lineEnd < len(raw) && raw[lineEnd] != '\n' {
			lineEnd++
		}
		line := bytes.TrimSpace(bytes.TrimRight(raw[lineStart:lineEnd], "\r"))
		if bytes.Equal(line, []byte("---")) {
			return 0, lineStart
		}
		lineStart = lineEnd
		if lineStart < len(raw) && raw[lineStart] == '\n' {
			lineStart++
		}
	}
	return -1, -1
}

func removeRuntimeSkillFileAndEmptyParent(path string) error {
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	_ = os.Remove(filepath.Dir(path))
	return nil
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

func runtimeHomesForActivation(active config.ActiveRef, targets []string, overrides map[string]string) map[string]string {
	homes := make(map[string]string)
	for runtime, home := range overrides {
		if home != "" {
			homes[runtime] = home
		}
	}
	for _, runtime := range targets {
		if !runtimeUsesIsolatedHome(runtime) {
			continue
		}
		if homes[runtime] == "" {
			homes[runtime] = config.RuntimeHomeDir(active, runtime)
		}
	}
	if len(homes) == 0 {
		return nil
	}
	return homes
}

func runtimeUsesIsolatedHome(runtime string) bool {
	switch runtime {
	case "codex", "claude-code", "opencode":
		return true
	default:
		return false
	}
}

func resetRuntimeHome(home string) error {
	if home == "" {
		return nil
	}
	clean := filepath.Clean(home)
	parent := filepath.Dir(clean)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return err
	}
	if err := os.RemoveAll(clean); err != nil {
		return err
	}
	return os.MkdirAll(clean, 0o700)
}

type runtimeHomeSidecar struct {
	RelPath string
	Content []byte
	Mode    os.FileMode
}

func captureRuntimeHomeSidecars(runtime, runtimeHome string) ([]runtimeHomeSidecar, error) {
	switch runtime {
	case "codex":
		return captureNamedSidecars([]string{"auth.json"}, codexSidecarSourceDirs(runtimeHome))
	case "claude-code":
		return captureNamedSidecars([]string{".credentials.json", "config.json"}, claudeSidecarSourceDirs(runtimeHome))
	default:
		return nil, nil
	}
}

func codexSidecarSourceDirs(runtimeHome string) []string {
	dirs := []string{runtimeHome}
	if envHome := strings.TrimSpace(os.Getenv("CODEX_HOME")); envHome != "" {
		dirs = append(dirs, envHome)
	}
	if userHome, err := os.UserHomeDir(); err == nil && userHome != "" {
		dirs = append(dirs, filepath.Join(userHome, ".codex"))
	}
	return uniqueCleanPaths(dirs)
}

func claudeSidecarSourceDirs(runtimeHome string) []string {
	dirs := []string{runtimeHome}
	if envHome := strings.TrimSpace(os.Getenv("CLAUDE_CONFIG_DIR")); envHome != "" {
		dirs = append(dirs, envHome)
	}
	if userHome, err := os.UserHomeDir(); err == nil && userHome != "" {
		dirs = append(dirs, filepath.Join(userHome, ".claude"))
	}
	return uniqueCleanPaths(dirs)
}

func captureNamedSidecars(relPaths []string, sourceDirs []string) ([]runtimeHomeSidecar, error) {
	sidecars := make([]runtimeHomeSidecar, 0, len(relPaths))
	for _, relPath := range relPaths {
		relPath = filepath.Clean(filepath.FromSlash(relPath))
		if relPath == "." || filepath.IsAbs(relPath) || strings.HasPrefix(relPath, ".."+string(os.PathSeparator)) {
			return nil, fmt.Errorf("invalid runtime home sidecar path %q", relPath)
		}
		sidecar, ok, err := captureFirstExistingSidecar(relPath, sourceDirs)
		if err != nil {
			return nil, err
		}
		if ok {
			sidecars = append(sidecars, sidecar)
		}
	}
	return sidecars, nil
}

func captureFirstExistingSidecar(relPath string, sourceDirs []string) (runtimeHomeSidecar, bool, error) {
	for _, sourceDir := range sourceDirs {
		if sourceDir == "" {
			continue
		}
		path := filepath.Join(sourceDir, relPath)
		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return runtimeHomeSidecar{}, false, err
		}
		if info.IsDir() {
			continue
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return runtimeHomeSidecar{}, false, err
		}
		return runtimeHomeSidecar{
			RelPath: relPath,
			Content: content,
			Mode:    0o600,
		}, true, nil
	}
	return runtimeHomeSidecar{}, false, nil
}

func restoreRuntimeHomeSidecars(runtimeHome string, sidecars []runtimeHomeSidecar) error {
	for _, sidecar := range sidecars {
		mode := sidecar.Mode
		if mode == 0 {
			mode = 0o600
		}
		path := filepath.Join(runtimeHome, sidecar.RelPath)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(path, sidecar.Content, mode); err != nil {
			return err
		}
	}
	return nil
}

func uniqueCleanPaths(paths []string) []string {
	out := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		if strings.TrimSpace(path) == "" {
			continue
		}
		clean := filepath.Clean(path)
		if _, ok := seen[clean]; ok {
			continue
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	}
	return out
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
