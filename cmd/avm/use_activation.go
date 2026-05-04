package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/xz1220/agent-vm/internal/boundary"
	"github.com/xz1220/agent-vm/internal/config"
	avmruntime "github.com/xz1220/agent-vm/internal/runtime"
	avmsync "github.com/xz1220/agent-vm/internal/sync"
)

type activationResult struct {
	Active     config.ActiveRef
	SyncStatus string
	Targets    []activationTargetResult
	Warnings   []string
}

type activationTargetResult struct {
	Runtime     string
	Status      avmsync.TargetStatus
	AgentName   string
	Boundary    boundary.RuntimeBoundary
	RuntimeHome string
}

type activationResolveError struct {
	kind     string
	name     string
	err      error
	notFound bool
}

func (e *activationResolveError) Error() string {
	if e.notFound {
		return fmt.Sprintf("%s %q not found", e.kind, e.name)
	}
	return fmt.Sprintf("%s %q could not be resolved: %v", e.kind, e.name, e.err)
}

func (e *activationResolveError) Unwrap() error {
	return e.err
}

func resolveActivationFromCommand(cmd *cobra.Command, name, cwd string) (*config.ResolvedActivation, error) {
	kind, err := cmd.Flags().GetString("kind")
	if err != nil {
		return nil, err
	}
	kind = strings.TrimSpace(kind)
	switch kind {
	case "":
		return resolveActivationAuto(name, cwd)
	case config.ActiveKindProfile, config.ActiveKindEnv:
		return resolveActivationRef(config.ActiveRef{Kind: kind, Name: name}, cwd)
	default:
		return nil, fmt.Errorf("invalid activation kind %q (want profile or env)", kind)
	}
}

func resolveActivationAuto(name, cwd string) (*config.ResolvedActivation, error) {
	resolved, profileErr := resolveActivationRef(config.ActiveRef{Kind: config.ActiveKindProfile, Name: name}, cwd)
	if profileErr == nil {
		return resolved, nil
	}

	resolved, envErr := resolveActivationRef(config.ActiveRef{Kind: config.ActiveKindEnv, Name: name}, cwd)
	if envErr == nil {
		return resolved, nil
	}

	if isActivationNotFound(profileErr) && isActivationNotFound(envErr) {
		return nil, fmt.Errorf("activation %q not found as profile or env", name)
	}
	return nil, fmt.Errorf("activation %q could not be resolved as profile or env: profile=%v; env=%v", name, profileErr, envErr)
}

func resolveActivationRef(ref config.ActiveRef, cwd string) (*config.ResolvedActivation, error) {
	if err := config.ValidateActiveRef(ref); err != nil {
		return nil, &activationResolveError{kind: ref.Kind, name: ref.Name, err: err}
	}

	resolved, err := config.ResolveActivation(ref, cwd)
	if err != nil {
		return nil, &activationResolveError{
			kind:     ref.Kind,
			name:     ref.Name,
			err:      err,
			notFound: !activationTargetExists(ref, cwd),
		}
	}
	return resolved, nil
}

func isActivationNotFound(err error) bool {
	var resolveErr *activationResolveError
	return errors.As(err, &resolveErr) && resolveErr.notFound
}

func activationTargetExists(ref config.ActiveRef, cwd string) bool {
	var paths []string
	switch ref.Kind {
	case config.ActiveKindProfile:
		paths = []string{
			config.ProjectAgentPath(cwd, ref.Name),
			config.AgentPath(ref.Name),
		}
	case config.ActiveKindEnv:
		paths = []string{config.EnvPath(ref.Name)}
	}
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil || !os.IsNotExist(err) {
			return true
		}
	}
	return false
}

func applyActivation(resolved *config.ResolvedActivation, cwd string) (*activationResult, error) {
	if resolved == nil {
		return nil, fmt.Errorf("resolved activation is nil")
	}
	syncOpts := activationSyncOptions(resolved, cwd)
	syncer := avmsync.NewSyncer(avmruntime.NewRegistry())

	syncResult, err := syncer.SyncActivation(context.Background(), resolved, syncOpts)
	if syncResult == nil {
		return nil, err
	}
	if !syncOpts.DryRun {
		if currentErr := writeCurrentActive(syncResult.Active); currentErr != nil {
			return nil, currentErr
		}
	}
	return activationResultFromSync(syncResult), err
}

func activationResultFromSync(result *avmsync.Result) *activationResult {
	out := &activationResult{
		Active:     result.Active,
		SyncStatus: "completed",
		Warnings:   append([]string(nil), result.Warnings...),
	}
	for _, target := range result.Targets {
		out.Targets = append(out.Targets, activationTargetResult{
			Runtime:     target.Runtime,
			Status:      target.Status,
			AgentName:   target.AgentName,
			Boundary:    target.Boundary,
			RuntimeHome: target.RuntimeHome,
		})
		for _, warning := range target.Warnings {
			out.Warnings = append(out.Warnings, fmt.Sprintf("%s: %s", target.Runtime, warning))
		}
		if target.Error != "" {
			out.Warnings = append(out.Warnings, fmt.Sprintf("%s: %s", target.Runtime, target.Error))
		}
	}
	return out
}

func activationSyncOptions(resolved *config.ResolvedActivation, cwd string) avmsync.Options {
	return avmsync.Options{
		ProjectRoot:  cwd,
		ActiveDir:    config.ActiveDir(),
		UpdateActive: true,
		Targets:      append([]string(nil), resolved.Targets...),
	}
}

func printActivationResult(out io.Writer, result *activationResult) {
	fmt.Fprintf(out, "active: %s\n", formatActiveRef(result.Active))
	syncStatus := result.SyncStatus
	if syncStatus == "" {
		syncStatus = "unknown"
	}
	fmt.Fprintf(out, "sync: %s\n", syncStatus)
	fmt.Fprintln(out, "targets:")
	if len(result.Targets) == 0 {
		fmt.Fprintln(out, "  none")
	} else {
		for _, target := range result.Targets {
			fmt.Fprintf(out, "  %s: %s\n", target.Runtime, target.Status)
		}
	}
	fmt.Fprintln(out, "warnings:")
	if len(result.Warnings) == 0 {
		fmt.Fprintln(out, "  none")
		return
	}
	for _, warning := range result.Warnings {
		fmt.Fprintf(out, "  - %s\n", warning)
	}
}

func writeCurrentActive(ref config.ActiveRef) error {
	if err := os.MkdirAll(config.StateDir(), 0o700); err != nil {
		return err
	}
	return os.WriteFile(currentActivePath(), []byte(formatActiveRef(ref)+"\n"), 0o600)
}

func currentActivePath() string {
	return filepath.Join(config.StateDir(), "current-active")
}

func syncStatePath() string {
	return filepath.Join(config.StateDir(), "sync-state.json")
}

func formatActiveRef(ref config.ActiveRef) string {
	if ref.Kind == "" || ref.Name == "" {
		return "none"
	}
	return ref.Kind + ":" + ref.Name
}

func runtimeAgentKeys(agents map[string]config.AgentProfile) []string {
	runtimes := make([]string, 0, len(agents))
	for runtime := range agents {
		runtimes = append(runtimes, runtime)
	}
	sort.Strings(runtimes)
	return runtimes
}
