// Package buildinfo exposes version metadata stamped into the binary at link
// time via -ldflags "-X". With no stamping (e.g. `go run`), it reports dev
// defaults so the binary still works.
package buildinfo

import "runtime"

var (
	// Version is the release/describe string, e.g. "v0.3.1-2-gabc123".
	Version = "dev"
	// Commit is the short git SHA.
	Commit = "none"
	// Date is the commit date (RFC3339), not wall-clock build time.
	Date = "unknown"
)

// String renders a single human-readable build-info line:
//
//	pldev <Version> (<Commit>, <Date>, go1.26.1)
func String() string {
	return "pldev " + Version + " (" + Commit + ", " + Date + ", " + runtime.Version() + ")"
}
