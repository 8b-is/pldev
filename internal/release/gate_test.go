package release

import (
	"bytes"
	"strings"
	"testing"
)

func TestGenChallengeDeterministicReader(t *testing.T) {
	// 2 bytes -> 4 hex chars uppercased.
	got, err := genChallenge(bytes.NewReader([]byte{0x7f, 0x3a}))
	if err != nil {
		t.Fatal(err)
	}
	if got != "7F3A" {
		t.Fatalf("got %q want 7F3A", got)
	}
}

func TestConfirmPublish(t *testing.T) {
	rd := func() (string, error) { return "7F3A", nil }

	const dest = "api.github.com/repos/peterlodri-sec/pldev/releases"

	// happy path: y, y, correct token
	in := strings.NewReader("y\ny\n7F3A\n")
	var out strings.Builder
	if err := confirmPublishWith(rd, in, &out, "v0.0.2", dest, []string{"dist/pldev-linux-arm64"}); err != nil {
		t.Fatalf("expected pass, got %v", err)
	}
	// gate preview must show the full asset manifest AND the network destination.
	s := out.String()
	if !strings.Contains(s, ".sig.mldsa") || !strings.Contains(s, "3 assets") {
		t.Errorf("gate preview missing full manifest:\n%s", s)
	}
	if !strings.Contains(s, "TO:") || !strings.Contains(s, dest) {
		t.Errorf("gate preview missing network destination:\n%s", s)
	}

	// wrong challenge token aborts
	in = strings.NewReader("y\ny\nDEAD\n")
	if err := confirmPublishWith(rd, in, &strings.Builder{}, "v0.0.2", dest, nil); err == nil {
		t.Error("expected abort on wrong token")
	}

	// first 'n' aborts
	in = strings.NewReader("n\n")
	if err := confirmPublishWith(rd, in, &strings.Builder{}, "v0.0.2", dest, nil); err == nil {
		t.Error("expected abort on first decline")
	}
}
