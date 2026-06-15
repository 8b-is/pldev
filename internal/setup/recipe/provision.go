package recipe

import (
	"context"
	"fmt"

	"github.com/peterlodri-sec/pldev/internal/sys"
)

// Authenticator gates a high-consequence action behind a presence/biometric
// check. The default impl unlocks Vaultwarden (Touch ID via the Bitwarden
// desktop integration); a failed unlock blocks the action.
type Authenticator interface {
	Authenticate(reason string) error
}

// RemoteRunner runs a command on a remote host (ssh over tailscale).
type RemoteRunner interface {
	Run(host string, argv []string) (string, error)
}

// --- default impls ---

// BitwardenAuth unlocks Vaultwarden as the biometric gate. `bw unlock` prompts
// Touch ID through the desktop integration; pldev never sees the secret.
type BitwardenAuth struct{}

func (BitwardenAuth) Authenticate(reason string) error {
	// Interactive: the Touch ID / passphrase prompt must reach the real TTY.
	return sys.RunInteractive(context.Background(), "bw", "unlock")
}

// SSHRunner runs argv on host via ssh (host is a tailscale alias).
type SSHRunner struct{}

func (SSHRunner) Run(host string, argv []string) (string, error) {
	return sys.Run(context.Background(), "ssh", append([]string{host}, argv...)...)
}

// --- Provision step ---

// Provision idempotently brings up a remote stack: if HealthProbe reports it
// already running, the step is satisfied; otherwise it authenticates (biometric)
// and runs the remote bring-up command over ssh. Mutating + auth-gated.
type Provision struct {
	Name        string
	Host        string
	Argv        []string // remote command, e.g. docker compose -f <path> up -d
	HealthProbe func(c Context) bool
	Auth        Authenticator
	Remote      RemoteRunner
}

func (s Provision) Label() (string, string) { return "provision", s.Name }
func (s Provision) Mutating() bool           { return true }

func (s Provision) Check(c Context) (Status, error) {
	if s.HealthProbe != nil && s.HealthProbe(c) {
		return StatusSatisfied, nil
	}
	return StatusPending, nil
}

func (s Provision) Apply(c Context) error {
	if s.Auth == nil || s.Remote == nil {
		return fmt.Errorf("provision %s: authenticator and remote runner required", s.Name)
	}
	if err := s.Auth.Authenticate("provision " + s.Name); err != nil {
		return fmt.Errorf("auth: %w", err)
	}
	if _, err := s.Remote.Run(s.Host, s.Argv); err != nil {
		return fmt.Errorf("remote: %w", err)
	}
	return nil
}
