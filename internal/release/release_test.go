package release

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peterlodri-sec/pldev/internal/report"
)

// Note: fakeRunner is defined in build_test.go (same package).

func TestRunDryRunSkipsPublish(t *testing.T) {
	origBuild, origSign, origVerify := buildStage, signStage, verifyStage
	defer func() { buildStage, signStage, verifyStage = origBuild, origSign, origVerify }()

	buildStage = func(ctx context.Context, r Runner, ld, dist string) ([]string, error) {
		dir := t.TempDir()
		base := filepath.Join(dir, "pldev-linux-arm64")
		for _, p := range []string{base, base + ".sig", base + ".sig.mldsa"} {
			if err := os.WriteFile(p, []byte("dummy"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return []string{base}, nil
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
