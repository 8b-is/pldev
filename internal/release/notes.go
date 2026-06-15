package release

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// prevTag returns the most recent tag reachable before HEAD, or "" if none.
func prevTag(ctx context.Context, r Runner) string {
	out, err := r.Capture(ctx, nil, "git", "describe", "--tags", "--abbrev=0", "HEAD^")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// gitLog returns commit subjects in range. If prev=="" uses all history.
func gitLog(ctx context.Context, r Runner, prev string) (string, error) {
	rng := "HEAD"
	if prev != "" {
		rng = prev + "..HEAD"
	}
	out, err := r.Capture(ctx, nil, "git", "log", "--pretty=%s", rng)
	if err != nil {
		return "", fmt.Errorf("git log: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

const signerID = "signer=brett-shaw@pldev pq=brett-shaw@pldev"

func sha256File(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// renderProvenance produces a checksum table + signer identity per artifact.
func renderProvenance(arts []string) (string, error) {
	var b strings.Builder
	b.WriteString("### Provenance\n\n")
	b.WriteString("Built and dual-signed locally (M3). ")
	b.WriteString("Verify: `pldev verify <artifact>`\n\n")
	b.WriteString("| artifact | sha256 | identity |\n|---|---|---|\n")
	for _, a := range arts {
		for _, p := range []string{a, a + ".sig", a + ".sig.mldsa"} {
			sum, err := sha256File(p)
			if err != nil {
				return "", err
			}
			fmt.Fprintf(&b, "| %s | `%s` | %s |\n", filepath.Base(p), sum, signerID)
		}
	}
	b.WriteString("\n")
	return b.String(), nil
}

// renderNotes groups commit subjects into Features / Fixes / Other.
func renderNotes(log string) string {
	var feats, fixes, other []string
	for _, line := range strings.Split(log, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "feat"):
			feats = append(feats, line)
		case strings.HasPrefix(line, "fix"):
			fixes = append(fixes, line)
		default:
			other = append(other, line)
		}
	}
	var b strings.Builder
	section := func(title string, items []string) {
		if len(items) == 0 {
			return
		}
		fmt.Fprintf(&b, "### %s\n\n", title)
		for _, it := range items {
			fmt.Fprintf(&b, "- %s\n", it)
		}
		b.WriteString("\n")
	}
	section("Features", feats)
	section("Fixes", fixes)
	section("Other", other)
	return b.String()
}
