package sync

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xz1220/agent-vm/internal/adapter/fake"
	"github.com/xz1220/agent-vm/internal/config"
	"github.com/xz1220/agent-vm/internal/state"
)

func TestSyncActivationFakeAdapterRenderSuccess(t *testing.T) {
	dir := t.TempDir()
	syncer := testSyncer()

	result, err := syncer.SyncActivation(context.Background(), testResolved("fake", "backend"), testOptions(dir))
	if err != nil {
		t.Fatalf("sync activation failed: %v", err)
	}
	if len(result.Targets) != 1 || result.Targets[0].Status != TargetStatusSynced {
		t.Fatalf("unexpected targets: %#v", result.Targets)
	}

	renderedPath := filepath.Join(dir, "project", ".avm-fake", "fake", "backend.rendered")
	assertFileContains(t, renderedPath, "agent: backend")
	assertFileExists(t, filepath.Join(dir, "active", "manifest.yaml"))

	syncState, err := state.LoadSyncState(filepath.Join(dir, "state", "sync-state.json"))
	if err != nil {
		t.Fatalf("load sync state: %v", err)
	}
	runtimeState := syncState.Runtimes["fake"]
	if runtimeState.Status != state.RuntimeStatusSynced {
		t.Fatalf("runtime state status = %q, want synced", runtimeState.Status)
	}
	if len(runtimeState.ManagedPaths) != 1 || runtimeState.ManagedPaths[0].FileHash == "" {
		t.Fatalf("managed path hash was not recorded: %#v", runtimeState.ManagedPaths)
	}
}

func TestSyncActivationDryRunDoesNotWriteFiles(t *testing.T) {
	dir := t.TempDir()
	opts := testOptions(dir)
	opts.DryRun = true

	result, err := testSyncer().SyncActivation(context.Background(), testResolved("fake", "backend"), opts)
	if err != nil {
		t.Fatalf("dry-run sync activation failed: %v", err)
	}
	if len(result.Targets) != 1 || result.Targets[0].Status != TargetStatusSynced || result.Targets[0].Plan == nil {
		t.Fatalf("dry-run did not return a successful plan result: %#v", result.Targets)
	}

	assertFileMissing(t, filepath.Join(dir, "active", "manifest.yaml"))
	assertFileMissing(t, filepath.Join(dir, "project", ".avm-fake", "fake", "backend.rendered"))
	assertFileMissing(t, filepath.Join(dir, "state", "sync-state.json"))
}

func TestSyncActivationDetectsManagedPathConflict(t *testing.T) {
	dir := t.TempDir()
	syncer := testSyncer()
	opts := testOptions(dir)
	resolved := testResolved("fake", "backend")

	if _, err := syncer.SyncActivation(context.Background(), resolved, opts); err != nil {
		t.Fatalf("initial sync activation failed: %v", err)
	}

	renderedPath := filepath.Join(dir, "project", ".avm-fake", "fake", "backend.rendered")
	if err := os.WriteFile(renderedPath, []byte("external change\n"), 0o600); err != nil {
		t.Fatalf("modify rendered path: %v", err)
	}

	result, err := syncer.SyncActivation(context.Background(), resolved, opts)
	if err == nil {
		t.Fatalf("expected conflict error")
	}
	if result == nil || len(result.Targets) != 1 || result.Targets[0].Status != TargetStatusFailed {
		t.Fatalf("unexpected conflict result: %#v", result)
	}
	if !strings.Contains(result.Targets[0].Error, "conflict detected") {
		t.Fatalf("target error did not describe conflict: %q", result.Targets[0].Error)
	}
	assertFileContains(t, renderedPath, "external change")
}

func TestMissingRuntimeAdapterReturnsSkippedTarget(t *testing.T) {
	dir := t.TempDir()
	syncer := NewSyncer(StaticAdapterRegistry{})
	syncer.Now = fixedNow

	result, err := syncer.SyncActivation(context.Background(), testResolved("missing", "backend"), testOptions(dir))
	if err != nil {
		t.Fatalf("missing adapter should not fail activation: %v", err)
	}
	if len(result.Targets) != 1 || result.Targets[0].Status != TargetStatusSkipped {
		t.Fatalf("missing adapter did not produce skipped target: %#v", result.Targets)
	}
}

func TestRebuildActiveFailurePreservesOldActive(t *testing.T) {
	dir := t.TempDir()
	activeDir := filepath.Join(dir, "active")
	if err := os.MkdirAll(activeDir, 0o700); err != nil {
		t.Fatalf("mkdir active: %v", err)
	}
	oldManifest := filepath.Join(activeDir, "manifest.yaml")
	if err := os.WriteFile(oldManifest, []byte("old active\n"), 0o600); err != nil {
		t.Fatalf("write old manifest: %v", err)
	}

	err := RebuildActive(&config.ResolvedActivation{
		Active: config.ActiveRef{Kind: config.ActiveKindProfile, Name: "bad"},
		RuntimeAgents: map[string]config.AgentProfile{
			"fake": {Name: "../bad"},
		},
		Targets: []string{"fake"},
	}, activeDir)
	if err == nil {
		t.Fatalf("expected active rebuild failure")
	}
	assertFileContains(t, oldManifest, "old active")
}

func TestSyncActivationFailedActiveRebuildDoesNotRender(t *testing.T) {
	dir := t.TempDir()
	opts := testOptions(dir)
	resolved := testResolved("fake", "../bad")

	result, err := testSyncer().SyncActivation(context.Background(), resolved, opts)
	if err == nil {
		t.Fatalf("expected active rebuild error")
	}
	if result != nil {
		t.Fatalf("active rebuild failure should happen before target render: %#v", result)
	}
	assertFileMissing(t, filepath.Join(dir, "project", ".avm-fake", "fake", "../bad.rendered"))
}

func TestRebuildActiveLinksResolvedSkills(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "registry", "skills", "probe-skill")
	if err := os.MkdirAll(skillDir, 0o700); err != nil {
		t.Fatalf("create skill dir: %v", err)
	}
	skillPath := filepath.Join(skillDir, "SKILL.md")
	if err := os.WriteFile(skillPath, []byte("AVM_SKILL_PROBE_MARKER_20260426\n"), 0o600); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	resolved := testResolved("fake", "backend")
	resolved.Capabilities["fake"] = config.ResolvedCapabilities{
		Skills: []string{"probe-skill"},
		SkillRefs: []config.ResolvedSkill{
			{Name: "probe-skill", SourceDir: skillDir, SourcePath: skillPath},
		},
	}

	activeDir := filepath.Join(dir, "active")
	if err := RebuildActive(resolved, activeDir); err != nil {
		t.Fatalf("RebuildActive returned error: %v", err)
	}

	assertFileContains(t, filepath.Join(activeDir, "skills", "probe-skill", "SKILL.md"), "AVM_SKILL_PROBE_MARKER_20260426")
	assertFileContains(t, filepath.Join(activeDir, "manifest.yaml"), "probe-skill")
}

func TestSyncActivationCleansStaleRuntimeSkillsFromPreviousActivation(t *testing.T) {
	dir := t.TempDir()
	opts := testOptions(dir)
	statePath := opts.StatePath
	oldActive := config.ActiveRef{Kind: config.ActiveKindEnv, Name: "skill-env"}
	syncState := state.NewSyncState(oldActive)

	staleSkillPath := filepath.Join(dir, "claude-home", "skills", "probe-skill", "SKILL.md")
	userSkillPath := filepath.Join(dir, "claude-home", "skills", "user-skill", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(staleSkillPath), 0o700); err != nil {
		t.Fatalf("create stale skill dir: %v", err)
	}
	if err := os.WriteFile(staleSkillPath, []byte("---\nname: \"probe-skill\"\ndescription: \"AVM skill probe-skill.\"\n---\n\nold\n"), 0o600); err != nil {
		t.Fatalf("write stale skill: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(userSkillPath), 0o700); err != nil {
		t.Fatalf("create user skill dir: %v", err)
	}
	if err := os.WriteFile(userSkillPath, []byte("---\nname: user-skill\ndescription: user-owned\n---\n\nkeep\n"), 0o600); err != nil {
		t.Fatalf("write user skill: %v", err)
	}
	syncState.Runtimes["claude-code"] = state.RuntimeState{
		Runtime: "claude-code",
		Status:  state.RuntimeStatusSynced,
		Active:  oldActive,
		ManagedPaths: []state.ManagedPathState{
			{Path: staleSkillPath, Owner: "avm", MergeMode: "whole-file"},
			{Path: userSkillPath, Owner: "avm", MergeMode: "whole-file"},
		},
	}
	if err := state.SaveSyncState(statePath, syncState); err != nil {
		t.Fatalf("save sync state: %v", err)
	}

	if _, err := testSyncer().SyncActivation(context.Background(), testResolved("fake", "backend"), opts); err != nil {
		t.Fatalf("sync activation failed: %v", err)
	}

	assertFileMissing(t, staleSkillPath)
	assertFileContains(t, userSkillPath, "user-owned")
}

func TestRuntimeHomeSidecarsUseRealHomeFallback(t *testing.T) {
	dir := t.TempDir()
	isolatedHome := filepath.Join(dir, "isolated-home")
	realHome := filepath.Join(dir, "real-home")
	t.Setenv("HOME", isolatedHome)
	t.Setenv("AVM_REAL_HOME", realHome)
	t.Setenv("CODEX_HOME", "")
	t.Setenv("CLAUDE_CONFIG_DIR", "")

	writeTestFile(t, filepath.Join(realHome, ".codex", "auth.json"), "codex-auth\n")
	writeTestFile(t, filepath.Join(realHome, ".claude", ".credentials.json"), "claude-credentials\n")
	writeTestFile(t, filepath.Join(realHome, ".claude", "config.json"), "claude-config\n")

	codexHome := filepath.Join(dir, "runtime-codex")
	codexSidecars, err := captureRuntimeHomeSidecars("codex", codexHome)
	if err != nil {
		t.Fatalf("capture codex sidecars: %v", err)
	}
	if err := os.MkdirAll(codexHome, 0o700); err != nil {
		t.Fatalf("create codex home: %v", err)
	}
	if err := restoreRuntimeHomeSidecars(codexHome, codexSidecars); err != nil {
		t.Fatalf("restore codex sidecars: %v", err)
	}
	assertFileContains(t, filepath.Join(codexHome, "auth.json"), "codex-auth")

	claudeHome := filepath.Join(dir, "runtime-claude")
	claudeSidecars, err := captureRuntimeHomeSidecars("claude-code", claudeHome)
	if err != nil {
		t.Fatalf("capture claude sidecars: %v", err)
	}
	if err := os.MkdirAll(claudeHome, 0o700); err != nil {
		t.Fatalf("create claude home: %v", err)
	}
	if err := restoreRuntimeHomeSidecars(claudeHome, claudeSidecars); err != nil {
		t.Fatalf("restore claude sidecars: %v", err)
	}
	assertFileContains(t, filepath.Join(claudeHome, ".credentials.json"), "claude-credentials")
	assertFileContains(t, filepath.Join(claudeHome, "config.json"), "claude-config")
}

func testSyncer() *Syncer {
	syncer := NewSyncer(StaticAdapterRegistry{
		"fake": fake.New(fake.WithName("fake")),
	})
	syncer.Now = fixedNow
	return syncer
}

func fixedNow() time.Time {
	return time.Date(2026, 4, 24, 10, 30, 0, 0, time.UTC)
}

func testOptions(dir string) Options {
	return Options{
		ProjectRoot: filepath.Join(dir, "project"),
		ActiveDir:   filepath.Join(dir, "active"),
		StatePath:   filepath.Join(dir, "state", "sync-state.json"),
		BackupDir:   filepath.Join(dir, "backup"),
	}
}

func testResolved(runtime, agentName string) *config.ResolvedActivation {
	return &config.ResolvedActivation{
		Active: config.ActiveRef{Kind: config.ActiveKindProfile, Name: agentName},
		RuntimeAgents: map[string]config.AgentProfile{
			runtime: {
				Name:        agentName,
				Description: "Backend implementation agent",
				Runtime:     config.RuntimePreferences{Preferred: runtime},
				Instructions: config.Instructions{
					System:    "System text",
					Developer: "Developer text",
				},
				Permissions: config.Permissions{
					Approval: "on-request",
					Sandbox:  "workspace-write",
				},
				Capabilities: config.CapabilityRefs{
					Skills: []string{"git", "test"},
					MCPs:   []string{"github"},
				},
			},
		},
		Capabilities: map[string]config.ResolvedCapabilities{
			runtime: {
				Skills: []string{"git", "test"},
				MCPs:   []string{"github"},
			},
		},
		Targets: []string{runtime},
	}
}

func assertFileExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected %s to exist: %v", path, err)
	}
}

func assertFileMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected %s to be missing, stat err: %v", path, err)
	}
}

func assertFileContains(t *testing.T, path, expected string) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !strings.Contains(string(raw), expected) {
		t.Fatalf("%s did not contain %q:\n%s", path, expected, string(raw))
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("create %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
