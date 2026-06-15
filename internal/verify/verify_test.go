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

// stubCosign makes lookPath find cosign and run return the given result.
func stubCosign(t *testing.T, gotArgs *[]string, runErr error, out string) {
	t.Helper()
	oldRun, oldLP := run, lookPath
	t.Cleanup(func() { run, lookPath = oldRun, oldLP })
	lookPath = func(string) (string, error) { return "/usr/bin/cosign", nil }
	run = func(_ context.Context, _ string, args ...string) (string, error) {
		if gotArgs != nil {
			*gotArgs = args
		}
		return out, runErr
	}
}

func signedBin(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "pldev")
	os.WriteFile(bin, []byte("ELF..."), 0o755)
	os.WriteFile(bin+".sig", []byte("sig"), 0o644)
	return bin
}

func TestVerifySuccessArgs(t *testing.T) {
	var args []string
	stubCosign(t, &args, nil, "Verified OK")
	r, _ := rep()
	bin := signedBin(t)
	if err := Verify(context.Background(), r, bin); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{"verify-blob", "--key", "--signature", bin + ".sig", "--insecure-ignore-tlog", bin} {
		if !strings.Contains(joined, want) {
			t.Errorf("argv missing %q: %s", want, joined)
		}
	}
}

func TestVerifyUnverifiedFails(t *testing.T) {
	stubCosign(t, nil, errors.New("exit 1"), "signature mismatch")
	r, b := rep()
	if err := Verify(context.Background(), r, signedBin(t)); err == nil {
		t.Error("tampered/unverified binary must fail")
	}
	if !strings.Contains(b.String(), "status=fail") {
		t.Errorf("expected fail log: %q", b.String())
	}
}

func TestVerifyMissingCosign(t *testing.T) {
	oldLP := lookPath
	t.Cleanup(func() { lookPath = oldLP })
	lookPath = func(string) (string, error) { return "", errors.New("not found") }
	r, _ := rep()
	err := Verify(context.Background(), r, signedBin(t))
	if err == nil || !strings.Contains(err.Error(), "cosign not found") {
		t.Errorf("expected actionable cosign-missing error, got %v", err)
	}
}

func TestVerifyMissingSignature(t *testing.T) {
	stubCosign(t, nil, nil, "")
	dir := t.TempDir()
	bin := filepath.Join(dir, "pldev")
	os.WriteFile(bin, []byte("x"), 0o755) // no .sig
	r, _ := rep()
	err := Verify(context.Background(), r, bin)
	if err == nil || !strings.Contains(err.Error(), "missing signature") {
		t.Errorf("expected missing-signature error, got %v", err)
	}
}

func TestEmbeddedPubPresent(t *testing.T) {
	if len(pubKey) == 0 {
		t.Error("cosign.pub must be embedded")
	}
}
