package report

import (
	"io"
	"testing"
	"time"
)

// BenchmarkPlainOK asserts the steady-state Plain path is allocation-free.
// Run: go test ./internal/report -bench=PlainOK -benchmem
func BenchmarkPlainOK(b *testing.B) {
	r := newReporter(io.Discard, "setup", Plain)
	r.now = fixed
	kv := Dur(820 * time.Millisecond)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.OK("install", "ripgrep", kv)
	}
}

func TestPlainOKZeroAlloc(t *testing.T) {
	r := newReporter(io.Discard, "setup", Plain)
	r.now = fixed
	kv := Dur(820 * time.Millisecond)
	avg := testing.AllocsPerRun(1000, func() {
		r.OK("install", "ripgrep", kv)
	})
	if avg != 0 {
		t.Errorf("steady-state Plain OK allocates %.2f allocs/op, want 0", avg)
	}
}
