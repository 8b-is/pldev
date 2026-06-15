package fix

import (
	"context"
	"testing"
)

type fakeFix struct {
	name string
	safe bool
	done *bool
}

func (f fakeFix) Title() string                         { return f.name }
func (f fakeFix) Safe() bool                             { return f.safe }
func (f fakeFix) Describe() string                      { return "does " + f.name }
func (f fakeFix) Apply(context.Context) (string, error) { *f.done = true; return "ok", nil }

func TestSafeOnlyFilters(t *testing.T) {
	var a, b bool
	all := []Fix{
		fakeFix{"safe1", true, &a},
		fakeFix{"danger", false, &b},
	}
	got := SafeOnly(all)
	if len(got) != 1 || got[0].Title() != "safe1" {
		t.Fatalf("SafeOnly wrong: %v", got)
	}
}

func TestApplyAllSafe(t *testing.T) {
	var a, b bool
	all := []Fix{fakeFix{"s", true, &a}, fakeFix{"d", false, &b}}
	ApplyAll(context.Background(), SafeOnly(all))
	if !a || b {
		t.Fatalf("expected only safe applied: a=%v b=%v", a, b)
	}
}
