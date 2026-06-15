package sys

import (
	"context"
	"testing"
	"time"
)

func TestRunCapturesStdout(t *testing.T) {
	out, err := Run(context.Background(), "echo", "hi")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != "hi\n" {
		t.Fatalf("got %q", out)
	}
}

func TestRunTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := Run(ctx, "sleep", "5")
	if err == nil {
		t.Fatal("expected timeout error")
	}
}
