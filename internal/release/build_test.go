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
	for _, c := range r.calls {
		if !strings.HasPrefix(c, "go build") {
			t.Errorf("unexpected call: %s", c)
		}
	}
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
