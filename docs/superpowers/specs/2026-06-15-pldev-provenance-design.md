# pldev #3 — binary provenance (design)

## Context

pldev is built/installed across the user's machines (M1/M3/dev boxes) via `task build`/`task install`. There is no way to confirm a given `pldev` binary was built by the user and not tampered with. #0 already embeds build info (`internal/buildinfo`: version/commit/date); #3 makes that **verifiable** — sign the binary and let any machine prove provenance before trusting it.

Goal (user-chosen): **binary provenance** — verify the pldev you run was built by you, unmodified. Not commit-signing, not the runtime identity/cabotage gate (both deferred to their own thread).

Fourth of the sequence (#0 logging → #1 engine → #2 ruflo → **#3 provenance**).

## Decisions (locked)

- **Two signing paths:** CI **keyless** (GitHub Actions OIDC → Fulcio cert → Rekor transparency log) for published releases; **local key-based** fallback for offline/ad-hoc signing.
- **Local key custody:** `cosign generate-key-pair`; encrypted private key on disk, password in **Vaultwarden** (fetched via `bw`, biometric); `cosign.pub` committed.
- **Verify UX:** a `pldev verify` subcommand; `task install` / `setup` verifies post-download.

## Architecture

```
tag v* ─► .github/workflows/release.yml (id-token: write)
            build 4 platforms with #0 build-info ldflags
            cosign sign-blob   --yes  (keyless: GH OIDC → Fulcio → Rekor)  → <bin>.sig, <bin>.pem
            cosign attest             (SLSA provenance: version/commit/date/builder)
            gh release upload  binaries + .sig + .pem + .att

local fallback:  task sign   → password=$(bw …) ; cosign sign-blob --key cosign.key <bin> → <bin>.sig

verify:  pldev verify [path]  → shells cosign verify-blob
            release binary → keyless identity (cert-identity + oidc issuer)
            local-signed   → embedded cosign.pub
            prints verified provenance, cross-checks #0 buildinfo

install: task install / `setup` recipes → pldev verify post-download (warn/abort on fail)
```

## Components (all in the pldev repo)

### 1. Release workflow — `.github/workflows/release.yml`
- Trigger: push tag `v*`. Permissions: `contents: write`, `id-token: write`.
- Build the 4 platform binaries with the same `-ldflags` build-info stamping as `task cross`.
- `cosign sign-blob --yes` each binary (keyless) → `.sig` + `.pem` (cert).
- `cosign attest` a SLSA-style provenance predicate (version, commit, date, builder = the workflow).
- `gh release create/upload` the binaries + signatures + certs + attestations.

### 2. Local signing — `Taskfile`
- `task sign` — `cosign generate-key-pair` if absent; `COSIGN_PASSWORD=$(bw get password <item>)` then `cosign sign-blob --key cosign.key --yes <bin>` → `<bin>.sig`. The key password never lands in a file or pldev's logs.
- `cosign.pub` committed to the repo (the local-trust anchor).

### 3. `internal/verify` + `pldev verify`
- `pldev verify [path]` (default: the running executable, via `os.Executable()`).
- Shells `cosign verify-blob`. Two trust modes, tried in order:
  - **keyless** (release binaries): `--certificate-identity-regexp <release.yml identity>` `--certificate-oidc-issuer https://token.actions.githubusercontent.com`, against the binary's `.sig` + `.pem`.
  - **local**: `--key cosign.pub` (embedded via `go:embed`) against the `.sig`.
- On success, print the verified provenance and cross-check it against `buildinfo.String()` (claimed vs verified version/commit); flag a mismatch.
- `cosign` absent → exit with a clear "install cosign (pldev setup adds it)" message, not a crash. Emits through `internal/report`.
- Trusted identity constants (cert-identity regexp `https://github.com/peterlodri-sec/pldev/.github/workflows/release.yml@refs/tags/v.*`, issuer) live in one place.

### 4. Trust material
- `cosign.pub` committed (local-trust anchor, embedded via `go:embed` for `pldev verify`).
- Keyless identity baked as constants in `internal/verify`.

### 5. Install integration
- `task install` and the `setup` recipes (`setup ruflo`/`arp` already install tools) gain a verify step: after placing a release binary, run `pldev verify` and warn (or abort, configurable) on failure. `cosign` added to the tool registry so it's installable.

## Safety / contract

- Keyless signing leaks **no** long-lived key (ephemeral Fulcio cert; Rekor is public-good transparency — note: the binary name + cert identity become public in Rekor, which is acceptable for a personal tool but recorded here).
- The local key password flows only through `bw` → `COSIGN_PASSWORD` env at sign time, never to disk or logs.
- `pldev verify` is read-only; it never mutates the binary or trusts an unverifiable artifact silently.
- Bootstrap-trust caveat: `pldev verify` trusts the embedded `cosign.pub` / identity constants compiled into *that* pldev — verifying a downloaded binary with a separately-trusted pldev (or `cosign` directly) is the clean chain; documented.

## Verifiable now vs deferred

- **Now (this spec's build):** `internal/verify` + `pldev verify` Go code + unit tests (mock the `cosign` invocation; assert argv, trust-mode selection, buildinfo cross-check, absent-cosign degradation); `release.yml`; `Taskfile` `sign` target; `cosign` in the registry; embed `cosign.pub` (a placeholder until the first real keypair).
- **Deferred to a live tag (documented):** an actual keyless-signed release (needs a real `v*` tag + GH OIDC) and a real local keypair. The code + workflow are exercised then.

## Verification

1. `go build/vet/test ./...` green; verify tests pass (argv construction, trust-mode pick, degradation).
2. `pldev verify` on an unsigned binary with `cosign` present → clean "unverified" failure; with `cosign` absent → actionable error, no crash.
3. `release.yml` lints (actionlint / yaml) and its steps are inspectable.
4. Live (deferred): tag `v0.0.1` → workflow signs + attests + uploads; `pldev verify` on the downloaded binary prints verified provenance matching `--version`.
