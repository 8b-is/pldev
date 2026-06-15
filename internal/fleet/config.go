package fleet

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// NodeConfig annotates a discovered node with what tailscale can't know.
type NodeConfig struct {
	Nickname string   `toml:"nickname"` // human/lore name; displayed + selectable
	Role     string   `toml:"role"`
	Caps     []string `toml:"caps"`
	SSHUser  string   `toml:"ssh_user"`
	Primary  bool     `toml:"primary"` // tie-breaker: canonical alias when a machine has multiple online registrations
	Exclude  bool     `toml:"exclude"`
}

// FleetConfig is ~/.config/pldev/fleet.toml.
type FleetConfig struct {
	DefaultSSHUser string                `toml:"default_ssh_user"`
	Node           map[string]NodeConfig `toml:"node"` // keyed by short name
	Federation     FederationConfig      `toml:"federation"`
}

// FederationConfig holds federation-wide settings.
type FederationConfig struct {
	MonitorURL string `toml:"monitor_url"` // overrides the role="bus" NATS monitoring URL
}

// DefaultFleetConfigPath is where fleet annotations live.
func DefaultFleetConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "pldev", "fleet.toml")
}

// LoadFleetConfig reads the annotation file; an absent file is not an error
// (the fleet is then pure tailscale + defaults).
func LoadFleetConfig(path string) (FleetConfig, error) {
	cfg := FleetConfig{Node: map[string]NodeConfig{}}
	if _, err := os.Stat(path); err != nil {
		return cfg, nil
	}
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return FleetConfig{}, err
	}
	if cfg.Node == nil {
		cfg.Node = map[string]NodeConfig{}
	}
	return cfg, nil
}
