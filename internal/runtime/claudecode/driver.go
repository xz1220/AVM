// Package claudecode implements the Claude Code RuntimeDriver. It maps
// AVM Agent semantics into Claude Code's CLAUDE_CONFIG_DIR-managed
// settings, detects the `claude` binary, and computes the per-Agent
// boundary used by AVM run.
//
// References: docs/engineering/runtime-research/claude-code-runtime.md
package claudecode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/xz1220/agent-vm/internal/app/model"
	"github.com/xz1220/agent-vm/internal/runtime"
)

// Name is the canonical Registry key for this driver.
const Name = "claude-code"

// Env vars that point Claude Code at AVM-managed state. AVM injects all
// of them so settings, plugin cache, tmp and debug logs stay inside the
// per-Agent boundary.
const (
	EnvConfigDir   = "CLAUDE_CONFIG_DIR"
	EnvPluginCache = "CLAUDE_CODE_PLUGIN_CACHE_DIR"
	EnvTmp         = "CLAUDE_CODE_TMPDIR"
	EnvDebugDir    = "CLAUDE_CODE_DEBUG_LOGS_DIR"
)

// Driver is the Claude Code runtime adapter.
type Driver struct{}

// New returns a Claude Code driver.
func New() *Driver { return &Driver{} }

// Name reports the canonical registry key.
func (d *Driver) Name() string { return Name }

// Facts probes the claude binary and reports static capabilities/risks.
func (d *Driver) Facts(ctx context.Context) (runtime.Facts, error) {
	bin, err := exec.LookPath("claude")
	if err != nil {
		return runtime.Facts{Name: Name, Available: false}, nil
	}
	version := probeVersion(ctx, bin)
	return runtime.Facts{
		Name:       Name,
		Available:  true,
		BinaryPath: bin,
		Version:    version,
		Capabilities: []string{
			"instructions",
			"skills",
			"mcp",
			"plugins",
			"settings",
			"sub-agents",
		},
		Risks: []runtime.Risk{
			{Code: "claude.split-state", Message: "Claude Code state spans multiple env vars (CLAUDE_CONFIG_DIR, plugin cache, tmp, debug); AVM must set all of them to fully isolate."},
			{Code: "claude.project-trust", Message: "Project-level settings/MCP/headersHelper are gated behind a trust prompt on first use."},
			{Code: "claude.auto-memory", Message: "auto-memory writes <CLAUDE_CONFIG_DIR>/projects/<git-root>/memory/MEMORY.md by default; AVM does not manage memory contents."},
			{Code: "claude.plain-creds", Message: "Non-macOS platforms fall back to plaintext ~/.claude/.credentials.json for OAuth tokens."},
		},
	}, nil
}

// DiscoverGlobal scans Claude Code's user-level skill and MCP roots.
func (d *Driver) DiscoverGlobal(ctx context.Context) ([]model.GlobalCapability, error) {
	out := []model.GlobalCapability{}

	homes := userClaudeHomes()
	for _, root := range homes {
		out = append(out, scanSkillDir(filepath.Join(root, "skills"))...)
	}

	// Global MCP lives in ~/.claude.json (or $CLAUDE_CONFIG_DIR/.claude.json).
	for _, jsonPath := range globalConfigPaths() {
		out = append(out, scanMCPFromGlobalConfig(jsonPath)...)
	}
	return out, nil
}

// Plan renders the Agent into Claude Code managed files.
func (d *Driver) Plan(ctx context.Context, agent *model.Agent) (*runtime.Plan, error) {
	if agent == nil {
		return nil, errors.New("claude-code: nil agent")
	}
	if err := agent.Validate(); err != nil {
		return nil, fmt.Errorf("claude-code: %w", err)
	}
	bnd, err := d.Boundary(ctx, agent)
	if err != nil {
		return nil, err
	}

	plan := &runtime.Plan{}

	// CLAUDE.md is the native instructions slot.
	claudeMD := filepath.Join(bnd.StateDir, "CLAUDE.md")
	plan.Files = append(plan.Files, runtime.ManagedFile{
		Path:     claudeMD,
		Mode:     0o600,
		Contents: []byte(renderInstructions(agent)),
	})

	// settings.json is the native runtime config slot.
	settingsPath := filepath.Join(bnd.StateDir, "settings.json")
	settingsBytes, err := renderSettings(agent)
	if err != nil {
		return nil, fmt.Errorf("claude-code: render settings: %w", err)
	}
	plan.Files = append(plan.Files, runtime.ManagedFile{
		Path:     settingsPath,
		Mode:     0o600,
		Contents: settingsBytes,
	})

	plan.Mappings = append(plan.Mappings,
		runtime.FieldMapping{
			Field: "identity.name", Status: model.MappingNative,
			Note: "rendered into CLAUDE.md heading and CLAUDE_CONFIG_DIR isolation key",
		},
		runtime.FieldMapping{
			Field: "identity.description", Status: model.MappingNative,
			Note: "rendered into CLAUDE.md preface",
		},
	)
	if agent.Identity.Role != "" {
		plan.Mappings = append(plan.Mappings, runtime.FieldMapping{
			Field: "identity.role", Status: model.MappingRenderedAsInstructions,
			Note: "Claude Code has no role slot; role concatenated into CLAUDE.md.",
		})
	}
	plan.Mappings = append(plan.Mappings, runtime.FieldMapping{
		Field: "instructions", Status: model.MappingNative,
		Note: "written to CLAUDE.md (loaded as user/project instructions).",
	})
	if len(agent.Skills) > 0 {
		plan.Mappings = append(plan.Mappings, runtime.FieldMapping{
			Field: "skills", Status: model.MappingNative,
			Note: "AVM materializes skills under <CLAUDE_CONFIG_DIR>/skills/<id>/SKILL.md.",
		})
	}
	if len(agent.MCP) > 0 {
		plan.Mappings = append(plan.Mappings, runtime.FieldMapping{
			Field: "mcp", Status: model.MappingNative,
			Note: "rendered into settings.json mcpServers; infra wires command/env per capability.",
		})
	}
	if len(agent.Runtimes) > 0 {
		plan.Mappings = append(plan.Mappings, runtime.FieldMapping{
			Field: "runtimes", Status: model.MappingIgnored,
			Note: "AVM-side preference; never written into Claude Code config.",
		})
	}

	plan.Warnings = append(plan.Warnings, model.Warning{
		Code:    "claude.split-state",
		Message: "Claude Code state spans multiple env vars; AVM sets CLAUDE_CONFIG_DIR/plugin-cache/tmp/debug to keep state isolated.",
	})
	plan.Warnings = append(plan.Warnings, model.Warning{
		Code:    "claude.auto-memory",
		Message: "auto-memory will write under " + filepath.Join(bnd.StateDir, "projects") + " — AVM isolates but does not manage memory contents.",
	})

	return plan, nil
}

// Boundary returns the per-Agent state dir and the env vars Claude Code
// honors to relocate its state. PRD §3.4 requires per-(Agent, runtime)
// isolation; Claude Code splits state across multiple env vars so we
// point them all at the same boundary directory.
func (d *Driver) Boundary(ctx context.Context, agent *model.Agent) (runtime.Boundary, error) {
	if agent == nil {
		return runtime.Boundary{}, errors.New("claude-code: nil agent")
	}
	if agent.Identity.Name == "" {
		return runtime.Boundary{}, errors.New("claude-code: agent identity.name required")
	}
	root, err := boundaryStateDir(agent.Identity.Name)
	if err != nil {
		return runtime.Boundary{}, err
	}
	return runtime.Boundary{
		StateDir: root,
		Env: map[string]string{
			EnvConfigDir:   root,
			EnvPluginCache: filepath.Join(root, "plugins"),
			EnvTmp:         filepath.Join(root, "tmp"),
			EnvDebugDir:    filepath.Join(root, "debug"),
		},
	}, nil
}

// LaunchSpec describes how to spawn `claude`.
func (d *Driver) LaunchSpec(ctx context.Context, agent *model.Agent, plan *runtime.Plan) (runtime.LaunchSpec, error) {
	facts, err := d.Facts(ctx)
	if err != nil {
		return runtime.LaunchSpec{}, err
	}
	if !facts.Available {
		return runtime.LaunchSpec{}, errors.New("claude-code: binary not available")
	}
	bnd, err := d.Boundary(ctx, agent)
	if err != nil {
		return runtime.LaunchSpec{}, err
	}
	env := map[string]string{}
	for k, v := range bnd.Env {
		env[k] = v
	}
	return runtime.LaunchSpec{
		Bin:   facts.BinaryPath,
		Args:  []string{}, // bare `claude` enters interactive mode
		Env:   env,
		Stdin: true,
	}, nil
}

// boundaryStateDir mirrors the layout in internal/infra/home so the
// driver can compute it without importing infra.
func boundaryStateDir(agentName string) (string, error) {
	root := os.Getenv("AVM_HOME")
	if root == "" {
		hd, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if hd == "" {
			return "", errors.New("claude-code: empty user home dir")
		}
		root = filepath.Join(hd, ".avm")
	}
	return filepath.Join(root, "boundaries", Name, agentName), nil
}

// userClaudeHomes returns user-level Claude Code config home candidates
// for global discovery: explicit CLAUDE_CONFIG_DIR plus ~/.claude.
func userClaudeHomes() []string {
	seen := map[string]struct{}{}
	out := []string{}
	add := func(p string) {
		if p == "" {
			return
		}
		if _, ok := seen[p]; ok {
			return
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	if v := os.Getenv(EnvConfigDir); v != "" {
		add(v)
	}
	if hd, err := os.UserHomeDir(); err == nil && hd != "" {
		add(filepath.Join(hd, ".claude"))
	}
	return out
}

// globalConfigPaths returns the candidate `.claude.json` files (the
// global config that holds user-scope MCP servers).
func globalConfigPaths() []string {
	out := []string{}
	if v := os.Getenv(EnvConfigDir); v != "" {
		out = append(out, filepath.Join(v, ".claude.json"))
	}
	if hd, err := os.UserHomeDir(); err == nil && hd != "" {
		out = append(out, filepath.Join(hd, ".claude.json"))
	}
	return out
}

func scanSkillDir(root string) []model.GlobalCapability {
	out := []model.GlobalCapability{}
	entries, err := os.ReadDir(root)
	if err != nil {
		return out
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		skillDir := filepath.Join(root, name)
		manifest := filepath.Join(skillDir, "SKILL.md")
		if _, err := os.Stat(manifest); err != nil {
			continue
		}
		out = append(out, model.GlobalCapability{
			Runtime: Name,
			Kind:    model.CapabilityKindSkill,
			Name:    name,
			Path:    skillDir,
			Version: readSkillVersion(manifest),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// readSkillVersion looks for `version:` inside the YAML frontmatter.
func readSkillVersion(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	if len(b) < 4 || string(b[:3]) != "---" {
		return ""
	}
	rest := string(b[3:])
	end := strings.Index(rest, "\n---")
	if end < 0 {
		end = len(rest)
	}
	for _, line := range strings.Split(rest[:end], "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "version:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "version:"))
		}
	}
	return ""
}

// scanMCPFromGlobalConfig pulls the names of mcpServers from a Claude
// Code global config JSON file. We don't decode the whole settings
// surface — we only need the keys for discovery display.
func scanMCPFromGlobalConfig(path string) []model.GlobalCapability {
	out := []model.GlobalCapability{}
	b, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return out
	}
	servers, ok := raw["mcpServers"]
	if !ok {
		return out
	}
	var byName map[string]json.RawMessage
	if err := json.Unmarshal(servers, &byName); err != nil {
		return out
	}
	for name := range byName {
		out = append(out, model.GlobalCapability{
			Runtime: Name,
			Kind:    model.CapabilityKindMCP,
			Name:    name,
			Path:    path,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func renderInstructions(a *model.Agent) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", a.Identity.Name)
	if a.Identity.Description != "" {
		fmt.Fprintf(&b, "%s\n\n", a.Identity.Description)
	}
	if a.Identity.Role != "" {
		fmt.Fprintf(&b, "Role: %s\n\n", a.Identity.Role)
	}
	if a.Instructions.System != "" {
		b.WriteString(a.Instructions.System)
		b.WriteString("\n\n")
	}
	if a.Instructions.Inline != "" {
		b.WriteString(a.Instructions.Inline)
		b.WriteString("\n\n")
	}
	for _, f := range a.Instructions.Files {
		fmt.Fprintf(&b, "<!-- include: %s -->\n", f)
	}
	return b.String()
}

// renderSettings emits a minimal Claude Code settings.json. Per PRD §6
// AVM never writes user-owned keys it doesn't understand; we declare
// the keys we own and leave the rest to Claude Code defaults.
func renderSettings(a *model.Agent) ([]byte, error) {
	type mcpEntry struct {
		// Empty body — actual command/args/env wiring is a job for the
		// infra layer once it materializes the capability. This driver
		// owns the layout, not the per-server transport details.
	}
	type settings struct {
		MCPServers map[string]mcpEntry `json:"mcpServers,omitempty"`
	}
	s := settings{MCPServers: map[string]mcpEntry{}}
	for _, m := range a.MCP {
		s.MCPServers[string(m.ID)] = mcpEntry{}
	}
	if len(s.MCPServers) == 0 {
		s.MCPServers = nil
	}
	return json.MarshalIndent(s, "", "  ")
}

func probeVersion(ctx context.Context, bin string) string {
	if ctx == nil {
		ctx = context.Background()
	}
	subCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(subCtx, bin, "--version")
	cmd.Stdin = nil
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
