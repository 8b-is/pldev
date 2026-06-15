# pldev

Single-entrypoint personal machine helper. v1: `doctor mem`.

## Build
    task build      # -> ./pldev
    task install    # -> ~/.local/bin/pldev
    task test

## Use
    pldev                 # interactive root menu
    pldev doctor mem      # memory doctor (report + guided fixes)
    pldev doctor mem --report   # non-interactive dump (ssh/scripts)

Flags: --report, --yes (safe fixes only), --no-color.
