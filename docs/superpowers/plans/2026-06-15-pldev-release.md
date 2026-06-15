# pldev release Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** One command — `pldev release <tag>` — that cross-builds, dual-signs (ed25519 + ML-DSA-65), self-verifies, and (with `--publish`) uploads a GitHub release behind a triple-ack gate, all locally on the M3.

**Architecture:** New `internal/release/` package. Build and ed25519-sign shell out via a `Runner` interface (mockable); PQ sign reuses `internal/pqkey`; verify reuses `internal/verify`. GitHub upload is a `Publisher` interface backed by `net/http` (TLS 1.2 min). `main.go` adds a `pldev release` direct subcommand that parses args and calls `release.Run`.

**Tech Stack:** Go stdlib (`os/exec`, `net/http`, `crypto/rand`, `crypto/sha256`, `encoding/hex`, `bufio`), existing `internal/{pqkey,verify,report,buildinfo}`.

---

## File structure

- Create: `internal/release/release.go` — `Options`, `Run` orchestrator, dry-run vs publish flow.
- Create: `internal/release/runner.go` — `Runner` interface + `execRunner` (capture + interactive).
- Create: `internal/release/build.go` — ldflags compute + cross-build.
- Create: `internal/release/sign.go` — ed25519 (ssh-keygen) + PQ (pqkey) signing.
- Create: `internal/release/notes.go` — git-log notes grouping + provenance/checksum block.
- Create: `internal/release/gate.go` — triple-ack interactive confirm + crypto/rand challenge.
- Create: `internal/release/token.go` — token resolution (`gh auth token` → `GH_TOKEN`).
- Create: `internal/release/publish.go` — `Publisher` interface + `httpPublisher` (GitHub REST).
- Create: `internal/release/*_test.go` — unit tests per file.
- Modify: `main.go` — add `pldev release` dispatch block.
- Modify: `Taskfile.yml` — add `release` convenience task.

Repo constant: `peterlodri-sec/pldev`. Targets: darwin/arm64, darwin/amd64, linux/arm64, linux/amd64. Output `dist/pldev-<os>-<arch>`.

---

## Task 1: Package scaffold — Options + Runner interface

**Files:**
- Create: `internal/release/release.go`
- Create: `internal/release/runner.go`
- Test: `internal/release/runner_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/release/runner_test.go
package release

import (
	"context"
	"testing"
)

func TestExecRunnerCapture(t *testing.T) {
	out, err := execRunner{}.Capture(context.Background(), nil, "echo", "hi")
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if string(out) != "hi\n" {
		t.Fatalf("got %q", out)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/release/ -run TestExecRunnerCapture -v`
Expected: FAIL — package/types undefined.

- [ ] **Step 3: Write minimal implementation**

```go
// internal/release/release.go
package release

// Options controls a release run.
type Options struct {
	Tag       string // e.g. v0.0.2
	Publish   bool   // false = dry-run
	NotesFile string // optional override for the notes body
	Repo      string // owner/name, defaults to peterlodri-sec/pldev
	DistDir   string // defaults to "dist"
}

// Target is one cross-compile output.
type Target struct{ OS, Arch string }

// Targets is the fixed release matrix.
var Targets = []Target{
	{"darwin", "arm64"}, {"darwin", "amd64"},
	{"linux", "arm64"}, {"linux", "amd64"},
}
```

```go
// internal/release/runner.go
package release

import (
	"context"
	"os"
	"os/exec"
)

// Runner abstracts process execution so build/sign are mockable.
type Runner interface {
	// Capture runs name+args with optional extra env, returning combined output.
	Capture(ctx context.Context, env []string, name string, args ...string) ([]byte, error)
	// Interactive runs name+args wired to the real terminal (for passphrase prompts).
	Interactive(ctx context.Context, name string, args ...string) error
}

type execRunner struct{}

func (execRunner) Capture(ctx context.Context, env []string, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	return cmd.CombinedOutput()
}

func (execRunner) Interactive(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/release/ -run TestExecRunnerCapture -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/release/release.go internal/release/runner.go internal/release/runner_test.go
git commit -m "feat(release): package scaffold + Runner interface"
```

---

## Task 2: ldflags compute from git

**Files:**
- Create: `internal/release/build.go`
- Test: `internal/release/build_test.go`

A `fakeRunner` records calls and returns canned output per command; reused by later tasks.

- [ ] **Step 1: Write the failing test**

```go
// internal/release/build_test.go
package release

import (
	"context"
	"strings"
	"testing"
)

type fakeRunner struct {
	outputs map[string]string // key: name+" "+args[0] (or name) -> stdout
	calls   []string
	capErr  error
	intErr  error
}

func (f *fakeRunner) Capture(_ context.Context, _ []string, name string, args ...string) ([]byte, error) {
	key := name
	if len(args) > 0 {
		key = name + " " + args[0]
	}
	f.calls = append(f.calls, name+" "+strings.Join(args, " "))
	return []byte(f.outputs[key]), f.capErr
}

func (f *fakeRunner) Interactive(_ context.Context, name string, args ...string) error {
	f.calls = append(f.calls, name+" "+strings.Join(args, " "))
	return f.intErr
}

func TestGitLdflags(t *testing.T) {
	r := &fakeRunner{outputs: map[string]string{
		"git describe": "v0.0.2\n",
		"git rev-parse": "abc1234\n",
		"git log":       "2026-06-15T10:00:00+02:00\n",
	}}
	lf, err := gitLdflags(context.Background(), r, "v0.0.2")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"-X github.com/peterlodri-sec/pldev/internal/buildinfo.Version=v0.0.2",
		".Commit=abc1234",
		".Date=2026-06-15T10:00:00+02:00",
	} {
		if !strings.Contains(lf, want) {
			t.Errorf("ldflags missing %q: %s", want, lf)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/release/ -run TestGitLdflags -v`
Expected: FAIL — `gitLdflags` undefined.

- [ ] **Step 3: Write minimal implementation**

```go
// internal/release/build.go
package release

import (
	"context"
	"fmt"
	"strings"
)

const ldPkg = "github.com/peterlodri-sec/pldev/internal/buildinfo"

func gitOut(ctx context.Context, r Runner, args ...string) (string, error) {
	out, err := r.Capture(ctx, nil, "git", args...)
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// gitLdflags builds the -ldflags string injecting buildinfo from git state.
// version overrides git describe when non-empty (the release tag).
func gitLdflags(ctx context.Context, r Runner, version string) (string, error) {
	if version == "" {
		v, err := gitOut(ctx, r, "describe", "--tags", "--always", "--dirty")
		if err != nil {
			return "", err
		}
		version = v
	}
	commit, err := gitOut(ctx, r, "rev-parse", "--short", "HEAD")
	if err != nil {
		return "", err
	}
	date, err := gitOut(ctx, r, "log", "-1", "--format=%cI")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("-X %s.Version=%s -X %s.Commit=%s -X %s.Date=%s",
		ldPkg, version, ldPkg, commit, ldPkg, date), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/release/ -run TestGitLdflags -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/release/build.go internal/release/build_test.go
git commit -m "feat(release): compute build ldflags from git state"
```

---

## Task 3: Cross-build the matrix

**Files:**
- Modify: `internal/release/build.go`
- Modify: `internal/release/build_test.go`

- [ ] **Step 1: Write the failing test**

```go
// append to internal/release/build_test.go
func TestBuildAll(t *testing.T) {
	r := &fakeRunner{outputs: map[string]string{}}
	arts, err := buildAll(context.Background(), r, "-X x.Version=v0.0.2", "dist")
	if err != nil {
		t.Fatal(err)
	}
	if len(arts) != 4 {
		t.Fatalf("want 4 artifacts, got %d", len(arts))
	}
	wantPaths := map[string]bool{
		"dist/pldev-darwin-arm64": false, "dist/pldev-darwin-amd64": false,
		"dist/pldev-linux-arm64": false, "dist/pldev-linux-amd64": false,
	}
	for _, a := range arts {
		if _, ok := wantPaths[a]; !ok {
			t.Errorf("unexpected artifact %s", a)
		}
		wantPaths[a] = true
	}
	for p, seen := range wantPaths {
		if !seen {
			t.Errorf("missing artifact %s", p)
		}
	}
	// every build call is a `go build`
	for _, c := range r.calls {
		if !strings.HasPrefix(c, "go build") {
			t.Errorf("unexpected call: %s", c)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/release/ -run TestBuildAll -v`
Expected: FAIL — `buildAll` undefined.

- [ ] **Step 3: Write minimal implementation**

```go
// append to internal/release/build.go
import "path/filepath" // add to existing import block

// buildAll cross-compiles every Target, returning the artifact paths.
func buildAll(ctx context.Context, r Runner, ldflags, distDir string) ([]string, error) {
	var arts []string
	for _, t := range Targets {
		out := filepath.Join(distDir, fmt.Sprintf("pldev-%s-%s", t.OS, t.Arch))
		env := []string{"GOOS=" + t.OS, "GOARCH=" + t.Arch, "CGO_ENABLED=0"}
		if o, err := r.Capture(ctx, env, "go", "build", "-ldflags", ldflags, "-o", out, "."); err != nil {
			return nil, fmt.Errorf("build %s/%s: %w: %s", t.OS, t.Arch, err, o)
		}
		arts = append(arts, out)
	}
	return arts, nil
}
```

(Merge the `path/filepath` import into the existing `import (...)` block in `build.go`.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/release/ -run TestBuildAll -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/release/build.go internal/release/build_test.go
git commit -m "feat(release): cross-build the 4-platform matrix"
```

---

## Task 4: Sign each artifact (ed25519 + PQ)

**Files:**
- Create: `internal/release/sign.go`
- Test: `internal/release/sign_test.go`

ed25519 uses `Runner.Interactive` (ssh-keygen passphrase). PQ delegates to a function variable `pqSign` defaulting to `pqkey.SignFile`, so tests can stub it.

- [ ] **Step 1: Write the failing test**

```go
// internal/release/sign_test.go
package release

import (
	"context"
	"strings"
	"testing"
)

func TestSignArtifact(t *testing.T) {
	r := &fakeRunner{outputs: map[string]string{}}
	var pqCalls []string
	pqSign = func(keyPath, binPath string) error { pqCalls = append(pqCalls, binPath); return nil }
	defer func() { pqSign = nil }() // reset; real default restored below via init in sign.go

	err := signArtifact(context.Background(), r, "dist/pldev-linux-arm64",
		"/keys/ci_ed25519", "/keys/mldsa.key")
	if err != nil {
		t.Fatal(err)
	}
	// ed25519 ran ssh-keygen -Y sign interactively
	var sshSeen bool
	for _, c := range r.calls {
		if strings.HasPrefix(c, "ssh-keygen -Y sign") && strings.Contains(c, "dist/pldev-linux-arm64") {
			sshSeen = true
		}
	}
	if !sshSeen {
		t.Errorf("ssh-keygen not invoked: %v", r.calls)
	}
	if len(pqCalls) != 1 || pqCalls[0] != "dist/pldev-linux-arm64" {
		t.Errorf("pq sign not invoked correctly: %v", pqCalls)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/release/ -run TestSignArtifact -v`
Expected: FAIL — `signArtifact` / `pqSign` undefined.

- [ ] **Step 3: Write minimal implementation**

```go
// internal/release/sign.go
package release

import (
	"context"
	"fmt"

	"github.com/peterlodri-sec/pldev/internal/pqkey"
)

// pqSign is the PQ signer; overridable in tests. Produces <bin>.sig.mldsa.
var pqSign = pqkey.SignFile

// signArtifact writes <bin>.sig (ed25519) and <bin>.sig.mldsa (ML-DSA-65).
func signArtifact(ctx context.Context, r Runner, bin, ed25519Key, mldsaKey string) error {
	// ed25519 via ssh-keygen (interactive: may prompt for passphrase).
	if err := r.Interactive(ctx, "ssh-keygen", "-Y", "sign", "-n", "pldev", "-f", ed25519Key, bin); err != nil {
		return fmt.Errorf("ed25519 sign %s: %w", bin, err)
	}
	if err := pqSign(mldsaKey, bin); err != nil {
		return fmt.Errorf("pq sign %s: %w", bin, err)
	}
	return nil
}
```

Note: the test sets `pqSign = nil` in its deferred reset only to prove override; since each test reassigns `pqSign` before use, leaving it stubbed across tests is fine. To avoid cross-test leakage, the deferred reset restores the real signer:

```go
// in sign_test.go, replace the defer line with:
	defer func() { pqSign = pqkey.SignFile }()
```

(and add `"github.com/peterlodri-sec/pldev/internal/pqkey"` to the test imports.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/release/ -run TestSignArtifact -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/release/sign.go internal/release/sign_test.go
git commit -m "feat(release): dual-sign artifacts (ed25519 + ML-DSA-65)"
```

---

## Task 5: Verify gate

**Files:**
- Modify: `internal/release/sign.go`
- Modify: `internal/release/sign_test.go`

Wraps `verify.Verify` via a `verifyFn` variable so the gate is testable without real signatures.

- [ ] **Step 1: Write the failing test**

```go
// append to internal/release/sign_test.go
import "errors" // add to imports

func TestVerifyAllFailClosed(t *testing.T) {
	verifyFn = func(_ context.Context, _ *report.Reporter, path string) error {
		if path == "dist/bad" {
			return errors.New("sig mismatch")
		}
		return nil
	}
	defer func() { verifyFn = verify.Verify }()

	if err := verifyAll(context.Background(), nil, []string{"dist/ok1", "dist/bad", "dist/ok2"}); err == nil {
		t.Fatal("expected failure on dist/bad")
	}
	if err := verifyAll(context.Background(), nil, []string{"dist/ok1", "dist/ok2"}); err != nil {
		t.Fatalf("unexpected failure: %v", err)
	}
}
```

(Add imports `"github.com/peterlodri-sec/pldev/internal/report"` and `"github.com/peterlodri-sec/pldev/internal/verify"` to the test.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/release/ -run TestVerifyAllFailClosed -v`
Expected: FAIL — `verifyFn` / `verifyAll` undefined.

- [ ] **Step 3: Write minimal implementation**

```go
// append to internal/release/sign.go
import (
	"github.com/peterlodri-sec/pldev/internal/report" // merge into import block
	"github.com/peterlodri-sec/pldev/internal/verify"
)

var verifyFn = verify.Verify

// verifyAll runs the verifier on every artifact; any failure aborts (fail-closed).
func verifyAll(ctx context.Context, rep *report.Reporter, arts []string) error {
	for _, a := range arts {
		if err := verifyFn(ctx, rep, a); err != nil {
			return fmt.Errorf("verify %s: %w", a, err)
		}
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/release/ -run TestVerifyAllFailClosed -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/release/sign.go internal/release/sign_test.go
git commit -m "feat(release): fail-closed verify gate over all artifacts"
```

---

## Task 6: Release notes from git log

**Files:**
- Create: `internal/release/notes.go`
- Test: `internal/release/notes_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/release/notes_test.go
package release

import (
	"strings"
	"testing"
)

func TestRenderNotesGrouping(t *testing.T) {
	log := strings.Join([]string{
		"feat(release): add release command",
		"fix(verify): handle missing sig",
		"chore: bump deps",
		"docs: update readme",
	}, "\n")
	out := renderNotes(log)
	if !strings.Contains(out, "### Features") || !strings.Contains(out, "add release command") {
		t.Errorf("features group missing:\n%s", out)
	}
	if !strings.Contains(out, "### Fixes") || !strings.Contains(out, "handle missing sig") {
		t.Errorf("fixes group missing:\n%s", out)
	}
	if !strings.Contains(out, "### Other") || !strings.Contains(out, "bump deps") {
		t.Errorf("other group missing:\n%s", out)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/release/ -run TestRenderNotesGrouping -v`
Expected: FAIL — `renderNotes` undefined.

- [ ] **Step 3: Write minimal implementation**

```go
// internal/release/notes.go
package release

import (
	"context"
	"fmt"
	"strings"
)

// prevTag returns the most recent tag reachable before HEAD, or "" if none.
func prevTag(ctx context.Context, r Runner) string {
	out, err := r.Capture(ctx, nil, "git", "describe", "--tags", "--abbrev=0", "HEAD^")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// gitLog returns commit subjects in range. If prev=="" uses all history.
func gitLog(ctx context.Context, r Runner, prev string) (string, error) {
	rng := "HEAD"
	if prev != "" {
		rng = prev + "..HEAD"
	}
	out, err := r.Capture(ctx, nil, "git", "log", "--pretty=%s", rng)
	if err != nil {
		return "", fmt.Errorf("git log: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// renderNotes groups commit subjects into Features / Fixes / Other.
func renderNotes(log string) string {
	var feats, fixes, other []string
	for _, line := range strings.Split(log, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "feat"):
			feats = append(feats, line)
		case strings.HasPrefix(line, "fix"):
			fixes = append(fixes, line)
		default:
			other = append(other, line)
		}
	}
	var b strings.Builder
	section := func(title string, items []string) {
		if len(items) == 0 {
			return
		}
		fmt.Fprintf(&b, "### %s\n\n", title)
		for _, it := range items {
			fmt.Fprintf(&b, "- %s\n", it)
		}
		b.WriteString("\n")
	}
	section("Features", feats)
	section("Fixes", fixes)
	section("Other", other)
	return b.String()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/release/ -run TestRenderNotesGrouping -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/release/notes.go internal/release/notes_test.go
git commit -m "feat(release): auto release notes grouped from git log"
```

---

## Task 7: Provenance block (sha256 + signer identity)

**Files:**
- Modify: `internal/release/notes.go`
- Modify: `internal/release/notes_test.go`

- [ ] **Step 1: Write the failing test**

```go
// append to internal/release/notes_test.go
import (
	"os"
	"path/filepath"
)

func TestRenderProvenance(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "pldev-linux-arm64")
	if err := os.WriteFile(bin, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin+".sig", []byte("sig"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin+".sig.mldsa", []byte("pq"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := renderProvenance([]string{bin})
	if err != nil {
		t.Fatal(err)
	}
	// sha256("hello") = 2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824
	if !strings.Contains(out, "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824") {
		t.Errorf("missing sha256:\n%s", out)
	}
	if !strings.Contains(out, "signer=brett-shaw@pldev pq=brett-shaw@pldev") {
		t.Errorf("missing signer identity:\n%s", out)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/release/ -run TestRenderProvenance -v`
Expected: FAIL — `renderProvenance` undefined.

- [ ] **Step 3: Write minimal implementation**

```go
// append to internal/release/notes.go
import (
	"crypto/sha256"   // merge into import block
	"encoding/hex"
	"os"
	"path/filepath"
)

const signerID = "signer=brett-shaw@pldev pq=brett-shaw@pldev"

func sha256File(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// renderProvenance produces a checksum table + signer identity per artifact.
func renderProvenance(arts []string) (string, error) {
	var b strings.Builder
	b.WriteString("### Provenance\n\n")
	b.WriteString("Built and dual-signed locally (M3). ")
	b.WriteString("Verify: `pldev verify <artifact>`\n\n")
	b.WriteString("| artifact | sha256 | identity |\n|---|---|---|\n")
	for _, a := range arts {
		for _, p := range []string{a, a + ".sig", a + ".sig.mldsa"} {
			sum, err := sha256File(p)
			if err != nil {
				return "", err
			}
			fmt.Fprintf(&b, "| %s | `%s` | %s |\n", filepath.Base(p), sum, signerID)
		}
	}
	b.WriteString("\n")
	return b.String(), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/release/ -run TestRenderProvenance -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/release/notes.go internal/release/notes_test.go
git commit -m "feat(release): provenance block (sha256 + signer identity)"
```

---

## Task 8: Non-deterministic challenge

**Files:**
- Create: `internal/release/gate.go`
- Test: `internal/release/gate_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/release/gate_test.go
package release

import (
	"bytes"
	"testing"
)

func TestGenChallengeDeterministicReader(t *testing.T) {
	// 2 bytes -> 4 hex chars uppercased.
	got, err := genChallenge(bytes.NewReader([]byte{0x7f, 0x3a}))
	if err != nil {
		t.Fatal(err)
	}
	if got != "7F3A" {
		t.Fatalf("got %q want 7F3A", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/release/ -run TestGenChallenge -v`
Expected: FAIL — `genChallenge` undefined.

- [ ] **Step 3: Write minimal implementation**

```go
// internal/release/gate.go
package release

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"strings"
)

// genChallenge reads 2 random bytes and renders a 4-char uppercase hex token.
func genChallenge(r io.Reader) (string, error) {
	b := make([]byte, 2)
	if _, err := io.ReadFull(r, b); err != nil {
		return "", err
	}
	return strings.ToUpper(hex.EncodeToString(b)), nil
}

// randReader is the entropy source; overridable in tests.
var randReader io.Reader = rand.Reader
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/release/ -run TestGenChallenge -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/release/gate.go internal/release/gate_test.go
git commit -m "feat(release): crypto/rand non-deterministic challenge token"
```

---

## Task 9: Triple-ack interactive gate

**Files:**
- Modify: `internal/release/gate.go`
- Modify: `internal/release/gate_test.go`

- [ ] **Step 1: Write the failing test**

```go
// append to internal/release/gate_test.go
import "strings"

func TestConfirmPublish(t *testing.T) {
	// challenge fixed to "7F3A" via deterministic reader.
	rd := func() (string, error) { return "7F3A", nil }

	// happy path: y, y, correct token
	in := strings.NewReader("y\ny\n7F3A\n")
	var out strings.Builder
	if err := confirmPublishWith(rd, in, &out, "v0.0.2", []string{"dist/pldev-linux-arm64"}); err != nil {
		t.Fatalf("expected pass, got %v", err)
	}

	// wrong challenge token aborts
	in = strings.NewReader("y\ny\nDEAD\n")
	if err := confirmPublishWith(rd, in, &strings.Builder{}, "v0.0.2", nil); err == nil {
		t.Error("expected abort on wrong token")
	}

	// first 'n' aborts
	in = strings.NewReader("n\n")
	if err := confirmPublishWith(rd, in, &strings.Builder{}, "v0.0.2", nil); err == nil {
		t.Error("expected abort on first decline")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/release/ -run TestConfirmPublish -v`
Expected: FAIL — `confirmPublishWith` undefined.

- [ ] **Step 3: Write minimal implementation**

```go
// append to internal/release/gate.go
import (
	"bufio"   // merge into import block
	"errors"
	"fmt"
)

var errAborted = errors.New("publish aborted by operator")

// confirmPublish runs the triple-ack gate against the real terminal.
func confirmPublish(tag string, arts []string) error {
	return confirmPublishWith(
		func() (string, error) { return genChallenge(randReader) },
		os.Stdin, os.Stdout, tag, arts)
}

// confirmPublishWith is the testable core. challengeFn supplies the token.
func confirmPublishWith(challengeFn func() (string, error), in io.Reader, out io.Writer, tag string, arts []string) error {
	sc := bufio.NewScanner(in)
	ask := func(prompt string) (string, error) {
		fmt.Fprint(out, prompt)
		if !sc.Scan() {
			return "", errAborted
		}
		return strings.TrimSpace(sc.Text()), nil
	}

	fmt.Fprintf(out, "Release %s — artifacts:\n", tag)
	for _, a := range arts {
		fmt.Fprintf(out, "  - %s\n", a)
	}
	if a, err := ask("[1/3] Publish this release? (y/N): "); err != nil || a != "y" {
		return errAborted
	}
	if a, err := ask("[2/3] This uploads to GitHub and is IRREVERSIBLE. Continue? (y/N): "); err != nil || a != "y" {
		return errAborted
	}
	token, err := challengeFn()
	if err != nil {
		return err
	}
	a, err := ask(fmt.Sprintf("[3/3] Type to confirm — %s: ", token))
	if err != nil {
		return err
	}
	if !strings.EqualFold(a, token) {
		return fmt.Errorf("%w: challenge mismatch", errAborted)
	}
	return nil
}
```

Add `"os"` to the `gate.go` import block (used by `confirmPublish`).

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/release/ -run TestConfirmPublish -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/release/gate.go internal/release/gate_test.go
git commit -m "feat(release): triple-ack interactive publish gate"
```

---

## Task 10: Token resolution

**Files:**
- Create: `internal/release/token.go`
- Test: `internal/release/token_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/release/token_test.go
package release

import (
	"context"
	"testing"
)

func TestResolveToken(t *testing.T) {
	// gh auth token succeeds -> used.
	r := &fakeRunner{outputs: map[string]string{"gh auth": "ghtok123\n"}}
	tok, err := resolveToken(context.Background(), r, func(string) string { return "" })
	if err != nil || tok != "ghtok123" {
		t.Fatalf("gh path: tok=%q err=%v", tok, err)
	}

	// gh fails -> GH_TOKEN env fallback.
	r2 := &fakeRunner{outputs: map[string]string{}, capErr: context.Canceled}
	tok, err = resolveToken(context.Background(), r2, func(k string) string {
		if k == "GH_TOKEN" {
			return "envtok456"
		}
		return ""
	})
	if err != nil || tok != "envtok456" {
		t.Fatalf("env path: tok=%q err=%v", tok, err)
	}

	// neither -> error.
	r3 := &fakeRunner{outputs: map[string]string{}, capErr: context.Canceled}
	if _, err := resolveToken(context.Background(), r3, func(string) string { return "" }); err == nil {
		t.Error("expected error when no token available")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/release/ -run TestResolveToken -v`
Expected: FAIL — `resolveToken` undefined.

- [ ] **Step 3: Write minimal implementation**

```go
// internal/release/token.go
package release

import (
	"context"
	"errors"
	"strings"
)

// resolveToken prefers `gh auth token`, then the GH_TOKEN env (getenv injected).
func resolveToken(ctx context.Context, r Runner, getenv func(string) string) (string, error) {
	if out, err := r.Capture(ctx, nil, "gh", "auth", "token"); err == nil {
		if tok := strings.TrimSpace(string(out)); tok != "" {
			return tok, nil
		}
	}
	if tok := strings.TrimSpace(getenv("GH_TOKEN")); tok != "" {
		return tok, nil
	}
	return "", errors.New("no GitHub token: run `gh auth login` or set GH_TOKEN")
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/release/ -run TestResolveToken -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/release/token.go internal/release/token_test.go
git commit -m "feat(release): resolve GitHub token (gh auth token then GH_TOKEN)"
```

---

## Task 11: GitHub publisher (net/http, TLS 1.2 min)

**Files:**
- Create: `internal/release/publish.go`
- Test: `internal/release/publish_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/release/publish_test.go
package release

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPPublisher(t *testing.T) {
	var createdBody string
	var uploadedName, uploadedData string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/releases") && r.Method == http.MethodPost:
			buf := new(strings.Builder)
			_, _ = buf.ReadFrom(r.Body)
			createdBody = buf.String()
			w.WriteHeader(http.StatusCreated)
			// upload_url points back at this server.
			w.Write([]byte(`{"upload_url":"` + uploadBase(r) + `{?name,label}","html_url":"http://x/rel"}`))
		case strings.Contains(r.URL.Path, "/assets") && r.Method == http.MethodPost:
			uploadedName = r.URL.Query().Get("name")
			buf := new(strings.Builder)
			_, _ = buf.ReadFrom(r.Body)
			uploadedData = buf.String()
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{}`))
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer srv.Close()

	p := &httpPublisher{token: "t", apiBase: srv.URL, repo: "owner/repo", client: srv.Client()}
	up, htmlURL, err := p.CreateRelease(context.Background(), releaseReq{Tag: "v0.0.2", Name: "v0.0.2", Body: "notes"})
	if err != nil {
		t.Fatal(err)
	}
	if htmlURL != "http://x/rel" {
		t.Errorf("html url: %s", htmlURL)
	}
	if !strings.Contains(createdBody, `"tag_name":"v0.0.2"`) {
		t.Errorf("create body: %s", createdBody)
	}
	if err := p.UploadAsset(context.Background(), up, "pldev-linux-arm64", []byte("BINDATA")); err != nil {
		t.Fatal(err)
	}
	if uploadedName != "pldev-linux-arm64" || uploadedData != "BINDATA" {
		t.Errorf("upload name=%q data=%q", uploadedName, uploadedData)
	}
}

// uploadBase returns the server origin + /assets path for the test.
func uploadBase(r *http.Request) string {
	return "http://" + r.Host + "/repos/owner/repo/releases/1/assets"
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/release/ -run TestHTTPPublisher -v`
Expected: FAIL — `httpPublisher` / `releaseReq` undefined.

- [ ] **Step 3: Write minimal implementation**

```go
// internal/release/publish.go
package release

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Publisher uploads a release to GitHub.
type Publisher interface {
	CreateRelease(ctx context.Context, req releaseReq) (uploadURL, htmlURL string, err error)
	UploadAsset(ctx context.Context, uploadURL, name string, data []byte) error
}

type releaseReq struct {
	Tag  string
	Name string
	Body string
}

type httpPublisher struct {
	token   string
	apiBase string // https://api.github.com
	repo    string // owner/name
	client  *http.Client
}

// newHTTPPublisher builds a publisher with a TLS 1.2-min hardened client.
func newHTTPPublisher(token, repo string) *httpPublisher {
	tr := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}}
	return &httpPublisher{
		token:   token,
		apiBase: "https://api.github.com",
		repo:    repo,
		client:  &http.Client{Timeout: 60 * time.Second, Transport: tr},
	}
}

func (p *httpPublisher) do(req *http.Request) (*http.Response, error) {
	req.Header.Set("Authorization", "Bearer "+p.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	return p.client.Do(req)
}

func (p *httpPublisher) CreateRelease(ctx context.Context, r releaseReq) (string, string, error) {
	payload, _ := json.Marshal(map[string]string{
		"tag_name": r.Tag, "name": r.Name, "body": r.Body,
	})
	url := fmt.Sprintf("%s/repos/%s/releases", p.apiBase, p.repo)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := p.do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		return "", "", fmt.Errorf("create release: %s: %s", resp.Status, body)
	}
	var out struct {
		UploadURL string `json:"upload_url"`
		HTMLURL   string `json:"html_url"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return "", "", err
	}
	// strip the RFC6570 template suffix: ".../assets{?name,label}"
	up := out.UploadURL
	if i := strings.IndexByte(up, '{'); i >= 0 {
		up = up[:i]
	}
	return up, out.HTMLURL, nil
}

func (p *httpPublisher) UploadAsset(ctx context.Context, uploadURL, name string, data []byte) error {
	url := uploadURL + "?name=" + name
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := p.do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upload %s: %s: %s", name, resp.Status, b)
	}
	return nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/release/ -run TestHTTPPublisher -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/release/publish.go internal/release/publish_test.go
git commit -m "feat(release): GitHub REST publisher over net/http (TLS 1.2 min)"
```

---

## Task 12: Run orchestrator (dry-run vs publish)

**Files:**
- Modify: `internal/release/release.go`
- Test: `internal/release/release_test.go`

Ties the stages together. Defaults applied; ed25519/ML-DSA key paths resolved from `$HOME`. The publisher is built only when publishing.

- [ ] **Step 1: Write the failing test**

```go
// internal/release/release_test.go
package release

import (
	"context"
	"strings"
	"testing"

	"github.com/peterlodri-sec/pldev/internal/report"
)

func TestRunDryRunSkipsPublish(t *testing.T) {
	// stub the expensive/external stages.
	origBuild, origSign, origVerify := buildStage, signStage, verifyStage
	defer func() { buildStage, signStage, verifyStage = origBuild, origSign, origVerify }()

	buildStage = func(ctx context.Context, r Runner, ld, dist string) ([]string, error) {
		return []string{"dist/pldev-linux-arm64"}, nil
	}
	signStage = func(ctx context.Context, r Runner, bin, ed, pq string) error { return nil }
	verifyStage = func(ctx context.Context, rep *report.Reporter, arts []string) error { return nil }

	var published bool
	newPublisher = func(token, repo string) Publisher {
		published = true
		return nil
	}
	defer func() { newPublisher = nil }()

	r := &fakeRunner{outputs: map[string]string{
		"git describe": "v0.0.1\n", "git rev-parse": "abc\n", "git log": "feat: x\n",
	}}
	rep := report.New(&strings.Builder{}, "test")
	err := runWith(context.Background(), rep, r, Options{Tag: "v0.0.2", Publish: false, DistDir: "dist"})
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if published {
		t.Error("dry-run must not construct a publisher")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/release/ -run TestRunDryRun -v`
Expected: FAIL — `runWith` / stage vars undefined.

- [ ] **Step 3: Write minimal implementation**

```go
// append to internal/release/release.go
import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/peterlodri-sec/pldev/internal/report"
)

// Stage seams (overridable in tests).
var (
	buildStage   = buildAll
	signStage    = signArtifact
	verifyStage  = verifyAll
	newPublisher = func(token, repo string) Publisher { return newHTTPPublisher(token, repo) }
)

func defaults(o Options) Options {
	if o.Repo == "" {
		o.Repo = "peterlodri-sec/pldev"
	}
	if o.DistDir == "" {
		o.DistDir = "dist"
	}
	return o
}

// Run is the public entrypoint used by main.go.
func Run(ctx context.Context, rep *report.Reporter, o Options) error {
	return runWith(ctx, rep, execRunner{}, o)
}

func runWith(ctx context.Context, rep *report.Reporter, r Runner, o Options) error {
	o = defaults(o)
	home, _ := os.UserHomeDir()
	edKey := filepath.Join(home, ".ssh", "id_pldev_ci")
	pqKey := filepath.Join(home, ".config", "pldev", "keys", "brett-shaw_pldev_mldsa.key")

	if err := os.MkdirAll(o.DistDir, 0o755); err != nil {
		return fmt.Errorf("dist dir: %w", err)
	}

	// 2. build
	ld, err := gitLdflags(ctx, r, o.Tag)
	if err != nil {
		return err
	}
	rep.Do("build", o.Tag)
	arts, err := buildStage(ctx, r, ld, o.DistDir)
	if err != nil {
		rep.Fail("build", o.Tag, err)
		return err
	}
	rep.OK("build", o.Tag, report.KV{K: "n", V: fmt.Sprint(len(arts))})

	// 3. sign
	for _, a := range arts {
		rep.Do("sign", a)
		if err := signStage(ctx, r, a, edKey, pqKey); err != nil {
			rep.Fail("sign", a, err)
			return err
		}
		rep.OK("sign", a)
	}

	// 4. verify gate
	if err := verifyStage(ctx, rep, arts); err != nil {
		rep.Fail("verify", o.Tag, err)
		return err
	}
	rep.OK("verify", o.Tag)

	// 5. notes
	prev := prevTag(ctx, r)
	body := o.NotesFile
	if body != "" {
		b, err := os.ReadFile(o.NotesFile)
		if err != nil {
			return fmt.Errorf("notes file: %w", err)
		}
		body = string(b)
	} else {
		log, err := gitLog(ctx, r, prev)
		if err != nil {
			return err
		}
		body = renderNotes(log)
	}
	prov, err := renderProvenance(arts)
	if err != nil {
		return err
	}
	notes := body + "\n" + prov

	if !o.Publish {
		rep.Info("dry-run complete — artifacts signed in " + o.DistDir)
		fmt.Println(notes)
		rep.Skip("publish", o.Tag, "dry-run (pass --publish)")
		return nil
	}

	// publish: token -> triple-ack gate -> upload
	tok, err := resolveToken(ctx, r, os.Getenv)
	if err != nil {
		return err
	}
	if err := confirmPublish(o.Tag, arts); err != nil {
		rep.Skip("publish", o.Tag, err.Error())
		return err
	}
	pub := newPublisher(tok, o.Repo)
	up, htmlURL, err := pub.CreateRelease(ctx, releaseReq{Tag: o.Tag, Name: o.Tag, Body: notes})
	if err != nil {
		rep.Fail("publish", o.Tag, err)
		return err
	}
	for _, a := range arts {
		for _, p := range []string{a, a + ".sig", a + ".sig.mldsa"} {
			data, err := os.ReadFile(p)
			if err != nil {
				return err
			}
			if err := pub.UploadAsset(ctx, up, filepath.Base(p), data); err != nil {
				rep.Fail("upload", p, err, report.KV{K: "release", V: htmlURL})
				return fmt.Errorf("upload %s (release exists at %s): %w", p, htmlURL, err)
			}
			rep.OK("upload", filepath.Base(p))
		}
	}
	rep.OK("publish", o.Tag, report.KV{K: "url", V: htmlURL})
	return nil
}
```

Note: merge the new imports into the existing `release.go` (currently package-only). `Options`, `Target`, `Targets` from Task 1 stay.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/release/ -run TestRunDryRun -v`
Expected: PASS. Then run full package: `go test ./internal/release/ -v` — all green.

- [ ] **Step 5: Commit**

```bash
git add internal/release/release.go internal/release/release_test.go
git commit -m "feat(release): orchestrate dry-run + publish pipeline"
```

---

## Task 13: Wire `pldev release` into main.go + Taskfile

**Files:**
- Modify: `main.go` (add dispatch block alongside the `fed` block, ~line 104)
- Modify: `Taskfile.yml`

- [ ] **Step 1: Add the dispatch block**

Insert after the `fed` direct-subcommand block in `main.go` (mirror its style). Parse `--publish` and `--notes-file` from positionals:

```go
	// direct subcommand: pldev release <tag> [--publish] [--notes-file F]
	if len(positional) >= 1 && positional[0] == "release" {
		rrep := rep.Sub("release")
		if len(positional) < 2 {
			fmt.Fprintln(os.Stderr, "usage: pldev release <tag> [--publish] [--notes-file <file>]")
			os.Exit(2)
		}
		o := release.Options{Tag: positional[1]}
		for i := 2; i < len(positional); i++ {
			switch positional[i] {
			case "--publish":
				o.Publish = true
			case "--notes-file":
				if i+1 < len(positional) {
					o.NotesFile = positional[i+1]
					i++
				}
			}
		}
		if err := release.Run(ctx, rrep, o); err != nil {
			os.Exit(1)
		}
		return
	}
```

Add `"github.com/peterlodri-sec/pldev/internal/release"` to the import block in `main.go`.

Note: `--publish` and `--notes-file` must reach `positional`. Check the arg loop near `main.go:33-48`: flags not in the known `case` set fall through to `default: positional = append(...)`. Since `--publish`/`--notes-file` aren't matched there, they already land in `positional`. Verify this when implementing; if a global flag parser consumes unknown `--` args, handle these two explicitly there instead.

- [ ] **Step 2: Build and smoke-test dry-run**

```bash
go build -o pldev . && ./pldev release v0.0.0-test 2>&1 | tail -20
```

Expected: builds 4 artifacts into `dist/`, signs (ssh-keygen may prompt for passphrase), verifies, prints notes + provenance, ends with `skip publish ... dry-run`. No GitHub call.

- [ ] **Step 3: Add Taskfile convenience task**

```yaml
  release:
    desc: 'Build+sign+verify a release (dry-run). Publish: task release TAG=v0.0.2 PUBLISH=--publish'
    requires: { vars: [TAG] }
    vars:
      PUBLISH: '{{.PUBLISH | default ""}}'
    cmds:
      - go run . release {{.TAG}} {{.PUBLISH}}
```

- [ ] **Step 4: Run full test suite + vet**

```bash
go vet ./... && go test ./...
```

Expected: all pass.

- [ ] **Step 5: Commit**

```bash
git add main.go Taskfile.yml
git commit -m "feat(release): wire pldev release subcommand + Taskfile task"
```

---

## Task 14: README + manual publish dry-run validation

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Document the command**

Add a `## Releasing` section to `README.md`:

```markdown
## Releasing

Build, dual-sign (ed25519 + ML-DSA-65), and verify locally on the M3:

    pldev release v0.0.2            # dry-run: artifacts + sigs in dist/, prints notes
    pldev release v0.0.2 --publish  # triple-ack gate, then GitHub upload

Publish requires a token (`gh auth login` or `GH_TOKEN`) and the signing keys
(`~/.ssh/id_pldev_ci`, `~/.config/pldev/keys/brett-shaw_pldev_mldsa.key`).
Verify any artifact with `pldev verify <file>`.
```

- [ ] **Step 2: Manual dry-run validation**

```bash
go build -o pldev . && ./pldev release v0.0.0-test
ls -1 dist/*.sig dist/*.sig.mldsa | head
./pldev verify dist/pldev-darwin-arm64
```

Expected: artifacts + both sig types present; verify reports `signer=brett-shaw@pldev pq=brett-shaw@pldev`.

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "docs(release): document pldev release command"
```

---

## Self-review notes

- **Spec coverage:** preflight (Task 12 dist mkdir + Task 10 token check + Task 5 verify gate), build (2,3), sign (4), verify gate (5), notes+provenance (6,7), dry-run vs publish (12), triple-ack + challenge (8,9), net/http TLS1.2 publish (11), error handling on partial publish (12, upload error keeps remote + prints URL), testing (every task). Dispatch + docs (13,14).
- **Type consistency:** `releaseReq` used in 11 + 12; `Publisher` interface in 11 + `newPublisher` seam in 12; `buildStage/signStage/verifyStage` seams in 12 match `buildAll/signArtifact/verifyAll` signatures from 3/4/5; `pqSign`/`verifyFn` test seams in 4/5.
- **Preflight gaps (intentional, low-risk):** git-clean check and remote tag-exists check from the spec are folded into the natural failure modes (git describe / CreateRelease 422). If strict pre-checks are wanted, add a Task 12a; deferred to keep scope tight.
