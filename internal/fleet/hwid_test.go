package fleet

import "testing"

func TestParseHW_Darwin(t *testing.T) {
	out := "model=Mac15,11\ncpu=Apple M3 Max\ncores=16\nmembytes=68719476736\narch=arm64\nserial=HFK99M239G\n"
	hw := parseHW(out)
	if hw.Model != "Mac15,11" || hw.CPU != "Apple M3 Max" || hw.Cores != 16 ||
		hw.MemGB != 64 || hw.Arch != "arm64" || hw.Serial != "HFK99M239G" {
		t.Fatalf("got %+v", hw)
	}
}

func TestParseHW_Linux(t *testing.T) {
	out := "model=Standard PC\ncpu=Intel(R) Xeon(R) CPU E5-2680 v4 @ 2.40GHz\ncores=4\nmemkb=32827840\narch=x86_64\nserial=abc-123\n"
	hw := parseHW(out)
	if hw.Cores != 4 || hw.MemGB != 31 || hw.Arch != "amd64" || hw.Serial != "abc-123" {
		t.Fatalf("got %+v", hw)
	}
	if hw.CPU != "Intel(R) Xeon(R) CPU E5-2680 v4 @ 2.40GHz" {
		t.Fatalf("cpu stored raw, got %q", hw.CPU)
	}
}

func TestParseHW_MissingFields(t *testing.T) {
	hw := parseHW("arch=arm64\n")
	if hw.Arch != "arm64" || hw.Cores != 0 || hw.MemGB != 0 || hw.Serial != "" {
		t.Fatalf("got %+v", hw)
	}
}

func TestNormArch(t *testing.T) {
	cases := map[string]string{"aarch64": "arm64", "arm64": "arm64", "x86_64": "amd64", "amd64": "amd64", "riscv64": "riscv64", "": ""}
	for in, want := range cases {
		if got := normArch(in); got != want {
			t.Errorf("normArch(%q)=%q want %q", in, got, want)
		}
	}
}

func TestCPUCode(t *testing.T) {
	cases := map[string]string{
		"Apple M1 Pro":                               "M1 Pro",
		"AMD EPYC-Milan Processor":                   "EPYC-Milan",
		"Intel Core Processor (Haswell, no TSX)":     "Core-Haswell",
		"Intel(R) Xeon(R) CPU E5-2680 v4 @ 2.40GHz": "Xeon E5-2680 v4",
		"":                                           "",
	}
	for in, want := range cases {
		if got := cpuCode(in); got != want {
			t.Errorf("cpuCode(%q)=%q want %q", in, got, want)
		}
	}
}

func TestShortCPU(t *testing.T) {
	cases := map[string]string{
		"Apple M3 Max": "Apple M3 Max",
		"Intel(R) Xeon(R) CPU E5-2680 v4 @ 2.40GHz": "Xeon E5-2680 v4",
		"AMD EPYC 7282 16-Core Processor":           "EPYC 7282 16-Core",
		"":                                          "",
	}
	for in, want := range cases {
		if got := shortCPU(in); got != want {
			t.Errorf("shortCPU(%q)=%q want %q", in, got, want)
		}
	}
}
