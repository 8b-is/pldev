# pldev

Single-entrypoint personal machine helper. Modules: `doctor mem`, `ps`.

## Build
    task build      # -> ./pldev
    task install    # -> ~/.local/bin/pldev
    task test

## Use
    pldev                 # interactive root menu
    pldev doctor mem      # memory doctor (report + guided fixes)
    pldev doctor mem --report   # non-interactive dump (ssh/scripts)
    pldev ps              # neofetch-style system overview panel

Flags: --report, --yes (safe fixes only), --no-color.
