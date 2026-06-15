# Quantum-Resistant Release Signing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a hybrid post-quantum signature (ML-DSA-65, FIPS 204) alongside the existing ed25519 ssh-keygen signature on every release, with `pldev verify` enforcing both.

**Architecture:** A new `internal/verify/pq.go` wraps `cloudflare/circl`'s ML-DSA-65 `sign.Scheme`. A `pq_signers` embedded anchor (mirroring `allowed_signers`) holds trusted PQ public keys. `Verify` is extended: ed25519 stays mandatory, the PQ signature is required when a `.sig.mldsa` is present. Two thin commands (`pq-keygen`, `pq-sign`) and `release.yml` dual-sign.

**Tech Stack:** Go 1.26, `github.com/cloudflare/circl/sign/mldsa/mldsa65`, existing `internal/verify` + `internal/report`, ssh-keygen (unchanged classical path).

**Branch:** `feat/pq-signing` (already created off main).

---

## File Structure

- `internal/verify/pq.go` (new) — `GenPQKeyPair`, `SignPQ`, `VerifyPQ`, `verifyPQ` (anchor match).
- `internal/verify/pq_test.go` (new) — PQ crypto + anchor-match tests.
- `internal/verify/pq_signers` (new, go:embed) — PQ trust anchor (starts with a header comment; real keys added in the live runbook).
- `internal/verify/anchors.go` (new) — `DefaultPQAnchorPath`, `PQAnchorBytes`, embed of `pq_signers`. (Keeps `verify.go` focused.)
- `internal/verify/verify.go` (modify) — extend `Verify` with the PQ step.
- `internal/verify/verify_test.go` (modify/create) — dual-sig policy tests.
- `internal/pqkey/pqkey.go` (new) — file IO for PQ keys + sign-a-file (used by commands; keeps `main.go` thin). Functions `WriteKeyPair`, `LoadPriv`, `SignFile`.
- `main.go` (modify) — dispatch `pq-keygen` / `pq-sign`.
- `internal/fleet/sync.go` (modify) — `SyncSigners` also pushes `pq_signers`.
- `.github/workflows/release.yml` (modify) — ML-DSA dual-sign step + `MLDSA_SIGNING_KEY`.
- `Taskfile.yml` (modify) — `pq-keygen` + optional pq-sign in `sign`.

---

## Task 1: ML-DSA-65 crypto wrapper

**Files:**
- Create: `internal/verify/pq.go`, `internal/verify/pq_test.go`
- Modify: `go.mod` (adds circl)

- [ ] **Step 1: Add the dependency**

Run: `cd /Users/peter.lodri/workspace/pldev && go get github.com/cloudflare/circl@latest`
Expected: `go.mod`/`go.sum` updated with `github.com/cloudflare/circl`.

- [ ] **Step 2: Write the failing tests**

Create `internal/verify/pq_test.go`:
```go
package verify

import (
	"bytes"
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
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd /Users/peter.lodri/workspace/pldev && go test ./internal/verify/ -run PQ -v`
Expected: FAIL — undefined: GenPQKeyPair/SignPQ/VerifyPQ.

- [ ] **Step 4: Implement**

Create `internal/verify/pq.go`:
```go
package verify

import (
	"github.com/cloudflare/circl/sign"
	"github.com/cloudflare/circl/sign/mldsa/mldsa65"
)

// pqScheme is ML-DSA-65 (FIPS 204), NIST security level 3.
var pqScheme sign.Scheme = mldsa65.Scheme()

// GenPQKeyPair returns a fresh ML-DSA-65 keypair as raw scheme bytes.
func GenPQKeyPair() (pub, priv []byte, err error) {
	pk, sk, err := pqScheme.GenerateKey()
	if err != nil {
		return nil, nil, err
	}
	pub, err = pk.MarshalBinary()
	if err != nil {
		return nil, nil, err
	}
	priv, err = sk.MarshalBinary()
	if err != nil {
		return nil, nil, err
	}
	return pub, priv, nil
}

// SignPQ signs data with a raw ML-DSA-65 private key.
func SignPQ(privRaw, data []byte) ([]byte, error) {
	sk, err := pqScheme.UnmarshalBinaryPrivateKey(privRaw)
	if err != nil {
		return nil, err
	}
	return pqScheme.Sign(sk, data, nil), nil
}

// VerifyPQ reports whether sig is a valid ML-DSA-65 signature of data under pubRaw.
func VerifyPQ(pubRaw, data, sig []byte) (bool, error) {
	pk, err := pqScheme.UnmarshalBinaryPublicKey(pubRaw)
	if err != nil {
		return false, err
	}
	return pqScheme.Verify(pk, data, sig, nil), nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /Users/peter.lodri/workspace/pldev && go test ./internal/verify/ -run PQ -v && go build ./...`
Expected: PASS, builds.

- [ ] **Step 6: Commit**

```bash
cd /Users/peter.lodri/workspace/pldev
git add go.mod go.sum internal/verify/pq.go internal/verify/pq_test.go
git commit -m "feat(verify): ML-DSA-65 post-quantum sign/verify (cloudflare/circl)"
```

---

## Task 2: PQ trust anchor (pq_signers) + match

**Files:**
- Create: `internal/verify/pq_signers`, `internal/verify/anchors.go`
- Modify: `internal/verify/pq_test.go`

- [ ] **Step 1: Create the embed file**

Create `internal/verify/pq_signers` with exactly this content (no trusted keys yet — added in the live runbook):
```
# pldev PQ trust anchor — ML-DSA-65 public keys. Format: <principal> mldsa65 <base64-pubkey>
# Real signing identities (e.g. brett-shaw@pldev) are appended during release setup.
```

- [ ] **Step 2: Write the failing test**

Add to `internal/verify/pq_test.go`:
```go
import "encoding/base64" // add to the import block

func TestVerifyPQAnchorMatch(t *testing.T) {
	pub, priv, _ := GenPQKeyPair()
	anchor := []byte("# comment\n\nbrett-shaw@pldev mldsa65 " + base64.StdEncoding.EncodeToString(pub) + "\n")
	msg := []byte("release binary bytes")
	sig, _ := SignPQ(priv, msg)

	id, ok := verifyPQ(anchor, msg, sig)
	if !ok || id != "brett-shaw@pldev" {
		t.Fatalf("trusted key should match: id=%q ok=%v", id, ok)
	}
	// untrusted: different key in anchor
	otherPub, _, _ := GenPQKeyPair()
	anchor2 := []byte("ci@other mldsa65 " + base64.StdEncoding.EncodeToString(otherPub) + "\n")
	if _, ok := verifyPQ(anchor2, msg, sig); ok {
		t.Fatal("signature from untrusted key must not match")
	}
	// tampered message
	if _, ok := verifyPQ(anchor, []byte("tampered"), sig); ok {
		t.Fatal("tampered message must not match")
	}
}
```

- [ ] **Step 3: Run to verify fail**

Run: `cd /Users/peter.lodri/workspace/pldev && go test ./internal/verify/ -run VerifyPQAnchorMatch -v`
Expected: FAIL — undefined: verifyPQ.

- [ ] **Step 4: Implement**

Create `internal/verify/anchors.go`:
```go
package verify

import (
	"bytes"
	_ "embed"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
)

//go:embed pq_signers
var pqSigners []byte

// DefaultPQAnchorPath is the runtime PQ trust-anchor override (synced across the
// fleet by `pldev fleet sync-signers`).
func DefaultPQAnchorPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "pldev", "pq_signers")
}

// PQAnchorBytes returns the effective pq_signers: the runtime file if present and
// non-empty, else the embedded default (mirrors AnchorBytes).
func PQAnchorBytes() []byte {
	if b, err := os.ReadFile(DefaultPQAnchorPath()); err == nil && len(bytes.TrimSpace(b)) > 0 {
		return b
	}
	return pqSigners
}

// verifyPQ returns the principal whose trusted ML-DSA-65 key in anchor verifies
// sig over data, or ("", false). Lines are "<principal> mldsa65 <base64-pub>";
// blank and #-comment lines are skipped.
func verifyPQ(anchor, data, sig []byte) (string, bool) {
	for _, line := range strings.Split(string(anchor), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Fields(line)
		if len(f) != 3 || f[1] != "mldsa65" {
			continue
		}
		pub, err := base64.StdEncoding.DecodeString(f[2])
		if err != nil {
			continue
		}
		if ok, _ := VerifyPQ(pub, data, sig); ok {
			return f[0], true
		}
	}
	return "", false
}
```

- [ ] **Step 5: Run to verify pass**

Run: `cd /Users/peter.lodri/workspace/pldev && go test ./internal/verify/ -run 'PQ|VerifyPQAnchorMatch' -v && go build ./...`
Expected: PASS, builds.

- [ ] **Step 6: Commit**

```bash
cd /Users/peter.lodri/workspace/pldev
git add internal/verify/pq_signers internal/verify/anchors.go internal/verify/pq_test.go
git commit -m "feat(verify): PQ trust anchor (pq_signers) + signer match"
```

---

## Task 3: Extend Verify with the PQ step

**Files:**
- Modify: `internal/verify/verify.go` (the tail of `Verify`)
- Test: `internal/verify/verify_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/verify/verify_test.go` (create with `package verify` + imports if absent). This test drives the dual policy by stubbing the ed25519 `run` indirection and pointing the PQ anchor at a temp file via `HOME`:
```go
import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"

	"github.com/peterlodri-sec/pldev/internal/report"
)

func TestVerifyDualPolicy(t *testing.T) {
	// stub ssh-keygen: find-principals + verify both succeed as "brett-shaw@pldev"
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
	os.WriteFile(bin+".sig", []byte("dummy-ed25519-sig"), 0o644) // presence only; run() is stubbed

	// real PQ sig + a runtime pq_signers anchor in a fake HOME
	pub, priv, _ := GenPQKeyPair()
	sig, _ := SignPQ(priv, data)
	os.WriteFile(bin+".sig.mldsa", []byte(base64.StdEncoding.EncodeToString(sig)), 0o644)

	home := t.TempDir()
	t.Setenv("HOME", home)
	os.MkdirAll(filepath.Join(home, ".config", "pldev"), 0o755)
	os.WriteFile(filepath.Join(home, ".config", "pldev", "pq_signers"),
		[]byte("brett-shaw@pldev mldsa65 "+base64.StdEncoding.EncodeToString(pub)+"\n"), 0o644)

	rep := report.New(os.Stderr, report.Plain)
	if err := Verify(context.Background(), rep, bin); err != nil {
		t.Fatalf("dual-valid binary must verify: %v", err)
	}

	// corrupt the PQ sig → must reject even though ed25519 (stubbed) passes
	os.WriteFile(bin+".sig.mldsa", []byte(base64.StdEncoding.EncodeToString([]byte("garbage"))), 0o644)
	if err := Verify(context.Background(), rep, bin); err == nil {
		t.Fatal("present-but-invalid PQ signature must reject")
	}

	// no PQ sig at all → ed25519-only still valid
	os.Remove(bin + ".sig.mldsa")
	if err := Verify(context.Background(), rep, bin); err != nil {
		t.Fatalf("ed25519-only binary must still verify: %v", err)
	}
}
```
(If `report.New`'s signature differs, mirror how other verify/fleet tests build a reporter.)

- [ ] **Step 2: Run to verify fail**

Run: `cd /Users/peter.lodri/workspace/pldev && go test ./internal/verify/ -run VerifyDualPolicy -v`
Expected: FAIL — the current `Verify` ignores `.sig.mldsa`, so the corrupt-PQ case wrongly passes.

- [ ] **Step 3: Implement**

In `internal/verify/verify.go`, add imports `encoding/base64` (keep existing). Replace the final success block (currently):
```go
	rep.OK("verify", path, report.KV{K: "signer", V: identity})
	return nil
}
```
with:
```go
	// ed25519 verified. Enforce the PQ signature when one is present.
	pqSigPath := path + ".sig.mldsa"
	if _, err := os.Stat(pqSigPath); err == nil {
		b64, err := os.ReadFile(pqSigPath)
		if err != nil {
			rep.Fail("verify", path, err)
			return err
		}
		pqSig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(b64)))
		if err != nil {
			e := fmt.Errorf("malformed PQ signature: %v", err)
			rep.Fail("verify", path, e)
			return e
		}
		pqID, ok := verifyPQ(PQAnchorBytes(), blob, pqSig)
		if !ok {
			e := fmt.Errorf("unverified PQ signature (no trusted ML-DSA key)")
			rep.Fail("verify", path, e)
			return e
		}
		rep.OK("verify", path, report.KV{K: "signer", V: identity}, report.KV{K: "pq", V: pqID})
		return nil
	}
	rep.OK("verify", path, report.KV{K: "signer", V: identity})
	return nil
}
```
(`blob` is already read above for the ed25519 verify; reuse it.)

- [ ] **Step 4: Run to verify pass**

Run: `cd /Users/peter.lodri/workspace/pldev && go test ./internal/verify/ -v && go build ./...`
Expected: PASS (all verify tests), builds.

- [ ] **Step 5: Commit**

```bash
cd /Users/peter.lodri/workspace/pldev
git add internal/verify/verify.go internal/verify/verify_test.go
git commit -m "feat(verify): enforce ML-DSA PQ signature alongside ed25519"
```

---

## Task 4: pq-keygen + pq-sign commands

**Files:**
- Create: `internal/pqkey/pqkey.go`, `internal/pqkey/pqkey_test.go`
- Modify: `main.go`

- [ ] **Step 1: Write the failing test**

Create `internal/pqkey/pqkey_test.go`:
```go
package pqkey

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/peterlodri-sec/pldev/internal/verify"
)

func TestWriteKeyPairAndSign(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "k.key")
	pubPath := keyPath + ".pub" // convention
	pubB64, err := WriteKeyPair(keyPath)
	if err != nil {
		t.Fatal(err)
	}
	// private file perms 0600
	fi, _ := os.Stat(keyPath)
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("private key perms = %o, want 600", fi.Mode().Perm())
	}
	if _, err := os.Stat(pubPath); err != nil {
		t.Fatalf("public key file missing: %v", err)
	}

	// sign a file, verify with the returned public key
	bin := filepath.Join(dir, "artifact")
	data := []byte("artifact bytes")
	os.WriteFile(bin, data, 0o644)
	if err := SignFile(keyPath, bin); err != nil {
		t.Fatal(err)
	}
	sigB64, _ := os.ReadFile(bin + ".sig.mldsa")
	sig := decodeB64(t, string(sigB64))
	pub := decodeB64(t, pubB64)
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
```
Add this helper at the bottom of the same test file:
```go
import "encoding/base64"

func decodeB64(t *testing.T, s string) []byte {
	t.Helper()
	b, err := base64.StdEncoding.DecodeString(trim(s))
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func trim(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == ' ') {
		s = s[:len(s)-1]
	}
	return s
}
```

- [ ] **Step 2: Run to verify fail**

Run: `cd /Users/peter.lodri/workspace/pldev && go test ./internal/pqkey/ -v`
Expected: FAIL — package/functions undefined.

- [ ] **Step 3: Implement**

Create `internal/pqkey/pqkey.go`:
```go
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
```

- [ ] **Step 4: Wire commands in `main.go`**

Add a `pqkey` import and two cases to the top-level positional dispatch (next to `verify`/`setup`). Use the existing reporter/arg conventions in `main.go`:
```go
	case "pq-keygen":
		// pldev pq-keygen [-out <dir>] — default ~/.config/pldev/keys/brett-shaw_mldsa.key
		home, _ := os.UserHomeDir()
		out := filepath.Join(home, ".config", "pldev", "keys")
		for i := 1; i < len(positional); i++ {
			if positional[i] == "-out" && i+1 < len(positional) {
				out = positional[i+1]
			}
		}
		if err := os.MkdirAll(out, 0o700); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		keyPath := filepath.Join(out, "brett-shaw_mldsa.key")
		pubB64, err := pqkey.WriteKeyPair(keyPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		// data → stdout; NEVER print the private key
		fmt.Printf("# ML-DSA-65 keypair written:\n#   private: %s (0600 — VAULT THIS, never commit/push to chat)\n#   public:  %s.pub\n", keyPath, keyPath)
		fmt.Printf("# add this line to internal/verify/pq_signers (and ~/.config/pldev/pq_signers):\nbrett-shaw@pldev mldsa65 %s\n", pubB64)
	case "pq-sign":
		// pldev pq-sign -key <keyfile> <bin>
		var keyPath, bin string
		for i := 1; i < len(positional); i++ {
			switch {
			case positional[i] == "-key" && i+1 < len(positional):
				keyPath = positional[i+1]
				i++
			default:
				bin = positional[i]
			}
		}
		if keyPath == "" || bin == "" {
			fmt.Fprintln(os.Stderr, "usage: pldev pq-sign -key <keyfile> <bin>")
			os.Exit(2)
		}
		if err := pqkey.SignFile(keyPath, bin); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		fmt.Printf("pq-signed: %s.sig.mldsa\n", bin)
```
(Match the actual positional-parsing/`positional` variable name used in `main.go`; adapt the loop to its existing flag handling. If `main.go` parses flags differently, follow its pattern — the behavior, not the exact loop, is what matters.)

- [ ] **Step 5: Run tests + build**

Run: `cd /Users/peter.lodri/workspace/pldev && go test ./... && go build -o pldev . && ./pldev pq-keygen -out /tmp/pqtest && cat /tmp/pqtest/brett-shaw_mldsa.key.pub`
Expected: tests pass; `pq-keygen` prints the `pq_signers` line + paths, NOT the private key; a `.pub` exists. (Clean up `/tmp/pqtest` after.)

- [ ] **Step 6: Commit**

```bash
cd /Users/peter.lodri/workspace/pldev
git add internal/pqkey/ main.go
git commit -m "feat(pqkey): pq-keygen + pq-sign commands (private key never printed)"
```

---

## Task 5: Fleet sync pushes pq_signers

**Files:**
- Modify: `internal/fleet/sync.go` (SyncSigners)
- Modify: `main.go` (pass the PQ anchor through)

- [ ] **Step 1: Implement (no new unit test — exercised by existing sync tests + manual)**

`SyncSigners` currently pushes one anchor file (`allowed_signers`) to each machine. Extend it to also push the PQ anchor. Change its signature to accept a second anchor + local path:
```go
func SyncSigners(ctx context.Context, rep *report.Reporter, machines []Machine, anchor []byte, localPath string, pqAnchor []byte, pqLocalPath string, dryRun, yes bool) error
```
After the existing per-machine `scp` of `allowed_signers`, add a second temp-file write of `pqAnchor` and `scp` it to `~/.config/pldev/pq_signers` on the same node; and in the local-write block, also write `pqAnchor` to `pqLocalPath`. Mirror the existing error/report handling exactly (so a PQ push failure increments `failed` and reports). Keep it DRY — factor the "write temp + scp to dest" into a small local helper if it reduces duplication.

In `main.go`, the `sync-signers` case currently calls:
```go
fleet.SyncSigners(ctx, rep, machines, verify.AnchorBytes(), verify.DefaultAnchorPath(), dryRun, yes)
```
Change to:
```go
fleet.SyncSigners(ctx, rep, machines, verify.AnchorBytes(), verify.DefaultAnchorPath(), verify.PQAnchorBytes(), verify.DefaultPQAnchorPath(), dryRun, yes)
```

Update any existing `SyncSigners(` callers in `internal/fleet/sync_test.go` to pass the two extra args (e.g. `nil, ""` plus a temp path where the test asserts output).

- [ ] **Step 2: Run tests + build**

Run: `cd /Users/peter.lodri/workspace/pldev && go test ./... && go build ./...`
Expected: green.

- [ ] **Step 3: Commit**

```bash
cd /Users/peter.lodri/workspace/pldev
git add internal/fleet/sync.go internal/fleet/sync_test.go main.go
git commit -m "feat(fleet): sync-signers also distributes the PQ trust anchor"
```

---

## Task 6: release.yml + Taskfile dual-sign

**Files:**
- Modify: `.github/workflows/release.yml`
- Modify: `Taskfile.yml`

- [ ] **Step 1: Extend release.yml**

In `.github/workflows/release.yml`, add `MLDSA_SIGNING_KEY` to the build step's `env` (alongside `SSH_SIGNING_KEY`). After the per-artifact `ssh-keygen -Y sign` line, write the ML-DSA key to a temp file and pq-sign each artifact with the freshly-built linux binary. The build loop becomes:
```yaml
        env:
          SSH_SIGNING_KEY: ${{ secrets.SSH_SIGNING_KEY }}
          MLDSA_SIGNING_KEY: ${{ secrets.MLDSA_SIGNING_KEY }}
        run: |
          set -euo pipefail
          install -m 600 /dev/stdin /tmp/pldev-sign.key <<< "$SSH_SIGNING_KEY"
          install -m 600 /dev/stdin /tmp/pldev-mldsa.key <<< "$MLDSA_SIGNING_KEY"
          PKG=github.com/peterlodri-sec/pldev/internal/buildinfo
          LD="-X $PKG.Version=$(git describe --tags --always --dirty) \
              -X $PKG.Commit=$(git rev-parse --short HEAD) \
              -X $PKG.Date=$(git log -1 --format=%cI)"
          mkdir -p dist
          for t in darwin/arm64 darwin/amd64 linux/arm64 linux/amd64; do
            os=${t%/*}; arch=${t#*/}; out=dist/pldev-$os-$arch
            GOOS=$os GOARCH=$arch go build -ldflags "$LD" -o "$out" .
            ssh-keygen -Y sign -n pldev -f /tmp/pldev-sign.key "$out"
          done
          # pq-sign every artifact with the freshly built linux/amd64 binary
          chmod +x dist/pldev-linux-amd64
          for out in dist/pldev-darwin-arm64 dist/pldev-darwin-amd64 dist/pldev-linux-arm64 dist/pldev-linux-amd64; do
            ./dist/pldev-linux-amd64 pq-sign -key /tmp/pldev-mldsa.key "$out"
          done
          rm -f /tmp/pldev-sign.key /tmp/pldev-mldsa.key
```
Update the `gh release create` glob — `dist/*` already includes `.sig` and `.sig.mldsa`, so no change needed there; update the `--notes` to mention the hybrid ed25519 + ML-DSA-65 signatures.

- [ ] **Step 2: Extend Taskfile**

In `Taskfile.yml`, add a `pq-keygen` task and an optional pq-sign to the `sign` task:
```yaml
  pq-keygen:
    desc: 'Generate the ML-DSA-65 release key (prints the pq_signers line; never prints the private key).'
    cmds:
      - go run . pq-keygen

  pq-sign:
    desc: 'PQ-sign a binary. Usage: task pq-sign BIN=dist/pldev-darwin-arm64 KEY=~/.config/pldev/keys/brett-shaw_mldsa.key'
    requires: { vars: [BIN, KEY] }
    cmds:
      - go run . pq-sign -key {{.KEY}} "{{.BIN}}"
      - 'echo "pq-signed: {{.BIN}}.sig.mldsa"'
```

- [ ] **Step 3: Validate workflow YAML + build**

Run: `cd /Users/peter.lodri/workspace/pldev && go build ./... && go vet ./... && python3 -c "import yaml,sys; yaml.safe_load(open('.github/workflows/release.yml')); print('yaml ok')"`
Expected: build/vet clean, `yaml ok`.

- [ ] **Step 4: Commit**

```bash
cd /Users/peter.lodri/workspace/pldev
git add .github/workflows/release.yml Taskfile.yml
git commit -m "feat(release): hybrid ed25519 + ML-DSA-65 dual-signing in CI + Taskfile"
```

---

## Final: full build + cross-compile

- [ ] Run: `cd /Users/peter.lodri/workspace/pldev && go test ./... && go vet ./... && GOOS=linux GOARCH=amd64 go build ./... && GOOS=darwin GOARCH=arm64 go build ./...`
  Expected: all green, both cross-compiles succeed.

---

## Live runbook (NOT code tasks — user-driven, after the PR merges; outward steps confirm-gated)

These create the REAL keys/secrets and populate the trust anchors. Do these once, manually:

1. **ed25519 CI key:** `ssh-keygen -t ed25519 -N "" -C brett-shaw@pldev -f ~/.ssh/id_pldev_ci` → vault `~/.ssh/id_pldev_ci`.
2. **ML-DSA key:** `pldev pq-keygen` → writes `~/.config/pldev/keys/brett-shaw_mldsa.key` (vault it) + prints the `pq_signers` line.
3. **Populate anchors (commit):** append `brett-shaw@pldev ssh-ed25519 <pub from id_pldev_ci.pub>` to `internal/verify/allowed_signers`; append the printed `brett-shaw@pldev mldsa65 <b64>` line to `internal/verify/pq_signers`. Commit + a follow-up PR (so the embedded anchors trust brett-shaw).
4. **Local proof:** `task build` → sign `./pldev` with both (`task sign KEY=~/.ssh/id_pldev_ci` + `task pq-sign BIN=./pldev KEY=~/.config/pldev/keys/brett-shaw_mldsa.key`) → `pldev verify ./pldev` shows `signer=brett-shaw@pldev pq=brett-shaw@pldev`; tamper a byte → reject.
5. **GitHub secrets (OUTWARD — confirm first):** `gh secret set SSH_SIGNING_KEY < ~/.ssh/id_pldev_ci` and `gh secret set MLDSA_SIGNING_KEY < ~/.config/pldev/keys/brett-shaw_mldsa.key`.
6. **Tag:** `git tag v0.0.1 && git push origin v0.0.1` → `release.yml` builds + dual-signs 4 platforms. Download one + `pldev verify` it.

---

## Self-review notes (addressed)

- Spec coverage: pq.go crypto (T1), pq_signers anchor + match (T2), Verify dual policy (T3), pq-keygen/pq-sign + private-never-printed (T4), fleet PQ anchor sync (T5), release.yml + Taskfile dual-sign (T6), live key/secret runbook (final section). All spec sections mapped.
- Type consistency: `GenPQKeyPair()(pub,priv,err)`, `SignPQ(priv,data)`, `VerifyPQ(pub,data,sig)(bool,err)`, `verifyPQ(anchor,data,sig)(string,bool)`, `PQAnchorBytes()`, `DefaultPQAnchorPath()`, `pqkey.WriteKeyPair(path)(string,err)`/`LoadPriv`/`SignFile` — consistent across tasks and main.go wiring.
- YAGNI: SPHINCS+/PQ-KEX excluded per spec; no key rotation tooling (revoke = delete the anchor line).
