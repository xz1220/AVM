package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/xz1220/agent-vm/internal/adapter"
)

func TestInitImportReportGeneratedReadOnly(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	runtimeRoot := filepath.Join(home, "runtime")
	t.Setenv("HOME", home)
	t.Setenv("PATH", filepath.Join(home, "bin"))
	t.Setenv("CODEX_HOME", filepath.Join(runtimeRoot, "codex"))
	t.Setenv("CLAUDE_CONFIG_DIR", filepath.Join(runtimeRoot, "claude"))
	t.Setenv("CLINE_DATA_HOME", filepath.Join(runtimeRoot, "cline-data"))
	chdirForTest(t, project)

	runtimeFiles := []string{
		writeTestFile(t, filepath.Join(runtimeRoot, "codex", "config.toml"), "model = \"gpt-5\"\n"),
		writeTestFile(t, filepath.Join(runtimeRoot, "claude", "agents", "global-reviewer.md"), "---\nname: global-reviewer\ndescription: global agent\n---\nReview globally.\n"),
		writeTestFile(t, filepath.Join(project, ".claude", "agents", "project-coder.md"), "---\nname: project-coder\ndescription: project agent\n---\nCode locally.\n"),
		writeTestFile(t, filepath.Join(runtimeRoot, "cline-data", "settings", "cline_mcp_settings.json"), "{}\n"),
		writeTestFile(t, filepath.Join(project, ".cursor", "mcp.json"), "{}\n"),
	}
	before := hashFiles(t, runtimeFiles)

	out, err := executeCommand("init")
	if err != nil {
		t.Fatalf("init returned error: %v", err)
	}
	if out != "initialized avm home\n" {
		t.Fatalf("unexpected init output: %q", out)
	}
	if after := hashFiles(t, runtimeFiles); !reflect.DeepEqual(after, before) {
		t.Fatalf("runtime files changed after init:\nbefore=%v\nafter=%v", before, after)
	}

	raw := readTestFile(t, initImportReportPath())
	for _, want := range []string{`"agent_candidates":`, `"warnings":`, `"errors":`} {
		if !strings.Contains(raw, want) {
			t.Fatalf("import report missing stable key %s:\n%s", want, raw)
		}
	}

	report := readInitImportReport(t)
	if report.Version != initImportReportVersion {
		t.Fatalf("report version = %q, want %q", report.Version, initImportReportVersion)
	}
	if _, err := time.Parse(time.RFC3339Nano, report.GeneratedAt); err != nil {
		t.Fatalf("generated_at is not RFC3339Nano: %q", report.GeneratedAt)
	}

	for _, runtimeName := range []string{"claude-code", "cline", "codex", "cursor"} {
		runtimeReport := mustInitRuntimeReport(t, report, runtimeName)
		if !runtimeReport.Found {
			t.Fatalf("%s not marked found in report: %#v", runtimeName, runtimeReport)
		}
		if runtimeReport.ConfigDir == "" {
			t.Fatalf("%s config_dir was empty: %#v", runtimeName, runtimeReport)
		}
	}

	claudeReport := mustInitRuntimeReport(t, report, "claude-code")
	assertImportReportAgent(t, claudeReport, "global-reviewer")
	assertImportReportAgent(t, claudeReport, "project-coder")
}

func TestInitImportReportRecordsImportErrorWithoutFailing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", filepath.Join(home, "bin"))
	withInitReportRegistry(t, staticInitRegistry{
		"codex": &initReportTestAdapter{
			name: "codex",
			detection: adapter.Detection{
				Runtime:   "codex",
				Found:     true,
				ConfigDir: filepath.ToSlash(filepath.Join(home, "runtime", "codex")),
			},
			importErr: errors.New("synthetic import failure"),
		},
	})

	if _, err := executeCommand("init"); err != nil {
		t.Fatalf("init should not fail on adapter import error: %v", err)
	}

	report := readInitImportReport(t)
	codexReport := mustInitRuntimeReport(t, report, "codex")
	if len(codexReport.Errors) != 1 || !strings.Contains(codexReport.Errors[0], "synthetic import failure") {
		t.Fatalf("codex import error not recorded: %#v", codexReport.Errors)
	}
}

func TestInitForceRefreshesImportReportAndPreservesExtraFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PATH", filepath.Join(home, "bin"))

	firstGeneratedAt := time.Date(2026, 4, 26, 10, 0, 0, 0, time.UTC)
	secondGeneratedAt := firstGeneratedAt.Add(time.Minute)
	withInitReportClock(t, firstGeneratedAt)
	withInitReportRegistry(t, staticInitRegistry{
		"codex": &initReportTestAdapter{
			name:      "codex",
			detection: adapter.Detection{Runtime: "codex", Found: true, ConfigDir: "/runtime/codex"},
			result:    &adapter.ImportResult{Runtime: "codex", Warnings: []string{"first scan"}},
		},
	})

	if _, err := executeCommand("init"); err != nil {
		t.Fatalf("initial init returned error: %v", err)
	}
	extraPath := writeTestFile(t, filepath.Join(home, ".avm", "custom", "notes.txt"), "keep me\n")
	initialReport := readInitImportReport(t)
	if initialReport.GeneratedAt != firstGeneratedAt.Format(time.RFC3339Nano) {
		t.Fatalf("initial generated_at = %q", initialReport.GeneratedAt)
	}

	withInitReportClock(t, secondGeneratedAt)
	withInitReportRegistry(t, staticInitRegistry{
		"codex": &initReportTestAdapter{
			name:      "codex",
			detection: adapter.Detection{Runtime: "codex", Found: true, ConfigDir: "/runtime/codex"},
			result:    &adapter.ImportResult{Runtime: "codex", Warnings: []string{"second scan"}},
		},
	})

	if _, err := executeCommand("init", "--force"); err != nil {
		t.Fatalf("forced init returned error: %v", err)
	}
	assertFileContains(t, extraPath, "keep me")

	refreshedReport := readInitImportReport(t)
	if refreshedReport.GeneratedAt != secondGeneratedAt.Format(time.RFC3339Nano) {
		t.Fatalf("forced generated_at = %q, want %q", refreshedReport.GeneratedAt, secondGeneratedAt.Format(time.RFC3339Nano))
	}
	codexReport := mustInitRuntimeReport(t, refreshedReport, "codex")
	if !reflect.DeepEqual(codexReport.Warnings, []string{"second scan"}) {
		t.Fatalf("forced report did not refresh warnings: %#v", codexReport.Warnings)
	}
}

type staticInitRegistry map[string]adapter.Adapter

func (r staticInitRegistry) Get(runtime string) (adapter.Adapter, bool) {
	adp, ok := r[runtime]
	return adp, ok && adp != nil
}

type initReportTestAdapter struct {
	name      string
	detection adapter.Detection
	result    *adapter.ImportResult
	importErr error
}

func (a *initReportTestAdapter) Name() string {
	return a.name
}

func (a *initReportTestAdapter) Detect(ctx adapter.Context) adapter.Detection {
	_ = ctx
	return a.detection
}

func (a *initReportTestAdapter) Import(ctx adapter.Context) (*adapter.ImportResult, error) {
	_ = ctx
	return a.result, a.importErr
}

func (a *initReportTestAdapter) Plan(ctx adapter.Context, input adapter.RenderInput) (*adapter.RenderPlan, error) {
	_ = ctx
	_ = input
	return nil, errors.New("not implemented")
}

func (a *initReportTestAdapter) Render(ctx adapter.Context, plan *adapter.RenderPlan) (*adapter.RenderResult, error) {
	_ = ctx
	_ = plan
	return nil, errors.New("not implemented")
}

func (a *initReportTestAdapter) ManagedPaths(ctx adapter.Context, plan *adapter.RenderPlan) []adapter.ManagedPath {
	_ = ctx
	_ = plan
	return nil
}

func withInitReportRegistry(t *testing.T, registry initAdapterRegistry) {
	t.Helper()
	previous := newInitAdapterRegistry
	newInitAdapterRegistry = func() initAdapterRegistry {
		return registry
	}
	t.Cleanup(func() {
		newInitAdapterRegistry = previous
	})
}

func withInitReportClock(t *testing.T, now time.Time) {
	t.Helper()
	previous := initImportNow
	initImportNow = func() time.Time {
		return now
	}
	t.Cleanup(func() {
		initImportNow = previous
	})
}

func readInitImportReport(t *testing.T) initImportReport {
	t.Helper()
	data, err := os.ReadFile(initImportReportPath())
	if err != nil {
		t.Fatalf("read import report: %v", err)
	}
	var report initImportReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatalf("unmarshal import report: %v\n%s", err, string(data))
	}
	return report
}

func mustInitRuntimeReport(t *testing.T, report initImportReport, runtimeName string) initRuntimeImportReport {
	t.Helper()
	for _, runtimeReport := range report.Runtimes {
		if runtimeReport.Runtime == runtimeName {
			return runtimeReport
		}
	}
	t.Fatalf("report missing runtime %q: %#v", runtimeName, report.Runtimes)
	return initRuntimeImportReport{}
}

func assertImportReportAgent(t *testing.T, runtimeReport initRuntimeImportReport, agentName string) {
	t.Helper()
	for _, agent := range runtimeReport.AgentCandidates {
		if agent.Name == agentName {
			return
		}
	}
	t.Fatalf("%s report missing candidate agent %q: %#v", runtimeReport.Runtime, agentName, runtimeReport.AgentCandidates)
}

func writeTestFile(t *testing.T, path, content string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func readTestFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func hashFiles(t *testing.T, paths []string) map[string]string {
	t.Helper()
	hashes := make(map[string]string, len(paths))
	for _, path := range paths {
		hashes[path] = hashFile(t, path)
	}
	return hashes
}

func hashFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s for hash: %v", path, err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func chdirForTest(t *testing.T, dir string) {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previous); err != nil {
			t.Fatalf("restore cwd %s: %v", previous, err)
		}
	})
}
