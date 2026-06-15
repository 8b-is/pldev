package release

import (
	"context"
	"testing"
)

func TestResolveToken(t *testing.T) {
	// gh auth token succeeds -> used.
	r := &fakeRunner{outputs: map[string]string{"gh auth": "ghtok123\n"}}
	tok, err := resolveToken(context.Background(), r, func(string) string { return "" })
	if err != nil || tok != "ghtok123" {
		t.Fatalf("gh path: tok=%q err=%v", tok, err)
	}

	// gh fails -> GH_TOKEN env fallback.
	r2 := &fakeRunner{outputs: map[string]string{}, capErr: context.Canceled}
	tok, err = resolveToken(context.Background(), r2, func(k string) string {
		if k == "GH_TOKEN" {
			return "envtok456"
		}
		return ""
	})
	if err != nil || tok != "envtok456" {
		t.Fatalf("env path: tok=%q err=%v", tok, err)
	}

	// neither -> error.
	r3 := &fakeRunner{outputs: map[string]string{}, capErr: context.Canceled}
	if _, err := resolveToken(context.Background(), r3, func(string) string { return "" }); err == nil {
		t.Error("expected error when no token available")
	}
}
