package setup

import (
	"errors"
	"io/fs"
	"os"

	"github.com/BurntSushi/toml"
)

// userManifest mirrors the TOML shape.
type userManifest struct {
	Tools    map[string]Tool    `toml:"tools"`
	Profiles map[string]Profile `toml:"profiles"`
}

// LoadUserManifest reads ~/.config/pldev/setup.toml (or given path). Missing file
// returns an empty Registry and no error. Malformed file returns an error.
func LoadUserManifest(path string) (Registry, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Registry{Tools: map[string]Tool{}, Profiles: map[string]Profile{}}, nil
	}
	if err != nil {
		return Registry{}, err
	}
	var m userManifest
	if err := toml.Unmarshal(b, &m); err != nil {
		return Registry{}, err
	}
	if m.Tools == nil {
		m.Tools = map[string]Tool{}
	}
	if m.Profiles == nil {
		m.Profiles = map[string]Profile{}
	}
	return Registry{Tools: m.Tools, Profiles: m.Profiles}, nil
}

// DefaultManifestPath returns ~/.config/pldev/setup.toml.
func DefaultManifestPath() string {
	return os.Getenv("HOME") + "/.config/pldev/setup.toml"
}

// Merge overlays user onto base: same-key entries fully replace; new keys add.
// Name fields are (re)backfilled from the map key.
func Merge(base, user Registry) Registry {
	out := Registry{
		Tools:    map[string]Tool{},
		Profiles: map[string]Profile{},
	}
	for k, v := range base.Tools {
		out.Tools[k] = v
	}
	for k, v := range base.Profiles {
		out.Profiles[k] = v
	}
	for k, v := range user.Tools {
		v.Name = k
		out.Tools[k] = v
	}
	for k, v := range user.Profiles {
		v.Name = k
		out.Profiles[k] = v
	}
	return out
}
