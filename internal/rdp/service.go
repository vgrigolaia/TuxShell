package rdp

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/vgrigolaia/tuxshell/internal/hosts"
	"github.com/vgrigolaia/tuxshell/internal/sshcmd"
)

// EventChanged is emitted when a session starts, changes state or ends.
const EventChanged = "rdp:changed"

// Session is one remote desktop window.
type Session struct {
	ID     string `json:"id"`
	HostID int64  `json:"hostId"`
	Name   string `json:"name"`
	// State is "connecting", "open" or "ended".
	State   string `json:"state"`
	Started int64  `json:"started"` // Unix milliseconds
	Ended   int64  `json:"ended"`
	// Error explains why it ended, in plain words ("" for a normal end).
	Error string `json:"error"`
	// Tunnel is "via <jump host>" when it goes through SSH.
	Tunnel string `json:"tunnel"`
}

// CertError stops a connection until the user trusts the certificate:
// Kind is "new" (first connection) or "changed" (it differs from the one
// trusted before). Connect again with Trust set to the fingerprint.
type CertError struct {
	Kind     string      `json:"kind"`
	Cert     Certificate `json:"cert"`
	Previous string      `json:"previous"`
}

func (e *CertError) Error() string {
	if e.Kind == "changed" {
		return "the remote desktop's certificate changed"
	}
	return "this remote desktop's certificate isn't trusted yet"
}

// ConnectOptions come from the password box and the certificate prompt.
type ConnectOptions struct {
	// Password typed by the user; empty uses the saved one, if any.
	Password string `json:"password"`
	// Remember saves the typed password in the keyring.
	Remember bool `json:"remember"`
	// Trust is a fingerprint the user accepted in the certificate prompt.
	Trust string `json:"trust"`
}

// Passwords reads and saves host passwords (the saved-passwords feature).
type Passwords interface {
	Saved(hostID int64) (string, bool)
	Save(hostID int64, password string) error
}

type running struct {
	info   Session
	cmd    *exec.Cmd
	tunnel *exec.Cmd
	log    []string
}

// Service opens remote desktops. Its exported methods are bound to the frontend.
type Service struct {
	store     *hosts.Store
	tunnels   tunneler
	passwords Passwords
	prefer    func() string
	emit      func(name string, data any)

	pickFolder func() (string, error)
	pickFiles  func() ([]string, error)
	relay      *Relay

	seq      atomic.Uint64
	mu       sync.Mutex
	sessions map[string]*running
}

type tunneler interface {
	JumpTunnelCommand(jump string, f sshcmd.Forward) (*exec.Cmd, error)
}

// NewService returns the RDP service. prefer says which FreeRDP client to use
// ("auto", "x11" or "sdl").
func NewService(store *hosts.Store, tunnels tunneler, passwords Passwords, prefer func() string, emit func(string, any)) *Service {
	return &Service{store: store, tunnels: tunnels, passwords: passwords, prefer: prefer, emit: emit, sessions: map[string]*running{}}
}

// ClientInfo says which FreeRDP is used, or how to install it.
type ClientInfo struct {
	Installed bool   `json:"installed"`
	Client    Client `json:"client"`
	Hint      string `json:"hint"`
}

// UseFolderPicker sets the dialog that chooses folders to share. A function,
// so it isn't bound.
func UseFolderPicker(s *Service, pick func() (string, error)) { s.pickFolder = pick }

// ChooseFolder asks for a local folder to share ("" if cancelled).
func (s *Service) ChooseFolder() (string, error) {
	if s.pickFolder == nil {
		return "", errors.New("folder picker unavailable")
	}
	return s.pickFolder()
}

// Client reports the FreeRDP client that would be used.
func (s *Service) Client() ClientInfo {
	c, err := FindClient(s.prefer())
	if err != nil {
		return ClientInfo{Hint: InstallHint}
	}
	return ClientInfo{Installed: true, Client: c}
}

// prepared is a host ready to connect: tunnel open and certificate checked.
type prepared struct {
	host        hosts.Host
	server      string
	port        int
	tunnel      *exec.Cmd
	fingerprint string
}

func (p *prepared) stop() {
	if p.tunnel != nil && p.tunnel.Process != nil {
		_ = p.tunnel.Process.Kill()
		_ = p.tunnel.Wait()
	}
}

// prepare loads an RDP host, opens its SSH tunnel and checks its
// certificate (a CertError asks the user; trust is a fingerprint they accepted).
func (s *Service) prepare(ctx context.Context, hostID int64, trust string) (*prepared, error) {
	h, err := s.store.Get(hostID)
	if err != nil {
		return nil, err
	}
	h.Normalize()
	if err := h.Validate(); err != nil {
		return nil, err
	}
	if h.Protocol != hosts.ProtocolRDP {
		return nil, errors.New("not a remote desktop host")
	}
	p := &prepared{host: h, server: h.Hostname, port: h.Port}

	// Through a jump host: a local SSH tunnel to the desktop's port.
	if h.ProxyJump != "" {
		// A hop may name a saved SSH host rather than an ssh_config alias.
		if list, err := s.store.List(); err == nil {
			byName := map[string]hosts.Host{}
			for _, x := range list {
				byName[strings.ToLower(x.Name)] = x
			}
			h.ProxyJump = hosts.ResolveJump(h.Name, h.ProxyJump, byName)
			p.host.ProxyJump = h.ProxyJump
		}
		local, err := freePort()
		if err != nil {
			return nil, err
		}
		tunnel, err := s.tunnels.JumpTunnelCommand(h.ProxyJump, sshcmd.LocalForward(local, h.Hostname, h.Port))
		if err != nil {
			return nil, err
		}
		if err := startTunnel(ctx, tunnel, local); err != nil {
			return nil, err
		}
		p.tunnel, p.server, p.port = tunnel, "127.0.0.1", local
	}

	// The certificate, checked by TuxShell (FreeRDP can't remember it
	// through a tunnel, whose local port changes each time). A gateway
	// stands between, so then FreeRDP checks it itself.
	if h.RDP.Gateway.Host == "" {
		pctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		cert, err := FetchCertificate(pctx, net.JoinHostPort(p.server, strconv.Itoa(p.port)), h.Hostname)
		cancel()
		switch {
		case errors.Is(err, ErrNoTLS):
			// Old RDP security has no certificate; FreeRDP decides.
		case err != nil:
			p.stop()
			return nil, explainDial(err, p.tunnel != nil)
		case cert.Fingerprint == h.RDPCert:
			p.fingerprint = cert.Fingerprint
		case trust == cert.Fingerprint:
			if err := s.store.SetRDPCert(h.ID, cert.Fingerprint); err != nil {
				p.stop()
				return nil, err
			}
			p.fingerprint = cert.Fingerprint
		default:
			p.stop()
			kind := "new"
			if h.RDPCert != "" {
				kind = "changed"
			}
			return nil, &CertError{Kind: kind, Cert: cert, Previous: h.RDPCert}
		}
	}
	return p, nil
}

// Connect opens a remote desktop for a saved RDP host in FreeRDP's window.
func (s *Service) Connect(ctx context.Context, hostID int64, opts ConnectOptions) (Session, error) {
	client, err := FindClient(s.prefer())
	if err != nil {
		return Session{}, fmt.Errorf("%w. %s", err, InstallHint)
	}
	p, err := s.prepare(ctx, hostID, opts.Trust)
	if err != nil {
		return Session{}, err
	}
	h, server, port, tunnel, fingerprint, stop := p.host, p.server, p.port, p.tunnel, p.fingerprint, p.stop

	password := opts.Password
	if password == "" {
		if saved, ok := s.passwords.Saved(h.ID); ok {
			password = saved
		}
	} else if opts.Remember {
		if err := s.passwords.Save(h.ID, password); err != nil {
			stop()
			return Session{}, err
		}
	}

	args, err := Args(h, Connection{Server: server, Port: port, Password: password, Fingerprint: fingerprint, Title: h.Name + " · TuxShell"})
	if err != nil {
		stop()
		return Session{}, err
	}
	cmd := exec.Command(client.Path, "/args-from:stdin")
	cmd.Env = append(os.Environ(), "WLOG_LEVEL=WARN")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		stop()
		return Session{}, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		stop()
		return Session{}, err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		stop()
		return Session{}, err
	}
	// All options, the password too, go through the pipe and nowhere else.
	_, werr := io.WriteString(stdin, strings.Join(args, "\n")+"\n")
	_ = stdin.Close()
	if werr != nil {
		_ = cmd.Process.Kill()
		stop()
		return Session{}, werr
	}

	r := &running{cmd: cmd, tunnel: tunnel, info: Session{
		ID: strconv.FormatUint(s.seq.Add(1), 10), HostID: h.ID, Name: h.Name, State: "connecting", Started: time.Now().UnixMilli(),
	}}
	if tunnel != nil {
		r.info.Tunnel = "via " + h.ProxyJump
	}
	s.mu.Lock()
	s.sessions[r.info.ID] = r
	s.mu.Unlock()
	s.changed()
	go s.watch(r, out)
	return r.info, nil
}

// watch keeps FreeRDP's last log lines and reports the end.
func (s *Service) watch(r *running, out io.Reader) {
	opened := time.AfterFunc(4*time.Second, func() { s.setState(r, "open") })
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for sc.Scan() {
		s.mu.Lock()
		r.log = append(r.log, sc.Text())
		if len(r.log) > 200 {
			r.log = r.log[len(r.log)-200:]
		}
		s.mu.Unlock()
	}
	err := r.cmd.Wait()
	opened.Stop()
	if r.tunnel != nil && r.tunnel.Process != nil {
		_ = r.tunnel.Process.Kill()
		_ = r.tunnel.Wait()
	}
	s.mu.Lock()
	r.info.State, r.info.Ended = "ended", time.Now().UnixMilli()
	r.info.Error = explain(r.log, err)
	s.mu.Unlock()
	s.changed()
}

func (s *Service) setState(r *running, state string) {
	s.mu.Lock()
	if r.info.State == "connecting" {
		r.info.State = state
	}
	s.mu.Unlock()
	s.changed()
}

func (s *Service) changed() {
	if s.emit != nil {
		s.emit(EventChanged, nil)
	}
}

// Sessions lists the remote desktops opened since the app started.
func (s *Service) Sessions() []Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	list := make([]Session, 0, len(s.sessions))
	for _, r := range s.sessions {
		list = append(list, r.info)
	}
	return list
}

// Log returns FreeRDP's last messages for a session ("Show details").
func (s *Service) Log(id string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r, ok := s.sessions[id]; ok {
		return append([]string{}, r.log...)
	}
	return []string{}
}

// Disconnect closes a remote desktop window.
func (s *Service) Disconnect(id string) error {
	s.mu.Lock()
	r, ok := s.sessions[id]
	s.mu.Unlock()
	if !ok || r.info.State == "ended" {
		return nil
	}
	return r.cmd.Process.Signal(syscall.SIGTERM)
}

// Forget removes an ended session from the list.
func (s *Service) Forget(id string) {
	s.mu.Lock()
	if r, ok := s.sessions[id]; ok && r.info.State == "ended" {
		delete(s.sessions, id)
	}
	s.mu.Unlock()
	s.changed()
}

// ServiceShutdown closes every remote desktop when the app quits.
func (s *Service) ServiceShutdown() error {
	if s.relay != nil {
		_ = s.relay.Close()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.sessions {
		if r.info.State != "ended" {
			_ = r.cmd.Process.Signal(syscall.SIGTERM)
		}
	}
	return nil
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// startTunnel runs ssh and waits until its local port accepts connections.
func startTunnel(ctx context.Context, cmd *exec.Cmd, port int) error {
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	deadline := time.After(20 * time.Second)
	for {
		select {
		case <-exited:
			msg := strings.TrimSpace(stderr.String())
			if i := strings.LastIndex(msg, "\n"); i >= 0 {
				msg = msg[i+1:]
			}
			if msg == "" || strings.Contains(msg, "Permission denied") {
				msg = "the jump host needs a key or the ssh agent (password logins can't open tunnels)"
			}
			return fmt.Errorf("couldn't open the SSH tunnel: %s", msg)
		case <-deadline:
			_ = cmd.Process.Kill()
			return errors.New("the SSH tunnel didn't open in 20 seconds")
		case <-ctx.Done():
			_ = cmd.Process.Kill()
			return ctx.Err()
		case <-time.After(150 * time.Millisecond):
			if c, err := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(port), time.Second); err == nil {
				c.Close()
				// Keep reaping the process after it ends.
				go func() { <-exited }()
				return nil
			}
		}
	}
}

// explainDial turns a failed certificate check into a reason.
func explainDial(err error, tunneled bool) error {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "no such host"):
		return errors.New("the host name couldn't be found")
	case strings.Contains(msg, "connection refused"):
		if tunneled {
			return errors.New("the jump host couldn't reach the remote desktop port (is RDP enabled there?)")
		}
		return errors.New("the server refused the connection (is Remote Desktop enabled and the port open?)")
	case strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline"):
		return errors.New("the server didn't answer (check the address, port and firewall)")
	case strings.Contains(msg, "not an RDP server"):
		return errors.New("something answered on that port, but it isn't Remote Desktop")
	}
	return fmt.Errorf("couldn't check the server's certificate: %w", err)
}

// reasons maps FreeRDP's error names to plain words.
var reasons = []struct{ code, text string }{
	{"ERRCONNECT_LOGON_FAILURE", "Wrong user name or password."},
	{"ERRCONNECT_AUTHENTICATION_FAILED", "Wrong user name or password."},
	{"ERRCONNECT_PASSWORD_EXPIRED", "The password has expired. Change it on the server first."},
	{"ERRCONNECT_PASSWORD_CERTAINLY_EXPIRED", "The password has expired. Change it on the server first."},
	{"ERRCONNECT_PASSWORD_MUST_CHANGE", "The password must be changed before you can log in."},
	{"ERRCONNECT_ACCOUNT_LOCKED_OUT", "The account is locked."},
	{"ERRCONNECT_ACCOUNT_DISABLED", "The account is disabled."},
	{"ERRCONNECT_ACCOUNT_EXPIRED", "The account has expired."},
	{"ERRCONNECT_ACCOUNT_RESTRICTION", "This account isn't allowed to log in now."},
	{"ERRCONNECT_LOGON_TYPE_NOT_GRANTED", "This user isn't allowed to use Remote Desktop."},
	{"ERRCONNECT_INSUFFICIENT_PRIVILEGES", "This user isn't allowed to use Remote Desktop."},
	{"ERRCONNECT_NO_OR_MISSING_CREDENTIALS", "The server needs a user name and password."},
	{"ERRCONNECT_DNS_NAME_NOT_FOUND", "The host name couldn't be found."},
	{"ERRCONNECT_DNS_ERROR", "The host name couldn't be found."},
	{"ERRCONNECT_CONNECT_TRANSPORT_FAILED", "Couldn't reach the server."},
	{"ERRCONNECT_CONNECT_FAILED", "Couldn't reach the server."},
	{"ERRCONNECT_SECURITY_NEGO_CONNECT_FAILED", "The server and FreeRDP couldn't agree on how to secure the connection."},
	{"ERRCONNECT_TLS_CONNECT_FAILED", "The secure (TLS) connection failed."},
	{"ERRCONNECT_KDC_UNREACHABLE", "The domain controller couldn't be reached (Kerberos)."},
	{"ERRCONNECT_ACTIVATION_TIMEOUT", "The server stopped answering while starting the session."},
	{"ERRINFO_DISCONNECTED_BY_OTHERCONNECTION", "Someone else connected to this session."},
	{"ERRINFO_LOGOFF_BY_USER", ""},
	{"ERRINFO_RPC_INITIATED_DISCONNECT_BYUSER", ""},
	{"ERRINFO_RPC_INITIATED_LOGOFF", ""},
	{"ERRINFO_IDLE_TIMEOUT", "Disconnected after being idle."},
	{"ERRINFO_LOGON_TIMEOUT", "Logging in took too long."},
	{"ERRINFO_SERVER_INSUFFICIENT_PRIVILEGES", "This user isn't allowed to use Remote Desktop."},
	{"ERRINFO_SERVER_FRESH_CREDENTIALS_REQUIRED", "The server wants you to log in again."},
	{"CERTIFICATE NAME MISMATCH", "The server's certificate didn't match."},
	{"Certificate verification failure", "The server's certificate didn't match."},
}

// explain says why FreeRDP ended, from its log; "" for a normal end.
func explain(log []string, err error) string {
	joined := strings.Join(log, "\n")
	for _, r := range reasons {
		if strings.Contains(joined, r.code) {
			return r.text
		}
	}
	var exit *exec.ExitError
	if err == nil || (errors.As(err, &exit) && (exit.ExitCode() <= 2 || exit.ExitCode() == 11 || exit.ExitCode() == 12)) {
		return "" // closed, disconnected or logged off
	}
	if errors.As(err, &exit) && !exit.Exited() {
		return "" // closed from TuxShell
	}
	return "FreeRDP stopped with an error. Show details for its messages."
}
