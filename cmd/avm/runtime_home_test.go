package main

import (
	"testing"

	"github.com/xz1220/agent-vm/internal/config"
)

func agentRuntimeHomeForTest(t *testing.T, agentName, runtime string) string {
	t.Helper()
	agent, err := config.ReadAgent(agentName, config.ScopeGlobal, "")
	if err != nil {
		t.Fatalf("read agent %s: %v", agentName, err)
	}
	return config.AgentRuntimeHomeDir(agent.ID, runtime)
}
