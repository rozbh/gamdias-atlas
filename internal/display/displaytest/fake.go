// Package displaytest provides a stand-in for the display, for tests.
package displaytest

import (
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/rozbh/gamdias-atlas/internal/display"
)

// RealReply is what the hardware answered: firmware 1.2, CPU mode, ambient 26.8 C.
var RealReply = []byte{0x38, 0xb5, 0x01, 0x01, 0x02, 0x01, 0x01, 0x02, 0x06, 0x08, 0x01, 0x04}

// Reply is RealReply with another mode, and the checksum to match.
func Reply(m display.Mode) []byte {
	r := make([]byte, 64)
	copy(r, RealReply)
	r[5], r[11] = byte(m), 0
	for _, x := range r[:11] {
		r[11] += x
	}
	return r
}

// Report is one report the fake display received.
type Report struct {
	Query bool          // a query; otherwise a frame to show
	Frame display.Frame // from the CPU slot
	Mode  display.Mode
}

// Fake is a display on the other end of a pipe. Like the real one it remembers
// the mode it is told and answers every report.
type Fake struct {
	mu      sync.Mutex
	mode    display.Mode
	reports []Report
	answer  func(n int, m display.Mode) []byte
	dev     net.Conn
}

// New returns a fake display in the given mode and a connection to it. The
// connection has a short reply timeout, since the fake answers at once.
func New(m display.Mode) (*display.Display, *Fake) {
	host, dev := net.Pipe()
	f := &Fake{mode: m, dev: dev}
	go f.serve()
	d := display.New(host)
	d.Timeout = 100 * time.Millisecond
	return d, f
}

func (f *Fake) serve() {
	for {
		var p [65]byte
		if _, err := io.ReadFull(f.dev, p[:]); err != nil {
			return
		}
		r := decode(p)
		f.mu.Lock()
		if !r.Query && r.Mode != display.Keep {
			f.mode = r.Mode
		}
		f.reports = append(f.reports, r)
		out := Reply(f.mode)
		if f.answer != nil {
			out = f.answer(len(f.reports), f.mode)
		}
		f.mu.Unlock()
		if out != nil {
			f.dev.Write(out)
		}
	}
}

// decode checks a report as the firmware would and takes it apart. A report
// the real display could not have understood is a bug in the caller.
func decode(p [65]byte) Report {
	b := p[1:]
	var sum byte
	for _, x := range b[:16] {
		sum += x
	}
	switch {
	case p[0] != 0, b[0] != 0x38, b[1] != 0xB5, b[3] != 1, b[8] != 2, sum != b[16],
		[4]byte(b[4:8]) != [4]byte(b[9:13]), b[2] != 0x01 && b[2] != 0xFF:
		panic(fmt.Sprintf("displaytest: malformed report % x", p[:18]))
	}
	for _, x := range b[17:] {
		if x != 0 {
			panic(fmt.Sprintf("displaytest: report padding is not zero: % x", p))
		}
	}
	return Report{
		Query: b[2] == 0xFF,
		Frame: display.Frame{Digits: [3]byte(b[4:7]), Dot: b[7], Flash: b[13]},
		Mode:  display.Mode(b[15]),
	}
}

// Reports returns every report received so far.
func (f *Fake) Reports() []Report {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]Report(nil), f.reports...)
}

// SetMode changes the mode, as the button on the real display would.
func (f *Fake) SetMode(m display.Mode) {
	f.mu.Lock()
	f.mode = m
	f.mu.Unlock()
}

// Answer replaces the fake's replies: answer gets the number of reports
// received and the current mode, and returns the bytes to send, or nil for
// no reply.
func (f *Fake) Answer(answer func(n int, m display.Mode) []byte) {
	f.mu.Lock()
	f.answer = answer
	f.mu.Unlock()
}

// Unplug closes the connection, as pulling the cable would.
func (f *Fake) Unplug() { f.dev.Close() }
