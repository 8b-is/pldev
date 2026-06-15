package verify

import (
	"bytes"
	"encoding/base64"
	"testing"
)

func TestPQRoundTrip(t *testing.T) {
	pub, priv, err := GenPQKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	msg := []byte("brett shaw, colonel of the praetorian guard")
	sig, err := SignPQ(priv, msg)
	if err != nil {
		t.Fatal(err)
	}
	ok, err := VerifyPQ(pub, msg, sig)
	if err != nil || !ok {
		t.Fatalf("valid sig must verify: ok=%v err=%v", ok, err)
	}
}

func TestPQTamperFails(t *testing.T) {
	pub, priv, _ := GenPQKeyPair()
	sig, _ := SignPQ(priv, []byte("original"))
	ok, _ := VerifyPQ(pub, []byte("tampered"), sig)
	if ok {
		t.Fatal("tampered message must not verify")
	}
}

func TestPQWrongKeyFails(t *testing.T) {
	_, priv, _ := GenPQKeyPair()
	otherPub, _, _ := GenPQKeyPair()
	sig, _ := SignPQ(priv, []byte("m"))
	ok, _ := VerifyPQ(otherPub, []byte("m"), sig)
	if ok {
		t.Fatal("wrong key must not verify")
	}
}

func TestPQCorruptKeyErrors(t *testing.T) {
	if _, err := SignPQ([]byte("not a key"), []byte("m")); err == nil {
		t.Fatal("corrupt private key must error")
	}
	if _, err := VerifyPQ([]byte("not a key"), []byte("m"), []byte("sig")); err == nil {
		t.Fatal("corrupt public key must error")
	}
}

func TestPQKeysAreDistinct(t *testing.T) {
	_, p1, _ := GenPQKeyPair()
	_, p2, _ := GenPQKeyPair()
	if bytes.Equal(p1, p2) {
		t.Fatal("two generated keys must differ")
	}
}

func TestVerifyPQAnchorMatch(t *testing.T) {
	pub, priv, _ := GenPQKeyPair()
	anchor := []byte("# comment\n\nbrett-shaw@pldev mldsa65 " + base64.StdEncoding.EncodeToString(pub) + "\n")
	msg := []byte("release binary bytes")
	sig, _ := SignPQ(priv, msg)

	id, ok := verifyPQ(anchor, msg, sig)
	if !ok || id != "brett-shaw@pldev" {
		t.Fatalf("trusted key should match: id=%q ok=%v", id, ok)
	}
	otherPub, _, _ := GenPQKeyPair()
	anchor2 := []byte("ci@other mldsa65 " + base64.StdEncoding.EncodeToString(otherPub) + "\n")
	if _, ok := verifyPQ(anchor2, msg, sig); ok {
		t.Fatal("signature from untrusted key must not match")
	}
	if _, ok := verifyPQ(anchor, []byte("tampered"), sig); ok {
		t.Fatal("tampered message must not match")
	}
}
