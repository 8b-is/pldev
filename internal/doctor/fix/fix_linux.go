//go:build linux

package fix

import (
	"context"
	"fmt"

	"github.com/peterlodri-sec/pldev/internal/sys"
)

// DisableUnit stops+disables a failed systemd --user unit (Destructive-ish).
type DisableUnit struct{ Unit string }

func (DisableUnit) Title() string { return "Disable failed systemd unit" }
func (DisableUnit) Safe() bool     { return false }
func (d DisableUnit) Describe() string {
	return "systemctl --user disable --now " + d.Unit
}
func (d DisableUnit) Apply(ctx context.Context) (string, error) {
	if _, err := sys.Run(ctx, "systemctl", "--user", "disable", "--now", d.Unit); err != nil {
		return "", err
	}
	return fmt.Sprintf("disabled %s", d.Unit), nil
}
