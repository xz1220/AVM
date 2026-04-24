// Package renderplan contains helpers for stable adapter render plans.
package renderplan

import (
	"sort"

	"github.com/xz1220/agent-vm/internal/adapter"
)

// Normalize returns a copy of plan with deterministic slice ordering. It does
// not mutate the input plan.
func Normalize(plan *adapter.RenderPlan) *adapter.RenderPlan {
	if plan == nil {
		return nil
	}

	normalized := &adapter.RenderPlan{
		Runtime:      plan.Runtime,
		Active:       plan.Active,
		AgentName:    plan.AgentName,
		ManagedPaths: cloneManagedPaths(plan.ManagedPaths),
		Operations:   cloneOperations(plan.Operations),
		Mappings:     cloneMappings(plan.Mappings),
		Warnings:     append([]string(nil), plan.Warnings...),
	}

	sort.SliceStable(normalized.ManagedPaths, func(i, j int) bool {
		left := normalized.ManagedPaths[i]
		right := normalized.ManagedPaths[j]
		if left.Path != right.Path {
			return left.Path < right.Path
		}
		if left.Owner != right.Owner {
			return left.Owner < right.Owner
		}
		return left.MergeMode < right.MergeMode
	})

	sort.SliceStable(normalized.Operations, func(i, j int) bool {
		left := normalized.Operations[i]
		right := normalized.Operations[j]
		if left.Path != right.Path {
			return left.Path < right.Path
		}
		if left.Action != right.Action {
			return left.Action < right.Action
		}
		return left.ID < right.ID
	})

	sort.SliceStable(normalized.Mappings, func(i, j int) bool {
		left := normalized.Mappings[i]
		right := normalized.Mappings[j]
		if left.SourcePath != right.SourcePath {
			return left.SourcePath < right.SourcePath
		}
		if left.TargetPath != right.TargetPath {
			return left.TargetPath < right.TargetPath
		}
		return left.Status < right.Status
	})

	sort.Strings(normalized.Warnings)

	return normalized
}

func cloneManagedPaths(paths []adapter.ManagedPath) []adapter.ManagedPath {
	return append([]adapter.ManagedPath(nil), paths...)
}

func cloneMappings(mappings []adapter.FieldMapping) []adapter.FieldMapping {
	return append([]adapter.FieldMapping(nil), mappings...)
}

func cloneOperations(operations []adapter.RenderOperation) []adapter.RenderOperation {
	cloned := make([]adapter.RenderOperation, len(operations))
	for i, operation := range operations {
		cloned[i] = operation
		cloned[i].Content = append([]byte(nil), operation.Content...)
	}
	return cloned
}
