# pldev Backlog

Future features / extensions beyond shipped v1 (`doctor mem`, `ps`, `setup`).

## doctor mcp
Detect orphaned / duplicate MCP-server processes and offer to kill.
- **Why:** `npx`/`npm exec` MCP servers leak — inner node reparents to PID 1 on unclean client exit and never dies (observed 100+ stale procs, some 16-day-old PPID-1 orphans). macOS has no cgroup to reap them.
- **Detect:** procs matching `mcp|npm exec` with `PPID==1`, OR N duplicates of the same server name. Trivial on top of the existing `probe.Proc` data.
- **Fix offered:** kill orphans (safe — clients respawn on demand). Print summary.
- **Note:** tighten the `doctor mem` zombie heuristic too — "user LaunchAgent w/ negative exit status" is too loose (healthy agents stopped at logout also show -15). Prefer missing-binary OR rapid-respawn signal.

## setup term (recipe extension)
Composite "recipe" flow: `pldev setup term` → install Ghostty (+ zellij, Nerd Font, starship) → write themed config (light-cyberpunk) → perf tweaks → interactive plugin/extension install.
- **Gap:** current `setup` spec is package-install only. `setup term` needs **file-writing/templating + config-backup** capability the engine doesn't have yet.
- **Decision:** model as a "recipe" subsystem (install steps + templated config files + backup-before-overwrite), separate spec.
- Ghostty builds with Zig (not cargo); wild/mold/lld = link-step only, mimalloc = runtime preload — offer source-build as opt-in, default to brew cask.

## sync --prune
Declarative convergence: uninstall tools not in the chosen profile. Destructive — opt-in flag, explicit confirm. (Deferred from setup v1.)

## Nix backend
Add a Nix/home-manager `Installer` impl for reproducible setups. Cheap given the `Installer` interface; deferred from setup v1.
