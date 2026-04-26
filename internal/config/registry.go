package config

import (
	"os"
	"path/filepath"
	"strings"
)

func SkillRegistryPath(name string) string {
	return filepath.Join(RegistryKindDir("skills"), name)
}

func SkillRegistryFilePath(name string) string {
	return filepath.Join(SkillRegistryPath(name), "SKILL.md")
}

func MCPRegistryPath(name string) string {
	return filepath.Join(RegistryKindDir("mcps"), name+".yaml")
}

func resolvedSkill(name string) (ResolvedSkill, string, error) {
	if !validName(name) {
		return ResolvedSkill{Name: name}, "", fieldError("", "name", "invalid name %q", name)
	}

	sourceDir := SkillRegistryPath(name)
	sourcePath := filepath.Join(sourceDir, "SKILL.md")
	if _, err := os.Stat(sourcePath); err != nil {
		return ResolvedSkill{Name: name}, sourcePath, err
	}
	return ResolvedSkill{
		Name:       name,
		SourceDir:  sourceDir,
		SourcePath: sourcePath,
	}, sourcePath, nil
}

func ReadMCPRegistryEntry(name string) (*MCPRegistryEntry, string, error) {
	if !validName(name) {
		return nil, "", fieldError("", "name", "invalid name %q", name)
	}

	path := MCPRegistryPath(name)
	var entry MCPRegistryEntry
	if err := readYAML(path, &entry); err != nil {
		return nil, path, err
	}
	if strings.TrimSpace(entry.Name) == "" {
		entry.Name = name
	}
	if entry.Name != name {
		return nil, path, fieldError(path, "name", "expected %q, got %q", name, entry.Name)
	}
	if entry.Kind != "" && entry.Kind != "mcp" {
		return nil, path, fieldError(path, "kind", "expected %q, got %q", "mcp", entry.Kind)
	}
	return &entry, path, nil
}

func resolvedMCPServer(name string) (ResolvedMCPServer, string, error) {
	entry, path, err := ReadMCPRegistryEntry(name)
	if err != nil {
		if os.IsNotExist(err) {
			return ResolvedMCPServer{Name: name}, path, err
		}
		return ResolvedMCPServer{Name: name}, path, err
	}
	return ResolvedMCPServer{
		Name:       entry.Name,
		Type:       entry.Server.Type,
		Command:    entry.Server.Command,
		Args:       cloneStringSlice(entry.Server.Args),
		Env:        cloneStringMap(entry.Server.Env),
		URL:        entry.Server.URL,
		Headers:    cloneStringMap(entry.Server.Headers),
		SourcePath: path,
	}, path, nil
}
