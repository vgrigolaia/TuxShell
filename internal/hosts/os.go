package hosts

import "strings"

// OSes are the operating systems a host can be marked with; the frontend has
// a logo for each. "" means not set.
var OSes = []string{"ubuntu", "debian", "rhel", "almalinux", "rocky", "fedora", "centos", "opensuse", "arch", "alpine", "freebsd", "windows", "linux"}

// osWords maps words found in free text ("Ubuntu 24.04", "Red Hat
// Enterprise Linux 9") to an OS id. Checked in order, so specific names come
// before the generic "linux".
var osWords = []struct{ word, os string }{
	{"ubuntu", "ubuntu"},
	{"debian", "debian"},
	{"alma", "almalinux"},
	{"rocky", "rocky"},
	{"centos", "centos"},
	{"fedora", "fedora"},
	{"red hat", "rhel"},
	{"redhat", "rhel"},
	{"rhel", "rhel"},
	{"suse", "opensuse"},
	{"sles", "opensuse"},
	{"arch", "arch"},
	{"alpine", "alpine"},
	{"freebsd", "freebsd"},
	{"windows", "windows"},
	{"linux", "linux"},
}

// ParseOS reads an OS from free text, returning "" when it isn't recognised.
func ParseOS(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	for _, id := range OSes {
		if s == id {
			return id
		}
	}
	for _, w := range osWords {
		if strings.Contains(s, w.word) {
			return w.os
		}
	}
	if s == "win" || strings.HasPrefix(s, "win ") || strings.HasPrefix(s, "windows") {
		return "windows"
	}
	return ""
}

func knownOS(id string) bool {
	for _, o := range OSes {
		if o == id {
			return true
		}
	}
	return false
}
