// Package claude renders AVM agents into Claude Code project configuration.
package claude

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
	"strconv"
	"strings"

	"github.com/xz1220/agent-vm/internal/adapter"
	"github.com/xz1220/agent-vm/internal/adapter/renderplan"
	"github.com/xz1220/agent-vm/internal/adapter/shared"
)

const (
	runtimeName         = "claude-code"
	claudeBinaryName    = "claude"
	claudeDirName       = ".claude"
	agentsDirName       = "agents"
	skillsDirName       = "skills"
	skillFileName       = "SKILL.md"
	settingsFileName    = "settings.json"
	mcpFileName         = "mcp.json"
	agentOperationID    = "claude-agent"
	settingsOperationID = "claude-settings"
	mcpOperationID      = "claude-mcp"
	skillOperationID    = "claude-skill"
	avmManagedKey       = "avm_managed"
	avmMetadataKey      = "_avm"
	avmMetadataSubkey   = "claude-code"
)

// Adapter renders the conservative Phase 1 Claude Code path.
type Adapter struct {
	configDir   string
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

func WithConfigDir(configDir string) Option {
	return func(a *Adapter) {
		a.configDir = configDir
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
	configDir := a.claudeHome()

	found := false
	if _, err := os.Stat(configDir); err == nil {
		found = true
	}
	for _, name := range []string{"settings.json", agentsDirName, "skills"} {
		if _, err := os.Stat(filepath.Join(configDir, name)); err == nil {
			found = true
			break
		}
	}

	version := ""
	if _, err := exec.LookPath(claudeBinaryName); err == nil {
		found = true
	}

	return adapter.Detection{
		Runtime:   runtimeName,
		Found:     found,
		Version:   version,
		ConfigDir: filepath.ToSlash(configDir),
	}
}

func (a *Adapter) Import(ctx adapter.Context) (*adapter.ImportResult, error) {
	_ = ctx

	agents := []adapter.ImportedAgent{}
	warnings := []string{}
	for _, dir := range []string{
		filepath.Join(a.defaultProjectRoot(), claudeDirName, agentsDirName),
		filepath.Join(a.claudeHome(), agentsDirName),
	} {
		imported, err := importAgents(dir)
		if err != nil {
			warnings = append(warnings, err.Error())
			continue
		}
		agents = append(agents, imported...)
	}

	sort.SliceStable(agents, func(i, j int) bool {
		if agents[i].SourcePath != agents[j].SourcePath {
			return agents[i].SourcePath < agents[j].SourcePath
		}
		return agents[i].Name < agents[j].Name
	})

	return &adapter.ImportResult{
		Runtime:  runtimeName,
		Agents:   agents,
		Warnings: warnings,
	}, nil
}

func (a *Adapter) Plan(ctx adapter.Context, input adapter.RenderInput) (*adapter.RenderPlan, error) {
	_ = ctx

	runtime := input.Runtime
	if runtime == "" {
		runtime = runtimeName
	}
	if runtime != runtimeName {
		return nil, fmt.Errorf("claude-code adapter cannot plan runtime %q", runtime)
	}

	agentName := shared.FirstNonEmpty(input.Agent.Name, input.Active.Name, "agent")
	agentFileName := shared.Slug(agentName)
	configDir := a.claudeHomeForInput(input)
	agentPath := filepath.ToSlash(filepath.Join(configDir, agentsDirName, agentFileName+".md"))
	settingsPath := filepath.ToSlash(filepath.Join(configDir, settingsFileName))
	mcpPath := filepath.ToSlash(filepath.Join(configDir, mcpFileName))
	skillFiles, skillWarnings := claudeSkillFiles(input, configDir)
	staleSkillFiles := staleClaudeSkillFiles(configDir, skillFileNames(skillFiles))

	render := renderContext{
		input:         input,
		agentName:     agentName,
		agentFileName: agentFileName,
		agentPath:     agentPath,
		mcpPath:       mcpPath,
	}

	managedPaths := []adapter.ManagedPath{
		{
			Path:        agentPath,
			Owner:       "avm",
			Description: "Claude Code agent definition rendered into an isolated AVM-owned runtime home.",
			Required:    true,
			MergeMode:   adapter.MergeModeWholeFile,
		},
		{
			Path:        settingsPath,
			Owner:       "avm",
			Description: "Claude Code settings selecting the active AVM agent inside the isolated runtime home.",
			Required:    true,
			MergeMode:   adapter.MergeModeWholeFile,
		},
	}
	operations := []adapter.RenderOperation{
		{
			ID:          agentOperationID,
			Action:      adapter.OperationWriteFile,
			Path:        agentPath,
			Content:     []byte(render.renderAgentFile()),
			Description: "write Claude Code AVM-managed agent file",
			Required:    true,
		},
		{
			ID:          settingsOperationID,
			Action:      adapter.OperationWriteFile,
			Path:        settingsPath,
			Content:     []byte(render.renderSettingsFile()),
			Description: "write Claude Code AVM-managed settings file",
			Required:    true,
		},
	}
	for _, skillFile := range skillFiles {
		managedPaths = append(managedPaths, adapter.ManagedPath{
			Path:        skillFile.target,
			Owner:       "avm",
			Description: "Claude Code skill file rendered from the AVM active skill set.",
			Required:    true,
			MergeMode:   adapter.MergeModeWholeFile,
		})
		operations = append(operations, adapter.RenderOperation{
			ID:          skillOperationID + "-" + shared.Slug(skillFile.name),
			Action:      adapter.OperationWriteFile,
			Path:        skillFile.target,
			Content:     skillFile.content,
			Description: "write Claude Code skill file from AVM active skill",
			Required:    true,
		})
	}
	for _, stale := range staleSkillFiles {
		managedPaths = append(managedPaths, adapter.ManagedPath{
			Path:        stale.target,
			Owner:       "avm",
			Description: "Stale Claude Code AVM-managed skill file removed because it is not in the current active skill set.",
			Required:    false,
			MergeMode:   adapter.MergeModeWholeFile,
		})
		operations = append(operations, adapter.RenderOperation{
			ID:          skillOperationID + "-remove-" + shared.Slug(stale.name),
			Action:      adapter.OperationRemoveFile,
			Path:        stale.target,
			Description: "remove stale Claude Code AVM-managed skill file",
			Required:    false,
		})
	}

	if len(render.renderableMCPServers()) > 0 {
		managedPaths = append(managedPaths, adapter.ManagedPath{
			Path:        mcpPath,
			Owner:       "avm",
			Description: "Claude Code MCP config rendered into the isolated AVM-owned runtime home.",
			Required:    true,
			MergeMode:   adapter.MergeModeWholeFile,
		})
		operations = append(operations, adapter.RenderOperation{
			ID:          mcpOperationID,
			Action:      adapter.OperationWriteFile,
			Path:        mcpPath,
			Content:     []byte(render.renderMCPDocument()),
			Description: "write Claude Code AVM-managed MCP config file",
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
	plan.Warnings = append(plan.Warnings, skillWarnings...)
	return renderplan.Normalize(plan), nil
}

func (a *Adapter) Render(ctx adapter.Context, plan *adapter.RenderPlan) (*adapter.RenderResult, error) {
	_ = ctx

	normalized := renderplan.Normalize(plan)
	if normalized == nil {
		return nil, fmt.Errorf("claude-code adapter render plan is nil")
	}
	if normalized.Runtime != "" && normalized.Runtime != runtimeName {
		return nil, fmt.Errorf("claude-code adapter cannot render runtime %q", normalized.Runtime)
	}

	managed := shared.ManagedPathIndex(normalized.ManagedPaths)
	for _, operation := range normalized.Operations {
		if _, ok := managed[operation.Path]; !ok {
			return nil, fmt.Errorf("claude-code render operation %q targets unmanaged path %s", operation.ID, operation.Path)
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

func (a *Adapter) claudeHome() string {
	if a.configDir != "" {
		return a.configDir
	}
	if value := os.Getenv("CLAUDE_CONFIG_DIR"); value != "" {
		return value
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".claude")
	}
	return ".claude"
}

func (a *Adapter) claudeHomeForInput(input adapter.RenderInput) string {
	if input.RuntimeHome != "" {
		return input.RuntimeHome
	}
	return a.claudeHome()
}

func (a *Adapter) defaultProjectRoot() string {
	if a.projectRoot != "" {
		return a.projectRoot
	}
	if cwd, err := os.Getwd(); err == nil && cwd != "" {
		return cwd
	}
	return "."
}

func claudeVersion(ctx context.Context, path string) string {
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
	input         adapter.RenderInput
	agentName     string
	agentFileName string
	agentPath     string
	mcpPath       string
}

func (r renderContext) renderAgentFile() string {
	var b strings.Builder
	b.WriteString("---\n")
	writeYAMLString(&b, "name", r.agentFileName)
	writeYAMLString(&b, "description", r.description())
	writeYAMLString(&b, "model", r.input.Agent.Model.Model)
	writeYAMLString(&b, "effort", r.input.Agent.Model.ReasoningEffort)
	writeYAMLStringList(&b, "tools", shared.SortedStrings(r.input.Agent.Permissions.Allow))
	writeYAMLStringList(&b, "disallowedTools", shared.SortedStrings(r.input.Agent.Permissions.Deny))
	writeYAMLStringList(&b, "skills", capabilityNames(r.input.Capabilities.Skills))
	writeYAMLStringList(&b, "mcpServers", mcpServerNames(r.renderableMCPServers()))
	writeYAMLStringList(&b, "hooks", capabilityNames(r.input.Capabilities.Hooks))
	if scope, ok := nativeMemoryScope(r.input.Agent.MemoryRefs); ok {
		writeYAMLString(&b, "memory", scope)
	}
	b.WriteString("---\n\n")
	b.WriteString(r.agentInstructions())
	b.WriteByte('\n')
	return b.String()
}

func (r renderContext) renderSettingsFile() string {
	data, err := shared.MarshalJSON(map[string]any{
		"agent": r.agentFileName,
	})
	if err != nil {
		return "{\n  \"agent\": " + strconv.Quote(r.agentFileName) + "\n}\n"
	}
	return string(data)
}

func (r renderContext) renderMCPDocument() string {
	servers := make(map[string]mcpServerConfig)
	for _, server := range r.renderableMCPServers() {
		servers[server.Name] = mcpServerConfigFromAdapter(server)
	}
	data, err := shared.MarshalJSON(map[string]any{"mcpServers": servers})
	if err != nil {
		return "{\n  \"mcpServers\": {}\n}\n"
	}
	return string(data)
}

func (r renderContext) description() string {
	return shared.FirstNonEmpty(r.input.Agent.Description, "AVM agent "+r.agentName)
}

func (r renderContext) agentInstructions() string {
	var sections []string
	if r.input.Agent.Instructions.System != "" {
		sections = append(sections, section("System instructions", r.input.Agent.Instructions.System))
	}
	if r.input.Agent.Instructions.Developer != "" {
		sections = append(sections, section("Developer instructions", r.input.Agent.Instructions.Developer))
	}
	if len(r.input.Agent.Instructions.References) > 0 {
		sections = append(sections, bulletSection("Instruction references", shared.SortedStrings(r.input.Agent.Instructions.References)))
	}
	if len(r.input.Capabilities.Skills) > 0 {
		sections = append(sections, bulletSection("Active AVM skills", skillLines(r.input.Capabilities.Skills)))
	}
	if len(r.input.Agent.MemoryRefs) > 0 {
		sections = append(sections, bulletSection("AVM memory refs", shared.MemoryRefLines(r.input.Agent.MemoryRefs)))
	}
	if len(r.input.Memory) > 0 {
		sections = append(sections, bulletSection("Portable memory", shared.PortableMemoryLines(r.input.Memory)))
	}
	if r.input.Agent.Permissions.Approval != "" {
		sections = append(sections, section("Permission approval policy", r.input.Agent.Permissions.Approval))
	}
	if r.input.Agent.Permissions.Sandbox != "" {
		sections = append(sections, section("Sandbox mode", r.input.Agent.Permissions.Sandbox))
	}
	if r.input.Agent.Model.Verbosity != "" {
		sections = append(sections, section("Response verbosity", r.input.Agent.Model.Verbosity))
	}
	if len(r.input.Capabilities.Toolsets) > 0 {
		sections = append(sections, bulletSection("Requested toolsets", shared.ToolsetLines(r.input.Capabilities.Toolsets)))
	}
	if len(r.input.Capabilities.Commands) > 0 {
		sections = append(sections, bulletSection("Requested AVM commands", shared.CapabilityLines(r.input.Capabilities.Commands)))
	}
	if len(sections) == 0 {
		return "Follow the AVM agent profile for " + r.agentName + "."
	}
	return strings.Join(sections, "\n\n")
}

func (r renderContext) mappings() []adapter.FieldMapping {
	targetAgent := r.agentPath
	targetBody := targetAgent + "#body"
	targetFrontmatter := targetAgent + "#frontmatter"
	mappings := []adapter.FieldMapping{
		{SourcePath: "active", TargetPath: targetAgent, Status: adapter.MappingNative},
		{SourcePath: "agent.name", TargetPath: targetFrontmatter + ".name", Status: adapter.MappingNative},
		{SourcePath: "agent.description", TargetPath: targetFrontmatter + ".description", Status: adapter.MappingNative},
		{SourcePath: "agent.instructions.system", TargetPath: targetBody, Status: adapter.MappingNative},
		{SourcePath: "agent.instructions.developer", TargetPath: targetBody, Status: adapter.MappingNative},
		{SourcePath: "agent.model.model", TargetPath: targetFrontmatter + ".model", Status: adapter.MappingNative},
		{SourcePath: "agent.model.reasoning_effort", TargetPath: targetFrontmatter + ".effort", Status: adapter.MappingNative},
		{SourcePath: "agent.permissions.allow", TargetPath: targetFrontmatter + ".tools", Status: adapter.MappingNative},
		{SourcePath: "agent.permissions.deny", TargetPath: targetFrontmatter + ".disallowedTools", Status: adapter.MappingNative},
		{
			SourcePath: "agent.permissions.approval",
			TargetPath: targetBody,
			Status:     adapter.MappingRenderedAsInstructions,
			Reason:     "Claude Code permission modes do not exactly match AVM approval policies in Phase 1.",
		},
		{
			SourcePath: "agent.permissions.sandbox",
			TargetPath: targetBody,
			Status:     adapter.MappingRenderedAsInstructions,
			Reason:     "Claude Code sandbox settings are not written by the project agent file in Phase 1.",
		},
		{SourcePath: "capabilities.skills", TargetPath: targetFrontmatter + ".skills", Status: adapter.MappingNative},
		{
			SourcePath: "project.CLAUDE.md",
			Status:     adapter.MappingIgnored,
			Reason:     "Claude project instructions are user-owned; the Claude Code adapter does not overwrite CLAUDE.md.",
		},
	}

	if len(r.input.Agent.MemoryRefs) > 0 {
		status := adapter.MappingRenderedAsInstructions
		targetPath := targetBody
		reason := "Claude Code native memory content is not written during avm use; AVM memory refs are rendered as agent instructions."
		if _, ok := nativeMemoryScope(r.input.Agent.MemoryRefs); ok {
			status = adapter.MappingNative
			targetPath = targetFrontmatter + ".memory"
			reason = "Claude Code can express this AVM memory scope in agent frontmatter; memory content remains read-only during avm use."
		}
		mappings = append(mappings, adapter.FieldMapping{
			SourcePath: "agent.memory_refs",
			TargetPath: targetPath,
			Status:     status,
			Reason:     reason,
		})
	}
	if len(r.input.Agent.Instructions.References) > 0 {
		mappings = append(mappings, adapter.FieldMapping{
			SourcePath: "agent.instructions.references",
			TargetPath: targetBody,
			Status:     adapter.MappingRenderedAsInstructions,
			Reason:     "Claude Code agent files do not have a separate references field in Phase 1.",
		})
	}
	if r.input.Agent.Model.Verbosity != "" {
		mappings = append(mappings, adapter.FieldMapping{
			SourcePath: "agent.model.verbosity",
			TargetPath: targetBody,
			Status:     adapter.MappingRenderedAsInstructions,
			Reason:     "Claude Code Phase 1 does not expose an AVM verbosity field; it is preserved as agent guidance.",
		})
	}
	if r.input.Agent.Model.Temperature != nil {
		mappings = append(mappings, adapter.FieldMapping{
			SourcePath: "agent.model.temperature",
			Status:     adapter.MappingUnsupported,
			Reason:     "Claude Code adapter Phase 1 does not support temperature.",
		})
	}
	if len(r.input.Agent.Permissions.AdditionalDirectories) > 0 {
		mappings = append(mappings, adapter.FieldMapping{
			SourcePath: "agent.permissions.additional_directories",
			Status:     adapter.MappingUnsupported,
			Reason:     "Claude Code adapter Phase 1 does not modify settings additionalDirectories.",
		})
	}
	if len(r.input.Memory) > 0 {
		mappings = append(mappings, adapter.FieldMapping{
			SourcePath: "memory",
			TargetPath: targetBody,
			Status:     adapter.MappingRenderedAsInstructions,
			Reason:     "Portable memory content is referenced from Claude Code agent instructions in Phase 1.",
		})
	}
	if len(r.input.Capabilities.Commands) > 0 {
		mappings = append(mappings, adapter.FieldMapping{
			SourcePath: "capabilities.commands",
			TargetPath: targetBody,
			Status:     adapter.MappingRenderedAsInstructions,
			Reason:     "Claude Code adapter Phase 1 preserves AVM command capability names as instructions only.",
		})
	}
	if len(r.input.Capabilities.Hooks) > 0 {
		mappings = append(mappings, adapter.FieldMapping{
			SourcePath: "capabilities.hooks",
			TargetPath: targetFrontmatter + ".hooks",
			Status:     adapter.MappingNative,
		})
	}
	if len(r.input.Capabilities.Toolsets) > 0 {
		mappings = append(mappings, adapter.FieldMapping{
			SourcePath: "capabilities.toolsets",
			TargetPath: targetBody,
			Status:     adapter.MappingRenderedAsInstructions,
			Reason:     "Claude Code adapter Phase 1 does not enforce AVM toolset modes natively.",
		})
	}
	for _, server := range shared.SortedMCPServers(r.input.Capabilities.MCPServers) {
		source := "capabilities.mcp_servers." + server.Name
		if shared.MCPServerRenderable(server) {
			mappings = append(mappings, adapter.FieldMapping{
				SourcePath: source,
				TargetPath: r.mcpPath + "#mcpServers." + server.Name,
				Status:     adapter.MappingNative,
			})
			continue
		}
		mappings = append(mappings, adapter.FieldMapping{
			SourcePath: source,
			Status:     adapter.MappingUnsupported,
			Reason:     "Claude Code MCP rendering requires command or URL.",
		})
	}
	return mappings
}

func (r renderContext) warnings() []string {
	var warnings []string
	for _, server := range shared.SortedMCPServers(r.input.Capabilities.MCPServers) {
		if !shared.MCPServerRenderable(server) {
			warnings = append(warnings, fmt.Sprintf("mcp server %q was not rendered because command or URL is missing", server.Name))
		}
	}
	if len(r.input.Agent.MemoryRefs) > 0 {
		if _, ok := nativeMemoryScope(r.input.Agent.MemoryRefs); !ok {
			warnings = append(warnings, "memory refs were rendered as instructions because their scopes cannot be represented by one Claude Code memory scope")
		}
	}
	return warnings
}

func (r renderContext) renderableMCPServers() []adapter.MCPServer {
	servers := shared.SortedMCPServers(r.input.Capabilities.MCPServers)
	out := make([]adapter.MCPServer, 0, len(servers))
	for _, server := range servers {
		if shared.MCPServerRenderable(server) {
			out = append(out, server)
		}
	}
	return out
}

func applyOperation(operation adapter.RenderOperation, managed adapter.ManagedPath) (bool, error) {
	switch operation.Action {
	case adapter.OperationWriteFile:
		if managed.MergeMode != adapter.MergeModeWholeFile {
			return false, fmt.Errorf("claude-code write operation %q requires whole-file managed path %s", operation.ID, operation.Path)
		}
		return shared.WriteFileAtomic(operation.Path, operation.Content)
	case adapter.OperationRemoveFile:
		if managed.MergeMode != adapter.MergeModeWholeFile {
			return false, fmt.Errorf("claude-code remove operation %q requires whole-file managed path %s", operation.ID, operation.Path)
		}
		return shared.RemoveFileAndEmptyParent(operation.Path)
	case adapter.OperationStructuredSet:
		if managed.MergeMode != adapter.MergeModeStructuredSection {
			return false, fmt.Errorf("claude-code structured operation %q requires structured-section managed path %s", operation.ID, operation.Path)
		}
		if operation.ID != mcpOperationID {
			return false, fmt.Errorf("claude-code adapter cannot apply structured operation %q at %s", operation.ID, operation.Path)
		}
		return mergeMCPDocument(operation.Path, operation.Content)
	default:
		return false, fmt.Errorf("claude-code adapter cannot apply %s operation %q at %s", operation.Action, operation.ID, operation.Path)
	}
}

func mergeMCPDocument(path string, desiredContent []byte) (bool, error) {
	var desired mcpDocument
	if err := json.Unmarshal(desiredContent, &desired); err != nil {
		return false, fmt.Errorf("parse desired Claude Code MCP content: %w", err)
	}

	root := make(map[string]any)
	existing, err := os.ReadFile(path)
	if err == nil && len(bytes.TrimSpace(existing)) > 0 {
		if err := json.Unmarshal(existing, &root); err != nil {
			return false, fmt.Errorf("parse existing Claude Code MCP file %s: %w", path, err)
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}

	servers, err := objectField(root, "mcpServers")
	if err != nil {
		return false, err
	}
	previousManaged, err := managedMCPServerSet(root)
	if err != nil {
		return false, err
	}
	for name := range previousManaged {
		delete(servers, name)
	}

	desiredNames := sortedMCPServerConfigNames(desired.MCPServers)
	for _, name := range desiredNames {
		if _, exists := servers[name]; exists {
			return false, fmt.Errorf("claude-code MCP server %q already exists and is not AVM-managed", name)
		}
		servers[name] = desired.MCPServers[name]
	}
	root["mcpServers"] = servers
	setManagedMCPServers(root, desiredNames)

	next, err := shared.MarshalJSON(root)
	if err != nil {
		return false, err
	}
	return shared.WriteFileAtomic(path, next)
}

func objectField(root map[string]any, key string) (map[string]any, error) {
	if value, ok := root[key]; ok {
		object, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("Claude Code MCP field %q must be an object", key)
		}
		return object, nil
	}
	object := make(map[string]any)
	root[key] = object
	return object, nil
}

func managedMCPServerSet(root map[string]any) (map[string]struct{}, error) {
	out := make(map[string]struct{})
	runtimeMeta, err := runtimeMetadata(root)
	if err != nil {
		return nil, err
	}
	if runtimeMeta == nil {
		return out, nil
	}
	value, ok := runtimeMeta["managedMCPServers"]
	if !ok {
		return out, nil
	}
	items, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("Claude Code AVM metadata managedMCPServers must be an array")
	}
	for _, item := range items {
		name, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("Claude Code AVM metadata managedMCPServers entries must be strings")
		}
		out[name] = struct{}{}
	}
	return out, nil
}

func runtimeMetadata(root map[string]any) (map[string]any, error) {
	avm, ok := root[avmMetadataKey]
	if !ok {
		return nil, nil
	}
	avmObject, ok := avm.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("Claude Code AVM metadata field %q must be an object", avmMetadataKey)
	}
	runtime, ok := avmObject[avmMetadataSubkey]
	if !ok {
		return nil, nil
	}
	runtimeObject, ok := runtime.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("Claude Code AVM metadata field %q.%s must be an object", avmMetadataKey, avmMetadataSubkey)
	}
	return runtimeObject, nil
}

func setManagedMCPServers(root map[string]any, names []string) {
	avmObject, ok := root[avmMetadataKey].(map[string]any)
	if !ok {
		avmObject = make(map[string]any)
		root[avmMetadataKey] = avmObject
	}
	runtimeObject, ok := avmObject[avmMetadataSubkey].(map[string]any)
	if !ok {
		runtimeObject = make(map[string]any)
		avmObject[avmMetadataSubkey] = runtimeObject
	}
	runtimeObject["managedMCPServers"] = names
}

type mcpDocument struct {
	MCPServers map[string]mcpServerConfig `json:"mcpServers,omitempty"`
}

type mcpServerConfig struct {
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	URL     string            `json:"url,omitempty"`
}

func mcpServerConfigFromAdapter(server adapter.MCPServer) mcpServerConfig {
	config := mcpServerConfig{
		Command: server.Command,
		Args:    append([]string(nil), server.Args...),
		URL:     server.URL,
	}
	if len(server.Env) > 0 {
		env := append([]adapter.EnvVar(nil), server.Env...)
		sort.SliceStable(env, func(i, j int) bool {
			return env[i].Name < env[j].Name
		})
		config.Env = make(map[string]string, len(env))
		for _, item := range env {
			if item.Name != "" {
				config.Env[item.Name] = item.Value
			}
		}
		if len(config.Env) == 0 {
			config.Env = nil
		}
	}
	return config
}

type skillFile struct {
	name    string
	target  string
	content []byte
}

type staleSkillFile struct {
	name   string
	target string
}

func claudeSkillFiles(input adapter.RenderInput, configDir string) ([]skillFile, []string) {
	if input.ActiveDir == "" || len(input.Capabilities.Skills) == 0 {
		return nil, nil
	}
	var files []skillFile
	var warnings []string
	seen := make(map[string]struct{}, len(input.Capabilities.Skills))
	for _, ref := range input.Capabilities.Skills {
		if ref.Name == "" || ref.Path == "" {
			continue
		}
		if _, ok := seen[ref.Name]; ok {
			continue
		}
		seen[ref.Name] = struct{}{}
		content, err := renderRuntimeSkillFile(ref.Name, filepath.FromSlash(ref.Path))
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("skill %q was not installed because %v", ref.Name, err))
			continue
		}
		files = append(files, skillFile{
			name:    ref.Name,
			target:  filepath.ToSlash(filepath.Join(configDir, skillsDirName, ref.Name, skillFileName)),
			content: content,
		})
	}
	sort.SliceStable(files, func(i, j int) bool {
		return files[i].name < files[j].name
	})
	sort.Strings(warnings)
	return files, warnings
}

func skillFileNames(files []skillFile) map[string]struct{} {
	names := make(map[string]struct{}, len(files))
	for _, file := range files {
		if file.name != "" {
			names[file.name] = struct{}{}
		}
	}
	return names
}

func staleClaudeSkillFiles(configDir string, desired map[string]struct{}) []staleSkillFile {
	skillsDir := filepath.Join(configDir, skillsDirName)
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return nil
	}
	var stale []staleSkillFile
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !safeRuntimeSkillName(name) {
			continue
		}
		if _, ok := desired[name]; ok {
			continue
		}
		path := filepath.Join(skillsDir, name, skillFileName)
		if runtimeSkillFileAVMManaged(path, name) {
			stale = append(stale, staleSkillFile{name: name, target: filepath.ToSlash(path)})
		}
	}
	sort.SliceStable(stale, func(i, j int) bool {
		return stale[i].name < stale[j].name
	})
	return stale
}

func renderRuntimeSkillFile(name, sourcePath string) ([]byte, error) {
	raw, err := os.ReadFile(sourcePath)
	if err != nil {
		return nil, err
	}
	if hasSkillFrontmatter(raw) {
		return ensureRuntimeSkillManaged(raw), nil
	}
	var b strings.Builder
	b.WriteString("---\n")
	writeYAMLString(&b, "name", name)
	writeYAMLString(&b, "description", "AVM skill "+name+".")
	shared.WriteLine(&b, "%s: true", avmManagedKey)
	b.WriteString("---\n\n")
	b.Write(bytes.TrimLeft(raw, "\n"))
	return []byte(b.String()), nil
}

func ensureRuntimeSkillManaged(raw []byte) []byte {
	if runtimeSkillContentAVMManaged(raw, "") {
		return raw
	}
	start, end := skillFrontmatterSpan(raw)
	if start < 0 || end < 0 {
		return raw
	}
	out := make([]byte, 0, len(raw)+len(avmManagedKey)+8)
	out = append(out, raw[:end]...)
	if end > 0 && raw[end-1] != '\n' {
		out = append(out, '\n')
	}
	out = append(out, []byte(avmManagedKey+": true\n")...)
	out = append(out, raw[end:]...)
	return out
}

func hasSkillFrontmatter(raw []byte) bool {
	start, end := skillFrontmatterSpan(raw)
	return start == 0 && end > 0
}

func runtimeSkillFileAVMManaged(path, name string) bool {
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return runtimeSkillContentAVMManaged(raw, name)
}

func runtimeSkillContentAVMManaged(raw []byte, name string) bool {
	start, end := skillFrontmatterSpan(raw)
	if start != 0 || end <= 0 {
		return false
	}
	frontmatter := string(raw[start:end])
	if strings.Contains(frontmatter, avmManagedKey+": true") || strings.Contains(frontmatter, avmManagedKey+": \"true\"") {
		return true
	}
	if name != "" && strings.Contains(frontmatter, `description: "AVM skill `+name+`."`) {
		return true
	}
	return false
}

func skillFrontmatterSpan(raw []byte) (int, int) {
	trimmed := bytes.TrimLeft(raw, "\ufeff\n\r\t ")
	if len(trimmed) != len(raw) {
		return -1, -1
	}
	if !bytes.HasPrefix(raw, []byte("---\n")) && !bytes.HasPrefix(raw, []byte("---\r\n")) {
		return -1, -1
	}
	lineStart := 3
	if len(raw) > 3 && raw[3] == '\r' {
		lineStart = 5
	} else if len(raw) > 3 && raw[3] == '\n' {
		lineStart = 4
	}
	for lineStart < len(raw) {
		lineEnd := lineStart
		for lineEnd < len(raw) && raw[lineEnd] != '\n' {
			lineEnd++
		}
		line := bytes.TrimSpace(bytes.TrimRight(raw[lineStart:lineEnd], "\r"))
		if bytes.Equal(line, []byte("---")) {
			return 0, lineStart
		}
		lineStart = lineEnd
		if lineStart < len(raw) && raw[lineStart] == '\n' {
			lineStart++
		}
	}
	return -1, -1
}

func safeRuntimeSkillName(name string) bool {
	return name != "" && name != "." && name != ".." && !strings.Contains(name, "/") && !strings.Contains(name, "\\")
}

func importAgents(dir string) ([]adapter.ImportedAgent, error) {
	info, err := os.Stat(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("Claude Code agents path %s is not a directory", dir)
	}

	var agents []adapter.ImportedAgent
	err = filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".md" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		agents = append(agents, parseImportedAgent(path, data))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return agents, nil
}

func parseImportedAgent(path string, data []byte) adapter.ImportedAgent {
	sourcePath := filepath.ToSlash(path)
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	description := ""
	body := string(data)

	if fields, rest, ok := splitFrontmatter(body); ok {
		if value := fields["name"]; value != "" {
			name = value
		}
		description = fields["description"]
		body = rest
	}

	return adapter.ImportedAgent{
		Name:        name,
		Description: description,
		SourcePath:  sourcePath,
		Instructions: adapter.Instructions{
			Developer: strings.TrimSpace(body),
		},
		Mappings: []adapter.FieldMapping{
			{SourcePath: sourcePath + "#frontmatter.name", TargetPath: "agent.name", Status: adapter.MappingNative},
			{SourcePath: sourcePath + "#frontmatter.description", TargetPath: "agent.description", Status: adapter.MappingNative},
			{SourcePath: sourcePath + "#body", TargetPath: "agent.instructions.developer", Status: adapter.MappingNative},
		},
	}
}

func splitFrontmatter(content string) (map[string]string, string, bool) {
	if !strings.HasPrefix(content, "---\n") {
		return nil, content, false
	}
	end := strings.Index(content[4:], "\n---")
	if end < 0 {
		return nil, content, false
	}
	frontmatter := content[4 : 4+end]
	restStart := 4 + end + len("\n---")
	if restStart < len(content) && content[restStart] == '\r' {
		restStart++
	}
	if restStart < len(content) && content[restStart] == '\n' {
		restStart++
	}

	fields := make(map[string]string)
	for _, line := range strings.Split(frontmatter, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if key != "" {
			fields[key] = value
		}
	}
	return fields, content[restStart:], true
}

func writeYAMLString(builder *strings.Builder, key, value string) {
	if value == "" {
		return
	}
	shared.WriteLine(builder, "%s: %s", key, strconv.Quote(value))
}

func writeYAMLStringList(builder *strings.Builder, key string, values []string) {
	if len(values) == 0 {
		return
	}
	shared.WriteLine(builder, "%s:", key)
	for _, value := range values {
		shared.WriteLine(builder, "  - %s", strconv.Quote(value))
	}
}

func section(title, body string) string {
	return title + ":\n" + strings.TrimSpace(body)
}

func bulletSection(title string, lines []string) string {
	var b strings.Builder
	b.WriteString(title)
	b.WriteString(":\n")
	for _, line := range lines {
		b.WriteString("- ")
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

func skillLines(refs []adapter.CapabilityRef) []string {
	lines := make([]string, 0, len(refs))
	for _, ref := range refs {
		if ref.Name == "" {
			continue
		}
		line := ref.Name
		if ref.Path != "" {
			line += " (" + ref.Path + ")"
		}
		lines = append(lines, line)
	}
	sort.Strings(lines)
	return lines
}

func capabilityNames(refs []adapter.CapabilityRef) []string {
	names := make([]string, 0, len(refs))
	for _, ref := range refs {
		if ref.Name != "" {
			names = append(names, ref.Name)
		}
	}
	sort.Strings(names)
	return names
}

func sortedMCPServerConfigNames(servers map[string]mcpServerConfig) []string {
	names := make([]string, 0, len(servers))
	for name := range servers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func mcpServerNames(servers []adapter.MCPServer) []string {
	names := make([]string, 0, len(servers))
	for _, server := range servers {
		if server.Name != "" {
			names = append(names, server.Name)
		}
	}
	sort.Strings(names)
	return names
}

func nativeMemoryScope(refs []adapter.MemoryRef) (string, bool) {
	if len(refs) == 0 {
		return "", false
	}
	scope := ""
	for _, ref := range refs {
		if ref.Scope == "" {
			return "", false
		}
		switch ref.Scope {
		case "user", "project", "local":
		default:
			return "", false
		}
		if scope == "" {
			scope = ref.Scope
			continue
		}
		if scope != ref.Scope {
			return "", false
		}
	}
	return scope, true
}
