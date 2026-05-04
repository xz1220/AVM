// Package opencode renders AVM agents into OpenCode configuration files.
package opencode

import (
	"bytes"
	"context"
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
	runtimeName       = "opencode"
	binaryName        = "opencode"
	configFileName    = "opencode.json"
	agentsDirName     = "agents"
	skillsDirName     = "skills"
	skillFileName     = "SKILL.md"
	configOperationID = "opencode-config"
	agentOperationID  = "opencode-agent"
	skillOperationID  = "opencode-skill"
	avmManagedKey     = "avm_managed"
)

// Adapter renders OpenCode config into an isolated AVM-owned runtime home.
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
	configDir := a.opencodeDir()
	configPath := a.opencodeConfigPath(configDir)

	found := false
	for _, path := range []string{
		configDir,
		configPath,
		filepath.Join(configDir, agentsDirName),
		filepath.Join(configDir, skillsDirName),
	} {
		if _, err := os.Stat(path); err == nil {
			found = true
			break
		}
	}

	version := ""
	if _, err := exec.LookPath(binaryName); err == nil {
		found = true
	}

	return adapter.Detection{
		Runtime:   runtimeName,
		Found:     found,
		Version:   version,
		ConfigDir: filepath.ToSlash(configDir),
	}
}

func (a *Adapter) Plan(ctx adapter.Context, input adapter.RenderInput) (*adapter.RenderPlan, error) {
	_ = ctx

	runtime := input.Runtime
	if runtime == "" {
		runtime = runtimeName
	}
	if runtime != runtimeName {
		return nil, fmt.Errorf("opencode adapter cannot plan runtime %q", runtime)
	}

	agentName := shared.FirstNonEmpty(input.Agent.Name, input.Active.Name, "agent")
	agentFileName := shared.Slug(agentName)
	configDir := a.opencodeDirForInput(input)
	configPath := filepath.ToSlash(filepath.Join(configDir, configFileName))
	agentPath := filepath.ToSlash(filepath.Join(configDir, agentsDirName, agentFileName+".md"))
	skillFiles, skillWarnings := opencodeSkillFiles(input, configDir)
	staleSkillFiles := staleOpenCodeSkillFiles(configDir, skillFileNames(skillFiles))

	render := renderContext{
		input:         input,
		agentName:     agentName,
		agentFileName: agentFileName,
		configPath:    configPath,
		agentPath:     agentPath,
	}

	managedPaths := []adapter.ManagedPath{
		{
			Path:        configPath,
			Owner:       "avm",
			Description: "OpenCode JSON config rendered into an isolated AVM-owned runtime home.",
			Required:    true,
			MergeMode:   adapter.MergeModeWholeFile,
		},
		{
			Path:        agentPath,
			Owner:       "avm",
			Description: "OpenCode agent markdown rendered from the AVM agent profile.",
			Required:    true,
			MergeMode:   adapter.MergeModeWholeFile,
		},
	}
	operations := []adapter.RenderOperation{
		{
			ID:          configOperationID,
			Action:      adapter.OperationWriteFile,
			Path:        configPath,
			Content:     []byte(render.renderConfigFile()),
			Description: "write OpenCode AVM-managed config file",
			Required:    true,
		},
		{
			ID:          agentOperationID,
			Action:      adapter.OperationWriteFile,
			Path:        agentPath,
			Content:     []byte(render.renderAgentFile()),
			Description: "write OpenCode AVM-managed agent file",
			Required:    true,
		},
	}

	for _, skillFile := range skillFiles {
		managedPaths = append(managedPaths, adapter.ManagedPath{
			Path:        skillFile.target,
			Owner:       "avm",
			Description: "OpenCode skill file rendered from the AVM active skill set.",
			Required:    true,
			MergeMode:   adapter.MergeModeWholeFile,
		})
		operations = append(operations, adapter.RenderOperation{
			ID:          skillOperationID + "-" + shared.Slug(skillFile.name),
			Action:      adapter.OperationWriteFile,
			Path:        skillFile.target,
			Content:     skillFile.content,
			Description: "write OpenCode skill file from AVM active skill",
			Required:    true,
		})
	}
	for _, stale := range staleSkillFiles {
		managedPaths = append(managedPaths, adapter.ManagedPath{
			Path:        stale.target,
			Owner:       "avm",
			Description: "Stale OpenCode AVM-managed skill file removed because it is not in the current active skill set.",
			Required:    false,
			MergeMode:   adapter.MergeModeWholeFile,
		})
		operations = append(operations, adapter.RenderOperation{
			ID:          skillOperationID + "-remove-" + shared.Slug(stale.name),
			Action:      adapter.OperationRemoveFile,
			Path:        stale.target,
			Description: "remove stale OpenCode AVM-managed skill file",
			Required:    false,
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
		return nil, fmt.Errorf("opencode adapter render plan is nil")
	}
	if normalized.Runtime != "" && normalized.Runtime != runtimeName {
		return nil, fmt.Errorf("opencode adapter cannot render runtime %q", normalized.Runtime)
	}

	managed, err := managedPathIndex(normalized.ManagedPaths, opencodeDirFromPlan(normalized, a.opencodeDir()))
	if err != nil {
		return nil, err
	}

	pending := make([]plannedOperation, 0, len(normalized.Operations))
	for _, operation := range normalized.Operations {
		cleanPath, err := cleanRenderPath(operation.Path)
		if err != nil {
			return nil, fmt.Errorf("opencode render operation %q has invalid path %q: %w", operation.ID, operation.Path, err)
		}
		operation.Path = cleanPath

		managedPath, ok := managed[operation.Path]
		if !ok {
			return nil, fmt.Errorf("opencode render operation %q targets unmanaged path %s", operation.ID, operation.Path)
		}
		if err := validateOperation(operation, managedPath); err != nil {
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

func (a *Adapter) opencodeDir() string {
	if a.configDir != "" {
		return a.configDir
	}
	if value := os.Getenv("OPENCODE_CONFIG_DIR"); value != "" {
		return value
	}
	if value := os.Getenv("OPENCODE_CONFIG"); value != "" {
		return filepath.Dir(value)
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".config", "opencode")
	}
	return filepath.Join(".config", "opencode")
}

func (a *Adapter) opencodeDirForInput(input adapter.RenderInput) string {
	if input.Boundary.Paths != nil {
		if configDir := input.Boundary.Paths["config_dir"]; configDir != "" {
			return configDir
		}
	}
	if input.RuntimeHome != "" {
		return input.RuntimeHome
	}
	return a.opencodeDir()
}

func (a *Adapter) opencodeConfigPath(configDir string) string {
	if a.configDir == "" {
		if value := os.Getenv("OPENCODE_CONFIG"); value != "" {
			return value
		}
	}
	return filepath.Join(configDir, configFileName)
}

func opencodeDirFromPlan(plan *adapter.RenderPlan, fallback string) string {
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

func (a *Adapter) defaultProjectRoot() string {
	if a.projectRoot != "" {
		return a.projectRoot
	}
	if cwd, err := os.Getwd(); err == nil && cwd != "" {
		return cwd
	}
	return "."
}

func opencodeVersion(ctx context.Context, path string) string {
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
	configPath    string
	agentPath     string
}

func (r renderContext) renderConfigFile() string {
	root := map[string]any{
		"$schema":       "https://opencode.ai/config.json",
		"default_agent": r.agentFileName,
	}
	if r.input.Agent.Model.Model != "" {
		root["model"] = r.input.Agent.Model.Model
	}
	if permission := opencodePermission(r.input.Agent.Permissions); len(permission) > 0 {
		root["permission"] = permission
	}
	if mcp := r.renderMCPServers(); len(mcp) > 0 {
		root["mcp"] = mcp
	}

	data, err := shared.MarshalJSON(root)
	if err != nil {
		return "{\n  \"$schema\": \"https://opencode.ai/config.json\",\n  \"default_agent\": " + strconv.Quote(r.agentFileName) + "\n}\n"
	}
	return string(data)
}

func (r renderContext) renderAgentFile() string {
	var b strings.Builder
	b.WriteString("---\n")
	writeYAMLString(&b, "description", r.description())
	writeYAMLString(&b, "mode", "primary")
	writeYAMLString(&b, "model", r.input.Agent.Model.Model)
	if r.input.Agent.Model.Temperature != nil {
		shared.WriteLine(&b, "temperature: %s", strconv.FormatFloat(*r.input.Agent.Model.Temperature, 'f', -1, 64))
	}
	writeYAMLPermission(&b, "permission", opencodePermission(r.input.Agent.Permissions))
	b.WriteString("---\n\n")
	b.WriteString(r.agentInstructions())
	b.WriteByte('\n')
	return b.String()
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
		sections = append(sections, bulletSection("Active AVM skills", shared.CapabilityLines(r.input.Capabilities.Skills)))
	}
	if r.input.Agent.Model.ReasoningEffort != "" {
		sections = append(sections, section("Reasoning effort", r.input.Agent.Model.ReasoningEffort))
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
	if len(sections) == 0 {
		return "Follow the AVM agent profile for " + r.agentName + "."
	}
	return strings.Join(sections, "\n\n")
}

func (r renderContext) renderMCPServers() map[string]opencodeMCPServer {
	servers := make(map[string]opencodeMCPServer)
	for _, server := range r.renderableMCPServers() {
		servers[server.Name] = opencodeMCPServerFromAdapter(server)
	}
	return servers
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

func (r renderContext) mappings() []adapter.FieldMapping {
	targetConfig := r.configPath
	targetAgent := r.agentPath
	targetBody := targetAgent + "#body"
	targetFrontmatter := targetAgent + "#frontmatter"
	mappings := []adapter.FieldMapping{
		{SourcePath: "active", TargetPath: targetConfig + "#default_agent", Status: adapter.MappingNative},
		{SourcePath: "agent.name", TargetPath: targetConfig + "#default_agent", Status: adapter.MappingNative},
		{SourcePath: "agent.description", TargetPath: targetFrontmatter + ".description", Status: adapter.MappingNative},
		{SourcePath: "agent.instructions.system", TargetPath: targetBody, Status: adapter.MappingNative},
		{SourcePath: "agent.instructions.developer", TargetPath: targetBody, Status: adapter.MappingNative},
		{SourcePath: "agent.model.model", TargetPath: targetFrontmatter + ".model", Status: adapter.MappingNative},
		{SourcePath: "agent.permissions.approval", TargetPath: targetFrontmatter + ".permission", Status: adapter.MappingNative},
		{SourcePath: "agent.permissions.sandbox", TargetPath: targetFrontmatter + ".permission", Status: adapter.MappingNative},
		{SourcePath: "agent.permissions.allow", TargetPath: targetFrontmatter + ".permission", Status: adapter.MappingNative},
		{SourcePath: "agent.permissions.deny", TargetPath: targetFrontmatter + ".permission", Status: adapter.MappingNative},
		{SourcePath: "agent.permissions.additional_directories", TargetPath: targetFrontmatter + ".permission.external_directory", Status: adapter.MappingNative},
		{SourcePath: "project.AGENTS.md", Status: adapter.MappingIgnored, Reason: "OpenCode project instructions are user-owned; the OpenCode adapter does not overwrite AGENTS.md."},
	}

	if r.input.Agent.Model.ReasoningEffort != "" {
		mappings = append(mappings, adapter.FieldMapping{
			SourcePath: "agent.model.reasoning_effort",
			TargetPath: targetBody,
			Status:     adapter.MappingRenderedAsInstructions,
			Reason:     "OpenCode agent config has no AVM reasoning effort field; it is preserved as agent guidance.",
		})
	}
	if r.input.Agent.Model.Verbosity != "" {
		mappings = append(mappings, adapter.FieldMapping{
			SourcePath: "agent.model.verbosity",
			TargetPath: targetBody,
			Status:     adapter.MappingRenderedAsInstructions,
			Reason:     "OpenCode agent config has no AVM verbosity field; it is preserved as agent guidance.",
		})
	}
	if r.input.Agent.Model.Temperature != nil {
		mappings = append(mappings, adapter.FieldMapping{
			SourcePath: "agent.model.temperature",
			TargetPath: targetFrontmatter + ".temperature",
			Status:     adapter.MappingNative,
		})
	}
	if len(r.input.Agent.Instructions.References) > 0 {
		mappings = append(mappings, adapter.FieldMapping{
			SourcePath: "agent.instructions.references",
			TargetPath: targetBody,
			Status:     adapter.MappingRenderedAsInstructions,
			Reason:     "OpenCode agent files do not have a separate AVM references field.",
		})
	}
	if len(r.input.Capabilities.Skills) > 0 {
		status := adapter.MappingNative
		targetPath := filepath.ToSlash(filepath.Join(filepath.Dir(filepath.Dir(targetAgent)), skillsDirName))
		reason := ""
		if !allSkillsHavePaths(r.input.Capabilities.Skills) {
			status = adapter.MappingRenderedAsInstructions
			targetPath = targetBody
			reason = "OpenCode skills require SKILL.md sources; unresolved AVM skill names are preserved as instructions."
		}
		mappings = append(mappings, adapter.FieldMapping{
			SourcePath: "capabilities.skills",
			TargetPath: targetPath,
			Status:     status,
			Reason:     reason,
		})
	}
	if len(r.input.Capabilities.Commands) > 0 {
		mappings = append(mappings, adapter.FieldMapping{
			SourcePath: "capabilities.commands",
			TargetPath: targetBody,
			Status:     adapter.MappingRenderedAsInstructions,
			Reason:     "OpenCode adapter Phase 1 preserves AVM command capability names as instructions only.",
		})
	}
	if len(r.input.Capabilities.Hooks) > 0 {
		mappings = append(mappings, adapter.FieldMapping{
			SourcePath: "capabilities.hooks",
			TargetPath: targetBody,
			Status:     adapter.MappingRenderedAsInstructions,
			Reason:     "OpenCode adapter Phase 1 does not install AVM hooks.",
		})
	}
	if len(r.input.Capabilities.Toolsets) > 0 {
		mappings = append(mappings, adapter.FieldMapping{
			SourcePath: "capabilities.toolsets",
			TargetPath: targetBody,
			Status:     adapter.MappingRenderedAsInstructions,
			Reason:     "OpenCode adapter Phase 1 does not enforce AVM toolset modes natively.",
		})
	}

	for _, server := range shared.SortedMCPServers(r.input.Capabilities.MCPServers) {
		source := "capabilities.mcp_servers." + server.Name
		if shared.MCPServerRenderable(server) {
			mappings = append(mappings, adapter.FieldMapping{
				SourcePath: source,
				TargetPath: targetConfig + "#mcp." + server.Name,
				Status:     adapter.MappingNative,
			})
			continue
		}
		mappings = append(mappings, adapter.FieldMapping{
			SourcePath: source,
			Status:     adapter.MappingUnsupported,
			Reason:     "OpenCode MCP rendering requires command or URL.",
		})
	}
	return mappings
}

func (r renderContext) warnings() []string {
	var warnings []string
	for _, server := range shared.SortedMCPServers(r.input.Capabilities.MCPServers) {
		if !shared.MCPServerRenderable(server) {
			warnings = append(warnings, fmt.Sprintf("mcp server %q was not rendered because command or URL is missing", server.Name))
			continue
		}
		if server.URL != "" && len(server.Env) > 0 && len(server.Headers) == 0 {
			warnings = append(warnings, fmt.Sprintf("mcp server %q has env values but OpenCode remote MCP uses headers; env values were not rendered", server.Name))
		}
	}
	return warnings
}

type opencodeMCPServer struct {
	Type        string            `json:"type"`
	Command     []string          `json:"command,omitempty"`
	Environment map[string]string `json:"environment,omitempty"`
	URL         string            `json:"url,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Enabled     bool              `json:"enabled"`
}

func opencodeMCPServerFromAdapter(server adapter.MCPServer) opencodeMCPServer {
	if server.URL != "" {
		config := opencodeMCPServer{
			Type:    "remote",
			URL:     server.URL,
			Headers: envVarMap(server.Headers),
			Enabled: true,
		}
		if len(config.Headers) == 0 {
			config.Headers = nil
		}
		return config
	}
	command := []string{server.Command}
	command = append(command, server.Args...)
	config := opencodeMCPServer{
		Type:        "local",
		Command:     command,
		Environment: envVarMap(server.Env),
		Enabled:     true,
	}
	if len(config.Environment) == 0 {
		config.Environment = nil
	}
	return config
}

func envVarMap(values []adapter.EnvVar) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for _, value := range values {
		if value.Name != "" {
			out[value.Name] = value.Value
		}
	}
	return out
}

func opencodePermission(perm adapter.PermissionConfig) map[string]any {
	root := make(map[string]any)
	defaultAction := approvalAction(perm.Approval)
	bash := map[string]string{"*": defaultAction}

	switch perm.Sandbox {
	case "read-only":
		root["edit"] = "deny"
		if bash["*"] == "allow" {
			bash["*"] = "ask"
		}
	case "danger-full-access":
		root["edit"] = "allow"
	default:
		root["edit"] = "allow"
	}

	for _, value := range perm.Allow {
		applyPermissionPattern(root, bash, value, "allow")
	}
	for _, value := range perm.Deny {
		applyPermissionPattern(root, bash, value, "deny")
	}
	if len(perm.AdditionalDirectories) > 0 {
		external := make(map[string]string, len(perm.AdditionalDirectories))
		for _, dir := range perm.AdditionalDirectories {
			if strings.TrimSpace(dir) != "" {
				external[dir] = "allow"
			}
		}
		if len(external) > 0 {
			root["external_directory"] = external
		}
	}
	if len(bash) > 0 {
		root["bash"] = bash
	}
	return root
}

func approvalAction(value string) string {
	switch value {
	case "never":
		return "allow"
	case "prompt", "on-request", "on-risky-actions", "untrusted":
		return "ask"
	default:
		return "ask"
	}
}

func applyPermissionPattern(root map[string]any, bash map[string]string, value, action string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	if pattern, ok := bashPattern(value); ok {
		bash[pattern] = action
		return
	}
	key := opencodePermissionKey(value)
	if key == "bash" {
		bash["*"] = action
		return
	}
	if key != "" {
		root[key] = action
		return
	}
	bash[value] = action
}

func bashPattern(value string) (string, bool) {
	if strings.HasPrefix(value, "Bash(") && strings.HasSuffix(value, ")") {
		pattern := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(value, "Bash("), ")"))
		return pattern, pattern != ""
	}
	return "", false
}

func opencodePermissionKey(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.ReplaceAll(normalized, "_", "-")
	switch normalized {
	case "read":
		return "read"
	case "edit", "write", "apply-patch", "apply_patch":
		return "edit"
	case "bash", "shell":
		return "bash"
	case "glob":
		return "glob"
	case "grep":
		return "grep"
	case "list", "ls":
		return "list"
	case "task":
		return "task"
	case "webfetch", "web-fetch":
		return "webfetch"
	case "websearch", "web-search":
		return "websearch"
	case "skill":
		return "skill"
	case "lsp":
		return "lsp"
	case "external-directory", "external_directory":
		return "external_directory"
	default:
		return ""
	}
}

type plannedOperation struct {
	operation adapter.RenderOperation
	managed   adapter.ManagedPath
}

func validateOperation(operation adapter.RenderOperation, managed adapter.ManagedPath) error {
	switch operation.Action {
	case adapter.OperationWriteFile, adapter.OperationRemoveFile:
		if managed.MergeMode != adapter.MergeModeWholeFile {
			return fmt.Errorf("opencode %s operation %q requires whole-file managed path %s", operation.Action, operation.ID, operation.Path)
		}
	default:
		return fmt.Errorf("opencode adapter cannot apply %s operation %q at %s", operation.Action, operation.ID, operation.Path)
	}
	return nil
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
	default:
		return false, fmt.Errorf("opencode adapter cannot apply %s operation %q at %s", operation.Action, operation.ID, operation.Path)
	}
}

func managedPathIndex(paths []adapter.ManagedPath, configDir string) (map[string]adapter.ManagedPath, error) {
	managed := make(map[string]adapter.ManagedPath, len(paths))
	for _, path := range paths {
		cleanPath, err := cleanRenderPath(path.Path)
		if err != nil {
			return nil, fmt.Errorf("opencode managed path %q is invalid: %w", path.Path, err)
		}
		if err := validateOpenCodeManagedPath(cleanPath, configDir); err != nil {
			return nil, err
		}
		if _, exists := managed[cleanPath]; exists {
			return nil, fmt.Errorf("opencode managed path %s declared more than once", cleanPath)
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

func validateOpenCodeManagedPath(path, configDir string) error {
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
	if err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && filepath.Dir(rel) == "." && filepath.Ext(rel) == ".md" {
		return nil
	}

	skillsDir := filepath.Join(home, skillsDirName)
	rel, err = filepath.Rel(skillsDir, target)
	if err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && filepath.Base(rel) == skillFileName && filepath.Dir(filepath.Dir(rel)) == "." {
		return nil
	}

	return fmt.Errorf("opencode managed path %s is outside adapter ownership; allowed paths are %s, %s, and %s", path, configPath, filepath.Join(agentsDir, "*.md"), filepath.Join(skillsDir, "*", "SKILL.md"))
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

func opencodeSkillFiles(input adapter.RenderInput, configDir string) ([]skillFile, []string) {
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

func staleOpenCodeSkillFiles(configDir string, desired map[string]struct{}) []staleSkillFile {
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

func allSkillsHavePaths(refs []adapter.CapabilityRef) bool {
	if len(refs) == 0 {
		return false
	}
	for _, ref := range refs {
		if ref.Name == "" || ref.Path == "" {
			return false
		}
	}
	return true
}

func writeYAMLString(builder *strings.Builder, key, value string) {
	if value == "" {
		return
	}
	shared.WriteLine(builder, "%s: %s", key, strconv.Quote(value))
}

func writeYAMLPermission(builder *strings.Builder, key string, permission map[string]any) {
	if len(permission) == 0 {
		return
	}
	shared.WriteLine(builder, "%s:", key)
	keys := make([]string, 0, len(permission))
	for name := range permission {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	for _, name := range keys {
		switch value := permission[name].(type) {
		case string:
			shared.WriteLine(builder, "  %s: %s", name, value)
		case map[string]string:
			if len(value) == 0 {
				continue
			}
			shared.WriteLine(builder, "  %s:", name)
			patterns := make([]string, 0, len(value))
			for pattern := range value {
				patterns = append(patterns, pattern)
			}
			sort.Strings(patterns)
			for _, pattern := range patterns {
				shared.WriteLine(builder, "    %s: %s", strconv.Quote(pattern), value[pattern])
			}
		}
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
