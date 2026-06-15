package fed

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peterlodri-sec/pldev/internal/report"
)

func msgFile(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "msg.vaked")
	os.WriteFile(p, []byte(`agent_msg { ts="t" from="Acolyte" to="Herald" kind="status" }`), 0o644)
	return p
}

func withVakedc(t *testing.T, found bool, out string, err error) {
	t.Helper()
	oc, ol := checkCmd, lookVakedc
	t.Cleanup(func() { checkCmd, lookVakedc = oc, ol })
	lookVakedc = func() error {
		if found {
			return nil
		}
		return errors.New("not found")
	}
	checkCmd = func(context.Context, string) (string, error) { return out, err }
}

func TestValidateOK(t *testing.T) {
	withVakedc(t, true, "ok", nil)
	var buf bytes.Buffer
	rep := report.New(&buf, "fed")
	if err := Validate(context.Background(), rep, msgFile(t)); err != nil {
		t.Fatalf("valid message should pass: %v", err)
	}
	if !strings.Contains(buf.String(), "status=ok") {
		t.Errorf("expected ok: %q", buf.String())
	}
}

func TestValidateInvalidRejected(t *testing.T) {
	withVakedc(t, true, "schema error: unknown field", errors.New("exit 1"))
	var buf bytes.Buffer
	rep := report.New(&buf, "fed")
	if err := Validate(context.Background(), rep, msgFile(t)); err == nil {
		t.Error("malformed message must be rejected")
	}
	if !strings.Contains(buf.String(), "status=fail") {
		t.Errorf("expected fail: %q", buf.String())
	}
}

func TestValidateNoVakedc(t *testing.T) {
	withVakedc(t, false, "", nil)
	rep := report.New(&bytes.Buffer{}, "fed")
	err := Validate(context.Background(), rep, msgFile(t))
	if err == nil || !strings.Contains(err.Error(), "vakedc not found") {
		t.Errorf("expected actionable vakedc-missing error, got %v", err)
	}
}

func TestValidateMissingFile(t *testing.T) {
	withVakedc(t, true, "ok", nil)
	rep := report.New(&bytes.Buffer{}, "fed")
	err := Validate(context.Background(), rep, "/nope/msg.vaked")
	if err == nil || !strings.Contains(err.Error(), "no such message file") {
		t.Errorf("expected missing-file error, got %v", err)
	}
}
