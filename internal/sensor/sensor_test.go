package sensor

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

func TestHwmon(t *testing.T) {
	root := sysTree(t, map[string]string{
		"hwmon0/name": "nvme\n", "hwmon0/temp1_input": "40850\n",
		"hwmon1/name": "amdgpu\n", "hwmon1/temp1_input": "43000\n",
		"hwmon3/name": "k10temp\n", "hwmon3/temp1_input": "48750\n",
		"hwmon4/name": "broken\n", // no temp1_input
		"hwmon5/name": "junk\n", "hwmon5/temp1_input": "not a number\n",
		"hwmon6/name": "empty\n", "hwmon6/temp1_input": "",
		"hwmon7/name": "cold\n", "hwmon7/temp1_input": "-5000\n",
		"hwmon8/name": "long\n", "hwmon8/temp1_input": strings.Repeat("9", 100) + "\n",
	})
	cpu := hwmon(root, "k10temp", "coretemp", "zenpower")
	if cpu == nil {
		t.Fatal("k10temp not found")
	}
	if v, err := cpu(); v != 488 || err != nil {
		t.Errorf("k10temp = %d, %v; want 488 (48.75 rounded)", v, err)
	}
	// the file is read again every time
	if err := os.WriteFile(filepath.Join(root, "hwmon3/temp1_input"), []byte("51200\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if v, err := cpu(); v != 512 || err != nil {
		t.Errorf("after a change k10temp = %d, %v; want 512", v, err)
	}
	if v, err := hwmon(root, "amdgpu", "nouveau")(); v != 430 || err != nil {
		t.Errorf("amdgpu = %d, %v; want 430", v, err)
	}
	if v, err := hwmon(root, "cold")(); v != -49 || err != nil {
		t.Errorf("below zero = %d, %v; want -49", v, err)
	}
	if v, err := hwmon(root, "long")(); v < 999 || err != nil {
		t.Errorf("an absurdly large value = %d, %v; want a large number, not a crash", v, err)
	}
	for _, name := range []string{"junk", "empty"} {
		if v, err := hwmon(root, name)(); err == nil || errors.Is(err, ErrNotReady) {
			t.Errorf("%s: an unreadable value gave %d, %v; want a lasting error", name, v, err)
		}
	}
	if hwmon(root, "coretemp") != nil || hwmon(root, "broken") != nil || hwmon(root) != nil {
		t.Error("a chip that is absent or has no temp1_input should give no sensor")
	}
	if hwmon(filepath.Join(root, "missing"), "k10temp") != nil {
		t.Error("a missing directory should give no sensor")
	}
}

func TestFollow(t *testing.T) {
	r, w := io.Pipe()
	gpu := follow(r, "nvidia-smi")
	if v, err := gpu(); !errors.Is(err, ErrNotReady) {
		t.Errorf("before any line: %d, %v; want ErrNotReady", v, err)
	}
	expect := func(lines string, want int) {
		t.Helper()
		io.WriteString(w, lines)
		for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(time.Millisecond) {
			if v, err := gpu(); err == nil && v == want {
				return
			}
		}
		v, err := gpu()
		t.Fatalf("after %q: %d, %v; want %d", lines, v, err, want)
	}
	expect("40\n", 400)
	expect(" 41 \n", 410)
	expect("garbage\nN/A\n\n1000\n-300\n99999999999999999999\n42\n", 420) // bad lines are skipped
	expect("0\n", 0)
	expect("-5\n", -50)

	w.Close()
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(time.Millisecond) {
		_, err := gpu()
		if err != nil && !errors.Is(err, ErrNotReady) {
			if !strings.Contains(err.Error(), "nvidia-smi stopped") {
				t.Errorf("error after the input ended = %v", err)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal(fmt.Sprint("no error after the input ended: ", err))
		}
	}
}

func TestFollowEndsWithoutReading(t *testing.T) {
	gpu := follow(strings.NewReader(""), "nvidia-smi")
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(time.Millisecond) {
		if _, err := gpu(); err != nil && !errors.Is(err, ErrNotReady) {
			return
		}
	}
	t.Fatal("a source that ends at once should report an error, not stay 'not ready'")
}
