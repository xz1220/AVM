package sync

import (
	"github.com/xz1220/agent-vm/internal/adapter"
	"github.com/xz1220/agent-vm/internal/boundary"
	"github.com/xz1220/agent-vm/internal/config"
)

type Options struct {
	ProjectRoot  string
	ActiveDir    string
	RuntimeHomes map[string]string
	StatePath    string
	BackupDir    string
	UpdateActive bool
	DryRun       bool
	Targets      []string
}

type AdapterRegistry interface {
	Get(runtime string) (adapter.Adapter, bool)
}

type TargetStatus string

const (
	TargetStatusSynced  TargetStatus = "synced"
	TargetStatusSkipped TargetStatus = "skipped"
	TargetStatusFailed  TargetStatus = "failed"
	TargetStatusPartial TargetStatus = "partial"
)

type TargetResult struct {
	Runtime      string                   `json:"runtime"`
	Status       TargetStatus             `json:"status"`
	Active       config.ActiveRef         `json:"active"`
	AgentName    string                   `json:"agent_name,omitempty"`
	Boundary     boundary.RuntimeBoundary `json:"boundary,omitempty"`
	RuntimeHome  string                   `json:"runtime_home,omitempty"`
	Plan         *adapter.RenderPlan      `json:"plan,omitempty"`
	RenderResult *adapter.RenderResult    `json:"render_result,omitempty"`
	ManagedPaths []adapter.ManagedPath    `json:"managed_paths,omitempty"`
	Mappings     []adapter.FieldMapping   `json:"mappings,omitempty"`
	Warnings     []string                 `json:"warnings,omitempty"`
	Error        string                   `json:"error,omitempty"`
}

type Result struct {
	Active   config.ActiveRef `json:"active"`
	DryRun   bool             `json:"dry_run"`
	Targets  []TargetResult   `json:"targets,omitempty"`
	Warnings []string         `json:"warnings,omitempty"`
}
