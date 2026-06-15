package sysinfo

import (
	"context"
	"time"

	"github.com/peterlodri-sec/pldev/internal/doctor/probe"
)

// Disk is one mounted real filesystem.
type Disk struct {
	Mount  string
	Total  uint64 // bytes
	Used   uint64 // bytes
	Free   uint64 // bytes
	UsePct float64
}

// Net is one network interface with its IPv4 addresses.
type Net struct {
	Name string
	IPv4 []string
}

// Host holds the static/system identity fields.
type Host struct {
	Hostname string
	OS       string // name + version
	Kernel   string
	Model    string
	Arch     string
	CPU      string // model string
	Cores    int
	Uptime   time.Duration
}

// SysInfo is the full neofetch-style snapshot.
type SysInfo struct {
	Hostname string
	OS       string
	Kernel   string
	Model    string
	Arch     string
	CPU      string
	Cores    int
	Uptime   time.Duration
	Mem      probe.MemStat
	Swap     probe.SwapStat
	Disks    []Disk
	Nets     []Net
}

// Collector is implemented per-platform. Mem/Swap are reused from probe.
type Collector interface {
	Host(ctx context.Context) (Host, error)
	Disk(ctx context.Context) ([]Disk, error)
	Net(ctx context.Context) ([]Net, error)
}

// Collect assembles a SysInfo, recording nothing fatal: missing sections stay zero.
func Collect(ctx context.Context, c Collector, mem probe.Collector) SysInfo {
	var s SysInfo
	if h, err := c.Host(ctx); err == nil {
		s.Hostname = h.Hostname
		s.OS = h.OS
		s.Kernel = h.Kernel
		s.Model = h.Model
		s.Arch = h.Arch
		s.CPU = h.CPU
		s.Cores = h.Cores
		s.Uptime = h.Uptime
	}
	if m, err := mem.Mem(ctx); err == nil {
		s.Mem = m
	}
	if sw, err := mem.Swap(ctx); err == nil {
		s.Swap = sw
	}
	if d, err := c.Disk(ctx); err == nil {
		s.Disks = d
	}
	if n, err := c.Net(ctx); err == nil {
		s.Nets = n
	}
	return s
}
