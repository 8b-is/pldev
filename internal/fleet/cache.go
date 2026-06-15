package fleet

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// NodeCache is the per-node persisted probe result.
type NodeCache struct {
	SSHUser string `json:"ssh_user,omitempty"`
	HW      HWInfo `json:"hw,omitempty"`
}

// FleetCache maps a tailnet Name to its cached probe result.
type FleetCache map[string]NodeCache

// DefaultCachePath is where the fleet probe cache lives.
func DefaultCachePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "pldev", "fleet-cache.json")
}

// LoadCache reads the cache; a missing file yields an empty (non-nil) cache.
func LoadCache(path string) (FleetCache, error) {
	c := FleetCache{}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return c, nil
		}
		return c, err
	}
	if len(b) == 0 {
		return c, nil
	}
	if err := json.Unmarshal(b, &c); err != nil {
		return FleetCache{}, err
	}
	if c == nil {
		c = FleetCache{}
	}
	return c, nil
}

// Save writes the cache as indented JSON, creating the parent dir.
func (c FleetCache) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}
