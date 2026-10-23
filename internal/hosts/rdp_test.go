package hosts

import (
	"errors"
	"strings"
	"testing"
)

func TestRDPHost(t *testing.T) {
	s := openTemp(t)
	h, err := s.Save(Host{Name: "win", Hostname: "10.0.5.4", User: "admin", Protocol: "RDP", Tags: []string{},
		RDP: RDP{Domain: "CORP", Folders: []Folder{{Name: "Projects", Path: "/home/me/projects"}}, Gateway: Gateway{Host: "gw.example.com"}}})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := s.Get(h.ID)
	r := got.RDP
	if got.Protocol != ProtocolRDP || got.Port != 3389 || r.Domain != "CORP" || r.Window != "window" || r.Quality != "balanced" ||
		len(r.Folders) != 1 || r.Gateway.Port != 443 {
		t.Fatalf("stored = %+v", got)
	}
	if err := s.SetRDPCert(h.ID, "ab12"); err != nil {
		t.Fatal(err)
	}
	got.RDPCert = "" // Save never changes the pinned certificate
	if _, err := s.Save(got); err != nil {
		t.Fatal(err)
	}
	if again, _ := s.Get(h.ID); again.RDPCert != "ab12" {
		t.Errorf("cert = %q", again.RDPCert)
	}

	// Older rows (no rdp JSON) get the defaults, and SSH hosts stay SSH.
	ssh, _ := s.Save(Host{Name: "lin", Hostname: "10.0.0.1", Tags: []string{}})
	if g, _ := s.Get(ssh.ID); g.Protocol != ProtocolSSH || g.Port != 22 || !g.RDP.Clipboard {
		t.Errorf("ssh host = %+v", g)
	}

	for name, bad := range map[string]Host{
		"comma user":   {User: "a,b"},
		"folder comma": {RDP: RDP{Folders: []Folder{{Name: "x", Path: "/a,b"}}}},
		"folder rel":   {RDP: RDP{Folders: []Folder{{Name: "x", Path: "rel"}}}},
		"folder name":  {RDP: RDP{Folders: []Folder{{Name: "a,b", Path: "/a"}}}},
		"folder dup":   {RDP: RDP{Folders: []Folder{{Name: "x", Path: "/a"}, {Name: "X", Path: "/b"}}}},
		"tiny size":    {RDP: RDP{Size: "fixed", Width: 10, Height: 10}},
		"domain":       {RDP: RDP{Domain: "a b"}},
		"gateway":      {RDP: RDP{Gateway: Gateway{Host: "-evil"}}},
		"gateway user": {RDP: RDP{Gateway: Gateway{Host: "gw", User: "a,b"}}},
		"no host":      {Hostname: "-"},
		"bad protocol": {Protocol: "vnc"},
	} {
		h := Host{Name: name, Hostname: "10.0.5.9", Protocol: ProtocolRDP, Tags: []string{}}
		if bad.User != "" {
			h.User = bad.User
		}
		if bad.Hostname != "" {
			h.Hostname = bad.Hostname
		}
		if bad.Protocol != "" {
			h.Protocol = bad.Protocol
		}
		h.RDP = DefaultRDP()
		if bad.RDP.Folders != nil {
			h.RDP.Folders = bad.RDP.Folders
		}
		if bad.RDP.Size != "" {
			h.RDP.Size, h.RDP.Width, h.RDP.Height = bad.RDP.Size, bad.RDP.Width, bad.RDP.Height
		}
		if bad.RDP.Domain != "" {
			h.RDP.Domain = bad.RDP.Domain
		}
		if bad.RDP.Gateway.Host != "" {
			h.RDP.Gateway = bad.RDP.Gateway
		}
		var ve *ValidationError
		if _, err := s.Save(h); !errors.As(err, &ve) {
			t.Errorf("%s: saved (%v)", name, err)
		}
	}
}

func TestRDPPhase2Options(t *testing.T) {
	s := openTemp(t)
	base := func() Host {
		return Host{Name: "win", Hostname: "10.0.5.4", Protocol: ProtocolRDP, Tags: []string{}, RDP: DefaultRDP()}
	}
	h := base()
	h.RDP.Printers, h.RDP.SmartCards = true, true
	h.RDP.USB = []USBDevice{{ID: " 046D:C52B ", Name: "Receiver"}, {ID: ""}}
	h.RDP.RemoteApp = RemoteApp{Program: " ||notepad ", Args: " a b "}
	saved, err := s.Save(h)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := s.Get(saved.ID)
	r := got.RDP
	if !r.Printers || !r.SmartCards || len(r.USB) != 1 || r.USB[0].ID != "046d:c52b" || r.RemoteApp != (RemoteApp{Program: "||notepad", Args: "a b"}) {
		t.Fatalf("stored = %+v", r)
	}
	// Only a name or arguments without a program is no RemoteApp.
	h = base()
	h.Name = "win2"
	h.RDP.RemoteApp = RemoteApp{Name: "x"}
	if saved, err = s.Save(h); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Get(saved.ID); got.RDP.RemoteApp != (RemoteApp{}) || got.RDP.USB == nil {
		t.Fatalf("empty remote app = %+v", got.RDP)
	}

	for name, set := range map[string]func(*RDP){
		"usb id":    func(r *RDP) { r.USB = []USBDevice{{ID: "46d:c52b"}} },
		"usb comma": func(r *RDP) { r.USB = []USBDevice{{ID: "046d:c52b", Name: "a,b"}} },
		"usb many": func(r *RDP) {
			r.USB = make([]USBDevice, 11)
			for i := range r.USB {
				r.USB[i].ID = "046d:c52b"
			}
		},
		"app comma": func(r *RDP) { r.RemoteApp = RemoteApp{Program: `C:\a,b.exe`} },
		"app args":  func(r *RDP) { r.RemoteApp = RemoteApp{Program: "||x", Args: "a,b"} },
		"app break": func(r *RDP) { r.RemoteApp = RemoteApp{Program: "||x\n/v:evil"} },
	} {
		h := base()
		h.Name = name
		set(&h.RDP)
		var ve *ValidationError
		if _, err := s.Save(h); !errors.As(err, &ve) || !strings.HasPrefix(ve.Field, "rdp.") {
			t.Errorf("%s: saved or wrong field (%v)", name, err)
		}
	}
}
