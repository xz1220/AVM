package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/xz1220/agent-vm/internal/adapter"
	"github.com/xz1220/agent-vm/internal/config"
	avmruntime "github.com/xz1220/agent-vm/internal/runtime"
)

const initImportReportVersion = "1"

type initAdapterRegistry interface {
	Get(runtime string) (adapter.Adapter, bool)
}

type initImportReport struct {
	Version     string                    `json:"version"`
	GeneratedAt string                    `json:"generated_at"`
	Runtimes    []initRuntimeImportReport `json:"runtimes"`
	Warnings    []string                  `json:"warnings"`
	Errors      []string                  `json:"errors"`
}

type initRuntimeImportReport struct {
	Runtime         string                  `json:"runtime"`
	Found           bool                    `json:"found"`
	ConfigDir       string                  `json:"config_dir"`
	Version         string                  `json:"version"`
	AgentCandidates []adapter.ImportedAgent `json:"agent_candidates"`
	Warnings        []string                `json:"warnings"`
	Errors          []string                `json:"errors"`
}

var (
	newInitAdapterRegistry = func() initAdapterRegistry {
		return avmruntime.NewRegistry()
	}
	initImportNow = func() time.Time {
		return time.Now().UTC()
	}
	initRuntimeNames = registeredInitRuntimeNames
)

func refreshInitImportReport() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	report := buildInitImportReport(ctx, newInitAdapterRegistry(), initImportNow().UTC())
	return saveInitImportReport(initImportReportPath(), report)
}

func buildInitImportReport(ctx context.Context, registry initAdapterRegistry, now time.Time) initImportReport {
	report := initImportReport{
		Version:     initImportReportVersion,
		GeneratedAt: now.UTC().Format(time.RFC3339Nano),
		Runtimes:    []initRuntimeImportReport{},
		Warnings:    []string{},
		Errors:      []string{},
	}
	if registry == nil {
		report.Errors = append(report.Errors, "runtime registry unavailable")
		return report
	}

	for _, runtimeName := range initRuntimeNames(registry) {
		adp, ok := registry.Get(runtimeName)
		if !ok || adp == nil {
			continue
		}
		report.Runtimes = append(report.Runtimes, scanInitRuntime(ctx, runtimeName, adp))
	}

	return report
}

func scanInitRuntime(ctx context.Context, runtimeName string, adp adapter.Adapter) initRuntimeImportReport {
	detection := adp.Detect(ctx)
	if detection.Runtime != "" {
		runtimeName = detection.Runtime
	}

	runtimeReport := initRuntimeImportReport{
		Runtime:         runtimeName,
		Found:           detection.Found,
		ConfigDir:       detection.ConfigDir,
		Version:         detection.Version,
		AgentCandidates: []adapter.ImportedAgent{},
		Warnings:        append([]string{}, detection.Warnings...),
		Errors:          []string{},
	}

	imported, err := adp.Import(ctx)
	if err != nil {
		runtimeReport.Errors = append(runtimeReport.Errors, err.Error())
		return runtimeReport
	}
	if imported == nil {
		runtimeReport.Warnings = append(runtimeReport.Warnings, "runtime import returned no result")
		return runtimeReport
	}

	runtimeReport.AgentCandidates = append(runtimeReport.AgentCandidates, imported.Agents...)
	runtimeReport.Warnings = append(runtimeReport.Warnings, imported.Warnings...)
	return runtimeReport
}

func registeredInitRuntimeNames(registry initAdapterRegistry) []string {
	names := make([]string, 0, len(config.KnownTargets))
	for name := range config.KnownTargets {
		if _, ok := registry.Get(name); ok {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func initImportReportPath() string {
	return filepath.Join(config.StateDir(), "import-report.json")
}

func saveInitImportReport(path string, report initImportReport) error {
	parent := filepath.Dir(path)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return err
	}

	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')

	tmp, err := os.CreateTemp(parent, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
