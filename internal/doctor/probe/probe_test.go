package probe

import "testing"

func TestSwapSeverity(t *testing.T) {
	cases := []struct {
		used, total uint64
		want        Severity
	}{
		{10, 100, SevOK},
		{60, 100, SevWarn},
		{80, 100, SevCrit},
	}
	for _, c := range cases {
		if got := SwapSeverity(c.used, c.total); got != c.want {
			t.Errorf("used=%d total=%d got %v want %v", c.used, c.total, got, c.want)
		}
	}
}

func TestFreeRAMSeverity(t *testing.T) {
	if FreeRAMSeverity(5, 100) != SevCrit {
		t.Error("5% free should be crit")
	}
	if FreeRAMSeverity(50, 100) != SevOK {
		t.Error("50% free should be ok")
	}
}

func TestFSEventsSeverity(t *testing.T) {
	mb := uint64(1 << 20)
	if FSEventsSeverity(300*mb) != SevOK {
		t.Error("300MB ok")
	}
	if FSEventsSeverity(700*mb) != SevWarn {
		t.Error("700MB warn")
	}
	if FSEventsSeverity(2000*mb) != SevCrit {
		t.Error("2GB crit")
	}
}
