package hostkeys

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

type fakeNames struct {
	names, files []string
}

func (f fakeNames) HostKeyNames(context.Context, int64, string) ([]string, []string, error) {
	return f.names, f.files, nil
}

func TestForgetOnlyOwnNames(t *testing.T) {
	var removed []string
	s := &Service{
		names: fakeNames{names: []string{"172.16.67.234", "[79.137.113.19]:2222"}, files: []string{"/k1", "/k2"}},
		keygen: func(_ context.Context, name, file string) (bool, error) {
			removed = append(removed, name+" "+file)
			return file == "/k1", nil
		},
	}
	if err := s.Forget(1, "", "github.com"); err == nil {
		t.Fatal("removed a key of an unrelated host")
	}
	if len(removed) != 0 {
		t.Fatalf("ssh-keygen ran for an unrelated host: %v", removed)
	}
	if err := s.Forget(1, "", "[79.137.113.19]:2222"); err != nil {
		t.Fatal(err)
	}
	if len(removed) != 2 || removed[0] != "[79.137.113.19]:2222 /k1" {
		t.Fatalf("removed = %v", removed)
	}
	s.keygen = func(context.Context, string, string) (bool, error) { return false, nil }
	if err := s.Forget(1, "", "172.16.67.234"); err == nil {
		t.Fatal("no error when the key wasn't in any file")
	}
}

func TestRemoveKeyWithSSHKeygen(t *testing.T) {
	if _, err := exec.LookPath("ssh-keygen"); err != nil {
		t.Skip("no ssh-keygen")
	}
	dir := t.TempDir()
	kh := filepath.Join(dir, "known_hosts")
	line := "10.0.0.5 ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl\n"  // gitleaks:allow
	other := "10.0.0.6 ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIOMqqnkVzrm0SdG6UOoqKLsabgH5C9okWi0dh2l9GKJl\n" // gitleaks:allow
	if err := os.WriteFile(kh, []byte(line+other), 0o600); err != nil {
		t.Fatal(err)
	}
	found, err := removeKey(context.Background(), "10.0.0.5", kh)
	if err != nil || !found {
		t.Fatalf("removeKey = %v, %v", found, err)
	}
	data, _ := os.ReadFile(kh)
	if string(data) != other {
		t.Fatalf("known_hosts now %q", data)
	}
	if found, err := removeKey(context.Background(), "10.0.0.9", kh); err != nil || found {
		t.Fatalf("missing host: %v, %v", found, err)
	}
}
