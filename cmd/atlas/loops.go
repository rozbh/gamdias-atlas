package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/rozbh/gamdias-atlas/internal/display"
	"github.com/rozbh/gamdias-atlas/internal/sensor"
)

// gone reports whether err means the display is gone, as opposed to it having
// missed one report.
func gone(err error) bool {
	return err != nil && !errors.Is(err, display.ErrNoReply)
}

// wait blocks until the next tick. It reports false if ctx ended first.
func wait(ctx context.Context, t *time.Ticker) bool {
	if ctx.Err() != nil {
		return false
	}
	select {
	case <-t.C:
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
		if !wait(ctx, t) {
			return nil
		}
	}
}

// hold shows custom frames, a new one every period, until ctx ends.
func hold(ctx context.Context, d *display.Display, every time.Duration, next func() display.Frame) error {
	mode := display.Keep // frames only show in CPU or GPU mode
	if r, err := d.Query(); gone(err) {
		return err
	} else if err != nil || (r.Mode() != display.CPU && r.Mode() != display.GPU) {
		mode = display.CPU
	}
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		f := next()
		if _, err := d.Show(f, mode); err == nil {
			mode = display.Keep
		} else if gone(err) {
			return err
		}
		if !wait(ctx, t) {
			if f.Flash != 0 { // don't leave the display blinking
				f.Flash = 0
				d.Show(f, display.Keep)
			}
			return nil
		}
	}
}
