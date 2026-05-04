package codex_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/xz1220/agent-vm/internal/adapter"
	"github.com/xz1220/agent-vm/internal/adapter/codex"
)

func TestAdapterImplementsContract(t *testing.T) {
	var _ adapter.Adapter = (*codex.Adapter)(nil)
}

func TestDetectUsesConfiguredCodexHome(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte("profile = \"user\"\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	detection := codex.New(codex.WithConfigDir(dir)).Detect(ctx)

	if detection.Runtime != "codex" {
		t.Fatalf("runtime = %q, want codex", detection.Runtime)
	}
	if !detection.Found {
		t.Fatalf("expected configured codex home to be found")
	}
	if detection.ConfigDir != filepath.ToSlash(dir) {
		t.Fatalf("config dir = %q, want %q", detection.ConfigDir, filepath.ToSlash(dir))
	}
}

func TestDetectEdgeCases(t *testing.T) {
	ctx := context.Background()
	t.Setenv("PATH", t.TempDir())

	t.Run("configured directory without config", func(t *testing.T) {
		dir := t.TempDir()

		detection := codex.New(codex.WithConfigDir(dir)).Detect(ctx)

		if !detection.Found {
			t.Fatalf("expected configured codex home directory to be found")
		}
		if detection.Version != "" {
			t.Fatalf("version = %q, want empty when codex binary is absent", detection.Version)
		}
	})

	t.Run("missing config and binary", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "missing")

		detection := codex.New(codex.WithConfigDir(dir)).Detect(ctx)

		if detection.Found {
			t.Fatalf("unexpectedly found codex for missing config dir and isolated PATH: %#v", detection)
		}
		if detection.ConfigDir != filepath.ToSlash(dir) {
			t.Fatalf("config dir = %q, want %q", detection.ConfigDir, filepath.ToSlash(dir))
		}
	})
}

func TestPlanIsDeterministic(t *testing.T) {
	ctx := context.Background()
	a := codex.New(codex.WithConfigDir("/tmp/codex-home"))
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

	roleContent := operationContent(t, first, "codex-agent-role")
	for _, expected := range []string{
		"Active AVM skills:\\n- git (/active/skills/git/SKILL.md)\\n- test (/active/skills/test/SKILL.md)",
		"Denied command guidance:\\n- Bash(rm -rf *)",
	} {
		if !strings.Contains(roleContent, expected) {
			t.Fatalf("role content missing deterministic block %q:\n%s", expected, roleContent)
		}
	}

	configContent := operationContent(t, first, "codex-config")
	if !strings.Contains(configContent, "env = { GITHUB_TOKEN = \"${GITHUB_TOKEN}\" }") {
		t.Fatalf("config content expanded or omitted env reference:\n%s", configContent)
	}
	if strings.Index(configContent, "[mcp_servers.github]") > strings.Index(configContent, "[mcp_servers.postgres]") {
		t.Fatalf("mcp servers were not sorted deterministically:\n%s", configContent)
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

	plan, err := codex.New(codex.WithConfigDir("/tmp/codex-home")).Plan(context.Background(), input)
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}

	for _, mapping := range plan.Mappings {
		if !mapping.Status.Valid() {
			t.Fatalf("mapping %s used invalid status %q", mapping.SourcePath, mapping.Status)
		}
	}

	assertMapping(t, plan, "agent.model.model", adapter.MappingNative)
	assertMapping(t, plan, "agent.name", adapter.MappingNative)
	assertMapping(t, plan, "agent.description", adapter.MappingNative)
	assertMapping(t, plan, "agent.instructions.system", adapter.MappingRenderedAsInstructions)
	assertMapping(t, plan, "agent.instructions.developer", adapter.MappingNative)
	assertMapping(t, plan, "agent.instructions.references", adapter.MappingRenderedAsInstructions)
	assertMapping(t, plan, "capabilities.skills", adapter.MappingRenderedAsInstructions)
	assertMapping(t, plan, "agent.model.verbosity", adapter.MappingRenderedAsInstructions)
	assertMapping(t, plan, "agent.permissions.allow", adapter.MappingRenderedAsInstructions)
	assertMapping(t, plan, "agent.permissions.deny", adapter.MappingRenderedAsInstructions)
	assertMapping(t, plan, "capabilities.commands", adapter.MappingRenderedAsInstructions)
	assertMapping(t, plan, "capabilities.hooks", adapter.MappingRenderedAsInstructions)
	assertMapping(t, plan, "capabilities.toolsets", adapter.MappingRenderedAsInstructions)
	assertMapping(t, plan, "project.AGENTS.md", adapter.MappingIgnored)
	assertMapping(t, plan, "capabilities.mcp_servers.github", adapter.MappingNative)
	assertMapping(t, plan, "capabilities.mcp_servers.postgres", adapter.MappingNative)
	assertMapping(t, plan, "agent.model.temperature", adapter.MappingUnsupported)
	assertMapping(t, plan, "agent.permissions.additional_directories", adapter.MappingUnsupported)
	assertMapping(t, plan, "capabilities.mcp_servers.missing", adapter.MappingUnsupported)

	wantUnsupported := []string{
		"agent.model.temperature",
		"agent.permissions.additional_directories",
		"capabilities.mcp_servers.missing",
	}
	if got := mappingSourcesWithStatus(plan, adapter.MappingUnsupported); !reflect.DeepEqual(got, wantUnsupported) {
		t.Fatalf("unsupported mappings = %v, want %v", got, wantUnsupported)
	}
}

func TestRenderWritesManagedPathsAsIsolatedConfig(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	a := codex.New(codex.WithConfigDir(dir))
	input := richInput("/repo")

	configPath := filepath.Join(dir, "config.toml")
	unmanagedPath := filepath.Join(dir, "AGENTS.md")
	existingConfig := "profile = \"user\"\n[user]\nsecret = \"${DO_NOT_EXPAND}\"\n# >>> avm:codex:codex-config\nold = true\n# <<< avm:codex:codex-config\n"
	if err := os.WriteFile(configPath, []byte(existingConfig), 0o600); err != nil {
		t.Fatalf("write existing config: %v", err)
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
	if !operationChanged(result, "codex-config") {
		t.Fatalf("codex-config should report changed")
	}
	if !operationChanged(result, "codex-agent-role") {
		t.Fatalf("codex-agent-role should report changed")
	}

	configContent := readFile(t, configPath)
	for _, expected := range []string{
		"profile = \"avm-coding\"",
		"[profiles.avm-coding]",
		"env = { GITHUB_TOKEN = \"${GITHUB_TOKEN}\" }",
	} {
		if !strings.Contains(configContent, expected) {
			t.Fatalf("rendered config missing %q:\n%s", expected, configContent)
		}
	}
	for _, old := range []string{"profile = \"user\"", "secret = \"${DO_NOT_EXPAND}\"", "# >>> avm:codex:codex-config", "old = true"} {
		if strings.Contains(configContent, old) {
			t.Fatalf("isolated config kept old content %q:\n%s", old, configContent)
		}
	}

	rolePath := filepath.Join(dir, "agents", "backend-coder.toml")
	roleContent := readFile(t, rolePath)
	if !strings.Contains(roleContent, "developer_instructions = ") {
		t.Fatalf("role file missing developer instructions:\n%s", roleContent)
	}
	if got := readFile(t, unmanagedPath); got != "user-owned\n" {
		t.Fatalf("unmanaged file changed: %q", got)
	}

	second, err := a.Render(ctx, plan)
	if err != nil {
		t.Fatalf("second render failed: %v", err)
	}
	for _, id := range []string{"codex-config", "codex-agent-role"} {
		if operationChanged(second, id) {
			t.Fatalf("%s should be unchanged on second render", id)
		}
	}
}

func TestRenderWholeFileConfigReplacesExistingContent(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name     string
		existing string
	}{
		{
			name:     "append",
			existing: "profile = \"user\"\n[user]\nsecret = \"${DO_NOT_EXPAND}\"\n",
		},
		{
			name:     "replace",
			existing: "profile = \"user\"\n# >>> avm:codex:codex-config\nold = true\n# <<< avm:codex:codex-config\n[user]\nsecret = \"${DO_NOT_EXPAND}\"\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			a := codex.New(codex.WithConfigDir(dir))
			configPath := filepath.Join(dir, "config.toml")
			if err := os.WriteFile(configPath, []byte(tc.existing), 0o600); err != nil {
				t.Fatalf("write existing config: %v", err)
			}
			plan, err := a.Plan(ctx, richInput("/repo"))
			if err != nil {
				t.Fatalf("plan failed: %v", err)
			}

			result, err := a.Render(ctx, plan)
			if err != nil {
				t.Fatalf("render failed: %v", err)
			}
			if !operationChanged(result, "codex-config") {
				t.Fatalf("codex-config should report changed")
			}

			content := readFile(t, configPath)
			for _, expected := range []string{
				"profile = \"avm-coding\"",
				"[profiles.avm-coding]",
				"env = { GITHUB_TOKEN = \"${GITHUB_TOKEN}\" }",
			} {
				if !strings.Contains(content, expected) {
					t.Fatalf("rendered config missing %q:\n%s", expected, content)
				}
			}
			for _, old := range []string{"profile = \"user\"", "secret = \"${DO_NOT_EXPAND}\"", "# >>> avm:codex:codex-config", "# <<< avm:codex:codex-config", "old = true"} {
				if strings.Contains(content, old) {
					t.Fatalf("isolated config kept old content %q:\n%s", old, content)
				}
			}
		})
	}
}

func TestRenderWholeFileConfigIgnoresMalformedLegacyBlocks(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name     string
		existing string
	}{
		{
			name:     "missing end marker",
			existing: "profile = \"user\"\n# >>> avm:codex:codex-config\nold = true\n",
		},
		{
			name:     "missing begin marker",
			existing: "profile = \"user\"\n# <<< avm:codex:codex-config\n",
		},
		{
			name:     "duplicate block",
			existing: "# >>> avm:codex:codex-config\nold = true\n# <<< avm:codex:codex-config\n# >>> avm:codex:codex-config\nold = true\n# <<< avm:codex:codex-config\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			a := codex.New(codex.WithConfigDir(dir))
			configPath := filepath.Join(dir, "config.toml")
			if err := os.WriteFile(configPath, []byte(tc.existing), 0o600); err != nil {
				t.Fatalf("write existing config: %v", err)
			}
			plan, err := a.Plan(ctx, richInput("/repo"))
			if err != nil {
				t.Fatalf("plan failed: %v", err)
			}

			_, err = a.Render(ctx, plan)
			if err != nil {
				t.Fatalf("render should replace malformed legacy blocks in isolated config: %v", err)
			}
			if got := readFile(t, configPath); strings.Contains(got, "old = true") || strings.Contains(got, "profile = \"user\"") {
				t.Fatalf("isolated config kept malformed legacy content:\n%s", got)
			}
			rolePath := filepath.Join(dir, "agents", "backend-coder.toml")
			if _, err := os.Stat(rolePath); err != nil {
				t.Fatalf("role file should be written for isolated config render, stat err: %v", err)
			}
		})
	}
}

func TestRenderRejectsOperationsOutsideManagedPaths(t *testing.T) {
	dir := t.TempDir()
	a := codex.New(codex.WithConfigDir(dir))
	managedPath := filepath.Join(dir, "agents", "backend-coder.toml")
	unmanagedPath := filepath.Join(dir, "unmanaged.toml")
	if err := os.WriteFile(unmanagedPath, []byte("keep\n"), 0o600); err != nil {
		t.Fatalf("write unmanaged file: %v", err)
	}

	plan := &adapter.RenderPlan{
		Runtime:   "codex",
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

func TestRenderRejectsManagedAGENTSMD(t *testing.T) {
	dir := t.TempDir()
	a := codex.New(codex.WithConfigDir(dir))
	agentsPath := filepath.Join(dir, "AGENTS.md")
	if err := os.WriteFile(agentsPath, []byte("user-owned\n"), 0o600); err != nil {
		t.Fatalf("write AGENTS.md: %v", err)
	}

	plan := &adapter.RenderPlan{
		Runtime:   "codex",
		AgentName: "backend-coder",
		ManagedPaths: []adapter.ManagedPath{
			{Path: agentsPath, Owner: "avm", Required: true, MergeMode: adapter.MergeModeWholeFile},
		},
		Operations: []adapter.RenderOperation{
			{ID: "agents-md", Action: adapter.OperationWriteFile, Path: agentsPath, Content: []byte("changed\n"), Required: true},
		},
	}

	_, err := a.Render(context.Background(), plan)
	if err == nil {
		t.Fatalf("render unexpectedly accepted AGENTS.md as a Codex managed path")
	}
	if got := readFile(t, agentsPath); got != "user-owned\n" {
		t.Fatalf("AGENTS.md changed despite render error: %q", got)
	}
}

func TestRenderLinksActiveSkillDirectories(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	activeDir := filepath.Join(t.TempDir(), "active")
	skillDir := filepath.Join(activeDir, "skills", "probe-skill")
	if err := os.MkdirAll(skillDir, 0o700); err != nil {
		t.Fatalf("create active skill dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("AVM_SKILL_PROBE_MARKER_20260426\n"), 0o600); err != nil {
		t.Fatalf("write active skill: %v", err)
	}

	a := codex.New(codex.WithConfigDir(dir))
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
	if !operationChanged(result, "codex-skill-probe-skill") {
		t.Fatalf("codex skill link should report changed")
	}
	runtimeSkillPath := filepath.Join(dir, "skills", "probe-skill", "SKILL.md")
	assertPathContains(t, runtimeSkillPath, "avm_managed: true")
	assertPathContains(t, runtimeSkillPath, "AVM_SKILL_PROBE_MARKER_20260426")

	second, err := a.Render(ctx, plan)
	if err != nil {
		t.Fatalf("second render failed: %v", err)
	}
	if operationChanged(second, "codex-skill-probe-skill") {
		t.Fatalf("codex skill link should be unchanged on second render")
	}

	userSkillPath := filepath.Join(dir, "skills", "user-skill", "SKILL.md")
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
	if !operationChanged(cleanupResult, "codex-skill-remove-probe-skill") {
		t.Fatalf("codex stale skill removal should report changed")
	}
	assertPathMissing(t, runtimeSkillPath)
	assertPathContains(t, userSkillPath, "user-owned")
}

func TestManagedPathsReturnsCopy(t *testing.T) {
	a := codex.New(codex.WithConfigDir("/tmp/codex-home"))
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

func TestFixturePlansMatchActualPlan(t *testing.T) {
	expected := expectedFixturePlan(t)

	for _, path := range []string{
		filepath.Join("..", "..", "..", "testdata", "adapter", "codex", "phase1_render_plan.json"),
		filepath.Join("..", "..", "..", "fixtures", "phase1", "minimal", "adapter-render-plan", "codex.plan.json"),
	} {
		t.Run(path, func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}

			var fixture planFixture
			if err := json.Unmarshal(data, &fixture); err != nil {
				t.Fatalf("unmarshal fixture: %v", err)
			}
			for _, mapping := range fixture.Mappings {
				if !mapping.Status.Valid() {
					t.Fatalf("fixture mapping %s has invalid status %q", mapping.SourcePath, mapping.Status)
				}
			}
			if !reflect.DeepEqual(fixture, expected) {
				actualJSON, _ := json.MarshalIndent(expected, "", "  ")
				fixtureJSON, _ := json.MarshalIndent(fixture, "", "  ")
				t.Fatalf("fixture does not match actual plan\nfixture:\n%s\nactual:\n%s", fixtureJSON, actualJSON)
			}
		})
	}
}

type planFixture struct {
	Schema       string                    `json:"fixture_schema"`
	Runtime      string                    `json:"runtime"`
	Active       adapter.ActiveRef         `json:"active"`
	AgentName    string                    `json:"agent_name"`
	ManagedPaths []adapter.ManagedPath     `json:"managed_paths,omitempty"`
	Operations   []adapter.RenderOperation `json:"operations,omitempty"`
	Mappings     []adapter.FieldMapping    `json:"mappings,omitempty"`
	Warnings     []string                  `json:"warnings,omitempty"`
}

func expectedFixturePlan(t *testing.T) planFixture {
	t.Helper()

	plan, err := codex.New(codex.WithConfigDir("<CODEX_HOME>")).Plan(context.Background(), fixtureInput())
	if err != nil {
		t.Fatalf("plan fixture input: %v", err)
	}

	operations := append([]adapter.RenderOperation(nil), plan.Operations...)
	for i := range operations {
		operations[i].Content = nil
	}

	return planFixture{
		Schema:       "avm.phase1.adapter-render-plan.v1",
		Runtime:      plan.Runtime,
		Active:       plan.Active,
		AgentName:    plan.AgentName,
		ManagedPaths: append([]adapter.ManagedPath(nil), plan.ManagedPaths...),
		Operations:   operations,
		Mappings:     append([]adapter.FieldMapping(nil), plan.Mappings...),
		Warnings:     append([]string(nil), plan.Warnings...),
	}
}

func fixtureInput() adapter.RenderInput {
	temperature := 0.0
	return adapter.RenderInput{
		Active:  adapter.ActiveRef{Kind: "env", Name: "coding"},
		Runtime: "codex",
		Agent: adapter.Agent{
			Name:        "backend-coder",
			Description: "Backend implementation agent",
			Instructions: adapter.Instructions{
				System:     "You implement backend changes with tests.",
				Developer:  "Prefer small, reviewable changes.",
				References: []string{"docs/backend-standards.md"},
			},
			Model: adapter.ModelConfig{
				Model:           "gpt-5.4",
				ReasoningEffort: "medium",
				Verbosity:       "normal",
				Temperature:     &temperature,
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
			},
			MCPServers: []adapter.MCPServer{
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
		ProjectRoot: "<PROJECT_ROOT>",
	}
}

func richInput(projectRoot string) adapter.RenderInput {
	return adapter.RenderInput{
		Active:  adapter.ActiveRef{Kind: "env", Name: "coding"},
		Runtime: "codex",
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

func mappingSourcesWithStatus(plan *adapter.RenderPlan, status adapter.MappingStatus) []string {
	var sources []string
	for _, mapping := range plan.Mappings {
		if mapping.Status == status {
			sources = append(sources, mapping.SourcePath)
		}
	}
	return sources
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
