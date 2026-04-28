package runtime

import (
	"github.com/xz1220/agent-vm/internal/adapter"
	"github.com/xz1220/agent-vm/internal/adapter/claude"
	"github.com/xz1220/agent-vm/internal/adapter/cline"
	"github.com/xz1220/agent-vm/internal/adapter/codex"
	"github.com/xz1220/agent-vm/internal/adapter/cursor"
	"github.com/xz1220/agent-vm/internal/adapter/opencode"
)

type Registry struct {
	adapters map[string]adapter.Adapter
}

func NewRegistry() *Registry {
	return &Registry{
		adapters: map[string]adapter.Adapter{
			"claude-code": claude.New(),
			"cline":       cline.New(),
			"codex":       codex.New(),
			"cursor":      cursor.New(),
			"opencode":    opencode.New(),
		},
	}
}

func (r *Registry) Get(runtime string) (adapter.Adapter, bool) {
	if r == nil {
		return nil, false
	}
	adp, ok := r.adapters[runtime]
	return adp, ok && adp != nil
}
