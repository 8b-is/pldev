package fleet

import (
	"path/filepath"
	"testing"
)

func TestCacheRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fleet-cache.json")
	c := FleetCache{
		"dev-main": {SSHUser: "lodripeter", HW: HWInfo{Serial: "RJ6W340X7F", Model: "MacBookPro18,3", CPU: "Apple M3 Max", Cores: 16, MemGB: 64, Arch: "arm64"}},
	}
	if err := c.Save(path); err != nil {
		t.Fatal(err)
	}
	got, err := LoadCache(path)
	if err != nil {
		t.Fatal(err)
	}
	if got["dev-main"].HW.Serial != "RJ6W340X7F" || got["dev-main"].SSHUser != "lodripeter" {
		t.Fatalf("round-trip lost data: %+v", got)
	}
}

func TestLoadCacheMissingFile(t *testing.T) {
	got, err := LoadCache(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatalf("missing file must not error: %v", err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("missing file → empty non-nil cache, got %v", got)
	}
}
