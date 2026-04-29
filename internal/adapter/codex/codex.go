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

	"github.com/xz1220/agent-vm/internal/adapter"
	"github.com/xz1220/agent-vm/internal/adapter/renderplan"
	"github.com/xz1220/agent-vm/internal/adapter/shared"
)

const (
	runtimeName       = "codex"
	configFileName    = "config.toml"
	agentsDirName     = "agents"
	skillsDirName     = "skills"
	skillFileName     = "SKILL.md"
	avmManagedKey     = "avm_managed"
	configOperationID = "codex-config"
	roleOperationID   = "codex-agent-role"
	skillOperationID  = "codex-skill"
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
	_ = ctx
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
	if _, err := exec.LookPath(runtimeName); err == nil {
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

	agentName := shared.FirstNonEmpty(input.Agent.Name, input.Active.Name, "agent")
	roleName := shared.Slug(agentName)
	activeName := shared.FirstNonEmpty(input.Active.Name, agentName, "default")
	profileName := "avm-" + shared.Slug(activeName)
	configDir := a.codexHomeForInput(input)
	configPath := filepath.ToSlash(filepath.Join(configDir, configFileName))
	rolePath := filepath.ToSlash(filepath.Join(configDir, agentsDirName, roleName+".toml"))
	roleConfigPath := "./" + filepath.ToSlash(filepath.Join(agentsDirName, roleName+".toml"))
	skillFiles, skillWarnings := codexSkillFiles(input, configDir)
	staleSkillFiles := staleCodexSkillFiles(configDir, skillFileNames(skillFiles))

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
				Owner:       "avm",
				Description: "Codex config rendered as an isolated AVM-owned runtime home.",
				Required:    true,
				MergeMode:   adapter.MergeModeWholeFile,
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
				Action:      adapter.OperationWriteFile,
				Path:        configPath,
				Content:     []byte(render.renderConfigSection()),
				Description: "write Codex AVM-managed config file",
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
	for _, skillFile := range skillFiles {
		plan.ManagedPaths = append(plan.ManagedPaths, adapter.ManagedPath{
			Path:        skillFile.target,
			Owner:       "avm",
			Description: "Codex skill file rendered from the AVM active skill set.",
			Required:    true,
			MergeMode:   adapter.MergeModeWholeFile,
		})
		plan.Operations = append(plan.Operations, adapter.RenderOperation{
			ID:          skillOperationID + "-" + shared.Slug(skillFile.name),
			Action:      adapter.OperationWriteFile,
			Path:        skillFile.target,
			Content:     skillFile.content,
			Description: "write Codex skill file from AVM active skill",
			Required:    true,
		})
	}
	for _, stale := range staleSkillFiles {
		plan.ManagedPaths = append(plan.ManagedPaths, adapter.ManagedPath{
			Path:        stale.target,
			Owner:       "avm",
			Description: "Stale Codex AVM-managed skill file removed because it is not in the current active skill set.",
			Required:    false,
			MergeMode:   adapter.MergeModeWholeFile,
		})
		plan.Operations = append(plan.Operations, adapter.RenderOperation{
			ID:          skillOperationID + "-remove-" + shared.Slug(stale.name),
			Action:      adapter.OperationRemoveFile,
			Path:        stale.target,
			Description: "remove stale Codex AVM-managed skill file",
			Required:    false,
		})
	}
	plan.Warnings = append(plan.Warnings, skillWarnings...)

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

	managed, err := managedPathIndex(normalized.ManagedPaths, codexHomeFromPlan(normalized, a.codexHome()))
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

func (a *Adapter) codexHomeForInput(input adapter.RenderInput) string {
	if input.RuntimeHome != "" {
		return input.RuntimeHome
	}
	return a.codexHome()
}

func codexHomeFromPlan(plan *adapter.RenderPlan, fallback string) string {
	if plan == nil {
		return fallback
	}
	for _, managedPath := range plan.ManagedPaths {
		path := filepath.Clean(filepath.FromSlash(managedPath.Path))
		if filepath.Base(path) == configFileName {
			return filepath.Dir(path)
		}
	}
	return fallback
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
	writeTomlString(&b, "profile", r.profileName)
	b.WriteByte('\n')
	shared.WriteLine(&b, "[profiles.%s]", r.profileName)
	writeTomlString(&b, "model", r.input.Agent.Model.Model)
	writeTomlString(&b, "model_reasoning_effort", r.input.Agent.Model.ReasoningEffort)
	writeTomlString(&b, "approval_policy", r.input.Agent.Permissions.Approval)
	writeTomlString(&b, "sandbox_mode", r.input.Agent.Permissions.Sandbox)
	b.WriteByte('\n')

	for _, server := range shared.SortedMCPServers(r.input.Capabilities.MCPServers) {
		if !shared.MCPServerRenderable(server) {
			continue
		}
		shared.WriteLine(&b, "[mcp_servers.%s]", shared.Slug(server.Name))
		writeTomlString(&b, "command", server.Command)
		if server.URL != "" {
			writeTomlString(&b, "url", server.URL)
		}
		if len(server.Args) > 0 {
			shared.WriteLine(&b, "args = %s", tomlStringArray(server.Args))
		}
		if len(server.Env) > 0 {
			shared.WriteLine(&b, "env = %s", tomlEnvInlineTable(server.Env))
		}
		b.WriteByte('\n')
	}

	shared.WriteLine(&b, "[agents.%s]", r.roleName)
	writeTomlString(&b, "description", r.description())
	writeTomlString(&b, "config_file", r.roleConfigPath)
	shared.WriteLine(&b, "nickname_candidates = %s", tomlStringArray([]string{r.agentName}))

	return strings.TrimRight(b.String(), "\n") + "\n"
}

func (r renderContext) renderRoleFile() string {
	var b strings.Builder
	writeTomlString(&b, "name", r.roleName)
	writeTomlString(&b, "description", r.description())
	shared.WriteLine(&b, "nickname_candidates = %s", tomlStringArray([]string{r.agentName}))
	writeTomlString(&b, "developer_instructions", r.developerInstructions())
	writeTomlString(&b, "model", r.input.Agent.Model.Model)
	writeTomlString(&b, "model_reasoning_effort", r.input.Agent.Model.ReasoningEffort)
	writeTomlString(&b, "approval_policy", r.input.Agent.Permissions.Approval)
	writeTomlString(&b, "sandbox_mode", r.input.Agent.Permissions.Sandbox)
	return b.String()
}

func (r renderContext) description() string {
	return shared.FirstNonEmpty(r.input.Agent.Description, "AVM agent "+r.agentName)
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
	if len(r.input.Agent.Permissions.Allow) > 0 {
		sections = append(sections, bulletSection("Allowed command guidance", shared.SortedStrings(r.input.Agent.Permissions.Allow)))
	}
	if len(r.input.Agent.Permissions.Deny) > 0 {
		sections = append(sections, bulletSection("Denied command guidance", shared.SortedStrings(r.input.Agent.Permissions.Deny)))
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
	if len(r.input.Capabilities.Hooks) > 0 {
		sections = append(sections, bulletSection("Requested AVM hooks", shared.CapabilityLines(r.input.Capabilities.Hooks)))
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

	for _, server := range shared.SortedMCPServers(r.input.Capabilities.MCPServers) {
		source := "capabilities.mcp_servers." + server.Name
		if shared.MCPServerRenderable(server) {
			mappings = append(mappings, adapter.FieldMapping{
				SourcePath: source,
				TargetPath: "mcp_servers." + shared.Slug(server.Name),
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
	for _, server := range shared.SortedMCPServers(r.input.Capabilities.MCPServers) {
		if !shared.MCPServerRenderable(server) {
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
	case adapter.OperationRemoveFile:
		if managed.MergeMode != adapter.MergeModeWholeFile {
			return fmt.Errorf("codex remove operation %q requires whole-file managed path %s", operation.ID, operation.Path)
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
		return shared.WriteFileAtomic(operation.Path, operation.Content)
	case adapter.OperationRemoveFile:
		return shared.RemoveFileAndEmptyParent(operation.Path)
	case adapter.OperationStructuredSet:
		if operation.ID == configOperationID {
			return mergeCodexConfigBlock(operation.Path, operation.ID, operation.Content)
		}
		return mergeMarkedBlock(operation.Path, operation.ID, operation.Content)
	default:
		return false, fmt.Errorf("codex adapter cannot apply %s operation %q at %s", operation.Action, operation.ID, operation.Path)
	}
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
	return shared.WriteFileAtomic(path, next)
}

func mergeCodexConfigBlock(path, operationID string, content []byte) (bool, error) {
	if operationID == "" {
		return false, fmt.Errorf("codex structured operation for %s must have an id", path)
	}

	begin := []byte("# >>> avm:codex:" + operationID)
	end := []byte("# <<< avm:codex:" + operationID)
	block := []byte(string(begin) + "\n" + strings.TrimRight(string(content), "\n") + "\n" + string(end) + "\n")

	existing, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
		existing = nil
	}

	next, err := replaceCodexConfigBlock(existing, begin, end, block)
	if err != nil {
		return false, err
	}
	return shared.WriteFileAtomic(path, next)
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

func replaceCodexConfigBlock(existing, begin, end, block []byte) ([]byte, error) {
	start, stop, found, err := markedBlockSpan(existing, begin, end)
	if err != nil {
		return nil, err
	}

	base := append([]byte(nil), existing...)
	if found {
		base = make([]byte, 0, len(existing)-stop+start)
		base = append(base, existing[:start]...)
		base = append(base, existing[stop:]...)
	}
	base = removeTopLevelProfile(base)
	return insertBeforeFirstTable(base, block), nil
}

func removeTopLevelProfile(content []byte) []byte {
	var out []byte
	for lineStart := 0; lineStart < len(content); {
		lineEnd := lineStart
		for lineEnd < len(content) && content[lineEnd] != '\n' {
			lineEnd++
		}
		nextLineStart := lineEnd
		if nextLineStart < len(content) && content[nextLineStart] == '\n' {
			nextLineStart++
		}
		lineWithNewline := content[lineStart:nextLineStart]
		line := bytes.TrimRight(content[lineStart:lineEnd], "\r")

		if isTomlTableHeader(line) {
			out = append(out, content[lineStart:]...)
			return out
		}
		if !isTopLevelProfileLine(line) {
			out = append(out, lineWithNewline...)
		}
		lineStart = nextLineStart
	}
	return out
}

func insertBeforeFirstTable(content, block []byte) []byte {
	offset := firstTomlTableOffset(content)
	if offset < 0 {
		out := append([]byte(nil), content...)
		if len(bytes.TrimSpace(out)) > 0 && !bytes.HasSuffix(out, []byte("\n")) {
			out = append(out, '\n')
		}
		if len(bytes.TrimSpace(out)) > 0 && !bytes.HasSuffix(out, []byte("\n\n")) {
			out = append(out, '\n')
		}
		out = append(out, block...)
		return out
	}

	prefix := append([]byte(nil), content[:offset]...)
	suffix := content[offset:]
	out := make([]byte, 0, len(prefix)+len(block)+len(suffix)+2)
	out = append(out, prefix...)
	if len(bytes.TrimSpace(prefix)) > 0 && !bytes.HasSuffix(out, []byte("\n")) {
		out = append(out, '\n')
	}
	if len(bytes.TrimSpace(prefix)) > 0 && !bytes.HasSuffix(out, []byte("\n\n")) {
		out = append(out, '\n')
	}
	out = append(out, block...)
	out = append(out, suffix...)
	return out
}

func firstTomlTableOffset(content []byte) int {
	for lineStart := 0; lineStart < len(content); {
		lineEnd := lineStart
		for lineEnd < len(content) && content[lineEnd] != '\n' {
			lineEnd++
		}
		line := bytes.TrimRight(content[lineStart:lineEnd], "\r")
		if isTomlTableHeader(line) {
			return lineStart
		}
		lineStart = lineEnd
		if lineStart < len(content) && content[lineStart] == '\n' {
			lineStart++
		}
	}
	return -1
}

func isTopLevelProfileLine(line []byte) bool {
	trimmed := bytes.TrimSpace(line)
	if len(trimmed) == 0 || trimmed[0] == '#' {
		return false
	}
	if !bytes.HasPrefix(trimmed, []byte("profile")) {
		return false
	}
	rest := bytes.TrimSpace(trimmed[len("profile"):])
	return len(rest) > 0 && rest[0] == '='
}

func isTomlTableHeader(line []byte) bool {
	trimmed := bytes.TrimSpace(line)
	return len(trimmed) > 0 && trimmed[0] == '['
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

	skillsDir := filepath.Join(home, skillsDirName)
	rel, err = filepath.Rel(skillsDir, target)
	if err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && filepath.Base(rel) == skillFileName && filepath.Dir(filepath.Dir(rel)) == "." {
		return nil
	}

	return fmt.Errorf("codex managed path %s is outside adapter ownership; allowed paths are %s, %s, and %s", path, configPath, filepath.Join(agentsDir, "*.toml"), filepath.Join(skillsDir, "*", "SKILL.md"))
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

func codexSkillFiles(input adapter.RenderInput, configDir string) ([]skillFile, []string) {
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

func staleCodexSkillFiles(configDir string, desired map[string]struct{}) []staleSkillFile {
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
	writeTomlLikeYAMLString(&b, "name", name)
	writeTomlLikeYAMLString(&b, "description", "AVM skill "+name+".")
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
	return name != "" && name != "." && name != ".." && !strings.ContainsAny(name, `/\`)
}

func writeTomlLikeYAMLString(builder *strings.Builder, key, value string) {
	shared.WriteLine(builder, "%s: %s", key, strconv.Quote(value))
}

func writeTomlString(builder *strings.Builder, key, value string) {
	if value == "" {
		return
	}
	shared.WriteLine(builder, "%s = %s", key, strconv.Quote(value))
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
