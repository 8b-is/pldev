package recipe

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/peterlodri-sec/pldev/internal/report"
)

type fakeStep struct {
	action, target string
	status         Status
	checkErr       error
	applyErr       error
	mutating       bool
	applied        *bool
}

func (f fakeStep) Label() (string, string) { return f.action, f.target }
func (f fakeStep) Mutating() bool           { return f.mutating }
func (f fakeStep) Check(Context) (Status, error) {
	return f.status, f.checkErr
}
func (f fakeStep) Apply(Context) error {
	if f.applied != nil {
		*f.applied = true
	}
	return f.applyErr
}

func runWith(t *testing.T, steps []Step, opts Options) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	opts.Rep = report.New(&buf, "test")
	err := Run(context.Background(), Recipe{Name: "demo", Steps: steps}, opts)
	return buf.String(), err
}

func TestSatisfiedSkips(t *testing.T) {
	applied := false
	out, err := runWith(t, []Step{
		fakeStep{action: "x", target: "t", status: StatusSatisfied, mutating: true, applied: &applied},
	}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if applied {
		t.Error("satisfied step must not Apply")
	}
	if !strings.Contains(out, "status=have") {
		t.Errorf("expected have: %q", out)
	}
}

func TestDryRunPreviewsWithoutApply(t *testing.T) {
	applied := false
	out, err := runWith(t, []Step{
		fakeStep{action: "x", target: "t", status: StatusPending, mutating: true, applied: &applied},
	}, Options{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if applied {
		t.Error("dry-run must not Apply")
	}
	if !strings.Contains(out, "status=dry") {
		t.Errorf("expected dry: %q", out)
	}
}

func TestConfirmDeclineSkips(t *testing.T) {
	old := confirm
	defer func() { confirm = old }()
	confirm = func(string) bool { return false }
	applied := false
	out, _ := runWith(t, []Step{
		fakeStep{action: "x", target: "t", status: StatusPending, mutating: true, applied: &applied},
	}, Options{})
	if applied {
		t.Error("declined step must not Apply")
	}
	if !strings.Contains(out, "status=skip") {
		t.Errorf("expected skip: %q", out)
	}
}

func TestYesBypassesConfirmAndApplies(t *testing.T) {
	old := confirm
	defer func() { confirm = old }()
	confirm = func(string) bool { t.Fatal("confirm must not be called with Yes"); return false }
	applied := false
	out, err := runWith(t, []Step{
		fakeStep{action: "x", target: "t", status: StatusPending, mutating: true, applied: &applied},
	}, Options{Yes: true})
	if err != nil || !applied {
		t.Fatalf("expected applied, err=%v applied=%v", err, applied)
	}
	if !strings.Contains(out, "status=ok") {
		t.Errorf("expected ok: %q", out)
	}
}

func TestFailContinuesAndSummaryCounts(t *testing.T) {
	a2 := false
	out, err := runWith(t, []Step{
		fakeStep{action: "a", target: "1", status: StatusPending, mutating: true, applyErr: errors.New("boom")},
		fakeStep{action: "a", target: "2", status: StatusPending, mutating: true, applied: &a2},
	}, Options{Yes: true})
	if err == nil {
		t.Error("run with a failed step must return error")
	}
	if !a2 {
		t.Error("run must continue to step 2 after step 1 fails")
	}
	if !strings.Contains(out, "applied=1") || !strings.Contains(out, "failed=1") {
		t.Errorf("summary counts wrong: %q", out)
	}
}

func TestCheckErrorIsFailure(t *testing.T) {
	out, err := runWith(t, []Step{
		fakeStep{action: "a", target: "1", checkErr: errors.New("probe failed"), mutating: true},
	}, Options{Yes: true})
	if err == nil {
		t.Error("check error must fail the run")
	}
	if !strings.Contains(out, "status=fail") {
		t.Errorf("expected fail: %q", out)
	}
}

func TestNonMutatingNeedsNoConfirm(t *testing.T) {
	old := confirm
	defer func() { confirm = old }()
	confirm = func(string) bool { t.Fatal("non-mutating step must not confirm"); return false }
	// Verify-like: non-mutating, pending -> Apply runs without confirm
	applied := false
	_, err := runWith(t, []Step{
		fakeStep{action: "verify", target: "t", status: StatusPending, mutating: false, applied: &applied},
	}, Options{})
	if err != nil || !applied {
		t.Fatalf("non-mutating pending step should Apply: err=%v applied=%v", err, applied)
	}
}
