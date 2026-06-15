# Quantum-Resistant Release Signing — Design

**Date:** 2026-06-15
**Subsystem:** `internal/verify` + a small `pq-keygen`/`pq-sign` command surface + `release.yml`
**Status:** approved
**Depends on:** the existing ssh-keygen provenance (`internal/verify/verify.go`, `allowed_signers`, `release.yml`).

## Motivation

The current release/provenance path signs with ed25519 via `ssh-keygen -Y sign` — broken by Shor's algorithm on a sufficiently large quantum computer. `ssh-keygen` has no post-quantum signature key type (OpenSSH 10 added only PQ *key exchange*), so a genuine PQ signature needs a real PQ scheme via a Go library.

This adds a **hybrid** signature: every release is signed with BOTH the existing ed25519 (ssh-keygen, unchanged trust path) AND an **ML-DSA-65** (FIPS 204) post-quantum signature. The classical path keeps working; the PQ signature is defense-in-depth and future-proofing. The signing identity `brett-shaw@pldev` carries both keys.

Note on threat model (documented, not a blocker): for *signatures* (unlike encryption) "harvest-now-forge-later" barely applies — a forged signature only helps before detection, and past releases are immutable. PQ release-signing here is future-proofing, not an urgent need.

## Scope

**In:**
- `internal/verify/pq.go` — ML-DSA-65 sign/verify over `github.com/cloudflare/circl/sign/mldsa/mldsa65`.
- `internal/verify/pq_signers` (go:embed, runtime-overridable) — PQ trust anchor.
- `pldev pq-keygen` — generate an ML-DSA-65 keypair to files (prints public key + paths only).
- `pldev pq-sign -key <f> <bin>` — write `<bin>.sig.mldsa`.
- `Verify` extended: enforce ed25519 (mandatory) + ML-DSA (when a `.sig.mldsa` and PQ anchor exist).
- `release.yml` + `Taskfile` dual-sign.

**Out:**
- Replacing ssh-keygen (the ed25519 path stays primary trust).
- PQ key exchange / encryption (signatures only).
- SPHINCS+ / other PQ schemes (ML-DSA-65 chosen: NIST L3, ~3.3KB sigs, fast).
- Auto-pushing GitHub secrets (the user pushes them after confirm).

## Architecture

### `internal/verify/pq.go` (new)

```go
// ML-DSA-65 (FIPS 204) post-quantum signatures via cloudflare/circl.
package verify

import "github.com/cloudflare/circl/sign/mldsa/mldsa65"

// GenPQKeyPair returns a fresh ML-DSA-65 keypair (raw scheme bytes).
func GenPQKeyPair() (pub, priv []byte, err error)

// SignPQ signs data with a raw ML-DSA-65 private key, returning the signature.
func SignPQ(privRaw, data []byte) ([]byte, error)

// VerifyPQ reports whether sig is a valid ML-DSA-65 signature of data under pubRaw.
func VerifyPQ(pubRaw, data, sig []byte) (bool, error)
```

Keys are stored as the scheme's raw bytes; transported/stored as base64 (files hold base64 text, one line). The functions unmarshal via `mldsa65.Scheme().UnmarshalBinary{Public,Private}Key`.

### PQ trust anchor — `internal/verify/pq_signers` (new, go:embed)

Format mirrors `allowed_signers` but for ML-DSA:
```
<principal> mldsa65 <base64 public key>
```
e.g. `brett-shaw@pldev mldsa65 <b64...>`.

```go
//go:embed pq_signers
var pqSigners []byte

// DefaultPQAnchorPath: ~/.config/pldev/pq_signers (runtime override, synced by fleet sync-signers).
func DefaultPQAnchorPath() string

// PQAnchorBytes: runtime file if present+non-empty, else embedded (mirrors AnchorBytes).
func PQAnchorBytes() []byte

// pqPrincipalFor(pub) → the principal whose key matches, or "" (untrusted).
```

`fleet sync-signers` (phase already pushes `allowed_signers`) is extended to also push `pq_signers` so every node trusts the same PQ identities. (Small addition to `SyncSigners`: push a second file.)

### `Verify` policy (extend `internal/verify/verify.go`)

After the existing ed25519 path succeeds, add the PQ step:
1. ed25519 (`.sig`) — **mandatory**, unchanged (untrusted/invalid → reject).
2. If `path + ".sig.mldsa"` exists: read the PQ anchor, find the principal whose ML-DSA pubkey verifies the signature over the file bytes; **fail if none** (a present PQ sig that doesn't verify against a trusted PQ key → reject). If no `.sig.mldsa`, skip (locally `task sign`'d binaries remain valid).
3. Report both signers (`signer=brett-shaw@pldev pq=brett-shaw@pldev`).

Rule: *verify every signature that exists; require ed25519; never accept if any present signature fails.*

### Commands (`main.go` dispatch + a thin command layer)

- `pldev pq-keygen [-out <dir>]` → writes `<dir>/brett-shaw_mldsa.key` (base64 private, 0600) + `.pub` (base64 public, 0644); prints the public key and the `pq_signers` line to paste; **never prints the private key**. Default dir `~/.config/pldev/keys/`.
- `pldev pq-sign -key <keyfile> <bin>` → writes `<bin>.sig.mldsa` (base64). Used in CI.

### CI (`release.yml`) + `Taskfile`

- Add secret `MLDSA_SIGNING_KEY` (base64 ML-DSA private) alongside `SSH_SIGNING_KEY`.
- After building each artifact and ssh-signing it (unchanged), pq-sign it: run the freshly-built `dist/pldev-linux-amd64` as `pldev pq-sign -key <tmp> <artifact>` on the ubuntu runner.
- `Taskfile` `sign` target gains an optional ML-DSA step; add `pq-keygen` convenience.
- Release notes mention both signatures.

## Keys & paths (user vaults the privates — never printed by pldev or in chat)

- ed25519 CI key: `~/.ssh/id_pldev_ci` (+`.pub`), principal `brett-shaw@pldev` — via `ssh-keygen -t ed25519 -N "" -C brett-shaw@pldev`.
- ML-DSA key: `~/.config/pldev/keys/brett-shaw_mldsa.key` (+`.pub`) — via `pldev pq-keygen`.
- Anchors: add the ed25519 line to `internal/verify/allowed_signers`; add the ML-DSA line to `internal/verify/pq_signers`. Both committed.
- GitHub secrets (outward, confirm-gated): `SSH_SIGNING_KEY` (ed25519 private), `MLDSA_SIGNING_KEY` (base64 ML-DSA private).

## Error handling

- `circl` unmarshal failure (corrupt key/sig) → `VerifyPQ` returns `(false, err)`; `Verify` rejects.
- Missing PQ anchor at runtime → fall back to embedded `pq_signers`; if a `.sig.mldsa` is present but no trusted PQ key matches → reject (do not silently pass).
- A binary with ed25519 sig but no `.sig.mldsa` → valid (PQ optional unless present), so old/locally-signed binaries still verify.
- `pq-keygen` refuses to overwrite an existing key file unless `-force` (avoid clobbering a vaulted key).

## Testing

- `GenPQKeyPair`→`SignPQ`→`VerifyPQ` round-trip; tampered data → false; wrong key → false; corrupt sig/key bytes → error.
- `pq_signers` parse: principal lookup by pubkey match; unknown key → "" ; runtime override beats embedded.
- `Verify` dual-policy (table tests with the `run`/file indirection): ed25519 ok + PQ ok → pass; ed25519 ok + PQ present-but-bad → reject; ed25519 ok + no PQ sig → pass; ed25519 bad → reject regardless of PQ.
- `pq-keygen` writes 0600 private / 0644 public, never logs the private; `-force` guard.

## Verification

1. `go test ./internal/verify/...` green; `go build ./...`; cross-compiles.
2. `pldev pq-keygen` produces the keypair + prints only the public line.
3. Locally: build a binary, `task sign` (ed25519) + `pldev pq-sign` → `pldev verify` reports `signer=brett-shaw@pldev pq=brett-shaw@pldev`; tamper a byte → reject; remove the PQ trust line → reject the PQ sig.
4. After secrets pushed + `v0.0.1` tagged: a downloaded release binary verifies both signatures.
