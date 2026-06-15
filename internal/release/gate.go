package release

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// genChallenge reads 2 random bytes and renders a 4-char uppercase hex token.
func genChallenge(r io.Reader) (string, error) {
	b := make([]byte, 2)
	if _, err := io.ReadFull(r, b); err != nil {
		return "", err
	}
	return strings.ToUpper(hex.EncodeToString(b)), nil
}

// randReader is the entropy source; overridable in tests.
var randReader io.Reader = rand.Reader

var errAborted = errors.New("publish aborted by operator")

// confirmPublish runs the triple-ack gate against the real terminal.
// dest is the network destination (host+path) shown before the operator acks.
func confirmPublish(tag, dest string, arts []string) error {
	return confirmPublishWith(
		func() (string, error) { return genChallenge(randReader) },
		os.Stdin, os.Stdout, tag, dest, arts)
}

// confirmPublishWith is the testable core. challengeFn supplies the token.
func confirmPublishWith(challengeFn func() (string, error), in io.Reader, out io.Writer, tag, dest string, arts []string) error {
	sc := bufio.NewScanner(in)
	ask := func(prompt string) (string, error) {
		fmt.Fprint(out, prompt)
		if !sc.Scan() {
			return "", errAborted
		}
		return strings.TrimSpace(sc.Text()), nil
	}

	fmt.Fprintf(out, "Release %s\n", tag)
	fmt.Fprintf(out, "  TO: %s  (HTTPS, TLS1.2+)\n", dest)
	fmt.Fprintf(out, "  uploading %d assets (%d builds, each + .sig + .sig.mldsa):\n", len(arts)*3, len(arts))
	for _, a := range arts {
		fmt.Fprintf(out, "  - %s\n", a)
		fmt.Fprintf(out, "      + %s.sig         (ed25519)\n", a)
		fmt.Fprintf(out, "      + %s.sig.mldsa   (ML-DSA-65)\n", a)
	}
	if a, err := ask("[1/3] Publish this release? (y/N): "); err != nil || a != "y" {
		return errAborted
	}
	if a, err := ask("[2/3] This uploads to GitHub and is IRREVERSIBLE. Continue? (y/N): "); err != nil || a != "y" {
		return errAborted
	}
	token, err := challengeFn()
	if err != nil {
		return err
	}
	a, err := ask(fmt.Sprintf("[3/3] Type to confirm — %s: ", token))
	if err != nil {
		return err
	}
	if !strings.EqualFold(a, token) {
		return fmt.Errorf("%w: challenge mismatch", errAborted)
	}
	return nil
}
