package process

import (
	"context"
	"errors"
	"os/exec"
	"testing"
	"time"

	"github.com/xz1220/agent-vm/internal/runtime"
)

func mustLookPath(t *testing.T, bin string) string {
	t.Helper()
	p, err := exec.LookPath(bin)
	if err != nil {
		t.Skipf("skipping: %s not on PATH: %v", bin, err)
	}
	return p
}

func TestRun_Success(t *testing.T) {
	bin := mustLookPath(t, "true")
	r := New()
	res, err := r.Run(context.Background(), runtime.LaunchSpec{Bin: bin})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("exit code: %d", res.ExitCode)
	}
}

func TestRun_NonZeroExit(t *testing.T) {
	bin := mustLookPath(t, "false")
	r := New()
	res, err := r.Run(context.Background(), runtime.LaunchSpec{Bin: bin})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.ExitCode == 0 {
		t.Fatal("expected non-zero exit code")
	}
}

func TestRun_EmptyBin(t *testing.T) {
	r := New()
	if _, err := r.Run(context.Background(), runtime.LaunchSpec{}); err == nil {
		t.Fatal("expected error for empty Bin")
	}
}

func TestRun_Cancel(t *testing.T) {
	bin := mustLookPath(t, "sleep")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	r := New()
	_, err := r.Run(ctx, runtime.LaunchSpec{Bin: bin, Args: []string{"5"}})
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		// Some platforms surface the kill as a generic exit error;
		// accept any error here as long as we did not block forever.
		t.Logf("non-context error: %v", err)
	}
}

func TestRun_EnvOverridesParent(t *testing.T) {
	bin := mustLookPath(t, "sh")
	t.Setenv("PARENT_ONLY_VAR", "parent-value")
	r := New()
	// Explicit empty-but-non-nil-via-presence Env: pass a single var
	// only and verify the child does NOT see PARENT_ONLY_VAR.
	res, err := r.Run(context.Background(), runtime.LaunchSpec{
		Bin:  bin,
		Args: []string{"-c", "test -z \"$PARENT_ONLY_VAR\""},
		Env:  map[string]string{"FOO": "bar"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("expected child env to NOT include PARENT_ONLY_VAR, exit=%d", res.ExitCode)
	}
}

func TestRun_InheritsParentEnvWhenEnvEmpty(t *testing.T) {
	bin := mustLookPath(t, "sh")
	t.Setenv("PARENT_ONLY_VAR", "parent-value")
	r := New()
	res, err := r.Run(context.Background(), runtime.LaunchSpec{
		Bin:  bin,
		Args: []string{"-c", "test \"$PARENT_ONLY_VAR\" = parent-value"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("expected parent env to be inherited, exit=%d", res.ExitCode)
	}
}
