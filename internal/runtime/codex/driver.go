// Package codex implements the Codex RuntimeDriver. It is responsible for
// translating AVM Agent semantics into CODEX_HOME-managed config files,
// detecting the codex binary, computing the per-Agent isolation boundary,
// and producing a launch spec.
//
// References: docs/engineering/runtime-research/codex-runtime.md
package codex

import (
	"context"
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
const Name = "codex"

// EnvHome is the env var Codex honors to relocate its state directory.
const EnvHome = "CODEX_HOME"

// Driver is the Codex runtime adapter. Construction is via New so we
// can later inject filesystem helpers, env probes, etc.
type Driver struct{}

// New returns a Codex driver.
func New() *Driver { return &Driver{} }

// Name reports the canonical registry key.
func (d *Driver) Name() string { return Name }

// Facts probes the codex binary, version, and declares static
// capabilities/risks documented in the runtime research notes.
func (d *Driver) Facts(ctx context.Context) (runtime.Facts, error) {
	bin, err := exec.LookPath("codex")
	if err != nil {
		// missing binary is not an error: report unavailable.
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
		},
		Risks: []runtime.Risk{
			{Code: "codex.auth-fork", Message: "per-Agent CODEX_HOME does not share auth.json with user home; first run may require re-login unless auth.json is copied in."},
			{Code: "codex.memory-subsystem", Message: "Codex memory subsystem writes artifacts and runs background jobs under CODEX_HOME/memories; isolated per Agent but still occupies disk."},
			{Code: "codex.skill-mcp-deps", Message: "Skill-declared MCP dependencies can mutate the user-level config.toml outside AVM's view."},
			{Code: "codex.approval-not-durable", Message: "Approval decisions are session-scoped and not persisted across runs."},
		},
	}, nil
}

// DiscoverGlobal scans Codex global skill/MCP locations.
func (d *Driver) DiscoverGlobal(ctx context.Context) ([]model.GlobalCapability, error) {
	out := []model.GlobalCapability{}
	homes, err := codexUserHomes()
	if err != nil {
		return nil, err
	}
	for _, root := range homes {
		// Skills live at <CODEX_HOME>/skills/<name>/SKILL.md
		out = append(out, scanSkillDir(filepath.Join(root, "skills"))...)
	}
	// User-scope skills root that Codex also scans.
	if hd, err := os.UserHomeDir(); err == nil && hd != "" {
		out = append(out, scanSkillDir(filepath.Join(hd, ".agents", "skills"))...)
	}
	// MCP servers from <CODEX_HOME>/config.toml (mcp_servers table)
	for _, root := range homes {
		out = append(out, scanMCPFromConfig(filepath.Join(root, "config.toml"))...)
	}
	return out, nil
}

// Plan renders the Agent into Codex's managed config files.
func (d *Driver) Plan(ctx context.Context, agent *model.Agent) (*runtime.Plan, error) {
	if agent == nil {
		return nil, errors.New("codex: nil agent")
	}
	if err := agent.Validate(); err != nil {
		return nil, fmt.Errorf("codex: %w", err)
	}
	bnd, err := d.Boundary(ctx, agent)
	if err != nil {
		return nil, err
	}

	plan := &runtime.Plan{}

	// Build instructions text: AVM Agent identity & instructions become
	// developer instructions in AGENTS.md (Codex native loader path).
	instructions := renderInstructions(agent)
	agentsMD := filepath.Join(bnd.StateDir, "AGENTS.md")
	plan.Files = append(plan.Files, runtime.ManagedFile{
		Path:     agentsMD,
		Mode:     0o600,
		Contents: []byte(instructions),
	})

	// Render config.toml that pins the runtime to AVM-managed roots.
	configTOML := filepath.Join(bnd.StateDir, "config.toml")
	plan.Files = append(plan.Files, runtime.ManagedFile{
		Path:     configTOML,
		Mode:     0o600,
		Contents: []byte(renderConfigTOML(agent)),
	})

	// Per-field mapping
	plan.Mappings = append(plan.Mappings,
		runtime.FieldMapping{
			Field: "identity.name", Status: model.MappingNative,
			Note: "rendered into AGENTS.md heading and CODEX_HOME isolation key",
		},
		runtime.FieldMapping{
			Field: "identity.description", Status: model.MappingNative,
			Note: "rendered into AGENTS.md preface",
		},
	)

	if agent.Identity.Role != "" {
		plan.Mappings = append(plan.Mappings, runtime.FieldMapping{
			Field: "identity.role", Status: model.MappingRenderedAsInstructions,
			Note: "Codex has no native role slot; role text concatenated into AGENTS.md.",
		})
	}

	plan.Mappings = append(plan.Mappings, runtime.FieldMapping{
		Field: "instructions", Status: model.MappingNative,
		Note: "written to <CODEX_HOME>/AGENTS.md (developer instructions)",
	})

	if len(agent.Skills) > 0 {
		plan.Mappings = append(plan.Mappings, runtime.FieldMapping{
			Field: "skills", Status: model.MappingNative,
			Note: "AVM materializes skills into <CODEX_HOME>/skills/<id>; infra performs the linking.",
		})
	}
	if len(agent.MCP) > 0 {
		plan.Mappings = append(plan.Mappings, runtime.FieldMapping{
			Field: "mcp", Status: model.MappingNative,
			Note: "rendered into <CODEX_HOME>/config.toml [mcp_servers] table.",
		})
	}
	if len(agent.Runtimes) > 0 {
		plan.Mappings = append(plan.Mappings, runtime.FieldMapping{
			Field: "runtimes", Status: model.MappingIgnored,
			Note: "AVM-side preference; never written into Codex config.",
		})
	}

	// Warnings
	plan.Warnings = append(plan.Warnings, model.Warning{
		Code:    "codex.auth-fork",
		Message: "Per-Agent CODEX_HOME isolates auth.json; first run may prompt re-login unless an existing auth.json is copied in.",
	})
	plan.Warnings = append(plan.Warnings, model.Warning{
		Code:    "codex.memory-side-effects",
		Message: "Codex memory subsystem will write artifacts under " + filepath.Join(bnd.StateDir, "memories") + " and may run background jobs.",
	})

	return plan, nil
}

// Boundary returns the per-Agent CODEX_HOME and env vars.
func (d *Driver) Boundary(ctx context.Context, agent *model.Agent) (runtime.Boundary, error) {
	if agent == nil {
		return runtime.Boundary{}, errors.New("codex: nil agent")
	}
	if agent.Identity.Name == "" {
		return runtime.Boundary{}, errors.New("codex: agent identity.name required")
	}
	root, err := boundaryStateDir(agent.Identity.Name)
	if err != nil {
		return runtime.Boundary{}, err
	}
	return runtime.Boundary{
		StateDir: root,
		Env: map[string]string{
			EnvHome: root,
		},
	}, nil
}

// LaunchSpec describes how to spawn codex.
func (d *Driver) LaunchSpec(ctx context.Context, agent *model.Agent, plan *runtime.Plan) (runtime.LaunchSpec, error) {
	facts, err := d.Facts(ctx)
	if err != nil {
		return runtime.LaunchSpec{}, err
	}
	if !facts.Available {
		return runtime.LaunchSpec{}, errors.New("codex: binary not available")
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
		Args:  []string{}, // bare `codex` enters the interactive TUI
		Env:   env,
		Stdin: true,
	}, nil
}

// boundaryStateDir computes $AVM_HOME/boundaries/codex/<agent-name> with
// the same fallback logic as internal/infra/home, but locally so the
// driver does not depend on that package.
func boundaryStateDir(agentName string) (string, error) {
	root := os.Getenv("AVM_HOME")
	if root == "" {
		hd, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if hd == "" {
			return "", errors.New("codex: empty user home dir")
		}
		root = filepath.Join(hd, ".avm")
	}
	return filepath.Join(root, "boundaries", Name, agentName), nil
}

// codexUserHomes returns the candidate user-level Codex home dirs to scan
// for global discovery: the explicit CODEX_HOME if set, plus ~/.codex.
func codexUserHomes() ([]string, error) {
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
	if v := os.Getenv(EnvHome); v != "" {
		add(v)
	}
	hd, err := os.UserHomeDir()
	if err == nil && hd != "" {
		add(filepath.Join(hd, ".codex"))
	}
	return out, nil
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
			// Not a skill dir; skip.
			continue
		}
		ver := readSkillVersion(manifest)
		out = append(out, model.GlobalCapability{
			Runtime: Name,
			Kind:    model.CapabilityKindSkill,
			Name:    name,
			Path:    skillDir,
			Version: ver,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// readSkillVersion looks for a YAML frontmatter `version:` line in
// SKILL.md. Best-effort; returns "" on any failure.
func readSkillVersion(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	if len(b) < 4 || string(b[:3]) != "---" {
		return ""
	}
	// Read until the next "---" or EOF.
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

// scanMCPFromConfig pulls top-level `[mcp_servers.NAME]` sections out of
// a config.toml. We don't need a full TOML parser — we only want the
// section names so the user can see what runtime-global MCP exists.
func scanMCPFromConfig(path string) []model.GlobalCapability {
	out := []model.GlobalCapability{}
	b, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	for _, raw := range strings.Split(string(b), "\n") {
		line := strings.TrimSpace(raw)
		if !strings.HasPrefix(line, "[mcp_servers.") {
			continue
		}
		line = strings.TrimSuffix(line, "]")
		name := strings.TrimPrefix(line, "[mcp_servers.")
		name = strings.Trim(name, "\"' ")
		if name == "" {
			continue
		}
		out = append(out, model.GlobalCapability{
			Runtime: Name,
			Kind:    model.CapabilityKindMCP,
			Name:    name,
			Path:    path,
		})
	}
	return out
}

// renderInstructions builds an AGENTS.md body from the Agent's identity
// and instructions. It is used both for the native instructions slot
// and for "rendered_as_instructions" overflow (e.g. role).
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

// renderConfigTOML emits a minimal Codex config.toml that records the
// AVM-managed MCP server set. Skill bundling stays off so AVM controls
// what skills are visible.
func renderConfigTOML(a *model.Agent) string {
	var b strings.Builder
	b.WriteString("# AVM-managed Codex config.toml\n")
	b.WriteString("# Do not edit by hand; AVM rewrites this file on each run.\n\n")
	b.WriteString("[skills.bundled]\n")
	b.WriteString("enabled = false\n\n")
	if len(a.MCP) > 0 {
		b.WriteString("# MCP servers materialized from AVM Agent definition.\n")
		for _, m := range a.MCP {
			fmt.Fprintf(&b, "[mcp_servers.%q]\n", string(m.ID))
			b.WriteString("# AVM capability reference — actual command wiring resolved by infra layer.\n\n")
		}
	}
	return b.String()
}

// probeVersion runs `<bin> --version` with a short timeout. Returns ""
// on any failure; never returns an error to callers.
func probeVersion(ctx context.Context, bin string) string {
	out, err := runVersion(ctx, bin)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

// runVersion is split out so tests can stub it via a fake binary on PATH.
func runVersion(ctx context.Context, bin string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	subCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(subCtx, bin, "--version")
	cmd.Stdin = nil
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
