package rdp

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/hex"
	"io"
	"math/big"
	"net"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/vgrigolaia/tuxshell/internal/hosts"
)

func rdpHost() hosts.Host {
	h := hosts.Host{Name: "win", Hostname: "10.0.5.4", User: "admin", Port: 3389, Protocol: hosts.ProtocolRDP, RDP: hosts.DefaultRDP()}
	h.RDP.Domain = "CORP"
	return h
}

func TestArgs(t *testing.T) {
	h := rdpHost()
	h.RDP.Folders = []hosts.Folder{{Name: "My Projects", Path: "/home/me/my projects"}}
	h.RDP.HomeFolder = true
	args, err := Args(h, Connection{Server: "10.0.5.4", Port: 3389, Password: "p@ss word,1", Fingerprint: "ab12", Title: "win"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"/v:10.0.5.4:3389", "/u:admin", "/d:CORP", "/p:p@ss word,1", "/cert:fingerprint:sha256:ab12", "/t:win",
		"/size:80%", "/dynamic-resolution", "/clipboard", "/drive:My Projects,/home/me/my projects", "+home-drive", "/sound", "/gfx"} {
		if !slices.Contains(args, want) {
			t.Errorf("missing %q in %q", want, args)
		}
	}

	h.RDP.Window, h.RDP.Multimon, h.RDP.Clipboard, h.RDP.Sound, h.RDP.Quality = "fullscreen", true, false, "off", "low"
	h.RDP.Gateway = hosts.Gateway{Host: "gw.example.com", Port: 443}
	args, _ = Args(h, Connection{Server: "::1", Port: 3390, Password: "pw"})
	for _, want := range []string{"/v:[::1]:3390", "/f", "/multimon", "-clipboard", "/audio-mode:none", "/network:modem", "/cert:tofu",
		"/gateway:g:gw.example.com:443,u:admin,d:CORP,p:pw"} {
		if !slices.Contains(args, want) {
			t.Errorf("missing %q in %q", want, args)
		}
	}
	if _, err := Args(h, Connection{Server: "x", Port: 1, Password: "a,b"}); err == nil {
		t.Error("comma in a gateway password accepted")
	}
	if _, err := Args(h, Connection{Server: "x", Port: 1, Password: "a\n/v:evil"}); err == nil {
		t.Error("a line break could add arguments")
	}
	h.Protocol = hosts.ProtocolSSH
	if _, err := Args(h, Connection{}); err == nil {
		t.Error("ssh host accepted")
	}
}

func selfSigned(t *testing.T) tls.Certificate {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "WIN-TEST"}, NotBefore: time.Now(), NotAfter: time.Now().Add(time.Hour)}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

// fakeRDP answers the negotiation like Windows does and then speaks TLS.
func fakeRDP(t *testing.T, cert tls.Certificate, proto uint32) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				head := make([]byte, 4)
				if _, err := io.ReadFull(c, head); err != nil {
					return
				}
				if _, err := io.ReadFull(c, make([]byte, int(binary.BigEndian.Uint16(head[2:]))-4)); err != nil {
					return
				}
				rsp := []byte{0x03, 0x00, 0x00, 0x13, 0x0E, 0xD0, 0, 0, 0x12, 0x34, 0, 0x02, 0x1F, 0x08, 0x00, 0, 0, 0, 0}
				binary.LittleEndian.PutUint32(rsp[15:], proto)
				if _, err := c.Write(rsp); err != nil || proto == 0 {
					return
				}
				s := tls.Server(c, &tls.Config{Certificates: []tls.Certificate{cert}})
				_ = s.Handshake()
				_, _ = io.Copy(io.Discard, s)
			}(c)
		}
	}()
	return ln.Addr().String()
}

func TestFetchCertificate(t *testing.T) {
	cert := selfSigned(t)
	addr := fakeRDP(t, cert, protocolSSL)
	got, err := FetchCertificate(context.Background(), addr, "win")
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(cert.Certificate[0])
	if got.Fingerprint != hex.EncodeToString(sum[:]) || got.Subject != "WIN-TEST" || !got.SelfSigned {
		t.Errorf("cert = %+v", got)
	}
	if p := Pretty("ab12cd"); p != "AB:12:CD" {
		t.Errorf("Pretty = %q", p)
	}
	if _, err := FetchCertificate(context.Background(), fakeRDP(t, cert, 0), "win"); err != ErrNoTLS {
		t.Errorf("no TLS: %v", err)
	}
	// Something that isn't RDP.
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	defer ln.Close()
	go func() {
		c, _ := ln.Accept()
		if c != nil {
			_, _ = c.Write([]byte("SSH-2.0-OpenSSH_9\r\n"))
			c.Close()
		}
	}()
	if _, err := FetchCertificate(context.Background(), ln.Addr().String(), "x"); err == nil || !strings.Contains(err.Error(), "not an RDP server") {
		t.Errorf("ssh server: %v", err)
	}
}

func TestArgsPhase2(t *testing.T) {
	h := rdpHost()
	h.RDP.Printers, h.RDP.SmartCards = true, true
	h.RDP.USB = []hosts.USBDevice{{ID: "046d:c52b", Name: "Logitech receiver"}, {ID: "1050:0407", Name: "YubiKey"}}
	args, err := Args(h, Connection{Server: "10.0.5.4", Port: 3389})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"/printer", "/smartcard", "/usb:id:046d:c52b#1050:0407", "/size:80%"} {
		if !slices.Contains(args, want) {
			t.Errorf("missing %q in %q", want, args)
		}
	}

	// A RemoteApp replaces the desktop's window options.
	h.RDP.Window = "fullscreen"
	h.RDP.RemoteApp = hosts.RemoteApp{Program: `C:\Program Files\App\app.exe`, Name: "App", Args: "/safe"}
	args, _ = Args(h, Connection{Server: "10.0.5.4", Port: 3389})
	if !slices.Contains(args, `/app:program:C:\Program Files\App\app.exe,name:App,cmd:/safe`) || slices.Contains(args, "/f") || slices.Contains(args, "/size:80%") {
		t.Errorf("remote app args: %q", args)
	}
	h.RDP.RemoteApp = hosts.RemoteApp{Program: "||notepad"}
	args, _ = Args(h, Connection{Server: "10.0.5.4", Port: 3389})
	if !slices.Contains(args, "/app:program:||notepad") {
		t.Errorf("alias: %q", args)
	}
}
