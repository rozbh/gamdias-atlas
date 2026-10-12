// Command atlas drives the GAMDIAS ATLAS Digital Display (USB 1b80:b538).
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/rozbh/gamdias-atlas/internal/anim"
	"github.com/rozbh/gamdias-atlas/internal/display"
	"github.com/rozbh/gamdias-atlas/internal/sensor"
)

const usage = `usage:
  atlas                          show the CPU or GPU temperature (what ZEUS CAST does)
  atlas show 12.3                show your own digits; with no argument, reads lines from stdin
  atlas fun [name]               play animations made of digits; all of them, or one by name
  atlas raw 1 2 3 [dot] [flash]  send raw bytes
  atlas scan [dot] [first last]  step through byte values, to find what else the firmware draws
  atlas mode cpu|gpu|amb|off
  atlas status`

// interval is how often ZEUS CAST refreshes the display.
const interval = 390 * time.Millisecond

// app is the program and its connections to the outside, which tests replace.
type app struct {
	open     func() (*display.Display, error)
	cpu, gpu func() sensor.Sensor
	refresh  time.Duration // how often temperatures and custom values are sent
	in       io.Reader
	out, err io.Writer
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	a := app{
		open: display.Open, cpu: sensor.CPU, gpu: sensor.GPU, refresh: interval,
		in: os.Stdin, out: os.Stdout, err: os.Stderr,
	}
	err := a.run(ctx, os.Args[1:])
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// run carries out one command line. Commands that would run forever stop when ctx ends.
func (a app) run(ctx context.Context, args []string) error {
	switch {
	case len(args) == 0:
		return a.temperatures(ctx)
	case args[0] == "show":
		return a.show(ctx, args[1:])
	case args[0] == "fun" && len(args) <= 2:
		next, err := anim.Player(strings.Join(args[1:], ""))
		if err != nil {
			return err
		}
		return a.hold(ctx, anim.Tick, next)
	case args[0] == "raw":
		f, err := rawFrame(args[1:])
		if err != nil {
			return err
		}
		return a.hold(ctx, a.refresh, func() display.Frame { return f })
	case args[0] == "scan":
		return a.scan(ctx, args[1:])
	case args[0] == "mode" && len(args) == 2:
		return a.mode(args[1])
	case args[0] == "status" && len(args) == 1:
		return a.status()
	}
	return errors.New(usage)
}

func (a app) temperatures(ctx context.Context) error {
	cpu := a.cpu()
	if cpu == nil {
		return errors.New("no CPU temperature sensor in /sys/class/hwmon")
	}
	d, err := a.open()
	if err != nil {
		return err
	}
	return feed(ctx, d, a.refresh, cpu, a.gpu)
}

func (a app) hold(ctx context.Context, every time.Duration, next func() display.Frame) error {
	d, err := a.open()
	if err != nil {
		return err
	}
	return hold(ctx, d, every, next)
}

// show displays a fixed value or, with no arguments, the latest valid line of input.
func (a app) show(ctx context.Context, args []string) error {
	var latest atomic.Pointer[display.Frame]
	set := func(text string) error {
		f, err := display.Parse(text)
		if err != nil {
			return fmt.Errorf("can't show %q: %v", text, err)
		}
		latest.Store(&f)
		return nil
	}
	if len(args) > 0 {
		if err := set(strings.Join(args, " ")); err != nil {
			return err
		}
	} else {
		lines := bufio.NewScanner(a.in)
		next := func() bool { // read lines until one is valid
			for lines.Scan() {
				err := set(lines.Text())
				if err == nil {
					return true
				}
				fmt.Fprintln(a.err, err)
			}
			if err := lines.Err(); err != nil {
				fmt.Fprintln(a.err, "stdin:", err)
			}
			return false
		}
		if !next() {
			return errors.New("nothing to show")
		}
		go func() {
			for next() {
			}
		}()
	}
	return a.hold(ctx, a.refresh, func() display.Frame { return *latest.Load() })
}

func (a app) mode(name string) error {
	m, ok := display.ParseMode(name)
	if !ok {
		return errors.New(usage)
	}
	d, err := a.open()
	if err != nil {
		return err
	}
	_, err = d.Show(display.Frame{}, m)
	return err
}

func (a app) status() error {
	d, err := a.open()
	if err != nil {
		return err
	}
	r, err := d.Query()
	if err != nil {
		return err
	}
	major, minor := r.Firmware()
	_, err = fmt.Fprintf(a.out, "firmware %d.%d\nmode     %s\nambient  %.1f C\nreply    % x\n",
		major, minor, r.Mode(), float64(r.Ambient())/10, r[:12])
	return err
}

// rawFrame turns the arguments of `atlas raw` into a frame: three digit bytes,
// then optionally the dot and flash flags.
func rawFrame(args []string) (display.Frame, error) {
	var b [5]byte
	if len(args) < 3 || len(args) > len(b) {
		return display.Frame{}, errors.New("raw takes 3 to 5 bytes")
	}
	for i, a := range args {
		v, err := parseByte(a)
		if err != nil {
			return display.Frame{}, fmt.Errorf("raw: %v", err)
		}
		b[i] = v
	}
	return display.Frame{Digits: [3]byte(b[:3]), Dot: b[3], Flash: b[4]}, nil
}

func parseByte(s string) (byte, error) {
	n, err := strconv.ParseUint(s, 0, 8)
	if err != nil {
		return 0, fmt.Errorf("%q is not a byte (0-255 or 0x00-0xff)", s)
	}
	return byte(n), nil
}

// scan steps through byte values, so that you can see which ones the firmware
// draws. It prints each value and shows it after a frame you know, then ends.
// With "dot" it steps through the dot flag instead of a digit.
func (a app) scan(ctx context.Context, args []string) error {
	known := display.Frame{} // 000
	try := func(v byte) display.Frame { return display.Frame{Digits: [3]byte{v, 8, 8}} }
	first, last := byte(10), byte(255)
	what := "Each value is shown in the left digit, next to 88. Between values the display shows 000."
	if len(args) > 0 && args[0] == "dot" {
		args = args[1:]
		known = display.Frame{Digits: [3]byte{1, 2, 3}}
		try = func(v byte) display.Frame { return display.Frame{Digits: known.Digits, Dot: v} }
		first = 2
		what = "The display shows 123. Each value is tried as the dot flag."
	}
	switch len(args) {
	case 0:
	case 2:
		var err error
		if first, err = parseByte(args[0]); err == nil {
			last, err = parseByte(args[1])
		}
		if err != nil {
			return fmt.Errorf("scan: %v", err)
		}
	default:
		return errors.New(usage)
	}
	if first > last {
		return errors.New("scan: the first value is larger than the last")
	}
	fmt.Fprintf(a.out, "%s\nNote the numbers where it shows something new. Ctrl-C stops.\n", what)

	ctx, finish := context.WithCancel(ctx)
	defer finish()
	v, tick := int(first), 0 // each value gets three ticks: the known frame, then the value twice
	return a.hold(ctx, a.refresh, func() display.Frame {
		if v > int(last) {
			finish()
			return known
		}
		f := known
		if tick > 0 {
			f = try(byte(v))
		}
		if tick == 1 {
			fmt.Fprintln(a.out, v)
		}
		if tick++; tick == 3 {
			v, tick = v+1, 0
		}
		return f
	})
}
