package claude_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/xz1220/agent-vm/internal/adapter"
	"github.com/xz1220/agent-vm/internal/adapter/claude"
)

func TestAdapterImplementsContract(t *testing.T) {
	var _ adapter.Adapter = (*claude.Adapter)(nil)
}

func TestDetectUsesConfiguredClaudeHome(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write settings: %v", err)
	}

	detection := claude.New(claude.WithConfigDir(dir)).Detect(ctx)

	if detection.Runtime != "claude-code" {
		t.Fatalf("runtime = %q, want claude-code", detection.Runtime)
	}
	if !detection.Found {
		t.Fatalf("expected configured Claude Code home to be found")
	}
	if detection.ConfigDir != filepath.ToSlash(dir) {
		t.Fatalf("config dir = %q, want %q", detection.ConfigDir, filepath.ToSlash(dir))
	}
}

func TestPlanIsDeterministic(t *testing.T) {
	ctx := context.Background()
	a := claude.New()
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

	agentContent := operationContent(t, first, "claude-agent")
	for _, expected := range []string{
		"skills:\n  - \"git\"\n  - \"test\"",
		"mcpServers:\n  - \"github\"\n  - \"postgres\"",
		"Active AVM skills:\n- git (/active/skills/git/SKILL.md)\n- test (/active/skills/test/SKILL.md)",
		"Permission approval policy:\non-request",
	} {
		if !strings.Contains(agentContent, expected) {
			t.Fatalf("agent content missing deterministic block %q:\n%s", expected, agentContent)
		}
	}

	mcpContent := operationContent(t, first, "claude-mcp")
	if !strings.Contains(mcpContent, "\"GITHUB_TOKEN\": \"${GITHUB_TOKEN}\"") {
		t.Fatalf("mcp content expanded or omitted env reference:\n%s", mcpContent)
	}
	if strings.Index(mcpContent, "\"github\"") > strings.Index(mcpContent, "\"postgres\"") {
		t.Fatalf("mcp servers were not sorted deterministically:\n%s", mcpContent)
	}
}

func TestPlanMappingsCoverNativeRenderedIgnoredAndUnsupportedFields(t *testing.T) {
	temperature := 0.2
	input := richInput("/repo")
	input.Agent.Model.Temperature = &temperature
	input.Agent.Permissions.AdditionalDirectories = []string{"/outside"}
	input.Capabilities.Commands = []adapter.CapabilityRef{{Name: "deploy"}}
	input.Capabilities.Hooks = []adapter.CapabilityRef{{Name: "preflight"}}
	input.Capabilities.MCPServers = append(input.Capabilities.MCPServers, adapter.MCPServer{Name: "missing"})

	plan, err := claude.New().Plan(context.Background(), input)
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}

	for _, mapping := range plan.Mappings {
		if !mapping.Status.Valid() {
			t.Fatalf("mapping %s used invalid status %q", mapping.SourcePath, mapping.Status)
		}
	}

	assertMapping(t, plan, "agent.model.model", adapter.MappingNative)
	assertMapping(t, plan, "capabilities.skills", adapter.MappingNative)
	assertMapping(t, plan, "agent.instructions.references", adapter.MappingRenderedAsInstructions)
	assertMapping(t, plan, "agent.permissions.approval", adapter.MappingRenderedAsInstructions)
	assertMapping(t, plan, "project.CLAUDE.md", adapter.MappingIgnored)
	assertMapping(t, plan, "agent.model.temperature", adapter.MappingUnsupported)
	assertMapping(t, plan, "agent.permissions.additional_directories", adapter.MappingUnsupported)
	assertMapping(t, plan, "capabilities.mcp_servers.missing", adapter.MappingUnsupported)
}

func TestRenderWritesManagedPathsAsIsolatedHomeAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	project := t.TempDir()
	configDir := t.TempDir()
	a := claude.New(claude.WithConfigDir(configDir))
	input := richInput(project)

	mcpPath := filepath.Join(configDir, "mcp.json")
	unmanagedPath := filepath.Join(project, "CLAUDE.md")
	existingMCP := `{
  "mcpServers": {
    "stale": {
      "command": "old-avm"
    },
    "user": {
      "command": "user-owned",
      "env": {
        "USER_TOKEN": "${DO_NOT_EXPAND}"
      }
    }
  },
  "_avm": {
    "claude-code": {
      "managedMCPServers": [
        "stale"
      ]
    }
  }
}
`
	if err := os.WriteFile(mcpPath, []byte(existingMCP), 0o600); err != nil {
		t.Fatalf("write existing mcp: %v", err)
	}
	if err := os.WriteFile(unmanagedPath, []byte("user-owned\n"), 0o600); err != nil {
		t.Fatalf("write unmanaged file: %v", err)
	}

	plan, err := a.Plan(ctx, input)
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}

	result, err := a.Render(ctx, plan)
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if !operationChanged(result, "claude-agent") {
		t.Fatalf("claude-agent should report changed")
	}
	if !operationChanged(result, "claude-mcp") {
		t.Fatalf("claude-mcp should report changed")
	}

	agentPath := filepath.Join(configDir, "agents", "backend-coder.md")
	agentContent := readFile(t, agentPath)
	for _, expected := range []string{
		"---\nname: \"backend-coder\"",
		"description: \"Backend implementation agent\"",
		"Developer instructions:\nPrefer small, reviewable changes.",
	} {
		if !strings.Contains(agentContent, expected) {
			t.Fatalf("rendered agent missing %q:\n%s", expected, agentContent)
		}
	}

	mcpContent := readFile(t, mcpPath)
	for _, expected := range []string{
		"\"github\"",
		"\"GITHUB_TOKEN\": \"${GITHUB_TOKEN}\"",
	} {
		if !strings.Contains(mcpContent, expected) {
			t.Fatalf("rendered mcp missing %q:\n%s", expected, mcpContent)
		}
	}
	for _, old := range []string{"\"stale\"", "\"user\"", "\"USER_TOKEN\"", "\"managedMCPServers\""} {
		if strings.Contains(mcpContent, old) {
			t.Fatalf("isolated mcp config kept old content %q:\n%s", old, mcpContent)
		}
	}
	if got := readFile(t, unmanagedPath); got != "user-owned\n" {
		t.Fatalf("unmanaged file changed: %q", got)
	}

	second, err := a.Render(ctx, plan)
	if err != nil {
		t.Fatalf("second render failed: %v", err)
	}
	for _, id := range []string{"claude-agent", "claude-settings", "claude-mcp"} {
		if operationChanged(second, id) {
			t.Fatalf("%s should be unchanged on second render", id)
		}
	}
}

func TestRenderRejectsOperationsOutsideManagedPaths(t *testing.T) {
	dir := t.TempDir()
	a := claude.New()
	managedPath := filepath.Join(dir, ".claude", "agents", "backend-coder.md")
	unmanagedPath := filepath.Join(dir, "unmanaged.md")
	if err := os.WriteFile(unmanagedPath, []byte("keep\n"), 0o600); err != nil {
		t.Fatalf("write unmanaged file: %v", err)
	}

	plan := &adapter.RenderPlan{
		Runtime:   "claude-code",
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

func TestRenderIsolatedMCPReplacesExistingConfig(t *testing.T) {
	project := t.TempDir()
	configDir := t.TempDir()
	a := claude.New(claude.WithConfigDir(configDir))
	input := richInput(project)
	mcpPath := filepath.Join(configDir, "mcp.json")
	existing := `{
  "mcpServers": {
    "github": {
      "command": "user-owned"
    }
  }
}
`
	if err := os.WriteFile(mcpPath, []byte(existing), 0o600); err != nil {
		t.Fatalf("write existing mcp: %v", err)
	}

	plan, err := a.Plan(context.Background(), input)
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}
	_, err = a.Render(context.Background(), plan)
	if err != nil {
		t.Fatalf("render should replace existing isolated MCP config: %v", err)
	}
	got := readFile(t, mcpPath)
	if strings.Contains(got, "user-owned") {
		t.Fatalf("isolated mcp config kept old content:\n%s", got)
	}
	if !strings.Contains(got, `"github"`) || !strings.Contains(got, `"command": "npx"`) {
		t.Fatalf("isolated mcp config missing rendered server:\n%s", got)
	}
}

func TestRenderLinksActiveSkillDirectories(t *testing.T) {
	ctx := context.Background()
	project := t.TempDir()
	configDir := t.TempDir()
	activeDir := filepath.Join(t.TempDir(), "active")
	skillDir := filepath.Join(activeDir, "skills", "probe-skill")
	if err := os.MkdirAll(skillDir, 0o700); err != nil {
		t.Fatalf("create active skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("AVM_SKILL_PROBE_MARKER_20260426\n"), 0o600); err != nil {
		t.Fatalf("write active skill: %v", err)
	}

	a := claude.New(claude.WithConfigDir(configDir))
	input := richInput(project)
	input.ActiveDir = activeDir
	input.Capabilities.Skills = []adapter.CapabilityRef{
		{Name: "probe-skill", Path: filepath.ToSlash(filepath.Join(skillDir, "SKILL.md"))},
	}

	plan, err := a.Plan(ctx, input)
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}
	result, err := a.Render(ctx, plan)
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if !operationChanged(result, "claude-skill-probe-skill") {
		t.Fatalf("claude skill link should report changed")
	}
	runtimeSkillPath := filepath.Join(configDir, "skills", "probe-skill", "SKILL.md")
	assertPathContains(t, runtimeSkillPath, "avm_managed: true")
	assertPathContains(t, runtimeSkillPath, "AVM_SKILL_PROBE_MARKER_20260426")

	second, err := a.Render(ctx, plan)
	if err != nil {
		t.Fatalf("second render failed: %v", err)
	}
	if operationChanged(second, "claude-skill-probe-skill") {
		t.Fatalf("claude skill link should be unchanged on second render")
	}

	userSkillPath := filepath.Join(configDir, "skills", "user-skill", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(userSkillPath), 0o700); err != nil {
		t.Fatalf("create user skill dir: %v", err)
	}
	if err := os.WriteFile(userSkillPath, []byte("---\nname: user-skill\ndescription: user-owned\n---\n\nkeep\n"), 0o600); err != nil {
		t.Fatalf("write user skill: %v", err)
	}

	cleanupInput := input
	cleanupInput.Capabilities.Skills = nil
	cleanupPlan, err := a.Plan(ctx, cleanupInput)
	if err != nil {
		t.Fatalf("cleanup plan failed: %v", err)
	}
	cleanupResult, err := a.Render(ctx, cleanupPlan)
	if err != nil {
		t.Fatalf("cleanup render failed: %v", err)
	}
	if !operationChanged(cleanupResult, "claude-skill-remove-probe-skill") {
		t.Fatalf("claude stale skill removal should report changed")
	}
	assertPathMissing(t, runtimeSkillPath)
	assertPathContains(t, userSkillPath, "user-owned")
}

func TestManagedPathsReturnsCopy(t *testing.T) {
	a := claude.New()
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
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "adapter", "claude", "phase1_render_plan.json"))
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
	if fixture.Runtime != "claude-code" {
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
		Runtime: "claude-code",
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
				{Name: "postgres", Command: "postgres-mcp"},
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

func readFile(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func assertPathContains(t *testing.T, path, expected string) {
	t.Helper()
	content := readFile(t, path)
	if !strings.Contains(content, expected) {
		t.Fatalf("%s missing %q:\n%s", path, expected, content)
	}
}

func assertPathMissing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected %s to be missing, stat err: %v", path, err)
	}
}
