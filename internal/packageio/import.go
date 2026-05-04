package packageio

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/xz1220/agent-vm/internal/config"
	"gopkg.in/yaml.v3"
)

type ImportOptions struct {
	PackagePath string
	CWD         string
	DryRun      bool
}

type ImportResult struct {
	Manifest  Manifest
	Added     []ImportAction
	Skipped   []ImportAction
	Conflicts []ImportAction
}

type ImportAction struct {
	PackagePath string
	TargetPath  string
}

type InspectOptions struct {
	PackagePath string
}

type InspectResult struct {
	Manifest Manifest
	Files    []string
}

type ConflictError struct {
	PackagePath string
	TargetPath  string
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("package import conflict: %s already exists with different content", e.PackagePath)
}

func ImportPackage(opts ImportOptions) (*ImportResult, error) {
	if opts.PackagePath == "" {
		return nil, fmt.Errorf("package import path is required")
	}
	cwd, err := cwdOrCurrent(opts.CWD)
	if err != nil {
		return nil, err
	}
	archive, err := readPackageArchive(opts.PackagePath)
	if err != nil {
		return nil, err
	}

	plans := make([]importPlan, 0, len(archive.includeFiles))
	seenTargets := make(map[string]importPlan, len(archive.includeFiles))
	for _, packagePath := range archive.includeFiles {
		data := archive.files[packagePath]
		plan, err := importPlanForFile(packagePath, data, cwd)
		if err != nil {
			return nil, err
		}
		if previous, ok := seenTargets[plan.targetPath]; ok {
			if !bytes.Equal(previous.data, plan.data) {
				return nil, fmt.Errorf("package import conflict: %s and %s map to %s with different content", previous.packagePath, plan.packagePath, plan.targetPath)
			}
			continue
		}
		seenTargets[plan.targetPath] = plan
		plans = append(plans, plan)
	}
	sort.Slice(plans, func(i, j int) bool {
		return plans[i].packagePath < plans[j].packagePath
	})
	if err := normalizeImportedAgentIDs(plans, cwd); err != nil {
		return nil, err
	}

	result := &ImportResult{Manifest: archive.manifest}
	for _, plan := range plans {
		existing, err := os.ReadFile(plan.targetPath)
		if err == nil {
			if bytes.Equal(existing, plan.data) {
				result.Skipped = append(result.Skipped, ImportAction{PackagePath: plan.packagePath, TargetPath: plan.targetPath})
				continue
			}
			result.Conflicts = append(result.Conflicts, ImportAction{PackagePath: plan.packagePath, TargetPath: plan.targetPath})
			continue
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		result.Added = append(result.Added, ImportAction{PackagePath: plan.packagePath, TargetPath: plan.targetPath})
	}

	if opts.DryRun {
		return result, nil
	}
	if len(result.Conflicts) > 0 {
		conflict := result.Conflicts[0]
		return nil, &ConflictError{PackagePath: conflict.PackagePath, TargetPath: conflict.TargetPath}
	}

	for _, plan := range plans {
		if containsAction(result.Skipped, plan.packagePath) {
			continue
		}
		if containsAction(result.Conflicts, plan.packagePath) {
			continue
		}
		if err := writeImportFile(plan.targetPath, plan.data); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func InspectPackage(opts InspectOptions) (*InspectResult, error) {
	if opts.PackagePath == "" {
		return nil, fmt.Errorf("package inspect path is required")
	}
	archive, err := readPackageArchive(opts.PackagePath)
	if err != nil {
		return nil, err
	}
	files := append([]string(nil), archive.includeFiles...)
	sort.Strings(files)
	return &InspectResult{
		Manifest: archive.manifest,
		Files:    files,
	}, nil
}

type packageArchive struct {
	manifest     Manifest
	files        map[string][]byte
	includeFiles []string
}

func readPackageArchive(packagePath string) (*packageArchive, error) {
	archive, err := readZip(packagePath)
	if err != nil {
		return nil, err
	}
	manifestBytes, ok := archive["manifest.yaml"]
	if !ok {
		return nil, fmt.Errorf("package manifest.yaml is required")
	}
	var manifest Manifest
	if err := unmarshalKnownYAML(manifestBytes, &manifest); err != nil {
		return nil, fmt.Errorf("manifest.yaml: %w", err)
	}
	if manifest.Version != ManifestVersion {
		return nil, fmt.Errorf("unsupported package manifest version %q (want %q)", manifest.Version, ManifestVersion)
	}

	includeFiles := manifest.IncludeFiles
	if len(includeFiles) == 0 {
		includeFiles = archiveFileNames(archive)
	}
	if err := validateIncludeFiles(includeFiles, archive); err != nil {
		return nil, err
	}
	return &packageArchive{
		manifest:     manifest,
		files:        archive,
		includeFiles: append([]string(nil), includeFiles...),
	}, nil
}

type importPlan struct {
	packagePath string
	targetPath  string
	data        []byte
	agent       *config.AgentProfile
}

func readZip(packagePath string) (map[string][]byte, error) {
	reader, err := zip.OpenReader(packagePath)
	if err != nil {
		return nil, err
	}
	defer reader.Close()

	files := make(map[string][]byte, len(reader.File))
	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}
		name, err := cleanPackagePath(file.Name)
		if err != nil {
			return nil, err
		}
		if _, ok := files[name]; ok {
			return nil, fmt.Errorf("duplicate package file %s", name)
		}
		rc, err := file.Open()
		if err != nil {
			return nil, err
		}
		data, readErr := io.ReadAll(rc)
		closeErr := rc.Close()
		if readErr != nil {
			return nil, readErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		files[name] = data
	}
	return files, nil
}

func validateIncludeFiles(includeFiles []string, archive map[string][]byte) error {
	seen := make(map[string]struct{}, len(includeFiles))
	for _, name := range includeFiles {
		clean, err := cleanPackagePath(name)
		if err != nil {
			return err
		}
		if clean == "manifest.yaml" {
			return fmt.Errorf("manifest include_files must not include manifest.yaml")
		}
		if clean != name {
			return fmt.Errorf("manifest include_files contains unclean path %q", name)
		}
		if _, ok := seen[clean]; ok {
			return fmt.Errorf("manifest include_files contains duplicate path %s", clean)
		}
		seen[clean] = struct{}{}
		if _, ok := archive[clean]; !ok {
			return fmt.Errorf("manifest include_files references missing file %s", clean)
		}
	}
	return nil
}

func importPlanForFile(packagePath string, data []byte, cwd string) (importPlan, error) {
	parts := strings.Split(packagePath, "/")
	if len(parts) < 2 {
		return importPlan{}, fmt.Errorf("unsupported package file %s", packagePath)
	}

	switch parts[0] {
	case "agents":
		if len(parts) != 2 || path.Ext(parts[1]) != ".yaml" {
			return importPlan{}, fmt.Errorf("unsupported agent package file %s", packagePath)
		}
		var agent config.AgentProfile
		if err := unmarshalKnownYAML(data, &agent); err != nil {
			return importPlan{}, fmt.Errorf("%s: %w", packagePath, err)
		}
		missingID := agent.ID == ""
		targetPath := targetAgentPath(agent.SourceScope, strings.TrimSuffix(parts[1], ".yaml"), cwd)
		if missingID {
			if existingID, ok, err := existingAgentIDAtPath(targetPath); err != nil {
				return importPlan{}, err
			} else if ok {
				agent.ID = existingID
			}
		}
		agent.ApplyDefaults(agent.SourceScope)
		if err := config.ValidateAgentProfile(&agent); err != nil {
			return importPlan{}, fmt.Errorf("%s: %w", packagePath, err)
		}
		name := strings.TrimSuffix(parts[1], ".yaml")
		if agent.Name != name {
			return importPlan{}, fmt.Errorf("%s: name: expected %q, got %q", packagePath, name, agent.Name)
		}
		data, err := marshalYAMLDocument(&agent)
		if err != nil {
			return importPlan{}, fmt.Errorf("%s: %w", packagePath, err)
		}
		return importPlan{
			packagePath: packagePath,
			targetPath:  targetPath,
			data:        data,
			agent:       &agent,
		}, nil
	case "envs":
		if len(parts) != 2 || path.Ext(parts[1]) != ".yaml" {
			return importPlan{}, fmt.Errorf("unsupported env package file %s", packagePath)
		}
		var env config.Environment
		if err := unmarshalKnownYAML(data, &env); err != nil {
			return importPlan{}, fmt.Errorf("%s: %w", packagePath, err)
		}
		env.ApplyDefaults()
		if err := config.ValidateEnvironment(&env); err != nil {
			return importPlan{}, fmt.Errorf("%s: %w", packagePath, err)
		}
		name := strings.TrimSuffix(parts[1], ".yaml")
		if env.Name != name {
			return importPlan{}, fmt.Errorf("%s: name: expected %q, got %q", packagePath, name, env.Name)
		}
		return importPlan{packagePath: packagePath, targetPath: config.EnvPath(env.Name), data: data}, nil
	case "registry":
		if len(parts) < 3 {
			return importPlan{}, fmt.Errorf("unsupported registry package file %s", packagePath)
		}
		return importPlan{packagePath: packagePath, targetPath: filepath.Join(config.RegistryDir(), filepath.FromSlash(strings.TrimPrefix(packagePath, "registry/"))), data: data}, nil
	default:
		return importPlan{}, fmt.Errorf("unsupported package file %s", packagePath)
	}
}

func targetAgentPath(scope, name, cwd string) string {
	switch config.Scope(scope) {
	case config.ScopeProject, config.ScopeLocal:
		return config.ProjectAgentPath(cwd, name)
	default:
		return config.AgentPath(name)
	}
}

func normalizeImportedAgentIDs(plans []importPlan, cwd string) error {
	used, err := existingAgentIDPaths(cwd)
	if err != nil {
		return err
	}
	for i := range plans {
		if plans[i].agent == nil {
			continue
		}
		targetPath := filepath.Clean(plans[i].targetPath)
		if ownerPath, ok := used[plans[i].agent.ID]; ok && filepath.Clean(ownerPath) != targetPath {
			plans[i].agent.ID = config.NewAgentID()
			data, err := marshalYAMLDocument(plans[i].agent)
			if err != nil {
				return err
			}
			plans[i].data = data
		}
		used[plans[i].agent.ID] = targetPath
	}
	return nil
}

func existingAgentIDPaths(cwd string) (map[string]string, error) {
	used := make(map[string]string)
	for _, dir := range []string{config.AgentsDir(), config.ProjectAgentsDir(cwd)} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".yaml" {
				continue
			}
			path := filepath.Join(dir, entry.Name())
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, err
			}
			var agent config.AgentProfile
			if err := unmarshalKnownYAML(data, &agent); err != nil {
				return nil, err
			}
			if agent.ID != "" {
				used[agent.ID] = path
			}
		}
	}
	return used, nil
}

func existingAgentIDAtPath(path string) (string, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", false, nil
		}
		return "", false, err
	}
	var agent config.AgentProfile
	if err := unmarshalKnownYAML(data, &agent); err != nil {
		return "", false, err
	}
	return agent.ID, agent.ID != "", nil
}

func marshalYAMLDocument(value any) ([]byte, error) {
	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(value); err != nil {
		_ = encoder.Close()
		return nil, err
	}
	if err := encoder.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func writeImportFile(targetPath string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(targetPath), ".avm-import-*")
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
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, targetPath); err != nil {
		return err
	}
	removeTemp = false
	return nil
}

func unmarshalKnownYAML(data []byte, out any) error {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(out); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if extra != nil {
		return fmt.Errorf("multiple YAML documents are not supported")
	}
	return nil
}

func archiveFileNames(archive map[string][]byte) []string {
	names := make([]string, 0, len(archive))
	for name := range archive {
		if name != "manifest.yaml" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func containsAction(actions []ImportAction, packagePath string) bool {
	for _, action := range actions {
		if action.PackagePath == packagePath {
			return true
		}
	}
	return false
}

func cleanPackagePath(value string) (string, error) {
	if value == "" {
		return "", fmt.Errorf("empty package path")
	}
	if strings.Contains(value, "\\") {
		return "", fmt.Errorf("package path %q must use slash separators", value)
	}
	clean := path.Clean(value)
	if clean == "." || clean == "" || clean != value {
		return "", fmt.Errorf("invalid package path %q", value)
	}
	if path.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(clean, "/../") {
		return "", fmt.Errorf("invalid package path %q", value)
	}
	return clean, nil
}
