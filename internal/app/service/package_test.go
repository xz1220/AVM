package service

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/xz1220/agent-vm/internal/app/model"
	"github.com/xz1220/agent-vm/internal/infra/agentstore"
	"github.com/xz1220/agent-vm/internal/infra/capstore"
	"github.com/xz1220/agent-vm/internal/infra/packageio"
)

// buildTestPkg writes a package zip to disk and returns the path.
func buildTestPkg(t *testing.T, dir string, manifest *model.PackageManifest, files map[string][]byte) string {
	t.Helper()
	dst := filepath.Join(dir, manifest.Name+".avm.zip")
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
	if err := packageio.New().Write(manifest, bytes.NewReader(buf.Bytes()), dst); err != nil {
		t.Fatalf("Write: %v", err)
	}
	return dst
}

func sumHex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func TestPackages_Install_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	skill := []byte("# skill content\n")
	agentYAML := []byte(`identity:
  name: alpha
runtimes:
  - runtime: codex
    default: true
skills:
  - id: demo-skill
    kind: skill
`)
	manifest := &model.PackageManifest{
		SchemaVersion: "1",
		Name:          "demo",
		Version:       "0.1.0",
		CreatedAt:     time.Unix(1, 0).UTC(),
		Agents: []model.PackageAgentRef{
			{Name: "alpha", Path: "agents/alpha.yaml"},
		},
		Capabilities: []model.PackageCapBlob{
			{
				Kind:     model.CapabilityKindSkill,
				Name:     "demo-skill",
				Path:     "capabilities/demo-skill.md",
				Checksum: sumHex(skill),
			},
		},
	}
	pkgPath := buildTestPkg(t, dir, manifest, map[string][]byte{
		"agents/alpha.yaml":          agentYAML,
		"capabilities/demo-skill.md": skill,
	})

	agents := agentstore.New(t.TempDir())
	caps := capstore.New(t.TempDir())
	pkgs := NewPackages(agents, caps, packageio.New())

	res, err := pkgs.Install(context.Background(), model.InstallRequest{Source: pkgPath})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if len(res.InstalledAgents) != 1 || res.InstalledAgents[0] != "alpha" {
		t.Fatalf("InstalledAgents=%+v", res.InstalledAgents)
	}
	if len(res.ImportedCaps) != 1 {
		t.Fatalf("ImportedCaps=%+v", res.ImportedCaps)
	}

	// CapabilityRef inside agent should now point at the local store ID.
	got, err := agents.Get("alpha")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got.Skills) != 1 {
		t.Fatalf("skills=%+v", got.Skills)
	}
	if got.Skills[0].ID != res.ImportedCaps[0] {
		t.Fatalf("skill id %q want imported %q", got.Skills[0].ID, res.ImportedCaps[0])
	}

	// Idempotent capability import: installing again should reuse ID.
	if err := agents.Delete("alpha"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	res2, err := pkgs.Install(context.Background(), model.InstallRequest{Source: pkgPath})
	if err != nil {
		t.Fatalf("Install #2: %v", err)
	}
	if res.ImportedCaps[0] != res2.ImportedCaps[0] {
		t.Fatalf("cap id changed across installs: %s vs %s", res.ImportedCaps[0], res2.ImportedCaps[0])
	}
}

func TestPackages_Install_RequiresRuntime(t *testing.T) {
	dir := t.TempDir()
	manifest := &model.PackageManifest{
		SchemaVersion: "1", Name: "demo", Version: "0",
		Agents: []model.PackageAgentRef{{Name: "alpha", Path: "agents/alpha.yaml"}},
	}
	pkgPath := buildTestPkg(t, dir, manifest, map[string][]byte{
		"agents/alpha.yaml": []byte("identity:\n  name: alpha\n"),
	})
	agents := agentstore.New(t.TempDir())
	pkgs := NewPackages(agents, capstore.New(t.TempDir()), packageio.New())
	_, err := pkgs.Install(context.Background(), model.InstallRequest{Source: pkgPath})
	if err == nil {
		t.Fatal("expected MISSING_INPUT for package agent with no runtimes")
	}
	se := AsError(err)
	if se == nil || se.Code != CodeMissingInput {
		t.Fatalf("expected MISSING_INPUT, got %T %v", err, err)
	}
	if got, _ := se.Details["field"].(string); got != "runtime" {
		t.Fatalf("details.field=%v want runtime", se.Details["field"])
	}
	if agents.Exists("alpha") {
		t.Fatal("agent should not be persisted on validation failure")
	}
}

func TestPackages_Install_ConflictAskNonInteractive(t *testing.T) {
	dir := t.TempDir()
	manifest := &model.PackageManifest{
		SchemaVersion: "1", Name: "demo", Version: "0",
		Agents: []model.PackageAgentRef{{Name: "alpha", Path: "agents/alpha.yaml"}},
	}
	pkgPath := buildTestPkg(t, dir, manifest, map[string][]byte{
		"agents/alpha.yaml": []byte("identity:\n  name: alpha\nruntimes:\n  - runtime: codex\n"),
	})
	agents := agentstore.New(t.TempDir())
	if err := agents.Save(&model.Agent{Identity: model.Identity{Name: "alpha"}}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	pkgs := NewPackages(agents, capstore.New(t.TempDir()), packageio.New())
	_, err := pkgs.Install(context.Background(), model.InstallRequest{
		Source:     pkgPath,
		Resolution: model.ResolveAsk,
	})
	if !errors.Is(err, agentstore.ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}

func TestPackages_Install_ConflictRename(t *testing.T) {
	dir := t.TempDir()
	manifest := &model.PackageManifest{
		SchemaVersion: "1", Name: "demo", Version: "0",
		Agents: []model.PackageAgentRef{{Name: "alpha", Path: "agents/alpha.yaml"}},
	}
	pkgPath := buildTestPkg(t, dir, manifest, map[string][]byte{
		"agents/alpha.yaml": []byte("identity:\n  name: alpha\nruntimes:\n  - runtime: codex\n"),
	})
	agents := agentstore.New(t.TempDir())
	if err := agents.Save(&model.Agent{Identity: model.Identity{Name: "alpha"}}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	pkgs := NewPackages(agents, capstore.New(t.TempDir()), packageio.New())
	res, err := pkgs.Install(context.Background(), model.InstallRequest{
		Source:     pkgPath,
		Resolution: model.ResolveRename,
	})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if res.Renamed["alpha"] != "alpha-1" {
		t.Fatalf("Renamed=%+v", res.Renamed)
	}
	if !agents.Exists("alpha-1") {
		t.Fatal("renamed agent not on disk")
	}
}

func TestPackages_Install_ConflictSkip(t *testing.T) {
	dir := t.TempDir()
	manifest := &model.PackageManifest{
		SchemaVersion: "1", Name: "demo", Version: "0",
		Agents: []model.PackageAgentRef{{Name: "alpha", Path: "agents/alpha.yaml"}},
	}
	pkgPath := buildTestPkg(t, dir, manifest, map[string][]byte{
		"agents/alpha.yaml": []byte("identity:\n  name: alpha\n  description: pkg\nruntimes:\n  - runtime: codex\n"),
	})
	agents := agentstore.New(t.TempDir())
	if err := agents.Save(&model.Agent{Identity: model.Identity{Name: "alpha", Description: "original"}}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	pkgs := NewPackages(agents, capstore.New(t.TempDir()), packageio.New())
	res, err := pkgs.Install(context.Background(), model.InstallRequest{
		Source:     pkgPath,
		Resolution: model.ResolveSkip,
	})
	if err != nil {
		t.Fatalf("Install: %v", err)
	}
	if len(res.Skipped) != 1 {
		t.Fatalf("Skipped=%+v", res.Skipped)
	}
	got, err := agents.Get("alpha")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Identity.Description != "original" {
		t.Fatalf("agent was overwritten by skip: %+v", got.Identity)
	}
}

func TestPackages_Install_ConflictOverwrite(t *testing.T) {
	dir := t.TempDir()
	manifest := &model.PackageManifest{
		SchemaVersion: "1", Name: "demo", Version: "0",
		Agents: []model.PackageAgentRef{{Name: "alpha", Path: "agents/alpha.yaml"}},
	}
	pkgPath := buildTestPkg(t, dir, manifest, map[string][]byte{
		"agents/alpha.yaml": []byte("identity:\n  name: alpha\n  description: pkg\nruntimes:\n  - runtime: codex\n"),
	})
	agents := agentstore.New(t.TempDir())
	if err := agents.Save(&model.Agent{Identity: model.Identity{Name: "alpha", Description: "original"}}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	pkgs := NewPackages(agents, capstore.New(t.TempDir()), packageio.New())
	if _, err := pkgs.Install(context.Background(), model.InstallRequest{
		Source:     pkgPath,
		Resolution: model.ResolveOverwrite,
	}); err != nil {
		t.Fatalf("Install: %v", err)
	}
	got, err := agents.Get("alpha")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Identity.Description != "pkg" {
		t.Fatalf("agent not overwritten: %+v", got.Identity)
	}
}

func TestPackages_Inspect(t *testing.T) {
	dir := t.TempDir()
	manifest := &model.PackageManifest{
		SchemaVersion: "1", Name: "demo", Version: "0",
	}
	pkgPath := buildTestPkg(t, dir, manifest, map[string][]byte{
		"some/extra.txt": []byte("hi"),
	})
	pkgs := NewPackages(nil, nil, packageio.New())
	det, err := pkgs.Inspect(context.Background(), pkgPath)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if det.Manifest.Name != "demo" {
		t.Fatalf("manifest=%+v", det.Manifest)
	}
	if det.Source != pkgPath {
		t.Fatalf("source=%q", det.Source)
	}
	if len(det.Files) == 0 {
		t.Fatal("Files empty")
	}
}

func TestPackages_Uninstall(t *testing.T) {
	agents := agentstore.New(t.TempDir())
	if err := agents.Save(&model.Agent{Identity: model.Identity{Name: "alpha"}}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	pkgs := NewPackages(agents, capstore.New(t.TempDir()), packageio.New())
	if err := pkgs.Uninstall(context.Background(), "alpha"); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if agents.Exists("alpha") {
		t.Fatal("agent still exists after uninstall")
	}
}

func TestPackages_Export_RoundTrip(t *testing.T) {
	agentDir := t.TempDir()
	capDir := t.TempDir()
	outDir := t.TempDir()

	agents := agentstore.New(agentDir)
	caps := capstore.New(capDir)

	// Add a capability and reference it from an agent.
	id, err := caps.Add(model.CapabilityRecord{
		Kind: model.CapabilityKindSkill, Name: "demo-skill",
	}, bytes.NewReader([]byte("# skill body\n")))
	if err != nil {
		t.Fatalf("caps.Add: %v", err)
	}
	if err := agents.Save(&model.Agent{
		Identity: model.Identity{Name: "alpha", Description: "exp"},
		Skills:   []model.CapabilityRef{{ID: id, Kind: model.CapabilityKindSkill}},
	}); err != nil {
		t.Fatalf("agents.Save: %v", err)
	}

	pkgs := NewPackages(agents, caps, packageio.New())
	out := filepath.Join(outDir, "alpha.avm.zip")
	res, err := pkgs.Export(context.Background(), model.ExportRequest{
		Agent:         "alpha",
		IncludeSkills: true,
		OutputPath:    out,
	})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if res.Path != out {
		t.Fatalf("path=%q", res.Path)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatalf("stat: %v", err)
	}

	// Verify the package: should be readable, contain the agent and skill.
	pio := packageio.New()
	if err := pio.Verify(out); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	mf, h, err := pio.Read(out)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	defer h.Close()
	if mf.Name != "alpha" {
		t.Fatalf("manifest name=%q", mf.Name)
	}
	if len(mf.Agents) != 1 || len(mf.Capabilities) != 1 {
		t.Fatalf("manifest=%+v", mf)
	}

	// Read agent yaml and confirm it parses.
	rc, err := h.Open(mf.Agents[0].Path)
	if err != nil {
		t.Fatalf("Open agent: %v", err)
	}
	data, _ := io.ReadAll(rc)
	rc.Close()
	var got model.Agent
	if err := yaml.Unmarshal(data, &got); err != nil {
		t.Fatalf("agent yaml: %v", err)
	}
	if got.Identity.Name != "alpha" {
		t.Fatalf("agent=%+v", got.Identity)
	}
}

func TestPackages_Export_NoCapsWhenFlagsFalse(t *testing.T) {
	agents := agentstore.New(t.TempDir())
	caps := capstore.New(t.TempDir())
	id, err := caps.Add(model.CapabilityRecord{
		Kind: model.CapabilityKindSkill, Name: "demo",
	}, bytes.NewReader([]byte("x")))
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if err := agents.Save(&model.Agent{
		Identity: model.Identity{Name: "alpha"},
		Skills:   []model.CapabilityRef{{ID: id, Kind: model.CapabilityKindSkill}},
	}); err != nil {
		t.Fatalf("save: %v", err)
	}
	out := filepath.Join(t.TempDir(), "alpha.avm.zip")
	pkgs := NewPackages(agents, caps, packageio.New())
	res, err := pkgs.Export(context.Background(), model.ExportRequest{
		Agent:      "alpha",
		OutputPath: out,
		// IncludeSkills false
	})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if len(res.Manifest.Capabilities) != 0 {
		t.Fatalf("expected no capabilities, got %+v", res.Manifest.Capabilities)
	}
}
