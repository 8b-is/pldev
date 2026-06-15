package verify

import (
	"bytes"
	"context"
	"encoding/base64"
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

func TestVerifyDualPolicy(t *testing.T) {
	origRun, origLook := run, lookPath
	defer func() { run, lookPath = origRun, origLook }()
	lookPath = func(string) (string, error) { return "/usr/bin/ssh-keygen", nil }
	run = func(ctx context.Context, stdin []byte, name string, args ...string) (string, error) {
		return "brett-shaw@pldev\n", nil
	}

	dir := t.TempDir()
	bin := filepath.Join(dir, "pldev-test")
	data := []byte("the bolygorozsa carries the order")
	os.WriteFile(bin, data, 0o644)
	os.WriteFile(bin+".sig", []byte("dummy-ed25519-sig"), 0o644)

	pub, priv, _ := GenPQKeyPair()
	sig, _ := SignPQ(priv, data)
	os.WriteFile(bin+".sig.mldsa", []byte(base64.StdEncoding.EncodeToString(sig)), 0o644)

	home := t.TempDir()
	t.Setenv("HOME", home)
	os.MkdirAll(filepath.Join(home, ".config", "pldev"), 0o755)
	os.WriteFile(filepath.Join(home, ".config", "pldev", "pq_signers"),
		[]byte("brett-shaw@pldev mldsa65 "+base64.StdEncoding.EncodeToString(pub)+"\n"), 0o644)

	r, _ := rep()
	if err := Verify(context.Background(), r, bin); err != nil {
		t.Fatalf("dual-valid binary must verify: %v", err)
	}

	os.WriteFile(bin+".sig.mldsa", []byte(base64.StdEncoding.EncodeToString([]byte("garbage"))), 0o644)
	r, _ = rep()
	if err := Verify(context.Background(), r, bin); err == nil {
		t.Fatal("present-but-invalid PQ signature must reject")
	}

	os.Remove(bin + ".sig.mldsa")
	r, _ = rep()
	if err := Verify(context.Background(), r, bin); err != nil {
		t.Fatalf("ed25519-only binary must still verify: %v", err)
	}
}

func TestEmbeddedAnchorPresent(t *testing.T) {
	if len(allowedSigners) == 0 || !strings.Contains(string(allowedSigners), "ssh-ed25519") {
		t.Error("allowed_signers trust anchor must be embedded")
	}
}

func TestAnchorRuntimeOverlay(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	// no runtime file → embedded default
	if string(AnchorBytes()) != string(allowedSigners) {
		t.Error("absent runtime file should yield the embedded anchor")
	}
	// runtime file present → it overrides
	p := DefaultAnchorPath()
	os.MkdirAll(filepath.Dir(p), 0o755)
	os.WriteFile(p, []byte("custom@x ssh-ed25519 AAAAruntime\n"), 0o644)
	if !strings.Contains(string(AnchorBytes()), "custom@x") {
		t.Error("runtime allowed_signers should override the embedded default")
	}
}
