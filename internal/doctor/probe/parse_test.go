package probe

import (
	"os"
	"testing"
)

func TestParseMeminfo(t *testing.T) {
	b, _ := os.ReadFile("testdata/meminfo.txt")
	m, err := parseMeminfo(string(b))
	if err != nil {
		t.Fatal(err)
	}
	if m.Total != 16384000*1024 {
		t.Errorf("total=%d", m.Total)
	}
	// used = total - available
	if m.Used != (16384000-2048000)*1024 {
		t.Errorf("used=%d", m.Used)
	}
}

func TestParseSwapusage(t *testing.T) {
	b, _ := os.ReadFile("testdata/swapusage.txt")
	s, err := parseSwapusage(string(b))
	if err != nil {
		t.Fatal(err)
	}
	// 44032.00M total
	if s.Total != uint64(44032.0*1024*1024) {
		t.Errorf("total=%d", s.Total)
	}
	if s.Used == 0 || s.Used >= s.Total {
		t.Errorf("used=%d total=%d", s.Used, s.Total)
	}
}
