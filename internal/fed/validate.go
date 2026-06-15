package fed

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/peterlodri-sec/pldev/internal/report"
	"github.com/peterlodri-sec/pldev/internal/sys"
)

// Indirection points for tests.
var (
	checkCmd   = func(ctx context.Context, path string) (string, error) { return sys.Run(ctx, "vakedc", "check", path) }
	lookVakedc = func() error { _, err := exec.LookPath("vakedc"); return err }
)

// Validate is the offline boundary check: a vaked agent message file is run
// through `vakedc check`. Valid → ok; invalid → the vakedc error surfaced;
// nothing malformed should ever cross the boundary into the swarm.
func Validate(ctx context.Context, rep *report.Reporter, path string) error {
	rep.Do("validate", path)
	if err := lookVakedc(); err != nil {
		e := fmt.Errorf("vakedc not found — install vaked-base (provides the agent_msg schema + checker)")
		rep.Fail("validate", path, e)
		return e
	}
	if _, err := os.Stat(path); err != nil {
		e := fmt.Errorf("no such message file: %s", path)
		rep.Fail("validate", path, e)
		return e
	}
	out, err := checkCmd(ctx, path)
	if err != nil {
		e := fmt.Errorf("invalid: %v", err)
		rep.Fail("validate", path, e, report.KV{K: "out", V: strings.TrimSpace(out)})
		return e
	}
	rep.OK("validate", path)
	return nil
}
