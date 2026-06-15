package recipe

// Named resolves a recipe by name, wiring its live dependencies (config from
// recipes.toml, biometric auth, ssh remote). Returns false for unknown names so
// the caller can fall through to plain `setup`.
func Named(name string) (Recipe, bool) {
	switch name {
	case "ruflo":
		cfg, _ := LoadRufloConfig(DefaultRufloConfigPath())
		return BuildRufloRecipe(cfg, BitwardenAuth{}, SSHRunner{}), true
	default:
		return Recipe{}, false
	}
}
