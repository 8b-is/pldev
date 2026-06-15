package recipe

import (
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// RufloConfig drives the ruflo recipe. Values are endpoints/refs, never secret
// values: secret-bearing rc lines use shell indirection ($(bao kv get ...)) so
// nothing sensitive lands on disk or passes through pldev. Defaults come from
// the live fleet (see BuiltinRufloConfig); override in recipes.toml.
type RufloConfig struct {
	Shellrc    string          `toml:"shellrc"`
	Plugins    []string        `toml:"plugins"`
	MemBackend string          `toml:"mem_backend"`
	CLICore    bool            `toml:"cli_core"`
	Honcho     HonchoConfig    `toml:"honcho"`
	Mempalace  MempalaceConfig `toml:"mempalace"`
	Langfuse   LangfuseConfig  `toml:"langfuse"`
	Provision  ProvisionConfig `toml:"provision"`
}

// HonchoConfig: relational/peer memory plane — a stdio MCP shim pointed at the
// remote honcho API (dev-cx53, tailscale-served HTTPS). Lane: peer/prefs/dialogue.
type HonchoConfig struct {
	ShimSrc   string `toml:"shim_src"`   // scp source (M1 path of honcho-mcp.py)
	ShimDest  string `toml:"shim_dest"`  // local destination for the shim
	BaseURL   string `toml:"base_url"`   // remote honcho API over tailscale
	UserPeer  string `toml:"user_peer"`
	AgentPeer string `toml:"agent_peer"`
}

// MempalaceConfig: corpus/recall memory plane — local mempalace, pgvector
// backend. Lane: file/code/transcript recall. DSNRef is a shell-indirection
// string so the Postgres DSN is resolved at source-time, not stored.
type MempalaceConfig struct {
	Install string `toml:"install"`
	Backend string `toml:"backend"`
	DSNRef  string `toml:"dsn_ref"`
}

// LangfuseConfig: trace/cost sink. Key refs are shell-indirection strings.
type LangfuseConfig struct {
	Host         string `toml:"host"`
	PublicKeyRef string `toml:"public_key_ref"`
	SecretKeyRef string `toml:"secret_key_ref"`
}

// ProvisionConfig: nixos-rebuild of the honcho stack, triggered on M1, targeting
// dev-cx53 over the tailnet.
type ProvisionConfig struct {
	SSHHost    string `toml:"ssh_host"`    // M1 (holds the flake + deploy access)
	RepoDir    string `toml:"repo_dir"`    // flake repo dir on M1
	Target     string `toml:"target"`      // nixos config name (.#<target>)
	TargetHost string `toml:"target_host"` // dev@<dev-cx53 tailnet>
	HealthURL  string `toml:"health_url"`  // honcho health endpoint over tailscale
}

// DefaultRufloConfigPath is where pldev looks for user overrides.
func DefaultRufloConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "pldev", "recipes.toml")
}

// BuiltinRufloConfig carries defaults discovered from the live fleet (M1 ->
// dev-cx53). The canonical copy is versioned in the nix-base repo; this is the
// fallback when no recipes.toml is present.
func BuiltinRufloConfig() RufloConfig {
	return RufloConfig{
		Shellrc:    "~/.zshrc",
		Plugins:    []string{"ruflo-agentdb", "ruflo-rag-memory", "ruflo-rvf"},
		MemBackend: "rvf",
		CLICore:    true,
		Honcho: HonchoConfig{
			ShimSrc:   "lodripeter@100.92.65.95:workspace/peterlodri-sec/nix-base-oracle/modules/honcho-mcp.py",
			ShimDest:  "~/.config/pldev/honcho-mcp.py",
			BaseURL:   "https://dev-cx53.tail2870dc.ts.net",
			UserPeer:  "lodripeter",
			AgentPeer: "ruflo",
		},
		Mempalace: MempalaceConfig{
			Install: "uv tool install mempalace[pgvector]",
			Backend: "pgvector",
			DSNRef:  "$(bao kv get -field=dsn kv/mempalace/pgvector)",
		},
		Langfuse: LangfuseConfig{
			Host:         "https://langfuse.crabcc.app",
			PublicKeyRef: "$(bao kv get -field=public_key kv/ruflo/langfuse)",
			SecretKeyRef: "$(bao kv get -field=secret_key kv/ruflo/langfuse)",
		},
		Provision: ProvisionConfig{
			SSHHost:    "lodripeter@100.92.65.95",
			RepoDir:    "~/workspace/peterlodri-sec/nix-base-oracle",
			Target:     "dev-cx53",
			TargetHost: "dev@dev-cx53.tail2870dc.ts.net",
			HealthURL:  "https://dev-cx53.tail2870dc.ts.net",
		},
	}
}

// LoadRufloConfig overlays recipes.toml's [ruflo] table onto the builtin
// defaults. An absent file yields the defaults unchanged.
func LoadRufloConfig(path string) (RufloConfig, error) {
	wrap := struct {
		Ruflo RufloConfig `toml:"ruflo"`
	}{Ruflo: BuiltinRufloConfig()}
	if _, err := os.Stat(path); err != nil {
		return wrap.Ruflo, nil
	}
	if _, err := toml.DecodeFile(path, &wrap); err != nil {
		return RufloConfig{}, err
	}
	return wrap.Ruflo, nil
}
