package osdetect

import (
	"context"
	"net"
	"strings"
	"testing"
)

func TestParseRelease(t *testing.T) {
	for _, c := range []struct{ text, os, name string }{
		{"NAME=\"Ubuntu\"\nID=ubuntu\nID_LIKE=debian\nPRETTY_NAME=\"Ubuntu 24.04.1 LTS\"\n", "ubuntu", "Ubuntu 24.04.1 LTS"},
		{"PRETTY_NAME=\"Debian GNU/Linux 12 (bookworm)\"\nID=debian\n", "debian", "Debian GNU/Linux 12 (bookworm)"},
		{"NAME=\"Rocky Linux\"\nID=\"rocky\"\nID_LIKE=\"rhel centos fedora\"\nPRETTY_NAME=\"Rocky Linux 9.4 (Blue Onyx)\"\n", "rocky", "Rocky Linux 9.4 (Blue Onyx)"},
		{"ID=\"almalinux\"\nID_LIKE=\"rhel centos fedora\"\n", "almalinux", ""},
		{"ID=\"rhel\"\nPRETTY_NAME=\"Red Hat Enterprise Linux 9.4 (Plow)\"\n", "rhel", "Red Hat Enterprise Linux 9.4 (Plow)"},
		{"ID=fedora\nPRETTY_NAME=\"Fedora Linux 44 (Workstation Edition)\"\n", "fedora", "Fedora Linux 44 (Workstation Edition)"},
		{"ID=\"opensuse-leap\"\nID_LIKE=\"suse opensuse\"\n", "opensuse", ""},
		{"ID=\"sles\"\n", "opensuse", ""},
		{"ID=arch\nPRETTY_NAME=\"Arch Linux\"\n", "arch", "Arch Linux"},
		{"ID=alpine\nPRETTY_NAME=\"Alpine Linux v3.20\"\n", "alpine", "Alpine Linux v3.20"},
		{"ID=freebsd\nPRETTY_NAME=\"FreeBSD 14.1-RELEASE\"\n", "freebsd", "FreeBSD 14.1-RELEASE"},
		{"ID=linuxmint\nID_LIKE=\"ubuntu debian\"\n", "ubuntu", ""},
		{"ID=raspbian\nID_LIKE=debian\n", "debian", ""},
		{"ID=\"ol\"\nID_LIKE=\"fedora\"\nPRETTY_NAME=\"Oracle Linux Server 9.4\"\n", "linux", "Oracle Linux Server 9.4"},
		{"ID=nixos\nPRETTY_NAME=\"NixOS 24.05\"\n", "linux", "NixOS 24.05"},
		{"PRETTY_NAME=\"Evil\x1b[2J name\"\nID=debian\n", "debian", "Evil[2J name"},
		{"", "", ""},
		{"cat: /etc/os-release: No such file", "", ""},
	} {
		os, name := ParseRelease(c.text)
		if os != c.os || name != c.name {
			t.Errorf("ParseRelease(%q) = %q, %q; want %q, %q", c.text, os, name, c.os, c.name)
		}
	}
}

func TestParseGreeting(t *testing.T) {
	for line, want := range map[string]string{
		"SSH-2.0-OpenSSH_9.6p1 Ubuntu-3ubuntu13.5":   "ubuntu",
		"SSH-2.0-OpenSSH_9.2p1 Debian-2+deb12u3":     "debian",
		"SSH-2.0-OpenSSH_9.2p1 Raspbian-2+deb12u3":   "debian",
		"SSH-2.0-OpenSSH_9.7 FreeBSD-20240806":       "freebsd",
		"SSH-2.0-OpenSSH_for_Windows_9.5":            "windows",
		"SSH-2.0-OpenSSH_8.7":                        "",
		"SSH-2.0-dropbear_2022.83":                   "",
		"SSH-1.99-Cisco-1.25":                        "",
		"HTTP/1.1 400 Bad Request (not ssh, ubuntu)": "",
	} {
		if got := ParseGreeting(line); got != want {
			t.Errorf("ParseGreeting(%q) = %q, want %q", line, got, want)
		}
	}
}

// The greeting is read without sending anything, after any banner lines.
func TestReadGreeting(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	got := make(chan string, 1)
	go func() {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		_, _ = c.Write([]byte("Welcome\r\nSSH-2.0-OpenSSH_9.6p1 Ubuntu-3ubuntu13\r\n"))
		buf := make([]byte, 64)
		n, _ := c.Read(buf) // returns when the client closes
		got <- string(buf[:n])
		_ = c.Close()
	}()
	line, err := readGreeting(context.Background(), ln.Addr().String())
	if err != nil || line != "SSH-2.0-OpenSSH_9.6p1 Ubuntu-3ubuntu13" {
		t.Fatalf("readGreeting = %q, %v", line, err)
	}
	if sent := <-got; sent != "" {
		t.Fatalf("client sent %q", sent)
	}
	if _, err := readGreeting(context.Background(), "127.0.0.1:1"); err == nil || strings.Contains(err.Error(), "SSH-") {
		t.Fatalf("closed port: %v", err)
	}
}
