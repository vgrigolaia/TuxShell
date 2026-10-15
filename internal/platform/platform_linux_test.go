package platform

import (
	"path/filepath"
	"testing"
)

// Linux paths must stay where earlier versions put them.
func TestLinuxFoldersUnchanged(t *testing.T) {
	t.Setenv("HOME", "/home/me")
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	if d, _ := DataDir(); d != filepath.FromSlash("/home/me/.local/share/tuxshell") {
		t.Errorf("DataDir = %q", d)
	}
	if d, _ := ConfigDir(); d != filepath.FromSlash("/home/me/.config/tuxshell") {
		t.Errorf("ConfigDir = %q", d)
	}
	t.Setenv("XDG_DATA_HOME", "/data")
	if d, _ := DataDir(); d != "/data/tuxshell" {
		t.Errorf("DataDir with XDG_DATA_HOME = %q", d)
	}
}
