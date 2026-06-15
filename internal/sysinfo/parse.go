package sysinfo

import (
	"strconv"
	"strings"
)

// skipFS reports whether a df row (by filesystem name + mount) is a pseudo /
// virtual filesystem we don't want to display.
func skipFS(fs, mount string) bool {
	switch fs {
	case "devfs", "map", "tmpfs", "devtmpfs", "udev", "none", "overlay", "shm":
		return true
	}
	if strings.HasPrefix(fs, "map ") || fs == "auto_home" {
		return true
	}
	if strings.HasPrefix(mount, "/dev") || mount == "/run" || mount == "/sys" ||
		strings.HasPrefix(mount, "/sys/") || strings.HasPrefix(mount, "/proc") ||
		strings.HasPrefix(mount, "/System/Volumes/VM") ||
		strings.Contains(mount, "/docker/overlay") {
		return true
	}
	return false
}

// parseDf parses `df -k` output (POSIX: Filesystem 1K-blocks Used Available
// Capacity ... Mounted-on). The mount point is the last field; total/used/avail
// are fields 1/2/3; the capacity % is the field ending in '%'. Sizes are KiB.
func parseDf(s string) []Disk {
	var disks []Disk
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if i == 0 { // header
			continue
		}
		f := strings.Fields(line)
		if len(f) < 6 {
			continue
		}
		// "map auto_home" style: filesystem name has a space. Detect a leading
		// token that is non-numeric followed by another non-numeric token.
		fs := f[0]
		nums := f[1:]
		if _, err := strconv.ParseUint(f[1], 10, 64); err != nil {
			// second token not numeric -> part of filesystem name
			fs = f[0] + " " + f[1]
			nums = f[2:]
		}
		if len(nums) < 4 {
			continue
		}
		total, e1 := strconv.ParseUint(nums[0], 10, 64)
		used, e2 := strconv.ParseUint(nums[1], 10, 64)
		avail, e3 := strconv.ParseUint(nums[2], 10, 64)
		if e1 != nil || e2 != nil || e3 != nil {
			continue
		}
		mount := f[len(f)-1]
		if skipFS(fs, mount) {
			continue
		}
		var pct float64
		for _, t := range nums {
			if strings.HasSuffix(t, "%") {
				if v, err := strconv.ParseFloat(strings.TrimSuffix(t, "%"), 64); err == nil {
					pct = v
					break
				}
			}
		}
		disks = append(disks, Disk{
			Mount:  mount,
			Total:  total * 1024,
			Used:   used * 1024,
			Free:   avail * 1024,
			UsePct: pct,
		})
	}
	return disks
}

// parseIfconfig parses macOS `ifconfig` output into per-interface IPv4 lists,
// excluding loopback and 127.0.0.1.
func parseIfconfig(s string) []Net {
	var nets []Net
	var cur *Net
	for _, line := range strings.Split(s, "\n") {
		if line == "" {
			continue
		}
		if line[0] != ' ' && line[0] != '\t' {
			// interface header "en0: flags=..."
			name := strings.TrimSuffix(strings.Fields(line)[0], ":")
			if name == "lo0" || strings.HasPrefix(name, "lo") {
				cur = nil
				continue
			}
			nets = append(nets, Net{Name: name})
			cur = &nets[len(nets)-1]
			continue
		}
		if cur == nil {
			continue
		}
		f := strings.Fields(line)
		if len(f) >= 2 && f[0] == "inet" {
			ip := f[1]
			if ip != "127.0.0.1" {
				cur.IPv4 = append(cur.IPv4, ip)
			}
		}
	}
	// drop interfaces with no IPv4
	var out []Net
	for _, n := range nets {
		if len(n.IPv4) > 0 {
			out = append(out, n)
		}
	}
	return out
}

// parseIPAddr parses `ip -o -4 addr` output into per-interface IPv4 lists,
// excluding loopback and 127.0.0.1. Lines look like:
// "2: eth0    inet 10.0.2.15/24 brd ... scope global ..."
func parseIPAddr(s string) []Net {
	byName := map[string]*Net{}
	var order []string
	for _, line := range strings.Split(s, "\n") {
		f := strings.Fields(line)
		if len(f) < 4 {
			continue
		}
		name := f[1]
		if name == "lo" {
			continue
		}
		var ip string
		for i, tok := range f {
			if tok == "inet" && i+1 < len(f) {
				ip = f[i+1]
				break
			}
		}
		if ip == "" {
			continue
		}
		if slash := strings.Index(ip, "/"); slash >= 0 {
			ip = ip[:slash]
		}
		if ip == "127.0.0.1" || strings.HasPrefix(ip, "127.") {
			continue
		}
		n, ok := byName[name]
		if !ok {
			order = append(order, name)
			byName[name] = &Net{Name: name}
			n = byName[name]
		}
		n.IPv4 = append(n.IPv4, ip)
	}
	var out []Net
	for _, name := range order {
		out = append(out, *byName[name])
	}
	return out
}
