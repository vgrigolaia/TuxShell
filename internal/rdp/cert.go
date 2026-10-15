package rdp

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

// Certificate is a remote desktop's TLS certificate, as the user is asked to
// trust it.
type Certificate struct {
	// Fingerprint is the SHA-256 of the certificate (lowercase hex), which
	// FreeRDP is told to accept and nothing else.
	Fingerprint string `json:"fingerprint"`
	Subject     string `json:"subject"`
	Issuer      string `json:"issuer"`
	NotAfter    int64  `json:"notAfter"` // Unix milliseconds
	// SelfSigned is true for the usual Windows default certificate.
	SelfSigned bool `json:"selfSigned"`
}

// ErrNoTLS means the server only offers old RDP security without TLS, so
// there is no certificate to check.
var ErrNoTLS = errors.New("the server doesn't offer TLS")

// Protocols in an RDP negotiation request (MS-RDPBCGR 2.2.1.1.1).
const (
	protocolSSL    = 0x1
	protocolHybrid = 0x2
)

// negotiationRequest is a TPKT + X.224 Connection Request asking for TLS
// (or CredSSP, which also starts with TLS).
func negotiationRequest() []byte {
	neg := []byte{0x01, 0x00, 0x08, 0x00, 0, 0, 0, 0}
	binary.LittleEndian.PutUint32(neg[4:], protocolSSL|protocolHybrid)
	x224 := append([]byte{byte(6 + len(neg)), 0xE0, 0, 0, 0, 0, 0}, neg...)
	pkt := []byte{0x03, 0x00, 0, 0}
	binary.BigEndian.PutUint16(pkt[2:], uint16(4+len(x224)))
	return append(pkt, x224...)
}

// readNegotiation reads the server's X.224 Connection Confirm and returns the
// protocol it chose.
func readNegotiation(r io.Reader) (uint32, error) {
	head := make([]byte, 4)
	if _, err := io.ReadFull(r, head); err != nil {
		return 0, err
	}
	if head[0] != 0x03 {
		return 0, errors.New("not an RDP server")
	}
	n := int(binary.BigEndian.Uint16(head[2:]))
	if n < 11 || n > 1024 {
		return 0, errors.New("not an RDP server")
	}
	body := make([]byte, n-4)
	if _, err := io.ReadFull(r, body); err != nil {
		return 0, err
	}
	if body[1]&0xF0 != 0xD0 {
		return 0, errors.New("not an RDP server")
	}
	// Without a negotiation response the server only speaks old RDP security.
	if len(body) < 7+8 {
		return 0, nil
	}
	neg := body[7:]
	switch neg[0] {
	case 0x02: // RDP_NEG_RSP
		return binary.LittleEndian.Uint32(neg[4:8]), nil
	case 0x03: // RDP_NEG_FAILURE
		return 0, fmt.Errorf("the server refused TLS (code %d)", binary.LittleEndian.Uint32(neg[4:8]))
	}
	return 0, errors.New("unexpected RDP negotiation reply")
}

// FetchCertificate connects to an RDP server and returns its certificate,
// without logging in or trusting it.
func FetchCertificate(ctx context.Context, addr, serverName string) (Certificate, error) {
	d := net.Dialer{Timeout: 10 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return Certificate{}, err
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	} else {
		_ = conn.SetDeadline(time.Now().Add(15 * time.Second))
	}
	if _, err := conn.Write(negotiationRequest()); err != nil {
		return Certificate{}, err
	}
	proto, err := readNegotiation(conn)
	if err != nil {
		return Certificate{}, err
	}
	if proto == 0 {
		return Certificate{}, ErrNoTLS
	}
	// The certificate is checked by its fingerprint, below, not by a CA.
	t := tls.Client(conn, &tls.Config{InsecureSkipVerify: true, ServerName: serverName, MinVersion: tls.VersionTLS10}) //nolint:gosec
	if err := t.HandshakeContext(ctx); err != nil {
		return Certificate{}, fmt.Errorf("TLS: %w", err)
	}
	certs := t.ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return Certificate{}, errors.New("the server sent no certificate")
	}
	return describe(certs[0]), nil
}

func describe(c *x509.Certificate) Certificate {
	sum := sha256.Sum256(c.Raw)
	return Certificate{
		Fingerprint: hex.EncodeToString(sum[:]),
		Subject:     name(c.Subject.CommonName, c.Subject.String()),
		Issuer:      name(c.Issuer.CommonName, c.Issuer.String()),
		NotAfter:    c.NotAfter.UnixMilli(),
		SelfSigned:  bytes.Equal(c.RawIssuer, c.RawSubject),
	}
}

func name(cn, full string) string {
	if cn != "" {
		return cn
	}
	return full
}

// Pretty formats a fingerprint as colon-separated pairs for showing.
func Pretty(fp string) string {
	var b strings.Builder
	for i := 0; i+1 < len(fp); i += 2 {
		if i > 0 {
			b.WriteByte(':')
		}
		b.WriteString(strings.ToUpper(fp[i : i+2]))
	}
	return b.String()
}
