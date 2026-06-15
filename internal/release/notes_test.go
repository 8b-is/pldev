package release

import (
	"os"
	"path/filepath"
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
