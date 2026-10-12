package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/rozbh/gamdias-atlas/internal/display"
	"github.com/rozbh/gamdias-atlas/internal/sensor"
)

// minGap is the least time between two frames when new values arrive faster
// than the regular refresh, so that a flood of values cannot flood the display.
const minGap = 100 * time.Millisecond

// gone reports whether err means the display is gone, as opposed to it having
// missed one report.
func gone(err error) bool {
	return err != nil && !errors.Is(err, display.ErrNoReply)
}

// wait blocks until the next tick, or until wake fires if that comes first,
// though never before notBefore. It reports false if ctx ended first.
func wait(ctx context.Context, t *time.Ticker, wake <-chan struct{}, notBefore time.Time) bool {
	if ctx.Err() != nil {
		return false
	}
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	case <-wake:
	}
	select {
	case <-time.After(time.Until(notBefore)):
		return true
	case <-ctx.Done():
		return false
	}
}

// feed shows the temperature for whichever mode the display is in, until ctx
// ends. The GPU sensor is only started once the display is in GPU mode; until
// it has a reading, or without one, the display shows the CPU.
func feed(ctx context.Context, d *display.Display, every time.Duration, cpu sensor.Sensor, startGPU func() sensor.Sensor) error {
	mode := display.CPU
	if r, err := d.Query(); err == nil {
		mode = r.Mode()
	} else if gone(err) {
		return err
	}
	var gpu sensor.Sensor
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		temp, err := cpu()
		if err != nil {
			return fmt.Errorf("CPU temperature: %w", err)
		}
		if mode == display.GPU {
			if startGPU != nil {
				gpu, startGPU = startGPU(), nil
			}
			if gpu != nil {
				if g, err := gpu(); err == nil {
					temp = g
				} else if !errors.Is(err, sensor.ErrNotReady) {
					return fmt.Errorf("GPU temperature: %w", err)
				}
			}
		}
		if r, err := d.Show(display.Temperature(temp), display.Keep); err == nil {
			mode = r.Mode()
		} else if gone(err) {
			return err
		}
		if !wait(ctx, t, nil, time.Time{}) {
			return nil
		}
	}
}

// hold shows custom frames until ctx ends: a new one every period, and one as
// soon as wake fires, though never less than minGap after the last. wake may
// be nil.
func hold(ctx context.Context, d *display.Display, every time.Duration, next func() display.Frame, wake <-chan struct{}) error {
	mode := display.Keep // frames only show in CPU or GPU mode
	if r, err := d.Query(); gone(err) {
		return err
	} else if err != nil || (r.Mode() != display.CPU && r.Mode() != display.GPU) {
		mode = display.CPU
	}
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		f, sent := next(), time.Now()
		if _, err := d.Show(f, mode); err == nil {
			mode = display.Keep
		} else if gone(err) {
			return err
		}
		if !wait(ctx, t, wake, sent.Add(minGap)) {
			if f.Flash != 0 { // don't leave the display blinking
				f.Flash = 0
				d.Show(f, display.Keep)
			}
			return nil
		}
	}
}
