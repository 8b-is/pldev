//go:build darwin

package setup

import (
	"context"
	"os/exec"

	"github.com/peterlodri-sec/pldev/internal/sys"
)

func has(bin string) bool { _, err := exec.LookPath(bin); return err == nil }

// AvailableBackends reports which backends exist on this machine.
func AvailableBackends() map[Backend]bool {
	return map[Backend]bool{
		BrewFormula: has("brew"),
		BrewCask:    has("brew"),
		Mise:        has("mise"),
		Script:      true,
	}
}

type sysDetector struct{}

// NewDetector returns the live detector for this OS.
func NewDetector() Detector { return sysDetector{} }

func (sysDetector) Installed(ctx context.Context, s PkgSpec) (bool, error) {
	switch s.Backend {
	case BrewFormula:
		out, err := sys.Run(ctx, "brew", "list", "--versions", s.Name)
		if err != nil {
			return false, nil // non-zero exit = not installed
		}
		return brewListHas(out, s.Name), nil
	case BrewCask:
		out, err := sys.Run(ctx, "brew", "list", "--cask", "--versions", s.Name)
		if err != nil {
			return false, nil
		}
		return brewListHas(out, s.Name), nil
	case Mise:
		out, err := sys.Run(ctx, "mise", "ls", "--installed")
		if err != nil {
			return false, nil
		}
		return miseLsHas(out, s.Name), nil
	default: // Script and others
		return has(s.Name), nil
	}
}
