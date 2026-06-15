// Package report is pldev's single action-logging surface. Every command emits
// through a Reporter: one action = one line, logfmt-parseable, TTY-aware
// (including nested pldev invocations), and allocation-frugal on the hot path.
//
// The action log goes to the reporter's sink (os.Stderr by default) so that a
// command's real data output on os.Stdout stays pipe-clean.
package report

import (
	"io"
	"os"
	"time"

	"github.com/mattn/go-isatty"
)

// Mode controls how records are rendered.
type Mode int

const (
	// Plain is pure logfmt, no color. Used when the sink is not a TTY, when
	// NO_COLOR is set, when PLDEV_LOG=plain, or for any nested pldev.
	Plain Mode = iota
	// Pretty is logfmt with the lvl/status fields colorized. TTY only.
	Pretty
	// JSON emits one JSON object per line with the same field set.
	JSON
)

// KV is an extra key=value pair appended after the fixed leading fields.
type KV struct{ K, V string }

// Dur formats a duration as a dur= field.
func Dur(d time.Duration) KV { return KV{K: "dur", V: d.String()} }

// Reporter renders action records to a sink. Construct with New; derive
// child scopes with Sub. Safe to share within a single goroutine of work;
// not designed for concurrent use from multiple goroutines.
type Reporter struct {
	w      io.Writer
	mode   Mode
	scope  string
	dryRun bool
	now    func() time.Time
}

// New builds a Reporter writing to w under the given scope. The render mode is
// resolved once from w and the environment (PLDEV_LOG, NO_COLOR, PLDEV_NESTED).
func New(w io.Writer, scope string) *Reporter {
	return newReporter(w, scope, resolveMode(w))
}

func newReporter(w io.Writer, scope string, mode Mode) *Reporter {
	return &Reporter{w: w, mode: mode, scope: scope, now: time.Now}
}

// resolveMode picks a render mode from environment and sink capabilities.
func resolveMode(w io.Writer) Mode {
	// Nested pldev never decorates and never competes for the terminal.
	if os.Getenv("PLDEV_NESTED") != "" {
		return Plain
	}
	switch os.Getenv("PLDEV_LOG") {
	case "json":
		return JSON
	case "plain":
		return Plain
	case "pretty":
		return Pretty
	}
	if os.Getenv("NO_COLOR") != "" {
		return Plain
	}
	if f, ok := w.(*os.File); ok && isatty.IsTerminal(f.Fd()) {
		return Pretty
	}
	return Plain
}

// Sub returns a child reporter whose scope is "<parent>><child>", inheriting
// sink, mode, dry-run, and clock.
func (r *Reporter) Sub(scope string) *Reporter {
	c := *r
	c.scope = r.scope + ">" + scope
	return &c
}

// SetDryRun toggles dry-run. When on, Do downgrades to a status=dry record
// instead of announcing a real action.
func (r *Reporter) SetDryRun(v bool) { r.dryRun = v }

// Do announces the start of a (potentially long) action. In dry-run it emits
// status=dry instead.
func (r *Reporter) Do(action, target string) {
	if r.dryRun {
		r.emit(lvlInfo, action, target, statusDry, nil, nil)
		return
	}
	r.emit(lvlInfo, action, target, statusDo, nil, nil)
}

// OK records a successful action.
func (r *Reporter) OK(action, target string, kv ...KV) {
	r.emit(lvlInfo, action, target, statusOK, nil, kv)
}

// Fail records a failed action; err is rendered as a quoted msg field.
func (r *Reporter) Fail(action, target string, err error, kv ...KV) {
	r.emit(lvlError, action, target, statusFail, err, kv)
}

// Skip records an action deliberately not taken; why becomes msg.
func (r *Reporter) Skip(action, target, why string) {
	var err error
	if why != "" {
		err = skipReason(why)
	}
	r.emit(lvlInfo, action, target, statusSkip, err, nil)
}

// Have records an idempotent no-op: the desired state already holds.
func (r *Reporter) Have(action, target string) {
	r.emit(lvlInfo, action, target, statusHave, nil, nil)
}

// Info emits a free-form, scope-tagged line (no action/target).
func (r *Reporter) Info(msg string, kv ...KV) {
	r.emit(lvlInfo, "", "", statusInfo, infoMsg(msg), kv)
}

// skipReason / infoMsg wrap a plain string as an error so emit's msg path
// renders it without a separate code path.
type strErr string

func (e strErr) Error() string  { return string(e) }
func skipReason(s string) error { return strErr(s) }
func infoMsg(s string) error    { return strErr(s) }
