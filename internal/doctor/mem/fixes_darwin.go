//go:build darwin

package mem

import (
	"os"
	"path/filepath"

	"github.com/peterlodri-sec/pldev/internal/doctor/fix"
	"github.com/peterlodri-sec/pldev/internal/doctor/probe"
)

func platformFixes(r probe.MemReport) []fix.Fix {
	var f []fix.Fix
	home := os.Getenv("HOME")
	f = append(f, fix.SpotlightExclude{Dirs: []string{
		filepath.Join(home, "workspace"),
		filepath.Join(home, ".orbstack"),
	}})
	for _, z := range r.Zombies {
		if z.Dead {
			f = append(f, fix.RemoveLaunchAgent{Label: z.Label, Path: z.Path})
		}
	}
	return f
}
