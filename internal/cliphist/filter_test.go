package cliphist

import (
	"strings"
	"testing"
)

func TestSensitive(t *testing.T) {
	secret := []string{
		// Fake samples for the filter (gitleaks:allow marks them for the secret scan).
		"-----BEGIN OPENSSH PRIVATE KEY-----\nb3BlbnNzaC1rZXktdjE=\n-----END OPENSSH PRIVATE KEY-----", // gitleaks:allow
		"eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.dozjgNryP4J3jVmNHl0w5N_XgL0n3I9PlFUP0THsR8U", // gitleaks:allow
		"AKIAIOSFODNN7EXAMPLE",
		"ghp_abcdefghijklmnopqrstuvwxyz0123456789",
		"export PASSWORD=hunter2",
		"api_key: 1234abcd",
		"Tr0ub4dor&3xK!q",
		"xK9#mP2$vL5nQ8wZ",
		// Secrets inside a line: pgbouncer userlist entries (fake values).
		`"pgbouncer_auth" "n7Qw2LzPx4RbT9kVm3"`,                         // gitleaks:allow
		`"app" "SCRAM-SHA-256$4096:c2FsdA==$c3RvcmVkS2V5:c2VydmVyS2V5"`, // gitleaks:allow
		`"app" "md5d41d8cd98f00b204e9800998ecf8427e"`,                   // gitleaks:allow
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
		"sudo dnf install -y NetworkManager-tui",
		"rpm -q falcon-sensor; sudo /opt/CrowdStrike/falconctl -g --cid",
		"kernel-modules-6.12.0-211.74.2.el10_0.x86_64",
		"docker run -d --name web-01 nginx:1.27",
		"git checkout 3f2a9c1e7b4d5a6f8e0c1b2a3d4e5f6a7b8c9d0e",
		"kubectl delete pod web-7d4b9c6f5-x2k8q",
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIHk7Qw2LzPx4RbT9kVm3Fq8Jd user@laptop",
		`"pgbouncer_auth" "ExampleUser"`,
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
