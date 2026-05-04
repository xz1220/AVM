package fake_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/xz1220/agent-vm/internal/adapter"
	"github.com/xz1220/agent-vm/internal/adapter/fake"
)

func TestAdapterImplementsContracts(t *testing.T) {
	var _ adapter.Adapter = (*fake.Adapter)(nil)
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
	dir := t.TempDir()
	zPath := filepath.Join(dir, "z")
	aPath := filepath.Join(dir, "a")
	plan := &adapter.RenderPlan{
		Runtime:   "fake",
		AgentName: "backend",
		ManagedPaths: []adapter.ManagedPath{
			{Path: zPath, Owner: "avm", MergeMode: adapter.MergeModeWholeFile},
			{Path: aPath, Owner: "avm", MergeMode: adapter.MergeModeWholeFile},
		},
		Operations: []adapter.RenderOperation{
			{ID: "z", Action: adapter.OperationWriteFile, Path: zPath, Content: []byte("z")},
			{ID: "a", Action: adapter.OperationWriteFile, Path: aPath, Content: []byte("a")},
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
	if got := rendered[0].ManagedPaths[0].Path; got != aPath {
		t.Fatalf("stored rendered plan was not normalized: first path %q", got)
	}
}

func TestRenderAppliesManagedPathOperations(t *testing.T) {
	ctx := context.Background()
	fakeAdapter := fake.New()
	dir := t.TempDir()
	nestedDir := filepath.Join(dir, "nested")
	targetPath := filepath.Join(nestedDir, "profile.txt")
	removePath := filepath.Join(dir, "stale.txt")
	if err := os.WriteFile(removePath, []byte("stale"), 0o600); err != nil {
		t.Fatalf("write stale file: %v", err)
	}

	plan := &adapter.RenderPlan{
		Runtime:   "fake",
		AgentName: "backend",
		Operations: []adapter.RenderOperation{
			{ID: "ensure-dir", Action: adapter.OperationEnsureDir, Path: nestedDir},
			{ID: "write", Action: adapter.OperationWriteFile, Path: targetPath, Content: []byte("hello")},
			{ID: "remove", Action: adapter.OperationRemoveFile, Path: removePath},
		},
	}

	first, err := fakeAdapter.Render(ctx, plan)
	if err != nil {
		t.Fatalf("first render failed: %v", err)
	}
	if got := operationChanged(first, "ensure-dir"); !got {
		t.Fatalf("ensure-dir should report changed on first render")
	}
	if got := operationChanged(first, "write"); !got {
		t.Fatalf("write should report changed on first render")
	}
	if got := operationChanged(first, "remove"); !got {
		t.Fatalf("remove should report changed on first render")
	}
	assertFileContent(t, targetPath, "hello")
	if _, err := os.Stat(removePath); !os.IsNotExist(err) {
		t.Fatalf("expected stale file to be removed, stat err: %v", err)
	}

	second, err := fakeAdapter.Render(ctx, plan)
	if err != nil {
		t.Fatalf("second render failed: %v", err)
	}
	for _, id := range []string{"ensure-dir", "write", "remove"} {
		if got := operationChanged(second, id); got {
			t.Fatalf("%s should not report changed on second render", id)
		}
	}
}

func operationChanged(result *adapter.RenderResult, operationID string) bool {
	for _, operation := range result.Operations {
		if operation.OperationID == operationID {
			return operation.Changed
		}
	}
	return false
}

func assertFileContent(t *testing.T, path, expected string) {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read rendered file: %v", err)
	}
	if string(content) != expected {
		t.Fatalf("unexpected file content %q, want %q", content, expected)
	}
}
