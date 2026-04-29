package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/xz1220/agent-vm/internal/adapter"
	"github.com/xz1220/agent-vm/internal/config"
	"gopkg.in/yaml.v3"
)

type runtimeRegistryBootstrapResult struct {
	SkillsAdded   int
	SkillsSkipped int
	MCPsAdded     int
	MCPsSkipped   int
	Warnings      []string
}

type nativeSkillSource struct {
	Name       string
	SourcePath string
}

func bootstrapRuntimeRegistry(report initImportReport) (*runtimeRegistryBootstrapResult, error) {
	result := &runtimeRegistryBootstrapResult{}
	cwd, _ := os.Getwd()
	for _, runtimeReport := range report.Runtimes {
		configDir := filepath.FromSlash(runtimeReport.ConfigDir)
		for _, skill := range scanRuntimeSkillSources(runtimeReport.Runtime, configDir, cwd) {
			added, err := importRuntimeSkill(skill)
			if err != nil {
				result.Warnings = append(result.Warnings, err.Error())
				continue
			}
			if added {
				result.SkillsAdded++
			} else {
				result.SkillsSkipped++
			}
		}
		servers, warnings := scanRuntimeMCPServers(runtimeReport.Runtime, configDir, cwd)
		result.Warnings = append(result.Warnings, warnings...)
		for _, server := range servers {
			added, err := importRuntimeMCP(runtimeReport.Runtime, server)
			if err != nil {
				result.Warnings = append(result.Warnings, err.Error())
				continue
			}
			if added {
				result.MCPsAdded++
			} else {
				result.MCPsSkipped++
			}
		}
	}
	sort.Strings(result.Warnings)
	return result, nil
}

func scanRuntimeSkillSources(runtimeName, configDir, cwd string) []nativeSkillSource {
	var roots []string
	home, _ := os.UserHomeDir()
	switch runtimeName {
	case "codex":
		roots = append(roots, filepath.Join(configDir, "skills"))
		if home != "" {
			roots = append(roots, filepath.Join(home, ".codex", "skills"))
		}
	case "claude-code":
		roots = append(roots, filepath.Join(configDir, "skills"))
		if home != "" {
			roots = append(roots, filepath.Join(home, ".claude", "skills"))
		}
		if cwd != "" {
			roots = append(roots, filepath.Join(cwd, ".claude", "skills"))
		}
	case "opencode":
		roots = append(roots, filepath.Join(configDir, "skills"))
		if home != "" {
			roots = append(roots, filepath.Join(home, ".config", "opencode", "skills"))
		}
		if cwd != "" {
			roots = append(roots, filepath.Join(cwd, ".opencode", "skills"))
		}
	}

	seen := map[string]struct{}{}
	var out []nativeSkillSource
	for _, root := range roots {
		for _, skill := range scanSkillRoot(root) {
			key := skill.Name + "\x00" + skill.SourcePath
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, skill)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].SourcePath < out[j].SourcePath
	})
	return out
}

func scanSkillRoot(root string) []nativeSkillSource {
	if root == "" || pathInsideDir(root, config.AvmDir()) {
		return nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	var out []nativeSkillSource
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := safeCreateName(entry.Name())
		if name == "" {
			continue
		}
		sourcePath := filepath.Join(root, entry.Name(), "SKILL.md")
		if _, err := os.Stat(sourcePath); err != nil {
			continue
		}
		if pathInsideDir(sourcePath, config.AvmDir()) {
			continue
		}
		out = append(out, nativeSkillSource{Name: name, SourcePath: sourcePath})
	}
	return out
}

func importRuntimeSkill(skill nativeSkillSource) (bool, error) {
	target := config.SkillRegistryFilePath(skill.Name)
	if _, err := os.Stat(target); err == nil {
		return false, nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	raw, err := os.ReadFile(skill.SourcePath)
	if err != nil {
		return false, fmt.Errorf("import skill %q from %s: %w", skill.Name, skill.SourcePath, err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return false, err
	}
	if err := os.WriteFile(target, raw, 0o600); err != nil {
		return false, err
	}
	return true, nil
}

func scanRuntimeMCPServers(runtimeName, configDir, cwd string) ([]adapter.MCPServer, []string) {
	home, _ := os.UserHomeDir()
	switch runtimeName {
	case "codex":
		var scans []mcpScanResult
		scans = append(scans, mcpScanResultFrom(scanCodexMCPServers(filepath.Join(configDir, "config.toml"))))
		if home != "" {
			scans = append(scans, mcpScanResultFrom(scanCodexMCPServers(filepath.Join(home, ".codex", "config.toml"))))
		}
		return mergeMCPScanResults(scans...)
	case "claude-code":
		var scans []mcpScanResult
		scans = append(scans, mcpScanResultFrom(scanJSONMCPServers(filepath.Join(configDir, "mcp.json"), "mcpServers")))
		if home != "" {
			scans = append(scans, mcpScanResultFrom(scanJSONMCPServers(filepath.Join(home, ".claude", "mcp.json"), "mcpServers")))
		}
		return mergeMCPScanResults(scans...)
	case "opencode":
		var scans []mcpScanResult
		scans = append(scans, mcpScanResultFrom(scanJSONMCPServers(filepath.Join(configDir, "opencode.json"), "mcp")))
		if home != "" {
			scans = append(scans, mcpScanResultFrom(scanJSONMCPServers(filepath.Join(home, ".config", "opencode", "opencode.json"), "mcp")))
		}
		return mergeMCPScanResults(scans...)
	case "cursor":
		if cwd != "" {
			return scanJSONMCPServers(filepath.Join(cwd, ".cursor", "mcp.json"), "mcpServers")
		}
	}
	return nil, nil
}

type mcpScanResult struct {
	Servers  []adapter.MCPServer
	Warnings []string
}

func mcpScanResultFrom(servers []adapter.MCPServer, warnings []string) mcpScanResult {
	return mcpScanResult{Servers: servers, Warnings: warnings}
}

func mergeMCPScanResults(scans ...mcpScanResult) ([]adapter.MCPServer, []string) {
	byName := map[string]adapter.MCPServer{}
	var warnings []string
	for _, scan := range scans {
		warnings = append(warnings, scan.Warnings...)
		for _, server := range scan.Servers {
			name := safeCreateName(server.Name)
			if name == "" {
				continue
			}
			if _, exists := byName[name]; exists {
				continue
			}
			server.Name = name
			byName[name] = server
		}
	}
	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)
	servers := make([]adapter.MCPServer, 0, len(names))
	for _, name := range names {
		servers = append(servers, byName[name])
	}
	sort.Strings(warnings)
	return servers, warnings
}

func importRuntimeMCP(runtimeName string, server adapter.MCPServer) (bool, error) {
	name := safeCreateName(server.Name)
	if name == "" || (server.Command == "" && server.URL == "") {
		return false, nil
	}
	target := config.MCPRegistryPath(name)
	if _, err := os.Stat(target); err == nil {
		return false, nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	entry := config.MCPRegistryEntry{
		Name:   name,
		Kind:   "mcp",
		Source: "runtime:" + runtimeName,
		Server: config.MCPServerConfig{
			Type:    serverType(server),
			Command: server.Command,
			Args:    append([]string(nil), server.Args...),
			Env:     envVarMapConfig(server.Env),
			URL:     server.URL,
			Headers: envVarMapConfig(server.Headers),
		},
	}
	raw, err := yaml.Marshal(entry)
	if err != nil {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
		return false, err
	}
	if err := os.WriteFile(target, raw, 0o600); err != nil {
		return false, err
	}
	return true, nil
}

func serverType(server adapter.MCPServer) string {
	if server.URL != "" {
		return "remote"
	}
	return "stdio"
}

func envVarMapConfig(values []adapter.EnvVar) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for _, value := range values {
		if value.Name != "" {
			out[value.Name] = value.Value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func scanJSONMCPServers(path, key string) ([]adapter.MCPServer, []string) {
	if path == "" || pathInsideDir(path, config.AvmDir()) {
		return nil, nil
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, []string{fmt.Sprintf("read MCP config %s: %v", path, err)}
	}
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, []string{fmt.Sprintf("parse MCP config %s: %v", path, err)}
	}
	rawServers, ok := root[key].(map[string]any)
	if !ok {
		return nil, nil
	}
	servers := make([]adapter.MCPServer, 0, len(rawServers))
	for name, value := range rawServers {
		cfg, ok := value.(map[string]any)
		if !ok {
			continue
		}
		server := mcpServerFromObject(name, cfg)
		if server.Command == "" && server.URL == "" {
			continue
		}
		servers = append(servers, server)
	}
	sort.SliceStable(servers, func(i, j int) bool {
		return servers[i].Name < servers[j].Name
	})
	return servers, nil
}

func mcpServerFromObject(name string, cfg map[string]any) adapter.MCPServer {
	server := adapter.MCPServer{Name: name}
	if command, ok := cfg["command"].(string); ok {
		server.Command = command
	} else if commandValues, ok := cfg["command"].([]any); ok && len(commandValues) > 0 {
		if command, ok := commandValues[0].(string); ok {
			server.Command = command
			for _, arg := range commandValues[1:] {
				if value, ok := arg.(string); ok {
					server.Args = append(server.Args, value)
				}
			}
		}
	}
	if args, ok := cfg["args"].([]any); ok {
		server.Args = append(server.Args, stringValues(args)...)
	}
	if url, ok := cfg["url"].(string); ok {
		server.URL = url
	}
	if env, ok := cfg["env"].(map[string]any); ok {
		server.Env = envVarsFromAnyMap(env)
	}
	if env, ok := cfg["environment"].(map[string]any); ok {
		server.Env = envVarsFromAnyMap(env)
	}
	if headers, ok := cfg["headers"].(map[string]any); ok {
		server.Headers = envVarsFromAnyMap(headers)
	}
	return server
}

func stringValues(values []any) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if text, ok := value.(string); ok {
			out = append(out, text)
		}
	}
	return out
}

func envVarsFromAnyMap(values map[string]any) []adapter.EnvVar {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]adapter.EnvVar, 0, len(keys))
	for _, key := range keys {
		if value, ok := values[key].(string); ok {
			out = append(out, adapter.EnvVar{Name: key, Value: value})
		}
	}
	return out
}

var codexMCPSectionPattern = regexp.MustCompile(`^\s*\[mcp_servers\.(?:"([^"]+)"|([A-Za-z0-9_-]+))\]\s*$`)

func scanCodexMCPServers(path string) ([]adapter.MCPServer, []string) {
	if path == "" || pathInsideDir(path, config.AvmDir()) {
		return nil, nil
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, []string{fmt.Sprintf("read Codex config %s: %v", path, err)}
	}
	servers := map[string]*adapter.MCPServer{}
	current := ""
	for _, line := range strings.Split(string(raw), "\n") {
		if match := codexMCPSectionPattern.FindStringSubmatch(line); match != nil {
			current = firstNonEmptyString(match[1], match[2])
			if current != "" {
				servers[current] = &adapter.MCPServer{Name: current}
			}
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(line), "[") {
			current = ""
			continue
		}
		if current == "" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(strings.SplitN(value, "#", 2)[0])
		server := servers[current]
		switch key {
		case "command":
			server.Command = parseQuotedString(value)
		case "url":
			server.URL = parseQuotedString(value)
		case "args":
			server.Args = parseQuotedStringArray(value)
		case "env":
			server.Env = envVarsFromStringMap(parseTomlInlineStringMap(value))
		}
	}
	var out []adapter.MCPServer
	for _, server := range servers {
		if server.Command == "" && server.URL == "" {
			continue
		}
		out = append(out, *server)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func parseQuotedString(value string) string {
	out, err := strconv.Unquote(strings.TrimSpace(value))
	if err != nil {
		return ""
	}
	return out
}

func parseQuotedStringArray(value string) []string {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "[") || !strings.HasSuffix(value, "]") {
		return nil
	}
	var out []string
	for _, token := range splitTomlCommaValues(strings.TrimSuffix(strings.TrimPrefix(value, "["), "]")) {
		if parsed := parseQuotedString(token); parsed != "" {
			out = append(out, parsed)
		}
	}
	return out
}

func parseTomlInlineStringMap(value string) map[string]string {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "{") || !strings.HasSuffix(value, "}") {
		return nil
	}
	out := map[string]string{}
	for _, token := range splitTomlCommaValues(strings.TrimSuffix(strings.TrimPrefix(value, "{"), "}")) {
		key, rawValue, ok := strings.Cut(token, "=")
		if !ok {
			continue
		}
		key = strings.Trim(strings.TrimSpace(key), `"`)
		parsed := parseQuotedString(rawValue)
		if key != "" && parsed != "" {
			out[key] = parsed
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func splitTomlCommaValues(value string) []string {
	var out []string
	var b strings.Builder
	inQuote := false
	escaped := false
	for _, r := range value {
		switch {
		case escaped:
			b.WriteRune(r)
			escaped = false
		case r == '\\' && inQuote:
			b.WriteRune(r)
			escaped = true
		case r == '"':
			b.WriteRune(r)
			inQuote = !inQuote
		case r == ',' && !inQuote:
			if token := strings.TrimSpace(b.String()); token != "" {
				out = append(out, token)
			}
			b.Reset()
		default:
			b.WriteRune(r)
		}
	}
	if token := strings.TrimSpace(b.String()); token != "" {
		out = append(out, token)
	}
	return out
}

func envVarsFromStringMap(values map[string]string) []adapter.EnvVar {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]adapter.EnvVar, 0, len(keys))
	for _, key := range keys {
		out = append(out, adapter.EnvVar{Name: key, Value: values[key]})
	}
	return out
}

func pathInsideDir(path, dir string) bool {
	if strings.TrimSpace(path) == "" || strings.TrimSpace(dir) == "" {
		return false
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absDir, absPath)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
