package verify

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peterlodri-sec/pldev/internal/report"
)

func rep() (*report.Reporter, *bytes.Buffer) {
	var b bytes.Buffer
	return report.New(&b, "verify"), &b
}

// stubSSH installs a fake ssh-keygen: find-principals returns principal, verify
// returns verifyErr. lastArgs captures the final invocation's argv.
func stubSSH(t *testing.T, principal string, findErr, verifyErr error, lastArgs *[]string) {
	t.Helper()
	oldRun, oldLP := run, lookPath
	t.Cleanup(func() { run, lookPath = oldRun, oldLP })
	lookPath = func(string) (string, error) { return "/usr/bin/ssh-keygen", nil }
	run = func(_ context.Context, _ []byte, _ string, args ...string) (string, error) {
		if lastArgs != nil {
			*lastArgs = args
		}
		for _, a := range args {
			if a == "find-principals" {
				return principal + "\n", findErr
			}
		}
		return "Good \"pldev\" signature", verifyErr
	}
}

func signedBin(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "pldev")
	os.WriteFile(bin, []byte("ELF..."), 0o755)
	os.WriteFile(bin+".sig", []byte("-----BEGIN SSH SIGNATURE-----\n..."), 0o644)
	return bin
}

func TestVerifySuccess(t *testing.T) {
	var args []string
	stubSSH(t, "peter.lodri@instructure.com", nil, nil, &args)
	r, b := rep()
	bin := signedBin(t)
	if err := Verify(context.Background(), r, bin); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{"-Y", "verify", "-n", "pldev", "-I", "peter.lodri@instructure.com", "-s", bin + ".sig"} {
		if !strings.Contains(joined, want) {
			t.Errorf("verify argv missing %q: %s", want, joined)
		}
	}
	if !strings.Contains(b.String(), "signer=peter.lodri@instructure.com") {
		t.Errorf("expected signer in log: %q", b.String())
	}
}

func TestVerifyUntrustedSigner(t *testing.T) {
	stubSSH(t, "", errors.New("no principals matched"), nil, nil)
	r, b := rep()
	err := Verify(context.Background(), r, signedBin(t))
	if err == nil || !strings.Contains(err.Error(), "untrusted signer") {
		t.Errorf("untrusted signer must be rejected, got %v", err)
	}
	if !strings.Contains(b.String(), "status=fail") {
		t.Errorf("expected fail log: %q", b.String())
	}
}

func TestVerifyTamperedFails(t *testing.T) {
	stubSSH(t, "peter.lodri@instructure.com", nil, errors.New("signature verification failed"), nil)
	r, _ := rep()
	if err := Verify(context.Background(), r, signedBin(t)); err == nil || !strings.Contains(err.Error(), "unverified") {
		t.Errorf("bad signature must fail, got %v", err)
	}
}

func TestVerifyMissingSSHKeygen(t *testing.T) {
	oldLP := lookPath
	t.Cleanup(func() { lookPath = oldLP })
	lookPath = func(string) (string, error) { return "", errors.New("not found") }
	r, _ := rep()
	if err := Verify(context.Background(), r, signedBin(t)); err == nil || !strings.Contains(err.Error(), "ssh-keygen not found") {
		t.Errorf("missing ssh-keygen should error, got %v", err)
	}
}

func TestVerifyMissingSignature(t *testing.T) {
	stubSSH(t, "x", nil, nil, nil)
	dir := t.TempDir()
	bin := filepath.Join(dir, "pldev")
	os.WriteFile(bin, []byte("x"), 0o755) // no .sig
	r, _ := rep()
	if err := Verify(context.Background(), r, bin); err == nil || !strings.Contains(err.Error(), "missing signature") {
		t.Errorf("missing sig should error, got %v", err)
	}
}

func TestEmbeddedAnchorPresent(t *testing.T) {
	if len(allowedSigners) == 0 || !strings.Contains(string(allowedSigners), "ssh-ed25519") {
		t.Error("allowed_signers trust anchor must be embedded")
	}
}
