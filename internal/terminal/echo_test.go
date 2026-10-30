package terminal

import (
	"testing"
	"time"
)

func TestEchoTimer(t *testing.T) {
	var e echoTimer
	t0 := time.Now()

	// Output nobody typed for isn't timed.
	e.output(t0)
	if got := e.get(); got.Samples != 0 {
		t.Fatalf("untyped output timed: %+v", got)
	}

	// The first key counts; a second key before the echo doesn't restart the clock.
	e.key(t0)
	e.key(t0.Add(10 * time.Millisecond))
	e.output(t0.Add(40 * time.Millisecond))
	if got := e.get(); got.Samples != 1 || got.LastMs != 40 || got.AvgMs != 40 {
		t.Fatalf("first echo: %+v", got)
	}

	// The average moves an eighth of the way to each new time.
	e.key(t0)
	e.output(t0.Add(120 * time.Millisecond))
	if got := e.get(); got.Samples != 2 || got.LastMs != 120 || got.AvgMs != 50 {
		t.Fatalf("second echo: %+v", got)
	}

	// A key that printed nothing isn't charged for much later output.
	e.key(t0)
	e.output(t0.Add(maxEcho + time.Second))
	if got := e.get(); got.Samples != 2 {
		t.Fatalf("silent key timed: %+v", got)
	}
}
