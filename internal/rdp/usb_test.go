package rdp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/vgrigolaia/tuxshell/internal/hosts"
)

func TestListUSB(t *testing.T) {
	root := t.TempDir()
	dev := func(name string, files map[string]string) {
		dir := filepath.Join(root, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for f, v := range files {
			os.WriteFile(filepath.Join(dir, f), []byte(v+"\n"), 0o644)
		}
	}
	dev("usb1", map[string]string{"idVendor": "1d6b", "idProduct": "0002", "bDeviceClass": "09", "product": "Root Hub"})
	dev("1-2", map[string]string{"idVendor": "1050", "idProduct": "0407", "bDeviceClass": "00", "manufacturer": "Yubico", "product": "YubiKey OTP+FIDO+CCID"})
	dev("1-2:1.0", map[string]string{"bInterfaceClass": "03"})
	dev("1-3", map[string]string{"idVendor": "046D", "idProduct": "C52B", "bDeviceClass": "00", "product": "USB Receiver, unifying"})
	dev("1-4", map[string]string{"idVendor": "046d", "idProduct": "c52b", "bDeviceClass": "00"}) // same model twice
	dev("1-5", map[string]string{"idVendor": "abcd", "idProduct": "1234", "bDeviceClass": "00"})

	want := []hosts.USBDevice{
		{ID: "abcd:1234", Name: "USB device"},
		{ID: "046d:c52b", Name: "USB Receiver unifying"},
		{ID: "1050:0407", Name: "Yubico YubiKey OTP+FIDO+CCID"},
	}
	got := listUSB(root)
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%d: got %+v, want %+v", i, got[i], want[i])
		}
	}
	if got := listUSB(filepath.Join(root, "missing")); got == nil || len(got) != 0 {
		t.Fatalf("missing root = %#v", got)
	}
}
