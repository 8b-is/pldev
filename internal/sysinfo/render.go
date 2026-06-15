package sysinfo

import (
	"fmt"
	"strings"

	"github.com/peterlodri-sec/pldev/internal/tui"
)

func gib(b uint64) float64 { return float64(b) / (1 << 30) }

const boxWidth = 64 // inner content width

// RenderPanel renders a neofetch-style ASCII panel. When color is false, no
// lipgloss styling/ANSI escapes are emitted (plain text, golden-test friendly).
func RenderPanel(s SysInfo, color bool) string {
	lbl := func(v string) string {
		if color {
			return tui.Dim.Render(v)
		}
		return v
	}
	val := func(v string) string {
		if color {
			return tui.OK.Render(v)
		}
		return v
	}

	var b strings.Builder
	hr := "+" + strings.Repeat("-", boxWidth+2) + "+\n"

	// box line with raw (uncolored) content used for width; styled parts inlined
	line := func(content string) {
		// pad to boxWidth based on visible length (content here is plain for width)
		pad := boxWidth - visibleLen(content)
		if pad < 0 {
			pad = 0
		}
		fmt.Fprintf(&b, "| %s%s |\n", content, strings.Repeat(" ", pad))
	}

	section := func(title string) {
		b.WriteString(hr)
		t := title
		if color {
			t = tui.Title.Render(title)
		}
		line(t)
		b.WriteString(hr)
	}

	kv := func(k, v string) {
		raw := fmt.Sprintf("%-10s %s", k+":", v)
		if color {
			line(fmt.Sprintf("%s %s", lbl(fmt.Sprintf("%-10s", k+":")), val(v)))
			_ = raw
			return
		}
		line(raw)
	}

	b.WriteString(hr)
	title := "pldev ps"
	if color {
		title = tui.Title.Render("pldev ps")
	}
	line(title)

	section("SYSTEM")
	kv("Host", s.Hostname)
	kv("OS", s.OS)
	kv("Kernel", s.Kernel)
	kv("Model", s.Model)
	kv("Arch", s.Arch)
	kv("Uptime", s.Uptime.Round(1e9).String())

	section("CPU & MEM")
	kv("CPU", s.CPU)
	kv("Cores", fmt.Sprintf("%d", s.Cores))
	kv("RAM", fmt.Sprintf("%.1f/%.1f GiB used  free %.1f  avail %.1f",
		gib(s.Mem.Used), gib(s.Mem.Total), gib(s.Mem.Free), gib(s.Mem.Available)))
	kv("Swap", fmt.Sprintf("%.1f/%.1f GiB used", gib(s.Swap.Used), gib(s.Swap.Total)))

	section("DISK")
	if len(s.Disks) == 0 {
		line("(no disks)")
	}
	for _, d := range s.Disks {
		raw := fmt.Sprintf("%-20s %.1f/%.1f GiB  %.0f%%", d.Mount, gib(d.Used), gib(d.Total), d.UsePct)
		if color {
			pctStyle := tui.OK
			switch {
			case d.UsePct >= 90:
				pctStyle = tui.Crit
			case d.UsePct >= 75:
				pctStyle = tui.Warn
			}
			styled := fmt.Sprintf("%s %s %s",
				val(fmt.Sprintf("%-20s", d.Mount)),
				fmt.Sprintf("%.1f/%.1f GiB", gib(d.Used), gib(d.Total)),
				pctStyle.Render(fmt.Sprintf("%.0f%%", d.UsePct)))
			line(styled)
			_ = raw
			continue
		}
		line(raw)
	}

	section("NETWORK")
	if len(s.Nets) == 0 {
		line("(no interfaces)")
	}
	for _, n := range s.Nets {
		raw := fmt.Sprintf("%-10s -> %s", n.Name, strings.Join(n.IPv4, ", "))
		if color {
			line(fmt.Sprintf("%s -> %s", val(fmt.Sprintf("%-10s", n.Name)), strings.Join(n.IPv4, ", ")))
			_ = raw
			continue
		}
		line(raw)
	}

	b.WriteString(hr)
	return b.String()
}

//nolint:unused
// visibleLen returns the printable length of s, ignoring ANSI escape sequences.
func visibleLen(s string) int {
	n := 0
	inEsc := false
	for _, r := range s {
		if inEsc {
			if r == 'm' {
				inEsc = false
			}
			continue
		}
		if r == '\x1b' {
			inEsc = true
			continue
		}
		n++
	}
	return n
}
