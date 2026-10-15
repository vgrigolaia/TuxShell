package terminal

import (
	"sync"
	"time"
)

// frameInterval is the shortest gap between two data events of one session.
// A PTY hands out output in tiny reads (seq 1 1000000 is ~270k reads of ~16
// bytes), and every event is a JavaScript call on the UI thread, so output is
// sent at most once per frame. The first chunk after a quiet spell (a typed
// key's echo) still goes out at once.
const frameInterval = 8 * time.Millisecond

// maxPending is how much unsent output a session may hold before its reader
// waits, so a flood (cat /dev/urandom) can't grow memory without bound.
const maxPending = 256 * 1024

// batcher gathers PTY output between sends. write is called by the reader,
// run by the sender; close ends run once everything written has been sent.
type batcher struct {
	mu     sync.Mutex
	full   *sync.Cond
	buf    []byte
	spare  []byte
	closed bool
	ready  chan struct{}
}

func newBatcher() *batcher {
	b := &batcher{ready: make(chan struct{}, 1)}
	b.full = sync.NewCond(&b.mu)
	return b
}

func (b *batcher) wake() {
	select {
	case b.ready <- struct{}{}:
	default:
	}
}

// write queues p for the next send, waiting while too much is unsent.
func (b *batcher) write(p []byte) {
	b.mu.Lock()
	for len(b.buf) >= maxPending {
		b.full.Wait()
	}
	b.buf = append(b.buf, p...)
	b.mu.Unlock()
	b.wake()
}

// close tells run that nothing more will be written.
func (b *batcher) close() {
	b.mu.Lock()
	b.closed = true
	b.mu.Unlock()
	b.wake()
}

// run calls send with each batch until close, then returns. send must not
// keep the slice.
func (b *batcher) run(send func([]byte)) {
	for {
		<-b.ready
		b.mu.Lock()
		data := b.buf
		b.buf = b.spare[:0]
		closed := b.closed
		b.full.Broadcast()
		b.mu.Unlock()

		if len(data) > 0 {
			send(data)
		}
		b.mu.Lock()
		b.spare = data
		b.mu.Unlock()
		if closed {
			return
		}
		if len(data) > 0 {
			time.Sleep(frameInterval)
		}
	}
}
