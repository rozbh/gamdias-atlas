// Package sensor reads the CPU and GPU temperatures.
package sensor

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
)

const hwmonRoot = "/sys/class/hwmon"

// ErrNotReady means the sensor has no reading yet. Any other error from a
// Sensor means it is gone.
var ErrNotReady = errors.New("no reading yet")

// Sensor reads a temperature in tenths of a degree C.
type Sensor func() (tenths int, err error)

// CPU returns the CPU temperature sensor, or nil if there is none it knows.
func CPU() Sensor {
	return hwmon(hwmonRoot, "k10temp", "coretemp", "zenpower")
}

// GPU returns the GPU temperature sensor, or nil if there is none it knows.
// With an NVIDIA driver it starts nvidia-smi, which keeps running.
func GPU() Sensor {
	if path, err := exec.LookPath("nvidia-smi"); err == nil {
		cmd := exec.Command(path, "-i", "0", "-l", "1",
			"--query-gpu=temperature.gpu", "--format=csv,noheader,nounits")
		if out, err := cmd.StdoutPipe(); err == nil && cmd.Start() == nil {
			return follow(out, "nvidia-smi")
		}
	}
	return hwmon(hwmonRoot, "amdgpu", "nouveau")
}

// hwmon returns the sensor of the first chip under root with one of the names, or nil.
func hwmon(root string, names ...string) Sensor {
	dirs, _ := filepath.Glob(filepath.Join(root, "hwmon*"))
	for _, d := range dirs {
		name, _ := os.ReadFile(filepath.Join(d, "name"))
		if !slices.Contains(names, strings.TrimSpace(string(name))) {
			continue
		}
		f, err := os.Open(filepath.Join(d, "temp1_input"))
		if err != nil {
			continue
		}
		return func() (int, error) {
			var buf [16]byte // millidegrees and a newline
			n, _ := f.ReadAt(buf[:], 0)
			milli, err := strconv.Atoi(strings.TrimSpace(string(buf[:n])))
			if err != nil {
				return 0, fmt.Errorf("%s: unreadable value %q", f.Name(), buf[:n])
			}
			return (milli + 50) / 100, nil
		}
	}
	return nil
}

// follow returns the latest whole-degree reading that r prints, one per line.
// source names r in the error given once r has ended.
func follow(r io.Reader, source string) Sensor {
	const none = -1 << 62
	var (
		latest atomic.Int64
		ended  atomic.Bool
	)
	latest.Store(none)
	go func() {
		for sc := bufio.NewScanner(r); sc.Scan(); {
			n, err := strconv.Atoi(strings.TrimSpace(sc.Text()))
			if err == nil && n >= -273 && n <= 999 {
				latest.Store(int64(n) * 10)
			}
		}
		ended.Store(true)
	}()
	return func() (int, error) {
		t := latest.Load()
		switch {
		case ended.Load():
			return 0, fmt.Errorf("%s stopped", source)
		case t == none:
			return 0, ErrNotReady
		}
		return int(t), nil
	}
}
