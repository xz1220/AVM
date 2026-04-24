package cursor_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/xz1220/agent-vm/internal/adapter"
	"github.com/xz1220/agent-vm/internal/adapter/cursor"
)

func TestAdapterImplementsContract(t *testing.T) {
	var _ adapter.Adapter = (*cursor.Adapter)(nil)
}

func TestDetectUsesProjectCursorDir(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	cursorDir := filepath.Join(root, ".cursor")
	if err := os.MkdirAll(cursorDir, 0o700); err != nil {
		t.Fatalf("create cursor dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cursorDir, "mcp.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write mcp file: %v", err)
	}

	detection := cursor.New(cursor.WithProjectRoot(root)).Detect(ctx)

	if detection.Runtime != "cursor" {
		t.Fatalf("runtime = %q, want cursor", detection.Runtime)
	}
	if !detection.Found {
		t.Fatalf("expected configured Cursor project dir to be found")
	}
	if detection.ConfigDir != filepath.ToSlash(cursorDir) {
		t.Fatalf("config dir = %q, want %q", detection.ConfigDir, filepath.ToSlash(cursorDir))
	}
	if !containsSubstring(detection.Warnings, "partial") {
		t.Fatalf("detect warnings missing partial status: %#v", detection.Warnings)
	}
}

func TestImportReportsPartialPlaceholder(t *testing.T) {
	result, err := cursor.New().Import(context.Background())
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}
	if result.Runtime != "cursor" {
		t.Fatalf("runtime = %q, want cursor", result.Runtime)
	}
	if !containsSubstring(result.Warnings, "partial") {
		t.Fatalf("import warnings missing partial status: %#v", result.Warnings)
	}
}

func TestPlanIsDeterministicAndPreservesEnvReferences(t *testing.T) {
	ctx := context.Background()
	a := cursor.New(cursor.WithProjectRoot("/repo"))
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
	if !containsSubstring(first.Warnings, "partial") {
		t.Fatalf("plan warnings missing partial status: %#v", first.Warnings)
	}

	ruleContent := operationContent(t, first, "cursor-avm-rule")
	if strings.Index(ruleContent, "/active/memory/a.md") > strings.Index(ruleContent, "/active/memory/z.md") {
		t.Fatalf("instruction references were not sorted deterministically:\n%s", ruleContent)
	}
	for _, expected := range []string{
		"# AVM Agent: backend-coder",
		"Cursor support is partial in Phase 1",
		"You implement backend changes with tests.",
		"Prefer small, reviewable changes.",
	} {
		if !strings.Contains(ruleContent, expected) {
			t.Fatalf("rule content missing %q:\n%s", expected, ruleContent)
		}
	}

	mcpContent := operationContent(t, first, "cursor-avm-mcp")
	if !strings.Contains(mcpContent, `"GITHUB_TOKEN": "${GITHUB_TOKEN}"`) {
		t.Fatalf("mcp content expanded or omitted env reference:\n%s", mcpContent)
	}
	if strings.Index(mcpContent, `"github"`) > strings.Index(mcpContent, `"postgres"`) {
		t.Fatalf("mcp servers were not sorted deterministically:\n%s", mcpContent)
	}
}

func TestPlanMappingsCoverPartialUnsupportedIgnoredAndNativeFields(t *testing.T) {
	temperature := 0.2
	input := richInput("/repo")
	input.Agent.Model.Temperature = &temperature
	input.Agent.Permissions.AdditionalDirectories = []string{"/outside"}
	input.Capabilities.Commands = []adapter.CapabilityRef{{Name: "deploy"}}
	input.Capabilities.Hooks = []adapter.CapabilityRef{{Name: "preflight"}}
	input.Capabilities.MCPServers = append(input.Capabilities.MCPServers, adapter.MCPServer{Name: "missing"})

	plan, err := cursor.New(cursor.WithProjectRoot("/repo")).Plan(context.Background(), input)
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}

	for _, mapping := range plan.Mappings {
		if !mapping.Status.Valid() {
			t.Fatalf("mapping %s used invalid status %q", mapping.SourcePath, mapping.Status)
		}
	}

	assertMapping(t, plan, "active", adapter.MappingIgnored)
	assertMapping(t, plan, "agent.profile", adapter.MappingUnsupported)
	assertMapping(t, plan, "agent.instructions.system", adapter.MappingRenderedAsInstructions)
	assertMapping(t, plan, "agent.model.model", adapter.MappingUnsupported)
	assertMapping(t, plan, "agent.model.temperature", adapter.MappingUnsupported)
	assertMapping(t, plan, "agent.permissions.approval", adapter.MappingUnsupported)
	assertMapping(t, plan, "agent.permissions.additional_directories", adapter.MappingUnsupported)
	assertMapping(t, plan, "agent.memory_refs", adapter.MappingUnsupported)
	assertMapping(t, plan, "memory", adapter.MappingUnsupported)
	assertMapping(t, plan, "capabilities.skills", adapter.MappingUnsupported)
	assertMapping(t, plan, "capabilities.commands", adapter.MappingUnsupported)
	assertMapping(t, plan, "capabilities.hooks", adapter.MappingUnsupported)
	assertMapping(t, plan, "capabilities.toolsets", adapter.MappingUnsupported)
	assertMapping(t, plan, "capabilities.mcp_servers.github", adapter.MappingNative)
	assertMapping(t, plan, "capabilities.mcp_servers.missing", adapter.MappingUnsupported)
	assertMapping(t, plan, "project..cursorrules", adapter.MappingIgnored)

	for _, expected := range []string{"model", "permissions", "memory", "non-MCP capability"} {
		if !containsSubstring(plan.Warnings, expected) {
			t.Fatalf("warnings missing %q: %#v", expected, plan.Warnings)
		}
	}
}

func TestRenderWritesManagedPathsAndPreservesUnmanagedCursorFiles(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	t.Setenv("GITHUB_TOKEN", "expanded-token")
	t.Setenv("DO_NOT_EXPAND", "expanded-user-token")

	cursorDir := filepath.Join(root, ".cursor")
	if err := os.MkdirAll(cursorDir, 0o700); err != nil {
		t.Fatalf("create cursor dir: %v", err)
	}
	mcpPath := filepath.Join(cursorDir, "mcp.json")
	existingMCP := "{\n  \"mcpServers\": {\n    \"user\": {\n      \"command\": \"user-mcp\",\n      \"env\": {\n        \"TOKEN\": \"${DO_NOT_EXPAND}\"\n      }\n    }\n  },\n  \"workspace\": true\n}\n"
	if err := os.WriteFile(mcpPath, []byte(existingMCP), 0o600); err != nil {
		t.Fatalf("write existing mcp: %v", err)
	}
	userRulesPath := filepath.Join(root, ".cursorrules")
	if err := os.WriteFile(userRulesPath, []byte("user-owned\n"), 0o600); err != nil {
		t.Fatalf("write user rules: %v", err)
	}

	a := cursor.New(cursor.WithProjectRoot(root))
	plan, err := a.Plan(ctx, richInput(root))
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}

	result, err := a.Render(ctx, plan)
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if !operationChanged(result, "cursor-avm-mcp") {
		t.Fatalf("cursor-avm-mcp should report changed")
	}
	if !operationChanged(result, "cursor-avm-rule") {
		t.Fatalf("cursor-avm-rule should report changed")
	}

	mcpContent := readFile(t, mcpPath)
	for _, expected := range []string{
		`"workspace": true`,
		`"user"`,
		`"github"`,
		`"${GITHUB_TOKEN}"`,
		`"${DO_NOT_EXPAND}"`,
	} {
		if !strings.Contains(mcpContent, expected) {
			t.Fatalf("rendered mcp missing %q:\n%s", expected, mcpContent)
		}
	}
	if strings.Contains(mcpContent, "expanded-token") || strings.Contains(mcpContent, "expanded-user-token") {
		t.Fatalf("rendered mcp expanded environment variables:\n%s", mcpContent)
	}

	rulePath := filepath.Join(root, ".cursor", "rules", "avm-backend-coder.md")
	ruleContent := readFile(t, rulePath)
	if !strings.Contains(ruleContent, "You implement backend changes with tests.") {
		t.Fatalf("rule file missing system instructions:\n%s", ruleContent)
	}
	if got := readFile(t, userRulesPath); got != "user-owned\n" {
		t.Fatalf("unmanaged .cursorrules changed: %q", got)
	}

	second, err := a.Render(ctx, plan)
	if err != nil {
		t.Fatalf("second render failed: %v", err)
	}
	for _, id := range []string{"cursor-avm-mcp", "cursor-avm-rule"} {
		if operationChanged(second, id) {
			t.Fatalf("%s should be unchanged on second render", id)
		}
	}
}

func TestRenderRejectsOperationsOutsideManagedPathsBeforeWriting(t *testing.T) {
	root := t.TempDir()
	managedPath := filepath.Join(root, ".cursor", "rules", "avm-backend-coder.md")
	unmanagedPath := filepath.Join(root, ".cursorrules")
	if err := os.WriteFile(unmanagedPath, []byte("keep\n"), 0o600); err != nil {
		t.Fatalf("write unmanaged file: %v", err)
	}

	plan := &adapter.RenderPlan{
		Runtime:   "cursor",
		AgentName: "backend-coder",
		ManagedPaths: []adapter.ManagedPath{
			{Path: managedPath, Owner: "avm", Required: true, MergeMode: adapter.MergeModeWholeFile},
		},
		Operations: []adapter.RenderOperation{
			{ID: "managed", Action: adapter.OperationWriteFile, Path: managedPath, Content: []byte("changed\n"), Required: true},
			{ID: "rogue", Action: adapter.OperationWriteFile, Path: unmanagedPath, Content: []byte("changed\n"), Required: true},
		},
	}

	_, err := cursor.New(cursor.WithProjectRoot(root)).Render(context.Background(), plan)
	if err == nil {
		t.Fatalf("render unexpectedly accepted unmanaged operation")
	}
	if _, err := os.Stat(managedPath); !os.IsNotExist(err) {
		t.Fatalf("managed path was written despite preflight failure: %v", err)
	}
	if got := readFile(t, unmanagedPath); got != "keep\n" {
		t.Fatalf("unmanaged file changed despite render error: %q", got)
	}
}

func TestManagedPathsReturnsCopy(t *testing.T) {
	a := cursor.New(cursor.WithProjectRoot("/repo"))
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
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "testdata", "adapter", "cursor", "phase1_render_plan.json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	var fixture struct {
		Schema       string                 `json:"fixture_schema"`
		Runtime      string                 `json:"runtime"`
		Status       string                 `json:"status"`
		ManagedPaths []adapter.ManagedPath  `json:"managed_paths"`
		Mappings     []adapter.FieldMapping `json:"mappings"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	if fixture.Schema != "avm.phase1.adapter-render-plan.v1" {
		t.Fatalf("fixture schema = %q", fixture.Schema)
	}
	if fixture.Runtime != "cursor" {
		t.Fatalf("fixture runtime = %q", fixture.Runtime)
	}
	if fixture.Status != "partial" {
		t.Fatalf("fixture status = %q, want partial", fixture.Status)
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
		Runtime: "cursor",
		Agent: adapter.Agent{
			Name:        "backend-coder",
			Description: "Backend implementation agent",
			Instructions: adapter.Instructions{
				System:    "You implement backend changes with tests.",
				Developer: "Prefer small, reviewable changes.",
				References: []string{
					"/active/memory/z.md",
					"/active/memory/a.md",
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
			MemoryRefs: []adapter.MemoryRef{
				{ID: "z-memory", Scope: "project", Path: "/active/memory/z.md", Mode: "read"},
				{ID: "a-memory", Scope: "project", Path: "/active/memory/a.md", Mode: "read"},
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
		Memory: []adapter.PortableMemory{
			{ID: "z-memory", Scope: "project", Path: "/active/memory/z.md", Mode: "read"},
			{ID: "a-memory", Scope: "project", Path: "/active/memory/a.md", Mode: "read"},
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

func containsSubstring(values []string, needle string) bool {
	for _, value := range values {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}
