//go:build linux

package setup

import (
	"context"
	"os/exec"

	"github.com/peterlodri-sec/pldev/internal/sys"
)

func has(bin string) bool { _, err := exec.LookPath(bin); return err == nil }

func AvailableBackends() map[Backend]bool {
	return map[Backend]bool{
		Apt:    has("apt-get"),
		Dnf:    has("dnf"),
		Pacman: has("pacman"),
		Mise:   has("mise"),
		Script: true,
	}
}

type sysDetector struct{}

func NewDetector() Detector { return sysDetector{} }

func (sysDetector) Installed(ctx context.Context, s PkgSpec) (bool, error) {
	switch s.Backend {
	case Apt:
		_, err := sys.Run(ctx, "dpkg", "-s", s.Name)
		return err == nil, nil
	case Dnf:
		_, err := sys.Run(ctx, "rpm", "-q", s.Name)
		return err == nil, nil
	case Pacman:
		_, err := sys.Run(ctx, "pacman", "-Qi", s.Name)
		return err == nil, nil
	case Mise:
		out, err := sys.Run(ctx, "mise", "ls", "--installed")
		if err != nil {
			return false, nil
		}
		return miseLsHas(out, s.Name), nil
	default:
		return has(s.Name), nil
	}
}
