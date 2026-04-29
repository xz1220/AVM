package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xz1220/agent-vm/internal/adapter"
	"github.com/xz1220/agent-vm/internal/config"
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

func TestRuntimeScanBootstrapsNativeSkillsAndMCPs(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)
	chdir(t, project)

	writeFileForTest(t, filepath.Join(home, ".codex", "skills", "codex-probe", "SKILL.md"), "# Codex Probe\n\nCodex skill.\n")
	writeFileForTest(t, filepath.Join(home, ".codex", "config.toml"), `[mcp_servers.github]
command = "gh"
args = ["mcp", "serve"]
env = { GITHUB_TOKEN = "${GITHUB_TOKEN}" }
`)
	writeFileForTest(t, filepath.Join(home, ".claude", "skills", "claude-probe", "SKILL.md"), "# Claude Probe\n\nClaude skill.\n")
	writeFileForTest(t, filepath.Join(home, ".claude", "mcp.json"), `{"mcpServers":{"playwright":{"command":"npx","args":["@playwright/mcp"],"env":{"TOKEN":"${TOKEN}"}}}}`)
	writeFileForTest(t, filepath.Join(home, ".config", "opencode", "skills", "opencode-probe", "SKILL.md"), "# OpenCode Probe\n\nOpenCode skill.\n")
	writeFileForTest(t, filepath.Join(home, ".config", "opencode", "opencode.json"), `{"mcp":{"docs":{"type":"remote","url":"https://example.com/mcp","headers":{"Authorization":"Bearer ${TOKEN}"}}}}`)

	out, err := executeCommand("runtime", "scan")
	if err != nil {
		t.Fatalf("runtime scan returned error: %v\n%s", err, out)
	}
	for _, name := range []string{"codex-probe", "claude-probe", "opencode-probe"} {
		if _, err := os.Stat(config.SkillRegistryFilePath(name)); err != nil {
			t.Fatalf("skill %s was not bootstrapped: %v", name, err)
		}
	}

	github, _, err := config.ReadMCPRegistryEntry("github")
	if err != nil {
		t.Fatalf("github mcp was not bootstrapped: %v", err)
	}
	if github.Server.Command != "gh" || strings.Join(github.Server.Args, ",") != "mcp,serve" || github.Server.Env["GITHUB_TOKEN"] != "${GITHUB_TOKEN}" {
		t.Fatalf("unexpected github mcp: %#v", github.Server)
	}
	playwright, _, err := config.ReadMCPRegistryEntry("playwright")
	if err != nil {
		t.Fatalf("playwright mcp was not bootstrapped: %v", err)
	}
	if playwright.Server.Command != "npx" || strings.Join(playwright.Server.Args, ",") != "@playwright/mcp" {
		t.Fatalf("unexpected playwright mcp: %#v", playwright.Server)
	}
	docs, _, err := config.ReadMCPRegistryEntry("docs")
	if err != nil {
		t.Fatalf("docs mcp was not bootstrapped: %v", err)
	}
	if docs.Server.URL != "https://example.com/mcp" || docs.Server.Headers["Authorization"] != "Bearer ${TOKEN}" {
		t.Fatalf("unexpected docs mcp: %#v", docs.Server)
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
