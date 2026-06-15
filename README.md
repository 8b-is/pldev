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

## Action log

Every command emits a one-action-per-line log to **stderr** (data output stays on
stdout, pipe-clean). Lines are logfmt:

    ts=2026-06-15T11:40:02Z lvl=info scope=pldev>setup action=install target=ripgrep status=ok dur=820ms

`status` ∈ ok/fail/skip/have/do/dry. Control via env:

- `PLDEV_LOG=plain|pretty|json` — force render mode (default: pretty on a TTY, plain when piped).
- `NO_COLOR=1` — disable color (forces plain).
- `PLDEV_NESTED` — set automatically for child pldev processes; forces plain + nested scope (`parent>child`). Don't set by hand.
