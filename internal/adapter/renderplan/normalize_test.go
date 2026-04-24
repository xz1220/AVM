package renderplan_test

import (
	"reflect"
	"testing"

	"github.com/xz1220/agent-vm/internal/adapter"
	"github.com/xz1220/agent-vm/internal/adapter/renderplan"
)

func TestNormalizeOrdersPlanDeterministically(t *testing.T) {
	plan := &adapter.RenderPlan{
		Runtime:   "fake",
		AgentName: "backend",
		ManagedPaths: []adapter.ManagedPath{
			{Path: "/z", Owner: "avm", MergeMode: adapter.MergeModeWholeFile},
			{Path: "/a", Owner: "shared-section", MergeMode: adapter.MergeModeMarkedBlock},
			{Path: "/a", Owner: "avm", MergeMode: adapter.MergeModeWholeFile},
		},
		Operations: []adapter.RenderOperation{
			{ID: "write-z", Action: adapter.OperationWriteFile, Path: "/z", Content: []byte("z")},
			{ID: "ensure-a", Action: adapter.OperationEnsureDir, Path: "/a"},
			{ID: "write-a", Action: adapter.OperationWriteFile, Path: "/a", Content: []byte("a")},
		},
		Mappings: []adapter.FieldMapping{
			{SourcePath: "instructions.developer", TargetPath: "/z", Status: adapter.MappingRenderedAsInstructions},
			{SourcePath: "model.model", TargetPath: "/a", Status: adapter.MappingNative},
			{SourcePath: "capabilities.hooks", TargetPath: "", Status: adapter.MappingIgnored},
		},
		Warnings: []string{"z", "a"},
	}

	normalized := renderplan.Normalize(plan)

	managedPaths := []string{
		normalized.ManagedPaths[0].Path + ":" + normalized.ManagedPaths[0].Owner,
		normalized.ManagedPaths[1].Path + ":" + normalized.ManagedPaths[1].Owner,
		normalized.ManagedPaths[2].Path + ":" + normalized.ManagedPaths[2].Owner,
	}
	expectedManagedPaths := []string{"/a:avm", "/a:shared-section", "/z:avm"}
	if !reflect.DeepEqual(managedPaths, expectedManagedPaths) {
		t.Fatalf("managed paths were not normalized: got %v want %v", managedPaths, expectedManagedPaths)
	}

	operationIDs := []string{
		normalized.Operations[0].ID,
		normalized.Operations[1].ID,
		normalized.Operations[2].ID,
	}
	expectedOperationIDs := []string{"ensure-a", "write-a", "write-z"}
	if !reflect.DeepEqual(operationIDs, expectedOperationIDs) {
		t.Fatalf("operations were not normalized: got %v want %v", operationIDs, expectedOperationIDs)
	}

	mappingSources := []string{
		normalized.Mappings[0].SourcePath,
		normalized.Mappings[1].SourcePath,
		normalized.Mappings[2].SourcePath,
	}
	expectedMappingSources := []string{"capabilities.hooks", "instructions.developer", "model.model"}
	if !reflect.DeepEqual(mappingSources, expectedMappingSources) {
		t.Fatalf("mappings were not normalized: got %v want %v", mappingSources, expectedMappingSources)
	}

	if !reflect.DeepEqual(normalized.Warnings, []string{"a", "z"}) {
		t.Fatalf("warnings were not normalized: got %v", normalized.Warnings)
	}
}

func TestNormalizeDoesNotMutateInput(t *testing.T) {
	plan := &adapter.RenderPlan{
		Operations: []adapter.RenderOperation{
			{ID: "b", Action: adapter.OperationWriteFile, Path: "/b", Content: []byte("b")},
			{ID: "a", Action: adapter.OperationWriteFile, Path: "/a", Content: []byte("a")},
		},
		Warnings: []string{"b", "a"},
	}

	normalized := renderplan.Normalize(plan)
	normalized.Operations[0].Content[0] = 'x'

	if plan.Operations[0].ID != "b" {
		t.Fatalf("input plan operations were mutated: got first operation %q", plan.Operations[0].ID)
	}
	if string(plan.Operations[1].Content) != "a" {
		t.Fatalf("input plan operation content was mutated: got %q", string(plan.Operations[1].Content))
	}
	if !reflect.DeepEqual(plan.Warnings, []string{"b", "a"}) {
		t.Fatalf("input plan warnings were mutated: got %v", plan.Warnings)
	}
}
