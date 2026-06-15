package release

import (
	"context"
	"fmt"

	"github.com/peterlodri-sec/pldev/internal/pqkey"
	"github.com/peterlodri-sec/pldev/internal/report"
	"github.com/peterlodri-sec/pldev/internal/verify"
)

// pqSign is the PQ signer; overridable in tests. Produces <bin>.sig.mldsa.
var pqSign = pqkey.SignFile

var verifyFn = verify.Verify

// verifyAll runs the verifier on every artifact; any failure aborts (fail-closed).
func verifyAll(ctx context.Context, rep *report.Reporter, arts []string) error {
	for _, a := range arts {
		if err := verifyFn(ctx, rep, a); err != nil {
			return fmt.Errorf("verify %s: %w", a, err)
		}
	}
	return nil
}

// signArtifact writes <bin>.sig (ed25519) and <bin>.sig.mldsa (ML-DSA-65).
func signArtifact(ctx context.Context, r Runner, bin, ed25519Key, mldsaKey string) error {
	// ed25519 via ssh-keygen (interactive: may prompt for passphrase).
	if err := r.Interactive(ctx, "ssh-keygen", "-Y", "sign", "-n", "pldev", "-f", ed25519Key, bin); err != nil {
		return fmt.Errorf("ed25519 sign %s: %w", bin, err)
	}
	if err := pqSign(mldsaKey, bin); err != nil {
		return fmt.Errorf("pq sign %s: %w", bin, err)
	}
	return nil
}
