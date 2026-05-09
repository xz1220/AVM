// Package claudecode implements the Claude Code RuntimeDriver. It maps
// AVM Agent semantics into Claude Code's CLAUDE_CONFIG_DIR-managed
// settings, detects the `claude` binary, and computes the per-Agent
// boundary used by AVM run.
//
// References: docs/engineering/runtime-research/claude-code-runtime.md
package claudecode

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

	"github.com/xz1220/agent-vm/internal/app/model"
	"github.com/xz1220/agent-vm/internal/infra/capstore"
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
//
// Caps is consulted by Plan to materialize Agent-referenced skill / MCP
// capability payloads into the boundary directory. Tests that exercise
// only Facts / DiscoverGlobal / ExportGlobal / Boundary / LaunchSpec may
// pass nil; Plan with non-empty refs requires it.
type Driver struct {
	Caps capstore.Store
}

// New returns a Claude Code driver bound to the given capability store.
// caps may be nil if the driver is only used for facts/discovery.
func New(caps capstore.Store) *Driver { return &Driver{Caps: caps} }

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
//
// Skill sources:
//   - <home>/skills/<name>/SKILL.md            (top-level user skills)
//   - <home>/plugins/*/skills/<name>/SKILL.md  (plugin-bundled skills)
//
// MCP sources:
//   - <home>/.claude.json's mcpServers         (primary user config)
//   - <home>/managed-settings.json's mcpServers (enterprise/admin push)
//
// Each source is best-effort; a missing or malformed file silently
// contributes zero candidates rather than failing the whole call.
func (d *Driver) DiscoverGlobal(ctx context.Context) ([]model.GlobalCapability, error) {
	out := []model.GlobalCapability{}
	seen := map[string]struct{}{}
	add := func(items []model.GlobalCapability) {
		for _, c := range items {
			key := string(c.Kind) + "\x00" + c.Name + "\x00" + c.Path
			if _, dup := seen[key]; dup {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, c)
		}
	}

	homes := userClaudeHomes()
	for _, root := range homes {
		add(scanSkillDir(filepath.Join(root, "skills")))
		add(scanPluginSkillDirs(filepath.Join(root, "plugins")))
	}

	for _, jsonPath := range globalConfigPaths() {
		add(scanMCPFromGlobalConfig(jsonPath))
	}
	for _, root := range homes {
		add(scanMCPFromGlobalConfig(filepath.Join(root, "managed-settings.json")))
	}
	return out, nil
}

// ExportGlobal materializes a single Claude Code global capability into
// AVM canonical bytes. Skills come from <skill-dir>/SKILL.md verbatim;
// MCP servers are pulled from ~/.claude.json's mcpServers object and
// reshaped into MCPConfigV1 JSON.
func (d *Driver) ExportGlobal(ctx context.Context, kind model.CapabilityKind, name string) (runtime.Exported, error) {
	if name == "" {
		return runtime.Exported{}, errors.New("claude-code: empty capability name")
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
			return runtime.Exported{}, fmt.Errorf("claude-code: open SKILL.md: %w", err)
		}
		return runtime.Exported{
			Capability: *match,
			Format:     model.PayloadFormatSkillMD,
			Content:    f,
			Filename:   "SKILL.md",
		}, nil
	case model.CapabilityKindMCP:
		raw, err := os.ReadFile(match.Path)
		if err != nil {
			return runtime.Exported{}, fmt.Errorf("claude-code: read config: %w", err)
		}
		cfg, err := claudeExtractMCP(raw, name)
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
	return runtime.Exported{}, fmt.Errorf("claude-code: unsupported kind %q", kind)
}

// claudeExtractMCP parses a .claude.json byte slice and pulls
// mcpServers[name] into MCPConfigV1.
func claudeExtractMCP(raw []byte, name string) (runtime.MCPConfigV1, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		return runtime.MCPConfigV1{}, fmt.Errorf("claude-code: parse config: %w", err)
	}
	serversRaw, ok := top["mcpServers"]
	if !ok {
		return runtime.MCPConfigV1{}, runtime.ErrGlobalCapabilityNotFound
	}
	var servers map[string]map[string]any
	if err := json.Unmarshal(serversRaw, &servers); err != nil {
		return runtime.MCPConfigV1{}, fmt.Errorf("claude-code: parse mcpServers: %w", err)
	}
	section, ok := servers[name]
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
			if a := jsonToStringSlice(v); a != nil {
				out.Args = a
			} else if v != nil {
				extra[k] = v
			}
		case "env":
			if m := jsonToStringMap(v); m != nil {
				out.Env = m
			} else if v != nil {
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

func jsonToStringSlice(v any) []string {
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

func jsonToStringMap(v any) map[string]string {
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

	// Resolve all MCP capability content from capstore upfront so
	// renderSettings can emit complete mcpServers entries (command/args/env)
	// keyed by the capability name (not the opaque cap_xxx ID).
	mcps, err := d.resolveMCPConfigs(agent.MCP)
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

	// settings.json: user defaults (model/theme/permissions/...) merged
	// with AVM-owned mcpServers. See renderSettings doc.
	settingsBytes, settingsWarn, err := renderSettings(mcps)
	if err != nil {
		return nil, fmt.Errorf("claude-code: render settings: %w", err)
	}
	plan.Files = append(plan.Files, runtime.ManagedFile{
		Path:     filepath.Join(bnd.StateDir, "settings.json"),
		Mode:     0o600,
		Contents: settingsBytes,
	})
	if settingsWarn != nil {
		plan.Warnings = append(plan.Warnings, *settingsWarn)
	}

	// settings.local.json: best-effort copy of user-level allow/deny
	// permission overlay so users keep their Bash allowances per Agent.
	if localFile, warning := readUserLocalSettings(bnd.StateDir); localFile != nil {
		plan.Files = append(plan.Files, *localFile)
	} else if warning != nil {
		plan.Warnings = append(plan.Warnings, *warning)
	}

	// Skills: materialize each capstore-resident SKILL.md into the
	// boundary at skills/<name>/SKILL.md so Claude Code's user-scope
	// loader picks it up under CLAUDE_CONFIG_DIR.
	skillFiles, err := d.materializeSkills(bnd.StateDir, agent.Skills)
	if err != nil {
		return nil, err
	}
	plan.Files = append(plan.Files, skillFiles...)

	// .credentials.json: best-effort copy from the user-level Claude
	// Code credentials so per-Agent CLAUDE_CONFIG_DIR doesn't force a
	// fresh OAuth round on every Agent. Missing source is silent;
	// real IO errors become a Plan warning.
	if credFile, warning := readUserCredentials(bnd.StateDir); credFile != nil {
		plan.Files = append(plan.Files, *credFile)
	} else if warning != nil {
		plan.Warnings = append(plan.Warnings, *warning)
	}

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
			Note: "materialized as <CLAUDE_CONFIG_DIR>/skills/<name>/SKILL.md for Claude Code's user-scope loader.",
		})
	}
	if len(agent.MCP) > 0 {
		plan.Mappings = append(plan.Mappings, runtime.FieldMapping{
			Field: "mcp", Status: model.MappingNative,
			Note: "rendered into settings.json mcpServers with full command/args/env.",
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

// resolvedMCP pairs an MCP capability's logical name with its parsed
// MCPConfigV1 payload, mirroring the codex driver's helper.
type resolvedMCP struct {
	Name string
	Cfg  runtime.MCPConfigV1
}

// resolveMCPConfigs reads each MCP capability from capstore and parses
// the payload as MCPConfigV1. Two refs resolving to the same logical
// name are rejected since they would collide as JSON keys.
func (d *Driver) resolveMCPConfigs(refs []model.CapabilityRef) ([]resolvedMCP, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	if d.Caps == nil {
		return nil, errors.New("claude-code: capability store not configured")
	}
	seen := map[string]struct{}{}
	out := make([]resolvedMCP, 0, len(refs))
	for _, ref := range refs {
		rec, err := d.Caps.Get(ref.ID)
		if err != nil {
			return nil, fmt.Errorf("claude-code: mcp %s: %w", ref.ID, err)
		}
		if _, dup := seen[rec.Name]; dup {
			return nil, fmt.Errorf("claude-code: agent has multiple MCPs named %q; pick one", rec.Name)
		}
		seen[rec.Name] = struct{}{}
		body, err := d.Caps.ReadPayload(ref.ID)
		if err != nil {
			return nil, fmt.Errorf("claude-code: read mcp %s payload: %w", ref.ID, err)
		}
		var cfg runtime.MCPConfigV1
		if err := json.Unmarshal(body, &cfg); err != nil {
			return nil, fmt.Errorf("claude-code: parse mcp_config_v1 for %s: %w", rec.Name, err)
		}
		if cfg.Name == "" {
			cfg.Name = rec.Name
		}
		out = append(out, resolvedMCP{Name: rec.Name, Cfg: cfg})
	}
	return out, nil
}

// materializeSkills returns one ManagedFile per skill ref placing the
// capstore payload at <boundary>/skills/<name>/SKILL.md. Same shape as
// the codex driver, only the parent dir layout matches Claude Code's
// user-scope skill loader (CLAUDE_CONFIG_DIR/skills/...).
func (d *Driver) materializeSkills(boundary string, refs []model.CapabilityRef) ([]runtime.ManagedFile, error) {
	if len(refs) == 0 {
		return nil, nil
	}
	if d.Caps == nil {
		return nil, errors.New("claude-code: capability store not configured")
	}
	seen := map[string]struct{}{}
	out := make([]runtime.ManagedFile, 0, len(refs))
	for _, ref := range refs {
		rec, err := d.Caps.Get(ref.ID)
		if err != nil {
			return nil, fmt.Errorf("claude-code: skill %s: %w", ref.ID, err)
		}
		if _, dup := seen[rec.Name]; dup {
			return nil, fmt.Errorf("claude-code: agent has multiple skills named %q; pick one", rec.Name)
		}
		seen[rec.Name] = struct{}{}
		body, err := d.Caps.ReadPayload(ref.ID)
		if err != nil {
			return nil, fmt.Errorf("claude-code: read skill %s payload: %w", ref.ID, err)
		}
		out = append(out, runtime.ManagedFile{
			Path:     filepath.Join(boundary, "skills", rec.Name, "SKILL.md"),
			Mode:     0o644,
			Contents: body,
		})
	}
	return out, nil
}

// readUserLocalSettings copies ~/.claude/settings.local.json into the
// boundary so users keep their per-allow Bash permissions across
// Agents. settings.local.json is a user-edited overlay (allow / deny /
// ask lists); rebuilding it from scratch on every Agent would be
// painful. Same best-effort semantics as readUserCredentials.
func readUserLocalSettings(boundary string) (*runtime.ManagedFile, *model.Warning) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil, nil
	}
	src := filepath.Join(home, ".claude", "settings.local.json")
	data, err := os.ReadFile(src)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, &model.Warning{
			Code:    "claude.local-settings-read-failed",
			Message: "could not read user-level settings.local.json: " + err.Error(),
		}
	}
	return &runtime.ManagedFile{
		Path:     filepath.Join(boundary, "settings.local.json"),
		Mode:     0o600,
		Contents: data,
	}, nil
}

// readUserCredentials copies the user-level Claude Code credentials
// (~/.claude/.credentials.json) into the boundary so per-Agent
// CLAUDE_CONFIG_DIR doesn't require a fresh OAuth round. The platform
// risk note (claude.plain-creds) already covers the security trade-off.
//
// Missing source returns (nil, nil); real IO failures return
// (nil, &Warning) so Plan still proceeds.
func readUserCredentials(boundary string) (*runtime.ManagedFile, *model.Warning) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return nil, nil
	}
	src := filepath.Join(home, ".claude", ".credentials.json")
	data, err := os.ReadFile(src)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, &model.Warning{
			Code:    "claude.creds-read-failed",
			Message: "could not read user-level .credentials.json: " + err.Error(),
		}
	}
	return &runtime.ManagedFile{
		Path:     filepath.Join(boundary, ".credentials.json"),
		Mode:     0o600,
		Contents: data,
	}, nil
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
			// HOME is isolated alongside CLAUDE_CONFIG_DIR because Claude
			// Code persists significant runtime state to ~/.claude.json
			// (oauthAccount, projects, skillUsage, onboarding flags),
			// which lives under HOME — not under CLAUDE_CONFIG_DIR. With
			// HOME unset Agents would share that state and the per-Agent
			// boundary would only hold for files under CLAUDE_CONFIG_DIR.
			"HOME":         root,
			EnvConfigDir:   root,
			EnvPluginCache: filepath.Join(root, "plugins"),
			EnvTmp:         filepath.Join(root, "tmp"),
			EnvDebugDir:    filepath.Join(root, "debug"),
		},
	}, nil
}

// LaunchSpec describes how to spawn `claude`.
//
// process.Runner replaces the child env wholesale when spec.Env is
// non-empty (see internal/infra/process/runner.go). Claude Code on
// Node-based installs (npm / nvm / volta) starts via a `#!/usr/bin/env
// node` shebang, so the spawned process needs PATH (and friends) to
// resolve `node`. We inherit the parent process environment first and
// then overlay the per-Agent boundary values so the four
// CLAUDE_CONFIG_DIR / plugin-cache / tmp / debug vars take precedence.
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
	env := inheritEnviron(os.Environ())
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

// inheritEnviron parses an os.Environ() slice into a map. Mirrors the
// codex driver's helper; kept local so each driver stays self-contained.
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
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		skillDir := filepath.Join(root, name)
		// Follow symlinks: ~/.claude/skills/<name> is commonly a symlink
		// pointing into ~/.agents/skills/<name>. e.IsDir() returns false
		// for symlinks (their mode is ModeSymlink, not ModeDir), so we
		// must Stat the path to resolve through the link before deciding.
		info, err := os.Stat(skillDir)
		if err != nil || !info.IsDir() {
			continue
		}
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

// scanPluginSkillDirs walks a plugins root (e.g. ~/.claude/plugins) and
// collects every <plugin>/skills/<name>/SKILL.md it finds. Plugin
// authors are free to ship skills alongside other assets, so we treat
// each plugin's "skills" subdir as a normal skill root.
func scanPluginSkillDirs(root string) []model.GlobalCapability {
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
		out = append(out, scanSkillDir(filepath.Join(root, name, "skills"))...)
	}
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

// renderSettings emits Claude Code settings.json with the user's own
// settings preserved and AVM-owned `mcpServers` overlaid on top.
//
// Per PRD §6, AVM only writes the keys it owns. AVM owns `mcpServers`
// (it knows command/args/env from the canonical MCPConfigV1 payload).
// Everything else — model, theme, permissions, effortLevel, hooks etc.
// — is read from the user's `~/.claude/settings.json` so per-Agent
// boundaries inherit the user's defaults instead of starting blank.
//
// The returned warning is non-nil only when a user settings.json was
// present but unparseable; missing-file is silent. Plan promotes the
// warning to plan.Warnings so the user sees why their settings aren't
// being inherited.
func renderSettings(mcps []resolvedMCP) ([]byte, *model.Warning, error) {
	base := map[string]any{}
	var warning *model.Warning
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		path := filepath.Join(home, ".claude", "settings.json")
		if data, rerr := os.ReadFile(path); rerr == nil {
			if jerr := json.Unmarshal(data, &base); jerr != nil {
				warning = &model.Warning{
					Code:    "claude.settings-parse-failed",
					Message: "could not parse user-level settings.json (falling back to AVM-only keys): " + jerr.Error(),
				}
				base = map[string]any{}
			}
		}
		// rerr non-nil + !IsNotExist would also be a warning, but the
		// best-effort contract is "only complain about parse errors";
		// genuine IO failure on a settings file is rare and Claude Code
		// itself will surface it on launch.
	}

	if len(mcps) > 0 {
		servers := map[string]map[string]any{}
		for _, m := range mcps {
			obj := map[string]any{}
			if m.Cfg.Command != "" {
				obj["command"] = m.Cfg.Command
			}
			if len(m.Cfg.Args) > 0 {
				obj["args"] = m.Cfg.Args
			}
			if len(m.Cfg.Env) > 0 {
				obj["env"] = m.Cfg.Env
			}
			for k, v := range m.Cfg.Extra {
				obj[k] = v
			}
			servers[m.Name] = obj
		}
		// AVM owns mcpServers fully — replace whatever the user had.
		base["mcpServers"] = servers
	} else {
		// No Agent MCP refs: drop any inherited mcpServers so the user
		// doesn't get user-global servers leaking into the boundary.
		delete(base, "mcpServers")
	}

	body, err := json.MarshalIndent(base, "", "  ")
	return body, warning, err
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
