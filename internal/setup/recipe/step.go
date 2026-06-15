// Package recipe is pldev's generic step engine: a Recipe is an ordered list
// of idempotent, dry-run-aware, confirm-before-mutate Steps. Composite setup
// flows (setup ruflo, setup term) are expressed as recipes rather than bespoke
// code. Every step logs through internal/report.
package recipe

import (
	"context"

	"github.com/peterlodri-sec/pldev/internal/report"
)

// Status is the result of a step's read-only idempotency probe.
type Status int

const (
	// StatusPending: the desired state does not hold; Apply will mutate.
	StatusPending Status = iota
	// StatusSatisfied: the desired state already holds; skip Apply.
	StatusSatisfied
)

// Context carries the run-wide knobs every step needs.
type Context struct {
	Ctx    context.Context
	DryRun bool
	Yes    bool // skip per-step confirm
	Rep    *report.Reporter
}

// Step is one unit of a recipe. Label maps to the report action/target fields;
// Check is read-only (no mutation); Apply performs the change; Mutating reports
// whether the step needs a confirm before Apply (false for pure checks).
type Step interface {
	Label() (action, target string)
	Check(c Context) (Status, error)
	Apply(c Context) error
	Mutating() bool
}

// Recipe is a named, ordered list of steps.
type Recipe struct {
	Name  string
	Steps []Step
}
