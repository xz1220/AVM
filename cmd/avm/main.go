// Command avm is the AVM CLI entry point. It is intentionally tiny:
// composition lives in app/wire.go; presentation lives in internal/presentation/cli;
// product rules live in internal/app/service.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/xz1220/agent-vm/internal/app/service"
	"github.com/xz1220/agent-vm/internal/infra/agentstore"
	"github.com/xz1220/agent-vm/internal/infra/capstore"
	"github.com/xz1220/agent-vm/internal/infra/home"
	"github.com/xz1220/agent-vm/internal/infra/managedfile"
	"github.com/xz1220/agent-vm/internal/infra/packageio"
	"github.com/xz1220/agent-vm/internal/infra/process"
	"github.com/xz1220/agent-vm/internal/infra/runlog"
	"github.com/xz1220/agent-vm/internal/presentation/cli"
	"github.com/xz1220/agent-vm/internal/runtime"
	"github.com/xz1220/agent-vm/internal/runtime/claudecode"
	"github.com/xz1220/agent-vm/internal/runtime/codex"
	"github.com/xz1220/agent-vm/internal/runtime/opencode"
)

func main() {
	os.Exit(run())
}

// run is the bootstrap. It returns the exit code rather than an error
// because the CLI layer has already rendered any error (human or JSON)
// via cli.NewRoot's RunE wrapper. Bootstrap-time errors (composition-
// root failures) bypass that wrapper, so we print them ourselves.
func run() int {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	deps, err := buildDeps()
	if err != nil {
		fmt.Fprintf(os.Stderr, "avm: %v\n", err)
		return 1
	}
	root := cli.NewRoot(deps)
	root.SetContext(ctx)
	if err := root.ExecuteContext(ctx); err != nil {
		// Honour exit-code-bearing errors so `avm run` can propagate
		// the underlying runtime's exit code to the shell.
		if ec, ok := err.(interface{ ExitCode() int }); ok {
			return ec.ExitCode()
		}
		return 1
	}
	return 0
}

func buildDeps() (cli.Deps, error) {
	layout, err := home.DefaultLayout()
	if err != nil {
		return cli.Deps{}, fmt.Errorf("home layout: %w", err)
	}

	agents := agentstore.New(layout.AgentsDir())
	caps := capstore.New(layout.CapabilitiesDir())
	pkgs := packageio.New()

	registry := runtime.NewRegistry()
	for _, d := range []runtime.Driver{
		codex.New(caps),
		claudecode.New(caps),
		opencode.New(),
	} {
		if err := registry.Register(d); err != nil {
			return cli.Deps{}, fmt.Errorf("register %s: %w", d.Name(), err)
		}
	}

	log := runlog.New(layout.RunLogDir())
	writer := managedfile.New()
	proc := process.New()

	container := service.Container{
		Agents:       service.NewAgents(agents, registry),
		Run:          service.NewRunner(agents, registry, writer, proc, log),
		Packages:     service.NewPackages(agents, caps, pkgs),
		Capabilities: service.NewCapabilities(caps, registry),
		Diagnostics:  service.NewDiagnostics(agents, registry, log),
		System:       service.NewSystem(layout),
	}
	return cli.Deps{Services: container}, nil
}
