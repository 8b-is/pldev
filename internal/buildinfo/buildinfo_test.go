package buildinfo

import (
	"runtime"
	"strings"
	"testing"
)

func TestStringDefaults(t *testing.T) {
	got := String()
	for _, want := range []string{"pldev", "dev", "none", "unknown", runtime.Version()} {
		if !strings.Contains(got, want) {
			t.Errorf("String()=%q missing %q", got, want)
		}
	}
}

func TestStringStamped(t *testing.T) {
	old := Version
	Version = "v1.2.3"
	defer func() { Version = old }()
	if got := String(); !strings.Contains(got, "v1.2.3") {
		t.Errorf("String()=%q missing stamped version", got)
	}
}
