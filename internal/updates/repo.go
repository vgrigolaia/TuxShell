package updates

import (
	"os"
	"path/filepath"
)

// Installed is the path the .deb and .rpm put the binary at.
const Installed = "/usr/bin/tuxshell"

// repoFiles are the source files the install steps on pkg.tuxshell.io create,
// by the command that updates TuxShell once they're there.
var repoFiles = []struct{ path, tool string }{
	{"/etc/apt/sources.list.d/tuxshell.sources", "apt"},
	{"/etc/apt/sources.list.d/tuxshell.list", "apt"},
	{"/etc/yum.repos.d/tuxshell.repo", "dnf"},
}

// Repo says which package manager updates this TuxShell ("apt" or "dnf"), or
// "" when it wasn't installed from the TuxShell package repository. The app
// still checks GitHub then, but tells the user to update with that tool.
func (s *Service) Repo() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	return repoTool(exe, func(p string) bool { _, err := os.Stat(p); return err == nil })
}

func repoTool(exe string, exists func(string) bool) string {
	// The AppImage, a Flatpak (its /etc is the runtime's) and builds run from
	// elsewhere aren't updated by the repository.
	if exe != Installed {
		return ""
	}
	for _, f := range repoFiles {
		if exists(f.path) {
			return f.tool
		}
	}
	return ""
}
