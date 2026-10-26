// Package keepassxc reads passwords from a running, unlocked KeePassXC through
// its browser integration. Hosts store only a reference such as
// "keepassxc:ssh://root@db01.example" (the URL of a KeePassXC entry); the
// password is fetched at connect time and never stored or sent to the UI.
// TuxShell never sees the database or its master password.
package keepassxc

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode"

	"golang.org/x/crypto/nacl/box"
)

// RefPrefix starts a KeePassXC reference.
const RefPrefix = "keepassxc:"

// Keyring keeps the association (KeePassXC's permission for TuxShell) in the
// system keyring; secrets.SystemKeyring satisfies it.
type Keyring interface {
	Set(service, user, secret string) error
	Get(service, user string) (string, error)
	Delete(service, user string) error
}

const (
	keyringService = "TuxShell"
	keyringUser    = "keepassxc-association"
	// timeout covers KeePassXC's "allow access?" question, which waits for the user.
	timeout = 2 * time.Minute
)

// association is what KeePassXC gave TuxShell when the user allowed it: the
// name they typed and the identity key TuxShell proves itself with.
type association struct {
	ID  string `json:"id"`
	Key string `json:"key"`
}

// Client talks to KeePassXC, one conversation at a time.
type Client struct {
	Keyring Keyring
	mu      sync.Mutex
}

// IsRef reports whether ref is meant for KeePassXC (valid or not).
func IsRef(ref string) bool { return strings.HasPrefix(strings.TrimSpace(ref), RefPrefix) }

// ValidRef reports whether ref looks like keepassxc:<scheme>://[user@]host[:port][/path].
func ValidRef(ref string) bool {
	_, _, err := parseRef(ref)
	return err == nil
}

// parseRef splits a reference into the URL KeePassXC matches entries by and
// the user that picks one entry when several match.
func parseRef(ref string) (site, user string, err error) {
	rest, ok := strings.CutPrefix(strings.TrimSpace(ref), RefPrefix)
	bad := errors.New("not a KeePassXC reference like keepassxc:ssh://user@host")
	if !ok || strings.ContainsFunc(rest, unicode.IsSpace) || strings.ContainsFunc(rest, unicode.IsControl) {
		return "", "", bad
	}
	u, err := url.Parse(rest)
	if err != nil || u.Scheme == "" || u.Hostname() == "" || strings.HasPrefix(u.Hostname(), "-") {
		return "", "", bad
	}
	if u.User != nil {
		user = u.User.Username()
		u.User = nil
	}
	return u.String(), user, nil
}

// Ref builds a reference for a host: keepassxc:ssh://user@host[:port].
func Ref(scheme, user, host string, port, defaultPort int) string {
	u := url.URL{Scheme: scheme, Host: host}
	if strings.Contains(host, ":") {
		u.Host = "[" + host + "]"
	}
	if port > 0 && port != defaultPort {
		u.Host += fmt.Sprintf(":%d", port)
	}
	if user != "" {
		u.User = url.User(user)
	}
	return RefPrefix + u.String()
}

func (c *Client) load() (association, error) {
	if c.Keyring == nil {
		return association{}, ErrNotAssociated
	}
	v, err := c.Keyring.Get(keyringService, keyringUser)
	if err != nil || v == "" {
		return association{}, ErrNotAssociated
	}
	var a association
	if json.Unmarshal([]byte(v), &a) != nil || a.ID == "" || a.Key == "" {
		return association{}, ErrNotAssociated
	}
	return a, nil
}

// session runs fn in one conversation with KeePassXC.
func (c *Client) session(ctx context.Context, fn func(*conn) error) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	k, err := dial(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = k.Close() }()
	return fn(k)
}

type entry struct {
	Login    string `json:"login"`
	Name     string `json:"name"`
	Password string `json:"password"`
}

// Read returns the password of the entry ref points to. KeePassXC may ask
// the user to unlock the database or to allow access first.
func (c *Client) Read(ctx context.Context, ref string) (string, error) {
	site, user, err := parseRef(ref)
	if err != nil {
		return "", err
	}
	a, err := c.load()
	if err != nil {
		return "", err
	}
	var res struct {
		Entries []entry `json:"entries"`
	}
	err = c.session(ctx, func(k *conn) error {
		return k.request("get-logins", map[string]any{
			"url": site, "submitUrl": site,
			"keys": []map[string]string{{"id": a.ID, "key": a.Key}},
		}, true, &res)
	})
	if err != nil {
		return "", err
	}
	return pick(res.Entries, site, user)
}

// pick chooses the one entry for user (any user when it's empty).
func pick(entries []entry, site, user string) (string, error) {
	var found []entry
	for _, e := range entries {
		if user == "" || e.Login == user {
			found = append(found, e)
		}
	}
	switch {
	case len(found) == 0 && user != "":
		return "", fmt.Errorf("no KeePassXC entry for %s has the user %s", site, user)
	case len(found) == 0:
		return "", ErrNoLogins
	case len(found) > 1:
		names := make([]string, len(found))
		for i, e := range found {
			names[i] = e.Name
		}
		return "", fmt.Errorf("%d KeePassXC entries match %s (%s); put the user in the reference, like keepassxc:ssh://user@host",
			len(found), site, strings.Join(names, ", "))
	case found[0].Password == "":
		return "", fmt.Errorf("the KeePassXC entry %q has no password", found[0].Name)
	}
	return found[0].Password, nil
}

// Associate asks KeePassXC to allow TuxShell: KeePassXC shows a window where
// the user names the connection. Returns that name.
func (c *Client) Associate(ctx context.Context) (string, error) {
	if c.Keyring == nil {
		return "", errors.New("no system keyring to keep KeePassXC's permission in")
	}
	idPub, _, err := box.GenerateKey(rand.Reader)
	if err != nil {
		return "", err
	}
	var res struct {
		ID string `json:"id"`
	}
	err = c.session(ctx, func(k *conn) error {
		return k.request("associate", map[string]any{"key": b64(k.pub[:]), "idKey": b64(idPub[:])}, true, &res)
	})
	if err != nil {
		return "", err
	}
	if res.ID == "" {
		return "", errors.New("KeePassXC didn't confirm the connection")
	}
	v, _ := json.Marshal(association{ID: res.ID, Key: b64(idPub[:])})
	if err := c.Keyring.Set(keyringService, keyringUser, string(v)); err != nil {
		return "", fmt.Errorf("saving KeePassXC's permission in the keyring: %w", err)
	}
	return res.ID, nil
}

// Forget drops TuxShell's side of the association (KeePassXC keeps its own
// entry until the user removes it there).
func (c *Client) Forget() error {
	if c.Keyring == nil {
		return nil
	}
	err := c.Keyring.Delete(keyringService, keyringUser)
	if err != nil && !strings.Contains(strings.ToLower(err.Error()), "not found") {
		return err
	}
	return nil
}

// Status is what Preferences shows about KeePassXC. It never holds a secret.
type Status struct {
	// Running is true when KeePassXC's browser integration answered.
	Running bool   `json:"running"`
	Version string `json:"version"`
	// Connected is true when KeePassXC knows TuxShell; Name is the name the user gave it there.
	Connected bool   `json:"connected"`
	Name      string `json:"name"`
	Locked    bool   `json:"locked"`
	// Error says what went wrong ("" when nothing did).
	Error string `json:"error"`
}

// Status checks KeePassXC without asking it to unlock or showing any window.
func (c *Client) Status(ctx context.Context) Status {
	var st Status
	a, aerr := c.load()
	if aerr == nil {
		st.Name = a.ID
	}
	err := c.session(ctx, func(k *conn) error {
		st.Running, st.Version = true, k.version
		if aerr != nil {
			return nil
		}
		var res struct {
			ID string `json:"id"`
		}
		if err := k.request("test-associate", map[string]any{"id": a.ID, "key": a.Key}, false, &res); err != nil {
			return err
		}
		st.Connected = true
		return nil
	})
	switch {
	case errors.Is(err, ErrLocked):
		st.Locked = true
	case err != nil:
		st.Error = err.Error()
	}
	return st
}
