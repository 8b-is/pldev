package verify

import (
	"bytes"
	_ "embed"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
)

//go:embed pq_signers
var pqSigners []byte

// DefaultPQAnchorPath is the runtime PQ trust-anchor override (synced across the
// fleet by `pldev fleet sync-signers`).
func DefaultPQAnchorPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "pldev", "pq_signers")
}

// PQAnchorBytes returns the effective pq_signers: the runtime file if present and
// non-empty, else the embedded default (mirrors AnchorBytes).
func PQAnchorBytes() []byte {
	if b, err := os.ReadFile(DefaultPQAnchorPath()); err == nil && len(bytes.TrimSpace(b)) > 0 {
		return b
	}
	return pqSigners
}

// verifyPQ returns the principal whose trusted ML-DSA-65 key in anchor verifies
// sig over data, or ("", false). Lines are "<principal> mldsa65 <base64-pub>";
// blank and #-comment lines are skipped.
func verifyPQ(anchor, data, sig []byte) (string, bool) {
	for _, line := range strings.Split(string(anchor), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		f := strings.Fields(line)
		if len(f) != 3 || f[1] != "mldsa65" {
			continue
		}
		pub, err := base64.StdEncoding.DecodeString(f[2])
		if err != nil {
			continue
		}
		if ok, _ := VerifyPQ(pub, data, sig); ok {
			return f[0], true
		}
	}
	return "", false
}
