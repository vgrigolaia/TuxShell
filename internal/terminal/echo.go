package terminal

import (
	"sync"
	"sync/atomic"
	"time"
)

// maxEcho is the longest wait still counted as a key's echo. Keys that print
// nothing (a password, Ctrl+key) would otherwise time the next unrelated output.
const maxEcho = 3 * time.Second

// Echo is how long typed keys took to come back as output: the round trip to
// the server and back for ssh, or the local program's reply for a shell.
// It doesn't include drawing in the window.
type Echo struct {
	// LastMs is the latest key's time, AvgMs a moving average of recent keys.
	LastMs float64 `json:"lastMs"`
	AvgMs  float64 `json:"avgMs"`
	// Samples is how many keys were timed.
	Samples int `json:"samples"`
}

// echoTimer times the first key typed after the previous echo until the next
// output. Called from Write (key) and the PTY reader (output).
type echoTimer struct {
	typed atomic.Int64 // UnixNano of the pending key, 0 when none
	mu    sync.Mutex
	stats Echo
}

func (e *echoTimer) key(now time.Time) { e.typed.CompareAndSwap(0, now.UnixNano()) }

func (e *echoTimer) output(now time.Time) {
	t := e.typed.Swap(0)
	if t == 0 {
		return
	}
	d := now.Sub(time.Unix(0, t))
	if d < 0 || d > maxEcho {
		return
	}
	ms := float64(d.Microseconds()) / 1000
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.stats.Samples == 0 {
		e.stats.AvgMs = ms
	} else {
		e.stats.AvgMs += (ms - e.stats.AvgMs) / 8
	}
	e.stats.LastMs = ms
	e.stats.Samples++
}

func (e *echoTimer) get() Echo {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.stats
}
