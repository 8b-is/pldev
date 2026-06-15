// Package arp drives the arp-corpus validity gate and sync: verify the local
// clone with vakedc (via the repo's tools/verify.py) and push batched
// host/date entries. Used by `pldev arp` and the setup-arp recipe's Verify step.
package arp

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/peterlodri-sec/pldev/internal/report"
	"github.com/peterlodri-sec/pldev/internal/sys"
)

// DefaultClonePath is where pldev keeps the arp-corpus clone.
func DefaultClonePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "state", "pldev", "arp-corpus")
}

// Host is the short hostname used to partition the corpus.
func Host() string {
	h, _ := os.Hostname()
	return strings.SplitN(h, ".", 2)[0]
}

func today() string { return time.Now().Format("2006-01-02") }

// commitMsg is the deterministic sync commit subject.
func commitMsg(host, date string) string { return "arp: sync " + host + " " + date }

// Verify runs the corpus repo's own validity gate (vakedc over every fence).
func Verify(ctx context.Context, clone string) (string, error) {
	return sys.Run(ctx, "sh", "-c", "cd "+clone+" && python3 tools/verify.py")
}

// Sync verifies the clone, then commits + pushes the batch. A clean tree is a
// no-op. Verification failure aborts before any push.
func Sync(ctx context.Context, rep *report.Reporter, clone string) error {
	rep.Do("arp-verify", clone)
	if out, err := Verify(ctx, clone); err != nil {
		rep.Fail("arp-verify", clone, err, report.KV{K: "out", V: strings.TrimSpace(out)})
		return err
	}
	rep.OK("arp-verify", clone)

	if _, err := sys.Run(ctx, "git", "-C", clone, "add", "-A"); err != nil {
		rep.Fail("arp-stage", clone, err)
		return err
	}
	status, _ := sys.Run(ctx, "git", "-C", clone, "status", "--porcelain")
	if strings.TrimSpace(status) == "" {
		rep.Have("arp-sync", "clean")
		return nil
	}
	msg := commitMsg(Host(), today())
	rep.Do("arp-sync", msg)
	if err := sys.RunInteractive(ctx, "git", "-C", clone, "commit", "-m", msg); err != nil {
		rep.Fail("arp-sync", "commit", err)
		return err
	}
	if err := sys.RunInteractive(ctx, "git", "-C", clone, "push"); err != nil {
		rep.Fail("arp-sync", "push", err)
		return err
	}
	rep.OK("arp-sync", msg)
	return nil
}
