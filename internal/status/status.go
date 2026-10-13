// Package status checks whether saved hosts answer, with the system ping.
//
// ICMP is used rather than a TCP connect to port 22 on purpose: a bare TCP
// connect makes sshd log "Connection closed [preauth]" every time, which
// fills auth logs and can get the user banned by fail2ban.
package status

import (
	"context"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/vgrigolaia/tuxshell/internal/hosts"
)

// EventChanged is emitted with a []Result after each round of checks.
const EventChanged = "hosts:status"

// SlowMS is the round-trip time from which a host counts as degraded.
const SlowMS = 80

// Interval between rounds while checks are on.
const Interval = 60 * time.Second

// Result is one host's status.
type Result struct {
	HostID int64 `json:"hostId"`
	// State is "up", "slow", "down" or "skipped" (behind a jump host, not pinged).
	State string `json:"state"`
	// MS is the round-trip time in milliseconds when up or slow.
	MS      float64 `json:"ms"`
	Checked int64   `json:"checked"` // Unix milliseconds
}

// Lister returns the saved hosts; satisfied by *hosts.Store.
type Lister interface {
	List() ([]hosts.Host, error)
}

// Service pings hosts in the background and reports results.
type Service struct {
	hosts   Lister
	drafts  Drafts
	emit    func([]Result)
	enabled func() bool
	ping    func(ctx context.Context, host string) (float64, bool)

	mu      sync.Mutex
	results map[int64]Result
	kick    chan struct{}
}

// NewService checks hosts from list while enabled() is true and reports
// each round through emit; drafts (may be nil) backs Test.
func NewService(list Lister, drafts Drafts, enabled func() bool, emit func([]Result)) *Service {
	return &Service{hosts: list, drafts: drafts, emit: emit, enabled: enabled, ping: systemPing, results: map[int64]Result{}, kick: make(chan struct{}, 1)}
}

// ServiceStartup starts background checks; Wails cancels ctx at shutdown.
func (s *Service) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	go s.run(ctx)
	return nil
}

// run checks every Interval, or sooner after Refresh, until ctx ends.
func (s *Service) run(ctx context.Context) {
	t := time.NewTicker(Interval)
	defer t.Stop()
	for {
		if s.enabled() {
			s.round(ctx)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-s.kick:
		}
	}
}

// Refresh asks for a new round now (e.g. after hosts were added or checks turned on).
func (s *Service) Refresh() {
	select {
	case s.kick <- struct{}{}:
	default:
	}
}

// Results returns the latest known status of every checked host.
func (s *Service) Results() []Result {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Result, 0, len(s.results))
	for _, r := range s.results {
		out = append(out, r)
	}
	return out
}

// round pings all hosts, at most 8 at a time.
func (s *Service) round(ctx context.Context) {
	list, err := s.hosts.List()
	if err != nil {
		return
	}
	var (
		wg  sync.WaitGroup
		mu  sync.Mutex
		out = make([]Result, 0, len(list))
		sem = make(chan struct{}, 8)
	)
	for _, h := range list {
		r := Result{HostID: h.ID, Checked: time.Now().UnixMilli()}
		target := h.Hostname
		if target == "" {
			target = h.Alias
		}
		// Hosts behind a jump host are usually unreachable directly.
		if h.ProxyJump != "" || !validTarget(target) {
			r.State = "skipped"
			mu.Lock()
			out = append(out, r)
			mu.Unlock()
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(r Result, target string) {
			defer wg.Done()
			defer func() { <-sem }()
			ms, ok := s.ping(ctx, target)
			switch {
			case !ok:
				r.State = "down"
			case ms >= SlowMS:
				r.State, r.MS = "slow", ms
			default:
				r.State, r.MS = "up", ms
			}
			mu.Lock()
			out = append(out, r)
			mu.Unlock()
		}(r, target)
	}
	wg.Wait()
	if ctx.Err() != nil {
		return
	}
	s.mu.Lock()
	s.results = make(map[int64]Result, len(out))
	for _, r := range out {
		s.results[r.HostID] = r
	}
	s.mu.Unlock()
	s.emit(out)
}

// Names and addresses only: never something ping could read as an option.
var targetRe = regexp.MustCompile(`^[A-Za-z0-9._:%\[\]-]{1,253}$`)

func validTarget(t string) bool {
	return t != "" && !strings.HasPrefix(t, "-") && targetRe.MatchString(t)
}

var timeRe = regexp.MustCompile(`time[=<]([0-9.]+) ?ms`)

// systemPing sends one echo request and returns the round-trip time.
func systemPing(ctx context.Context, host string) (float64, bool) {
	ctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "ping", "-c", "1", "-W", "2", "-n", "--", host).Output()
	if err != nil {
		return 0, false
	}
	return parsePing(string(out))
}

func parsePing(out string) (float64, bool) {
	m := timeRe.FindStringSubmatch(out)
	if m == nil {
		return 0, false
	}
	ms, err := strconv.ParseFloat(m[1], 64)
	return ms, err == nil
}
