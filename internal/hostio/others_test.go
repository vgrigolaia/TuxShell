package hostio

import (
	"crypto/cipher"
	"crypto/des" //nolint:gosec // matches Remmina's encryption
	"encoding/base64"
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
	data, config := t.TempDir(), t.TempDir()
	t.Setenv("XDG_DATA_HOME", data)
	t.Setenv("XDG_CONFIG_HOME", config)
	t.Setenv("HOME", t.TempDir())
	dir := filepath.Join(data, "remmina")
	for _, d := range []string{dir, filepath.Join(config, "remmina")} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	secret := []byte("0123456789abcdefghijklmnIVIVIVIV")
	pref := "[remmina_pref]\nsecret=" + base64.StdEncoding.EncodeToString(secret) + "\n"
	if err := os.WriteFile(filepath.Join(config, "remmina", "remmina.pref"), []byte(pref), 0o600); err != nil {
		t.Fatal(err)
	}
	opened := 0
	remminaKeyring = func() (func(string) string, func()) {
		opened++
		return func(file string) string {
			if file == filepath.Join(dir, "3.remmina") {
				return "from-keyring" // gitleaks:allow
			}
			return ""
		}, func() {}
	}
	t.Cleanup(func() { remminaKeyring = systemRemminaKeyring })
	files := map[string]string{
		"1.remmina": "[remmina]\nname=Jump box\ngroup=Infra\nprotocol=SSH\nserver=203.0.113.10:2200\nusername=ops\nssh_privatekey=/home/me/.ssh/id_ed25519\npassword=" +
			encryptRemmina(t, secret, "in-the-file") + "\n",
		"2.remmina": "[remmina]\nname=Desktop\nprotocol=RDP\nserver=10.0.0.20\n",
		// The keys and values Remmina 1.4 writes for an RDP connection.
		"3.remmina": "[remmina]\nname=COPY 10.14.51.40-Khudadovi\ngroup=milesone-branches\nprotocol=RDP\nserver=10.14.51.40:3390\nusername=admin\ndomain=CORP\n" +
			"password=.\nresolution_mode=0\nresolution_width=1920\nresolution_height=1080\nviewmode=4\nmultimon=1\ndisableclipboard=0\nsound=local\n" +
			"quality=9\ncolordepth=99\ndrive=/home/me/Shared Stuff/\ngateway_server=gw.example.com:8443\ngateway_username=gwuser\n" +
			"ssh_tunnel_enabled=1\nssh_tunnel_server=bastion.example.com:2222\nssh_tunnel_username=ops\nshareprinter=1\nsharesmartcard=0\n",
		"4.remmina": "[remmina]\nname=VNC box\nprotocol=VNC\nserver=10.0.0.30\n",
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
	if len(rows) != 3 || opened != 1 {
		t.Fatalf("rows = %v, keyring opened %d times", rows, opened)
	}
	fields, _ := tb.guess()
	_, plain := buildRows(&pending{table: tb, fields: fields, header: true}, nil)
	got := map[string]bool{}
	for _, pw := range plain {
		got[pw] = true
	}
	if len(plain) != 2 || !got["in-the-file"] || !got["from-keyring"] {
		t.Errorf("passwords = %d, want the file's and the keyring's", len(plain))
	}
	expect(t, rows["Jump box"], hosts.Host{Name: "Jump box", Hostname: "203.0.113.10", User: "ops", Port: 2200, Group: "Infra", IdentityFile: "/home/me/.ssh/id_ed25519", Tags: []string{}})

	d := rows["Desktop"].Host
	if d.Protocol != hosts.ProtocolRDP || d.Port != 3389 || d.RDP.Size != "fit" || !d.RDP.Clipboard || d.RDP.Sound != "off" {
		t.Errorf("Desktop = %+v (%s)", d, rows["Desktop"].Error)
	}
	w := rows["COPY 10.14.51.40-Khudadovi"]
	r := w.Host.RDP
	if w.Error != "" || w.Host.Protocol != hosts.ProtocolRDP || w.Host.Hostname != "10.14.51.40" || w.Host.Port != 3390 || w.Host.Group != "milesone-branches" ||
		r.Domain != "CORP" || r.Size != "fixed" || r.Width != 1920 || r.Window != "fullscreen" || !r.Multimon || r.Sound != "local" || r.Quality != "best" ||
		len(r.Folders) != 1 || r.Folders[0].Name != "Shared Stuff" || r.Folders[0].Path != "/home/me/Shared Stuff" ||
		r.Gateway.Host != "gw.example.com" || r.Gateway.Port != 8443 || r.Gateway.User != "gwuser" || w.Host.ProxyJump != "ops@bastion.example.com:2222" ||
		!r.Printers || r.SmartCards {
		t.Errorf("RDP row = %+v (%s)", w.Host, w.Error)
	}
}

// encryptRemmina encrypts like Remmina does: 3DES-CBC, zero padding.
func encryptRemmina(t *testing.T, secret []byte, pw string) string {
	t.Helper()
	block, err := des.NewTripleDESCipher(secret[:24])
	if err != nil {
		t.Fatal(err)
	}
	buf := []byte(pw)
	for len(buf)%des.BlockSize != 0 {
		buf = append(buf, 0)
	}
	cipher.NewCBCEncrypter(block, secret[24:32]).CryptBlocks(buf, buf)
	return base64.StdEncoding.EncodeToString(buf)
}

func TestDecryptRemmina(t *testing.T) {
	secret := []byte("0123456789abcdefghijklmnIVIVIVIV")
	for _, pw := range []string{"a", "exactly8", "a longer password with spaces"} {
		if got, ok := decryptRemmina(secret, encryptRemmina(t, secret, pw)); !ok || got != pw {
			t.Errorf("round trip of %d bytes failed", len(pw))
		}
	}
	for _, bad := range []string{"not base64!", "c2VjcmV0", base64.StdEncoding.EncodeToString(make([]byte, 8))} {
		if _, ok := decryptRemmina(secret, bad); ok {
			t.Errorf("%q decrypted", bad)
		}
	}
	if _, ok := decryptRemmina(nil, encryptRemmina(t, secret, "x")); ok {
		t.Error("decrypted without a secret")
	}
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
