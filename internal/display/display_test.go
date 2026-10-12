package display_test

import (
	"errors"
	"io"
	"testing"
	"time"

	"github.com/rozbh/gamdias-atlas/internal/display"
	"github.com/rozbh/gamdias-atlas/internal/display/displaytest"
)

func TestQueryAndShow(t *testing.T) {
	d, fake := displaytest.New(display.Ambient)
	r, err := d.Query()
	if err != nil || r.Mode() != display.Ambient || r.Ambient() != 268 {
		t.Fatalf("Query = mode %v, ambient %d, %v", r.Mode(), r.Ambient(), err)
	}

	frame := display.Frame{Digits: [3]byte{1, 2, 3}, Dot: 1, Flash: 1}
	if r, err = d.Show(frame, display.Keep); err != nil || r.Mode() != display.Ambient {
		t.Errorf("Show with Keep = mode %v, %v; the mode should not change", r.Mode(), err)
	}
	if r, err = d.Show(frame, display.GPU); err != nil || r.Mode() != display.GPU {
		t.Errorf("Show with GPU = mode %v, %v; the display should switch", r.Mode(), err)
	}

	want := []displaytest.Report{
		{Query: true, Mode: display.Keep},
		{Frame: frame, Mode: display.Keep},
		{Frame: frame, Mode: display.GPU},
	}
	got := fake.Reports()
	if len(got) != len(want) {
		t.Fatalf("the display received %d reports, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("report %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestBadRepliesAreIgnored(t *testing.T) {
	for name, answer := range map[string][]byte{
		"no answer":    nil,
		"too short":    displaytest.RealReply[:11],
		"bad checksum": append(append([]byte{}, displaytest.RealReply[:11]...), 0x99),
		"bad header":   append([]byte{0x39}, displaytest.RealReply[1:]...),
		"empty":        {},
	} {
		d, fake := displaytest.New(display.CPU)
		fake.Answer(func(int, display.Mode) []byte { return answer })
		start := time.Now()
		if r, err := d.Query(); !errors.Is(err, display.ErrNoReply) {
			t.Errorf("%s: Query = % x, %v; want ErrNoReply", name, r[:12], err)
		}
		if took := time.Since(start); took < d.Timeout || took > d.Timeout+time.Second {
			t.Errorf("%s: gave up after %v, want about %v", name, took, d.Timeout)
		}
		// one missed reply is not the end: the next report goes out as usual
		fake.Answer(nil)
		if _, err := d.Query(); err != nil {
			t.Errorf("%s: the Query after a missed reply failed: %v", name, err)
		}
	}
}

// A late answer to an earlier report must not be taken for the answer to the next one.
func TestStaleReplyIsDropped(t *testing.T) {
	d, fake := displaytest.New(display.CPU)
	fake.Answer(func(n int, m display.Mode) []byte {
		if n == 1 {
			time.Sleep(d.Timeout + 50*time.Millisecond)
			return displaytest.Reply(display.Ambient)
		}
		return displaytest.Reply(display.GPU)
	})
	if _, err := d.Query(); !errors.Is(err, display.ErrNoReply) {
		t.Fatalf("the slow answer should time out, got %v", err)
	}
	time.Sleep(100 * time.Millisecond) // let the late answer arrive
	if r, err := d.Query(); err != nil || r.Mode() != display.GPU {
		t.Errorf("Query = mode %v, %v; want the fresh answer (gpu)", r.Mode(), err)
	}
}

func TestUnpluggedDisplay(t *testing.T) {
	d, fake := displaytest.New(display.CPU)
	if _, err := d.Query(); err != nil {
		t.Fatal(err)
	}
	fake.Unplug()
	_, err := d.Query()
	if err == nil || errors.Is(err, display.ErrNoReply) {
		t.Fatalf("Query on an unplugged display = %v, want a lasting error", err)
	}
	if _, again := d.Show(display.Frame{}, display.Keep); again == nil || again.Error() != err.Error() {
		t.Errorf("the error should stick: first %v, then %v", err, again)
	}
}

// brokenPipe reads fine, forever, but cannot be written to.
type brokenPipe struct{ block chan struct{} }

func (b brokenPipe) Read([]byte) (int, error)  { <-b.block; return 0, io.EOF }
func (b brokenPipe) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestWriteFailure(t *testing.T) {
	d := display.New(brokenPipe{make(chan struct{})})
	if _, err := d.Query(); !errors.Is(err, io.ErrClosedPipe) {
		t.Errorf("Query = %v, want the write error", err)
	}
}
