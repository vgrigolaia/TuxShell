package terminal

import (
	"encoding/base64"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

// recorder collects emitted events and signals when a session exits.
type recorder struct {
	mu     sync.Mutex
	output strings.Builder
	exits  chan ExitEvent
}

func newRecorder() *recorder {
	return &recorder{exits: make(chan ExitEvent, 1)}
}

func (r *recorder) emit(name string, data any) {
	switch ev := data.(type) {
	case DataEvent:
		b, _ := base64.StdEncoding.DecodeString(ev.Data)
		r.mu.Lock()
		r.output.Write(b)
		r.mu.Unlock()
	case ExitEvent:
		r.exits <- ev
	}
}

func (r *recorder) waitExit(t *testing.T) ExitEvent {
	t.Helper()
	select {
	case ev := <-r.exits:
		return ev
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for exit event")
		return ExitEvent{}
	}
}

func TestStartStreamsOutputAndExitCode(t *testing.T) {
	rec := newRecorder()
	svc := NewService(rec.emit)

	id, err := svc.start(exec.Command("sh", "-c", "printf 'héllo'; exit 3"), 80, 24)
	if err != nil {
		t.Fatal(err)
	}

	ev := rec.waitExit(t)
	if ev.ID != id || ev.Code != 3 {
		t.Fatalf("exit event = %+v, want id %s code 3", ev, id)
	}
	if got := rec.output.String(); got != "héllo" {
		t.Fatalf("output = %q, want %q", got, "héllo")
	}
	if _, err := svc.get(id); err == nil {
		t.Fatal("session still registered after exit")
	}
}

func TestWriteResizeAndClose(t *testing.T) {
	rec := newRecorder()
	svc := NewService(rec.emit)

	// `read` blocks until we write a line; stty reports the PTY size.
	id, err := svc.start(exec.Command("sh", "-c", "read line; stty size; echo \"got:$line\"; sleep 30"), 80, 24)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.Resize(id, 100, 40); err != nil {
		t.Fatal(err)
	}
	if err := svc.Write(id, "ping\n"); err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		rec.mu.Lock()
		out := rec.output.String()
		rec.mu.Unlock()
		if strings.Contains(out, "got:ping") {
			if !strings.Contains(out, "40 100") {
				t.Fatalf("output %q missing resized size", out)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out; output so far %q", out)
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := svc.Close(id); err != nil {
		t.Fatal(err)
	}
	rec.waitExit(t)
}

func TestUnknownSession(t *testing.T) {
	svc := NewService(func(string, any) {})
	if err := svc.Write("nope", "x"); err == nil {
		t.Fatal("expected error for unknown session")
	}
}
