//go:build !linux

package hostio

// Remmina only runs on Linux, so there's no keyring to look in elsewhere.
func systemRemminaKeyring() (lookup func(file string) string, done func()) {
	return func(string) string { return "" }, func() {}
}
