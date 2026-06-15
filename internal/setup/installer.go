package setup

import (
	"context"

	"github.com/peterlodri-sec/pldev/internal/sys"
)

// BuildCmd returns the argv for installing the given spec. System managers are
// sudo-prefixed (sudo prompts in the TTY; pldev never reads the password).
func BuildCmd(s PkgSpec) []string {
	switch s.Backend {
	case BrewFormula:
		return []string{"brew", "install", s.Name}
	case BrewCask:
		return []string{"brew", "install", "--cask", s.Name}
	case Apt:
		return []string{"sudo", "apt-get", "install", "-y", s.Name}
	case Dnf:
		return []string{"sudo", "dnf", "install", "-y", s.Name}
	case Pacman:
		return []string{"sudo", "pacman", "-S", "--noconfirm", s.Name}
	case Mise:
		return []string{"mise", "use", "-g", s.Name + "@latest"}
	case Script:
		return []string{"sh", "-c", s.Name}
	default:
		return nil
	}
}

// Install runs the spec's command and any post-install hooks wired to the real
// terminal, so sudo can prompt and brew/apt output streams live. No output is
// captured (interactive); only the exit error is returned.
func Install(ctx context.Context, s PkgSpec) (string, error) {
	argv := BuildCmd(s)
	if err := sys.RunInteractive(ctx, argv[0], argv[1:]...); err != nil {
		return "", err
	}
	for _, hook := range s.PostInstall {
		if herr := sys.RunInteractive(ctx, "sh", "-c", hook); herr != nil {
			return "", herr
		}
	}
	return "", nil
}
