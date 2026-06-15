package release

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/peterlodri-sec/pldev/internal/pqkey"
	"github.com/peterlodri-sec/pldev/internal/report"
	"github.com/peterlodri-sec/pldev/internal/verify"
)

func TestSignArtifact(t *testing.T) {
	r := &fakeRunner{outputs: map[string]string{}}
	var pqCalls []string
	pqSign = func(keyPath, binPath string) error { pqCalls = append(pqCalls, binPath); return nil }
	defer func() { pqSign = pqkey.SignFile }()

	err := signArtifact(context.Background(), r, "dist/pldev-linux-arm64",
		"/keys/ci_ed25519", "/keys/mldsa.key")
	if err != nil {
		t.Fatal(err)
	}
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
