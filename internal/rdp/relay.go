package rdp

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os/exec"
	"strconv"
	"sync"
	"time"
)

// Relay carries in-tab remote desktops from TuxShell's page to the server
// (RDCleanPath over a WebSocket on 127.0.0.1). The page can only use a
// one-time ticket that Go issued for a saved host: where to connect and
// which certificate to accept are decided here, never by the page.
type Relay struct {
	ln  net.Listener
	srv *http.Server

	mu      sync.Mutex
	tickets map[string]*ticket
}

type ticket struct {
	addr        string // what to dial: the host, or the local end of a tunnel
	serverName  string
	fingerprint string // "" accepts any certificate (old RDP security has none)
	tunnel      *exec.Cmd
	expires     time.Time
}

// ticketLife is how long the page has to start the connection.
const ticketLife = 60 * time.Second

// StartRelay listens on a random loopback port.
func StartRelay() (*Relay, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	r := &Relay{ln: ln, tickets: map[string]*ticket{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/rdp", r.serve)
	r.srv = &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = r.srv.Serve(ln) }()
	go r.expire()
	return r, nil
}

// URL is the WebSocket address the page connects to.
func (r *Relay) URL() string { return "ws://" + r.ln.Addr().String() + "/rdp" }

// Close stops the relay and its tunnels.
func (r *Relay) Close() error {
	r.mu.Lock()
	for k, t := range r.tickets {
		killTunnel(t.tunnel)
		delete(r.tickets, k)
	}
	r.mu.Unlock()
	return r.srv.Close()
}

func (r *Relay) issue(t *ticket) string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	tok := hex.EncodeToString(b)
	t.expires = time.Now().Add(ticketLife)
	r.mu.Lock()
	r.tickets[tok] = t
	r.mu.Unlock()
	return tok
}

// take returns a ticket once.
func (r *Relay) take(tok string) *ticket {
	r.mu.Lock()
	defer r.mu.Unlock()
	t := r.tickets[tok]
	delete(r.tickets, tok)
	if t != nil && time.Now().After(t.expires) {
		killTunnel(t.tunnel)
		return nil
	}
	return t
}

func (r *Relay) expire() {
	for range time.Tick(15 * time.Second) {
		r.mu.Lock()
		for k, t := range r.tickets {
			if time.Now().After(t.expires) {
				killTunnel(t.tunnel)
				delete(r.tickets, k)
			}
		}
		r.mu.Unlock()
	}
}

func killTunnel(c *exec.Cmd) {
	if c != nil && c.Process != nil {
		_ = c.Process.Kill()
	}
}

func (r *Relay) serve(w http.ResponseWriter, req *http.Request) {
	ws, err := upgrade(w, req)
	if err != nil {
		return
	}
	defer func() { _ = ws.Close() }()
	first, err := ws.readMessage()
	if err != nil {
		return
	}
	pdu, err := parseCleanPath(first)
	if err != nil {
		_ = ws.writeBinary(cleanPathError(cleanPathGeneralError))
		return
	}
	t := r.take(pdu.ProxyAuth)
	if t == nil {
		_ = ws.writeBinary(cleanPathError(cleanPathGeneralError))
		return
	}
	defer killTunnel(t.tunnel)
	if err := relay(ws, t, pdu.X224); err != nil {
		log.Printf("rdp tab: %v", err)
	}
}

// relay connects to the server, negotiates, does TLS and then copies.
func relay(ws *wsConn, t *ticket, x224 []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	d := net.Dialer{Timeout: 10 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", t.addr)
	if err != nil {
		_ = ws.writeBinary(cleanPathError(cleanPathGeneralError))
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(20 * time.Second))
	if _, err := conn.Write(x224); err != nil {
		return err
	}
	reply, err := readTPKT(conn)
	if err != nil {
		_ = ws.writeBinary(cleanPathError(cleanPathNegotiationError))
		return err
	}
	tc := tls.Client(conn, &tls.Config{InsecureSkipVerify: true, ServerName: t.serverName, MinVersion: tls.VersionTLS10}) //nolint:gosec // pinned below
	if err := tc.HandshakeContext(ctx); err != nil {
		_ = ws.writeBinary(cleanPathError(cleanPathGeneralError))
		return fmt.Errorf("TLS: %w", err)
	}
	certs := tc.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return errors.New("no certificate")
	}
	if t.fingerprint != "" {
		sum := sha256.Sum256(certs[0].Raw)
		if hex.EncodeToString(sum[:]) != t.fingerprint {
			_ = ws.writeBinary(cleanPathError(cleanPathGeneralError))
			return errors.New("the certificate changed since it was checked")
		}
	}
	chain := make([][]byte, len(certs))
	for i, c := range certs {
		chain[i] = c.Raw
	}
	resp, err := cleanPathResponse(reply, chain, conn.RemoteAddr().String())
	if err != nil {
		return err
	}
	if err := ws.writeBinary(resp); err != nil {
		return err
	}
	_ = conn.SetDeadline(time.Time{})

	// From here on: the page's bytes go into TLS, and back.
	done := make(chan struct{}, 2)
	go func() {
		defer func() { done <- struct{}{} }()
		buf := make([]byte, 64<<10)
		for {
			n, err := tc.Read(buf)
			if n > 0 {
				if werr := ws.writeBinary(buf[:n]); werr != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	go func() {
		defer func() { done <- struct{}{} }()
		for {
			msg, err := ws.readMessage()
			if err != nil {
				return
			}
			if _, err := tc.Write(msg); err != nil {
				return
			}
		}
	}()
	<-done
	return nil
}

// readTPKT reads one TPKT packet (the X.224 Connection Confirm).
func readTPKT(r io.Reader) ([]byte, error) {
	head := make([]byte, 4)
	if _, err := io.ReadFull(r, head); err != nil {
		return nil, err
	}
	if head[0] != 0x03 {
		return nil, errors.New("not an RDP server")
	}
	n := int(binary.BigEndian.Uint16(head[2:]))
	if n < 4 || n > 64<<10 {
		return nil, errors.New("bad TPKT length")
	}
	body := make([]byte, n-4)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}
	return append(head, body...), nil
}

// TabSession is what the page needs to draw a remote desktop in a tab.
type TabSession struct {
	// Proxy is the relay's WebSocket address and Token the one-time ticket.
	Proxy string `json:"proxy"`
	Token string `json:"token"`
	// Destination is shown by the client; the relay ignores it.
	Destination string `json:"destination"`
	User        string `json:"user"`
	Domain      string `json:"domain"`
	Name        string `json:"name"`
}

// UseRelay lets s open remote desktops in tabs. A function, so it isn't bound.
func UseRelay(s *Service, r *Relay) { s.relay = r }

// OpenTab prepares a remote desktop for a TuxShell tab: the same certificate
// check and SSH tunnel as Connect, then a ticket for the relay. The password
// is typed into the tab and goes from the page straight to the server.
func (s *Service) OpenTab(ctx context.Context, hostID int64, trust string) (TabSession, error) {
	if s.relay == nil {
		return TabSession{}, errors.New("remote desktops in tabs aren't available")
	}
	p, err := s.prepare(ctx, hostID, trust)
	if err != nil {
		return TabSession{}, err
	}
	tok := s.relay.issue(&ticket{addr: net.JoinHostPort(p.server, strconv.Itoa(p.port)), serverName: p.host.Hostname, fingerprint: p.fingerprint, tunnel: p.tunnel})
	return TabSession{
		Proxy: s.relay.URL(), Token: tok, Destination: net.JoinHostPort(p.host.Hostname, strconv.Itoa(p.host.Port)),
		User: p.host.User, Domain: p.host.RDP.Domain, Name: p.host.Name,
	}, nil
}
