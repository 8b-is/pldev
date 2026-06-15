package arp

import (
	"strings"
	"testing"
)

func TestDefaultClonePath(t *testing.T) {
	p := DefaultClonePath()
	if !strings.HasSuffix(p, "arp-corpus") || !strings.Contains(p, "pldev") {
		t.Errorf("clone path = %q", p)
	}
}

func TestHostNoDomain(t *testing.T) {
	if h := Host(); strings.Contains(h, ".") {
		t.Errorf("host should be the short name, got %q", h)
	}
}

func TestCommitMsg(t *testing.T) {
	got := commitMsg("M3", "2026-06-15")
	if got != "arp: sync M3 2026-06-15" {
		t.Errorf("commitMsg = %q", got)
	}
}
