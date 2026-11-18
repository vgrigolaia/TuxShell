package status

import (
	"context"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/vgrigolaia/tuxshell/internal/hosts"
)

type fakeList []hosts.Host

func (f fakeList) List() ([]hosts.Host, error) { return f, nil }

func TestRoundStates(t *testing.T) {
	list := fakeList{
		{ID: 1, Hostname: "fast.example"},
		{ID: 2, Hostname: "slow.example"},
		{ID: 3, Hostname: "gone.example"},
		{ID: 4, Hostname: "10.0.0.11", ProxyJump: "bastion"},
		{ID: 5, Alias: "-oProxyCommand=evil"},
	}
	var got []Result
	s := NewService(list, nil, func() bool { return true }, func(r []Result) { got = r })
	var (
		mu     sync.Mutex
		pinged []string
	)
	s.ping = func(_ context.Context, h string) (float64, bool) {
		mu.Lock()
		pinged = append(pinged, h)
		mu.Unlock()
		switch h {
		case "fast.example":
			return 3, true
		case "slow.example":
			return 120, true
		}
		return 0, false
	}
	s.round(context.Background(), true)

	sort.Slice(got, func(i, j int) bool { return got[i].HostID < got[j].HostID })
	want := []string{"up", "slow", "down", "skipped", "skipped"}
	for i, r := range got {
		if r.State != want[i] {
			t.Errorf("host %d state = %q, want %q", r.HostID, r.State, want[i])
		}
	}
	for _, h := range pinged {
		if h == "10.0.0.11" || h == "-oProxyCommand=evil" {
			t.Errorf("pinged %q, which should be skipped", h)
		}
	}
	if len(s.Results()) != 5 {
		t.Fatalf("Results() = %v", s.Results())
	}
}

func TestParsePing(t *testing.T) {
	for in, want := range map[string]float64{
		"64 bytes from 127.0.0.1: icmp_seq=1 ttl=64 time=0.037 ms": 0.037,
		"64 bytes from 10.0.0.1: icmp_seq=1 ttl=63 time=88.4 ms":   88.4,
		"64 bytes from ::1: icmp_seq=1 ttl=64 time<1ms":            1,
	} {
		if got, ok := parsePing(in); !ok || got != want {
			t.Errorf("parsePing(%q) = %v, %v; want %v", in, got, ok, want)
		}
	}
	if _, ok := parsePing("1 packets transmitted, 0 received"); ok {
		t.Error("parsePing accepted a lost packet")
	}
}

func TestSystemPingLoopback(t *testing.T) {
	if _, ok := systemPing(context.Background(), "127.0.0.1"); !ok {
		t.Skip("ping to 127.0.0.1 not permitted here")
	}
}

func TestExplain(t *testing.T) {
	for in, want := range map[string]string{
		"user@10.0.0.1: Permission denied (publickey,password).":          "no key or agent",
		"ssh: Could not resolve hostname nope: Name or service not known": "resolve",
		"ssh: connect to host 10.0.0.1 port 22: Connection refused":       "refused",
	} {
		if got := explain(in, nil); !strings.Contains(got, want) {
			t.Errorf("explain(%q) = %q, want it to mention %q", in, got, want)
		}
	}
}

func TestRoundBacksOffDownHosts(t *testing.T) {
	list := fakeList{{ID: 1, Hostname: "up.example"}, {ID: 2, Hostname: "gone.example"}}
	s := NewService(list, nil, func() bool { return true }, func([]Result) {})
	var mu sync.Mutex
	count := map[string]int{}
	s.ping = func(_ context.Context, h string) (float64, bool) {
		mu.Lock()
		count[h]++
		mu.Unlock()
		return 1, h == "up.example"
	}
	// Rounds 1–4 (round 1 full); gone.example is down from round 1, so it's
	// pinged in rounds 1 and 2, then left alone until the 5th round.
	s.round(context.Background(), true)
	for range 3 {
		s.round(context.Background(), false)
	}
	if count["up.example"] != 4 || count["gone.example"] != 2 {
		t.Fatalf("after 4 rounds pings = %v, want up 4, gone 2", count)
	}
	for _, r := range s.Results() {
		if r.HostID == 2 && r.State != "down" {
			t.Errorf("skipped host lost its result: %+v", r)
		}
	}
	s.round(context.Background(), false) // 5th: recheck
	if count["gone.example"] != 3 {
		t.Fatalf("5th round didn't recheck: %v", count)
	}
	s.round(context.Background(), true) // Refresh: full
	if count["gone.example"] != 4 {
		t.Fatalf("full round didn't ping a down host: %v", count)
	}
}
