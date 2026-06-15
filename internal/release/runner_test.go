package release

import (
	"context"
	"testing"
)

func TestExecRunnerCapture(t *testing.T) {
	out, err := execRunner{}.Capture(context.Background(), nil, "echo", "hi")
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if string(out) != "hi\n" {
		t.Fatalf("got %q", out)
	}
}
