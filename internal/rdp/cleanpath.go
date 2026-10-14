package rdp

import (
	"encoding/asn1"
	"errors"
)

// RDCleanPath (from IronRDP) lets a client that can't do TLS itself - the
// remote desktop drawn in a TuxShell tab - reach an RDP server: the relay
// sends the client's X.224 request, does the TLS handshake, returns the
// server's reply and certificates, and then passes the decrypted stream on.

const cleanPathVersion = 3389 + 1

// Error codes in cleanPathErr.
const (
	cleanPathGeneralError     = 1
	cleanPathNegotiationError = 2
)

type cleanPathErr struct {
	ErrorCode      int `asn1:"explicit,tag:0"`
	HTTPStatusCode int `asn1:"explicit,tag:1,optional"`
	WSALastError   int `asn1:"explicit,tag:2,optional"`
	TLSAlertCode   int `asn1:"explicit,tag:3,optional"`
}

type cleanPathPdu struct {
	Version           int64        `asn1:"explicit,tag:0"`
	Error             cleanPathErr `asn1:"explicit,tag:1,optional"`
	Destination       string       `asn1:"explicit,tag:2,optional,utf8"`
	ProxyAuth         string       `asn1:"explicit,tag:3,optional,utf8"`
	ServerAuth        string       `asn1:"explicit,tag:4,optional,utf8"`
	PreconnectionBlob string       `asn1:"explicit,tag:5,optional,utf8"`
	X224              []byte       `asn1:"explicit,tag:6,optional"`
	ServerCertChain   [][]byte     `asn1:"explicit,tag:7,optional"`
	ServerAddr        string       `asn1:"explicit,tag:9,optional,utf8"`
}

func parseCleanPath(b []byte) (cleanPathPdu, error) {
	var p cleanPathPdu
	rest, err := asn1.Unmarshal(b, &p)
	if err != nil {
		return p, err
	}
	if len(rest) != 0 {
		return p, errors.New("extra bytes after the RDCleanPath request")
	}
	if p.Version != cleanPathVersion {
		return p, errors.New("unsupported RDCleanPath version")
	}
	return p, nil
}

func cleanPathResponse(x224 []byte, certs [][]byte, addr string) ([]byte, error) {
	return asn1.Marshal(cleanPathPdu{Version: cleanPathVersion, X224: x224, ServerCertChain: certs, ServerAddr: addr})
}

func cleanPathError(code int) []byte {
	b, _ := asn1.Marshal(cleanPathPdu{Version: cleanPathVersion, Error: cleanPathErr{ErrorCode: code}})
	return b
}
