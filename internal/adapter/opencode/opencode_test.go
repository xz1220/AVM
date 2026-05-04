package opencode_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/xz1220/agent-vm/internal/adapter"
	"github.com/xz1220/agent-vm/internal/adapter/opencode"
)

func TestAdapterImplementsContract(t *testing.T) {
	var _ adapter.Adapter = (*opencode.Adapter)(nil)
}

func TestDetectUsesConfiguredOpenCodeHome(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "opencode.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	detection := opencode.New(opencode.WithConfigDir(dir)).Detect(ctx)

	if detection.Runtime != "opencode" {
		t.Fatalf("runtime = %q, want opencode", detection.Runtime)
	}
	if !detection.Found {
		t.Fatalf("expected configured OpenCode home to be found")
	}
	if detection.ConfigDir != filepath.ToSlash(dir) {
		t.Fatalf("config dir = %q, want %q", detection.ConfigDir, filepath.ToSlash(dir))
	}
}

func TestPlanIsDeterministic(t *testing.T) {
	ctx := context.Background()
	a := opencode.New(opencode.WithConfigDir("/tmp/opencode-home"))
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

	configContent := operationContent(t, first, "opencode-config")
	var cfg map[string]any
	if err := json.Unmarshal([]byte(configContent), &cfg); err != nil {
		t.Fatalf("config is not json: %v\n%s", err, configContent)
	}
	if cfg["default_agent"] != "backend-coder" {
		t.Fatalf("default_agent = %#v, want backend-coder", cfg["default_agent"])
	}
	if !strings.Contains(configContent, `"GITHUB_TOKEN": "${GITHUB_TOKEN}"`) {
		t.Fatalf("config content expanded or omitted env reference:\n%s", configContent)
	}
	if strings.Index(configContent, `"github"`) > strings.Index(configContent, `"postgres"`) {
		t.Fatalf("mcp servers were not sorted deterministically:\n%s", configContent)
	}

	agentContent := operationContent(t, first, "opencode-agent")
	for _, expected := range []string{
		`description: "Backend implementation agent"`,
		`mode: "primary"`,
		`"go test ./...": allow`,
		`"rm -rf *": deny`,
		"Active AVM skills:\n- git\n- test",
		"Reasoning effort:\nmedium",
	} {
		if !strings.Contains(agentContent, expected) {
			t.Fatalf("agent content missing deterministic block %q:\n%s", expected, agentContent)
		}
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

	plan, err := opencode.New(opencode.WithConfigDir("/tmp/opencode-home")).Plan(context.Background(), input)
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}

	for _, mapping := range plan.Mappings {
		if !mapping.Status.Valid() {
			t.Fatalf("mapping %s used invalid status %q", mapping.SourcePath, mapping.Status)
		}
	}

	assertMapping(t, plan, "agent.model.model", adapter.MappingNative)
	assertMapping(t, plan, "agent.model.temperature", adapter.MappingNative)
	assertMapping(t, plan, "agent.model.reasoning_effort", adapter.MappingRenderedAsInstructions)
	assertMapping(t, plan, "agent.permissions.additional_directories", adapter.MappingNative)
	assertMapping(t, plan, "capabilities.skills", adapter.MappingRenderedAsInstructions)
	assertMapping(t, plan, "capabilities.commands", adapter.MappingRenderedAsInstructions)
	assertMapping(t, plan, "capabilities.hooks", adapter.MappingRenderedAsInstructions)
	assertMapping(t, plan, "project.AGENTS.md", adapter.MappingIgnored)
	assertMapping(t, plan, "capabilities.mcp_servers.github", adapter.MappingNative)
	assertMapping(t, plan, "capabilities.mcp_servers.missing", adapter.MappingUnsupported)
}

func TestRenderWritesIsolatedOpenCodeConfigAndSkills(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	activeDir := filepath.Join(t.TempDir(), "active")
	skillDir := filepath.Join(activeDir, "skills", "probe-skill")
	if err := os.MkdirAll(skillDir, 0o700); err != nil {
		t.Fatalf("create active skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("OpenCode probe skill.\n"), 0o600); err != nil {
		t.Fatalf("write active skill: %v", err)
	}

	configPath := filepath.Join(dir, "opencode.json")
	if err := os.WriteFile(configPath, []byte(`{"default_agent":"user","secret":"${DO_NOT_EXPAND}"}`+"\n"), 0o600); err != nil {
		t.Fatalf("write existing config: %v", err)
	}
	unmanagedPath := filepath.Join(dir, "AGENTS.md")
	if err := os.WriteFile(unmanagedPath, []byte("user-owned\n"), 0o600); err != nil {
		t.Fatalf("write unmanaged file: %v", err)
	}

	a := opencode.New(opencode.WithConfigDir(dir))
	input := richInput("/repo")
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
	for _, id := range []string{"opencode-config", "opencode-agent", "opencode-skill-probe-skill"} {
		if !operationChanged(result, id) {
			t.Fatalf("%s should report changed", id)
		}
	}

	configContent := readFile(t, configPath)
	for _, expected := range []string{
		`"default_agent": "backend-coder"`,
		`"type": "local"`,
		`"environment": {`,
	} {
		if !strings.Contains(configContent, expected) {
			t.Fatalf("rendered config missing %q:\n%s", expected, configContent)
		}
	}
	for _, old := range []string{`"default_agent":"user"`, "DO_NOT_EXPAND"} {
		if strings.Contains(configContent, old) {
			t.Fatalf("isolated config kept old content %q:\n%s", old, configContent)
		}
	}

	agentPath := filepath.Join(dir, "agents", "backend-coder.md")
	agentContent := readFile(t, agentPath)
	if !strings.Contains(agentContent, "Developer instructions:\nPrefer small, reviewable changes.") {
		t.Fatalf("agent file missing developer instructions:\n%s", agentContent)
	}
	runtimeSkillPath := filepath.Join(dir, "skills", "probe-skill", "SKILL.md")
	assertPathContains(t, runtimeSkillPath, "avm_managed: true")
	assertPathContains(t, runtimeSkillPath, "OpenCode probe skill.")
	if got := readFile(t, unmanagedPath); got != "user-owned\n" {
		t.Fatalf("unmanaged file changed: %q", got)
	}

	second, err := a.Render(ctx, plan)
	if err != nil {
		t.Fatalf("second render failed: %v", err)
	}
	for _, id := range []string{"opencode-config", "opencode-agent", "opencode-skill-probe-skill"} {
		if operationChanged(second, id) {
			t.Fatalf("%s should be unchanged on second render", id)
		}
	}
}

func TestRenderRejectsOperationsOutsideManagedPaths(t *testing.T) {
	dir := t.TempDir()
	a := opencode.New(opencode.WithConfigDir(dir))
	managedPath := filepath.Join(dir, "opencode.json")
	unmanagedPath := filepath.Join(dir, "unmanaged.json")
	if err := os.WriteFile(unmanagedPath, []byte("keep\n"), 0o600); err != nil {
		t.Fatalf("write unmanaged file: %v", err)
	}

	plan := &adapter.RenderPlan{
		Runtime:   "opencode",
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

func richInput(projectRoot string) adapter.RenderInput {
	return adapter.RenderInput{
		Active:  adapter.ActiveRef{Kind: "env", Name: "coding"},
		Runtime: "opencode",
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
				Model:           "anthropic/claude-sonnet-4-5",
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
				{Name: "test"},
				{Name: "git"},
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
