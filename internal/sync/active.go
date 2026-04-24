package sync

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/xz1220/agent-vm/internal/config"
)

type activeManifest struct {
	Active        config.ActiveRef  `yaml:"active"`
	GeneratedAt   time.Time         `yaml:"generated_at"`
	RuntimeAgents map[string]string `yaml:"runtime_agents,omitempty"`
	Profiles      []string          `yaml:"profiles,omitempty"`
	Targets       []string          `yaml:"targets,omitempty"`
}

func RebuildActive(resolved *config.ResolvedActivation, activeDir string) error {
	return rebuildActive(resolved, activeDir, time.Now().UTC())
}

func rebuildActive(resolved *config.ResolvedActivation, activeDir string, now time.Time) error {
	if resolved == nil {
		return fmt.Errorf("resolved activation is nil")
	}
	if activeDir == "" {
		activeDir = config.ActiveDir()
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}

	activeDir = filepath.Clean(activeDir)
	parent := filepath.Dir(activeDir)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return err
	}

	tmpDir, err := os.MkdirTemp(parent, filepath.Base(activeDir)+".tmp-*")
	if err != nil {
		return err
	}
	cleanupTmp := true
	defer func() {
		if cleanupTmp {
			_ = os.RemoveAll(tmpDir)
		}
	}()

	if err := buildActiveTree(tmpDir, resolved, now.UTC()); err != nil {
		return err
	}

	prevDir := activeDir + ".prev"
	if err := os.RemoveAll(prevDir); err != nil {
		return err
	}

	movedOld := false
	if _, err := os.Lstat(activeDir); err == nil {
		if err := os.Rename(activeDir, prevDir); err != nil {
			return err
		}
		movedOld = true
	} else if !os.IsNotExist(err) {
		return err
	}

	if err := os.Rename(tmpDir, activeDir); err != nil {
		if movedOld {
			_ = os.Rename(prevDir, activeDir)
		}
		return err
	}
	cleanupTmp = false

	if movedOld {
		if err := os.RemoveAll(prevDir); err != nil {
			return err
		}
	}
	return nil
}

func buildActiveTree(root string, resolved *config.ResolvedActivation, now time.Time) error {
	for _, dir := range []string{"agents", "skills", "mcps", "commands", "hooks", "memory", "render"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o700); err != nil {
			return err
		}
	}

	runtimeAgents := runtimeAgentNames(resolved)
	manifest := activeManifest{
		Active:        resolved.Active,
		GeneratedAt:   now.UTC(),
		RuntimeAgents: runtimeAgents,
		Profiles:      sortedUniqueValues(runtimeAgents),
		Targets:       append([]string(nil), resolved.Targets...),
	}
	if err := writeYAML(filepath.Join(root, "manifest.yaml"), manifest); err != nil {
		return err
	}

	for runtime := range runtimeAgents {
		if !safeActiveName(runtime) {
			return fmt.Errorf("runtime name %q cannot be used in active path", runtime)
		}
		if err := os.MkdirAll(filepath.Join(root, "render", runtime), 0o700); err != nil {
			return err
		}
	}

	writtenAgents := make(map[string]struct{})
	for runtime, agent := range resolved.RuntimeAgents {
		name := agent.Name
		if name == "" {
			name = runtimeAgents[runtime]
		}
		if name == "" {
			name = runtime
		}
		if !safeActiveName(name) {
			return fmt.Errorf("agent name %q cannot be used in active path", name)
		}
		if _, ok := writtenAgents[name]; ok {
			continue
		}
		if err := writeYAML(filepath.Join(root, "agents", name+".yaml"), agent); err != nil {
			return err
		}
		writtenAgents[name] = struct{}{}
	}

	return nil
}

func runtimeAgentNames(resolved *config.ResolvedActivation) map[string]string {
	if len(resolved.RuntimeAgents) == 0 {
		return nil
	}

	names := make(map[string]string, len(resolved.RuntimeAgents))
	for runtime, agent := range resolved.RuntimeAgents {
		name := agent.Name
		if name == "" && resolved.Env != nil {
			if runtimeAgent, ok := resolved.Env.RuntimeAgents[runtime]; ok {
				name = runtimeAgent.Primary
			}
		}
		if name == "" {
			name = runtime
		}
		names[runtime] = name
	}
	return names
}

func sortedUniqueValues(values map[string]string) []string {
	if len(values) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func writeYAML(path string, value any) error {
	raw, err := yaml.Marshal(value)
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o600)
}

func safeActiveName(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	return !strings.ContainsAny(name, `/\`)
}
