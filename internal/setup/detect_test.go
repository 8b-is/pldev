package setup

import (
	"os"
	"testing"
)

func TestBrewListHas(t *testing.T) {
	b, _ := os.ReadFile("testdata/brew-list.txt")
	if !brewListHas(string(b), "jq") {
		t.Error("jq should be present")
	}
	if brewListHas(string(b), "tmux") {
		t.Error("tmux should be absent")
	}
}

func TestMiseLsHas(t *testing.T) {
	b, _ := os.ReadFile("testdata/mise-ls.txt")
	if !miseLsHas(string(b), "bun") {
		t.Error("bun should be present")
	}
	if miseLsHas(string(b), "go") {
		t.Error("go should be absent")
	}
}
