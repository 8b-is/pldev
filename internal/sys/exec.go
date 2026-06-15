package sys

import (
	"context"
	"os"
	"os/exec"
	"strings"
)

// Run executes name+args, returns trimmed-of-nothing stdout. Honors ctx deadline.
func Run(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var b strings.Builder
	cmd.Stdout = &b
	err := cmd.Run()
	return b.String(), err
}

// RunInteractive runs a command wired to the real terminal (stdin/stdout/stderr),
// so child programs like sudo can prompt and install output streams live.
// Returns only the exit error (no captured output).
func RunInteractive(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
