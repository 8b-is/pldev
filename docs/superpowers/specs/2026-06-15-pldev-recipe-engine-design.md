# pldev #1 — recipe engine (design)

## Context

`pldev setup` (v1) installs packages only. Two upcoming flows — `setup ruflo` (#2) and `setup term` (backlog) — need to write config files, append shell-rc lines, register MCP servers, run commands, and verify outcomes. They share the same step vocabulary, so this spec adds a small generic **recipe step engine** (Option A, user-approved) that those flows express as data.

This is the second of the decomposed sequence (#0 logging → **#1 engine** → #2 setup ruflo → #3 signing). It builds on #0: every step emits through `internal/report`.

**Scope boundary (user-approved):** #1 ships the six *local-mutation* step types + the runner. The `Provision` step and the `SecretProvider`/`Authenticator`/`RemoteRunner` interfaces are tied to the remote stack + bao/bw and land in #2 with their impls. #1 ships as a tested library; `main` dispatch (`pldev setup <recipe>`) is wired in #2 when a real recipe exists to route to.

## Architecture

New package `internal/setup/recipe/`. A `Recipe` is an ordered `[]Step`. The runner drives dry-run, per-step idempotency skip, confirm-before-mutate, and a summary — mirroring `setup.Run`'s plan→confirm→apply contract — and logs every transition through `internal/report`.

### Step contract

```go
type Status int
const (
    StatusPending   Status = iota // not satisfied; will mutate
    StatusSatisfied               // already done; skip
)

type Context struct {
    Ctx    context.Context
    DryRun bool
    Yes    bool
    Rep    *report.Reporter
}

type Step interface {
    Label() (action, target string) // maps to report fields
    Check(c Context) (Status, error) // read-only idempotency probe
    Apply(c Context) error           // performs the mutation
    Mutating() bool                  // false => no confirm (Verify)
}

type Recipe struct {
    Name  string
    Steps []Step
}
```

### Runner (`Run`)

`func Run(ctx context.Context, r Recipe, opts Options) error`, `Options{DryRun, Yes bool; Rep *report.Reporter}`.

Per step, in order:
1. `Check`. On error → `rep.Fail`, count as failed, continue.
2. `StatusSatisfied` → `rep.Have(action, target)`, skip.
3. `DryRun` → `rep.Do` (the reporter is in dry-run, so it emits `status=dry`), skip Apply.
4. `Mutating()` and not `Yes` → `tui.Confirm`; declined → `rep.Skip(action,target,"declined")`, continue.
5. `rep.Do` → `Apply` → `rep.OK` / `rep.Fail`.

Ends with a `rep.Info` summary (applied/satisfied/skipped/failed). Returns an error if any step failed.

### Step types (this spec)

| Step | Fields | Check (idempotent) | Apply |
|---|---|---|---|
| `InstallTools` | `Tokens string` | resolve via `setup.Resolve`+`BuildPlan`; satisfied if `ToInstall` empty | `setup.Install` each pending tool, `rep.OK` per tool |
| `WriteConfig` | `Path, Content string; Backup bool` | file exists with identical content | backup `Path`→`Path.bak-<ts>` if present, write |
| `AppendRC` | `Shellrc, Line string` | line already present | append line |
| `RunCmd` | `ActionName, TargetName, Name string; Args []string; Interactive bool; Probe func(Context) bool` | `Probe != nil && Probe(c)` | `sys.Run` / `sys.RunInteractive` |
| `RegisterMCP` | `MCPName, Transport, Target, Scope string` | `claude mcp list` contains `MCPName` | `claude mcp add` (argv per transport/scope) |
| `Verify` | `ActionName, TargetName string; Probe func(Context) error` | runs `Probe`; satisfied on nil, error otherwise | no-op (`Mutating()=false`) |

`InstallTools` reuses exported `setup` symbols (`BuiltinRegistry`, `LoadUserManifest`, `Merge`, `Resolve`, `BuildPlan`, `NewDetector`, `AvailableBackends`, `Install`) — no duplication. `WriteConfig`/`AppendRC` use `os` with `<ts>`-suffixed backups (no clobber). `RunCmd`/`RegisterMCP` shell through `internal/sys`.

## Files

**Create:**
- `internal/setup/recipe/step.go` — `Status`, `Context`, `Step`, `Recipe`
- `internal/setup/recipe/run.go` — `Run`, `Options`, summary
- `internal/setup/recipe/steps.go` — the six step structs
- `internal/setup/recipe/run_test.go` — runner: dry-run, satisfied-skip, confirm-decline, fail-continue, summary (fake steps)
- `internal/setup/recipe/steps_test.go` — `WriteConfig`/`AppendRC` idempotency on tmp files; `RegisterMCP`/`RunCmd` argv; `Verify` probe

**Reuse (no change):** `internal/setup/{registry,manifest,resolve,plan,detect,installer}.go`, `internal/sys/exec.go`, `internal/report`, `internal/tui/confirm.go`.

## Safety / contract (carried from setup v1 + #0)

- Every mutating step is dry-run-previewable and confirm-gated (unless `Yes`).
- `WriteConfig` backs up before overwrite; `AppendRC` is append-only + idempotent.
- Steps log through `internal/report`; no secret values pass through `Label`/report (secret-bearing steps arrive in #2).
- A `Check` is read-only; mutation happens only in `Apply`.

## Verification

1. `go build ./...`, `go vet ./...`, `go test ./...` green.
2. Runner unit tests prove: satisfied steps skip (`have`), dry-run emits `dry` and mutates nothing, declined confirm skips, a failed step is logged and the run continues, summary counts are correct.
3. `WriteConfig`/`AppendRC` re-run on the same input is a no-op (idempotent), and `WriteConfig` backs up an existing differing file.
