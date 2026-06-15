package release

import (
	"context"
	"os"
	"os/exec"
)

// Runner abstracts process execution so build/sign are mockable.
type Runner interface {
	// Capture runs name+args with optional extra env, returning combined output.
	Capture(ctx context.Context, env []string, name string, args ...string) ([]byte, error)
	// Interactive runs name+args wired to the real terminal (for passphrase prompts).
	Interactive(ctx context.Context, name string, args ...string) error
}

type execRunner struct{}

func (execRunner) Capture(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	return cmd.CombinedOutput()
}

func (execRunner) Interactive(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}
