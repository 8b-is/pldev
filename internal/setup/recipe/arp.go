package recipe

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/peterlodri-sec/pldev/internal/arp"
)

// ArpConfig drives the arp-corpus pipeline. ARP_LOG_PATH points at the clone's
// corpus/ dir (a base dir); the #273 hook computes <host>/<date>.md under it.
type ArpConfig struct {
	ClonePath   string `toml:"clone_path"`
	Remote      string `toml:"remote"`
	Shellrc     string `toml:"shellrc"`
	HookInstall string `toml:"hook_install"` // command that installs+registers the #273 hook
}

// BuiltinArpConfig carries the discovered defaults.
func BuiltinArpConfig() ArpConfig {
	home, _ := os.UserHomeDir()
	return ArpConfig{
		ClonePath:   filepath.Join(home, ".local", "state", "pldev", "arp-corpus"),
		Remote:      "git@github.com:peterlodri-sec/arp-corpus.git",
		Shellrc:     "~/.zshrc",
		HookInstall: "", // set to the vaked-base meta-hook bootstrap command for this machine
	}
}

// LoadArpConfig overlays recipes.toml's [arp] table onto the defaults.
func LoadArpConfig(path string) (ArpConfig, error) {
	wrap := struct {
		Arp ArpConfig `toml:"arp"`
	}{Arp: BuiltinArpConfig()}
	if _, err := os.Stat(path); err != nil {
		return wrap.Arp, nil
	}
	if _, err := toml.DecodeFile(path, &wrap); err != nil {
		return ArpConfig{}, err
	}
	return wrap.Arp, nil
}

// BuildArpRecipe assembles the arp-corpus setup recipe: clone the corpus,
// install the #273 hook with output retargeted to the clone, point MemPalace at
// it, and verify validity. The hook-install step is skipped when no command is
// configured (env-specific via the meta-hook bootstrap).
func BuildArpRecipe(cfg ArpConfig) Recipe {
	rc := expandHome(cfg.Shellrc)
	clone := expandHome(cfg.ClonePath)
	corpus := filepath.Join(clone, "corpus")

	steps := []Step{
		InstallTools{Tokens: "git+uv"},
		RunCmd{ActionName: "clone", TargetName: "arp-corpus",
			Name: "sh", Args: []string{"-c", "test -d " + clone + "/.git || git clone " + cfg.Remote + " " + clone},
			Probe: func(Context) bool { _, err := os.Stat(filepath.Join(clone, ".git")); return err == nil }},
	}

	if cfg.HookInstall != "" {
		steps = append(steps, RunCmd{ActionName: "install", TargetName: "arp-hook",
			Name: "sh", Args: []string{"-c", cfg.HookInstall},
			Probe: func(Context) bool { return hookRegistered() }})
	}

	steps = append(steps,
		// hook writes here; it computes <host>/<date>.md under this base dir
		AppendRC{Shellrc: rc, Line: "export ARP_LOG_PATH=" + corpus},
		// MemPalace mines the corpus as a recall source (re-mined by `pldev arp sync`)
		RunCmd{ActionName: "mempalace-mine", TargetName: "arp-corpus",
			Name: "sh", Args: []string{"-c", "mempalace mine " + corpus}},
		Verify{ActionName: "verify", TargetName: "corpus-valid", Probe: func(c Context) error {
			_, err := arp.Verify(c.Ctx, clone)
			return err
		}},
		Verify{ActionName: "verify", TargetName: "arp-log-path", Probe: func(Context) error {
			b, err := os.ReadFile(rc)
			if err != nil || !strings.Contains(string(b), "ARP_LOG_PATH="+corpus) {
				return &probeError{"ARP_LOG_PATH not set in " + rc}
			}
			return nil
		}},
	)
	return Recipe{Name: "arp", Steps: steps}
}

// hookRegistered reports whether the arp hook is wired in ~/.claude/settings.json.
func hookRegistered() bool {
	home, _ := os.UserHomeDir()
	b, err := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	return err == nil && strings.Contains(string(b), "arp_log")
}
