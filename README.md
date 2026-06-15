# pldev

Single-entrypoint personal machine helper. Modules: `doctor mem`, `ps`, `setup`.

## Build
    task build      # -> ./pldev   (stamps version/commit/date)
    task install    # -> ~/.local/bin/pldev
    task version    # build + print embedded build info
    task test
    pldev --version # pldev <ver> (<sha>, <commit-date>, go1.x)

## Use
    pldev                 # interactive root menu
    pldev doctor mem      # memory doctor (report + guided fixes)
    pldev doctor mem --report   # non-interactive dump (ssh/scripts)
    pldev ps              # neofetch-style system overview panel
    pldev setup              # interactive tool installer
    pldev setup @dev         # install the 'dev' profile
    pldev setup jq+bun --dry-run

Flags: --report, --yes (safe fixes only), --no-color, --version.

## Releasing

Build, dual-sign (ed25519 + ML-DSA-65), and verify locally on the M3:

    pldev release v0.0.2            # dry-run: artifacts + sigs in dist/, prints notes
    pldev release v0.0.2 --publish  # triple-ack gate, then GitHub upload

Publish requires a token (`gh auth login` or `GH_TOKEN`) and the signing keys
(`~/.ssh/id_pldev_ci`, `~/.config/pldev/keys/brett-shaw_pldev_mldsa.key`).
Verify any artifact with `pldev verify <file>`.

## Action log

Every command emits a one-action-per-line log to **stderr** (data output stays on
stdout, pipe-clean). Lines are logfmt:

    ts=2026-06-15T11:40:02Z lvl=info scope=pldev>setup action=install target=ripgrep status=ok dur=820ms

`status` ∈ ok/fail/skip/have/do/dry. Control via env:

- `PLDEV_LOG=plain|pretty|json` — force render mode (default: pretty on a TTY, plain when piped).
- `NO_COLOR=1` — disable color (forces plain).
- `PLDEV_NESTED` — set automatically for child pldev processes; forces plain + nested scope (`parent>child`). Don't set by hand.
