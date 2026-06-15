package setup

type Backend string

const (
	BrewFormula Backend = "brew"
	BrewCask    Backend = "brew-cask"
	Apt         Backend = "apt"
	Dnf         Backend = "dnf"
	Pacman      Backend = "pacman"
	Mise        Backend = "mise"
	Script      Backend = "script"
)

// PkgSpec is one install method. For Script, Name is the shell command.
type PkgSpec struct {
	Backend     Backend  `toml:"backend"`
	Name        string   `toml:"name"`
	Tap         string   `toml:"tap,omitempty"`
	PostInstall []string `toml:"post_install,omitempty"`
}

// Tool: Darwin/Linux are ordered candidates; first available backend wins.
type Tool struct {
	Name     string    `toml:"-"`
	Desc     string    `toml:"desc"`
	Category string    `toml:"category"`
	Darwin   []PkgSpec `toml:"darwin"`
	Linux    []PkgSpec `toml:"linux"`
}

type Profile struct {
	Name  string   `toml:"-"`
	Desc  string   `toml:"desc"`
	Tools []string `toml:"tools"`
}

type Registry struct {
	Tools    map[string]Tool
	Profiles map[string]Profile
}

func d(specs ...PkgSpec) []PkgSpec { return specs }

func brew(name string) PkgSpec { return PkgSpec{Backend: BrewFormula, Name: name} }
func cask(name string) PkgSpec { return PkgSpec{Backend: BrewCask, Name: name} }
func apt(name string) PkgSpec  { return PkgSpec{Backend: Apt, Name: name} }
func mise(name string) PkgSpec { return PkgSpec{Backend: Mise, Name: name} }

// BuiltinRegistry returns the curated, embedded registry.
func BuiltinRegistry() Registry {
	tools := map[string]Tool{
		"ghostty":   {Desc: "GPU terminal", Category: "terminal", Darwin: d(cask("ghostty")), Linux: d(apt("ghostty"))},
		"zsh":       {Desc: "Z shell", Category: "shell", Darwin: d(brew("zsh")), Linux: d(apt("zsh"))},
		"fish":      {Desc: "fish shell", Category: "shell", Darwin: d(brew("fish")), Linux: d(apt("fish"))},
		"jq":        {Desc: "JSON processor", Category: "cli", Darwin: d(brew("jq")), Linux: d(apt("jq"))},
		"ripgrep":   {Desc: "fast grep", Category: "cli", Darwin: d(brew("ripgrep")), Linux: d(apt("ripgrep"))},
		"fd":        {Desc: "fast find", Category: "cli", Darwin: d(brew("fd")), Linux: d(apt("fd-find"))},
		"bat":       {Desc: "cat w/ wings", Category: "cli", Darwin: d(brew("bat")), Linux: d(apt("bat"))},
		"eza":       {Desc: "modern ls", Category: "cli", Darwin: d(brew("eza")), Linux: d(apt("eza"))},
		"fzf":       {Desc: "fuzzy finder", Category: "cli", Darwin: d(brew("fzf")), Linux: d(apt("fzf"))},
		"tmux":      {Desc: "terminal multiplexer", Category: "cli", Darwin: d(brew("tmux")), Linux: d(apt("tmux"))},
		"neovim":    {Desc: "editor", Category: "editor", Darwin: d(brew("neovim")), Linux: d(apt("neovim"))},
		"starship":  {Desc: "prompt", Category: "shell", Darwin: d(brew("starship")), Linux: d(mise("starship"))},
		"gh":        {Desc: "GitHub CLI", Category: "cli", Darwin: d(brew("gh")), Linux: d(apt("gh"))},
		"git":       {Desc: "git", Category: "cli", Darwin: d(brew("git")), Linux: d(apt("git"))},
		"tailscale": {Desc: "mesh VPN", Category: "net", Darwin: d(cask("tailscale")), Linux: d(apt("tailscale"))},
		"docker":    {Desc: "containers", Category: "containers", Darwin: d(cask("docker")), Linux: d(apt("docker.io"))},
		"bun":       {Desc: "JS runtime", Category: "cli", Darwin: d(mise("bun")), Linux: d(mise("bun"))},
		"uv":        {Desc: "Python pkg/runtime mgr", Category: "cli", Darwin: d(brew("uv")), Linux: d(mise("uv"))},
		"cosign":    {Desc: "artifact signing", Category: "build", Darwin: d(brew("cosign")), Linux: d(mise("cosign"))},
		"node":      {Desc: "Node.js", Category: "cli", Darwin: d(mise("node")), Linux: d(mise("node"))},
		"go":        {Desc: "Go toolchain", Category: "cli", Darwin: d(mise("go")), Linux: d(mise("go"))},
		"mold":      {Desc: "fast linker", Category: "build", Darwin: d(brew("mold")), Linux: d(apt("mold"))},
		"mimalloc":  {Desc: "allocator", Category: "build", Darwin: d(brew("mimalloc")), Linux: d(apt("libmimalloc-dev"))},
		"wild":      {Desc: "wild linker", Category: "build", Darwin: d(PkgSpec{Backend: Script, Name: "cargo install wild-linker"}), Linux: d(PkgSpec{Backend: Script, Name: "cargo install wild-linker"})},
	}
	for name, t := range tools { // backfill Name
		t.Name = name
		tools[name] = t
	}
	profiles := map[string]Profile{
		"minimal": {Desc: "bare essentials", Tools: []string{"zsh", "git", "ripgrep", "fzf", "neovim"}},
		"dev":     {Desc: "full dev box", Tools: []string{"zsh", "git", "ripgrep", "fzf", "neovim", "ghostty", "jq", "fd", "bat", "eza", "bun", "node", "gh", "docker", "tmux", "starship"}},
		"server":  {Desc: "headless server", Tools: []string{"zsh", "git", "ripgrep", "fzf", "neovim", "tmux", "docker", "tailscale"}},
		"work":    {Desc: "work machine", Tools: []string{"zsh", "git", "ripgrep", "fzf", "neovim", "ghostty", "jq", "fd", "bat", "eza", "bun", "node", "gh", "docker", "tmux", "starship", "tailscale"}},
	}
	for name, p := range profiles {
		p.Name = name
		profiles[name] = p
	}
	return Registry{Tools: tools, Profiles: profiles}
}
