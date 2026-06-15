package sys

import (
	"context"
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
