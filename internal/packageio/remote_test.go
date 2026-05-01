package packageio

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveSource(t *testing.T) {
	tests := []struct {
		input    string
		wantKind SourceKind
		wantVal  string
	}{
		{"backend-coder.avm.zip", SourceLocal, "backend-coder.avm.zip"},
		{"/tmp/pkg.avm.zip", SourceLocal, "/tmp/pkg.avm.zip"},
		{"./relative/path.avm.zip", SourceLocal, "./relative/path.avm.zip"},
		{"https://example.com/pkg.avm.zip", SourceHTTP, "https://example.com/pkg.avm.zip"},
		{"http://localhost:8080/pkg.avm.zip", SourceHTTP, "http://localhost:8080/pkg.avm.zip"},
		{"github:user/repo@v1.0.0", SourceGitHub, "github:user/repo@v1.0.0"},
		{"github:user/repo", SourceGitHub, "github:user/repo"},
		{"", SourceLocal, ""},
		{"github-file.zip", SourceLocal, "github-file.zip"},
		{"httpstuff.zip", SourceLocal, "httpstuff.zip"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			kind, val := ResolveSource(tt.input)
			if kind != tt.wantKind {
				t.Fatalf("ResolveSource(%q) kind = %d, want %d", tt.input, kind, tt.wantKind)
			}
			if val != tt.wantVal {
				t.Fatalf("ResolveSource(%q) val = %q, want %q", tt.input, val, tt.wantVal)
			}
		})
	}
}

func TestGitHubReleaseURL(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{
			input: "github:user/repo@v1.0.0",
			want:  "https://github.com/user/repo/releases/download/v1.0.0/repo.avm.zip",
		},
		{
			input: "github:org/my-agents@latest",
			want:  "https://github.com/org/my-agents/releases/download/latest/my-agents.avm.zip",
		},
		{
			input: "github:user/repo",
			want:  "https://github.com/user/repo/releases/latest/download/repo.avm.zip",
		},
		{input: "github:", wantErr: true},
		{input: "github:user", wantErr: true},
		{input: "github:user/", wantErr: true},
		{input: "github:/repo", wantErr: true},
		{input: "github:user/repo@", wantErr: true},
		{input: "not-github:user/repo", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := GitHubReleaseURL(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("GitHubReleaseURL(%q) = %q, want error", tt.input, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("GitHubReleaseURL(%q) error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("GitHubReleaseURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestDownloadPackage(t *testing.T) {
	content := []byte("PK\x03\x04fake-zip-content-for-testing")

	t.Run("success", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("User-Agent") != "avm" {
				t.Errorf("User-Agent = %q, want avm", r.Header.Get("User-Agent"))
			}
			w.Write(content)
		}))
		defer srv.Close()

		path, cleanup, err := DownloadPackage(DownloadOptions{
			URL:    srv.URL + "/pkg.avm.zip",
			Dir:    t.TempDir(),
			Client: srv.Client(),
		})
		if err != nil {
			t.Fatalf("DownloadPackage error: %v", err)
		}
		defer cleanup()

		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read downloaded file: %v", err)
		}
		if string(got) != string(content) {
			t.Fatalf("content mismatch: got %q", got)
		}
	})

	t.Run("checksum match", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write(content)
		}))
		defer srv.Close()

		h := sha256.Sum256(content)
		checksum := "sha256:" + hex.EncodeToString(h[:])

		path, cleanup, err := DownloadPackage(DownloadOptions{
			URL:      srv.URL + "/pkg.avm.zip",
			Dir:      t.TempDir(),
			Checksum: checksum,
			Client:   srv.Client(),
		})
		if err != nil {
			t.Fatalf("DownloadPackage error: %v", err)
		}
		defer cleanup()

		if _, err := os.Stat(path); err != nil {
			t.Fatalf("downloaded file missing: %v", err)
		}
	})

	t.Run("checksum mismatch", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write(content)
		}))
		defer srv.Close()

		dir := t.TempDir()
		_, _, err := DownloadPackage(DownloadOptions{
			URL:      srv.URL + "/pkg.avm.zip",
			Dir:      dir,
			Checksum: "sha256:0000000000000000000000000000000000000000000000000000000000000000",
			Client:   srv.Client(),
		})
		if err == nil {
			t.Fatal("expected checksum mismatch error")
		}
		if got := err.Error(); !strings.Contains(got, "checksum mismatch") {
			t.Fatalf("unexpected error: %v", err)
		}
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			t.Fatalf("temp file not cleaned up: %s", e.Name())
		}
	})

	t.Run("http 404", func(t *testing.T) {
		srv := httptest.NewServer(http.NotFoundHandler())
		defer srv.Close()

		_, _, err := DownloadPackage(DownloadOptions{
			URL:    srv.URL + "/missing.avm.zip",
			Dir:    t.TempDir(),
			Client: srv.Client(),
		})
		if err == nil {
			t.Fatal("expected error for 404")
		}
		if got := err.Error(); !strings.Contains(got, "HTTP 404") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("http 500", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(500)
		}))
		defer srv.Close()

		_, _, err := DownloadPackage(DownloadOptions{
			URL:    srv.URL + "/error.avm.zip",
			Dir:    t.TempDir(),
			Client: srv.Client(),
		})
		if err == nil {
			t.Fatal("expected error for 500")
		}
		if got := err.Error(); !strings.Contains(got, "HTTP 500") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("cleanup removes file", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write(content)
		}))
		defer srv.Close()

		path, cleanup, err := DownloadPackage(DownloadOptions{
			URL:    srv.URL + "/pkg.avm.zip",
			Dir:    t.TempDir(),
			Client: srv.Client(),
		})
		if err != nil {
			t.Fatalf("DownloadPackage error: %v", err)
		}
		cleanup()
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("cleanup did not remove file, stat err: %v", err)
		}
	})
}

func TestVerifyChecksum(t *testing.T) {
	content := []byte("test content for checksum")
	h := sha256.Sum256(content)
	correctChecksum := "sha256:" + hex.EncodeToString(h[:])

	path := filepath.Join(t.TempDir(), "test.zip")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	t.Run("correct", func(t *testing.T) {
		if err := VerifyChecksum(path, correctChecksum); err != nil {
			t.Fatalf("VerifyChecksum error: %v", err)
		}
	})

	t.Run("wrong hash", func(t *testing.T) {
		err := VerifyChecksum(path, "sha256:0000000000000000000000000000000000000000000000000000000000000000")
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "checksum mismatch") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("malformed format", func(t *testing.T) {
		err := VerifyChecksum(path, "nocolon")
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "invalid checksum format") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("unsupported algorithm", func(t *testing.T) {
		err := VerifyChecksum(path, fmt.Sprintf("md5:%s", hex.EncodeToString(h[:])))
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "unsupported checksum algorithm") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	t.Run("invalid hex in checksum", func(t *testing.T) {
		err := VerifyChecksum(path, "sha256:not-valid-hex")
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "invalid checksum hex") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}
