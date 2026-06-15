package setup

import (
	"context"
	"strings"
)

// Detector reports whether the package described by a spec is already installed.
type Detector interface {
	Installed(ctx context.Context, spec PkgSpec) (bool, error)
}

// brewListHas reports whether `brew list --versions` output contains name as the
// first field of any line.
func brewListHas(out, name string) bool {
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) > 0 && f[0] == name {
			return true
		}
	}
	return false
}

// miseLsHas reports whether `mise ls` output lists the tool (first field).
func miseLsHas(out, name string) bool {
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) > 0 && f[0] == name {
			return true
		}
	}
	return false
}
