package cliphist

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/zalando/go-keyring"
)

func TestServiceRecordsAndPauses(t *testing.T) {
	keyring.MockInit()
	system := "copied elsewhere"
	notified := 0
	svc := NewService(filepath.Join(t.TempDir(), "clip.db"),
		func() (string, bool) { return system, true },
		func() { notified++ })
	if err := svc.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil {
		t.Fatal(err)
	}
	defer svc.ServiceShutdown()
	if st := svc.Status(); !st.Enabled || st.Error != "" {
		t.Fatalf("status = %+v", st)
	}

	if ok, _ := svc.Add("echo from terminal"); !ok {
		t.Fatal("terminal copy not recorded")
	}
	svc.CaptureSystem()
	svc.SetPaused(true)
	if ok, _ := svc.Add("while paused"); ok {
		t.Fatal("recorded while paused")
	}

	list, _ := svc.List()
	if len(list) != 2 || list[0].Text != "copied elsewhere" || notified != 2 {
		t.Fatalf("list = %+v, notified = %d", list, notified)
	}

	// A second start reuses the key stored in the keyring.
	svc.ServiceShutdown()
	again := NewService(svc.path, func() (string, bool) { return "", false }, func() {})
	again.ServiceStartup(context.Background(), application.ServiceOptions{})
	defer again.ServiceShutdown()
	if list, _ := again.List(); len(list) != 2 {
		t.Fatalf("after restart = %+v", list)
	}
}

func TestServiceDisabledWithoutKeyring(t *testing.T) {
	keyring.MockInitWithError(errors.New("no secret service"))
	svc := NewService(filepath.Join(t.TempDir(), "clip.db"), func() (string, bool) { return "", false }, func() {})
	svc.ServiceStartup(context.Background(), application.ServiceOptions{})
	st := svc.Status()
	if st.Enabled || st.Error == "" {
		t.Fatalf("status = %+v, want disabled with error", st)
	}
	if ok, _ := svc.Add("anything"); ok {
		t.Fatal("recorded without a key")
	}
}
