package packageio

import (
	"archive/zip"
	"bytes"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/xz1220/agent-vm/internal/config"
	"gopkg.in/yaml.v3"
)

type ExportOptions struct {
	Name       string
	Kind       string
	OutputPath string
	CWD        string
	Now        time.Time
}

type ExportResult struct {
	Manifest Manifest
	Output   string
	Warnings []string
}

type packageBuilder struct {
	cwd      string
	files    map[string][]byte
	agents   map[string]struct{}
	envs     map[string]struct{}
	caps     capabilitySets
	warnings []string
}

type capabilitySets struct {
	skills   map[string]struct{}
	mcps     map[string]struct{}
	commands map[string]struct{}
	hooks    map[string]struct{}
	toolsets map[string]struct{}
}

func ExportPackage(opts ExportOptions) (*ExportResult, error) {
	name := strings.TrimSpace(opts.Name)
	if name == "" {
		return nil, fmt.Errorf("package export target is required")
	}
	if opts.OutputPath == "" {
		return nil, fmt.Errorf("package export output path is required")
	}
	cwd, err := cwdOrCurrent(opts.CWD)
	if err != nil {
		return nil, err
	}
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}

	builder := newPackageBuilder(cwd)
	kind, err := builder.addRoot(opts.Kind, name)
	if err != nil {
		return nil, err
	}

	manifest := Manifest{
		Version:      ManifestVersion,
		ExportedAt:   now.UTC().Format(time.RFC3339),
		Kind:         kind,
		Name:         name,
		Agents:       sortedSet(builder.agents),
		Envs:         sortedSet(builder.envs),
		Capabilities: builder.caps.manifest(),
		IncludeFiles: sortedFileNames(builder.files),
	}
	if manifest.Capabilities.empty() {
		manifest.Capabilities = CapabilityManifest{}
	}

	if err := writeZip(opts.OutputPath, manifest, builder.files); err != nil {
		return nil, err
	}
	return &ExportResult{
		Manifest: manifest,
		Output:   opts.OutputPath,
		Warnings: append([]string(nil), builder.warnings...),
	}, nil
}

func newPackageBuilder(cwd string) *packageBuilder {
	return &packageBuilder{
		cwd:    cwd,
		files:  make(map[string][]byte),
		agents: make(map[string]struct{}),
		envs:   make(map[string]struct{}),
		caps: capabilitySets{
			skills:   make(map[string]struct{}),
			mcps:     make(map[string]struct{}),
			commands: make(map[string]struct{}),
			hooks:    make(map[string]struct{}),
			toolsets: make(map[string]struct{}),
		},
	}
}

func (b *packageBuilder) addRoot(kind, name string) (string, error) {
	switch strings.TrimSpace(kind) {
	case "", "agent", "profile":
		agent, source, err := readAgentPreferProject(name, b.cwd)
		if err == nil {
			if err := b.addAgent(agent, source); err != nil {
				return "", err
			}
			return "agent", nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		if strings.TrimSpace(kind) == "" {
			return "", fmt.Errorf("package export target %q not found as agent", name)
		}
		return "", err
	case "env":
		env, err := config.ReadEnvironment(name)
		if err != nil {
			return "", err
		}
		if err := b.addEnvironment(env); err != nil {
			return "", err
		}
		return "env", nil
	default:
		return "", fmt.Errorf("unsupported package export kind %q", kind)
	}
}

func (b *packageBuilder) addEnvironment(env *config.Environment) error {
	if env == nil {
		return fmt.Errorf("environment is nil")
	}
	b.envs[env.Name] = struct{}{}
	if err := b.addFile(packageEnvPath(env.Name), config.EnvPath(env.Name)); err != nil {
		return err
	}

	for _, name := range envAgentNames(env) {
		agent, source, err := readAgentPreferProject(name, b.cwd)
		if err != nil {
			return fmt.Errorf("env %q references agent %q: %w", env.Name, name, err)
		}
		if err := b.addAgent(agent, source); err != nil {
			return err
		}
	}
	return nil
}

func (b *packageBuilder) addAgent(agent *config.AgentProfile, source string) error {
	if agent == nil {
		return fmt.Errorf("agent profile is nil")
	}
	if _, ok := b.agents[agent.Name]; ok {
		return nil
	}
	b.agents[agent.Name] = struct{}{}
	if err := b.addFile(packageAgentPath(agent.Name), source); err != nil {
		return err
	}

	b.addCapabilities(agent.Capabilities)
	return nil
}

func (b *packageBuilder) addCapabilities(caps config.CapabilityRefs) {
	for _, name := range caps.Skills {
		b.caps.skills[name] = struct{}{}
		b.addCapabilityMetadata("skills", name)
	}
	for _, name := range caps.MCPs {
		b.caps.mcps[name] = struct{}{}
		b.addCapabilityMetadata("mcps", name)
	}
	for _, name := range caps.Commands {
		b.caps.commands[name] = struct{}{}
		b.addCapabilityMetadata("commands", name)
	}
	for _, name := range caps.Hooks {
		b.caps.hooks[name] = struct{}{}
		b.addCapabilityMetadata("hooks", name)
	}
	for name := range caps.Toolsets {
		b.caps.toolsets[name] = struct{}{}
		b.addCapabilityMetadata("toolsets", name)
	}
}

func (b *packageBuilder) addCapabilityMetadata(kind, name string) {
	base := config.RegistryKindDir(kind)
	candidates := []string{
		filepath.Join(base, name),
		filepath.Join(base, name+".yaml"),
		filepath.Join(base, name+".yml"),
	}
	for _, candidate := range candidates {
		info, err := os.Stat(candidate)
		if err != nil {
			continue
		}
		if info.IsDir() {
			if err := b.addRegistryDirectory(candidate, packageRegistryPath(kind, name)); err != nil {
				b.warnings = append(b.warnings, err.Error())
			}
			continue
		}
		if err := b.addRegistryFile(packageRegistryPath(kind, filepath.Base(candidate)), candidate); err != nil {
			b.warnings = append(b.warnings, err.Error())
		}
	}
}

func (b *packageBuilder) addDirectory(sourceDir, packageDir string) error {
	return filepath.WalkDir(sourceDir, func(source string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(sourceDir, source)
		if err != nil {
			return err
		}
		return b.addFile(path.Join(packageDir, filepath.ToSlash(rel)), source)
	})
}

func (b *packageBuilder) addRegistryDirectory(sourceDir, packageDir string) error {
	return filepath.WalkDir(sourceDir, func(source string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(sourceDir, source)
		if err != nil {
			return err
		}
		return b.addRegistryFile(path.Join(packageDir, filepath.ToSlash(rel)), source)
	})
}

func (b *packageBuilder) addFile(packagePath, source string) error {
	clean, err := cleanPackagePath(packagePath)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	return b.addBytes(clean, data)
}

func (b *packageBuilder) addRegistryFile(packagePath, source string) error {
	clean, err := cleanPackagePath(packagePath)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	if isStructuredMetadata(source) && containsPlaintextSecret(data) {
		b.warnings = append(b.warnings, fmt.Sprintf("skipped registry metadata with possible plaintext secret: %s", source))
		return nil
	}
	return b.addBytes(clean, data)
}

func (b *packageBuilder) addBytes(clean string, data []byte) error {
	if existing, ok := b.files[clean]; ok {
		if !bytes.Equal(existing, data) {
			return fmt.Errorf("package path %s has multiple source files with different content", clean)
		}
		return nil
	}
	b.files[clean] = data
	return nil
}

func (c capabilitySets) manifest() CapabilityManifest {
	return CapabilityManifest{
		Skills:   sortedSet(c.skills),
		MCPs:     sortedSet(c.mcps),
		Commands: sortedSet(c.commands),
		Hooks:    sortedSet(c.hooks),
		Toolsets: sortedSet(c.toolsets),
	}
}

func writeZip(outputPath string, manifest Manifest, files map[string][]byte) error {
	dir := filepath.Dir(outputPath)
	if dir == "" {
		dir = "."
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".avm-package-*.zip")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	removeTemp := true
	defer func() {
		if removeTemp {
			_ = os.Remove(tmpPath)
		}
	}()

	zw := zip.NewWriter(tmp)
	manifestBytes, err := marshalYAML(manifest)
	if err != nil {
		_ = zw.Close()
		_ = tmp.Close()
		return err
	}
	if err := addZipFile(zw, "manifest.yaml", manifestBytes); err != nil {
		_ = zw.Close()
		_ = tmp.Close()
		return err
	}
	for _, name := range sortedFileNames(files) {
		if err := addZipFile(zw, name, files[name]); err != nil {
			_ = zw.Close()
			_ = tmp.Close()
			return err
		}
	}
	if err := zw.Close(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, outputPath); err != nil {
		return err
	}
	removeTemp = false
	return nil
}

func addZipFile(zw *zip.Writer, name string, data []byte) error {
	header := &zip.FileHeader{
		Name:   name,
		Method: zip.Deflate,
	}
	header.SetMode(0o600)
	header.Modified = time.Unix(0, 0).UTC()
	writer, err := zw.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = writer.Write(data)
	return err
}

func marshalYAML(value any) ([]byte, error) {
	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(value); err != nil {
		_ = encoder.Close()
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func readAgentPreferProject(name, cwd string) (*config.AgentProfile, string, error) {
	projectPath := config.ProjectAgentPath(cwd, name)
	agent, err := config.ReadAgent(name, config.ScopeProject, cwd)
	if err == nil {
		return agent, projectPath, nil
	}
	if !os.IsNotExist(err) {
		return nil, "", err
	}

	globalPath := config.AgentPath(name)
	agent, err = config.ReadAgent(name, config.ScopeGlobal, cwd)
	if err == nil {
		return agent, globalPath, nil
	}
	return nil, "", err
}

func envAgentNames(env *config.Environment) []string {
	seen := make(map[string]struct{})
	for _, runtimeAgent := range env.RuntimeAgents {
		if runtimeAgent.Primary != "" {
			seen[runtimeAgent.Primary] = struct{}{}
		}
		for _, name := range runtimeAgent.Available {
			if name != "" {
				seen[name] = struct{}{}
			}
		}
	}
	return sortedSet(seen)
}

func isStructuredMetadata(source string) bool {
	switch strings.ToLower(filepath.Ext(source)) {
	case ".yaml", ".yml", ".json", ".toml":
		return true
	default:
		return false
	}
}

func containsPlaintextSecret(data []byte) bool {
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") {
			continue
		}
		key, value, ok := splitSecretCandidate(line)
		if !ok || !secretLikeKey(key) {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"',`)
		if value == "" || value == "null" || value == "false" || value == "true" || value == "[]" || value == "{}" {
			continue
		}
		if strings.HasPrefix(value, "$") || strings.Contains(value, "${") {
			continue
		}
		return true
	}
	return false
}

func splitSecretCandidate(line string) (string, string, bool) {
	if idx := strings.Index(line, ":"); idx >= 0 {
		return strings.ToLower(strings.TrimSpace(line[:idx])), strings.TrimSpace(line[idx+1:]), true
	}
	if idx := strings.Index(line, "="); idx >= 0 {
		return strings.ToLower(strings.TrimSpace(line[:idx])), strings.TrimSpace(line[idx+1:]), true
	}
	return "", "", false
}

func secretLikeKey(key string) bool {
	key = strings.Trim(key, `"'`)
	return key == "token" ||
		strings.HasSuffix(key, "_token") ||
		strings.Contains(key, "secret") ||
		strings.Contains(key, "password") ||
		strings.Contains(key, "api_key") ||
		strings.Contains(key, "apikey") ||
		strings.Contains(key, "private_key")
}

func packageAgentPath(name string) string {
	return path.Join("agents", name+".yaml")
}

func packageEnvPath(name string) string {
	return path.Join("envs", name+".yaml")
}

func packageRegistryPath(kind, name string) string {
	return path.Join("registry", kind, name)
}

func sortedFileNames(files map[string][]byte) []string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func sortedSet(values map[string]struct{}) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func cwdOrCurrent(cwd string) (string, error) {
	if cwd != "" {
		return cwd, nil
	}
	return os.Getwd()
}
