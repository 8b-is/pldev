// Package verify provides pldev's binary-provenance check: it verifies a
// candidate binary against its detached cosign signature using the embedded
// trust anchor (cosign.pub). It verifies a TARGET, never itself — a tampered
// binary cannot be trusted to self-verify. Trust is the committed cosign.pub.
package verify

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/peterlodri-sec/pldev/internal/report"
)

//go:embed cosign.pub
var pubKey []byte

// Indirection points for tests.
var (
	run      = defaultRun
	lookPath = exec.LookPath
)

func defaultRun(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// writeTempPub materializes the embedded trust anchor for cosign (which reads a
// key file, not stdin). Caller removes it.
func writeTempPub() (string, error) {
	f, err := os.CreateTemp("", "pldev-cosign-*.pub")
	if err != nil {
		return "", err
	}
	if _, err := f.Write(pubKey); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), f.Close()
}

// Verify checks the candidate binary at path against path+".sig" using the
// embedded cosign.pub. Key-based, no transparency log (nothing public).
func Verify(ctx context.Context, rep *report.Reporter, path string) error {
	rep.Do("verify", path)

	if _, err := lookPath("cosign"); err != nil {
		e := fmt.Errorf("cosign not found — install it (pldev setup adds it)")
		rep.Fail("verify", path, e)
		return e
	}
	sig := path + ".sig"
	if _, err := os.Stat(sig); err != nil {
		e := fmt.Errorf("missing signature %s", sig)
		rep.Fail("verify", path, e)
		return e
	}
	pub, err := writeTempPub()
	if err != nil {
		rep.Fail("verify", path, err)
		return err
	}
	defer os.Remove(pub)

	out, err := run(ctx, "cosign", verifyArgs(pub, sig, path)...)
	if err != nil {
		e := fmt.Errorf("unverified: %v", err)
		rep.Fail("verify", path, e, report.KV{K: "out", V: strings.TrimSpace(out)})
		return e
	}
	rep.OK("verify", path, report.KV{K: "by", V: "cosign.pub"})
	return nil
}

// verifyArgs is the cosign argv: key-based, transparency log ignored (we never
// upload to one).
func verifyArgs(pub, sig, path string) []string {
	return []string{"verify-blob", "--key", pub, "--signature", sig, "--insecure-ignore-tlog", path}
}
