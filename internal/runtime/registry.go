package runtime

import (
	"github.com/xz1220/agent-vm/internal/adapter"
	"github.com/xz1220/agent-vm/internal/adapter/codex"
)

type Registry struct {
	adapters map[string]adapter.Adapter
}

func NewRegistry() *Registry {
	return &Registry{
		adapters: map[string]adapter.Adapter{
			"codex": codex.New(),
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
