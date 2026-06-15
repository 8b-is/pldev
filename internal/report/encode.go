package report

import (
	"sync"
	"time"
)

// Level and status tokens. Kept as untyped string consts so appends are
// copy-only (no allocation).
const (
	lvlInfo  = "info"
	lvlError = "error"

	statusOK   = "ok"
	statusFail = "fail"
	statusSkip = "skip"
	statusHave = "have"
	statusDo   = "do"
	statusDry  = "dry"
	statusInfo = "info"
)

// ANSI 256-color prefixes/suffixes for Pretty mode. Constant slices -> no
// per-line allocation. Color numbers mirror internal/tui/styles.go.
const ansiReset = "\x1b[0m"

func ansiFor(token string) string {
	switch token {
	case statusOK:
		return "\x1b[38;5;42m" // green
	case statusFail, lvlError:
		return "\x1b[38;5;196m" // red
	case statusSkip, statusHave:
		return "\x1b[38;5;241m" // dim
	case statusDo:
		return "\x1b[38;5;63m" // blue
	case statusDry:
		return "\x1b[38;5;214m" // orange
	case lvlInfo:
		return "\x1b[38;5;63m" // blue
	default:
		return ""
	}
}

// bufPool recycles line buffers. Storing *[]byte (not []byte) keeps the slice
// header off the heap on Get/Put.
var bufPool = sync.Pool{New: func() any { b := make([]byte, 0, 256); return &b }}

// emit renders one record to the sink. The Plain steady-state path is
// allocation-free (see bench_test.go).
func (r *Reporter) emit(lvl, action, target, status string, err error, kv []KV) {
	bp := bufPool.Get().(*[]byte)
	b := (*bp)[:0]

	switch r.mode {
	case JSON:
		b = r.encodeJSON(b, lvl, action, target, status, err, kv)
	default:
		b = r.encodeLogfmt(b, lvl, action, target, status, err, kv, r.mode == Pretty)
	}

	_, _ = r.w.Write(b) // best-effort: logging never crashes a command
	*bp = b
	bufPool.Put(bp)
}

func (r *Reporter) encodeLogfmt(b []byte, lvl, action, target, status string, err error, kv []KV, color bool) []byte {
	b = append(b, "ts="...)
	b = r.now().UTC().AppendFormat(b, time.RFC3339)

	b = append(b, " lvl="...)
	b = appendToken(b, lvl, color)

	b = append(b, " scope="...)
	b = appendVal(b, r.scope)

	// action/target are omitted for free-form Info lines.
	if action != "" || target != "" {
		b = append(b, " action="...)
		b = appendVal(b, action)
		b = append(b, " target="...)
		b = appendVal(b, target)
	}

	b = append(b, " status="...)
	b = appendToken(b, status, color)

	for _, p := range kv {
		b = append(b, ' ')
		b = append(b, p.K...)
		b = append(b, '=')
		b = appendVal(b, p.V)
	}
	if err != nil {
		b = append(b, " msg="...)
		b = appendQuoted(b, err.Error())
	}
	return append(b, '\n')
}

func (r *Reporter) encodeJSON(b []byte, lvl, action, target, status string, err error, kv []KV) []byte {
	b = append(b, `{"ts":`...)
	b = append(b, '"')
	b = r.now().UTC().AppendFormat(b, time.RFC3339)
	b = append(b, '"')
	b = appendJSONField(b, "lvl", lvl)
	b = appendJSONField(b, "scope", r.scope)
	if action != "" || target != "" {
		b = appendJSONField(b, "action", action)
		b = appendJSONField(b, "target", target)
	}
	b = appendJSONField(b, "status", status)
	for _, p := range kv {
		b = appendJSONField(b, p.K, p.V)
	}
	if err != nil {
		b = appendJSONField(b, "msg", err.Error())
	}
	return append(b, '}', '\n')
}

// appendToken appends a fixed token, optionally wrapped in ANSI color.
func appendToken(b []byte, token string, color bool) []byte {
	if !color {
		return append(b, token...)
	}
	if c := ansiFor(token); c != "" {
		b = append(b, c...)
		b = append(b, token...)
		return append(b, ansiReset...)
	}
	return append(b, token...)
}

// appendVal appends a logfmt value, quoting only when necessary.
func appendVal(b []byte, v string) []byte {
	if needsQuote(v) {
		return appendQuoted(b, v)
	}
	return append(b, v...)
}

func needsQuote(v string) bool {
	if v == "" {
		return true
	}
	for i := 0; i < len(v); i++ {
		c := v[i]
		if c <= ' ' || c == '=' || c == '"' || c == '\\' {
			return true
		}
	}
	return false
}

func appendQuoted(b []byte, v string) []byte {
	b = append(b, '"')
	for i := 0; i < len(v); i++ {
		c := v[i]
		switch c {
		case '"', '\\':
			b = append(b, '\\', c)
		case '\n':
			b = append(b, '\\', 'n')
		case '\r':
			b = append(b, '\\', 'r')
		case '\t':
			b = append(b, '\\', 't')
		default:
			b = append(b, c)
		}
	}
	return append(b, '"')
}

func appendJSONField(b []byte, k, v string) []byte {
	b = append(b, ',', '"')
	b = append(b, k...)
	b = append(b, '"', ':')
	return appendQuoted(b, v)
}
