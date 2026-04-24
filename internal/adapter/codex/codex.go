// Package codex renders AVM agents into Codex configuration files.
package codex

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/xz1220/agent-vm/internal/adapter"
	"github.com/xz1220/agent-vm/internal/adapter/renderplan"
)

const (
	runtimeName       = "codex"
	configFileName    = "config.toml"
	agentsDirName     = "agents"
	configOperationID = "codex-config"
	roleOperationID   = "codex-agent-role"
)

// Adapter renders the conservative Phase 1 Codex path.
type Adapter struct {
	configDir string
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

func (a *Adapter) Name() string {
	return runtimeName
}

func (a *Adapter) Detect(ctx adapter.Context) adapter.Detection {
	configDir := a.codexHome()
	configPath := filepath.Join(configDir, configFileName)

	found := false
	if _, err := os.Stat(configDir); err == nil {
		found = true
	}
	if _, err := os.Stat(configPath); err == nil {
		found = true
	}

	version := ""
	if path, err := exec.LookPath(runtimeName); err == nil {
		found = true
		version = codexVersion(ctx, path)
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

	return &adapter.ImportResult{
		Runtime: runtimeName,
		Warnings: []string{
			"codex import is read-only placeholder in Phase 1",
		},
	}, nil
}

func (a *Adapter) Plan(ctx adapter.Context, input adapter.RenderInput) (*adapter.RenderPlan, error) {
	_ = ctx

	runtime := input.Runtime
	if runtime == "" {
		runtime = runtimeName
	}
	if runtime != runtimeName {
		return nil, fmt.Errorf("codex adapter cannot plan runtime %q", runtime)
	}

	agentName := firstNonEmpty(input.Agent.Name, input.Active.Name, "agent")
	roleName := slug(agentName)
	activeName := firstNonEmpty(input.Active.Name, agentName, "default")
	profileName := "avm-" + slug(activeName)
	configPath := filepath.ToSlash(filepath.Join(a.codexHome(), configFileName))
	rolePath := filepath.ToSlash(filepath.Join(a.codexHome(), agentsDirName, roleName+".toml"))
	roleConfigPath := "./" + filepath.ToSlash(filepath.Join(agentsDirName, roleName+".toml"))

	render := renderContext{
		input:          input,
		agentName:      agentName,
		roleName:       roleName,
		profileName:    profileName,
		configPath:     configPath,
		rolePath:       rolePath,
		roleConfigPath: roleConfigPath,
	}

	plan := &adapter.RenderPlan{
		Runtime:   runtimeName,
		Active:    input.Active,
		AgentName: agentName,
		ManagedPaths: []adapter.ManagedPath{
			{
				Path:        configPath,
				Owner:       "shared-section",
				Description: "Codex AVM-managed profile, MCP server, and role registration sections.",
				Required:    true,
				MergeMode:   adapter.MergeModeStructuredSection,
			},
			{
				Path:        rolePath,
				Owner:       "avm",
				Description: "Codex role configuration rendered from the AVM agent profile.",
				Required:    true,
				MergeMode:   adapter.MergeModeWholeFile,
			},
		},
		Operations: []adapter.RenderOperation{
			{
				ID:          configOperationID,
				Action:      adapter.OperationStructuredSet,
				Path:        configPath,
				Content:     []byte(render.renderConfigSection()),
				Description: "merge Codex AVM-managed config sections",
				Required:    true,
			},
			{
				ID:          roleOperationID,
				Action:      adapter.OperationWriteFile,
				Path:        rolePath,
				Content:     []byte(render.renderRoleFile()),
				Description: "write Codex AVM-managed role file",
				Required:    true,
			},
		},
		Mappings: render.mappings(),
		Warnings: render.warnings(),
	}

	return renderplan.Normalize(plan), nil
}

func (a *Adapter) Render(ctx adapter.Context, plan *adapter.RenderPlan) (*adapter.RenderResult, error) {
	_ = ctx

	normalized := renderplan.Normalize(plan)
	if normalized == nil {
		return nil, fmt.Errorf("codex adapter render plan is nil")
	}
	if normalized.Runtime != "" && normalized.Runtime != runtimeName {
		return nil, fmt.Errorf("codex adapter cannot render runtime %q", normalized.Runtime)
	}

	managed, err := managedPathIndex(normalized.ManagedPaths, a.codexHome())
	if err != nil {
		return nil, err
	}

	pending := make([]plannedOperation, 0, len(normalized.Operations))
	for _, operation := range normalized.Operations {
		cleanPath, err := cleanRenderPath(operation.Path)
		if err != nil {
			return nil, fmt.Errorf("codex render operation %q has invalid path %q: %w", operation.ID, operation.Path, err)
		}
		operation.Path = cleanPath

		managedPath, ok := managed[operation.Path]
		if !ok {
			return nil, fmt.Errorf("codex render operation %q targets unmanaged path %s", operation.ID, operation.Path)
		}
		if err := validateOperation(operation, managedPath); err != nil {
			return nil, err
		}
		if err := preflightOperation(operation); err != nil {
			return nil, err
		}
		pending = append(pending, plannedOperation{operation: operation, managed: managedPath})
	}

	results := make([]adapter.RenderOperationResult, 0, len(pending))
	for _, item := range pending {
		changed, err := applyOperation(item.operation, item.managed)
		if err != nil {
			return nil, err
		}
		results = append(results, adapter.RenderOperationResult{
			OperationID: item.operation.ID,
			Action:      item.operation.Action,
			Path:        item.operation.Path,
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

func (a *Adapter) codexHome() string {
	if a.configDir != "" {
		return a.configDir
	}
	if value := os.Getenv("CODEX_HOME"); value != "" {
		return value
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".codex")
	}
	return ".codex"
}

func codexVersion(ctx context.Context, path string) string {
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
	input          adapter.RenderInput
	agentName      string
	roleName       string
	profileName    string
	configPath     string
	rolePath       string
	roleConfigPath string
}

func (r renderContext) renderConfigSection() string {
	var b strings.Builder
	writeLine(&b, "[profiles.%s]", r.profileName)
	writeTomlString(&b, "model", r.input.Agent.Model.Model)
	writeTomlString(&b, "model_reasoning_effort", r.input.Agent.Model.ReasoningEffort)
	writeTomlString(&b, "approval_policy", r.input.Agent.Permissions.Approval)
	writeTomlString(&b, "sandbox_mode", r.input.Agent.Permissions.Sandbox)
	b.WriteByte('\n')

	for _, server := range sortedMCPServers(r.input.Capabilities.MCPServers) {
		if !mcpServerRenderable(server) {
			continue
		}
		writeLine(&b, "[mcp_servers.%s]", slug(server.Name))
		writeTomlString(&b, "command", server.Command)
		if server.URL != "" {
			writeTomlString(&b, "url", server.URL)
		}
		if len(server.Args) > 0 {
			writeLine(&b, "args = %s", tomlStringArray(server.Args))
		}
		if len(server.Env) > 0 {
			writeLine(&b, "env = %s", tomlEnvInlineTable(server.Env))
		}
		b.WriteByte('\n')
	}

	writeLine(&b, "[agents.%s]", r.roleName)
	writeTomlString(&b, "description", r.description())
	writeTomlString(&b, "config_file", r.roleConfigPath)
	writeLine(&b, "nickname_candidates = %s", tomlStringArray([]string{r.agentName}))

	return strings.TrimRight(b.String(), "\n") + "\n"
}

func (r renderContext) renderRoleFile() string {
	var b strings.Builder
	writeTomlString(&b, "name", r.roleName)
	writeTomlString(&b, "description", r.description())
	writeLine(&b, "nickname_candidates = %s", tomlStringArray([]string{r.agentName}))
	writeTomlString(&b, "developer_instructions", r.developerInstructions())
	writeTomlString(&b, "model", r.input.Agent.Model.Model)
	writeTomlString(&b, "model_reasoning_effort", r.input.Agent.Model.ReasoningEffort)
	writeTomlString(&b, "approval_policy", r.input.Agent.Permissions.Approval)
	writeTomlString(&b, "sandbox_mode", r.input.Agent.Permissions.Sandbox)
	return b.String()
}

func (r renderContext) description() string {
	return firstNonEmpty(r.input.Agent.Description, "AVM agent "+r.agentName)
}

func (r renderContext) developerInstructions() string {
	var sections []string
	if r.input.Agent.Instructions.System != "" {
		sections = append(sections, section("System instructions", r.input.Agent.Instructions.System))
	}
	if r.input.Agent.Instructions.Developer != "" {
		sections = append(sections, section("Developer instructions", r.input.Agent.Instructions.Developer))
	}
	if len(r.input.Agent.Instructions.References) > 0 {
		sections = append(sections, bulletSection("Instruction references", sortedStrings(r.input.Agent.Instructions.References)))
	}
	if len(r.input.Capabilities.Skills) > 0 {
		sections = append(sections, bulletSection("Active AVM skills", skillLines(r.input.Capabilities.Skills)))
	}
	if len(r.input.Agent.MemoryRefs) > 0 {
		sections = append(sections, bulletSection("AVM memory refs", memoryRefLines(r.input.Agent.MemoryRefs)))
	}
	if len(r.input.Memory) > 0 {
		sections = append(sections, bulletSection("Portable memory", portableMemoryLines(r.input.Memory)))
	}
	if len(r.input.Agent.Permissions.Allow) > 0 {
		sections = append(sections, bulletSection("Allowed command guidance", sortedStrings(r.input.Agent.Permissions.Allow)))
	}
	if len(r.input.Agent.Permissions.Deny) > 0 {
		sections = append(sections, bulletSection("Denied command guidance", sortedStrings(r.input.Agent.Permissions.Deny)))
	}
	if r.input.Agent.Model.Verbosity != "" {
		sections = append(sections, section("Response verbosity", r.input.Agent.Model.Verbosity))
	}
	if len(r.input.Capabilities.Toolsets) > 0 {
		sections = append(sections, bulletSection("Requested toolsets", toolsetLines(r.input.Capabilities.Toolsets)))
	}
	if len(r.input.Capabilities.Commands) > 0 {
		sections = append(sections, bulletSection("Requested AVM commands", capabilityLines(r.input.Capabilities.Commands)))
	}
	if len(r.input.Capabilities.Hooks) > 0 {
		sections = append(sections, bulletSection("Requested AVM hooks", capabilityLines(r.input.Capabilities.Hooks)))
	}

	return strings.Join(sections, "\n\n")
}

func (r renderContext) mappings() []adapter.FieldMapping {
	targetProfile := "profiles." + r.profileName
	targetInstructions := r.rolePath + "#developer_instructions"
	mappings := []adapter.FieldMapping{
		{
			SourcePath: "active",
			TargetPath: targetProfile,
			Status:     adapter.MappingNative,
		},
		{
			SourcePath: "agent.name",
			TargetPath: "agents." + r.roleName + ".name",
			Status:     adapter.MappingNative,
		},
		{
			SourcePath: "agent.description",
			TargetPath: "agents." + r.roleName + ".description",
			Status:     adapter.MappingNative,
		},
		{
			SourcePath: "agent.instructions.system",
			TargetPath: targetInstructions,
			Status:     adapter.MappingRenderedAsInstructions,
			Reason:     "Codex role files have developer instructions but no separate AVM system instruction field in Phase 1.",
		},
		{
			SourcePath: "agent.instructions.developer",
			TargetPath: targetInstructions,
			Status:     adapter.MappingNative,
		},
		{
			SourcePath: "agent.model.model",
			TargetPath: targetProfile + ".model",
			Status:     adapter.MappingNative,
		},
		{
			SourcePath: "agent.model.reasoning_effort",
			TargetPath: targetProfile + ".model_reasoning_effort",
			Status:     adapter.MappingNative,
		},
		{
			SourcePath: "agent.permissions.approval",
			TargetPath: targetProfile + ".approval_policy",
			Status:     adapter.MappingNative,
		},
		{
			SourcePath: "agent.permissions.sandbox",
			TargetPath: targetProfile + ".sandbox_mode",
			Status:     adapter.MappingNative,
		},
		{
			SourcePath: "capabilities.skills",
			TargetPath: targetInstructions,
			Status:     adapter.MappingRenderedAsInstructions,
			Reason:     "Codex has no native AVM skill registry mount in Phase 1.",
		},
		{
			SourcePath: "agent.memory_refs",
			TargetPath: targetInstructions,
			Status:     adapter.MappingRenderedAsInstructions,
			Reason:     "Codex has no native portable memory scope in Phase 1.",
		},
		{
			SourcePath: "project.AGENTS.md",
			Status:     adapter.MappingIgnored,
			Reason:     "Codex project instructions are user-owned; the Codex adapter does not overwrite AGENTS.md.",
		},
	}

	if len(r.input.Agent.Instructions.References) > 0 {
		mappings = append(mappings, adapter.FieldMapping{
			SourcePath: "agent.instructions.references",
			TargetPath: targetInstructions,
			Status:     adapter.MappingRenderedAsInstructions,
			Reason:     "Codex role files do not have a separate references field in Phase 1.",
		})
	}
	if r.input.Agent.Model.Verbosity != "" {
		mappings = append(mappings, adapter.FieldMapping{
			SourcePath: "agent.model.verbosity",
			TargetPath: targetInstructions,
			Status:     adapter.MappingRenderedAsInstructions,
			Reason:     "Codex Phase 1 does not expose an AVM verbosity field; it is preserved as role guidance.",
		})
	}
	if r.input.Agent.Model.Temperature != nil {
		mappings = append(mappings, adapter.FieldMapping{
			SourcePath: "agent.model.temperature",
			Status:     adapter.MappingUnsupported,
			Reason:     "Codex adapter Phase 1 does not support temperature.",
		})
	}
	if len(r.input.Agent.Permissions.Allow) > 0 {
		mappings = append(mappings, adapter.FieldMapping{
			SourcePath: "agent.permissions.allow",
			TargetPath: targetInstructions,
			Status:     adapter.MappingRenderedAsInstructions,
			Reason:     "Codex Phase 1 cannot enforce AVM command allow patterns natively.",
		})
	}
	if len(r.input.Agent.Permissions.Deny) > 0 {
		mappings = append(mappings, adapter.FieldMapping{
			SourcePath: "agent.permissions.deny",
			TargetPath: targetInstructions,
			Status:     adapter.MappingRenderedAsInstructions,
			Reason:     "Codex Phase 1 cannot enforce AVM command deny patterns natively.",
		})
	}
	if len(r.input.Agent.Permissions.AdditionalDirectories) > 0 {
		mappings = append(mappings, adapter.FieldMapping{
			SourcePath: "agent.permissions.additional_directories",
			Status:     adapter.MappingUnsupported,
			Reason:     "Codex adapter Phase 1 does not grant additional writable directories.",
		})
	}
	if len(r.input.Memory) > 0 {
		mappings = append(mappings, adapter.FieldMapping{
			SourcePath: "memory",
			TargetPath: targetInstructions,
			Status:     adapter.MappingRenderedAsInstructions,
			Reason:     "Portable memory content is referenced from Codex instructions in Phase 1.",
		})
	}
	if len(r.input.Capabilities.Commands) > 0 {
		mappings = append(mappings, adapter.FieldMapping{
			SourcePath: "capabilities.commands",
			TargetPath: targetInstructions,
			Status:     adapter.MappingRenderedAsInstructions,
			Reason:     "Codex adapter Phase 1 preserves AVM command capability names as instructions only.",
		})
	}
	if len(r.input.Capabilities.Hooks) > 0 {
		mappings = append(mappings, adapter.FieldMapping{
			SourcePath: "capabilities.hooks",
			TargetPath: targetInstructions,
			Status:     adapter.MappingRenderedAsInstructions,
			Reason:     "Codex adapter Phase 1 does not install AVM hooks.",
		})
	}
	if len(r.input.Capabilities.Toolsets) > 0 {
		mappings = append(mappings, adapter.FieldMapping{
			SourcePath: "capabilities.toolsets",
			TargetPath: targetInstructions,
			Status:     adapter.MappingRenderedAsInstructions,
			Reason:     "Codex adapter Phase 1 does not enforce AVM toolset modes natively.",
		})
	}

	for _, server := range sortedMCPServers(r.input.Capabilities.MCPServers) {
		source := "capabilities.mcp_servers." + server.Name
		if mcpServerRenderable(server) {
			mappings = append(mappings, adapter.FieldMapping{
				SourcePath: source,
				TargetPath: "mcp_servers." + slug(server.Name),
				Status:     adapter.MappingNative,
			})
			continue
		}
		mappings = append(mappings, adapter.FieldMapping{
			SourcePath: source,
			Status:     adapter.MappingUnsupported,
			Reason:     "Codex MCP rendering requires command or URL.",
		})
	}

	return mappings
}

func (r renderContext) warnings() []string {
	var warnings []string
	for _, server := range sortedMCPServers(r.input.Capabilities.MCPServers) {
		if !mcpServerRenderable(server) {
			warnings = append(warnings, fmt.Sprintf("mcp server %q was not rendered because command or URL is missing", server.Name))
		}
	}
	return warnings
}

type plannedOperation struct {
	operation adapter.RenderOperation
	managed   adapter.ManagedPath
}

func validateOperation(operation adapter.RenderOperation, managed adapter.ManagedPath) error {
	switch operation.Action {
	case adapter.OperationWriteFile:
		if managed.MergeMode != adapter.MergeModeWholeFile {
			return fmt.Errorf("codex write operation %q requires whole-file managed path %s", operation.ID, operation.Path)
		}
	case adapter.OperationStructuredSet:
		if managed.MergeMode != adapter.MergeModeStructuredSection {
			return fmt.Errorf("codex structured operation %q requires structured-section managed path %s", operation.ID, operation.Path)
		}
		if operation.ID == "" {
			return fmt.Errorf("codex structured operation for %s must have an id", operation.Path)
		}
	default:
		return fmt.Errorf("codex adapter cannot apply %s operation %q at %s", operation.Action, operation.ID, operation.Path)
	}
	return nil
}

func preflightOperation(operation adapter.RenderOperation) error {
	if operation.Action != adapter.OperationStructuredSet {
		return nil
	}
	return validateMarkedBlock(operation.Path, operation.ID)
}

func applyOperation(operation adapter.RenderOperation, managed adapter.ManagedPath) (bool, error) {
	if err := validateOperation(operation, managed); err != nil {
		return false, err
	}

	switch operation.Action {
	case adapter.OperationWriteFile:
		return writeFileAtomic(operation.Path, operation.Content)
	case adapter.OperationStructuredSet:
		return mergeMarkedBlock(operation.Path, operation.ID, operation.Content)
	default:
		return false, fmt.Errorf("codex adapter cannot apply %s operation %q at %s", operation.Action, operation.ID, operation.Path)
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

func mergeMarkedBlock(path, operationID string, content []byte) (bool, error) {
	if operationID == "" {
		return false, fmt.Errorf("codex structured operation for %s must have an id", path)
	}

	begin := []byte("# >>> avm:codex:" + operationID)
	end := []byte("# <<< avm:codex:" + operationID)
	block := []byte(string(begin) + "\n" + strings.TrimRight(string(content), "\n") + "\n" + string(end) + "\n")

	existing, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}

	next := block
	if err == nil {
		next, err = replaceOrAppendMarkedBlock(existing, begin, end, block)
		if err != nil {
			return false, err
		}
	}
	return writeFileAtomic(path, next)
}

func validateMarkedBlock(path, operationID string) error {
	begin := []byte("# >>> avm:codex:" + operationID)
	end := []byte("# <<< avm:codex:" + operationID)
	existing, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	_, _, _, err = markedBlockSpan(existing, begin, end)
	return err
}

func replaceOrAppendMarkedBlock(existing, begin, end, block []byte) ([]byte, error) {
	start, stop, found, err := markedBlockSpan(existing, begin, end)
	if err != nil {
		return nil, err
	}
	if found {
		out := make([]byte, 0, len(existing)-stop+start+len(block))
		out = append(out, existing[:start]...)
		out = append(out, block...)
		out = append(out, existing[stop:]...)
		return out, nil
	}

	out := append([]byte(nil), existing...)
	if len(out) > 0 && out[len(out)-1] != '\n' {
		out = append(out, '\n')
	}
	if len(out) > 0 {
		out = append(out, '\n')
	}
	out = append(out, block...)
	return out, nil
}

func markedBlockSpan(existing, begin, end []byte) (int, int, bool, error) {
	startOffset := -1
	stopOffset := -1
	for lineStart := 0; lineStart < len(existing); {
		lineEnd := lineStart
		for lineEnd < len(existing) && existing[lineEnd] != '\n' {
			lineEnd++
		}
		nextLineStart := lineEnd
		if nextLineStart < len(existing) && existing[nextLineStart] == '\n' {
			nextLineStart++
		}

		line := bytes.TrimRight(existing[lineStart:lineEnd], "\r")
		switch {
		case bytes.Equal(line, begin):
			if startOffset >= 0 && stopOffset < 0 {
				return 0, 0, false, fmt.Errorf("malformed Codex AVM block: duplicate begin marker %q", string(begin))
			}
			if stopOffset >= 0 {
				return 0, 0, false, fmt.Errorf("malformed Codex AVM block: multiple blocks for marker %q", string(begin))
			}
			startOffset = lineStart
		case bytes.Equal(line, end):
			if startOffset < 0 {
				return 0, 0, false, fmt.Errorf("malformed Codex AVM block: end marker %q appears before begin marker", string(end))
			}
			if stopOffset >= 0 {
				return 0, 0, false, fmt.Errorf("malformed Codex AVM block: duplicate end marker %q", string(end))
			}
			stopOffset = nextLineStart
		}

		lineStart = nextLineStart
	}

	switch {
	case startOffset < 0 && stopOffset < 0:
		return 0, 0, false, nil
	case startOffset >= 0 && stopOffset >= 0:
		return startOffset, stopOffset, true, nil
	default:
		return 0, 0, false, fmt.Errorf("malformed Codex AVM block: marker pair %q / %q is incomplete", string(begin), string(end))
	}
}

func managedPathIndex(paths []adapter.ManagedPath, configDir string) (map[string]adapter.ManagedPath, error) {
	managed := make(map[string]adapter.ManagedPath, len(paths))
	for _, path := range paths {
		cleanPath, err := cleanRenderPath(path.Path)
		if err != nil {
			return nil, fmt.Errorf("codex managed path %q is invalid: %w", path.Path, err)
		}
		if err := validateCodexManagedPath(cleanPath, configDir); err != nil {
			return nil, err
		}
		if _, exists := managed[cleanPath]; exists {
			return nil, fmt.Errorf("codex managed path %s declared more than once", cleanPath)
		}
		path.Path = cleanPath
		managed[cleanPath] = path
	}
	return managed, nil
}

func cleanRenderPath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", fmt.Errorf("path is empty")
	}
	native := filepath.FromSlash(path)
	clean := filepath.Clean(native)
	if clean != native {
		return "", fmt.Errorf("path must be clean")
	}
	return clean, nil
}

func validateCodexManagedPath(path, configDir string) error {
	home, err := filepath.Abs(filepath.Clean(filepath.FromSlash(configDir)))
	if err != nil {
		return err
	}
	target, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	configPath := filepath.Join(home, configFileName)
	if target == configPath {
		return nil
	}

	agentsDir := filepath.Join(home, agentsDirName)
	rel, err := filepath.Rel(agentsDir, target)
	if err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && filepath.Dir(rel) == "." && filepath.Ext(rel) == ".toml" {
		return nil
	}

	return fmt.Errorf("codex managed path %s is outside adapter ownership; allowed paths are %s and %s", path, configPath, filepath.Join(agentsDir, "*.toml"))
}

func writeLine(builder *strings.Builder, format string, args ...any) {
	builder.WriteString(fmt.Sprintf(format, args...))
	builder.WriteByte('\n')
}

func writeTomlString(builder *strings.Builder, key, value string) {
	if value == "" {
		return
	}
	writeLine(builder, "%s = %s", key, strconv.Quote(value))
}

func tomlStringArray(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, strconv.Quote(value))
	}
	return "[" + strings.Join(quoted, ", ") + "]"
}

func tomlEnvInlineTable(env []adapter.EnvVar) string {
	items := append([]adapter.EnvVar(nil), env...)
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].Name < items[j].Name
	})
	parts := make([]string, 0, len(items))
	for _, item := range items {
		if item.Name == "" {
			continue
		}
		parts = append(parts, item.Name+" = "+strconv.Quote(item.Value))
	}
	return "{ " + strings.Join(parts, ", ") + " }"
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

func capabilityLines(refs []adapter.CapabilityRef) []string {
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

func memoryRefLines(refs []adapter.MemoryRef) []string {
	lines := make([]string, 0, len(refs))
	for _, ref := range refs {
		if ref.ID == "" {
			continue
		}
		var parts []string
		if ref.Scope != "" {
			parts = append(parts, "scope="+ref.Scope)
		}
		if ref.Mode != "" {
			parts = append(parts, "mode="+ref.Mode)
		}
		if ref.Path != "" {
			parts = append(parts, "path="+ref.Path)
		}
		line := ref.ID
		if len(parts) > 0 {
			line += " (" + strings.Join(parts, ", ") + ")"
		}
		lines = append(lines, line)
	}
	sort.Strings(lines)
	return lines
}

func portableMemoryLines(memory []adapter.PortableMemory) []string {
	lines := make([]string, 0, len(memory))
	for _, item := range memory {
		if item.ID == "" {
			continue
		}
		var parts []string
		if item.Scope != "" {
			parts = append(parts, "scope="+item.Scope)
		}
		if item.Mode != "" {
			parts = append(parts, "mode="+item.Mode)
		}
		if item.Path != "" {
			parts = append(parts, "path="+item.Path)
		}
		line := item.ID
		if len(parts) > 0 {
			line += " (" + strings.Join(parts, ", ") + ")"
		}
		lines = append(lines, line)
	}
	sort.Strings(lines)
	return lines
}

func toolsetLines(toolsets []adapter.Toolset) []string {
	lines := make([]string, 0, len(toolsets))
	for _, toolset := range toolsets {
		if toolset.Name == "" {
			continue
		}
		line := toolset.Name
		if toolset.Mode != "" {
			line += "=" + toolset.Mode
		}
		lines = append(lines, line)
	}
	sort.Strings(lines)
	return lines
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
