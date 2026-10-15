package rdp

import (
	"bufio"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/asn1"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

// echoRDP negotiates like Windows, then echoes what it gets inside TLS.
func echoRDP(t *testing.T, cert tls.Certificate) string {
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
				if _, err := readTPKT(c); err != nil {
					return
				}
				rsp := []byte{0x03, 0x00, 0x00, 0x13, 0x0E, 0xD0, 0, 0, 0x12, 0x34, 0, 0x02, 0x1F, 0x08, 0x00, 0x01, 0, 0, 0}
				if _, err := c.Write(rsp); err != nil {
					return
				}
				s := tls.Server(c, &tls.Config{Certificates: []tls.Certificate{cert}})
				_, _ = io.Copy(s, s)
			}(c)
		}
	}()
	return ln.Addr().String()
}

// wsClient is just enough of a WebSocket client for the test.
type wsClient struct {
	c net.Conn
	r *bufio.Reader
}

func dialWS(t *testing.T, url string) *wsClient {
	t.Helper()
	addr := strings.TrimPrefix(strings.TrimSuffix(url, "/rdp"), "ws://")
	c, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	key := make([]byte, 16)
	_, _ = rand.Read(key)
	req := "GET /rdp HTTP/1.1\r\nHost: " + addr + "\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: " +
		base64.StdEncoding.EncodeToString(key) + "\r\nSec-WebSocket-Version: 13\r\n\r\n"
	if _, err := c.Write([]byte(req)); err != nil {
		t.Fatal(err)
	}
	r := bufio.NewReader(c)
	resp, err := http.ReadResponse(r, nil)
	if err != nil || resp.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("handshake: %v %v", resp, err)
	}
	_ = resp.Body.Close()
	return &wsClient{c: c, r: r}
}

func (w *wsClient) send(p []byte) {
	mask := []byte{1, 2, 3, 4}
	head := []byte{0x82}
	if len(p) < 126 {
		head = append(head, 0x80|byte(len(p)))
	} else {
		head = append(head, 0x80|126, byte(len(p)>>8), byte(len(p)))
	}
	m := make([]byte, len(p))
	for i := range p {
		m[i] = p[i] ^ mask[i%4]
	}
	_, _ = w.c.Write(append(append(head, mask...), m...))
}

func (w *wsClient) recv(t *testing.T) []byte {
	t.Helper()
	_ = w.c.SetReadDeadline(time.Now().Add(5 * time.Second))
	var h [2]byte
	if _, err := io.ReadFull(w.r, h[:]); err != nil {
		t.Fatal(err)
	}
	n := int(h[1] & 0x7F)
	if n == 126 {
		var b [2]byte
		_, _ = io.ReadFull(w.r, b[:])
		n = int(binary.BigEndian.Uint16(b[:]))
	}
	p := make([]byte, n)
	if _, err := io.ReadFull(w.r, p); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRelay(t *testing.T) {
	cert := selfSigned(t)
	sum := sha256.Sum256(cert.Certificate[0])
	fp := hex.EncodeToString(sum[:])
	addr := echoRDP(t, cert)
	r, err := StartRelay()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()

	request := func(tok string) []byte {
		b, _ := asmRequest(tok)
		return b
	}

	// A good ticket: negotiation, certificates, then data through TLS.
	tok := r.issue(&ticket{addr: addr, serverName: "win", fingerprint: fp})
	ws := dialWS(t, r.URL())
	ws.send(request(tok))
	resp, err := parseCleanPath(ws.recv(t))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Error.ErrorCode != 0 || len(resp.X224) != 19 || len(resp.ServerCertChain) != 1 || resp.ServerAddr != addr {
		t.Fatalf("response = %+v", resp)
	}
	ws.send([]byte("hello through tls"))
	if got := string(ws.recv(t)); got != "hello through tls" {
		t.Errorf("echo = %q", got)
	}
	ws.c.Close()

	// The ticket was used up, and a made-up one doesn't work either.
	for _, bad := range []string{tok, "nope"} {
		ws := dialWS(t, r.URL())
		ws.send(request(bad))
		if resp, _ := parseCleanPath(ws.recv(t)); resp.Error.ErrorCode == 0 {
			t.Errorf("ticket %q accepted", bad)
		}
		ws.c.Close()
	}

	// A different certificate than the one checked is refused.
	tok = r.issue(&ticket{addr: addr, serverName: "win", fingerprint: strings.Repeat("0", 64)})
	ws = dialWS(t, r.URL())
	ws.send(request(tok))
	if resp, _ := parseCleanPath(ws.recv(t)); resp.Error.ErrorCode == 0 {
		t.Error("changed certificate accepted")
	}
}

// asmRequest builds what IronRDP's web client sends first.
func asmRequest(token string) ([]byte, error) {
	return asn1Marshal(cleanPathPdu{Version: cleanPathVersion, Destination: "win:3389", ProxyAuth: token, X224: negotiationRequest()})
}

func asn1Marshal(v any) ([]byte, error) { return asn1.Marshal(v) }
