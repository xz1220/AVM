package home

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultLayout_EnvOverride(t *testing.T) {
	want := t.TempDir()
	t.Setenv(EnvVar, want)
	l, err := DefaultLayout()
	if err != nil {
		t.Fatalf("DefaultLayout: %v", err)
	}
	if l.Root != want {
		t.Fatalf("root: got %q want %q", l.Root, want)
	}
}

func TestDefaultLayout_FallbackHome(t *testing.T) {
	// Clear env and force a fake HOME so we don't depend on the real one.
	t.Setenv(EnvVar, "")
	fake := t.TempDir()
	t.Setenv("HOME", fake)
	l, err := DefaultLayout()
	if err != nil {
		t.Fatalf("DefaultLayout: %v", err)
	}
	if !strings.HasSuffix(l.Root, ".avm") {
		t.Fatalf("expected .avm suffix, got %q", l.Root)
	}
	if filepath.Dir(l.Root) != fake {
		t.Fatalf("root parent: got %q want %q", filepath.Dir(l.Root), fake)
	}
}

func TestLayoutSubdirs(t *testing.T) {
	l := Layout{Root: "/tmp/x"}
	cases := map[string]string{
		"agents":       l.AgentsDir(),
		"capabilities": l.CapabilitiesDir(),
		"packages":     l.PackagesDir(),
		"runlog":       l.RunLogDir(),
		"boundaries":   l.BoundaryDir(),
	}
	for suffix, p := range cases {
		if filepath.Base(p) != suffix {
			t.Errorf("%s: got %q", suffix, p)
		}
		if filepath.Dir(p) != "/tmp/x" {
			t.Errorf("%s: parent mismatch %q", suffix, p)
		}
	}
}

func TestEnsureDirs(t *testing.T) {
	root := t.TempDir()
	l := Layout{Root: root}
	if err := l.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs: %v", err)
	}
	for _, p := range []string{l.AgentsDir(), l.CapabilitiesDir(), l.PackagesDir(), l.RunLogDir(), l.BoundaryDir()} {
		st, err := os.Stat(p)
		if err != nil {
			t.Fatalf("stat %s: %v", p, err)
		}
		if !st.IsDir() {
			t.Fatalf("not a dir: %s", p)
		}
	}
	// Idempotent.
	if err := l.EnsureDirs(); err != nil {
		t.Fatalf("second EnsureDirs: %v", err)
	}
}
