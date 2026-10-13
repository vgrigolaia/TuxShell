package tunnels

import (
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vgrigolaia/tuxshell/internal/hosts"
	"github.com/vgrigolaia/tuxshell/internal/sshcmd"
)

type memStore struct{ list map[int64]hosts.Tunnel }

func (m *memStore) ListTunnels() ([]hosts.Tunnel, error) {
	out := []hosts.Tunnel{}
	for _, t := range m.list {
		out = append(out, t)
	}
	return out, nil
}
func (m *memStore) GetTunnel(id int64) (hosts.Tunnel, error) { return m.list[id], nil }
func (m *memStore) SaveTunnel(t hosts.Tunnel) (hosts.Tunnel, error) {
	m.list[t.ID] = t
	return t, nil
}
func (m *memStore) DeleteTunnel(id int64) error { delete(m.list, id); return nil }

// fakeSSH plays ssh: tunnel is the script for the forward-only process,
// masterUp says whether a terminal's shared connection exists.
type fakeSSH struct {
	mu       sync.Mutex
	tunnel   string
	masterUp bool
	ops      []string
}

func (f *fakeSSH) TunnelCommand(int64, sshcmd.Forward) (*exec.Cmd, error) {
	return exec.Command("sh", "-c", f.tunnel), nil
}

func (f *fakeSSH) ControlCommand(_ int64, op string, fw sshcmd.Forward) (*exec.Cmd, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ops = append(f.ops, op+" "+fw[1])
	if f.masterUp {
		return exec.Command("true"), nil
	}
	return exec.Command("false"), nil
}

func setup(script string, master bool) (*Service, *fakeSSH) {
	startWait = 300 * time.Millisecond
	store := &memStore{list: map[int64]hosts.Tunnel{1: {ID: 1, HostID: 7, Kind: "local", BindPort: 15432, TargetHost: "db", TargetPort: 5432}}}
	f := &fakeSSH{tunnel: script, masterUp: master}
	return NewService(store, f, func() {}), f
}

func status(t *testing.T, s *Service) State {
	t.Helper()
	list, err := s.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("List = %v, %v", list, err)
	}
	return list[0]
}

func TestOwnConnection(t *testing.T) {
	s, _ := setup("sleep 30", false)
	if err := s.Start(1); err != nil {
		t.Fatal(err)
	}
	if st := status(t, s); st.Status != "running" || st.Shared {
		t.Fatalf("state = %+v", st)
	}
	s.Stop(1)
	time.Sleep(100 * time.Millisecond)
	if st := status(t, s); st.Status != "stopped" {
		t.Fatalf("after stop = %+v", st)
	}
}

func TestPortInUse(t *testing.T) {
	s, _ := setup(`echo "bind [127.0.0.1]:15432: Address already in use" >&2; exit 255`, false)
	if err := s.Start(1); err == nil {
		t.Fatal("start succeeded")
	}
	if st := status(t, s); st.Status != "failed" || !strings.Contains(st.Message, "already in use") {
		t.Fatalf("state = %+v", st)
	}
}

func TestPasswordHostUsesOpenTerminal(t *testing.T) {
	s, f := setup(`echo "user@db: Permission denied (password)." >&2; exit 255`, true)
	if err := s.Start(1); err != nil {
		t.Fatal(err)
	}
	if st := status(t, s); st.Status != "running" || !st.Shared {
		t.Fatalf("state = %+v", st)
	}
	s.Stop(1)
	f.mu.Lock()
	ops := strings.Join(f.ops, ",")
	f.mu.Unlock()
	want := "check 127.0.0.1:15432:db:5432,forward 127.0.0.1:15432:db:5432,cancel 127.0.0.1:15432:db:5432"
	if ops != want {
		t.Fatalf("control ops = %s, want %s", ops, want)
	}
}

func TestPasswordHostWithoutTerminal(t *testing.T) {
	s, _ := setup(`echo "user@db: Permission denied (password)." >&2; exit 255`, false)
	if err := s.Start(1); err == nil || !strings.Contains(err.Error(), "open a terminal") {
		t.Fatalf("err = %v", err)
	}
}

func TestDisconnectIsReported(t *testing.T) {
	s, _ := setup(`sleep 0.6; echo "Connection to db closed by remote host." >&2; exit 255`, false)
	if err := s.Start(1); err != nil {
		t.Fatal(err)
	}
	time.Sleep(700 * time.Millisecond)
	if st := status(t, s); st.Status != "failed" || !strings.Contains(st.Message, "Disconnected") {
		t.Fatalf("state = %+v", st)
	}
}

func TestForwardArgs(t *testing.T) {
	if f := sshcmd.LocalForward(5432, "fe80::1", 5432); f[1] != "127.0.0.1:5432:[fe80::1]:5432" {
		t.Fatalf("ipv6 forward = %v", f)
	}
	if f := sshcmd.DynamicForward(1080); f != (sshcmd.Forward{"-D", "127.0.0.1:1080"}) {
		t.Fatalf("dynamic = %v", f)
	}
}
