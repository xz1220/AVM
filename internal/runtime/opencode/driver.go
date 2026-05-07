// Package opencode implements the OpenCode (a.k.a. OpenClaw) RuntimeDriver.
// It maps AVM Agent semantics into OpenClaw's per-Agent state directory,
// detects the `openclaw`/`opencode` binary, and produces a launch spec.
//
// References: docs/engineering/runtime-research/openclaw-runtime.md
package opencode

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
const Name = "opencode"

// Env vars that point OpenClaw at AVM-managed state.
const (
	EnvStateDir   = "OPENCLAW_STATE_DIR"
	EnvConfigPath = "OPENCLAW_CONFIG_PATH"
	EnvAgentDir   = "OPENCLAW_AGENT_DIR"
)

// candidateBinaries are the binary names we look up. The OpenClaw
// research doc names `openclaw` as the primary CLI; `opencode` is kept
// as a forward-compat alias in case the upstream rename ever happens.
var candidateBinaries = []string{"openclaw", "opencode"}

// Driver is the OpenCode runtime adapter.
type Driver struct{}

// New returns an OpenCode driver.
func New() *Driver { return &Driver{} }

// Name reports the canonical registry key.
func (d *Driver) Name() string { return Name }

// Facts probes the binary and reports static capabilities/risks.
func (d *Driver) Facts(ctx context.Context) (runtime.Facts, error) {
	bin := lookupBinary()
	if bin == "" {
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
			"sandbox",
			"approval",
			"workspace",
		},
		Risks: []runtime.Risk{
			{Code: "opencode.unsafe-defaults", Message: "OpenClaw defaults: sandbox=off, workspaceOnly=false, approval=off; AVM must override or warn."},
			{Code: "opencode.host-exec", Message: "Skills, plugins, and stdio MCP can spawn host commands; AVM should treat install actions as high risk."},
			{Code: "opencode.process-memory", Message: "Memory plugin state is process-global; do not reuse one OpenClaw process across AVM Agents."},
			{Code: "opencode.gateway-vs-local", Message: "Default execution path is Gateway RPC with local fallback; AVM should pin a single mode (currently --local)."},
		},
	}, nil
}

// DiscoverGlobal scans OpenClaw's user-level skill and MCP locations.
func (d *Driver) DiscoverGlobal(ctx context.Context) ([]model.GlobalCapability, error) {
	out := []model.GlobalCapability{}

	// Personal skills root (~/.agents/skills) is scanned by OpenClaw.
	if hd, err := os.UserHomeDir(); err == nil && hd != "" {
		out = append(out, scanSkillDir(filepath.Join(hd, ".agents", "skills"))...)
	}

	// User-level OpenClaw state dir contributes both workspace skills and
	// configured MCP servers.
	for _, root := range userOpenclawHomes() {
		out = append(out, scanSkillDir(filepath.Join(root, "workspace", "skills"))...)
		out = append(out, scanMCPFromConfig(filepath.Join(root, "openclaw.json"))...)
	}
	return out, nil
}

// Plan renders the Agent into OpenClaw managed config.
func (d *Driver) Plan(ctx context.Context, agent *model.Agent) (*runtime.Plan, error) {
	if agent == nil {
		return nil, errors.New("opencode: nil agent")
	}
	if err := agent.Validate(); err != nil {
		return nil, fmt.Errorf("opencode: %w", err)
	}
	bnd, err := d.Boundary(ctx, agent)
	if err != nil {
		return nil, err
	}

	plan := &runtime.Plan{}

	// AGENTS.md is the closest native instructions slot OpenClaw honors
	// (it is loaded as system/developer instructions when present).
	agentsMD := filepath.Join(bnd.StateDir, "agent", "AGENTS.md")
	plan.Files = append(plan.Files, runtime.ManagedFile{
		Path:     agentsMD,
		Mode:     0o600,
		Contents: []byte(renderInstructions(agent)),
	})

	// openclaw.json is the runtime config file.
	configPath := filepath.Join(bnd.StateDir, "openclaw.json")
	configBytes, err := renderConfig(agent)
	if err != nil {
		return nil, fmt.Errorf("opencode: render config: %w", err)
	}
	plan.Files = append(plan.Files, runtime.ManagedFile{
		Path:     configPath,
		Mode:     0o600,
		Contents: configBytes,
	})

	plan.Mappings = append(plan.Mappings,
		runtime.FieldMapping{
			Field: "identity.name", Status: model.MappingNative,
			Note: "stable agentId for OpenClaw session/state addressing",
		},
		runtime.FieldMapping{
			Field: "identity.description", Status: model.MappingNative,
			Note: "rendered into AGENTS.md preface",
		},
	)
	if agent.Identity.Role != "" {
		plan.Mappings = append(plan.Mappings, runtime.FieldMapping{
			Field: "identity.role", Status: model.MappingRenderedAsInstructions,
			Note: "OpenClaw has no role slot; role concatenated into AGENTS.md.",
		})
	}
	plan.Mappings = append(plan.Mappings, runtime.FieldMapping{
		Field: "instructions", Status: model.MappingNative,
		Note: "written to <stateDir>/agent/AGENTS.md and loaded as developer instructions.",
	})
	if len(agent.Skills) > 0 {
		plan.Mappings = append(plan.Mappings, runtime.FieldMapping{
			Field: "skills", Status: model.MappingNative,
			Note: "AVM materializes skills under workspace/skills/<id>/SKILL.md.",
		})
	}
	if len(agent.MCP) > 0 {
		plan.Mappings = append(plan.Mappings, runtime.FieldMapping{
			Field: "mcp", Status: model.MappingNative,
			Note: "rendered into openclaw.json mcp.servers; infra wires transport details.",
		})
	}
	if len(agent.Runtimes) > 0 {
		plan.Mappings = append(plan.Mappings, runtime.FieldMapping{
			Field: "runtimes", Status: model.MappingIgnored,
			Note: "AVM-side preference; never written into OpenClaw config.",
		})
	}

	plan.Warnings = append(plan.Warnings, model.Warning{
		Code:    "opencode.unsafe-defaults",
		Message: "OpenClaw ships unsafe defaults (sandbox off, workspaceOnly false, approval off); AVM-managed config opts in to safer settings.",
	})
	plan.Warnings = append(plan.Warnings, model.Warning{
		Code:    "opencode.host-exec",
		Message: "Skills, plugins, and stdio MCP can spawn host commands; install actions should be treated as high risk.",
	})

	return plan, nil
}

// Boundary returns the per-Agent OPENCLAW_STATE_DIR + companion env vars.
func (d *Driver) Boundary(ctx context.Context, agent *model.Agent) (runtime.Boundary, error) {
	if agent == nil {
		return runtime.Boundary{}, errors.New("opencode: nil agent")
	}
	if agent.Identity.Name == "" {
		return runtime.Boundary{}, errors.New("opencode: agent identity.name required")
	}
	root, err := boundaryStateDir(agent.Identity.Name)
	if err != nil {
		return runtime.Boundary{}, err
	}
	return runtime.Boundary{
		StateDir: root,
		Env: map[string]string{
			EnvStateDir:   root,
			EnvConfigPath: filepath.Join(root, "openclaw.json"),
			EnvAgentDir:   filepath.Join(root, "agent"),
		},
	}, nil
}

// LaunchSpec describes how to spawn openclaw/opencode.
func (d *Driver) LaunchSpec(ctx context.Context, agent *model.Agent, plan *runtime.Plan) (runtime.LaunchSpec, error) {
	facts, err := d.Facts(ctx)
	if err != nil {
		return runtime.LaunchSpec{}, err
	}
	if !facts.Available {
		return runtime.LaunchSpec{}, errors.New("opencode: binary not available")
	}
	bnd, err := d.Boundary(ctx, agent)
	if err != nil {
		return runtime.LaunchSpec{}, err
	}
	env := map[string]string{}
	for k, v := range bnd.Env {
		env[k] = v
	}
	// Per research §3.1 / §3.3: pin --local to keep session/auth on a
	// single path and avoid Gateway-vs-local divergence; agent is the
	// canonical interactive subcommand.
	return runtime.LaunchSpec{
		Bin:   facts.BinaryPath,
		Args:  []string{"agent", "--local"},
		Env:   env,
		Stdin: true,
	}, nil
}

func lookupBinary() string {
	for _, name := range candidateBinaries {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return ""
}

// boundaryStateDir mirrors internal/infra/home semantics.
func boundaryStateDir(agentName string) (string, error) {
	root := os.Getenv("AVM_HOME")
	if root == "" {
		hd, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if hd == "" {
			return "", errors.New("opencode: empty user home dir")
		}
		root = filepath.Join(hd, ".avm")
	}
	return filepath.Join(root, "boundaries", Name, agentName), nil
}

// userOpenclawHomes returns ~/.openclaw plus an explicit OPENCLAW_STATE_DIR.
func userOpenclawHomes() []string {
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
	if v := os.Getenv(EnvStateDir); v != "" {
		add(v)
	}
	if hd, err := os.UserHomeDir(); err == nil && hd != "" {
		add(filepath.Join(hd, ".openclaw"))
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

// scanMCPFromConfig pulls mcp.servers names from openclaw.json.
func scanMCPFromConfig(path string) []model.GlobalCapability {
	out := []model.GlobalCapability{}
	b, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return out
	}
	mcpRaw, ok := raw["mcp"]
	if !ok {
		return out
	}
	var mcp map[string]json.RawMessage
	if err := json.Unmarshal(mcpRaw, &mcp); err != nil {
		return out
	}
	serversRaw, ok := mcp["servers"]
	if !ok {
		return out
	}
	var byName map[string]json.RawMessage
	if err := json.Unmarshal(serversRaw, &byName); err != nil {
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

// renderConfig emits a minimal openclaw.json that flips the unsafe
// defaults (per research §4) and declares the AVM-managed MCP server set.
func renderConfig(a *model.Agent) ([]byte, error) {
	type sandbox struct {
		Mode string `json:"mode"`
	}
	type mcp struct {
		Servers map[string]struct{} `json:"servers,omitempty"`
	}
	type config struct {
		Sandbox        sandbox `json:"sandbox"`
		WorkspaceOnly  bool    `json:"workspaceOnly"`
		Approval       string  `json:"approval"`
		MCP            mcp     `json:"mcp"`
		AVMAgentName   string  `json:"_avmAgentName,omitempty"`
		AVMAgentSchema string  `json:"_avmSchema,omitempty"`
	}
	c := config{
		Sandbox:        sandbox{Mode: "off"}, // honest default; infra may upgrade if Docker is available.
		WorkspaceOnly:  true,
		Approval:       "moderate",
		AVMAgentName:   a.Identity.Name,
		AVMAgentSchema: "v0",
	}
	if len(a.MCP) > 0 {
		c.MCP.Servers = map[string]struct{}{}
		for _, m := range a.MCP {
			c.MCP.Servers[string(m.ID)] = struct{}{}
		}
	}
	return json.MarshalIndent(c, "", "  ")
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
