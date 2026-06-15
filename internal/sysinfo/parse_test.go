package sysinfo

import (
	"os"
	"testing"
)

func TestParseDfDarwin(t *testing.T) {
	b, _ := os.ReadFile("testdata/df_darwin.txt")
	disks := parseDf(string(b))
	// devfs + "map auto_home" must be skipped
	for _, d := range disks {
		if d.Mount == "/dev" {
			t.Errorf("devfs /dev should be skipped")
		}
		if d.Mount == "/System/Volumes/Data/home" {
			t.Errorf("map auto_home should be skipped")
		}
	}
	// root must be present
	var root *Disk
	for i := range disks {
		if disks[i].Mount == "/" {
			root = &disks[i]
		}
	}
	if root == nil {
		t.Fatalf("root mount / not found; got %v", disks)
	}
	if root.Total != 971350180*1024 {
		t.Errorf("root total=%d", root.Total)
	}
	if root.Used != 12636760*1024 {
		t.Errorf("root used=%d", root.Used)
	}
	if root.UsePct != 8 {
		t.Errorf("root usepct=%v", root.UsePct)
	}
}

func TestParseDfLinux(t *testing.T) {
	b, _ := os.ReadFile("testdata/df_linux.txt")
	disks := parseDf(string(b))
	for _, d := range disks {
		if d.Mount == "/dev" || d.Mount == "/run" || d.Mount == "/dev/shm" {
			t.Errorf("pseudo fs %s should be skipped", d.Mount)
		}
		if d.Mount == "/var/lib/docker/overlay2/abc/merged" {
			t.Errorf("overlay should be skipped")
		}
	}
	var root *Disk
	for i := range disks {
		if disks[i].Mount == "/" {
			root = &disks[i]
		}
	}
	if root == nil {
		t.Fatalf("root not found; got %v", disks)
	}
	if root.Total != 491252320*1024 {
		t.Errorf("root total=%d", root.Total)
	}
	if root.UsePct != 22 {
		t.Errorf("root usepct=%v", root.UsePct)
	}
}

func TestParseIfconfig(t *testing.T) {
	b, _ := os.ReadFile("testdata/ifconfig_darwin.txt")
	nets := parseIfconfig(string(b))
	for _, n := range nets {
		if n.Name == "lo0" {
			t.Errorf("loopback lo0 should be excluded")
		}
		for _, ip := range n.IPv4 {
			if ip == "127.0.0.1" {
				t.Errorf("127.0.0.1 should be excluded")
			}
		}
	}
	var en0 *Net
	for i := range nets {
		if nets[i].Name == "en0" {
			en0 = &nets[i]
		}
	}
	if en0 == nil {
		t.Fatalf("en0 not found; got %v", nets)
	}
	if len(en0.IPv4) != 1 || en0.IPv4[0] != "192.168.0.113" {
		t.Errorf("en0 ipv4=%v", en0.IPv4)
	}
}

func TestParseIPAddr(t *testing.T) {
	b, _ := os.ReadFile("testdata/ip_linux.txt")
	nets := parseIPAddr(string(b))
	for _, n := range nets {
		if n.Name == "lo" {
			t.Errorf("lo should be excluded")
		}
		for _, ip := range n.IPv4 {
			if ip == "127.0.0.1" {
				t.Errorf("127.0.0.1 should be excluded")
			}
		}
	}
	var eth0 *Net
	for i := range nets {
		if nets[i].Name == "eth0" {
			eth0 = &nets[i]
		}
	}
	if eth0 == nil {
		t.Fatalf("eth0 not found; got %v", nets)
	}
	if len(eth0.IPv4) != 1 || eth0.IPv4[0] != "10.0.2.15" {
		t.Errorf("eth0 ipv4=%v", eth0.IPv4)
	}
}
