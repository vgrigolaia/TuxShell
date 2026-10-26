package terminal

import (
	"os/exec"
	"slices"
	"testing"
	"time"
)

func TestParseSSH(t *testing.T) {
	for _, c := range []struct {
		argv []string
		want Foreground
	}{
		{[]string{"ssh", "root@172.16.67.98"}, Foreground{SSH: true, User: "root", Host: "172.16.67.98", Target: "root@172.16.67.98"}},
		{[]string{"/usr/bin/ssh", "-p", "2222", "-l", "ops", "db1"}, Foreground{SSH: true, User: "ops", Host: "db1", Port: 2222, Target: "ops@db1:2222"}},
		{[]string{"ssh", "-vtp2200", "web"}, Foreground{SSH: true, Host: "web", Port: 2200, Target: "web:2200"}},
		{[]string{"ssh", "-J", "jump", "-i", "key", "u@h", "uptime"}, Foreground{SSH: true, User: "u", Host: "h", Target: "u@h", Extra: true}},
		{[]string{"ssh", "ssh://me@[fe80::1]:2022"}, Foreground{SSH: true, User: "me", Host: "fe80::1", Port: 2022, Target: "me@[fe80::1]:2022"}},
		{[]string{"ssh", "-p", "22", "--", "h"}, Foreground{SSH: true, Host: "h", Port: 22, Target: "h"}},
		{[]string{"-bash"}, Foreground{}},
		{[]string{"sshd", "x"}, Foreground{}},
		{[]string{"ssh", "-V"}, Foreground{}},
	} {
		if got := parseSSH(c.argv); got != c.want {
			t.Errorf("parseSSH(%q) = %+v, want %+v", c.argv, got, c.want)
		}
	}
}

func TestForegroundArgs(t *testing.T) {
	svc := NewService(func(string, any) {}, nil, nil)
	id, err := svc.start(exec.Command("sleep", "3"), 80, 24, visit{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = svc.Close(id) })
	sess, _ := svc.get(id)
	var argv []string
	for range 50 {
		if argv = foregroundArgs(sess.ptmx); len(argv) > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !slices.Equal(argv, []string{"sleep", "3"}) {
		t.Fatalf("foreground = %q, want sleep 3", argv)
	}
	if f, err := svc.Foreground(id); err != nil || f.SSH {
		t.Fatalf("Foreground = %+v, %v; want no ssh", f, err)
	}
}
