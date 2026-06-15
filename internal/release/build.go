package release

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
)

const ldPkg = "github.com/peterlodri-sec/pldev/internal/buildinfo"

func gitOut(ctx context.Context, r Runner, args ...string) (string, error) {
	out, err := r.Capture(ctx, nil, "git", args...)
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// gitLdflags builds the -ldflags string injecting buildinfo from git state.
// version overrides git describe when non-empty (the release tag).
func gitLdflags(ctx context.Context, r Runner, version string) (string, error) {
	if version == "" {
		v, err := gitOut(ctx, r, "describe", "--tags", "--always", "--dirty")
		if err != nil {
			return "", err
		}
		version = v
	}
	commit, err := gitOut(ctx, r, "rev-parse", "--short", "HEAD")
	if err != nil {
		return "", err
	}
	date, err := gitOut(ctx, r, "log", "-1", "--format=%cI")
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("-X %s.Version=%s -X %s.Commit=%s -X %s.Date=%s",
		ldPkg, version, ldPkg, commit, ldPkg, date), nil
}

// buildAll cross-compiles every Target, returning the artifact paths.
func buildAll(ctx context.Context, r Runner, ldflags, distDir string) ([]string, error) {
	var arts []string
	for _, t := range Targets {
		out := filepath.Join(distDir, fmt.Sprintf("pldev-%s-%s", t.OS, t.Arch))
		env := []string{"GOOS=" + t.OS, "GOARCH=" + t.Arch, "CGO_ENABLED=0"}
		if o, err := r.Capture(ctx, env, "go", "build", "-ldflags", ldflags, "-o", out, "."); err != nil {
			return nil, fmt.Errorf("build %s/%s: %w: %s", t.OS, t.Arch, err, o)
		}
		arts = append(arts, out)
	}
	return arts, nil
}
