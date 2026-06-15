package setup

import (
	"reflect"
	"testing"
)

func TestBuildCmd(t *testing.T) {
	cases := []struct {
		spec PkgSpec
		want []string
	}{
		{PkgSpec{Backend: BrewFormula, Name: "jq"}, []string{"brew", "install", "jq"}},
		{PkgSpec{Backend: BrewCask, Name: "ghostty"}, []string{"brew", "install", "--cask", "ghostty"}},
		{PkgSpec{Backend: Apt, Name: "zsh"}, []string{"sudo", "apt-get", "install", "-y", "zsh"}},
		{PkgSpec{Backend: Dnf, Name: "zsh"}, []string{"sudo", "dnf", "install", "-y", "zsh"}},
		{PkgSpec{Backend: Pacman, Name: "zsh"}, []string{"sudo", "pacman", "-S", "--noconfirm", "zsh"}},
		{PkgSpec{Backend: Mise, Name: "bun"}, []string{"mise", "use", "-g", "bun@latest"}},
		{PkgSpec{Backend: Script, Name: "cargo install wild-linker"}, []string{"sh", "-c", "cargo install wild-linker"}},
	}
	for _, c := range cases {
		got := BuildCmd(c.spec)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("BuildCmd(%v) = %v, want %v", c.spec, got, c.want)
		}
	}
}
