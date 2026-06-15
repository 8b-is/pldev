# pldev release — design spec

**Date:** 2026-06-15
**Status:** Approved (brainstorm)
**Author:** peter.lodri

## Goal

Codify the manual M3-local release ritual into one command. Today a release is
a hand-run chain: `task cross` (4 platforms) → `task sign` per binary
(ssh-keygen ed25519) → `task pq-sign` per binary (ML-DSA-65) → `gh release
create`. `pldev release <tag>` replaces that with a single, fail-closed,
verify-before-publish pipeline that builds and signs locally on the M3 and
publishes to GitHub only behind an explicit, hard-to-fumble gate.

Serves the standing directive: **releases build + sign on M3 locally, never
wait on CI** (GitHub Actions billing intentionally blocked).

## Command surface

```
pldev release <tag>              # dry-run: build → sign → verify → print plan. No network write.
pldev release <tag> --publish    # same pipeline + TRIPLE-ack gate → GitHub upload.
```

- `<tag>` required, e.g. `v0.0.2`. Validated shape `vMAJOR.MINOR.PATCH`.
- Optional `--notes-file <path>` overrides auto-generated notes body (provenance
  block still appended).
- Dispatched as a direct subcommand in `main.go`, mirroring the existing
  `arp` / `fed` / `fleet` blocks. Logs via `rep.Sub("release")`.

## New package: `internal/release/`

All logic lives here, behind interfaces so build / sign / publish are mockable.
`main.go` only parses args and calls `release.Run(ctx, rep, opts)`.

Key seams (interfaces):
- `Runner` — wraps `os/exec` (build + ssh-keygen). Mock in tests.
- `Publisher` — GitHub REST client. Mock via `httptest` roundtripper.
- PQ signing reuses `internal/pqkey` directly (Go / cloudflare/circl).
- Verify reuses `internal/verify` directly.

## Pipeline stages (fail-closed; stop on first error)

### 1. Preflight
- Git tree clean (no uncommitted changes).
- Tag not already published on the remote (GET releases/tags/<tag> → expect 404).
- `dist/` exists and is writable.
- Required keys present:
  - ed25519: `~/.ssh/id_pldev_ci`
  - ML-DSA:  `~/.config/pldev/keys/brett-shaw_pldev_mldsa.key`
- Token resolvable: `gh auth token` first, then `GH_TOKEN` env. (Resolution only
  — not required for dry-run, but checked + warned so publish won't surprise.)

### 2. Build
- Compute ldflags in Go (no Taskfile dependency):
  - `Version` = `git describe --tags --always --dirty` (or the passed tag).
  - `Commit`  = `git rev-parse --short HEAD`.
  - `Date`    = `git log -1 --format=%cI`.
  - Injected into `github.com/peterlodri-sec/pldev/internal/buildinfo`.
- `os/exec` `go build -ldflags "..."` for each target:
  - darwin/arm64, darwin/amd64, linux/arm64, linux/amd64
  - output `dist/pldev-<os>-<arch>`.

### 3. Sign (per artifact)
- ed25519: `ssh-keygen -Y sign -n pldev -f ~/.ssh/id_pldev_ci <artifact>` → `<artifact>.sig`.
  (Passphrase/agent-protected key; no secret on the CLI.)
- ML-DSA-65: internal `pqkey` sign with the brett-shaw key → `<artifact>.sig.mldsa`.

### 4. Verify gate
- Call internal `verify` on each artifact: ed25519 mandatory + ML-DSA enforced
  when `.sig.mldsa` present (fail-closed).
- ANY verification failure → abort. No publish. Artifacts left in `dist/` for
  inspection.

### 5. Notes
- Body = `git log <prevtag>..HEAD` subjects, grouped by conventional-commit
  prefix (`feat`, `fix`, other). `<prevtag>` = most recent tag before `<tag>`.
- `--notes-file` overrides the git-log body if supplied.
- Always append a **provenance block**:
  - sha256 table for each artifact + its `.sig` / `.sig.mldsa`.
  - signer identity line per artifact: `signer=brett-shaw@pldev pq=brett-shaw@pldev`.

## Dry-run vs publish

- **Dry-run (default):** runs stages 1–5, writes real signed artifacts to
  `dist/`, prints the full plan + notes + provenance block. No network write.
  Lets the operator inspect before re-running with `--publish`.
- **Publish (`--publish`):** stages 1–5, then the gate, then upload.

### Triple-ack interactive gate (publish only)

Requires a TTY (stdin). Three acknowledgments in sequence:
1. Shows tag + artifact list → operator types `y`.
2. Shows "this publishes to GitHub and is irreversible" → operator types `y`.
3. **Non-deterministic challenge:** `crypto/rand` generates a short hex token
   (e.g. `7F3A`); prompt prints it; operator must type it back exactly.
   Defeats blind-Enter mashing and scripted auto-confirm.

Any mismatch / EOF / non-TTY → abort without publishing.

### Upload
- GitHub REST via `net/http` (no go-github / gh-CLI dependency for the write):
  - `POST /repos/peterlodri-sec/pldev/releases` with `{tag_name, name, body}`.
  - For each artifact + `.sig` + `.sig.mldsa`: `POST` to the release
    `upload_url` as a binary asset.
- HTTP client hardening:
  - `http.Transport{ TLSClientConfig: &tls.Config{ MinVersion: tls.VersionTLS12 } }`.
  - Sane timeouts on the client.
  - `Authorization: Bearer <token>`, `Accept: application/vnd.github+json`,
    `X-GitHub-Api-Version` header.

## Error handling
- Each stage wraps context via `report.KV` and the `report.Reporter` OK/Fail API.
- On any pre-publish failure: partial `dist/` left for inspection; non-zero exit.
- On publish failure **after** the release was created remotely: print the
  release URL + the remaining manual upload command. Do **not** auto-delete the
  remote release (avoid destructive rollback of a half-published tag).

## Testing
- Unit (no real network / no real builds):
  - ldflags computation from a fake git state.
  - notes/changelog grouping (feat/fix/other) from sample `git log` output.
  - sha256 provenance table rendering.
  - challenge generation + match (inject a deterministic rand reader).
  - token resolution order (`gh auth token` → `GH_TOKEN`).
- Build / sign / publish exercised through mocked `Runner` + `httptest`
  `Publisher`. Verify gate tested against known-good and tampered fixtures.

## Out of scope (YAGNI)
- Multi-repo / monorepo releases.
- Auto version bump / changelog file commit.
- Windows builds.
- Rollback / un-publish automation.
