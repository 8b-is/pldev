# pldev

Single-entrypoint personal machine helper. Modules: `doctor mem`, `ps`, `setup`.

## Build
    task build      # -> ./pldev
    task install    # -> ~/.local/bin/pldev
    task test

## Use
    pldev                 # interactive root menu
    pldev doctor mem      # memory doctor (report + guided fixes)
    pldev doctor mem --report   # non-interactive dump (ssh/scripts)
    pldev ps              # neofetch-style system overview panel
    pldev setup              # interactive tool installer
    pldev setup @dev         # install the 'dev' profile
    pldev setup jq+bun --dry-run

Flags: --report, --yes (safe fixes only), --no-color.
