// Package verify provides pldev's binary-provenance check: it verifies a
// candidate binary against its detached ssh-keygen signature using the embedded
// allowed_signers trust anchor. SSH signatures are key-based with no
// transparency log (private repos stay private) and reuse the user's existing
// ssh keys. It verifies a TARGET, never itself — a tampered binary cannot be
// trusted to self-verify. Trust is the committed allowed_signers file.
package verify

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/peterlodri-sec/pldev/internal/report"
)

//go:embed allowed_signers
var allowedSigners []byte

// namespace scopes signatures to pldev (ssh-keygen -Y -n).
const namespace = "pldev"

// Indirection points for tests.
var (
	run      = defaultRun
	lookPath = exec.LookPath
)

func defaultRun(ctx context.Context, stdin []byte, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func writeTempAnchor() (string, error) {
	f, err := os.CreateTemp("", "pldev-allowed-signers-*")
	if err != nil {
		return "", err
	}
	if _, err := f.Write(allowedSigners); err != nil {
		f.Close()
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), f.Close()
}

// Verify checks the candidate binary at path against path+".sig" using the
// embedded allowed_signers. It first resolves the signer (which must be
// trusted), then verifies the signature over the file contents.
func Verify(ctx context.Context, rep *report.Reporter, path string) error {
	rep.Do("verify", path)

	if _, err := lookPath("ssh-keygen"); err != nil {
		e := fmt.Errorf("ssh-keygen not found")
		rep.Fail("verify", path, e)
		return e
	}
	sig := path + ".sig"
	if _, err := os.Stat(sig); err != nil {
		e := fmt.Errorf("missing signature %s", sig)
		rep.Fail("verify", path, e)
		return e
	}
	anchor, err := writeTempAnchor()
	if err != nil {
		rep.Fail("verify", path, err)
		return err
	}
	defer os.Remove(anchor)

	// Who signed it? Errors if the signer isn't in allowed_signers (untrusted).
	out, err := run(ctx, nil, "ssh-keygen", "-Y", "find-principals", "-s", sig, "-f", anchor)
	if err != nil {
		e := fmt.Errorf("untrusted signer (not in allowed_signers)")
		rep.Fail("verify", path, e, report.KV{K: "out", V: strings.TrimSpace(out)})
		return e
	}
	identity := strings.TrimSpace(strings.SplitN(out, "\n", 2)[0])

	blob, err := os.ReadFile(path)
	if err != nil {
		rep.Fail("verify", path, err)
		return err
	}
	vout, err := run(ctx, blob, "ssh-keygen", "-Y", "verify", "-n", namespace, "-f", anchor, "-I", identity, "-s", sig)
	if err != nil {
		e := fmt.Errorf("unverified: %v", err)
		rep.Fail("verify", path, e, report.KV{K: "out", V: strings.TrimSpace(vout)})
		return e
	}
	rep.OK("verify", path, report.KV{K: "signer", V: identity})
	return nil
}
