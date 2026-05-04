// Package cline renders AVM agents into Cline rules and MCP settings.
package cline

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

	"github.com/xz1220/agent-vm/internal/adapter"
	"github.com/xz1220/agent-vm/internal/adapter/renderplan"
	"github.com/xz1220/agent-vm/internal/adapter/shared"
)

const (
	runtimeName         = "cline"
	settingsDirName     = "settings"
	mcpSettingsFileName = "cline_mcp_settings.json"
	rulesOperationID    = "cline-agent-rules"
	mcpOperationID      = "cline-mcp-settings"
	avmMetadataKey      = "_avm"
)

// Adapter renders the conservative Phase 1 Cline path.
type Adapter struct {
	dataDir     string
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

func WithDataDir(dataDir string) Option {
	return func(a *Adapter) {
		a.dataDir = dataDir
	}
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
	_ = ctx
	dataDir := a.clineDataHome()
	settingsPath := filepath.Join(dataDir, settingsDirName, mcpSettingsFileName)

	found := pathExists(dataDir) || pathExists(settingsPath)
	version := ""
	if _, err := exec.LookPath(runtimeName); err == nil {
		found = true
	}

	return adapter.Detection{
		Runtime:   runtimeName,
		Found:     found,
		Version:   version,
		ConfigDir: filepath.ToSlash(dataDir),
	}
}

func (a *Adapter) Plan(ctx adapter.Context, input adapter.RenderInput) (*adapter.RenderPlan, error) {
	_ = ctx

	runtime := input.Runtime
	if runtime == "" {
		runtime = runtimeName
	}
	if runtime != runtimeName {
		return nil, fmt.Errorf("cline adapter cannot plan runtime %q", runtime)
	}

	agentName := shared.FirstNonEmpty(input.Agent.Name, input.Active.Name, "agent")
	agentSlug := shared.Slug(agentName)
	projectRoot := shared.FirstNonEmpty(input.ProjectRoot, a.projectRoot, ".")
	rulesPath := filepath.ToSlash(filepath.Join(projectRoot, ".clinerules", "avm", agentSlug+".md"))
	mcpSettingsPath := filepath.ToSlash(filepath.Join(a.clineDataHome(), settingsDirName, mcpSettingsFileName))

	render := renderContext{
		input:           input,
		agentName:       agentName,
		rulesPath:       rulesPath,
		mcpSettingsPath: mcpSettingsPath,
	}

	managedPaths := []adapter.ManagedPath{
		{
			Path:        rulesPath,
			Owner:       "avm",
			Description: "Cline rules rendered from the AVM agent profile.",
			Required:    true,
			MergeMode:   adapter.MergeModeWholeFile,
		},
	}
	operations := []adapter.RenderOperation{
		{
			ID:          rulesOperationID,
			Action:      adapter.OperationWriteFile,
			Path:        rulesPath,
			Content:     []byte(render.renderRulesFile()),
			Description: "write Cline AVM-managed rules file",
			Required:    true,
		},
	}

	if render.hasRenderableMCPServers() {
		managedPaths = append(managedPaths, adapter.ManagedPath{
			Path:        mcpSettingsPath,
			Owner:       "shared-section",
			Description: "Cline MCP server entries managed by AVM metadata without overwriting user-owned servers.",
			Required:    true,
			MergeMode:   adapter.MergeModeStructuredSection,
		})
		operations = append(operations, adapter.RenderOperation{
			ID:          mcpOperationID,
			Action:      adapter.OperationStructuredSet,
			Path:        mcpSettingsPath,
			Content:     []byte(render.renderMCPSettingsPatch()),
			Description: "merge Cline AVM-managed MCP server entries",
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
		return nil, fmt.Errorf("cline adapter render plan is nil")
	}
	if normalized.Runtime != "" && normalized.Runtime != runtimeName {
		return nil, fmt.Errorf("cline adapter cannot render runtime %q", normalized.Runtime)
	}

	managed := shared.ManagedPathIndex(normalized.ManagedPaths)
	results := make([]adapter.RenderOperationResult, 0, len(normalized.Operations))
	warnings := append([]string(nil), normalized.Warnings...)
	for _, operation := range normalized.Operations {
		managedPath, ok := managed[operation.Path]
		if !ok {
			return nil, fmt.Errorf("cline render operation %q targets unmanaged path %s", operation.ID, operation.Path)
		}

		changed, operationWarnings, err := applyOperation(operation, managedPath)
		if err != nil {
			return nil, err
		}
		warnings = append(warnings, operationWarnings...)
		results = append(results, adapter.RenderOperationResult{
			OperationID: operation.ID,
			Action:      operation.Action,
			Path:        operation.Path,
			Changed:     changed,
		})
	}

	sort.Strings(warnings)
	return &adapter.RenderResult{
		Runtime:      runtimeName,
		Operations:   results,
		ManagedPaths: append([]adapter.ManagedPath(nil), normalized.ManagedPaths...),
		Mappings:     append([]adapter.FieldMapping(nil), normalized.Mappings...),
		Warnings:     warnings,
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

func (a *Adapter) clineDataHome() string {
	if a.dataDir != "" {
		return a.dataDir
	}
	if value := os.Getenv("CLINE_DATA_HOME"); value != "" {
		return value
	}
	if value := os.Getenv("CLINE_DIR"); value != "" {
		return filepath.Join(value, "data")
	}

	defaultHome := filepath.Join(userHomeOrDot(), ".cline", "data")
	if pathExists(defaultHome) || pathExists(filepath.Join(defaultHome, settingsDirName, mcpSettingsFileName)) {
		return defaultHome
	}
	for _, candidate := range clineExtensionDataHomes() {
		if pathExists(candidate) || pathExists(filepath.Join(candidate, settingsDirName, mcpSettingsFileName)) {
			return candidate
		}
	}
	return defaultHome
}

func clineVersion(ctx context.Context, path string) string {
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

func clineExtensionDataHomes() []string {
	home := userHomeOrDot()
	appData := os.Getenv("APPDATA")
	bases := []string{
		filepath.Join(home, "Library", "Application Support", "Code", "User", "globalStorage"),
		filepath.Join(home, "Library", "Application Support", "Cursor", "User", "globalStorage"),
		filepath.Join(home, "Library", "Application Support", "VSCodium", "User", "globalStorage"),
		filepath.Join(home, ".config", "Code", "User", "globalStorage"),
		filepath.Join(home, ".config", "Cursor", "User", "globalStorage"),
		filepath.Join(home, ".config", "VSCodium", "User", "globalStorage"),
	}
	if appData != "" {
		bases = append(bases,
			filepath.Join(appData, "Code", "User", "globalStorage"),
			filepath.Join(appData, "Cursor", "User", "globalStorage"),
		)
	}

	candidates := make([]string, 0, len(bases))
	for _, base := range bases {
		candidates = append(candidates, filepath.Join(base, "saoudrizwan.claude-dev"))
	}
	return candidates
}

func userHomeOrDot() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "."
	}
	return home
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

type renderContext struct {
	input           adapter.RenderInput
	agentName       string
	rulesPath       string
	mcpSettingsPath string
}

func (r renderContext) hasRenderableMCPServers() bool {
	for _, server := range r.input.Capabilities.MCPServers {
		if shared.MCPServerRenderable(server) {
			return true
		}
	}
	return false
}

func (r renderContext) renderRulesFile() string {
	var sections []string
	sections = append(sections, "# AVM Agent: "+r.agentName)

	var details []string
	details = append(details, "Runtime: cline")
	if r.input.Active.Kind != "" || r.input.Active.Name != "" {
		details = append(details, "Active: "+strings.Trim(shared.FirstNonEmpty(r.input.Active.Kind, "active")+"/"+shared.FirstNonEmpty(r.input.Active.Name, "default"), "/"))
	}
	if r.input.Agent.Description != "" {
		details = append(details, "Description: "+r.input.Agent.Description)
	}
	sections = append(sections, bulletSection("AVM profile", details))

	if r.input.Agent.Instructions.System != "" {
		sections = append(sections, section("System instructions", r.input.Agent.Instructions.System))
	}
	if r.input.Agent.Instructions.Developer != "" {
		sections = append(sections, section("Developer instructions", r.input.Agent.Instructions.Developer))
	}
	if len(r.input.Agent.Instructions.References) > 0 {
		sections = append(sections, bulletSection("Instruction references", shared.SortedStrings(r.input.Agent.Instructions.References)))
	}
	if modelLines := r.modelPreferenceLines(); len(modelLines) > 0 {
		sections = append(sections, bulletSection("Requested model preferences", modelLines))
	}
	if permissionLines := r.permissionGuidanceLines(); len(permissionLines) > 0 {
		sections = append(sections, bulletSection("Permission guidance", permissionLines))
	}
	if len(r.input.Capabilities.Skills) > 0 {
		sections = append(sections, bulletSection("Active AVM skills", shared.CapabilityLines(r.input.Capabilities.Skills)))
	}
	if len(r.input.Capabilities.Commands) > 0 {
		sections = append(sections, bulletSection("Requested AVM commands", shared.CapabilityLines(r.input.Capabilities.Commands)))
	}
	if len(r.input.Capabilities.Hooks) > 0 {
		sections = append(sections, bulletSection("Requested AVM hooks", shared.CapabilityLines(r.input.Capabilities.Hooks)))
	}
	if len(r.input.Capabilities.Toolsets) > 0 {
		sections = append(sections, bulletSection("Requested toolsets", shared.ToolsetLines(r.input.Capabilities.Toolsets)))
	}
	if mcpLines := r.mcpInstructionLines(); len(mcpLines) > 0 {
		sections = append(sections, bulletSection("MCP servers configured by AVM", mcpLines))
	}

	return strings.Join(sections, "\n\n") + "\n"
}

func (r renderContext) modelPreferenceLines() []string {
	var lines []string
	if r.input.Agent.Model.Model != "" {
		lines = append(lines, "model="+r.input.Agent.Model.Model)
	}
	if r.input.Agent.Model.ReasoningEffort != "" {
		lines = append(lines, "reasoning_effort="+r.input.Agent.Model.ReasoningEffort)
	}
	if r.input.Agent.Model.Verbosity != "" {
		lines = append(lines, "verbosity="+r.input.Agent.Model.Verbosity)
	}
	sort.Strings(lines)
	return lines
}

func (r renderContext) permissionGuidanceLines() []string {
	var lines []string
	if r.input.Agent.Permissions.Approval != "" {
		lines = append(lines, "approval="+r.input.Agent.Permissions.Approval)
	}
	if r.input.Agent.Permissions.Sandbox != "" {
		lines = append(lines, "sandbox="+r.input.Agent.Permissions.Sandbox)
	}
	for _, value := range shared.SortedStrings(r.input.Agent.Permissions.Allow) {
		lines = append(lines, "allow="+value)
	}
	for _, value := range shared.SortedStrings(r.input.Agent.Permissions.Deny) {
		lines = append(lines, "deny="+value)
	}
	sort.Strings(lines)
	return lines
}

func (r renderContext) mcpInstructionLines() []string {
	var lines []string
	for _, server := range shared.SortedMCPServers(r.input.Capabilities.MCPServers) {
		if !shared.MCPServerRenderable(server) {
			continue
		}
		lines = append(lines, server.Name)
	}
	return lines
}

func (r renderContext) renderMCPSettingsPatch() string {
	servers := make(map[string]clineMCPServer)
	for _, server := range shared.SortedMCPServers(r.input.Capabilities.MCPServers) {
		if !shared.MCPServerRenderable(server) {
			continue
		}
		servers[server.Name] = clineMCPServer{
			Command:     server.Command,
			Args:        append([]string(nil), server.Args...),
			Env:         envMap(server.Env),
			URL:         server.URL,
			AlwaysAllow: []string{},
			Disabled:    false,
		}
	}

	data, err := json.MarshalIndent(mcpSettingsPatch{MCPServers: servers}, "", "  ")
	if err != nil {
		return "{}\n"
	}
	return string(data) + "\n"
}

func (r renderContext) mappings() []adapter.FieldMapping {
	targetRules := r.rulesPath
	mappings := []adapter.FieldMapping{
		{
			SourcePath: "active",
			TargetPath: targetRules,
			Status:     adapter.MappingRenderedAsInstructions,
			Reason:     "Cline has no stable local active Agent Profile switch in Phase 1.",
		},
		{
			SourcePath: "agent.name",
			TargetPath: targetRules,
			Status:     adapter.MappingRenderedAsInstructions,
			Reason:     "Cline has no stable local Agent Profile file in Phase 1.",
		},
		{
			SourcePath: "agent.description",
			TargetPath: targetRules,
			Status:     adapter.MappingRenderedAsInstructions,
			Reason:     "Cline has no stable local Agent Profile description field in Phase 1.",
		},
		{
			SourcePath: "agent.instructions.system",
			TargetPath: targetRules,
			Status:     adapter.MappingRenderedAsInstructions,
			Reason:     "Cline consumes workspace rules as instruction text.",
		},
		{
			SourcePath: "agent.instructions.developer",
			TargetPath: targetRules,
			Status:     adapter.MappingRenderedAsInstructions,
			Reason:     "Cline consumes workspace rules as instruction text.",
		},
		{
			SourcePath: "project.clinerules",
			Status:     adapter.MappingIgnored,
			Reason:     "Existing user Cline rules are preserved; AVM writes only .clinerules/avm/<agent>.md.",
		},
		{
			SourcePath: "project.AGENTS.md",
			Status:     adapter.MappingIgnored,
			Reason:     "Cline may read AGENTS.md, but the Cline adapter does not overwrite user-owned project instructions.",
		},
	}

	if len(r.input.Agent.Instructions.References) > 0 {
		mappings = append(mappings, adapter.FieldMapping{
			SourcePath: "agent.instructions.references",
			TargetPath: targetRules,
			Status:     adapter.MappingRenderedAsInstructions,
			Reason:     "Cline rules do not have a separate references field.",
		})
	}
	if r.input.Agent.Model.Model != "" {
		mappings = append(mappings, adapter.FieldMapping{
			SourcePath: "agent.model.model",
			TargetPath: targetRules,
			Status:     adapter.MappingRenderedAsInstructions,
			Reason:     "Cline Phase 1 does not expose a stable per-agent model setting.",
		})
	}
	if r.input.Agent.Model.ReasoningEffort != "" {
		mappings = append(mappings, adapter.FieldMapping{
			SourcePath: "agent.model.reasoning_effort",
			TargetPath: targetRules,
			Status:     adapter.MappingRenderedAsInstructions,
			Reason:     "Cline Phase 1 does not expose a stable per-agent reasoning setting.",
		})
	}
	if r.input.Agent.Model.Verbosity != "" {
		mappings = append(mappings, adapter.FieldMapping{
			SourcePath: "agent.model.verbosity",
			TargetPath: targetRules,
			Status:     adapter.MappingRenderedAsInstructions,
			Reason:     "Cline Phase 1 does not expose a stable AVM verbosity field.",
		})
	}
	if r.input.Agent.Model.Temperature != nil {
		mappings = append(mappings, adapter.FieldMapping{
			SourcePath: "agent.model.temperature",
			Status:     adapter.MappingUnsupported,
			Reason:     "Cline adapter Phase 1 does not support temperature.",
		})
	}
	if r.input.Agent.Permissions.Approval != "" {
		mappings = append(mappings, adapter.FieldMapping{
			SourcePath: "agent.permissions.approval",
			TargetPath: targetRules,
			Status:     adapter.MappingRenderedAsInstructions,
			Reason:     "Cline auto-approval settings are not modified by the Phase 1 adapter.",
		})
	}
	if r.input.Agent.Permissions.Sandbox != "" {
		mappings = append(mappings, adapter.FieldMapping{
			SourcePath: "agent.permissions.sandbox",
			TargetPath: targetRules,
			Status:     adapter.MappingRenderedAsInstructions,
			Reason:     "Cline has no stable AVM sandbox equivalent in Phase 1.",
		})
	}
	if len(r.input.Agent.Permissions.Allow) > 0 {
		mappings = append(mappings, adapter.FieldMapping{
			SourcePath: "agent.permissions.allow",
			TargetPath: targetRules,
			Status:     adapter.MappingRenderedAsInstructions,
			Reason:     "Cline Phase 1 preserves AVM allow patterns as instruction guidance only.",
		})
	}
	if len(r.input.Agent.Permissions.Deny) > 0 {
		mappings = append(mappings, adapter.FieldMapping{
			SourcePath: "agent.permissions.deny",
			TargetPath: targetRules,
			Status:     adapter.MappingRenderedAsInstructions,
			Reason:     "Cline Phase 1 preserves AVM deny patterns as instruction guidance only.",
		})
	}
	if len(r.input.Agent.Permissions.AdditionalDirectories) > 0 {
		mappings = append(mappings, adapter.FieldMapping{
			SourcePath: "agent.permissions.additional_directories",
			Status:     adapter.MappingUnsupported,
			Reason:     "Cline adapter Phase 1 does not grant additional writable directories.",
		})
	}
	if len(r.input.Capabilities.Skills) > 0 {
		mappings = append(mappings, adapter.FieldMapping{
			SourcePath: "capabilities.skills",
			TargetPath: targetRules,
			Status:     adapter.MappingRenderedAsInstructions,
			Reason:     "Cline skills are not installed by the Phase 1 adapter.",
		})
	}
	if len(r.input.Capabilities.Commands) > 0 {
		mappings = append(mappings, adapter.FieldMapping{
			SourcePath: "capabilities.commands",
			TargetPath: targetRules,
			Status:     adapter.MappingRenderedAsInstructions,
			Reason:     "Cline adapter Phase 1 preserves AVM command capability names as instructions only.",
		})
	}
	if len(r.input.Capabilities.Hooks) > 0 {
		mappings = append(mappings, adapter.FieldMapping{
			SourcePath: "capabilities.hooks",
			TargetPath: targetRules,
			Status:     adapter.MappingRenderedAsInstructions,
			Reason:     "Cline adapter Phase 1 does not install AVM hooks.",
		})
	}
	if len(r.input.Capabilities.Toolsets) > 0 {
		mappings = append(mappings, adapter.FieldMapping{
			SourcePath: "capabilities.toolsets",
			TargetPath: targetRules,
			Status:     adapter.MappingRenderedAsInstructions,
			Reason:     "Cline adapter Phase 1 does not enforce AVM toolset modes natively.",
		})
	}
	for _, server := range shared.SortedMCPServers(r.input.Capabilities.MCPServers) {
		source := "capabilities.mcp_servers." + server.Name
		if shared.MCPServerRenderable(server) {
			mappings = append(mappings, adapter.FieldMapping{
				SourcePath: source,
				TargetPath: r.mcpSettingsPath + "#mcpServers." + server.Name,
				Status:     adapter.MappingNative,
			})
			continue
		}
		mappings = append(mappings, adapter.FieldMapping{
			SourcePath: source,
			Status:     adapter.MappingUnsupported,
			Reason:     "Cline MCP rendering requires a server name and command or URL.",
		})
	}

	return mappings
}

func (r renderContext) warnings() []string {
	var warnings []string
	for _, server := range shared.SortedMCPServers(r.input.Capabilities.MCPServers) {
		if !shared.MCPServerRenderable(server) {
			warnings = append(warnings, fmt.Sprintf("mcp server %q was not rendered because name and command or URL are required", server.Name))
		}
	}
	return warnings
}

type clineMCPServer struct {
	Command     string            `json:"command,omitempty"`
	Args        []string          `json:"args,omitempty"`
	Env         map[string]string `json:"env,omitempty"`
	URL         string            `json:"url,omitempty"`
	AlwaysAllow []string          `json:"alwaysAllow"`
	Disabled    bool              `json:"disabled"`
}

type mcpSettingsPatch struct {
	MCPServers map[string]clineMCPServer `json:"mcpServers"`
}

type rawMCPSettingsPatch struct {
	MCPServers map[string]json.RawMessage `json:"mcpServers"`
}

type avmSettingsMetadata struct {
	ManagedMCPServers []string `json:"managed_mcp_servers,omitempty"`
}

func applyOperation(operation adapter.RenderOperation, managed adapter.ManagedPath) (bool, []string, error) {
	switch operation.Action {
	case adapter.OperationWriteFile:
		if managed.MergeMode != adapter.MergeModeWholeFile {
			return false, nil, fmt.Errorf("cline write operation %q requires whole-file managed path %s", operation.ID, operation.Path)
		}
		changed, err := shared.WriteFileAtomic(operation.Path, operation.Content)
		return changed, nil, err
	case adapter.OperationStructuredSet:
		if managed.MergeMode != adapter.MergeModeStructuredSection {
			return false, nil, fmt.Errorf("cline structured operation %q requires structured-section managed path %s", operation.ID, operation.Path)
		}
		return mergeMCPSettings(operation.Path, operation.Content)
	default:
		return false, nil, fmt.Errorf("cline adapter cannot apply %s operation %q at %s", operation.Action, operation.ID, operation.Path)
	}
}

func mergeMCPSettings(path string, content []byte) (bool, []string, error) {
	var patch rawMCPSettingsPatch
	if err := json.Unmarshal(content, &patch); err != nil {
		return false, nil, fmt.Errorf("decode Cline MCP patch: %w", err)
	}
	if patch.MCPServers == nil {
		patch.MCPServers = map[string]json.RawMessage{}
	}

	document, err := readJSONDocument(path)
	if err != nil {
		return false, nil, err
	}
	servers, err := readRawObject(document, "mcpServers")
	if err != nil {
		return false, nil, err
	}
	metadata, err := readAVMMetadata(document)
	if err != nil {
		return false, nil, err
	}

	managed := stringSet(metadata.ManagedMCPServers)
	originalManaged := stringSet(metadata.ManagedMCPServers)
	desired := make(map[string]json.RawMessage, len(patch.MCPServers))
	for name, raw := range patch.MCPServers {
		if strings.TrimSpace(name) == "" {
			continue
		}
		compact, err := compactJSON(raw)
		if err != nil {
			return false, nil, fmt.Errorf("compact desired MCP server %q: %w", name, err)
		}
		desired[name] = compact
	}

	mutated := false
	for name := range managed {
		if _, stillDesired := desired[name]; !stillDesired {
			if _, exists := servers[name]; exists {
				delete(servers, name)
				mutated = true
			}
			delete(managed, name)
			if originalManaged[name] {
				mutated = true
			}
		}
	}

	var warnings []string
	for name, raw := range desired {
		if _, exists := servers[name]; exists && !managed[name] {
			warnings = append(warnings, fmt.Sprintf("cline mcp server %q already exists and is not AVM-managed; left unchanged", name))
			continue
		}
		if existing, exists := servers[name]; !exists || !bytes.Equal(existing, raw) {
			mutated = true
		}
		if !managed[name] {
			mutated = true
		}
		servers[name] = raw
		managed[name] = true
	}
	if !mutated {
		return false, warnings, nil
	}

	managedNames := sortedSetValues(managed)
	if len(managedNames) == 0 {
		delete(document, avmMetadataKey)
	} else {
		metadata.ManagedMCPServers = managedNames
		metadataRaw, err := json.Marshal(metadata)
		if err != nil {
			return false, nil, err
		}
		document[avmMetadataKey] = metadataRaw
	}

	serversRaw, err := json.Marshal(canonicalRawObject(servers))
	if err != nil {
		return false, nil, err
	}
	document["mcpServers"] = serversRaw

	next, err := json.MarshalIndent(document, "", "  ")
	if err != nil {
		return false, nil, err
	}
	next = append(next, '\n')
	changed, err := shared.WriteFileAtomic(path, next)
	return changed, warnings, err
}

func readJSONDocument(path string) (map[string]json.RawMessage, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]json.RawMessage{}, nil
	}
	if err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return map[string]json.RawMessage{}, nil
	}

	document := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("decode Cline MCP settings %s: %w", path, err)
	}
	return document, nil
}

func readRawObject(document map[string]json.RawMessage, key string) (map[string]json.RawMessage, error) {
	raw, ok := document[key]
	if !ok || len(raw) == 0 || string(raw) == "null" {
		return map[string]json.RawMessage{}, nil
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, fmt.Errorf("Cline MCP settings field %q must be an object: %w", key, err)
	}
	if object == nil {
		object = map[string]json.RawMessage{}
	}
	out := make(map[string]json.RawMessage, len(object))
	for name, value := range object {
		compact, err := compactJSON(value)
		if err != nil {
			return nil, fmt.Errorf("compact existing MCP server %q: %w", name, err)
		}
		out[name] = compact
	}
	return out, nil
}

func readAVMMetadata(document map[string]json.RawMessage) (avmSettingsMetadata, error) {
	raw, ok := document[avmMetadataKey]
	if !ok || len(raw) == 0 || string(raw) == "null" {
		return avmSettingsMetadata{}, nil
	}
	var metadata avmSettingsMetadata
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return avmSettingsMetadata{}, fmt.Errorf("decode Cline AVM metadata: %w", err)
	}
	metadata.ManagedMCPServers = sortedUnique(metadata.ManagedMCPServers)
	return metadata, nil
}

func canonicalRawObject(object map[string]json.RawMessage) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(object))
	for name, value := range object {
		compact, err := compactJSON(value)
		if err != nil {
			out[name] = value
			continue
		}
		out[name] = compact
	}
	return out
}

func compactJSON(raw json.RawMessage) (json.RawMessage, error) {
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return nil, err
	}
	return append(json.RawMessage(nil), buf.Bytes()...), nil
}

func section(title, body string) string {
	return "## " + title + "\n\n" + strings.TrimSpace(body)
}

func bulletSection(title string, lines []string) string {
	var b strings.Builder
	b.WriteString("## ")
	b.WriteString(title)
	b.WriteString("\n\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		b.WriteString("- ")
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

func envMap(env []adapter.EnvVar) map[string]string {
	if len(env) == 0 {
		return nil
	}
	out := make(map[string]string, len(env))
	for _, item := range env {
		if item.Name == "" {
			continue
		}
		out[item.Name] = item.Value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func stringSet(values []string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out[value] = true
		}
	}
	return out
}

func sortedSetValues(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for value := range set {
		if strings.TrimSpace(value) != "" {
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}

func sortedUnique(values []string) []string {
	return sortedSetValues(stringSet(values))
}
