package fleet

import (
	"context"
	"math"
	"strconv"
	"strings"

	"github.com/peterlodri-sec/pldev/internal/sys"
)

// HWInfo is a node's hardware fingerprint, gathered over ssh and cached.
type HWInfo struct {
	Serial string `json:"serial,omitempty"` // dup key; linux falls back to /etc/machine-id
	Model  string `json:"model,omitempty"`  // "Mac15,11", linux product_name
	CPU    string `json:"cpu,omitempty"`    // chip name, stored raw; shortened at render
	Cores  int    `json:"cores,omitempty"`  // physical cores
	MemGB  int    `json:"mem_gb,omitempty"` // total RAM rounded to GB
	Arch   string `json:"arch,omitempty"`   // normalized arm64|amd64|<raw>
}

// gatherScript is one POSIX sh script that emits key=val lines for either OS.
func gatherScript() string {
	return `os=$(uname -s)
if [ "$os" = "Darwin" ]; then
  echo "model=$(sysctl -n hw.model 2>/dev/null)"
  echo "cpu=$(sysctl -n machdep.cpu.brand_string 2>/dev/null)"
  echo "cores=$(sysctl -n hw.physicalcpu 2>/dev/null)"
  echo "membytes=$(sysctl -n hw.memsize 2>/dev/null)"
  echo "arch=$(uname -m)"
  echo "serial=$(ioreg -rd1 -c IOPlatformExpertDevice | awk -F'\"' '/IOPlatformSerialNumber/{print $4}')"
else
  echo "model=$(cat /sys/class/dmi/id/product_name 2>/dev/null)"
  echo "cpu=$(awk -F: '/model name/{print $2; exit}' /proc/cpuinfo | sed 's/^ *//')"
  echo "cores=$(nproc 2>/dev/null)"
  echo "memkb=$(awk '/MemTotal/{print $2}' /proc/meminfo)"
  echo "arch=$(uname -m)"
  echo "serial=$(cat /sys/class/dmi/id/product_uuid 2>/dev/null || cat /etc/machine-id 2>/dev/null)"
fi`
}

// parseHW turns key=val lines (from gatherScript) into an HWInfo.
func parseHW(out string) HWInfo {
	var hw HWInfo
	for _, line := range strings.Split(out, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		switch k {
		case "model":
			hw.Model = v
		case "cpu":
			hw.CPU = v
		case "cores":
			hw.Cores, _ = strconv.Atoi(v)
		case "membytes":
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				hw.MemGB = normMemBytes(n)
			}
		case "memkb":
			if n, err := strconv.ParseInt(v, 10, 64); err == nil {
				hw.MemGB = normMemKB(n)
			}
		case "arch":
			hw.Arch = normArch(v)
		case "serial":
			hw.Serial = v
		}
	}
	return hw
}

func normArch(raw string) string {
	switch raw {
	case "aarch64", "arm64":
		return "arm64"
	case "x86_64", "amd64":
		return "amd64"
	default:
		return raw
	}
}

func normMemBytes(b int64) int { return int(math.Round(float64(b) / (1 << 30))) }
func normMemKB(kb int64) int   { return int(math.Round(float64(kb) / (1 << 20))) }

// shortCPU trims vendor noise for display: drops (R)/(TM), the word Intel/AMD/CPU/
// Processor, and anything after an "@" clock suffix; collapses runs of spaces.
func shortCPU(full string) string {
	if full == "" {
		return ""
	}
	s := full
	for _, junk := range []string{"(R)", "(r)", "(TM)", "(tm)"} {
		s = strings.ReplaceAll(s, junk, "")
	}
	if i := strings.Index(s, "@"); i >= 0 {
		s = s[:i]
	}
	var keep []string
	for _, w := range strings.Fields(s) {
		switch w {
		case "Intel", "AMD", "CPU", "Processor":
			continue
		}
		keep = append(keep, w)
	}
	return strings.Join(keep, " ")
}

// gatherHW runs gatherScript over ssh and parses the result. Injectable for tests.
var gatherHW = func(ctx context.Context, user, ip string) (HWInfo, error) {
	args := append(append([]string{}, sshOpts...), user+"@"+ip, gatherScript())
	out, err := sys.Run(ctx, "ssh", args...)
	if err != nil {
		return HWInfo{}, err
	}
	return parseHW(out), nil
}
