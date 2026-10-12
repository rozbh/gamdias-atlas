package display

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const (
	hidID     = "HID_ID=0003:00001B80:0000B538"
	sysHidraw = "/sys/class/hidraw"
)

var (
	// ErrNotFound means no ATLAS Digital Display is connected.
	ErrNotFound = errors.New("ATLAS Digital Display (1b80:b538) not found")

	// ErrNoReply means the display did not answer in time. It is worth trying
	// again; any other error from a Display means it is gone.
	ErrNoReply = errors.New("no reply from the display")
)

// Display is a connection to the display. Use it from one goroutine.
type Display struct {
	// Timeout is how long to wait for a reply.
	Timeout time.Duration

	rw      io.ReadWriter
	replies chan Reply
	failed  chan error // why reading stopped
	err     error      // set once the display is gone
}

// Open finds the display and connects to it.
func Open() (*Display, error) {
	name := find(sysHidraw)
	if name == "" {
		return nil, ErrNotFound
	}
	f, err := os.OpenFile("/dev/"+name, os.O_RDWR, 0)
	if errors.Is(err, os.ErrPermission) {
		return nil, fmt.Errorf("no permission for /dev/%s - install the udev rule (see README) or use sudo", name)
	} else if err != nil {
		return nil, err
	}
	return New(f), nil
}

// find returns the device name of the display, like "hidraw3", or "" without one.
func find(root string) string {
	uevents, _ := filepath.Glob(filepath.Join(root, "hidraw*", "device", "uevent"))
	for _, u := range uevents {
		b, _ := os.ReadFile(u)
		if slices.Contains(strings.Split(string(b), "\n"), hidID) {
			return filepath.Base(filepath.Dir(filepath.Dir(u)))
		}
	}
	return ""
}

// New connects to a display that is reached through rw. Open is the usual way in.
func New(rw io.ReadWriter) *Display {
	d := &Display{
		Timeout: time.Second,
		rw:      rw,
		replies: make(chan Reply, 8),
		failed:  make(chan error, 1),
	}
	go d.read()
	return d
}

// read collects the display's replies until reading fails.
func (d *Display) read() {
	var buf [64]byte
	for {
		n, err := d.rw.Read(buf[:])
		if err != nil {
			d.failed <- err
			return
		}
		var r Reply
		copy(r[:], buf[:n])
		if r.valid(n) {
			d.replies <- r
		}
	}
}

// Query asks the display for its state without changing what it shows.
func (d *Display) Query() (Reply, error) {
	return d.send(report(cmdQuery, Frame{}, Keep))
}

// Show puts a frame on the display. A mode other than Keep also switches the
// display to that mode.
func (d *Display) Show(f Frame, m Mode) (Reply, error) {
	return d.send(report(cmdSet, f, m))
}

// send writes one report and waits for the display's reply.
func (d *Display) send(p [65]byte) (Reply, error) {
	if d.err != nil {
		return Reply{}, d.err
	}
	for len(d.replies) > 0 {
		<-d.replies // drop stale replies
	}
	if _, err := d.rw.Write(p[:]); err != nil {
		return Reply{}, d.fail(err)
	}
	select {
	case r := <-d.replies:
		return r, nil
	case err := <-d.failed:
		return Reply{}, d.fail(err)
	case <-time.After(d.Timeout):
		return Reply{}, ErrNoReply
	}
}

func (d *Display) fail(err error) error {
	d.err = fmt.Errorf("display: %w", err)
	return d.err
}
