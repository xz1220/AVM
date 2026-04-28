package main

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/xz1220/agent-vm/internal/adapter"
)

func TestRuntimeListShowsImportCandidates(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	chdir(t, project)

	if _, err := executeCommand("init"); err != nil {
		t.Fatalf("init returned error: %v", err)
	}
	report := initImportReport{
		Version:     initImportReportVersion,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Runtimes: []initRuntimeImportReport{
			{
				Runtime:   "claude-code",
				Found:     true,
				ConfigDir: "/tmp/claude",
				AgentCandidates: []adapter.ImportedAgent{
					{Name: "reviewer", Description: "Review risky changes"},
				},
			},
			{Runtime: "codex", Found: false},
		},
	}
	if err := saveInitImportReport(initImportReportPath(), report); err != nil {
		t.Fatalf("save import report: %v", err)
	}

	out, err := executeCommand("runtime", "list")
	if err != nil {
		t.Fatalf("runtime list returned error: %v\n%s", err, out)
	}
	for _, want := range []string{
		"RUNTIME\tFOUND\tCANDIDATES\tCONFIG_DIR",
		"claude-code\tyes\t1\t/tmp/claude",
		"IMPORT_CANDIDATE\tSUMMARY\tCREATE",
		"claude-code/reviewer",
		"Review risky changes",
		"avm create --from-import claude-code/reviewer",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("runtime list output missing %q:\n%s", want, out)
		}
	}
}

func TestRuntimeScanRefreshesReport(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	chdir(t, project)

	out, err := executeCommand("runtime", "scan")
	if err != nil {
		t.Fatalf("runtime scan returned error: %v\n%s", err, out)
	}
	if !strings.Contains(out, "runtime import report updated:") {
		t.Fatalf("unexpected runtime scan output:\n%s", out)
	}
	if _, err := os.Stat(initImportReportPath()); err != nil {
		t.Fatalf("import report missing after scan: %v", err)
	}
}

func TestShellTokenQuotesUnsafeRefs(t *testing.T) {
	if got := shellToken("claude-code/code reviewer"); got != "'claude-code/code reviewer'" {
		t.Fatalf("shellToken with space = %q", got)
	}
	if got := shellToken("claude-code/reviewer"); got != "claude-code/reviewer" {
		t.Fatalf("shellToken safe ref = %q", got)
	}
}
