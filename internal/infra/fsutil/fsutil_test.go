package fsutil

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureWithin(t *testing.T) {
	root := t.TempDir()
	if err := EnsureWithin(root, filepath.Join(root, "a", "b")); err != nil {
		t.Fatalf("expected ok, got %v", err)
	}
	if err := EnsureWithin(root, root); err != nil {
		t.Fatalf("expected ok for root itself, got %v", err)
	}
	if err := EnsureWithin(root, filepath.Join(root, "..", "evil")); err == nil {
		t.Fatalf("expected error for escape via ..")
	}
	// absolute path outside root
	if err := EnsureWithin(root, "/etc/passwd"); err == nil {
		t.Fatalf("expected error for absolute path outside root")
	}
}

func TestAtomicWriteFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "sub", "out.txt")
	want := []byte("hello world")
	if err := AtomicWriteFile(target, want, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("content mismatch: got %q want %q", got, want)
	}
	st, err := os.Stat(target)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("mode mismatch: got %o want %o", st.Mode().Perm(), 0o600)
	}
	// no leftover temp files in dir
	entries, err := os.ReadDir(filepath.Dir(target))
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".tmp-") {
			t.Fatalf("leftover temp file: %s", e.Name())
		}
	}

	// overwrite
	if err := AtomicWriteFile(target, []byte("again"), 0o644); err != nil {
		t.Fatalf("overwrite: %v", err)
	}
	got, _ = os.ReadFile(target)
	if string(got) != "again" {
		t.Fatalf("overwrite content: %q", got)
	}
}

func TestAtomicWriteFile_EmptyPath(t *testing.T) {
	if err := AtomicWriteFile("", []byte("x"), 0o600); err == nil {
		t.Fatalf("expected error for empty path")
	}
}

func TestSha256Sum(t *testing.T) {
	got, err := Sha256Sum(bytes.NewReader([]byte("abc")))
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	const want = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
	if _, err := Sha256Sum(nil); err == nil {
		t.Fatalf("expected error for nil reader")
	}
}
