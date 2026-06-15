package probe

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"
)

// parseMeminfo reads /proc/meminfo content. Used = Total - Available.
func parseMeminfo(s string) (MemStat, error) {
	vals := map[string]uint64{}
	sc := bufio.NewScanner(strings.NewReader(s))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 2 {
			continue
		}
		key := strings.TrimSuffix(f[0], ":")
		kb, err := strconv.ParseUint(f[1], 10, 64)
		if err != nil {
			continue
		}
		vals[key] = kb * 1024
	}
	total := vals["MemTotal"]
	if total == 0 {
		return MemStat{}, fmt.Errorf("no MemTotal")
	}
	avail := vals["MemAvailable"]
	return MemStat{Total: total, Free: vals["MemFree"], Used: total - avail}, nil
}

// parseSwapusage reads `sysctl vm.swapusage` output (macOS), values like 44032.00M.
func parseSwapusage(s string) (SwapStat, error) {
	get := func(label string) (uint64, error) {
		i := strings.Index(s, label+" = ")
		if i < 0 {
			return 0, fmt.Errorf("missing %s", label)
		}
		rest := s[i+len(label)+3:]
		fields := strings.Fields(rest)
		if len(fields) == 0 {
			return 0, fmt.Errorf("empty %s", label)
		}
		return parseSizeToken(fields[0])
	}
	total, err := get("total")
	if err != nil {
		return SwapStat{}, err
	}
	used, err := get("used")
	if err != nil {
		return SwapStat{}, err
	}
	return SwapStat{Total: total, Used: used}, nil
}

// parseSizeToken converts "44032.00M" / "1115.81M" / "1.5G" to bytes.
func parseSizeToken(tok string) (uint64, error) {
	if tok == "" {
		return 0, fmt.Errorf("empty token")
	}
	mult := float64(1)
	switch tok[len(tok)-1] {
	case 'K':
		mult, tok = 1024, tok[:len(tok)-1]
	case 'M':
		mult, tok = 1024*1024, tok[:len(tok)-1]
	case 'G':
		mult, tok = 1024*1024*1024, tok[:len(tok)-1]
	}
	f, err := strconv.ParseFloat(tok, 64)
	if err != nil {
		return 0, err
	}
	return uint64(f * mult), nil
}
