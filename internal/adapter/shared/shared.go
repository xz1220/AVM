// Package shared provides utility functions used by multiple adapter implementations.
package shared

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/xz1220/agent-vm/internal/adapter"
)

func WriteFileAtomic(path string, content []byte) (bool, error) {
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

func RemoveFileAndEmptyParent(path string) (bool, error) {
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	_ = os.Remove(filepath.Dir(path))
	return true, nil
}

func Slug(value string) string {
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

func FirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func SortedStrings(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}

func SortedMCPServers(servers []adapter.MCPServer) []adapter.MCPServer {
	out := append([]adapter.MCPServer(nil), servers...)
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

func MCPServerRenderable(server adapter.MCPServer) bool {
	return server.Name != "" && (server.Command != "" || server.URL != "")
}

func ManagedPathIndex(paths []adapter.ManagedPath) map[string]adapter.ManagedPath {
	managed := make(map[string]adapter.ManagedPath, len(paths))
	for _, path := range paths {
		managed[path.Path] = path
	}
	return managed
}

func WriteLine(builder *strings.Builder, format string, args ...any) {
	builder.WriteString(fmt.Sprintf(format, args...))
	builder.WriteByte('\n')
}

func MarshalJSON(value any) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func CapabilityLines(refs []adapter.CapabilityRef) []string {
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

func ToolsetLines(toolsets []adapter.Toolset) []string {
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
