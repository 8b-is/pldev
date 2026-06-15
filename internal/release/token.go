package release

import (
	"context"
	"errors"
	"strings"
)

// resolveToken prefers `gh auth token`, then the GH_TOKEN env (getenv injected).
func resolveToken(ctx context.Context, r Runner, getenv func(string) string) (string, error) {
	if out, err := r.Capture(ctx, nil, "gh", "auth", "token"); err == nil {
		if tok := strings.TrimSpace(string(out)); tok != "" {
			return tok, nil
		}
	}
	if tok := strings.TrimSpace(getenv("GH_TOKEN")); tok != "" {
		return tok, nil
	}
	return "", errors.New("no GitHub token: run `gh auth login` or set GH_TOKEN")
}
