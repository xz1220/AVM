package adapter

import (
	"reflect"
	"testing"

	"github.com/xz1220/agent-vm/internal/config"
)

func TestRenderInputFromResolved(t *testing.T) {
	temperature := 0.2
	resolved := &config.ResolvedActivation{
		Active: config.ActiveRef{Kind: config.ActiveKindProfile, Name: "backend"},
		RuntimeAgents: map[string]config.AgentProfile{
			"codex": {
				Name:        "backend",
				Description: "Backend agent",
				SourceScope: "global",
				Instructions: config.Instructions{
					System:     "system",
					Developer:  "developer",
					References: []string{"ref"},
				},
				ModelRun: config.ModelRun{
					Model:           "gpt-5.4",
					ReasoningEffort: "high",
					Verbosity:       "normal",
					Temperature:     &temperature,
				},
				Permissions: config.Permissions{
					Approval:              "on-request",
					Sandbox:               "workspace-write",
					Allow:                 []string{"Read"},
					Deny:                  []string{"Bash(rm *)"},
					AdditionalDirectories: []string{"../shared"},
				},
			},
		},
		Capabilities: map[string]config.ResolvedCapabilities{
			"codex": {
				Skills: []string{"test"},
				SkillRefs: []config.ResolvedSkill{
					{Name: "test", SourceDir: "/registry/skills/test", SourcePath: "/registry/skills/test/SKILL.md"},
				},
				MCPs: []string{"github"},
				MCPServers: []config.ResolvedMCPServer{
					{
						Name:    "github",
						Command: "printf",
						Args:    []string{"avm-test-mcp"},
						Env: map[string]string{
							"GITHUB_TOKEN": "${GITHUB_TOKEN}",
							"Z_TOKEN":      "${Z_TOKEN}",
						},
					},
				},
				Commands: []string{"fmt"},
				Hooks:    []string{"pre"},
				Toolsets: map[string]string{"shell": "limited"},
			},
		},
	}

	input, err := RenderInputFromResolved(resolved, "codex", RenderInputOptions{ProjectRoot: "/repo", ActiveDir: "/active"})
	if err != nil {
		t.Fatalf("RenderInputFromResolved returned error: %v", err)
	}

	if input.Active.Name != "backend" || input.Runtime != "codex" || input.ProjectRoot != "/repo" || input.ActiveDir != "/active" {
		t.Fatalf("unexpected input identity: %#v", input)
	}
	if input.Agent.Name != "backend" || input.Agent.Instructions.Developer != "developer" {
		t.Fatalf("agent projection not populated: %#v", input.Agent)
	}
	if input.Agent.Model.Temperature == nil || *input.Agent.Model.Temperature != temperature {
		t.Fatalf("model projection not populated: %#v", input.Agent.Model)
	}
	if got := input.Capabilities.Skills[0]; got != (CapabilityRef{Name: "test", Path: "/active/skills/test/SKILL.md"}) {
		t.Fatalf("skill projection not populated: %#v", got)
	}
	if got := input.Capabilities.MCPServers[0]; !reflect.DeepEqual(got, MCPServer{
		Name:    "github",
		Command: "printf",
		Args:    []string{"avm-test-mcp"},
		Env: []EnvVar{
			{Name: "GITHUB_TOKEN", Value: "${GITHUB_TOKEN}"},
			{Name: "Z_TOKEN", Value: "${Z_TOKEN}"},
		},
	}) {
		t.Fatalf("mcp projection not populated: %#v", got)
	}
	if got := input.Capabilities.Toolsets[0]; got != (Toolset{Name: "shell", Mode: "limited"}) {
		t.Fatalf("toolset projection not populated: %#v", got)
	}
}

func TestRenderInputsFromResolvedOrdersTargetsAndSkipsMissingAgents(t *testing.T) {
	resolved := &config.ResolvedActivation{
		Active:  config.ActiveRef{Kind: config.ActiveKindEnv, Name: "backend-dev"},
		Targets: []string{"claude-code", "cursor", "codex"},
		RuntimeAgents: map[string]config.AgentProfile{
			"codex":       {Name: "coder"},
			"claude-code": {Name: "reviewer"},
			"cline":       {Name: "assistant"},
		},
	}

	inputs, err := RenderInputsFromResolved(resolved, RenderInputOptions{})
	if err != nil {
		t.Fatalf("RenderInputsFromResolved returned error: %v", err)
	}

	var runtimes []string
	for _, input := range inputs {
		runtimes = append(runtimes, input.Runtime)
	}
	want := []string{"claude-code", "codex", "cline"}
	if !reflect.DeepEqual(runtimes, want) {
		t.Fatalf("unexpected runtime order: got %v want %v", runtimes, want)
	}
}
