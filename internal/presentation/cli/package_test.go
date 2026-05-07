package cli

import (
	"strings"
	"testing"

	"github.com/xz1220/agent-vm/internal/app/model"
	"github.com/xz1220/agent-vm/internal/app/service"
)

func TestPackageList_EmptyShowsNotImplementedHint(t *testing.T) {
	deps := newTestDeps(nil, &fakePackages{}, nil, nil, nil)
	out, _, err := runCmd(t, deps, "package", "list")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, "(installed-package registry not yet implemented") {
		t.Fatalf("expected hint, got: %q", out)
	}
}

func TestPackageShow_RegistryNotSupportedExitsZero(t *testing.T) {
	deps := newTestDeps(nil, &fakePackages{showErr: service.ErrPackageRegistryNotSupported}, nil, nil, nil)
	out, _, err := runCmd(t, deps, "package", "show", "demo")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, "(installed-package registry not yet implemented") {
		t.Fatalf("expected hint, got: %q", out)
	}
}

func TestPackageInspect(t *testing.T) {
	pkgs := &fakePackages{
		inspectResp: &model.PackageDetail{
			Manifest: model.PackageManifest{
				Name:    "demo",
				Version: "1.0.0",
				Agents: []model.PackageAgentRef{
					{Name: "alpha", Path: "agents/alpha.yaml"},
				},
			},
			Source: "demo.avm.zip",
		},
	}
	deps := newTestDeps(nil, pkgs, nil, nil, nil)
	out, _, err := runCmd(t, deps, "package", "inspect", "demo.avm.zip")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	for _, want := range []string{"demo", "1.0.0", "alpha", "agents/alpha.yaml"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in output, got: %q", want, out)
		}
	}
}

func TestPackageInstall_NonInteractive(t *testing.T) {
	pkgs := &fakePackages{
		installRes: &model.InstallResult{
			Manifest:        model.PackageManifest{Name: "demo"},
			InstalledAgents: []string{"alpha"},
		},
	}
	deps := newTestDeps(nil, pkgs, nil, nil, nil)
	out, _, err := runCmd(t, deps, "--non-interactive", "package", "install", "demo.avm.zip")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, "Installed package") || !strings.Contains(out, "alpha") {
		t.Fatalf("unexpected output: %q", out)
	}
	if len(pkgs.installReqs) != 1 || pkgs.installReqs[0].Source != "demo.avm.zip" {
		t.Fatalf("unexpected install reqs: %+v", pkgs.installReqs)
	}
}

func TestPackageExport(t *testing.T) {
	pkgs := &fakePackages{exportPath: "/tmp/alpha.avm.zip"}
	deps := newTestDeps(nil, pkgs, nil, nil, nil)
	out, _, err := runCmd(t, deps, "package", "export", "alpha", "-o", "/tmp/alpha.avm.zip")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, "Wrote /tmp/alpha.avm.zip") {
		t.Fatalf("unexpected output: %q", out)
	}
	if len(pkgs.exportCalls) != 1 || pkgs.exportCalls[0].Agent != "alpha" {
		t.Fatalf("unexpected export call: %+v", pkgs.exportCalls)
	}
}

func TestPackageUninstall_NonInteractive(t *testing.T) {
	pkgs := &fakePackages{}
	deps := newTestDeps(nil, pkgs, nil, nil, nil)
	out, _, err := runCmd(t, deps, "--non-interactive", "package", "uninstall", "alpha")
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out, `Uninstalled "alpha"`) {
		t.Fatalf("unexpected output: %q", out)
	}
	if len(pkgs.uninstalled) != 1 || pkgs.uninstalled[0] != "alpha" {
		t.Fatalf("unexpected uninstalled: %v", pkgs.uninstalled)
	}
}
