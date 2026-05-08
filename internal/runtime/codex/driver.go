// Package codex implements the Codex RuntimeDriver. It is responsible for
// translating AVM Agent semantics into CODEX_HOME-managed config files,
// detecting the codex binary, computing the per-Agent isolation boundary,
// and producing a launch spec.
//
// References: docs/engineering/runtime-research/codex-runtime.md
package codex

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/xz1220/agent-vm/internal/app/model"
	"github.com/xz1220/agent-vm/internal/infra/capstore"
	"github.com/xz1220/agent-vm/internal/runtime"
)

// Name is the canonical Registry key for this driver.
const Name = "codex"

// EnvHome is the env var Codex honors to relocate its state directory.
const EnvHome = "CODEX_HOME"

// Driver is the Codex runtime adapter. Construction is via New so we
// can later inject filesystem helpers, env probes, etc.
//
// Caps is consulted by Plan to materialize Agent-referenced skill /
// MCP capability payloads into the boundary directory. It may be nil
// in tests that exercise paths not requiring capability content.
type Driver struct {
	Caps capstore.Store
}

// New returns a Codex driver bound to the given capability store.
func New(caps capstore.Store) *Driver {
	return &Driver{Caps: caps}
}

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
			{Code: "codex.skill-mcp-deps", Message: "Skill-declared MCP dependencies can mutate the boundary config.toml outside AVM's view."},
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

// ExportGlobal materializes a single Codex global capability into AVM
// canonical bytes. Skills are read straight from <skill-dir>/SKILL.md;
// MCP servers are extracted from CODEX_HOME/config.toml's
// [mcp_servers.NAME] table and serialized as MCPConfigV1 JSON.
func (d *Driver) ExportGlobal(ctx context.Context, kind model.CapabilityKind, name string) (runtime.Exported, error) {
	if name == "" {
		return runtime.Exported{}, errors.New("codex: empty capability name")
	}
	candidates, err := d.DiscoverGlobal(ctx)
	if err != nil {
		return runtime.Exported{}, err
	}
	var match *model.GlobalCapability
	for i := range candidates {
		c := candidates[i]
		if c.Kind == kind && c.Name == name {
			match = &candidates[i]
			break
		}
	}
	if match == nil {
		return runtime.Exported{}, runtime.ErrGlobalCapabilityNotFound
	}

	switch kind {
	case model.CapabilityKindSkill:
		manifest := filepath.Join(match.Path, "SKILL.md")
		f, err := os.Open(manifest)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return runtime.Exported{}, runtime.ErrGlobalCapabilityNotFound
			}
			return runtime.Exported{}, fmt.Errorf("codex: open SKILL.md: %w", err)
		}
		return runtime.Exported{
			Capability: *match,
			Format:     model.PayloadFormatSkillMD,
			Content:    f,
			Filename:   "SKILL.md",
		}, nil
	case model.CapabilityKindMCP:
		// match.Path is the config.toml file the section came from.
		raw, err := os.ReadFile(match.Path)
		if err != nil {
			return runtime.Exported{}, fmt.Errorf("codex: read config: %w", err)
		}
		cfg, err := codexExtractMCP(raw, name)
		if err != nil {
			return runtime.Exported{}, err
		}
		buf, err := json.MarshalIndent(cfg, "", "  ")
		if err != nil {
			return runtime.Exported{}, err
		}
		return runtime.Exported{
			Capability: *match,
			Format:     model.PayloadFormatMCPConfigV1,
			Content:    io.NopCloser(bytes.NewReader(buf)),
			Filename:   "mcp.json",
		}, nil
	}
	return runtime.Exported{}, fmt.Errorf("codex: unsupported kind %q", kind)
}

// codexExtractMCP parses a Codex config.toml byte slice and pulls
// [mcp_servers.NAME] into a canonical MCPConfigV1. Unknown TOML keys
// inside the section are preserved in Extra so an import→export round
// trip keeps fidelity.
func codexExtractMCP(raw []byte, name string) (runtime.MCPConfigV1, error) {
	var doc struct {
		MCPServers map[string]map[string]any `toml:"mcp_servers"`
	}
	if err := toml.Unmarshal(raw, &doc); err != nil {
		return runtime.MCPConfigV1{}, fmt.Errorf("codex: parse config.toml: %w", err)
	}
	section, ok := doc.MCPServers[name]
	if !ok {
		return runtime.MCPConfigV1{}, runtime.ErrGlobalCapabilityNotFound
	}

	out := runtime.MCPConfigV1{Kind: string(model.CapabilityKindMCP), Name: name}
	extra := map[string]any{}
	for k, v := range section {
		switch k {
		case "command":
			if s, ok := v.(string); ok {
				out.Command = s
			} else {
				extra[k] = v
			}
		case "args":
			out.Args = toStringSlice(v)
			if out.Args == nil {
				extra[k] = v
			}
		case "env":
			out.Env = toStringMap(v)
			if out.Env == nil {
				extra[k] = v
			}
		default:
			extra[k] = v
		}
	}
	if len(extra) > 0 {
		out.Extra = extra
	}
	return out, nil
}

func toStringSlice(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, e := range arr {
		s, ok := e.(string)
		if !ok {
			return nil
		}
		out = append(out, s)
	}
	return out
}

func toStringMap(v any) map[string]string {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, val := range m {
		s, ok := val.(string)
		if !ok {
			return nil
		}
		out[k] = s
	}
	return out
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

	// Resolve all MCP capability content from capstore upfront so
	// renderConfigTOML can emit complete [mcp_servers.<name>] sections.
	mcps, err := d.resolveMCPConfigs(agent.MCP)
	if err != nil {
		return nil, err
	}

	// Build instructions text: AVM Agent identity & instructions become
	// developer instructions in AGENTS.md (Codex native loader path).
	instructions := renderInstructions(agent)
	plan.Files = append(plan.Files, runtime.ManagedFile{
		Path:     filepath.Join(bnd.StateDir, "AGENTS.md"),
		Mode:     0o600,
		Contents: []byte(instructions),
	})

	// Render config.toml that pins the runtime to AVM-managed roots.
	plan.Files = append(plan.Files, runtime.ManagedFile{
		Path:     filepath.Join(bnd.StateDir, "config.toml"),
		Mode:     0o600,
		Contents: []byte(renderConfigTOML(mcps)),
	})

	// Skills: copy each capstore-resident SKILL.md into the boundary
	// under skills/<name>/SKILL.md so Codex's loader (root #2) finds them.
	skillFiles, err := d.materializeSkills(bnd.StateDir, agent.Skills)
	if err != nil {
		return nil, err
	}
	plan.Files = append(plan.Files, skillFiles...)

	// auth.json: best-effort copy from user-level ~/.codex/auth.json.
	// Missing source is silently skipped (user not logged in to codex
	// globally yet — codex itself will prompt on first run). Real IO
	// errors are surfaced as a warning so Plan still proceeds.
	if authFile, warning := readUserAuthJSON(bnd.StateDir); authFile != nil {
		plan.Files = append(plan.Files, *authFile)
	} else if warning != nil {
		plan.Warnings = append(plan.Warnings, *warning)
	}

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
			Note: "materialized as <CODEX_HOME>/skills/<name>/SKILL.md for codex's user-scope loader.",
		})
	}
	if len(agent.MCP) > 0 {
		plan.Mappings = append(plan.Mappings, runtime.FieldMapping{
			Field: "mcp", Status: model.MappingNative,
			Note: "rendered into <CODEX_HOME>/config.toml [mcp_servers.<name>] sections.",
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

// resolvedMCP pairs an MCP capability's logical name (the section key
// in config.toml) with its parsed mcp_config_v1 payload.
type resolvedMCP struct {
	Name string
	Cfg  runtime.MCPConfigV1
}

// resolveMCPConfigs reads each MCP capability from capstore and parses
// the payload as MCPConfigV1. Two refs resolving to the same logical
// name are rejected since they would collide in TOML.
func (d *Driver) resolveMCPConfigs(refs []model.CapabilityRef) ([]resolvedMCP, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	if d.Caps == nil {
		return nil, errors.New("codex: capability store not configured")
	}
	seen := map[string]struct{}{}
	out := make([]resolvedMCP, 0, len(refs))
	for _, ref := range refs {
		rec, err := d.Caps.Get(ref.ID)
		if err != nil {
			return nil, fmt.Errorf("codex: mcp %s: %w", ref.ID, err)
		}
		if _, dup := seen[rec.Name]; dup {
			return nil, fmt.Errorf("codex: agent has multiple MCPs named %q; pick one", rec.Name)
		}
		seen[rec.Name] = struct{}{}
		body, err := d.Caps.ReadPayload(ref.ID)
		if err != nil {
			return nil, fmt.Errorf("codex: read mcp %s payload: %w", ref.ID, err)
		}
		var cfg runtime.MCPConfigV1
		if err := json.Unmarshal(body, &cfg); err != nil {
			return nil, fmt.Errorf("codex: parse mcp_config_v1 for %s: %w", rec.Name, err)
		}
		if cfg.Name == "" {
			cfg.Name = rec.Name
		}
		out = append(out, resolvedMCP{Name: rec.Name, Cfg: cfg})
	}
	return out, nil
}

// materializeSkills returns one ManagedFile per skill ref placing the
// capstore payload at <boundary>/skills/<name>/SKILL.md.
func (d *Driver) materializeSkills(boundary string, refs []model.CapabilityRef) ([]runtime.ManagedFile, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	if d.Caps == nil {
		return nil, errors.New("codex: capability store not configured")
	}
	seen := map[string]struct{}{}
	out := make([]runtime.ManagedFile, 0, len(refs))
	for _, ref := range refs {
		rec, err := d.Caps.Get(ref.ID)
		if err != nil {
			return nil, fmt.Errorf("codex: skill %s: %w", ref.ID, err)
		}
		if _, dup := seen[rec.Name]; dup {
			return nil, fmt.Errorf("codex: agent has multiple skills named %q; pick one", rec.Name)
		}
		seen[rec.Name] = struct{}{}
		body, err := d.Caps.ReadPayload(ref.ID)
		if err != nil {
			return nil, fmt.Errorf("codex: read skill %s payload: %w", ref.ID, err)
		}
		out = append(out, runtime.ManagedFile{
			Path:     filepath.Join(boundary, "skills", rec.Name, "SKILL.md"),
			Mode:     0o644,
			Contents: body,
		})
	}
	return out, nil
}

// readUserAuthJSON copies the user-level codex credentials into the
// boundary so per-Agent CODEX_HOME does not require re-login. Missing
// source returns (nil, nil); other IO errors return (nil, warning).
func readUserAuthJSON(boundary string) (*runtime.ManagedFile, *model.Warning) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil, nil
	}
	src := filepath.Join(home, ".codex", "auth.json")
	data, err := os.ReadFile(src)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, &model.Warning{
			Code:    "codex.auth-read-failed",
			Message: "could not read user-level auth.json: " + err.Error(),
		}
	}
	return &runtime.ManagedFile{
		Path:     filepath.Join(boundary, "auth.json"),
		Mode:     0o600,
		Contents: data,
	}, nil
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
			"HOME":  root,
		},
	}, nil
}

// LaunchSpec describes how to spawn codex.
//
// Codex on Node-based installs (npm / nvm) starts via a `#!/usr/bin/env
// node` shebang, so the spawned process needs PATH (and friends) to
// resolve `node`. process.Runner replaces the child env wholesale when
// spec.Env is non-empty, so we explicitly inherit the parent process
// environment first and then override with our per-Agent boundary
// values. HOME is intentionally isolated too: current Codex builds also
// scan ~/.agents/skills, so CODEX_HOME alone would still leak user-global
// skills into an AVM Agent run.
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
	env := inheritEnviron(os.Environ())
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

// inheritEnviron parses an os.Environ() slice into a map.
func inheritEnviron(parent []string) map[string]string {
	out := make(map[string]string, len(parent))
	for _, e := range parent {
		i := strings.IndexByte(e, '=')
		if i <= 0 {
			continue
		}
		out[e[:i]] = e[i+1:]
	}
	return out
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
// a config.toml using a real TOML parser so nested sub-tables (e.g.
// `[mcp_servers.gh.env]`) are not mistaken for independent servers.
func scanMCPFromConfig(path string) []model.GlobalCapability {
	out := []model.GlobalCapability{}
	raw, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	var doc struct {
		MCPServers map[string]map[string]any `toml:"mcp_servers"`
	}
	if err := toml.Unmarshal(raw, &doc); err != nil {
		return out
	}
	names := make([]string, 0, len(doc.MCPServers))
	for name := range doc.MCPServers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
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

// renderConfigTOML emits a Codex config.toml that:
//  1. Disables Codex's bundled-with-binary skills (AVM controls skills).
//  2. Emits a complete [mcp_servers.<name>] section for every Agent MCP,
//     using the capstore record's Name (not the opaque ID) so codex
//     surfaces tools as "<name>/<tool>".
func renderConfigTOML(mcps []resolvedMCP) string {
	var b strings.Builder
	b.WriteString("# AVM-managed Codex config.toml\n")
	b.WriteString("# Do not edit by hand; AVM rewrites this file on each run.\n\n")
	b.WriteString("[skills.bundled]\n")
	b.WriteString("enabled = false\n\n")
	if len(mcps) == 0 {
		return b.String()
	}
	b.WriteString("# MCP servers materialized from AVM Agent definition.\n")
	for _, m := range mcps {
		fmt.Fprintf(&b, "[mcp_servers.%q]\n", m.Name)
		if m.Cfg.Command != "" {
			fmt.Fprintf(&b, "command = %q\n", m.Cfg.Command)
		}
		if len(m.Cfg.Args) > 0 {
			b.WriteString("args = [")
			for i, a := range m.Cfg.Args {
				if i > 0 {
					b.WriteString(", ")
				}
				fmt.Fprintf(&b, "%q", a)
			}
			b.WriteString("]\n")
		}
		if len(m.Cfg.Env) > 0 {
			keys := make([]string, 0, len(m.Cfg.Env))
			for k := range m.Cfg.Env {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			b.WriteString("env = { ")
			for i, k := range keys {
				if i > 0 {
					b.WriteString(", ")
				}
				fmt.Fprintf(&b, "%q = %q", k, m.Cfg.Env[k])
			}
			b.WriteString(" }\n")
		}
		b.WriteString("\n")
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
