package memory

type DiffStatus string

const (
	DiffStatusNew      DiffStatus = "new"
	DiffStatusChanged  DiffStatus = "changed"
	DiffStatusConflict DiffStatus = "conflict"
	DiffStatusSkipped  DiffStatus = "skipped"
)

var diffStatusOrder = []DiffStatus{
	DiffStatusNew,
	DiffStatusChanged,
	DiffStatusConflict,
	DiffStatusSkipped,
}

func DiffStatuses() []DiffStatus {
	return append([]DiffStatus(nil), diffStatusOrder...)
}

func (s DiffStatus) Valid() bool {
	for _, allowed := range diffStatusOrder {
		if s == allowed {
			return true
		}
	}
	return false
}

type ImportOptions struct {
	Source       string
	DryRun       bool
	Runtime      string
	DefaultScope string
}

type MemoryImportPlan struct {
	Runtime      string                    `json:"runtime,omitempty"`
	Source       string                    `json:"source"`
	DryRun       bool                      `json:"dry_run"`
	Candidates   []PortableMemoryCandidate `json:"candidates"`
	Diffs        []MemoryDiff              `json:"diffs"`
	StatusCounts []StatusCount             `json:"status_counts"`
	Warnings     []string                  `json:"warnings"`
}

type StatusCount struct {
	Status DiffStatus `json:"status"`
	Count  int        `json:"count"`
}

type PortableMemoryCandidate struct {
	ID          string            `json:"id" yaml:"id"`
	Scope       string            `json:"scope" yaml:"scope"`
	Format      string            `json:"format" yaml:"format"`
	Path        string            `json:"path" yaml:"path"`
	Description string            `json:"description,omitempty" yaml:"description,omitempty"`
	Mode        string            `json:"mode" yaml:"mode"`
	Tags        []string          `json:"tags,omitempty" yaml:"tags,omitempty"`
	Origin      MemoryOrigin      `json:"origin" yaml:"origin"`
	WritePolicy MemoryWritePolicy `json:"write_policy" yaml:"write_policy"`
	Content     string            `json:"-" yaml:"content,omitempty"`
}

type MemoryOrigin struct {
	Type       string `json:"type,omitempty" yaml:"type,omitempty"`
	Runtime    string `json:"runtime,omitempty" yaml:"runtime,omitempty"`
	SourcePath string `json:"source_path,omitempty" yaml:"source_path,omitempty"`
}

type MemoryWritePolicy struct {
	AllowPush           bool `json:"allow_push" yaml:"allow_push"`
	RequireConfirmation bool `json:"require_confirmation" yaml:"require_confirmation"`
}

type MemoryDiff struct {
	MemoryID   string     `json:"memory_id"`
	Status     DiffStatus `json:"status"`
	SourcePath string     `json:"source_path,omitempty"`
	TargetPath string     `json:"target_path,omitempty"`
	Preview    string     `json:"preview,omitempty"`
}
