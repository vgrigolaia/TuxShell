package updates

import "testing"

func TestRepoTool(t *testing.T) {
	has := func(paths ...string) func(string) bool {
		return func(p string) bool {
			for _, x := range paths {
				if p == x {
					return true
				}
			}
			return false
		}
	}
	for _, c := range []struct {
		name, exe string
		exists    func(string) bool
		want      string
	}{
		{"apt deb822", Installed, has("/etc/apt/sources.list.d/tuxshell.sources"), "apt"},
		{"apt list", Installed, has("/etc/apt/sources.list.d/tuxshell.list"), "apt"},
		{"dnf", Installed, has("/etc/yum.repos.d/tuxshell.repo"), "dnf"},
		{"package without the repository", Installed, has(), ""},
		{"AppImage", "/tmp/.mount_TuxShe/usr/bin/tuxshell", has("/etc/yum.repos.d/tuxshell.repo"), ""},
		{"dev build", "/home/u/TuxShell/bin/tuxshell", has("/etc/apt/sources.list.d/tuxshell.list"), ""},
	} {
		if got := repoTool(c.exe, c.exists); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}
