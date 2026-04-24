package memory

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/xz1220/agent-vm/internal/config"
	"gopkg.in/yaml.v3"
)

const (
	defaultMode  = "read"
	defaultScope = string(config.ScopeProject)
)

var repeatedDash = regexp.MustCompile(`-+`)

type sourceDocument struct {
	ID          string            `yaml:"id"`
	Scope       string            `yaml:"scope"`
	Format      string            `yaml:"format"`
	Description string            `yaml:"description"`
	Mode        string            `yaml:"mode"`
	Tags        []string          `yaml:"tags"`
	Content     string            `yaml:"content"`
	Runtime     string            `yaml:"runtime"`
	Origin      MemoryOrigin      `yaml:"origin"`
	WritePolicy MemoryWritePolicy `yaml:"write_policy"`
	Memories    []sourceDocument  `yaml:"memories"`
}

func ImportDryRun(opts ImportOptions) (*MemoryImportPlan, error) {
	if strings.TrimSpace(opts.Source) == "" {
		return nil, fmt.Errorf("memory import source is required")
	}
	if !opts.DryRun {
		return nil, fmt.Errorf("memory import only supports dry-run in phase 1")
	}

	source := filepath.Clean(opts.Source)
	info, err := os.Stat(source)
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, fmt.Errorf("%s: memory import source must be a file", source)
	}

	raw, err := os.ReadFile(source)
	if err != nil {
		return nil, err
	}

	plan := &MemoryImportPlan{
		Runtime: opts.Runtime,
		Source:  source,
		DryRun:  true,
	}

	candidates, skipped, warnings, err := candidatesFromFile(source, raw, opts)
	if err != nil {
		return nil, err
	}
	plan.Candidates = candidates
	plan.Diffs = append(plan.Diffs, skipped...)
	plan.Warnings = append(plan.Warnings, warnings...)

	for _, candidate := range plan.Candidates {
		plan.Diffs = append(plan.Diffs, diffCandidate(candidate))
	}

	sortPlan(plan)
	plan.StatusCounts = countStatuses(plan.Diffs)
	ensurePlanSlices(plan)
	return plan, nil
}

func candidatesFromFile(source string, raw []byte, opts ImportOptions) ([]PortableMemoryCandidate, []MemoryDiff, []string, error) {
	ext := strings.ToLower(filepath.Ext(source))
	switch ext {
	case ".md", ".markdown":
		candidate, skipped, warnings := markdownCandidate(source, raw, opts)
		if skipped != nil {
			return nil, []MemoryDiff{*skipped}, warnings, nil
		}
		return []PortableMemoryCandidate{candidate}, nil, warnings, nil
	case ".yaml", ".yml":
		return yamlCandidates(source, raw, opts)
	default:
		id := normalizeID(strings.TrimSuffix(filepath.Base(source), filepath.Ext(source)))
		return nil, []MemoryDiff{{
			MemoryID:   id,
			Status:     DiffStatusSkipped,
			SourcePath: source,
			Preview:    "unsupported memory import file extension",
		}}, []string{fmt.Sprintf("%s: skipped unsupported memory import file extension %q", source, ext)}, nil
	}
}

func markdownCandidate(source string, raw []byte, opts ImportOptions) (PortableMemoryCandidate, *MemoryDiff, []string) {
	text := normalizeContent(string(raw))
	meta, body, warnings := parseMarkdownFrontMatter(source, text)
	if strings.TrimSpace(body) == "" {
		return PortableMemoryCandidate{}, &MemoryDiff{
			MemoryID:   sourceMemoryID(source, meta.ID),
			Status:     DiffStatusSkipped,
			SourcePath: source,
			Preview:    "empty markdown memory source",
		}, warnings
	}

	doc := sourceDocument{
		ID:          meta.ID,
		Scope:       meta.Scope,
		Format:      "markdown",
		Description: meta.Description,
		Mode:        meta.Mode,
		Tags:        meta.Tags,
		Content:     body,
		Runtime:     meta.Runtime,
		Origin:      meta.Origin,
		WritePolicy: meta.WritePolicy,
	}
	candidate, skipped, candidateWarnings := buildCandidate(source, doc, opts)
	warnings = append(warnings, candidateWarnings...)
	return candidate, skipped, warnings
}

func yamlCandidates(source string, raw []byte, opts ImportOptions) ([]PortableMemoryCandidate, []MemoryDiff, []string, error) {
	text := normalizeContent(string(raw))
	var doc sourceDocument
	decoder := yaml.NewDecoder(bytes.NewReader([]byte(text)))
	if err := decoder.Decode(&doc); err != nil {
		return nil, nil, nil, fmt.Errorf("%s: %w", source, err)
	}

	docs := []sourceDocument{doc}
	if len(doc.Memories) > 0 {
		docs = doc.Memories
	}

	candidates := make([]PortableMemoryCandidate, 0, len(docs))
	var skipped []MemoryDiff
	var warnings []string
	for i, current := range docs {
		if current.Content == "" && len(doc.Memories) == 0 {
			current.Content = text
		}
		if current.Content == "" {
			id := sourceMemoryID(source, current.ID)
			skipped = append(skipped, MemoryDiff{
				MemoryID:   id,
				Status:     DiffStatusSkipped,
				SourcePath: source,
				Preview:    "empty yaml memory source",
			})
			continue
		}
		if current.Format == "" {
			current.Format = "yaml"
		}
		candidate, skip, candidateWarnings := buildCandidate(source, current, opts)
		warnings = append(warnings, candidateWarnings...)
		if skip != nil {
			if len(doc.Memories) > 0 {
				skip.SourcePath = fmt.Sprintf("%s#memories[%d]", source, i)
			}
			skipped = append(skipped, *skip)
			continue
		}
		if len(doc.Memories) > 0 {
			candidate.Origin.SourcePath = fmt.Sprintf("%s#memories[%d]", source, i)
		}
		candidates = append(candidates, candidate)
	}
	return candidates, skipped, warnings, nil
}

func parseMarkdownFrontMatter(source, text string) (sourceDocument, string, []string) {
	if !strings.HasPrefix(text, "---\n") {
		return sourceDocument{}, text, nil
	}
	end := strings.Index(text[len("---\n"):], "\n---\n")
	if end < 0 {
		return sourceDocument{}, text, []string{fmt.Sprintf("%s: ignored unterminated markdown front matter", source)}
	}

	frontMatter := text[len("---\n") : len("---\n")+end]
	body := text[len("---\n")+end+len("\n---\n"):]
	var doc sourceDocument
	if err := yaml.Unmarshal([]byte(frontMatter), &doc); err != nil {
		return sourceDocument{}, text, []string{fmt.Sprintf("%s: ignored invalid markdown front matter: %v", source, err)}
	}
	return doc, body, nil
}

func buildCandidate(source string, doc sourceDocument, opts ImportOptions) (PortableMemoryCandidate, *MemoryDiff, []string) {
	var warnings []string
	id := sourceMemoryID(source, doc.ID)
	if doc.ID != "" && id != doc.ID {
		return PortableMemoryCandidate{}, &MemoryDiff{
			MemoryID:   id,
			Status:     DiffStatusSkipped,
			SourcePath: source,
			Preview:    "invalid memory id",
		}, []string{fmt.Sprintf("%s: skipped invalid memory id %q", source, doc.ID)}
	}

	scope := doc.Scope
	if scope == "" {
		scope = opts.DefaultScope
	}
	if scope == "" {
		scope = defaultScope
	}

	format := doc.Format
	if format == "" {
		format = "markdown"
	}
	mode := doc.Mode
	if mode == "" {
		mode = defaultMode
	}

	runtime := doc.Runtime
	if runtime == "" {
		runtime = doc.Origin.Runtime
	}
	if runtime == "" {
		runtime = opts.Runtime
	}

	candidate := PortableMemoryCandidate{
		ID:          id,
		Scope:       scope,
		Format:      format,
		Description: doc.Description,
		Mode:        mode,
		Tags:        append([]string(nil), doc.Tags...),
		Origin: MemoryOrigin{
			Type:       "import",
			Runtime:    runtime,
			SourcePath: source,
		},
		WritePolicy: MemoryWritePolicy{
			AllowPush:           doc.WritePolicy.AllowPush,
			RequireConfirmation: true,
		},
		Content: normalizeContent(doc.Content),
	}
	if doc.Origin.Type != "" {
		candidate.Origin.Type = doc.Origin.Type
	}
	if doc.Origin.SourcePath != "" {
		candidate.Origin.SourcePath = doc.Origin.SourcePath
	}
	if doc.WritePolicy.RequireConfirmation {
		candidate.WritePolicy.RequireConfirmation = true
	}
	candidate.Path = targetPath(candidate.ID, candidate.Scope, candidate.Format)

	if strings.TrimSpace(candidate.Content) == "" {
		return PortableMemoryCandidate{}, &MemoryDiff{
			MemoryID:   id,
			Status:     DiffStatusSkipped,
			SourcePath: candidate.Origin.SourcePath,
			TargetPath: candidate.Path,
			Preview:    "empty memory content",
		}, warnings
	}
	if err := validateCandidate(candidate); err != nil {
		return PortableMemoryCandidate{}, &MemoryDiff{
			MemoryID:   id,
			Status:     DiffStatusSkipped,
			SourcePath: candidate.Origin.SourcePath,
			TargetPath: candidate.Path,
			Preview:    err.Error(),
		}, append(warnings, fmt.Sprintf("%s: skipped %s", source, err))
	}
	return candidate, nil, warnings
}

func validateCandidate(candidate PortableMemoryCandidate) error {
	memory := config.PortableMemory{
		ID:          candidate.ID,
		Scope:       candidate.Scope,
		Format:      candidate.Format,
		Path:        candidate.Path,
		Description: candidate.Description,
		Mode:        candidate.Mode,
		Tags:        append([]string(nil), candidate.Tags...),
		Origin: config.MemoryOrigin{
			Type:       candidate.Origin.Type,
			Runtime:    candidate.Origin.Runtime,
			SourcePath: candidate.Origin.SourcePath,
		},
		WritePolicy: config.MemoryWritePolicy{
			AllowPush:           candidate.WritePolicy.AllowPush,
			RequireConfirmation: candidate.WritePolicy.RequireConfirmation,
		},
	}
	return config.Validate(&memory)
}

func diffCandidate(candidate PortableMemoryCandidate) MemoryDiff {
	diff := MemoryDiff{
		MemoryID:   candidate.ID,
		Status:     DiffStatusNew,
		SourcePath: candidate.Origin.SourcePath,
		TargetPath: candidate.Path,
		Preview:    preview(candidate.Content),
	}

	if candidate.Format == "markdown" {
		if status, ok := metadataConflictStatus(candidate); ok {
			diff.Status = status
			return diff
		}
	}

	info, err := os.Stat(candidate.Path)
	if err != nil {
		if os.IsNotExist(err) {
			return diff
		}
		diff.Status = DiffStatusConflict
		diff.Preview = err.Error()
		return diff
	}
	if info.IsDir() {
		diff.Status = DiffStatusConflict
		diff.Preview = "target path is a directory"
		return diff
	}

	existing, err := os.ReadFile(candidate.Path)
	if err != nil {
		diff.Status = DiffStatusConflict
		diff.Preview = err.Error()
		return diff
	}
	if equivalentContent(string(existing), candidate.Content) {
		diff.Status = DiffStatusSkipped
		return diff
	}
	diff.Status = DiffStatusChanged
	return diff
}

func metadataConflictStatus(candidate PortableMemoryCandidate) (DiffStatus, bool) {
	metadataPath := config.MemoryPath(candidate.ID, config.Scope(candidate.Scope))
	info, err := os.Stat(metadataPath)
	if err != nil {
		return "", false
	}
	if info.IsDir() {
		return DiffStatusConflict, true
	}

	existing, err := config.ReadPortableMemory(candidate.ID, config.Scope(candidate.Scope))
	if err != nil {
		return DiffStatusConflict, true
	}
	if existing.Format != candidate.Format {
		return DiffStatusConflict, true
	}
	if existing.Path != "" && filepath.Clean(existing.Path) != filepath.Clean(candidate.Path) {
		return DiffStatusConflict, true
	}
	return "", false
}

func sortPlan(plan *MemoryImportPlan) {
	sort.SliceStable(plan.Candidates, func(i, j int) bool {
		return compareCandidate(plan.Candidates[i], plan.Candidates[j]) < 0
	})
	sort.SliceStable(plan.Diffs, func(i, j int) bool {
		return compareDiff(plan.Diffs[i], plan.Diffs[j]) < 0
	})
	sort.Strings(plan.Warnings)
}

func compareCandidate(a, b PortableMemoryCandidate) int {
	for _, pair := range [][2]string{
		{a.Scope, b.Scope},
		{a.ID, b.ID},
		{a.Path, b.Path},
		{a.Origin.SourcePath, b.Origin.SourcePath},
	} {
		if pair[0] < pair[1] {
			return -1
		}
		if pair[0] > pair[1] {
			return 1
		}
	}
	return 0
}

func compareDiff(a, b MemoryDiff) int {
	for _, pair := range [][2]string{
		{a.MemoryID, b.MemoryID},
		{string(a.Status), string(b.Status)},
		{a.TargetPath, b.TargetPath},
		{a.SourcePath, b.SourcePath},
	} {
		if pair[0] < pair[1] {
			return -1
		}
		if pair[0] > pair[1] {
			return 1
		}
	}
	return 0
}

func countStatuses(diffs []MemoryDiff) []StatusCount {
	counts := make(map[DiffStatus]int, len(diffStatusOrder))
	for _, diff := range diffs {
		counts[diff.Status]++
	}

	statusCounts := make([]StatusCount, 0, len(diffStatusOrder))
	for _, status := range diffStatusOrder {
		statusCounts = append(statusCounts, StatusCount{
			Status: status,
			Count:  counts[status],
		})
	}
	return statusCounts
}

func ensurePlanSlices(plan *MemoryImportPlan) {
	if plan.Candidates == nil {
		plan.Candidates = []PortableMemoryCandidate{}
	}
	if plan.Diffs == nil {
		plan.Diffs = []MemoryDiff{}
	}
	if plan.Warnings == nil {
		plan.Warnings = []string{}
	}
}

func targetPath(id, scope, format string) string {
	extension := ".md"
	if format == "yaml" {
		extension = ".yaml"
	}
	return filepath.Join(config.MemoryScopeDir(config.Scope(scope)), id+extension)
}

func sourceMemoryID(source, explicit string) string {
	if explicit != "" {
		return normalizeID(explicit)
	}
	base := filepath.Base(source)
	ext := filepath.Ext(base)
	return normalizeID(strings.TrimSuffix(base, ext))
}

func normalizeID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var builder strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case unicode.IsLetter(r), unicode.IsDigit(r):
			builder.WriteRune('-')
		default:
			builder.WriteRune('-')
		}
	}
	id := strings.Trim(repeatedDash.ReplaceAllString(builder.String(), "-"), "-")
	if id == "" {
		id = "memory"
	}
	if len(id) > 64 {
		id = strings.TrimRight(id[:64], "-")
	}
	if id == "" {
		id = "memory"
	}
	return id
}

func normalizeContent(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	return content + "\n"
}

func equivalentContent(a, b string) bool {
	return normalizeContent(a) == normalizeContent(b)
}

func preview(content string) string {
	lines := strings.Split(normalizeContent(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		return truncate(line, 96)
	}
	return ""
}

func truncate(value string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	if utf8.RuneCountInString(value) <= maxRunes {
		return value
	}
	runes := []rune(value)
	return string(runes[:maxRunes])
}
