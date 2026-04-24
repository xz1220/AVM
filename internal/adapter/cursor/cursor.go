// Package cursor renders AVM agents into Cursor's project-level PoC files.
package cursor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/xz1220/agent-vm/internal/adapter"
	"github.com/xz1220/agent-vm/internal/adapter/renderplan"
)

const (
	runtimeName     = "cursor"
	cursorDirName   = ".cursor"
	mcpFileName     = "mcp.json"
	rulesDirName    = "rules"
	ruleOperationID = "cursor-avm-rule"
	mcpOperationID  = "cursor-avm-mcp"
)

// Adapter renders the conservative Phase 1 Cursor path.
type Adapter struct {
	projectRoot string
}

type Option func(*Adapter)

func New(opts ...Option) *Adapter {
	a := &Adapter{}
	for _, opt := range opts {
		opt(a)
	}
	return a
}

func WithProjectRoot(projectRoot string) Option {
	return func(a *Adapter) {
		a.projectRoot = projectRoot
	}
}

func (a *Adapter) Name() string {
	return runtimeName
}

func (a *Adapter) Detect(ctx adapter.Context) adapter.Detection {
	configDir := a.cursorDir("")
	mcpPath := filepath.Join(configDir, mcpFileName)
	rulesDir := filepath.Join(configDir, rulesDirName)

	found := false
	for _, path := range []string{configDir, mcpPath, rulesDir} {
		if _, err := os.Stat(path); err == nil {
			found = true
			break
		}
	}

	version := ""
	if path, err := exec.LookPath(runtimeName); err == nil {
		found = true
		version = cursorVersion(ctx, path)
	}

	return adapter.Detection{
		Runtime:   runtimeName,
		Found:     found,
		Version:   version,
		ConfigDir: filepath.ToSlash(configDir),
		Warnings:  partialWarnings(),
	}
}

func (a *Adapter) Import(ctx adapter.Context) (*adapter.ImportResult, error) {
	_ = ctx

	return &adapter.ImportResult{
		Runtime: runtimeName,
		Warnings: append(partialWarnings(),
			"cursor import is read-only placeholder in Phase 1 because Cursor has no stable local Agent Profile format",
		),
	}, nil
}

func (a *Adapter) Plan(ctx adapter.Context, input adapter.RenderInput) (*adapter.RenderPlan, error) {
	_ = ctx

	runtime := input.Runtime
	if runtime == "" {
		runtime = runtimeName
	}
	if runtime != runtimeName {
		return nil, fmt.Errorf("cursor adapter cannot plan runtime %q", runtime)
	}

	agentName := firstNonEmpty(input.Agent.Name, input.Active.Name, "agent")
	ruleName := "avm-" + slug(agentName) + ".md"
	projectRoot := a.projectRootFor(input.ProjectRoot)
	mcpPath := filepath.ToSlash(filepath.Join(projectRoot, cursorDirName, mcpFileName))
	rulePath := filepath.ToSlash(filepath.Join(projectRoot, cursorDirName, rulesDirName, ruleName))

	render := renderContext{
		input:     input,
		agentName: agentName,
		mcpPath:   mcpPath,
		rulePath:  rulePath,
	}

	managedPaths := []adapter.ManagedPath{
		{
			Path:        rulePath,
			Owner:       "avm",
			Description: "Cursor rule rendered from AVM agent instructions.",
			Required:    true,
			MergeMode:   adapter.MergeModeWholeFile,
		},
	}
	operations := []adapter.RenderOperation{
		{
			ID:          ruleOperationID,
			Action:      adapter.OperationWriteFile,
			Path:        rulePath,
			Content:     []byte(render.renderRuleFile()),
			Description: "write Cursor AVM-managed rule file",
			Required:    true,
		},
	}

	if len(render.renderableMCPServers()) > 0 {
		managedPaths = append(managedPaths, adapter.ManagedPath{
			Path:        mcpPath,
			Owner:       "shared-section",
			Description: "Cursor MCP server entries managed by AVM.",
			Required:    true,
			MergeMode:   adapter.MergeModeStructuredSection,
		})
		operations = append(operations, adapter.RenderOperation{
			ID:          mcpOperationID,
			Action:      adapter.OperationStructuredSet,
			Path:        mcpPath,
			Content:     render.renderMCPSection(),
			Description: "merge Cursor AVM-managed MCP server entries",
			Required:    true,
		})
	}

	plan := &adapter.RenderPlan{
		Runtime:      runtimeName,
		Active:       input.Active,
		AgentName:    agentName,
		ManagedPaths: managedPaths,
		Operations:   operations,
		Mappings:     render.mappings(),
		Warnings:     render.warnings(),
	}

	return renderplan.Normalize(plan), nil
}

func (a *Adapter) Render(ctx adapter.Context, plan *adapter.RenderPlan) (*adapter.RenderResult, error) {
	_ = ctx

	normalized := renderplan.Normalize(plan)
	if normalized == nil {
		return nil, fmt.Errorf("cursor adapter render plan is nil")
	}
	if normalized.Runtime != "" && normalized.Runtime != runtimeName {
		return nil, fmt.Errorf("cursor adapter cannot render runtime %q", normalized.Runtime)
	}

	managed := managedPathIndex(normalized.ManagedPaths)
	for _, operation := range normalized.Operations {
		if _, ok := managed[operation.Path]; !ok {
			return nil, fmt.Errorf("cursor render operation %q targets unmanaged path %s", operation.ID, operation.Path)
		}
	}

	results := make([]adapter.RenderOperationResult, 0, len(normalized.Operations))
	for _, operation := range normalized.Operations {
		managedPath := managed[operation.Path]
		changed, err := applyOperation(operation, managedPath)
		if err != nil {
			return nil, err
		}
		results = append(results, adapter.RenderOperationResult{
			OperationID: operation.ID,
			Action:      operation.Action,
			Path:        operation.Path,
			Changed:     changed,
		})
	}

	return &adapter.RenderResult{
		Runtime:      runtimeName,
		Operations:   results,
		ManagedPaths: append([]adapter.ManagedPath(nil), normalized.ManagedPaths...),
		Mappings:     append([]adapter.FieldMapping(nil), normalized.Mappings...),
		Warnings:     append([]string(nil), normalized.Warnings...),
	}, nil
}

func (a *Adapter) ManagedPaths(ctx adapter.Context, plan *adapter.RenderPlan) []adapter.ManagedPath {
	_ = ctx

	normalized := renderplan.Normalize(plan)
	if normalized == nil {
		return nil
	}
	return append([]adapter.ManagedPath(nil), normalized.ManagedPaths...)
}

func (a *Adapter) projectRootFor(inputRoot string) string {
	if inputRoot != "" {
		return inputRoot
	}
	if a.projectRoot != "" {
		return a.projectRoot
	}
	return "."
}

func (a *Adapter) cursorDir(inputRoot string) string {
	return filepath.Join(a.projectRootFor(inputRoot), cursorDirName)
}

func cursorVersion(ctx context.Context, path string) string {
	if ctx == nil {
		ctx = context.Background()
	}
	cmd := exec.CommandContext(ctx, path, "--version")
	output, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

type renderContext struct {
	input     adapter.RenderInput
	agentName string
	mcpPath   string
	rulePath  string
}

func (r renderContext) renderRuleFile() string {
	var b strings.Builder
	writeLine(&b, "# AVM Agent: %s", r.agentName)
	b.WriteByte('\n')
	writeLine(&b, "<!-- Managed by Agent VM. Cursor support is partial in Phase 1 and only renders safe rules/instructions. -->")

	if r.input.Agent.Description != "" {
		b.WriteByte('\n')
		writeLine(&b, "## Description")
		b.WriteString(strings.TrimSpace(r.input.Agent.Description))
		b.WriteString("\n")
	}
	if r.input.Agent.Instructions.System != "" {
		b.WriteByte('\n')
		writeLine(&b, "## System Instructions")
		b.WriteString(strings.TrimSpace(r.input.Agent.Instructions.System))
		b.WriteString("\n")
	}
	if r.input.Agent.Instructions.Developer != "" {
		b.WriteByte('\n')
		writeLine(&b, "## Developer Instructions")
		b.WriteString(strings.TrimSpace(r.input.Agent.Instructions.Developer))
		b.WriteString("\n")
	}
	if len(r.input.Agent.Instructions.References) > 0 {
		b.WriteByte('\n')
		writeLine(&b, "## Instruction References")
		for _, ref := range sortedStrings(r.input.Agent.Instructions.References) {
			writeLine(&b, "- %s", ref)
		}
	}

	return strings.TrimRight(b.String(), "\n") + "\n"
}

func (r renderContext) renderMCPSection() []byte {
	payload := cursorMCPFile{
		MCPServers: make(map[string]cursorMCPServer),
	}
	for _, server := range r.renderableMCPServers() {
		payload.MCPServers[server.Name] = cursorMCPServerFrom(server)
	}
	data, err := marshalJSON(payload)
	if err != nil {
		return nil
	}
	return data
}

func (r renderContext) renderableMCPServers() []adapter.MCPServer {
	var servers []adapter.MCPServer
	for _, server := range sortedMCPServers(r.input.Capabilities.MCPServers) {
		if mcpServerRenderable(server) {
			servers = append(servers, server)
		}
	}
	return servers
}

func (r renderContext) mappings() []adapter.FieldMapping {
	ruleTarget := r.rulePath + "#instructions"
	mappings := []adapter.FieldMapping{
		{
			SourcePath: "active",
			Status:     adapter.MappingIgnored,
			Reason:     "Cursor Phase 1 PoC has no active profile selector; AVM only renders project files.",
		},
		{
			SourcePath: "agent.profile",
			Status:     adapter.MappingUnsupported,
			Reason:     "Cursor Phase 1 has no stable local Agent Profile format.",
		},
		{
			SourcePath: "agent.name",
			TargetPath: r.rulePath + "#agent-name",
			Status:     adapter.MappingRenderedAsInstructions,
			Reason:     "Cursor Phase 1 identifies the AVM agent through the AVM-owned rules file.",
		},
		{
			SourcePath: "agent.instructions.system",
			TargetPath: ruleTarget,
			Status:     adapter.MappingRenderedAsInstructions,
			Reason:     "Cursor Phase 1 renders AVM instructions as project rules.",
		},
		{
			SourcePath: "agent.instructions.developer",
			TargetPath: ruleTarget,
			Status:     adapter.MappingRenderedAsInstructions,
			Reason:     "Cursor Phase 1 renders AVM instructions as project rules.",
		},
		{
			SourcePath: "project..cursorrules",
			Status:     adapter.MappingIgnored,
			Reason:     "Cursor user and team rules are user-owned; the Cursor adapter writes only AVM-owned rule files.",
		},
	}

	if r.input.Agent.Description != "" {
		mappings = append(mappings, adapter.FieldMapping{
			SourcePath: "agent.description",
			TargetPath: r.rulePath + "#description",
			Status:     adapter.MappingRenderedAsInstructions,
			Reason:     "Cursor Phase 1 has no native AVM agent description field.",
		})
	}
	if len(r.input.Agent.Instructions.References) > 0 {
		mappings = append(mappings, adapter.FieldMapping{
			SourcePath: "agent.instructions.references",
			TargetPath: ruleTarget,
			Status:     adapter.MappingRenderedAsInstructions,
			Reason:     "Cursor Phase 1 preserves instruction references as rules text.",
		})
	}

	mappings = append(mappings, r.modelMappings()...)
	mappings = append(mappings, r.permissionMappings()...)
	mappings = append(mappings, r.memoryMappings()...)
	mappings = append(mappings, r.capabilityMappings()...)
	return mappings
}

func (r renderContext) modelMappings() []adapter.FieldMapping {
	var mappings []adapter.FieldMapping
	if r.input.Agent.Model.Model != "" {
		mappings = append(mappings, unsupported("agent.model.model", "Cursor Phase 1 PoC does not reliably set the selected model."))
	}
	if r.input.Agent.Model.ReasoningEffort != "" {
		mappings = append(mappings, unsupported("agent.model.reasoning_effort", "Cursor Phase 1 PoC does not reliably set reasoning effort."))
	}
	if r.input.Agent.Model.Verbosity != "" {
		mappings = append(mappings, unsupported("agent.model.verbosity", "Cursor Phase 1 PoC does not reliably set response verbosity."))
	}
	if r.input.Agent.Model.Temperature != nil {
		mappings = append(mappings, unsupported("agent.model.temperature", "Cursor Phase 1 PoC does not support temperature."))
	}
	return mappings
}

func (r renderContext) permissionMappings() []adapter.FieldMapping {
	var mappings []adapter.FieldMapping
	if r.input.Agent.Permissions.Approval != "" {
		mappings = append(mappings, unsupported("agent.permissions.approval", "Cursor Phase 1 PoC cannot enforce AVM approval policy."))
	}
	if r.input.Agent.Permissions.Sandbox != "" {
		mappings = append(mappings, unsupported("agent.permissions.sandbox", "Cursor Phase 1 PoC cannot enforce AVM sandbox mode."))
	}
	if len(r.input.Agent.Permissions.Allow) > 0 {
		mappings = append(mappings, unsupported("agent.permissions.allow", "Cursor Phase 1 PoC cannot enforce command allow patterns."))
	}
	if len(r.input.Agent.Permissions.Deny) > 0 {
		mappings = append(mappings, unsupported("agent.permissions.deny", "Cursor Phase 1 PoC cannot enforce command deny patterns."))
	}
	if len(r.input.Agent.Permissions.AdditionalDirectories) > 0 {
		mappings = append(mappings, unsupported("agent.permissions.additional_directories", "Cursor Phase 1 PoC cannot grant additional writable directories."))
	}
	return mappings
}

func (r renderContext) memoryMappings() []adapter.FieldMapping {
	var mappings []adapter.FieldMapping
	if len(r.input.Agent.MemoryRefs) > 0 {
		mappings = append(mappings, unsupported("agent.memory_refs", "Cursor Phase 1 PoC does not provide reliable portable memory scoping."))
	}
	if len(r.input.Memory) > 0 {
		mappings = append(mappings, unsupported("memory", "Cursor Phase 1 PoC does not write native memory."))
	}
	return mappings
}

func (r renderContext) capabilityMappings() []adapter.FieldMapping {
	var mappings []adapter.FieldMapping
	if len(r.input.Capabilities.Skills) > 0 {
		mappings = append(mappings, unsupported("capabilities.skills", "Cursor Phase 1 PoC does not install or mount AVM skills."))
	}
	if len(r.input.Capabilities.Commands) > 0 {
		mappings = append(mappings, unsupported("capabilities.commands", "Cursor Phase 1 PoC does not install AVM commands."))
	}
	if len(r.input.Capabilities.Hooks) > 0 {
		mappings = append(mappings, unsupported("capabilities.hooks", "Cursor Phase 1 PoC does not install AVM hooks."))
	}
	if len(r.input.Capabilities.Toolsets) > 0 {
		mappings = append(mappings, unsupported("capabilities.toolsets", "Cursor Phase 1 PoC does not enforce AVM toolset modes."))
	}

	for _, server := range sortedMCPServers(r.input.Capabilities.MCPServers) {
		source := "capabilities.mcp_servers"
		if server.Name != "" {
			source += "." + server.Name
		}
		if mcpServerRenderable(server) {
			mappings = append(mappings, adapter.FieldMapping{
				SourcePath: source,
				TargetPath: r.mcpPath + "#mcpServers." + server.Name,
				Status:     adapter.MappingNative,
			})
			continue
		}
		mappings = append(mappings, unsupported(source, "Cursor MCP rendering requires a server name and command or URL."))
	}

	return mappings
}

func (r renderContext) warnings() []string {
	warnings := partialWarnings()

	if hasModelConfig(r.input.Agent.Model) {
		warnings = append(warnings, "cursor partial support does not render model settings; agent.model.* mappings are unsupported")
	}
	if hasPermissionConfig(r.input.Agent.Permissions) {
		warnings = append(warnings, "cursor partial support does not enforce permissions; agent.permissions.* mappings are unsupported")
	}
	if len(r.input.Agent.MemoryRefs) > 0 || len(r.input.Memory) > 0 {
		warnings = append(warnings, "cursor partial support does not render portable memory; memory mappings are unsupported")
	}
	if hasNonMCPCapabilities(r.input.Capabilities) {
		warnings = append(warnings, "cursor partial support renders MCP only; non-MCP capability mappings are unsupported")
	}
	for _, server := range sortedMCPServers(r.input.Capabilities.MCPServers) {
		if !mcpServerRenderable(server) {
			name := firstNonEmpty(server.Name, "<unnamed>")
			warnings = append(warnings, fmt.Sprintf("mcp server %q was not rendered because name and command or URL are required", name))
		}
	}

	return warnings
}

func unsupported(sourcePath, reason string) adapter.FieldMapping {
	return adapter.FieldMapping{
		SourcePath: sourcePath,
		Status:     adapter.MappingUnsupported,
		Reason:     reason,
	}
}

func partialWarnings() []string {
	return []string{
		"cursor adapter is partial in Phase 1; only AVM-owned Cursor rules and MCP server entries are rendered",
	}
}

func applyOperation(operation adapter.RenderOperation, managed adapter.ManagedPath) (bool, error) {
	switch operation.Action {
	case adapter.OperationWriteFile:
		if managed.MergeMode != adapter.MergeModeWholeFile {
			return false, fmt.Errorf("cursor write operation %q requires whole-file managed path %s", operation.ID, operation.Path)
		}
		return writeFileAtomic(operation.Path, operation.Content)
	case adapter.OperationStructuredSet:
		if managed.MergeMode != adapter.MergeModeStructuredSection {
			return false, fmt.Errorf("cursor structured operation %q requires structured-section managed path %s", operation.ID, operation.Path)
		}
		return mergeMCPServers(operation.Path, operation.Content)
	default:
		return false, fmt.Errorf("cursor adapter cannot apply %s operation %q at %s", operation.Action, operation.ID, operation.Path)
	}
}

func writeFileAtomic(path string, content []byte) (bool, error) {
	existing, err := os.ReadFile(path)
	if err == nil && bytes.Equal(existing, content) {
		return false, nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return false, err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".avm-*.tmp")
	if err != nil {
		return false, err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)

	if _, err := temp.Write(content); err != nil {
		temp.Close()
		return false, err
	}
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return false, err
	}
	if err := temp.Close(); err != nil {
		return false, err
	}
	if err := os.Rename(tempName, path); err != nil {
		return false, err
	}
	return true, nil
}

func mergeMCPServers(path string, content []byte) (bool, error) {
	var payload cursorMCPFile
	if err := json.Unmarshal(content, &payload); err != nil {
		return false, fmt.Errorf("decode Cursor MCP payload for %s: %w", path, err)
	}

	existing := map[string]json.RawMessage{}
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if err == nil && len(bytes.TrimSpace(data)) > 0 {
		if err := json.Unmarshal(data, &existing); err != nil {
			return false, fmt.Errorf("decode existing Cursor MCP file %s: %w", path, err)
		}
	}

	servers := map[string]json.RawMessage{}
	if raw, ok := existing["mcpServers"]; ok && len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &servers); err != nil {
			return false, fmt.Errorf("decode existing Cursor MCP servers in %s: %w", path, err)
		}
	}

	for name, server := range payload.MCPServers {
		if name == "" {
			continue
		}
		raw, err := marshalJSON(server)
		if err != nil {
			return false, err
		}
		servers[name] = bytes.TrimSpace(raw)
	}

	rawServers, err := marshalJSON(servers)
	if err != nil {
		return false, err
	}
	existing["mcpServers"] = bytes.TrimSpace(rawServers)

	next, err := marshalJSON(existing)
	if err != nil {
		return false, err
	}
	return writeFileAtomic(path, next)
}

type cursorMCPFile struct {
	MCPServers map[string]cursorMCPServer `json:"mcpServers,omitempty"`
}

type cursorMCPServer struct {
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	URL     string            `json:"url,omitempty"`
}

func cursorMCPServerFrom(server adapter.MCPServer) cursorMCPServer {
	out := cursorMCPServer{
		Command: server.Command,
		Args:    append([]string(nil), server.Args...),
		URL:     server.URL,
	}
	if len(server.Env) > 0 {
		out.Env = make(map[string]string, len(server.Env))
		for _, env := range server.Env {
			if env.Name == "" {
				continue
			}
			out.Env[env.Name] = env.Value
		}
	}
	return out
}

func marshalJSON(value any) ([]byte, error) {
	var b bytes.Buffer
	encoder := json.NewEncoder(&b)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

func managedPathIndex(paths []adapter.ManagedPath) map[string]adapter.ManagedPath {
	managed := make(map[string]adapter.ManagedPath, len(paths))
	for _, path := range paths {
		managed[path.Path] = path
	}
	return managed
}

func writeLine(builder *strings.Builder, format string, args ...any) {
	builder.WriteString(fmt.Sprintf(format, args...))
	builder.WriteByte('\n')
}

func sortedStrings(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}

func sortedMCPServers(servers []adapter.MCPServer) []adapter.MCPServer {
	out := append([]adapter.MCPServer(nil), servers...)
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

func mcpServerRenderable(server adapter.MCPServer) bool {
	return server.Name != "" && (server.Command != "" || server.URL != "")
}

func hasModelConfig(model adapter.ModelConfig) bool {
	return model.Model != "" || model.ReasoningEffort != "" || model.Verbosity != "" || model.Temperature != nil
}

func hasPermissionConfig(permissions adapter.PermissionConfig) bool {
	return permissions.Approval != "" ||
		permissions.Sandbox != "" ||
		len(permissions.Allow) > 0 ||
		len(permissions.Deny) > 0 ||
		len(permissions.AdditionalDirectories) > 0
}

func hasNonMCPCapabilities(capabilities adapter.CapabilitySet) bool {
	return len(capabilities.Skills) > 0 ||
		len(capabilities.Commands) > 0 ||
		len(capabilities.Hooks) > 0 ||
		len(capabilities.Toolsets) > 0
}

func slug(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	var builder strings.Builder
	lastDash := false
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
			lastDash = false
			continue
		}
		if r == '-' || r == '_' || unicode.IsSpace(r) {
			if !lastDash && builder.Len() > 0 {
				builder.WriteByte('-')
				lastDash = true
			}
		}
	}
	slugged := strings.Trim(builder.String(), "-")
	if slugged == "" {
		return "agent"
	}
	return slugged
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
