// Package pqkey is file IO for pldev's ML-DSA-65 signing keys: generate a
// keypair to disk and sign a file. Private keys are written 0600 and never
// logged or returned in printable form.
package pqkey

import (
	"encoding/base64"
	"fmt"
	"os"
	"strings"

	"github.com/peterlodri-sec/pldev/internal/verify"
)

// WriteKeyPair generates an ML-DSA-65 keypair, writes the base64 private key to
// keyPath (0600) and the base64 public key to keyPath+".pub" (0644), and returns
// the base64 public key. It refuses to overwrite an existing private key file.
func WriteKeyPair(keyPath string) (pubB64 string, err error) {
	if _, err := os.Stat(keyPath); err == nil {
		return "", fmt.Errorf("refusing to overwrite existing key %s", keyPath)
	}
	pub, priv, err := verify.GenPQKeyPair()
	if err != nil {
		return "", err
	}
	pubB64 = base64.StdEncoding.EncodeToString(pub)
	if err := os.WriteFile(keyPath, []byte(base64.StdEncoding.EncodeToString(priv)+"\n"), 0o600); err != nil {
		return "", err
	}
	if err := os.WriteFile(keyPath+".pub", []byte(pubB64+"\n"), 0o644); err != nil {
		return "", err
	}
	return pubB64, nil
}

// LoadPriv reads a base64 private key file into raw bytes.
func LoadPriv(keyPath string) ([]byte, error) {
	b, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(strings.TrimSpace(string(b)))
}

// SignFile signs binPath with the key at keyPath, writing binPath+".sig.mldsa"
// (base64) next to it.
func SignFile(keyPath, binPath string) error {
	priv, err := LoadPriv(keyPath)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(binPath)
	if err != nil {
		return err
	}
	sig, err := verify.SignPQ(priv, data)
	if err != nil {
		return err
	}
	return os.WriteFile(binPath+".sig.mldsa", []byte(base64.StdEncoding.EncodeToString(sig)+"\n"), 0o644)
}
