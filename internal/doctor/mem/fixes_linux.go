//go:build linux

package mem

import (
	"github.com/peterlodri-sec/pldev/internal/doctor/fix"
	"github.com/peterlodri-sec/pldev/internal/doctor/probe"
)

func platformFixes(r probe.MemReport) []fix.Fix {
	var f []fix.Fix
	for _, z := range r.Zombies {
		if z.Dead {
			f = append(f, fix.DisableUnit{Unit: z.Label})
		}
	}
	return f
}
