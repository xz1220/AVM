// Package runlog persists run history (RunRecord) per PRD §4.4.
package runlog

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/xz1220/agent-vm/internal/app/model"
)

// FileName is the on-disk file inside Dir that holds the JSON-lines log.
const FileName = "runs.jsonl"

// Log is the contract for run history persistence.
type Log interface {
	Append(record model.RunRecord) error
	List(limit int) ([]model.RunRecord, error)
}

// FSLog is the on-disk default implementation. It writes one JSON record
// per line in append-only fashion.
type FSLog struct {
	Dir string
	mu  sync.Mutex
}

func New(dir string) *FSLog { return &FSLog{Dir: dir} }

func (l *FSLog) path() string { return filepath.Join(l.Dir, FileName) }

// Append serializes record as JSON and appends it as a single line.
func (l *FSLog) Append(record model.RunRecord) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := os.MkdirAll(l.Dir, 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(record)
	if err != nil {
		return err
	}
	data = append(data, '\n')
	f, err := os.OpenFile(l.path(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return err
	}
	return nil
}

// List returns the most recent `limit` records. limit == 0 means all.
func (l *FSLog) List(limit int) ([]model.RunRecord, error) {
	if limit < 0 {
		return nil, errors.New("runlog: negative limit")
	}
	f, err := os.Open(l.path())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var out []model.RunRecord
	br := bufio.NewReader(f)
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			// Strip the trailing newline if any.
			if line[len(line)-1] == '\n' {
				line = line[:len(line)-1]
			}
			if len(line) > 0 {
				var rec model.RunRecord
				if jerr := json.Unmarshal(line, &rec); jerr != nil {
					// Skip malformed lines but keep going.
					continue
				}
				out = append(out, rec)
			}
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
	}
	if limit == 0 || limit >= len(out) {
		return out, nil
	}
	// Most recent `limit` entries (records were appended in order, so
	// take the tail).
	return out[len(out)-limit:], nil
}
