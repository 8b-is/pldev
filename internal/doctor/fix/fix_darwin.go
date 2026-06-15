//go:build darwin

package fix

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/peterlodri-sec/pldev/internal/sys"
)

// SpotlightExclude writes .metadata_never_index markers (Safe, reversible).
type SpotlightExclude struct{ Dirs []string }

func (SpotlightExclude) Title() string { return "Exclude heavy dirs from Spotlight" }
func (SpotlightExclude) Safe() bool     { return true }
func (s SpotlightExclude) Describe() string {
	return fmt.Sprintf("touch .metadata_never_index in %d dir(s)", len(s.Dirs))
}
func (s SpotlightExclude) Apply(_ context.Context) (string, error) {
	var done int
	for _, d := range s.Dirs {
		marker := filepath.Join(d, ".metadata_never_index")
		f, err := os.Create(marker)
		if err != nil {
			continue // e.g. read-only / SIP — skip, report below
		}
		f.Close()
		done++
	}
	return fmt.Sprintf("marked %d/%d dirs", done, len(s.Dirs)), nil
}

// RemoveLaunchAgent boots out + deletes a zombie LaunchAgent plist (Destructive).
type RemoveLaunchAgent struct {
	Label string
	Path  string
}

func (RemoveLaunchAgent) Title() string { return "Remove zombie LaunchAgent" }
func (RemoveLaunchAgent) Safe() bool     { return false }
func (r RemoveLaunchAgent) Describe() string {
	return "launchctl bootout + rm " + r.Path
}
func (r RemoveLaunchAgent) Apply(ctx context.Context) (string, error) {
	uid := os.Getuid()
	_, _ = sys.Run(ctx, "launchctl", "bootout", fmt.Sprintf("gui/%d/%s", uid, r.Label))
	if err := os.Remove(r.Path); err != nil {
		return "", err
	}
	return "booted out + removed " + r.Label, nil
}
