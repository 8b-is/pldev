package setup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMergeOverridesAndAdds(t *testing.T) {
	base := BuiltinRegistry()
	toml := `
[tools.ghostty]
desc = "custom"
category = "terminal"
darwin = [{ backend = "brew-cask", name = "ghostty-tip" }]

[tools.mytool]
category = "cli"
darwin = [{ backend = "brew", name = "mytool" }]

[profiles.dev]
desc = "slim dev"
tools = ["ghostty", "mytool"]
`
	dir := t.TempDir()
	path := filepath.Join(dir, "setup.toml")
	if err := os.WriteFile(path, []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
	user, err := LoadUserManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	merged := Merge(base, user)

	if got := merged.Tools["ghostty"].Darwin[0].Name; got != "ghostty-tip" {
		t.Errorf("override failed: ghostty darwin name = %q", got)
	}
	if merged.Tools["ghostty"].Name != "ghostty" {
		t.Error("Name backfill lost on override")
	}
	if _, ok := merged.Tools["mytool"]; !ok {
		t.Error("new tool not added")
	}
	if got := merged.Profiles["dev"].Tools; len(got) != 2 {
		t.Errorf("profile override failed: %v", got)
	}
	// untouched builtin remains
	if _, ok := merged.Tools["jq"]; !ok {
		t.Error("builtin jq lost after merge")
	}
}

func TestLoadMissingFileIsEmptyNoError(t *testing.T) {
	user, err := LoadUserManifest(filepath.Join(t.TempDir(), "nope.toml"))
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if len(user.Tools) != 0 || len(user.Profiles) != 0 {
		t.Error("missing file should yield empty registry")
	}
}
