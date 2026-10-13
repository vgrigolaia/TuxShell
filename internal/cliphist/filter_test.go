package cliphist

import (
	"strings"
	"testing"
)

func TestSensitive(t *testing.T) {
	secret := []string{
		"-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXktdjE=\n-----END OPENSSH PRIVATE KEY-----",
		"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U",
		"AKIAIOSFODNN7EXAMPLE",
		"ghp_abcdefghijklmnopqrstuvwxyz0123456789",
		"export PASSWORD=hunter2",
		"api_key: 1234abcd",
		"Tr0ub4dor&3xK!q",
		"xK9#mP2$vL5nQ8wZ",
	}
	for _, s := range secret {
		if !Sensitive(s) {
			t.Errorf("not flagged: %q", s)
		}
	}

	plain := []string{
		"ls -la /var/log",
		"sudo systemctl restart patroni",
		"10.50.200.231",
		"pg-primary.internal.example.com",
		"/etc/ssh/sshd_config",
		"https://github.com/vgrigolaia/tuxshell",
		"vgrigolaia@outlook.com",
		"kubectl get pods -n monitoring",
		"PostgreSQL",
		"Password policy is enforced", // the word alone is fine
	}
	for _, s := range plain {
		if Sensitive(s) {
			t.Errorf("wrongly flagged: %q", s)
		}
	}
}

func TestRecordable(t *testing.T) {
	if Recordable("   \n") {
		t.Error("blank text recorded")
	}
	if Recordable(strings.Repeat("a ", MaxClipBytes)) {
		t.Error("oversized text recorded")
	}
	if !Recordable("echo hello") {
		t.Error("plain command not recorded")
	}
}
