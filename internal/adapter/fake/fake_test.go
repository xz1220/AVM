package fake_test

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/xz1220/agent-vm/internal/adapter"
	"github.com/xz1220/agent-vm/internal/adapter/fake"
)

func TestAdapterImplementsContracts(t *testing.T) {
	var _ adapter.Adapter = (*fake.Adapter)(nil)
	var _ adapter.MemoryImportCapable = (*fake.Adapter)(nil)
}

func TestPlanIsDeterministic(t *testing.T) {
	ctx := context.Background()
	fakeAdapter := fake.New(fake.WithName("codex"))
	input := adapter.RenderInput{
		Active: adapter.ActiveRef{Kind: "profile", Name: "backend"},
		Agent: adapter.Agent{
			Name:        "backend",
			Description: "Backend implementation agent",
			Instructions: adapter.Instructions{
				System:    "System text",
				Developer: "Developer text",
			},
			Model: adapter.ModelConfig{
				Model:           "gpt-5.4",
				ReasoningEffort: "medium",
			},
			Permissions: adapter.PermissionConfig{
				Approval: "on-request",
				Sandbox:  "workspace-write",
			},
		},
		Capabilities: adapter.CapabilitySet{
			Skills: []adapter.CapabilityRef{
				{Name: "test"},
				{Name: "git"},
			},
			MCPServers: []adapter.MCPServer{
				{Name: "postgres"},
				{Name: "github"},
			},
		},
		Memory: []adapter.PortableMemory{
			{ID: "z-memory"},
			{ID: "a-memory"},
		},
		ProjectRoot: "/repo",
	}

	first, err := fakeAdapter.Plan(ctx, input)
	if err != nil {
		t.Fatalf("first plan failed: %v", err)
	}
	second, err := fakeAdapter.Plan(ctx, input)
	if err != nil {
		t.Fatalf("second plan failed: %v", err)
	}

	if !reflect.DeepEqual(first, second) {
		t.Fatalf("fake plans are not deterministic:\nfirst: %#v\nsecond:%#v", first, second)
	}

	content := string(first.Operations[0].Content)
	for _, expected := range []string{
		"skill: git\nskill: test",
		"mcp: github\nmcp: postgres",
		"memory: a-memory\nmemory: z-memory",
	} {
		if !strings.Contains(content, expected) {
			t.Fatalf("rendered content missing deterministic block %q:\n%s", expected, content)
		}
	}
}

func TestPlanUsesOnlyAllowedMappingStatuses(t *testing.T) {
	plan, err := fake.New().Plan(context.Background(), adapter.RenderInput{
		Active: adapter.ActiveRef{Kind: "profile", Name: "backend"},
		Agent:  adapter.Agent{Name: "backend"},
	})
	if err != nil {
		t.Fatalf("plan failed: %v", err)
	}

	for _, mapping := range plan.Mappings {
		if !mapping.Status.Valid() {
			t.Fatalf("mapping %s used invalid status %q", mapping.SourcePath, mapping.Status)
		}
	}
}

func TestRenderCapturesNormalizedPlan(t *testing.T) {
	ctx := context.Background()
	fakeAdapter := fake.New()
	plan := &adapter.RenderPlan{
		Runtime:   "fake",
		AgentName: "backend",
		ManagedPaths: []adapter.ManagedPath{
			{Path: "/z", Owner: "avm", MergeMode: adapter.MergeModeWholeFile},
			{Path: "/a", Owner: "avm", MergeMode: adapter.MergeModeWholeFile},
		},
		Operations: []adapter.RenderOperation{
			{ID: "z", Action: adapter.OperationWriteFile, Path: "/z"},
			{ID: "a", Action: adapter.OperationWriteFile, Path: "/a"},
		},
	}

	result, err := fakeAdapter.Render(ctx, plan)
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if got := result.Operations[0].OperationID; got != "a" {
		t.Fatalf("render result was not normalized: first operation %q", got)
	}

	rendered := fakeAdapter.RenderedPlans()
	if len(rendered) != 1 {
		t.Fatalf("expected one rendered plan, got %d", len(rendered))
	}
	if got := rendered[0].ManagedPaths[0].Path; got != "/a" {
		t.Fatalf("stored rendered plan was not normalized: first path %q", got)
	}
}

func TestImportMemoryDefaultUsesOptions(t *testing.T) {
	plan, err := fake.New().ImportMemory(context.Background(), adapter.MemoryImportOptions{
		Runtime: "codex",
		Source:  "native",
		DryRun:  true,
	})
	if err != nil {
		t.Fatalf("import memory failed: %v", err)
	}

	if plan.Runtime != "codex" || plan.Source != "native" {
		t.Fatalf("unexpected memory import plan: %#v", plan)
	}
}
