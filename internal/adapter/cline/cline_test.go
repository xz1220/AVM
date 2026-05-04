package cline_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/xz1220/agent-vm/internal/adapter"
	"github.com/xz1220/agent-vm/internal/adapter/cline"
)

func TestAdapterImplementsContract(t *testing.T) {
	var _ adapter.Adapter = (*cline.Adapter)(nil)
}

func TestDetectUsesConfiguredDataDir(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	settingsDir := filepath.Join(dir, "settings")
	if err := os.MkdirAll(settingsDir, 0o700); err != nil {
		t.Fatalf("mkdir settings: %v", err)
	}
	if err := os.WriteFile(filepath.Join(settingsDir, "cline_mcp_settings.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	detection := cline.New(cline.WithDataDir(dir)).Detect(ctx)

	if detection.Runtime != "cline" {
		t.Fatalf("runtime = %q, want cline", detection.Runtime)
	}
	if !detection.Found {
		t.Fatalf("expected configured Cline data dir to be found")
	}
	if detection.ConfigDir != filepath.ToSlash(dir) {
		t.Fatalf("config dir = %q, want %q", detection.ConfigDir, filepath.ToSlash(dir))
	}
}

func TestPlanIsDeterministic(t *testing.T) {
	ctx := context.Background()
	a := cline.New(cline.WithDataDir("/tmp/cline-data"))
	input := richInput("/repo")

	first, err := a.Plan(ctx, input)
	if err != nil {
		t.Fatalf("first plan failed: %v", err)
	}
	second, err := a.Plan(ctx, input)
	if err != nil {
		t.Fatalf("second plan failed: %v", err)
	}

	if !reflect.DeepEqual(first, second) {
		t.Fatalf("plans are not deterministic:\nfirst: %#v\nsecond:%#v", first, second)
	}

	rulesContent := operationContent(t, first, "cline-agent-rules")
	for _, expected := range []string{
		"## Active AVM skills\n\n- git (/active/skills/git/SKILL.md)\n- test (/active/skills/test/SKILL.md)",
		"deny=Bash(rm -rf *)",
	} {
		if !strings.Contains(rulesContent, expected) {
			t.Fatalf("rules content missing deterministic block %q:\n%s", expected, rulesContent)
		}
	}

	mcpContent := operationContent(t, first, "cline-mcp-settings")
	if !strings.Contains(mcpContent, `"GITHUB_TOKEN": "${GITHUB_TOKEN}"`) {
		t.Fatalf("mcp content expanded or omitted env reference:\n%s", mcpContent)
	}
	if strings.Index(mcpContent, `"github"`) > strings.Index(mcpContent, `"postgres"`) {
		t.Fatalf("mcp servers were not sorted deterministically:\n%s", mcpContent)
	}
}

func TestPlanMappingsCoverRenderedIgnoredAndUnsupportedFields(t *testing.T) {
	temperature := 0.2
	input := richInput("/repo")
	input.Agent.Model.Temperature = &temperature
	input.Agent.Permissions.AdditionalDirectories = []string{"/outside"}
	input.Capabilities.Commands = []adapter.CapabilityRef{{Name: "deploy"}}
	input.Capabilities.Hooks = []adapter.CapabilityRef{{Name: "preflight"}}
	input.Capabilities.MCPServers = append(input.Capabilities.MCPServers, adapter.MCPServer{Name: "missing"})

	plan, err := cline.New(cline.WithDataDir("/tmp/cline-data")).Plan(context.Background(), input)
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}

	for _, mapping := range plan.Mappings {
		if !mapping.Status.Valid() {
			t.Fatalf("mapping %s used invalid status %q", mapping.SourcePath, mapping.Status)
		}
	}

	assertMapping(t, plan, "agent.instructions.system", adapter.MappingRenderedAsInstructions)
	assertMapping(t, plan, "agent.model.model", adapter.MappingRenderedAsInstructions)
	assertMapping(t, plan, "agent.permissions.approval", adapter.MappingRenderedAsInstructions)
	assertMapping(t, plan, "capabilities.skills", adapter.MappingRenderedAsInstructions)
	assertMapping(t, plan, "capabilities.mcp_servers.github", adapter.MappingNative)
	assertMapping(t, plan, "project.clinerules", adapter.MappingIgnored)
	assertMapping(t, plan, "project.AGENTS.md", adapter.MappingIgnored)
	assertMapping(t, plan, "agent.model.temperature", adapter.MappingUnsupported)
	assertMapping(t, plan, "agent.permissions.additional_directories", adapter.MappingUnsupported)
	assertMapping(t, plan, "capabilities.mcp_servers.missing", adapter.MappingUnsupported)
}

func TestRenderWritesManagedPathsAndPreservesUserConfig(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dataDir := filepath.Join(root, "cline-data")
	projectRoot := filepath.Join(root, "project")
	a := cline.New(cline.WithDataDir(dataDir))
	input := richInput(projectRoot)

	settingsPath := filepath.Join(dataDir, "settings", "cline_mcp_settings.json")
	unmanagedRulesPath := filepath.Join(projectRoot, ".clinerules", "user.md")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o700); err != nil {
		t.Fatalf("mkdir settings: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(unmanagedRulesPath), 0o700); err != nil {
		t.Fatalf("mkdir rules: %v", err)
	}
	existingSettings := `{
  "mcpServers": {
    "user": {
      "command": "user-mcp",
      "env": {
        "SECRET": "${DO_NOT_EXPAND}"
      }
    }
  },
  "otherSetting": true
}
`
	if err := os.WriteFile(settingsPath, []byte(existingSettings), 0o600); err != nil {
		t.Fatalf("write existing settings: %v", err)
	}
	if err := os.WriteFile(unmanagedRulesPath, []byte("user-owned\n"), 0o600); err != nil {
		t.Fatalf("write unmanaged rules: %v", err)
	}

	plan, err := a.Plan(ctx, input)
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}

	result, err := a.Render(ctx, plan)
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if !operationChanged(result, "cline-agent-rules") {
		t.Fatalf("cline-agent-rules should report changed")
	}
	if !operationChanged(result, "cline-mcp-settings") {
		t.Fatalf("cline-mcp-settings should report changed")
	}

	rulesPath := filepath.Join(projectRoot, ".clinerules", "avm", "backend-coder.md")
	rulesContent := readFile(t, rulesPath)
	for _, expected := range []string{
		"# AVM Agent: backend-coder",
		"You implement backend changes with tests.",
		"Active AVM skills",
	} {
		if !strings.Contains(rulesContent, expected) {
			t.Fatalf("rendered rules missing %q:\n%s", expected, rulesContent)
		}
	}

	settingsContent := readFile(t, settingsPath)
	for _, expected := range []string{
		`"user"`,
		`"SECRET": "${DO_NOT_EXPAND}"`,
		`"github"`,
		`"GITHUB_TOKEN": "${GITHUB_TOKEN}"`,
		`"postgres"`,
		`"managed_mcp_servers"`,
		`"otherSetting": true`,
	} {
		if !strings.Contains(settingsContent, expected) {
			t.Fatalf("rendered settings missing %q:\n%s", expected, settingsContent)
		}
	}
	if got := readFile(t, unmanagedRulesPath); got != "user-owned\n" {
		t.Fatalf("unmanaged rules file changed: %q", got)
	}

	second, err := a.Render(ctx, plan)
	if err != nil {
		t.Fatalf("second render failed: %v", err)
	}
	for _, id := range []string{"cline-agent-rules", "cline-mcp-settings"} {
		if operationChanged(second, id) {
			t.Fatalf("%s should be unchanged on second render", id)
		}
	}
}

func TestRenderDoesNotOverwriteUserOwnedMCPServer(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	dataDir := filepath.Join(root, "cline-data")
	projectRoot := filepath.Join(root, "project")
	a := cline.New(cline.WithDataDir(dataDir))
	input := richInput(projectRoot)
	input.Capabilities.MCPServers = []adapter.MCPServer{
		{Name: "github", Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-github"}},
	}

	settingsPath := filepath.Join(dataDir, "settings", "cline_mcp_settings.json")
	if err := os.MkdirAll(filepath.Dir(settingsPath), 0o700); err != nil {
		t.Fatalf("mkdir settings: %v", err)
	}
	existingSettings := `{
  "mcpServers": {
    "github": {
      "command": "user-github"
    }
  }
}
`
	if err := os.WriteFile(settingsPath, []byte(existingSettings), 0o600); err != nil {
		t.Fatalf("write existing settings: %v", err)
	}

	plan, err := a.Plan(ctx, input)
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}
	result, err := a.Render(ctx, plan)
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}

	settingsContent := readFile(t, settingsPath)
	if !strings.Contains(settingsContent, `"command": "user-github"`) {
		t.Fatalf("user-owned github server was overwritten:\n%s", settingsContent)
	}
	if strings.Contains(settingsContent, `@modelcontextprotocol/server-github`) {
		t.Fatalf("desired AVM github server overwrote user-owned server:\n%s", settingsContent)
	}
	if operationChanged(result, "cline-mcp-settings") {
		t.Fatalf("conflicting user-owned MCP server should not rewrite settings")
	}
	if !containsWarning(result.Warnings, `github`) {
		t.Fatalf("expected warning for user-owned github server, got %v", result.Warnings)
	}
}

func TestRenderRejectsOperationsOutsideManagedPaths(t *testing.T) {
	dir := t.TempDir()
	a := cline.New(cline.WithDataDir(filepath.Join(dir, "cline-data")))
	managedPath := filepath.Join(dir, "project", ".clinerules", "avm", "backend-coder.md")
	unmanagedPath := filepath.Join(dir, "project", ".clinerules", "user.md")
	if err := os.MkdirAll(filepath.Dir(unmanagedPath), 0o700); err != nil {
		t.Fatalf("mkdir unmanaged parent: %v", err)
	}
	if err := os.WriteFile(unmanagedPath, []byte("keep\n"), 0o600); err != nil {
		t.Fatalf("write unmanaged file: %v", err)
	}

	plan := &adapter.RenderPlan{
		Runtime:   "cline",
		AgentName: "backend-coder",
		ManagedPaths: []adapter.ManagedPath{
			{Path: managedPath, Owner: "avm", Required: true, MergeMode: adapter.MergeModeWholeFile},
		},
		Operations: []adapter.RenderOperation{
			{ID: "rogue", Action: adapter.OperationWriteFile, Path: unmanagedPath, Content: []byte("changed\n"), Required: true},
		},
	}

	_, err := a.Render(context.Background(), plan)
	if err == nil {
		t.Fatalf("render unexpectedly accepted unmanaged operation")
	}
	if got := readFile(t, unmanagedPath); got != "keep\n" {
		t.Fatalf("unmanaged file changed despite render error: %q", got)
	}
}

func TestManagedPathsReturnsCopy(t *testing.T) {
	a := cline.New(cline.WithDataDir("/tmp/cline-data"))
	plan, err := a.Plan(context.Background(), richInput("/repo"))
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}

	paths := a.ManagedPaths(context.Background(), plan)
	if len(paths) == 0 {
		t.Fatalf("expected managed paths")
	}
	paths[0].Path = "mutated"

	again := a.ManagedPaths(context.Background(), plan)
	if again[0].Path == "mutated" {
		t.Fatalf("ManagedPaths returned mutable plan backing slice")
	}
}

func TestFixturePlanShape(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "adapter", "cline", "phase1_render_plan.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	var fixture struct {
		Schema       string                 `json:"fixture_schema"`
		Runtime      string                 `json:"runtime"`
		ManagedPaths []adapter.ManagedPath  `json:"managed_paths"`
		Mappings     []adapter.FieldMapping `json:"mappings"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	if fixture.Schema != "avm.phase1.adapter-render-plan.v1" {
		t.Fatalf("fixture schema = %q", fixture.Schema)
	}
	if fixture.Runtime != "cline" {
		t.Fatalf("fixture runtime = %q", fixture.Runtime)
	}
	if len(fixture.ManagedPaths) == 0 {
		t.Fatalf("fixture has no managed paths")
	}
	for _, mapping := range fixture.Mappings {
		if !mapping.Status.Valid() {
			t.Fatalf("fixture mapping %s has invalid status %q", mapping.SourcePath, mapping.Status)
		}
	}
}

func richInput(projectRoot string) adapter.RenderInput {
	return adapter.RenderInput{
		Active:  adapter.ActiveRef{Kind: "env", Name: "coding"},
		Runtime: "cline",
		Agent: adapter.Agent{
			Name:        "backend-coder",
			Description: "Backend implementation agent",
			Instructions: adapter.Instructions{
				System:    "You implement backend changes with tests.",
				Developer: "Prefer small, reviewable changes.",
				References: []string{
					"/active/docs/z.md",
					"/active/docs/a.md",
				},
			},
			Model: adapter.ModelConfig{
				Model:           "gpt-5.4",
				ReasoningEffort: "medium",
				Verbosity:       "normal",
			},
			Permissions: adapter.PermissionConfig{
				Approval: "on-request",
				Sandbox:  "workspace-write",
				Allow: []string{
					"Bash(go test ./...)",
					"Bash(git status --short)",
				},
				Deny: []string{
					"Bash(rm -rf *)",
				},
			},
		},
		Capabilities: adapter.CapabilitySet{
			Skills: []adapter.CapabilityRef{
				{Name: "test", Path: "/active/skills/test/SKILL.md"},
				{Name: "git", Path: "/active/skills/git/SKILL.md"},
			},
			MCPServers: []adapter.MCPServer{
				{
					Name:    "postgres",
					Command: "postgres-mcp",
				},
				{
					Name:    "github",
					Command: "npx",
					Args:    []string{"-y", "@modelcontextprotocol/server-github"},
					Env:     []adapter.EnvVar{{Name: "GITHUB_TOKEN", Value: "${GITHUB_TOKEN}"}},
				},
			},
			Toolsets: []adapter.Toolset{
				{Name: "browser", Mode: "disabled"},
				{Name: "shell", Mode: "limited"},
			},
		},
		ProjectRoot: projectRoot,
	}
}

func operationContent(t *testing.T, plan *adapter.RenderPlan, id string) string {
	t.Helper()

	for _, operation := range plan.Operations {
		if operation.ID == id {
			return string(operation.Content)
		}
	}
	t.Fatalf("operation %q not found", id)
	return ""
}

func assertMapping(t *testing.T, plan *adapter.RenderPlan, sourcePath string, status adapter.MappingStatus) {
	t.Helper()

	for _, mapping := range plan.Mappings {
		if mapping.SourcePath == sourcePath {
			if mapping.Status != status {
				t.Fatalf("mapping %s status = %q, want %q", sourcePath, mapping.Status, status)
			}
			return
		}
	}
	t.Fatalf("mapping %s not found in %#v", sourcePath, plan.Mappings)
}

func operationChanged(result *adapter.RenderResult, operationID string) bool {
	for _, operation := range result.Operations {
		if operation.OperationID == operationID {
			return operation.Changed
		}
	}
	return false
}

func containsWarning(warnings []string, needle string) bool {
	for _, warning := range warnings {
		if strings.Contains(warning, needle) {
			return true
		}
	}
	return false
}

func readFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
