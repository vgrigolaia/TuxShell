package files

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"io"
)

// Some servers print text (a banner, "ok") from their login files even for
// commands run over ssh, before the script starts. Each remote script first
// prints a random marker line, and only what comes after it is kept, so that
// text never ends up in a listing or a downloaded file.

// maxNoise caps how much text before the marker is held while looking for it.
const maxNoise = 64 << 10

func newMarker() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "tuxshell-" + hex.EncodeToString(b)
}

// withMarker prints marker on a line of its own, then runs script.
func withMarker(marker, script string) string {
	return "printf '%s\\n' " + marker + "\n" + script
}

// afterMarker passes on only what w receives after the marker line.
type afterMarker struct {
	w      io.Writer
	marker []byte
	held   []byte
	found  bool
}

func newAfterMarker(w io.Writer, marker string) *afterMarker {
	return &afterMarker{w: w, marker: []byte(marker + "\n")}
}

func (a *afterMarker) Write(p []byte) (int, error) {
	if a.found {
		return a.w.Write(p)
	}
	a.held = append(a.held, p...)
	i := bytes.Index(a.held, a.marker)
	if i < 0 {
		// Keep only a tail that could still be the start of the marker.
		if len(a.held) > maxNoise {
			a.held = append([]byte(nil), a.held[len(a.held)-len(a.marker):]...)
		}
		return len(p), nil
	}
	a.found = true
	rest := a.held[i+len(a.marker):]
	a.held = nil
	if len(rest) > 0 {
		if _, err := a.w.Write(rest); err != nil {
			return 0, err
		}
	}
	return len(p), nil
}
