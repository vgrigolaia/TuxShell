package keepassxc

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"golang.org/x/crypto/nacl/box"
)

// KeePassXC's browser integration: JSON messages over a local socket, the
// same ones its browser extension sends through keepassxc-proxy. After a key
// exchange every message is encrypted with NaCl box.

const socketName = "org.keepassxc.KeePassXC.BrowserServer"

// Errors the UI explains.
var (
	ErrNotRunning    = errors.New("KeePassXC isn't running, or its browser integration is off (KeePassXC → Settings → Browser Integration → Enable)")
	ErrLocked        = errors.New("the KeePassXC database is locked; unlock it and try again")
	ErrNotAssociated = errors.New("TuxShell isn't connected to KeePassXC yet (Preferences → Security → Connect to KeePassXC)")
	ErrDenied        = errors.New("access was denied in KeePassXC")
	ErrNoLogins      = errors.New("no KeePassXC entry has this URL")
)

// socketPaths lists where KeePassXC listens: since 2.7 under app/ in the
// runtime folder (also for its Flatpak), before that in the runtime folder,
// and in the temp folder without one (and on macOS).
func socketPaths() []string {
	var paths []string
	if runtime.GOOS == "linux" {
		if dir := os.Getenv("XDG_RUNTIME_DIR"); dir != "" {
			paths = append(paths,
				filepath.Join(dir, "app", "org.keepassxc.KeePassXC", socketName),
				filepath.Join(dir, socketName))
		}
	}
	return append(paths, filepath.Join(os.TempDir(), socketName))
}

// conn is one conversation with KeePassXC.
type conn struct {
	c        net.Conn
	dec      *json.Decoder
	pub, sec *[32]byte
	host     *[32]byte
	clientID string
	version  string
}

// reply is a message from KeePassXC, before decryption.
type reply struct {
	Action    string `json:"action"`
	Message   string `json:"message"`
	Nonce     string `json:"nonce"`
	PublicKey string `json:"publicKey"`
	Version   string `json:"version"`
	Error     string `json:"error"`
	ErrorCode any    `json:"errorCode"`
}

// dial connects and exchanges keys. ctx bounds the whole conversation.
func dial(ctx context.Context) (*conn, error) {
	var c net.Conn
	var d net.Dialer
	for _, p := range socketPaths() {
		if cc, err := d.DialContext(ctx, "unix", p); err == nil {
			c = cc
			break
		}
	}
	if c == nil {
		return nil, ErrNotRunning
	}
	deadline, ok := ctx.Deadline()
	if !ok {
		deadline = time.Now().Add(2 * time.Minute)
	}
	_ = c.SetDeadline(deadline)
	go func() {
		<-ctx.Done()
		_ = c.SetDeadline(time.Now())
	}()

	pub, sec, err := box.GenerateKey(rand.Reader)
	if err != nil {
		_ = c.Close()
		return nil, err
	}
	k := &conn{c: c, dec: json.NewDecoder(c), pub: pub, sec: sec, clientID: b64(randomBytes(24))}
	nonce := newNonce()
	r, err := k.roundTrip("change-public-keys", map[string]any{
		"action": "change-public-keys", "publicKey": b64(pub[:]), "nonce": b64(nonce[:]), "clientID": k.clientID,
	}, nonce)
	if err != nil {
		_ = c.Close()
		return nil, err
	}
	host, err := base64.StdEncoding.DecodeString(r.PublicKey)
	if err != nil || len(host) != 32 {
		_ = c.Close()
		return nil, errors.New("KeePassXC sent an invalid public key")
	}
	k.host, k.version = (*[32]byte)(host), r.Version
	return k, nil
}

func (k *conn) Close() error { return k.c.Close() }

// roundTrip sends msg and waits for KeePassXC's answer to the same action,
// skipping notices it sends on its own (such as "database-locked").
func (k *conn) roundTrip(action string, msg map[string]any, nonce *[24]byte) (reply, error) {
	if err := json.NewEncoder(k.c).Encode(msg); err != nil {
		return reply{}, fmt.Errorf("talking to KeePassXC: %w", err)
	}
	for {
		var r reply
		if err := k.dec.Decode(&r); err != nil {
			return reply{}, fmt.Errorf("talking to KeePassXC: %w", err)
		}
		if r.Action != action {
			continue
		}
		if r.Error != "" || r.ErrorCode != nil {
			return reply{}, replyError(r.ErrorCode, r.Error)
		}
		// The answer carries our nonce plus one, so it can't be an old one replayed.
		if want := increment(*nonce); r.Nonce != b64(want[:]) {
			return reply{}, errors.New("KeePassXC's answer didn't match the request")
		}
		return r, nil
	}
}

// request sends an encrypted action and returns KeePassXC's decrypted answer.
// unlock asks KeePassXC to show its unlock window when the database is locked.
func (k *conn) request(action string, payload map[string]any, unlock bool, out any) error {
	payload["action"] = action
	plain, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	nonce := newNonce()
	msg := map[string]any{
		"action": action, "message": b64(box.Seal(nil, plain, nonce, k.host, k.sec)),
		"nonce": b64(nonce[:]), "clientID": k.clientID,
	}
	if unlock {
		msg["triggerUnlock"] = "true"
	}
	r, err := k.roundTrip(action, msg, nonce)
	if err != nil {
		return err
	}
	sealed, err := base64.StdEncoding.DecodeString(r.Message)
	if err != nil || r.Message == "" {
		return errors.New("KeePassXC sent an empty answer")
	}
	rn, _ := base64.StdEncoding.DecodeString(r.Nonce)
	opened, ok := box.Open(nil, sealed, (*[24]byte)(rn), k.host, k.sec)
	if !ok {
		return errors.New("couldn't decrypt KeePassXC's answer")
	}
	var inner struct {
		Error     string `json:"error"`
		ErrorCode any    `json:"errorCode"`
	}
	if json.Unmarshal(opened, &inner) == nil && (inner.Error != "" || inner.ErrorCode != nil) {
		return replyError(inner.ErrorCode, inner.Error)
	}
	return json.Unmarshal(opened, out)
}

// replyError turns KeePassXC's error codes (keepassxc-browser's protocol) into errors.
func replyError(code any, msg string) error {
	switch fmt.Sprint(code) {
	case "1":
		return ErrLocked
	case "6":
		return ErrDenied
	case "8", "10":
		return ErrNotAssociated
	case "15":
		return ErrNoLogins
	}
	if msg == "" {
		msg = fmt.Sprintf("error %v", code)
	}
	return fmt.Errorf("KeePassXC: %s", msg)
}

func newNonce() *[24]byte {
	return (*[24]byte)(randomBytes(24))
}

// increment adds one to a little-endian nonce, like libsodium's sodium_increment.
func increment(n [24]byte) [24]byte {
	for i := range n {
		n[i]++
		if n[i] != 0 {
			break
		}
	}
	return n
}

func randomBytes(n int) []byte {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return b
}

func b64(b []byte) string { return base64.StdEncoding.EncodeToString(b) }
