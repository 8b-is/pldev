package setup

import "testing"

func TestResolveNamesProfilesDedup(t *testing.T) {
	r := BuiltinRegistry()
	tools, err := Resolve(r, "@minimal+jq+git")
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, tl := range tools {
		if names[tl.Name] {
			t.Errorf("duplicate %q in result", tl.Name)
		}
		names[tl.Name] = true
	}
	// minimal includes git already; +git must not duplicate
	if !names["git"] || !names["jq"] || !names["zsh"] {
		t.Errorf("missing expected tools: %v", names)
	}
}

func TestResolveUnknownErrors(t *testing.T) {
	r := BuiltinRegistry()
	_, err := Resolve(r, "jq+nonsuch+@nope")
	if err == nil {
		t.Fatal("expected error for unknown tokens")
	}
}
