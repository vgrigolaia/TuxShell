package keepassxc

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"golang.org/x/crypto/nacl/box"
)

// memKeyring is a keyring in memory.
type memKeyring struct {
	mu sync.Mutex
	m  map[string]string
}

func (k *memKeyring) Set(s, u, v string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.m[s+"/"+u] = v
	return nil
}

func (k *memKeyring) Get(s, u string) (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	v, ok := k.m[s+"/"+u]
	if !ok {
		return "", errors.New("secret not found in keyring")
	}
	return v, nil
}

func (k *memKeyring) Delete(s, u string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	delete(k.m, s+"/"+u)
	return nil
}

// fakeKeePassXC answers the browser protocol like KeePassXC does.
type fakeKeePassXC struct {
	locked  atomic.Bool
	entries map[string][]entry // by URL
	idKeys  map[string]string  // association name → idKey
}

func (f *fakeKeePassXC) serve(t *testing.T, l net.Listener) {
	for {
		c, err := l.Accept()
		if err != nil {
			return
		}
		go f.handle(t, c)
	}
}

func (f *fakeKeePassXC) handle(t *testing.T, c net.Conn) {
	defer c.Close()
	pub, sec, _ := box.GenerateKey(rand.Reader)
	var client *[32]byte
	dec, enc := json.NewDecoder(c), json.NewEncoder(c)
	// An unrelated notice first: the client must skip it.
	_ = enc.Encode(map[string]string{"action": "database-unlocked"})
	for {
		var m map[string]string
		if dec.Decode(&m) != nil {
			return
		}
		n, _ := base64.StdEncoding.DecodeString(m["nonce"])
		next := increment(*(*[24]byte)(n))
		if m["action"] == "change-public-keys" {
			k, _ := base64.StdEncoding.DecodeString(m["publicKey"])
			client = (*[32]byte)(k)
			_ = enc.Encode(map[string]string{"action": "change-public-keys", "publicKey": b64(pub[:]), "nonce": b64(next[:]), "version": "2.8.0", "success": "true"})
			continue
		}
		if f.locked.Load() {
			_ = enc.Encode(map[string]string{"action": m["action"], "errorCode": "1", "error": "Database not opened"})
			continue
		}
		sealed, _ := base64.StdEncoding.DecodeString(m["message"])
		plain, ok := box.Open(nil, sealed, (*[24]byte)(n), client, sec)
		if !ok {
			t.Errorf("couldn't decrypt %s", m["action"])
			return
		}
		var req map[string]any
		_ = json.Unmarshal(plain, &req)
		var res map[string]any
		switch m["action"] {
		case "associate":
			if req["key"] != b64(client[:]) {
				t.Errorf("associate key isn't the client's public key")
			}
			f.idKeys["tuxshell-test"] = req["idKey"].(string)
			res = map[string]any{"id": "tuxshell-test", "success": "true"}
		case "test-associate":
			if f.idKeys[req["id"].(string)] != req["key"] {
				_ = enc.Encode(map[string]string{"action": m["action"], "errorCode": "8", "error": "Association failed"})
				continue
			}
			res = map[string]any{"id": req["id"], "success": "true"}
		case "get-logins":
			keys := req["keys"].([]any)[0].(map[string]any)
			if f.idKeys[keys["id"].(string)] != keys["key"] {
				_ = enc.Encode(map[string]string{"action": m["action"], "errorCode": "8", "error": "Association failed"})
				continue
			}
			found := f.entries[req["url"].(string)]
			if len(found) == 0 {
				_ = enc.Encode(map[string]string{"action": m["action"], "errorCode": "15", "error": "No logins found"})
				continue
			}
			res = map[string]any{"entries": found, "count": len(found), "success": "true"}
		}
		out, _ := json.Marshal(res)
		_ = enc.Encode(map[string]string{"action": m["action"], "message": b64(box.Seal(nil, out, &next, client, sec)), "nonce": b64(next[:])})
	}
}

func startFake(t *testing.T, f *fakeKeePassXC) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("the fake listens where KeePassXC does on Linux")
	}
	dir, err := os.MkdirTemp("", "kpxc")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	t.Setenv("XDG_RUNTIME_DIR", dir)
	sock := filepath.Join(dir, "app", "org.keepassxc.KeePassXC", socketName)
	if err := os.MkdirAll(filepath.Dir(sock), 0o700); err != nil {
		t.Fatal(err)
	}
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go f.serve(t, l)
}

func TestReadThroughKeePassXC(t *testing.T) {
	f := &fakeKeePassXC{idKeys: map[string]string{}, entries: map[string][]entry{
		"ssh://db01": {{Login: "root", Name: "db01 root", Password: "s3cret"}, {Login: "app", Name: "db01 app", Password: "other"}}, // gitleaks:allow
		"ssh://web":  {{Login: "deploy", Name: "web", Password: "w"}},                                                               // gitleaks:allow
	}}
	startFake(t, f)
	c := &Client{Keyring: &memKeyring{m: map[string]string{}}}
	ctx := context.Background()

	if _, err := c.Read(ctx, "keepassxc:ssh://web"); !errors.Is(err, ErrNotAssociated) {
		t.Fatalf("before associating: %v", err)
	}
	if st := c.Status(ctx); !st.Running || st.Connected || st.Version != "2.8.0" {
		t.Fatalf("status before associating = %+v", st)
	}
	name, err := c.Associate(ctx)
	if err != nil || name != "tuxshell-test" {
		t.Fatalf("Associate = %q, %v", name, err)
	}
	if st := c.Status(ctx); !st.Connected || st.Name != "tuxshell-test" {
		t.Fatalf("status after associating = %+v", st)
	}

	for ref, want := range map[string]string{
		"keepassxc:ssh://root@db01": "s3cret", // gitleaks:allow
		"keepassxc:ssh://web":       "w",
	} {
		if got, err := c.Read(ctx, ref); err != nil || got != want {
			t.Errorf("Read(%s) = %q, %v; want %q", ref, got, err, want)
		}
	}
	for ref, want := range map[string]string{
		"keepassxc:ssh://db01":        "2 KeePassXC entries match",
		"keepassxc:ssh://nobody@db01": "has the user nobody",
		"keepassxc:ssh://missing":     ErrNoLogins.Error(),
	} {
		if _, err := c.Read(ctx, ref); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("Read(%s) error = %v, want %q", ref, err, want)
		}
	}

	f.locked.Store(true)
	if st := c.Status(ctx); !st.Locked {
		t.Errorf("locked status = %+v", st)
	}
	if _, err := c.Read(ctx, "keepassxc:ssh://web"); !errors.Is(err, ErrLocked) {
		t.Errorf("locked read: %v", err)
	}
	f.locked.Store(false)

	if err := c.Forget(); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Read(ctx, "keepassxc:ssh://web"); !errors.Is(err, ErrNotAssociated) {
		t.Errorf("after Forget: %v", err)
	}
}

func TestNotRunning(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	t.Setenv("TMPDIR", t.TempDir())
	if st := (&Client{}).Status(context.Background()); st.Running || st.Error != ErrNotRunning.Error() {
		t.Fatalf("status = %+v", st)
	}
}

func TestRefs(t *testing.T) {
	for ref, ok := range map[string]bool{
		"keepassxc:ssh://root@db01":          true,
		"keepassxc:ssh://db01:2222":          true,
		"keepassxc:rdp://win.example/office": true,
		"keepassxc:ssh://[fe80::1]:22":       true,
		"keepassxc:db01":                     false,
		"keepassxc:ssh://":                   false,
		"keepassxc:ssh://-oProxy":            false,
		"keepassxc:ssh://a b":                false,
		"op://Vault/Item/password":           false,
	} {
		if ValidRef(ref) != ok {
			t.Errorf("ValidRef(%q) = %v", ref, !ok)
		}
	}
	site, user, _ := parseRef("keepassxc:ssh://root@db01:2222")
	if site != "ssh://db01:2222" || user != "root" {
		t.Errorf("parseRef = %q, %q", site, user)
	}
	for _, c := range []struct {
		got, want string
	}{
		{Ref("ssh", "root", "db01", 22, 22), "keepassxc:ssh://root@db01"},
		{Ref("ssh", "", "db01", 2222, 22), "keepassxc:ssh://db01:2222"},
		{Ref("rdp", "ad\\me", "fe80::1", 3389, 3389), "keepassxc:rdp://ad%5Cme@[fe80::1]"},
	} {
		if c.got != c.want {
			t.Errorf("Ref = %q, want %q", c.got, c.want)
		}
	}
}

func TestIncrement(t *testing.T) {
	n := [24]byte{0xff, 0xff, 1}
	if got := increment(n); got[0] != 0 || got[1] != 0 || got[2] != 2 {
		t.Fatalf("increment = %v", got[:3])
	}
}
