package display

import (
	"os"
	"path/filepath"
	"testing"
)

// sysTree writes files under a fresh directory: path -> content.
func sysTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestFind(t *testing.T) {
	const other = "DRIVER=hid-generic\nHID_ID=0003:00001532:00000088\nHID_NAME=Mouse\n"
	const atlas = "DRIVER=hid-generic\nHID_ID=0003:00001B80:0000B538\nHID_NAME=ATLAS Digital Display\n"
	if got := find(sysTree(t, map[string]string{
		"hidraw0/device/uevent":  other,
		"hidraw13/device/uevent": atlas,
		"hidraw2/device/uevent":  other,
	})); got != "hidraw13" {
		t.Errorf("find = %q, want hidraw13", got)
	}
	for name, files := range map[string]map[string]string{
		"no devices":      {},
		"other devices":   {"hidraw0/device/uevent": other},
		"longer id":       {"hidraw0/device/uevent": "HID_ID=0003:00001B80:0000B5380\n"},
		"id inside value": {"hidraw0/device/uevent": "HID_NAME=x HID_ID=0003:00001B80:0000B538\n"},
		"wrong directory": {"input5/device/uevent": atlas},
		"missing uevent":  {"hidraw0/device/other": atlas},
	} {
		if got := find(sysTree(t, files)); got != "" {
			t.Errorf("%s: find = %q, want nothing", name, got)
		}
	}
	if got := find(filepath.Join(t.TempDir(), "missing")); got != "" {
		t.Errorf("find in a missing directory = %q", got)
	}
}
