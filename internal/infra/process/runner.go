// Package process provides ProcessRunner: spawning runtime processes
// described by runtime.LaunchSpec and reporting their lifecycle.
package process

import (
	"context"
	"errors"
	"os"
	"os/exec"

	"github.com/xz1220/agent-vm/internal/runtime"
)

// Result reports a finished process.
type Result struct {
	ExitCode int
}

// Runner is the contract Application layer uses to launch runtimes.
type Runner interface {
	Run(ctx context.Context, spec runtime.LaunchSpec) (Result, error)
}

// OS is the default os/exec-backed runner.
type OS struct{}

func New() *OS { return &OS{} }

// Run spawns the configured binary and waits for it. The runtime's
// stdout/stderr always inherit from the parent. Stdin is inherited only
// when spec.Stdin == true. spec.Env replaces the child's environment
// completely (no merge with os.Environ); when empty, the parent env is
// inherited.
func (OS) Run(ctx context.Context, spec runtime.LaunchSpec) (Result, error) {
	if spec.Bin == "" {
		return Result{}, errors.New("process: empty Bin")
	}
	cmd := exec.CommandContext(ctx, spec.Bin, spec.Args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if spec.Stdin {
		cmd.Stdin = os.Stdin
	}
	if spec.Workdir != "" {
		cmd.Dir = spec.Workdir
	}
	if len(spec.Env) > 0 {
		env := make([]string, 0, len(spec.Env))
		for k, v := range spec.Env {
			env = append(env, k+"="+v)
		}
		cmd.Env = env
	}
	// else: cmd.Env remains nil → child inherits os.Environ.

	if err := cmd.Run(); err != nil {
		// Honor cancellation explicitly.
		if ctx.Err() != nil {
			return Result{ExitCode: -1}, ctx.Err()
		}
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return Result{ExitCode: ee.ExitCode()}, nil
		}
		return Result{ExitCode: -1}, err
	}
	return Result{ExitCode: cmd.ProcessState.ExitCode()}, nil
}
