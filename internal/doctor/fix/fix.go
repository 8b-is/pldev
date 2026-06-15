package fix

import "context"

// Fix is one guided remediation. Safe() fixes are reversible and may be
// auto-applied with --yes; destructive fixes require explicit confirmation.
type Fix interface {
	Title() string
	Describe() string // what it will do, shown before applying
	Safe() bool
	Apply(ctx context.Context) (result string, err error)
}

func SafeOnly(in []Fix) []Fix {
	var out []Fix
	for _, f := range in {
		if f.Safe() {
			out = append(out, f)
		}
	}
	return out
}

// ApplyAll applies fixes in order, returning per-fix results (best effort).
func ApplyAll(ctx context.Context, fixes []Fix) []string {
	var res []string
	for _, f := range fixes {
		out, err := f.Apply(ctx)
		if err != nil {
			res = append(res, f.Title()+": ERROR "+err.Error())
			continue
		}
		res = append(res, f.Title()+": "+out)
	}
	return res
}
