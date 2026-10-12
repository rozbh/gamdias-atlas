package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rozbh/gamdias-atlas/internal/display"
	"github.com/rozbh/gamdias-atlas/internal/display/displaytest"
	"github.com/rozbh/gamdias-atlas/internal/sensor"
)

const tick = 3 * time.Millisecond

// output is a buffer that the program and the test may use at the same time.
type output struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (o *output) Write(p []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.b.Write(p)
}

func (o *output) String() string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.b.String()
}

func fixed(tenths int) sensor.Sensor {
	return func() (int, error) { return tenths, nil }
}

// testApp is the program wired to d, with a CPU at 48.7 and a GPU at 61.2 degrees.
func testApp(d *display.Display) (a app, out, errOut *output) {
	out, errOut = &output{}, &output{}
	return app{
		open:    func() (*display.Display, error) { return d, nil },
		cpu:     func() sensor.Sensor { return fixed(487) },
		gpu:     func() sensor.Sensor { return fixed(612) },
		refresh: tick,
		in:      strings.NewReader(""),
		out:     out,
		err:     errOut,
	}, out, errOut
}

// start runs a command line in the background. stop ends it and returns its error.
func start(t *testing.T, a app, args ...string) (stop func() error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- a.run(ctx, args) }()
	return func() error {
		cancel()
		select {
		case err := <-done:
			return err
		case <-time.After(5 * time.Second):
			t.Fatal("the command did not stop")
			return nil
		}
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(time.Millisecond) {
		if cond() {
			return
		}
	}
	t.Fatalf("timed out waiting for %s", what)
}

func digits(d1, d2, d3, dot, flash byte) display.Frame {
	return display.Frame{Digits: [3]byte{d1, d2, d3}, Dot: dot, Flash: flash}
}

// shows reports whether the display's latest report is frame f.
func shows(fake *displaytest.Fake, f display.Frame) func() bool {
	return func() bool {
		r := fake.Reports()
		return len(r) > 1 && r[len(r)-1].Frame == f
	}
}

var (
	query = displaytest.Report{Query: true, Mode: display.Keep}
	cpu   = display.Temperature(487)
	gpu   = display.Temperature(612)
)

func TestStatus(t *testing.T) {
	d, _ := displaytest.New(display.GPU)
	a, out, _ := testApp(d)
	if err := a.run(context.Background(), []string{"status"}); err != nil {
		t.Fatal(err)
	}
	const want = "firmware 1.2\nmode     gpu\nambient  26.8 C\nreply    38 b5 01 01 02 02 01 02 06 08 01 05\n"
	if out.String() != want {
		t.Errorf("status printed:\n%s\nwant:\n%s", out, want)
	}
}

func TestMode(t *testing.T) {
	d, fake := displaytest.New(display.CPU)
	a, _, _ := testApp(d)
	if err := a.run(context.Background(), []string{"mode", "amb"}); err != nil {
		t.Fatal(err)
	}
	want := displaytest.Report{Mode: display.Ambient}
	if got := fake.Reports(); len(got) != 1 || got[0] != want {
		t.Errorf("the display received %+v, want %+v", got, want)
	}
}

func TestBadCommandLinesNeverTouchTheDisplay(t *testing.T) {
	a, _, _ := testApp(nil)
	a.open = func() (*display.Display, error) {
		t.Error("the display was opened")
		return nil, errors.New("opened")
	}
	a.cpu = func() sensor.Sensor { return nil }
	for _, c := range []struct{ args, want string }{
		{"help", usage}, {"--help", usage}, {"nope", usage},
		{"status extra", usage}, {"mode", usage}, {"mode sideways", usage}, {"mode cpu gpu", usage},
		{"fun fill ball", usage},
		{"fun nope", `no animation "nope"`},
		{"raw 1 2", "raw takes 3 to 5 bytes"},
		{"raw 1 2 3 4 5 6", "raw takes 3 to 5 bytes"},
		{"raw 1 2 256", `"256" is not a byte`},
		{"raw 1 2 -1", `"-1" is not a byte`},
		{"show 1.23", "the dot only goes before the last digit"},
		{"show abc", "use up to 3 characters"},
		{"show 1 2", "use up to 3 characters"},
		{"show", "nothing to show"},
		{"", "no CPU temperature sensor"},
	} {
		err := a.run(context.Background(), strings.Fields(c.args))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("atlas %s: error %v, want %q", c.args, err, c.want)
		}
	}
}

func TestOpenErrorIsReported(t *testing.T) {
	a, _, _ := testApp(nil)
	a.open = func() (*display.Display, error) { return nil, display.ErrNotFound }
	for _, args := range []string{"", "status", "mode cpu", "show 123", "fun fill", "raw 1 2 3"} {
		if err := a.run(context.Background(), strings.Fields(args)); !errors.Is(err, display.ErrNotFound) {
			t.Errorf("atlas %s: error %v, want ErrNotFound", args, err)
		}
	}
}

func TestRawFrame(t *testing.T) {
	for in, want := range map[string]display.Frame{
		"1 2 3": digits(1, 2, 3, 0, 0), "1 2 3 1": digits(1, 2, 3, 1, 0), "1 2 3 1 1": digits(1, 2, 3, 1, 1),
		"0 0 0": {}, "255 255 255 255 255": digits(255, 255, 255, 255, 255), "0x20 0x41 0x2d": digits(0x20, 0x41, 0x2d, 0, 0),
	} {
		if got, err := rawFrame(strings.Fields(in)); err != nil || got != want {
			t.Errorf("rawFrame(%q) = %+v, %v; want %+v", in, got, err, want)
		}
	}
	for _, in := range []string{"", "1", "1 2", "1 2 3 4 5 6", "1 2 256", "1 2 -1", "1 2 x", "1 2 3.5", "1 2 99999999999999999999"} {
		if got, err := rawFrame(strings.Fields(in)); err == nil || got != (display.Frame{}) {
			t.Errorf("rawFrame(%q) = %+v, %v; want an error", in, got, err)
		}
	}
	if _, err := rawFrame([]string{"1", "2", ""}); err == nil {
		t.Error("rawFrame with an empty argument should fail")
	}
}

func TestTemperaturesFollowTheDisplaysMode(t *testing.T) {
	d, fake := displaytest.New(display.CPU)
	a, _, _ := testApp(d)
	var gpuStarts atomic.Int32
	var gpuReady atomic.Bool
	a.gpu = func() sensor.Sensor {
		gpuStarts.Add(1)
		return func() (int, error) {
			if !gpuReady.Load() {
				return 0, sensor.ErrNotReady
			}
			return 612, nil
		}
	}
	stop := start(t, a)

	waitFor(t, "CPU temperatures", func() bool { return len(fake.Reports()) >= 4 })
	if got := fake.Reports()[0]; got != query {
		t.Errorf("the first report was %+v, want the query", got)
	}
	if n := gpuStarts.Load(); n != 0 {
		t.Errorf("the GPU sensor was started %d times while in CPU mode", n)
	}

	fake.SetMode(display.GPU) // as if switched on the display itself
	waitFor(t, "the GPU sensor to start", func() bool { return gpuStarts.Load() == 1 })
	before := len(fake.Reports())
	waitFor(t, "more reports", func() bool { return len(fake.Reports()) >= before+3 })
	for i, r := range fake.Reports()[1:] {
		if (r != displaytest.Report{Frame: cpu, Mode: display.Keep}) {
			t.Fatalf("report %d was %+v; without a GPU reading the display should show the CPU", i+1, r)
		}
	}

	gpuReady.Store(true)
	waitFor(t, "GPU temperatures", shows(fake, gpu))
	fake.SetMode(display.CPU)
	waitFor(t, "CPU temperatures again", shows(fake, cpu))
	if n := gpuStarts.Load(); n != 1 {
		t.Errorf("the GPU sensor was started %d times, want once", n)
	}
	if err := stop(); err != nil {
		t.Errorf("stopping returned %v", err)
	}
}

func TestTemperaturesWithoutGPUSensor(t *testing.T) {
	d, fake := displaytest.New(display.GPU)
	a, _, _ := testApp(d)
	a.gpu = func() sensor.Sensor { return nil }
	stop := start(t, a)
	waitFor(t, "temperatures", func() bool { return len(fake.Reports()) >= 4 })
	if !shows(fake, cpu)() {
		t.Errorf("with no GPU sensor the display should show the CPU, got %+v", fake.Reports()[3])
	}
	if err := stop(); err != nil {
		t.Error(err)
	}
}

func TestTemperaturesStopWhenSomethingIsGone(t *testing.T) {
	stopped := errors.New("nvidia-smi stopped")
	for name, c := range map[string]struct {
		mode     display.Mode
		cpu, gpu sensor.Sensor
		unplug   bool
		want     string
	}{
		"CPU sensor":           {display.CPU, func() (int, error) { return 0, errors.New("gone") }, fixed(612), false, "CPU temperature: gone"},
		"GPU sensor":           {display.GPU, fixed(487), func() (int, error) { return 0, stopped }, false, "GPU temperature: nvidia-smi stopped"},
		"display":              {display.CPU, fixed(487), fixed(612), true, "display:"},
		"GPU sensor, CPU mode": {display.CPU, fixed(487), func() (int, error) { return 0, stopped }, false, ""},
	} {
		d, fake := displaytest.New(c.mode)
		a, _, _ := testApp(d)
		a.cpu = func() sensor.Sensor { return c.cpu }
		a.gpu = func() sensor.Sensor { return c.gpu }
		if c.unplug {
			fake.Unplug()
		}
		ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
		err := a.run(ctx, nil)
		cancel()
		switch {
		case c.want == "" && err != nil:
			t.Errorf("%s: stopped with %v, but nothing it needs is gone", name, err)
		case c.want != "" && (err == nil || !strings.Contains(err.Error(), c.want)):
			t.Errorf("%s gone: error %v, want %q", name, err, c.want)
		}
	}
}

func TestTemperaturesSurviveMissedReplies(t *testing.T) {
	d, fake := displaytest.New(display.CPU)
	d.Timeout = 5 * time.Millisecond
	fake.Answer(func(n int, m display.Mode) []byte {
		if n%2 == 1 { // every other report gets no answer, the first one included
			return nil
		}
		return displaytest.Reply(m)
	})
	a, _, _ := testApp(d)
	stop := start(t, a)
	waitFor(t, "temperatures despite missed replies", func() bool { return len(fake.Reports()) >= 8 })
	if err := stop(); err != nil {
		t.Errorf("a missed reply should not stop it, got %v", err)
	}
}

func TestShowSwitchesToAModeThatShowsIt(t *testing.T) {
	for from, want := range map[display.Mode]display.Mode{
		display.CPU: display.Keep, display.GPU: display.Keep,
		display.Ambient: display.CPU, display.Off: display.CPU,
	} {
		d, fake := displaytest.New(from)
		a, _, _ := testApp(d)
		stop := start(t, a, "show", "12.3")
		waitFor(t, "frames", func() bool { return len(fake.Reports()) >= 5 })
		if err := stop(); err != nil {
			t.Error(err)
		}
		got := fake.Reports()
		if got[0] != query {
			t.Errorf("from %v: the first report was %+v, want the query", from, got[0])
		}
		if (got[1] != displaytest.Report{Frame: digits(1, 2, 3, 1, 0), Mode: want}) {
			t.Errorf("from %v: the first frame was %+v, want mode %v", from, got[1], want)
		}
		for i, r := range got[2:] {
			if (r != displaytest.Report{Frame: digits(1, 2, 3, 1, 0), Mode: display.Keep}) {
				t.Errorf("from %v: report %d was %+v", from, i+2, r)
			}
		}
	}
}

func TestShowAsksForTheModeUntilAnswered(t *testing.T) {
	d, fake := displaytest.New(display.Ambient)
	d.Timeout = 5 * time.Millisecond
	fake.Answer(func(n int, m display.Mode) []byte {
		if n <= 3 { // the query and the first two frames get no answer
			return nil
		}
		return displaytest.Reply(m)
	})
	a, _, _ := testApp(d)
	stop := start(t, a, "show", "123")
	waitFor(t, "frames", func() bool { return len(fake.Reports()) >= 6 })
	stop()
	for i, want := range []display.Mode{display.CPU, display.CPU, display.CPU, display.Keep, display.Keep} {
		if got := fake.Reports()[i+1].Mode; got != want {
			t.Errorf("frame %d asked for mode %v, want %v", i+1, got, want)
		}
	}
}

func TestStoppingTurnsTheFlashOff(t *testing.T) {
	d, fake := displaytest.New(display.CPU)
	a, _, _ := testApp(d)
	stop := start(t, a, "raw", "7", "7", "7", "0", "1")
	waitFor(t, "frames", func() bool { return len(fake.Reports()) >= 4 })
	if err := stop(); err != nil {
		t.Error(err)
	}
	got := fake.Reports()
	for _, r := range got[1 : len(got)-1] {
		if r.Frame != digits(7, 7, 7, 0, 1) {
			t.Errorf("while running: %+v, want flashing 777", r)
		}
	}
	if last := got[len(got)-1]; (last != displaytest.Report{Frame: digits(7, 7, 7, 0, 0), Mode: display.Keep}) {
		t.Errorf("the last report was %+v, want 777 with the flash off", last)
	}

	// without a flash there is nothing to undo
	d, fake = displaytest.New(display.CPU)
	a, _, _ = testApp(d)
	stop = start(t, a, "raw", "1", "2", "3")
	waitFor(t, "frames", func() bool { return len(fake.Reports()) >= 4 })
	stop()
	n := len(fake.Reports())
	time.Sleep(20 * time.Millisecond)
	if after := len(fake.Reports()); after != n {
		t.Errorf("%d more reports were sent after stopping", after-n)
	}
}

func TestShowFromInput(t *testing.T) {
	d, fake := displaytest.New(display.CPU)
	a, _, errOut := testApp(d)
	in, lines := io.Pipe()
	a.in = in
	stop := start(t, a, "show")

	io.WriteString(lines, "junk\n 42\n")
	waitFor(t, "042", shows(fake, digits(0, 4, 2, 0, 0)))
	if got := errOut.String(); !strings.Contains(got, `can't show "junk"`) {
		t.Errorf("the bad first line should be reported, got %q", got)
	}

	io.WriteString(lines, "4.3\n")
	waitFor(t, "04.3", shows(fake, digits(0, 4, 3, 1, 0)))

	io.WriteString(lines, "1.2.3\n")
	waitFor(t, "the bad line to be reported", func() bool { return strings.Contains(errOut.String(), `can't show "1.2.3"`) })
	io.WriteString(lines, "777\n")
	waitFor(t, "777", shows(fake, digits(7, 7, 7, 0, 0)))

	lines.Close() // the end of the input is not the end of the program
	n := len(fake.Reports())
	waitFor(t, "frames after the input ended", func() bool { return len(fake.Reports()) >= n+3 })
	if !shows(fake, digits(7, 7, 7, 0, 0))() {
		t.Error("after the input ended the last value should stay")
	}
	if err := stop(); err != nil {
		t.Error(err)
	}
}

func TestFunPlaysAnAnimation(t *testing.T) {
	d, fake := displaytest.New(display.CPU)
	a, _, _ := testApp(d)
	stop := start(t, a, "fun", "fill") // 000 800 880 ...
	waitFor(t, "three frames", func() bool { return len(fake.Reports()) >= 4 })
	if err := stop(); err != nil {
		t.Error(err)
	}
	got := fake.Reports()
	for i, want := range []display.Frame{digits(0, 0, 0, 0, 0), digits(8, 0, 0, 0, 0), digits(8, 8, 0, 0, 0)} {
		if got[i+1].Frame != want {
			t.Errorf("frame %d was %+v, want %+v", i, got[i+1].Frame, want)
		}
	}
}

func TestShowStopsWhenTheDisplayIsGone(t *testing.T) {
	d, fake := displaytest.New(display.CPU)
	a, _, _ := testApp(d)
	done := make(chan error, 1)
	go func() { done <- a.run(context.Background(), []string{"show", "123"}) }()
	waitFor(t, "frames", func() bool { return len(fake.Reports()) >= 3 })
	fake.Unplug()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "display:") {
			t.Errorf("error %v, want one about the display", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("show kept running after the display was unplugged")
	}
}

func FuzzRawFrame(f *testing.F) {
	f.Add("1 2 3")
	f.Add("0x20 255 0 1 1")
	f.Add("1 2 256")
	f.Fuzz(func(t *testing.T, s string) {
		rawFrame(strings.Fields(s)) // must not panic, whatever the arguments
	})
}
