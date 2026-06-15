package pqkey

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peterlodri-sec/pldev/internal/verify"
)

func TestWriteKeyPairAndSign(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "k.key")
	pubB64, err := WriteKeyPair(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(keyPath)
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("private key perms = %o, want 600", fi.Mode().Perm())
	}
	if _, err := os.Stat(keyPath + ".pub"); err != nil {
		t.Fatalf("public key file missing: %v", err)
	}

	bin := filepath.Join(dir, "artifact")
	data := []byte("artifact bytes")
	os.WriteFile(bin, data, 0o644)
	if err := SignFile(keyPath, bin); err != nil {
		t.Fatal(err)
	}
	sigB64, _ := os.ReadFile(bin + ".sig.mldsa")
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sigB64)))
	if err != nil {
		t.Fatal(err)
	}
	pub, _ := base64.StdEncoding.DecodeString(pubB64)
	ok, err := verify.VerifyPQ(pub, data, sig)
	if err != nil || !ok {
		t.Fatalf("pq-sign output must verify: ok=%v err=%v", ok, err)
	}
}

func TestWriteKeyPairNoClobber(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "k.key")
	if _, err := WriteKeyPair(keyPath); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteKeyPair(keyPath); err == nil {
		t.Fatal("second WriteKeyPair must refuse to clobber an existing key")
	}
}
