package recipe

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func ctx() Context { return Context{Ctx: context.Background()} }

func TestWriteConfigIdempotent(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "conf")
	s := WriteConfig{Path: p, Content: "hello\n"}

	if st, _ := s.Check(ctx()); st != StatusPending {
		t.Fatal("absent file should be pending")
	}
	if err := s.Apply(ctx()); err != nil {
		t.Fatal(err)
	}
	if st, _ := s.Check(ctx()); st != StatusSatisfied {
		t.Error("after write, should be satisfied")
	}
	b, _ := os.ReadFile(p)
	if string(b) != "hello\n" {
		t.Errorf("content = %q", b)
	}
}

func TestWriteConfigBacksUpDifferingFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "conf")
	os.WriteFile(p, []byte("old\n"), 0o644)
	s := WriteConfig{Path: p, Content: "new\n", Backup: true}
	if err := s.Apply(ctx()); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if string(b) != "new\n" {
		t.Errorf("content = %q", b)
	}
	entries, _ := os.ReadDir(dir)
	var baks int
	for _, e := range entries {
		if strings.Contains(e.Name(), ".bak-") {
			baks++
		}
	}
	if baks != 1 {
		t.Errorf("expected 1 backup, got %d", baks)
	}
}

func TestAppendRCIdempotent(t *testing.T) {
	dir := t.TempDir()
	rc := filepath.Join(dir, ".zshrc")
	os.WriteFile(rc, []byte("# header\n"), 0o644)
	s := AppendRC{Shellrc: rc, Line: "export CLI_CORE=1"}

	if st, _ := s.Check(ctx()); st != StatusPending {
		t.Fatal("line absent should be pending")
	}
	if err := s.Apply(ctx()); err != nil {
		t.Fatal(err)
	}
	if st, _ := s.Check(ctx()); st != StatusSatisfied {
		t.Error("after append, line should be present")
	}
	b, _ := os.ReadFile(rc)
	if strings.Count(string(b), "CLI_CORE=1") != 1 {
		t.Errorf("line should appear once: %q", b)
	}
}

func TestRegisterMCPAddArgs(t *testing.T) {
	stdio := RegisterMCP{MCPName: "ruflo", Target: "npx ruflo@latest mcp start", Scope: "user"}
	got := strings.Join(stdio.addArgs(), " ")
	want := "mcp add ruflo -s user -- npx ruflo@latest mcp start"
	if got != want {
		t.Errorf("stdio argv:\n got  %q\n want %q", got, want)
	}

	http := RegisterMCP{MCPName: "mempalace", Transport: "http", Target: "https://x/mcp", Scope: "user"}
	got = strings.Join(http.addArgs(), " ")
	want = "mcp add --transport http mempalace -s user https://x/mcp"
	if got != want {
		t.Errorf("http argv:\n got  %q\n want %q", got, want)
	}

	def := RegisterMCP{MCPName: "x", Target: "cmd"} // empty scope defaults to user
	if !strings.Contains(strings.Join(def.addArgs(), " "), "-s user") {
		t.Error("empty scope should default to user")
	}
}

func TestVerifyProbe(t *testing.T) {
	ok := Verify{ActionName: "verify", TargetName: "mcp", Probe: func(Context) error { return nil }}
	if st, err := ok.Check(ctx()); st != StatusSatisfied || err != nil {
		t.Errorf("passing probe => satisfied, got %v %v", st, err)
	}
	bad := Verify{ActionName: "verify", TargetName: "mcp", Probe: func(Context) error { return os.ErrNotExist }}
	if st, err := bad.Check(ctx()); st != StatusPending || err == nil {
		t.Errorf("failing probe => pending+err, got %v %v", st, err)
	}
	if ok.Mutating() {
		t.Error("Verify must be non-mutating")
	}
}
