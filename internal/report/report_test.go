package report

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func fixed() time.Time { return time.Date(2026, 6, 15, 11, 40, 2, 0, time.UTC) }

func newTest(mode Mode) (*Reporter, *bytes.Buffer) {
	var b bytes.Buffer
	r := newReporter(&b, "setup", mode)
	r.now = fixed
	return r, &b
}

func TestPlainOK(t *testing.T) {
	r, b := newTest(Plain)
	r.OK("install", "ripgrep", Dur(820*time.Millisecond))
	want := "ts=2026-06-15T11:40:02Z lvl=info scope=setup action=install target=ripgrep status=ok dur=820ms\n"
	if b.String() != want {
		t.Errorf("got  %q\nwant %q", b.String(), want)
	}
}

func TestPlainNoColorCodes(t *testing.T) {
	r, b := newTest(Plain)
	r.OK("install", "ripgrep")
	r.Fail("install", "ghostty", errors.New("boom"))
	if strings.Contains(b.String(), "\x1b[") {
		t.Errorf("plain output contains ANSI: %q", b.String())
	}
}

func TestPrettyHasColorButSameFields(t *testing.T) {
	r, b := newTest(Pretty)
	r.OK("install", "ripgrep")
	out := b.String()
	if !strings.Contains(out, "\x1b[38;5;42m") || !strings.Contains(out, ansiReset) {
		t.Errorf("pretty output missing color: %q", out)
	}
	// Strip ANSI; the logfmt fields must be identical to plain.
	stripped := stripANSI(out)
	want := "ts=2026-06-15T11:40:02Z lvl=info scope=setup action=install target=ripgrep status=ok\n"
	if stripped != want {
		t.Errorf("pretty stripped\n got  %q\n want %q", stripped, want)
	}
}

func TestFailQuotesMessage(t *testing.T) {
	r, b := newTest(Plain)
	r.Fail("provision", "stack", errors.New(`ssh died: host "x" down`))
	out := b.String()
	if !strings.Contains(out, `status=fail`) || !strings.Contains(out, `msg="ssh died: host \"x\" down"`) {
		t.Errorf("bad fail line: %q", out)
	}
	if !strings.Contains(out, "lvl=error") {
		t.Errorf("fail should be lvl=error: %q", out)
	}
}

func TestHaveSkipInfo(t *testing.T) {
	r, b := newTest(Plain)
	r.Have("install", "git")
	r.Skip("install", "docker", "already running")
	r.Info("build", KV{"v", "v1.2.3"})
	lines := strings.Split(strings.TrimRight(b.String(), "\n"), "\n")
	if !strings.Contains(lines[0], "status=have") {
		t.Errorf("have: %q", lines[0])
	}
	if !strings.Contains(lines[1], "status=skip") || !strings.Contains(lines[1], `msg="already running"`) {
		t.Errorf("skip: %q", lines[1])
	}
	// Info has no action/target keys.
	if strings.Contains(lines[2], "action=") || strings.Contains(lines[2], "target=") {
		t.Errorf("info should omit action/target: %q", lines[2])
	}
	if !strings.Contains(lines[2], `msg=build`) && !strings.Contains(lines[2], `msg="build"`) {
		t.Errorf("info missing msg: %q", lines[2])
	}
}

func TestSubScope(t *testing.T) {
	r, b := newTest(Plain)
	r.Sub("recipe").OK("step", "register-mcp")
	if !strings.Contains(b.String(), "scope=setup>recipe") {
		t.Errorf("sub scope wrong: %q", b.String())
	}
}

func TestDryRunDowngradesDo(t *testing.T) {
	r, b := newTest(Plain)
	r.SetDryRun(true)
	r.Do("write", "~/.zshrc")
	if !strings.Contains(b.String(), "status=dry") {
		t.Errorf("dry-run Do should be status=dry: %q", b.String())
	}
}

func TestJSONMode(t *testing.T) {
	r, b := newTest(JSON)
	r.OK("install", "ripgrep", Dur(5*time.Millisecond))
	want := `{"ts":"2026-06-15T11:40:02Z","lvl":"info","scope":"setup","action":"install","target":"ripgrep","status":"ok","dur":"5ms"}` + "\n"
	if b.String() != want {
		t.Errorf("json\n got  %q\n want %q", b.String(), want)
	}
}

func TestResolveModeNested(t *testing.T) {
	t.Setenv("PLDEV_NESTED", "1")
	t.Setenv("PLDEV_LOG", "pretty") // nested wins over PLDEV_LOG
	if m := resolveMode(os.Stderr); m != Plain {
		t.Errorf("nested should force Plain, got %v", m)
	}
}

func TestResolveModeEnv(t *testing.T) {
	t.Setenv("PLDEV_NESTED", "")
	t.Setenv("NO_COLOR", "")
	t.Setenv("PLDEV_LOG", "json")
	if m := resolveMode(&bytes.Buffer{}); m != JSON {
		t.Errorf("PLDEV_LOG=json => JSON, got %v", m)
	}
	t.Setenv("PLDEV_LOG", "")
	// non-*os.File sink is never a TTY => Plain
	if m := resolveMode(&bytes.Buffer{}); m != Plain {
		t.Errorf("buffer sink => Plain, got %v", m)
	}
}

// stripANSI removes CSI sequences for comparison.
func stripANSI(s string) string {
	var out strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\x1b' {
			for i < len(s) && s[i] != 'm' {
				i++
			}
			continue
		}
		out.WriteByte(s[i])
	}
	return out.String()
}
