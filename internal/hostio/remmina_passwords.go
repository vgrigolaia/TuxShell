package hostio

import (
	"bytes"
	"crypto/cipher"
	"crypto/des" //nolint:gosec // Remmina encrypts saved passwords with 3DES; we only read them.
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// Remmina keeps a connection's password in one of two places:
//   - password=. means it's in the system keyring (Remmina's Secret Service
//     plugin), keyed by the connection file's path.
//   - any other value is 3DES-encrypted with the "secret" from remmina.pref.
//
// Passwords found here go into the PasswordRef column like any other import,
// so they're only kept when saving passwords is on, and only in our keyring.

// remminaKeyring opens the system keyring for Remmina lookups. lookup returns
// "" when there's no password; done releases the connection. Swapped in tests.
var remminaKeyring = systemRemminaKeyring

// remminaPasswords reads the passwords of the connections in one Remmina folder.
type remminaPasswords struct {
	secret []byte
	lookup func(file string) string
}

// remminaPrefFile is the remmina.pref that belongs to a connections folder:
// the Flatpak keeps its own, the distribution package uses XDG_CONFIG_HOME.
func remminaPrefFile(dir string) string {
	home, _ := os.UserHomeDir()
	if dir == filepath.Join(home, ".var", "app", "org.remmina.Remmina", "data", "remmina") {
		return filepath.Join(home, ".var", "app", "org.remmina.Remmina", "config", "remmina", "remmina.pref")
	}
	config := os.Getenv("XDG_CONFIG_HOME")
	if config == "" {
		config = filepath.Join(home, ".config")
	}
	return filepath.Join(config, "remmina", "remmina.pref")
}

// remminaSecret reads the key Remmina encrypts passwords with: 24 bytes of
// 3DES key followed by an 8-byte IV, base64 in remmina.pref.
func remminaSecret(pref string) []byte {
	data, err := readFile(pref)
	if err != nil {
		return nil
	}
	for _, l := range textLines(data) {
		if v, ok := strings.CutPrefix(l, "secret="); ok {
			if b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(v)); err == nil && len(b) >= 32 {
				return b
			}
		}
	}
	return nil
}

// decryptRemmina undoes Remmina's password encryption (3DES-CBC, padded
// with zero bytes). ok is false when the value can't be read; a wrong key
// gives bytes that aren't text, which are never taken as a password.
func decryptRemmina(secret []byte, value string) (string, bool) {
	enc, err := base64.StdEncoding.DecodeString(value)
	if err != nil || len(secret) < 32 || len(enc) == 0 || len(enc)%des.BlockSize != 0 {
		return "", false
	}
	block, err := des.NewTripleDESCipher(secret[:24])
	if err != nil {
		return "", false
	}
	out := make([]byte, len(enc))
	cipher.NewCBCDecrypter(block, secret[24:32]).CryptBlocks(out, enc)
	out = bytes.TrimRight(out, "\x00")
	if len(out) == 0 || bytes.IndexByte(out, 0) >= 0 || !utf8.Valid(out) {
		return "", false
	}
	return string(out), true
}

// password returns the connection's password, or "" if there's none or it
// can't be read.
func (p *remminaPasswords) password(file, value string) string {
	switch value {
	case "":
		return ""
	case ".":
		if p.lookup == nil {
			return ""
		}
		return p.lookup(file)
	}
	pw, _ := decryptRemmina(p.secret, value)
	return pw
}
