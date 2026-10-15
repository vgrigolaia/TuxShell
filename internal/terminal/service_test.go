package terminal

import (
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
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
	svc := NewService(rec.emit, nil, nil)

	id, err := svc.start(exec.Command("sh", "-c", "printf 'héllo'; exit 3"), 80, 24, visit{})
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
	svc := NewService(rec.emit, nil, nil)

	// `read` blocks until we write a line; stty reports the PTY size.
	id, err := svc.start(exec.Command("sh", "-c", "read line; stty size; echo \"got:$line\"; sleep 30"), 80, 24, visit{})
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
	svc := NewService(func(string, any) {}, nil, nil)
	if err := svc.Write("nope", "x"); err == nil {
		t.Fatal("expected error for unknown session")
	}
}

func TestConfiguredShellAndLiveCount(t *testing.T) {
	rec := newRecorder()
	svc := NewService(rec.emit, nil, func() string { return "/bin/sh" })
	t.Setenv("SHELL", "/nonexistent")
	id, err := svc.StartShell(80, 24)
	if err != nil {
		t.Fatal(err)
	}
	if n := svc.LiveCount(); n != 1 {
		t.Fatalf("LiveCount = %d, want 1", n)
	}
	svc.CloseAll()
	if n := svc.LiveCount(); n != 0 {
		t.Fatalf("LiveCount after CloseAll = %d, want 0", n)
	}
	if ev := rec.waitExit(t); ev.ID != id {
		t.Fatalf("exit event = %+v, want id %s", ev, id)
	}
}

type fakeAuth struct {
	host  int64
	pid   int
	ended bool
}

func (a *fakeAuth) Begin(_ string, hostID int64) ([]string, func(int), func()) {
	a.host = hostID
	return []string{"TUXSHELL_TEST_AUTH=on"}, func(pid int) { a.pid = pid }, func() { a.ended = true }
}

// Sessions for saved hosts get the auth environment; the hooks see ssh's PID
// and the end of the session.
func TestAuthHooks(t *testing.T) {
	rec := newRecorder()
	svc := NewService(rec.emit, nil, nil)
	auth := &fakeAuth{}
	SetAuth(svc, auth)
	cmd := exec.Command("sh", "-c", "printf \"$TUXSHELL_TEST_AUTH\"")
	if _, err := svc.start(cmd, 80, 24, visit{hostID: 7}); err != nil {
		t.Fatal(err)
	}
	rec.waitExit(t)
	if got := rec.output.String(); got != "on" {
		t.Errorf("env not passed: %q", got)
	}
	if auth.host != 7 || auth.pid != cmd.Process.Pid || !auth.ended {
		t.Errorf("hooks = %+v, want host 7, pid %d, ended", auth, cmd.Process.Pid)
	}
}

type fakeHistory struct {
	begun []visit
	codes chan int
}

func (f *fakeHistory) Begin(hostID int64, user, target string) func(code int) {
	f.begun = append(f.begun, visit{hostID, user, target})
	return func(code int) { f.codes <- code }
}

func TestHistoryRecordsSessions(t *testing.T) {
	rec := newRecorder()
	svc := NewService(rec.emit, nil, nil)
	h := &fakeHistory{codes: make(chan int, 2)}
	SetHistory(svc, h)

	// Local shells aren't recorded; ssh sessions are, with their exit code.
	if _, err := svc.start(exec.Command("sh", "-c", "exit 0"), 80, 24, visit{}); err != nil {
		t.Fatal(err)
	}
	rec.waitExit(t)
	if _, err := svc.start(exec.Command("sh", "-c", "exit 255"), 80, 24, visit{hostID: 4, user: "root"}); err != nil {
		t.Fatal(err)
	}
	rec.waitExit(t)
	select {
	case code := <-h.codes:
		if code != 255 {
			t.Errorf("code = %d", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("end not recorded")
	}
	if len(h.begun) != 1 || h.begun[0] != (visit{4, "root", ""}) {
		t.Errorf("begun = %+v", h.begun)
	}
}

func TestRecording(t *testing.T) {
	rec := newRecorder()
	svc := NewService(rec.emit, nil, nil)
	dir := t.TempDir()
	SetRecordings(svc, dir)
	id, err := svc.start(exec.Command("sh", "-c", "read x; echo out:$x; sleep 0.2"), 80, 24, visit{})
	if err != nil {
		t.Fatal(err)
	}
	name, err := svc.StartRecording(id, "local")
	if err != nil {
		t.Fatal(err)
	}
	_ = svc.Write(id, "secret-typed\n")
	rec.waitExit(t)
	data, _ := os.ReadFile(filepath.Join(dir, name))
	// Output is recorded (the shell echoes the line back), the keystrokes themselves are not a separate stream.
	if !strings.Contains(string(data), "out:secret-typed") || strings.Contains(string(data), `"i"`) {
		t.Errorf("recording = %s", data)
	}
}

func TestOutputIsBatched(t *testing.T) {
	var mu sync.Mutex
	events, size := 0, 0
	exits := make(chan struct{})
	svc := NewService(func(name string, data any) {
		switch ev := data.(type) {
		case DataEvent:
			b, _ := base64.StdEncoding.DecodeString(ev.Data)
			mu.Lock()
			events++
			size += len(b)
			mu.Unlock()
		case ExitEvent:
			close(exits)
		}
	}, nil, nil)

	// About 590 KB that the PTY hands out in thousands of small reads.
	if _, err := svc.start(exec.Command("seq", "1", "100000"), 80, 24, visit{}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-exits:
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for exit event")
	}
	mu.Lock()
	defer mu.Unlock()
	if want := 588895 + 100000; size != want { // "\n" becomes "\r\n" on a PTY
		t.Fatalf("got %d bytes, want %d", size, want)
	}
	t.Logf("%d bytes in %d data events", size, events)
	if events > 200 {
		t.Fatalf("%d data events for one burst, want them batched", events)
	}
}

func TestBatcherKeepsOrderAndLimitsPending(t *testing.T) {
	b := newBatcher()
	var got []byte
	done := make(chan struct{})
	go func() {
		b.run(func(p []byte) { got = append(got, p...) })
		close(done)
	}()
	var want []byte
	chunk := make([]byte, 4096)
	for i := range 200 { // 800 KB, more than maxPending
		for j := range chunk {
			chunk[j] = byte(i)
		}
		b.write(chunk)
		want = append(want, chunk...)
	}
	b.close()
	<-done
	if string(got) != string(want) {
		t.Fatalf("got %d bytes out of order or missing, want %d", len(got), len(want))
	}
}
