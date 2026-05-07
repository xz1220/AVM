package packageio

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/xz1220/agent-vm/internal/app/model"
)

func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// buildPayload returns a zip stream containing the named files.
func buildPayload(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, data := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestWriteReadVerify(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "pkg.avm.zip")

	skillBytes := []byte("# skill content\n")
	manifest := &model.PackageManifest{
		SchemaVersion: "1",
		Name:          "demo",
		Version:       "0.1.0",
		CreatedAt:     time.Unix(1700000000, 0).UTC(),
		Agents: []model.PackageAgentRef{
			{Name: "agent-x", Path: "agents/agent-x.yaml"},
		},
		Capabilities: []model.PackageCapBlob{
			{
				Kind:     model.CapabilityKindSkill,
				Name:     "demo-skill",
				Path:     "capabilities/demo-skill.md",
				Checksum: sha256Hex(skillBytes),
			},
		},
	}
	payload := buildPayload(t, map[string][]byte{
		"agents/agent-x.yaml":        []byte("identity:\n  name: agent-x\n"),
		"capabilities/demo-skill.md": skillBytes,
	})

	pio := New()
	if err := pio.Write(manifest, bytes.NewReader(payload), dst); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if err := pio.Verify(dst); err != nil {
		t.Fatalf("Verify: %v", err)
	}

	gotManifest, h, err := pio.Read(dst)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	defer h.Close()
	if gotManifest.Name != "demo" || gotManifest.Version != "0.1.0" {
		t.Fatalf("manifest: %+v", gotManifest)
	}
	if len(h.Files()) == 0 {
		t.Fatalf("Files() empty")
	}

	rc, err := h.Open("capabilities/demo-skill.md")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	got, err := io.ReadAll(rc)
	rc.Close()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, skillBytes) {
		t.Fatalf("payload mismatch: %q vs %q", got, skillBytes)
	}
}

func TestRead_NotZip(t *testing.T) {
	dir := t.TempDir()
	notZip := filepath.Join(dir, "x.zip")
	if err := os.WriteFile(notZip, []byte("definitely not a zip"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := New().Read(notZip); err == nil {
		t.Fatal("expected error reading non-zip file")
	}
}

func TestVerify_BadChecksum(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "bad.avm.zip")
	skill := []byte("real")
	manifest := &model.PackageManifest{
		SchemaVersion: "1",
		Name:          "x",
		Version:       "0",
		Capabilities: []model.PackageCapBlob{
			{
				Kind:     model.CapabilityKindSkill,
				Name:     "x",
				Path:     "capabilities/x",
				Checksum: sha256Hex([]byte("not-real")),
			},
		},
	}
	payload := buildPayload(t, map[string][]byte{"capabilities/x": skill})
	if err := New().Write(manifest, bytes.NewReader(payload), dst); err != nil {
		t.Fatal(err)
	}
	err := New().Verify(dst)
	if err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("expected checksum mismatch, got %v", err)
	}
}

func TestVerify_UnsafePath(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "unsafe.avm.zip")

	// Hand-roll a zip that includes a "../escape" entry alongside a
	// minimal manifest.
	manifest := &model.PackageManifest{SchemaVersion: "1", Name: "x", Version: "0"}
	mb, err := yaml.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create(ManifestName)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(mb); err != nil {
		t.Fatal(err)
	}
	w2, err := zw.Create("../escape")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w2.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := New().Verify(dst); err == nil {
		t.Fatal("expected unsafe-path error")
	}
}

func TestWrite_NilManifest(t *testing.T) {
	if err := New().Write(nil, nil, "x.zip"); err == nil {
		t.Fatal("expected error for nil manifest")
	}
}
