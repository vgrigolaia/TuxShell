package hostio

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf16"

	"github.com/vgrigolaia/tuxshell/internal/hosts"
)

// rowsOf previews a fixed-format table and returns its hosts by name.
func rowsOf(t *testing.T, tb *table) (map[string]Row, int) {
	t.Helper()
	fields, header := tb.guess()
	if !header {
		t.Fatal("fixed header not recognised")
	}
	rows, plain := buildRows(&pending{table: tb, fields: fields, header: true}, nil)
	out := map[string]Row{}
	for _, r := range rows {
		out[r.Host.Name] = r
	}
	return out, len(plain)
}

func expect(t *testing.T, r Row, want hosts.Host) {
	t.Helper()
	got := r.Host
	if r.Error != "" || got.Hostname != want.Hostname || got.User != want.User || got.Port != want.Port || got.Group != want.Group ||
		got.IdentityFile != want.IdentityFile || strings.Join(got.Tags, ",") != strings.Join(want.Tags, ",") {
		t.Errorf("%s:\n got %+v (%s)\nwant %+v", want.Name, got, r.Error, want)
	}
}

func TestMobaXterm(t *testing.T) {
	data := "[Bookmarks]\r\nSubRep=\r\nImgNum=42\r\n" +
		"db-01=#109#0%10.0.0.7%2222%admin%%-1%-1%%%%%0%0%0%%%-1%0%0%0%%1080%%0%0%1#MobaFont%10%0%0%-1%15%236,236,236%30,30,30%180,180,192%0%-1%0%%xterm%-1%0%_Std_Colors_0_%80%24%0%1%-1%<none>%%0%0%-1%-1#0# #-1\r\n" +
		"[Bookmarks_1]\r\nSubRep=Prod\\Web\r\nImgNum=41\r\n" +
		"web=#109#0%web.example.com%22%root%%-1%-1%%\r\n" +
		"rdp box=#91#4%10.0.0.9%3389%%\r\n"
	tb, err := readMobaXterm([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := rowsOf(t, tb)
	if len(rows) != 2 {
		t.Fatalf("rows = %v (the RDP session should be skipped)", rows)
	}
	expect(t, rows["db-01"], hosts.Host{Name: "db-01", Hostname: "10.0.0.7", User: "admin", Port: 2222, Tags: []string{}})
	expect(t, rows["web"], hosts.Host{Name: "web", Hostname: "web.example.com", User: "root", Port: 22, Group: "Prod / Web", Tags: []string{}})
}

func TestRemmina(t *testing.T) {
	data := t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("HOME", t.TempDir())
	dir := filepath.Join(data, "remmina")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"1.remmina": "[remmina]\nname=Jump box\ngroup=Infra\nprotocol=SSH\nserver=203.0.113.10:2200\nusername=ops\nssh_privatekey=/home/me/.ssh/id_ed25519\npassword=c2VjcmV0\n",
		"2.remmina": "[remmina]\nname=Desktop\nprotocol=RDP\nserver=10.0.0.20\n",
	}
	for n, body := range files {
		if err := os.WriteFile(filepath.Join(dir, n), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	tb, found, err := scanRemmina()
	if err != nil || found != dir {
		t.Fatalf("found %q, err %v", found, err)
	}
	rows, _ := rowsOf(t, tb)
	if len(rows) != 1 {
		t.Fatalf("rows = %v", rows)
	}
	expect(t, rows["Jump box"], hosts.Host{Name: "Jump box", Hostname: "203.0.113.10", User: "ops", Port: 2200, Group: "Infra", IdentityFile: "/home/me/.ssh/id_ed25519", Tags: []string{}})
}

func TestPuTTYDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".putty", "sessions")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for n, body := range map[string]string{
		"my%20server":        "HostName=10.0.0.7\nPortNumber=2222\nUserName=admin\nProtocol=ssh\nPublicKeyFile=/home/me/key.ppk\n",
		"Default%20Settings": "HostName=\nProtocol=ssh\n",
		"serial":             "HostName=/dev/ttyS0\nProtocol=serial\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, n), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	tb, _, err := scanPuTTY()
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := rowsOf(t, tb)
	if len(rows) != 1 {
		t.Fatalf("rows = %v", rows)
	}
	expect(t, rows["my server"], hosts.Host{Name: "my server", Hostname: "10.0.0.7", User: "admin", Port: 2222, Tags: []string{}})
}

func TestPuTTYReg(t *testing.T) {
	reg := "Windows Registry Editor Version 5.00\r\n\r\n" +
		"[HKEY_CURRENT_USER\\Software\\SimonTatham\\PuTTY\\Sessions\\web%20prod]\r\n" +
		"\"HostName\"=\"web.example.com\"\r\n\"PortNumber\"=dword:00000016\r\n\"UserName\"=\"deploy\"\r\n\"Protocol\"=\"ssh\"\r\n\r\n" +
		"[HKEY_CURRENT_USER\\Software\\SimonTatham\\PuTTY\\Sessions\\telnet]\r\n\"HostName\"=\"10.0.0.1\"\r\n\"Protocol\"=\"telnet\"\r\n"
	// regedit saves UTF-16LE with a BOM.
	u := utf16.Encode([]rune(reg))
	data := []byte{0xFF, 0xFE}
	for _, c := range u {
		data = append(data, byte(c), byte(c>>8))
	}
	tb, err := readPuTTYReg(data)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := rowsOf(t, tb)
	if len(rows) != 1 {
		t.Fatalf("rows = %v", rows)
	}
	expect(t, rows["web prod"], hosts.Host{Name: "web prod", Hostname: "web.example.com", User: "deploy", Port: 22, Tags: []string{}})
}

func TestAnsibleINI(t *testing.T) {
	inv := `# inventory
bastion ansible_host=203.0.113.10 ansible_user=ops

[web]
web[01:02].lan ansible_port=2222
db ansible_host=10.0.0.7 ansible_password='hunter2'

[db]
db

[web:vars]
ansible_user=deploy
ansible_ssh_private_key_file=~/.ssh/web

[prod:children]
web
`
	tb, err := readAnsibleINI([]byte(inv))
	if err != nil {
		t.Fatal(err)
	}
	rows, passwords := rowsOf(t, tb)
	if len(rows) != 4 || passwords != 1 {
		t.Fatalf("rows = %d, passwords = %d", len(rows), passwords)
	}
	expect(t, rows["bastion"], hosts.Host{Name: "bastion", Hostname: "203.0.113.10", User: "ops", Port: 22, Tags: []string{}})
	expect(t, rows["web01.lan"], hosts.Host{Name: "web01.lan", Hostname: "web01.lan", User: "deploy", Port: 2222, Group: "web", IdentityFile: "~/.ssh/web", Tags: []string{}})
	expect(t, rows["web02.lan"], hosts.Host{Name: "web02.lan", Hostname: "web02.lan", User: "deploy", Port: 2222, Group: "web", IdentityFile: "~/.ssh/web", Tags: []string{}})
	expect(t, rows["db"], hosts.Host{Name: "db", Hostname: "10.0.0.7", User: "deploy", Port: 22, Group: "web", IdentityFile: "~/.ssh/web", Tags: []string{"db"}})
	if rows["db"].Host.PasswordRef != "" {
		t.Error("an Ansible password was kept")
	}
}
