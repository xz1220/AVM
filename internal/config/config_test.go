package config

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestYAMLRoundTripAndList(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)

	var global GlobalConfig
	readFixture(t, "global.yaml", &global)
	global.ApplyDefaults()
	if err := WriteGlobalConfig(&global); err != nil {
		t.Fatalf("WriteGlobalConfig returned error: %v", err)
	}
	assertStableWrite(t, GlobalConfigPath(), func() error {
		got, err := ReadGlobalConfig()
		if err != nil {
			return err
		}
		return WriteGlobalConfig(got)
	})
	gotGlobal, err := ReadGlobalConfig()
	if err != nil {
		t.Fatalf("ReadGlobalConfig returned error: %v", err)
	}
	if !reflect.DeepEqual(&global, gotGlobal) {
		t.Fatalf("global round trip mismatch:\nwant %#v\ngot  %#v", global, *gotGlobal)
	}

	var agent AgentProfile
	readFixture(t, "agent.yaml", &agent)
	agent.ApplyDefaults("")
	if err := WriteAgent(&agent, ScopeGlobal, project); err != nil {
		t.Fatalf("WriteAgent returned error: %v", err)
	}
	assertStableWrite(t, AgentPath(agent.Name), func() error {
		got, err := ReadAgent(agent.Name, ScopeGlobal, project)
		if err != nil {
			return err
		}
		return WriteAgent(got, ScopeGlobal, project)
	})
	gotAgent, err := ReadAgent(agent.Name, ScopeGlobal, project)
	if err != nil {
		t.Fatalf("ReadAgent returned error: %v", err)
	}
	if !reflect.DeepEqual(&agent, gotAgent) {
		t.Fatalf("agent round trip mismatch:\nwant %#v\ngot  %#v", agent, *gotAgent)
	}
	agents, err := ListAgents(ScopeGlobal, project)
	if err != nil {
		t.Fatalf("ListAgents returned error: %v", err)
	}
	if len(agents) != 1 || agents[0].Name != agent.Name {
		t.Fatalf("unexpected agent summaries: %#v", agents)
	}

	var env Environment
	readFixture(t, "env.yaml", &env)
	env.ApplyDefaults()
	if err := WriteEnvironment(&env); err != nil {
		t.Fatalf("WriteEnvironment returned error: %v", err)
	}
	assertStableWrite(t, EnvPath(env.Name), func() error {
		got, err := ReadEnvironment(env.Name)
		if err != nil {
			return err
		}
		return WriteEnvironment(got)
	})
	gotEnv, err := ReadEnvironment(env.Name)
	if err != nil {
		t.Fatalf("ReadEnvironment returned error: %v", err)
	}
	if !reflect.DeepEqual(&env, gotEnv) {
		t.Fatalf("environment round trip mismatch:\nwant %#v\ngot  %#v", env, *gotEnv)
	}
	envs, err := ListEnvironments()
	if err != nil {
		t.Fatalf("ListEnvironments returned error: %v", err)
	}
	if len(envs) != 1 || envs[0].Name != env.Name {
		t.Fatalf("unexpected environment summaries: %#v", envs)
	}
}

func TestValidationRejectsInvalidValues(t *testing.T) {
	var agent AgentProfile
	readFixture(t, "agent.yaml", &agent)
	agent.Permissions.Sandbox = "full"
	if err := ValidateAgentProfile(&agent); err == nil || !strings.Contains(err.Error(), "permissions.sandbox") {
		t.Fatalf("expected permissions.sandbox validation error, got %v", err)
	}

	var env Environment
	readFixture(t, "env.yaml", &env)
	env.Targets = []string{"openclaw"}
	if err := ValidateEnvironment(&env); err == nil || !strings.Contains(err.Error(), "targets[0]") {
		t.Fatalf("expected target validation error, got %v", err)
	}
}

func TestReadRejectsUnknownYAMLFields(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("HOME", home)

	writeFixture(t, filepath.Join(AgentsDir(), "bad-agent.yaml"), "invalid-agent-workspace-isolation.yaml")
	if _, err := ReadAgent("bad-agent", ScopeGlobal, project); err == nil || !strings.Contains(err.Error(), "workspace_isolation") {
		t.Fatalf("expected workspace_isolation decode error, got %v", err)
	}

	writeFixture(t, filepath.Join(EnvsDir(), "bad-env.yaml"), "invalid-env-capabilities.yaml")
	if _, err := ReadEnvironment("bad-env"); err == nil || !strings.Contains(err.Error(), "capabilities") {
		t.Fatalf("expected capabilities decode error, got %v", err)
	}
}

func readFixture(t *testing.T, name string, out any) {
	t.Helper()
	if err := readYAML(filepath.Join("..", "..", "testdata", "config", name), out); err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
}

func writeFixture(t *testing.T, dest, name string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "config", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o700); err != nil {
		t.Fatalf("mkdir fixture dest: %v", err)
	}
	if err := os.WriteFile(dest, data, 0o600); err != nil {
		t.Fatalf("write fixture %s: %v", dest, err)
	}
}

func assertStableWrite(t *testing.T, path string, rewrite func() error) {
	t.Helper()
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s before rewrite: %v", path, err)
	}
	if bytes.Contains(before, []byte("[]")) || bytes.Contains(before, []byte("{}")) {
		t.Fatalf("encoded YAML contains empty collection in %s:\n%s", path, before)
	}
	if err := rewrite(); err != nil {
		t.Fatalf("rewrite %s: %v", path, err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s after rewrite: %v", path, err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("YAML rewrite was not stable for %s:\nbefore:\n%s\nafter:\n%s", path, before, after)
	}
}
