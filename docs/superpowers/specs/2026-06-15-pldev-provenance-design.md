# pldev #3 — binary provenance (design)

## Context

pldev is built/installed across the user's machines (M1/M3/dev boxes) via `task build`/`task install`. There is no way to confirm a given `pldev` binary was built by the user and not tampered with. #0 already embeds build info (`internal/buildinfo`: version/commit/date); #3 makes that **verifiable** — sign the binary and let a trusted pldev prove a candidate binary's provenance before it's installed.

Goal (user-chosen): **binary provenance** — verify a pldev binary was built by you, unmodified. Not commit-signing, not the runtime identity/cabotage gate (both deferred to their own thread).

Fourth of the sequence (#0 logging → #1 engine → #2 ruflo → **#3 provenance**).

## Decisions (locked)

- **Key-based signing only — nothing public.** A single `cosign` key pair is the trust anchor; `--tlog-upload=false` everywhere, so **no public Rekor/transparency-log entry** and no Fulcio/OIDC. Keyless was rejected: it would leak the build identity to the public Rekor log even though the repos stay **private**.
- **One key, two sign sites.** Local `task sign` (password via Vaultwarden/`bw`) and the CI release workflow both sign with the same key; CI gets the encrypted key + password as GitHub Actions secrets.
- **Local key custody:** `cosign generate-key-pair` → encrypted `cosign.key` (password in Vaultwarden, fetched via `bw`) + `cosign.pub` committed to the repo.
- **Verify-the-target, never self-verify.** `pldev verify <path>` verifies a *candidate* binary using the already-trusted running pldev. Startup self-verify is rejected as circular (a tampered binary would lie). Binary #1 is bootstrapped once with raw `cosign` (documented); thereafter a trusted pldev vouches for the next.

## Architecture

```
cosign generate-key-pair (once) → cosign.key (encrypted) + cosign.pub (committed, go:embed)

tag v* ─► .github/workflows/release.yml
            build 4 platforms with #0 build-info ldflags
            cosign sign-blob --key env://COSIGN_KEY --tlog-upload=false --yes <bin> → <bin>.sig
            gh release upload  binaries + .sig
          (COSIGN_KEY + COSIGN_PASSWORD = GitHub Actions secrets)

local:   task sign  →  COSIGN_PASSWORD=$(bw get password <item>)
                        cosign sign-blob --key cosign.key --tlog-upload=false <bin> → <bin>.sig

verify:  pldev verify <path>   (path = a candidate binary; NOT self)
            cosign verify-blob --key <embedded cosign.pub>
                               --signature <path>.sig --insecure-ignore-tlog <path>
            on success: cross-check the candidate's buildinfo claim; print provenance

install: task install / `setup` recipes → download binary + .sig → pldev verify → place on success
bootstrap: binary #1 verified once with raw cosign (documented), then it vouches for the rest
```

No transparency log, no Fulcio, no OIDC — trust is the committed `cosign.pub` (trusted via your GitHub auth to the private repo).

## Components (all in the pldev repo)

### 1. Release workflow — `.github/workflows/release.yml`
- Trigger: push tag `v*`. Permissions: `contents: write` (no `id-token`).
- Build the 4 platform binaries with the same `-ldflags` build-info stamping as `task cross`.
- `cosign sign-blob --key env://COSIGN_KEY --tlog-upload=false --yes <bin>` → `<bin>.sig`, using `COSIGN_KEY` + `COSIGN_PASSWORD` repo secrets.
- `gh release create/upload` the binaries + `.sig` files.

### 2. Local signing — `Taskfile`
- `task keygen` — `cosign generate-key-pair` (one-time); reminder to store the password in Vaultwarden + commit `cosign.pub`.
- `task sign` — `COSIGN_PASSWORD=$(bw get password <item>) cosign sign-blob --key cosign.key --tlog-upload=false --yes <bin>` → `<bin>.sig`. Password only via env at sign time, never on disk/logs.

### 3. `internal/verify` + `pldev verify`
- `pldev verify <path>` — verifies the candidate at `<path>` (required arg; no implicit self-target). Expects `<path>.sig` beside it.
- Shells `cosign verify-blob --key <pub> --signature <path>.sig --insecure-ignore-tlog <path>`, where `<pub>` is the `cosign.pub` embedded via `go:embed` (written to a temp file for cosign).
- On success: read the candidate's claimed build info if available and print verified provenance; the running pldev's own `buildinfo` is shown for context. Mismatch/failure → non-zero, clear message.
- `cosign` absent → actionable error ("install cosign — `pldev setup` adds it"), not a crash. Emits through `internal/report`.

### 4. Trust material
- `cosign.pub` committed + embedded via `go:embed` (the sole trust anchor). Ships as a placeholder until the first real `task keygen`; documented.

### 5. Install integration
- `cosign` added to the tool registry (installable; currently MISSING on M3).
- `task install` and `setup` recipes gain an optional verify step: when installing a release binary, run `pldev verify` and **warn by default** (abort opt-in) on failure — non-blocking so a missing sig doesn't break install.

## Safety / contract

- **No public exposure:** `--tlog-upload=false` everywhere; private repos stay fully private, nothing hits a public log.
- The local key password flows only through `bw` → `COSIGN_PASSWORD` env at sign time, never to disk or pldev logs. CI key + password are GitHub Actions secrets.
- `pldev verify` is read-only and verifies a *target*; it never self-verifies (circular) and never silently trusts an unverifiable artifact.
- Bootstrap-trust is explicit: the committed `cosign.pub` is trusted via GitHub repo auth; binary #1 is verified once with raw `cosign`; the trusted pldev then vouches for subsequent binaries.

## Verifiable now vs deferred

- **Now (this build):** `internal/verify` + `pldev verify` Go code + unit tests (mock the `cosign` invocation — assert argv, the embedded-pub temp-file handling, missing-`.sig` and absent-`cosign` degradation, buildinfo print); `release.yml`; `Taskfile` `keygen`/`sign`; `cosign` in the registry; `cosign.pub` placeholder embedded.
- **Deferred to a live tag (documented):** a real `task keygen` keypair + Vaultwarden item, the GH Actions secrets, and an actual signed `v*` release. Code + workflow are exercised then.

## Verification

1. `go build/vet/test ./...` green; verify tests pass (argv, temp-pub handling, degradations).
2. `pldev verify <unsigned-bin>` with `cosign` present → clean "unverified" failure; with `cosign` absent → actionable error, no crash; missing `.sig` → clear message.
3. `release.yml` lints (actionlint/yaml), steps inspectable.
4. Live (deferred): `task keygen`; sign a binary locally; `pldev verify` prints verified provenance; tamper a byte → verify fails.
