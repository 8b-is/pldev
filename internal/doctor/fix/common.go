package fix

import (
	"context"
	"fmt"
	"os"

	"github.com/peterlodri-sec/pldev/internal/sys"
)

// PurgeNodeModules deletes the given node_modules dirs (Destructive).
type PurgeNodeModules struct{ Dirs []string }

func (p PurgeNodeModules) Title() string { return "Purge selected node_modules" }
func (p PurgeNodeModules) Safe() bool     { return false }
func (p PurgeNodeModules) Describe() string {
	return fmt.Sprintf("rm -rf %d node_modules dir(s) (recoverable via reinstall)", len(p.Dirs))
}
func (p PurgeNodeModules) Apply(_ context.Context) (string, error) {
	for _, d := range p.Dirs {
		if err := os.RemoveAll(d); err != nil {
			return "", err
		}
	}
	return fmt.Sprintf("removed %d dirs", len(p.Dirs)), nil
}

// RebootHint is advice only (Safe, no action).
type RebootHint struct{ Reason string }

func (RebootHint) Title() string     { return "Reboot recommended" }
func (RebootHint) Safe() bool         { return true }
func (h RebootHint) Describe() string { return h.Reason }
func (h RebootHint) Apply(context.Context) (string, error) {
	return "advice only — reboot when convenient", nil
}

func _unused(ctx context.Context) { _, _ = sys.Run(ctx, "true") } // keep sys imported where needed
