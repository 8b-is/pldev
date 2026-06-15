package setup

import (
	"fmt"
	"sort"
	"strings"
)

// Resolve parses a "+"-joined token string. Tokens prefixed "@" are profiles;
// others are tool names. Profiles expand to their tool lists. Result is deduped,
// order-preserving. Unknown tokens produce an error listing them.
func Resolve(r Registry, input string) ([]Tool, error) {
	var unknown []string
	var order []string
	seen := map[string]bool{}

	add := func(name string) {
		if _, ok := r.Tools[name]; !ok {
			unknown = append(unknown, name)
			return
		}
		if !seen[name] {
			seen[name] = true
			order = append(order, name)
		}
	}

	for _, tok := range strings.Split(input, "+") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		if strings.HasPrefix(tok, "@") {
			pname := strings.TrimPrefix(tok, "@")
			p, ok := r.Profiles[pname]
			if !ok {
				unknown = append(unknown, "@"+pname)
				continue
			}
			for _, tn := range p.Tools {
				add(tn)
			}
			continue
		}
		add(tok)
	}

	if len(unknown) > 0 {
		valid := make([]string, 0, len(r.Tools))
		for n := range r.Tools {
			valid = append(valid, n)
		}
		sort.Strings(valid)
		return nil, fmt.Errorf("unknown: %s\nvalid tools: %s", strings.Join(unknown, ", "), strings.Join(valid, ", "))
	}

	out := make([]Tool, 0, len(order))
	for _, n := range order {
		out = append(out, r.Tools[n])
	}
	return out, nil
}
