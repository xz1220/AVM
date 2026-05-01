package packageio

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

type SourceKind int

const (
	SourceLocal SourceKind = iota
	SourceHTTP
	SourceGitHub
)

const maxDownloadSize = 100 << 20 // 100 MB

type HTTPClient interface {
	Do(req *http.Request) (*http.Response, error)
}

type DownloadOptions struct {
	URL      string
	Dir      string
	Checksum string
	Client   HTTPClient
}

func ResolveSource(input string) (SourceKind, string) {
	if strings.HasPrefix(input, "https://") || strings.HasPrefix(input, "http://") {
		return SourceHTTP, input
	}
	if strings.HasPrefix(input, "github:") {
		return SourceGitHub, input
	}
	return SourceLocal, input
}

func GitHubReleaseURL(input string) (string, error) {
	rest := strings.TrimPrefix(input, "github:")
	if rest == "" || rest == input {
		return "", fmt.Errorf("invalid github source %q: expected github:owner/repo[@tag]", input)
	}

	var ownerRepo, tag string
	if i := strings.Index(rest, "@"); i >= 0 {
		ownerRepo = rest[:i]
		tag = rest[i+1:]
		if tag == "" {
			return "", fmt.Errorf("invalid github source %q: empty tag after @", input)
		}
	} else {
		ownerRepo = rest
	}

	parts := strings.SplitN(ownerRepo, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", fmt.Errorf("invalid github source %q: expected owner/repo", input)
	}
	owner, repo := parts[0], parts[1]

	if tag != "" {
		return fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/%s.avm.zip", owner, repo, tag, repo), nil
	}
	return fmt.Sprintf("https://github.com/%s/%s/releases/latest/download/%s.avm.zip", owner, repo, repo), nil
}

func DownloadPackage(opts DownloadOptions) (string, func(), error) {
	client := opts.Client
	if client == nil {
		client = http.DefaultClient
	}

	req, err := http.NewRequest("GET", opts.URL, nil)
	if err != nil {
		return "", nil, fmt.Errorf("download %s: %w", opts.URL, err)
	}
	req.Header.Set("User-Agent", "avm")

	resp, err := client.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("download %s: %w", opts.URL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("download %s: HTTP %d", opts.URL, resp.StatusCode)
	}

	dir := opts.Dir
	if dir == "" {
		dir = os.TempDir()
	}
	tmp, err := os.CreateTemp(dir, ".avm-download-*.zip")
	if err != nil {
		return "", nil, fmt.Errorf("download %s: create temp file: %w", opts.URL, err)
	}
	tmpPath := tmp.Name()
	cleanup := func() { os.Remove(tmpPath) }

	n, err := io.Copy(tmp, io.LimitReader(resp.Body, maxDownloadSize+1))
	if err != nil {
		tmp.Close()
		cleanup()
		return "", nil, fmt.Errorf("download %s: %w", opts.URL, err)
	}
	if n > maxDownloadSize {
		tmp.Close()
		cleanup()
		return "", nil, fmt.Errorf("download %s: exceeds maximum size (%d MB)", opts.URL, maxDownloadSize>>20)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return "", nil, fmt.Errorf("download %s: %w", opts.URL, err)
	}

	if opts.Checksum != "" {
		if err := VerifyChecksum(tmpPath, opts.Checksum); err != nil {
			cleanup()
			return "", nil, err
		}
	}

	return tmpPath, cleanup, nil
}

func VerifyChecksum(path, checksum string) error {
	algo, expected, err := parseChecksum(checksum)
	if err != nil {
		return err
	}
	if algo != "sha256" {
		return fmt.Errorf("unsupported checksum algorithm %q: only sha256 is supported", algo)
	}

	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("verify checksum: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("verify checksum: %w", err)
	}
	actual := hex.EncodeToString(h.Sum(nil))
	if actual != expected {
		return fmt.Errorf("checksum mismatch: expected sha256:%s, got sha256:%s", expected, actual)
	}
	return nil
}

func parseChecksum(s string) (algo, hash string, err error) {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid checksum format %q: expected algo:hex (e.g. sha256:abc123...)", s)
	}
	if _, err := hex.DecodeString(parts[1]); err != nil {
		return "", "", fmt.Errorf("invalid checksum hex %q: %w", parts[1], err)
	}
	return parts[0], parts[1], nil
}
