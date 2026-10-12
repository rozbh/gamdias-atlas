// Package anim plays animations made of digits on the display.
package anim

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/rozbh/gamdias-atlas/internal/display"
)

// Tick is how long one frame of an animation stays on the display.
const Tick = 150 * time.Millisecond

// Animation is a named list of frames, separated by spaces.
type Animation struct{ Name, Frames string }

// step is one frame, shown for a number of ticks. roll marks the digits that
// are random on every tick.
type step struct {
	frame display.Frame
	roll  [3]bool
	ticks int
}

// compile turns one frame, like "7??*8" or "777!", into a step.
func compile(s string) (st step, err error) {
	body, count, repeated := strings.Cut(s, "*")
	st.ticks = 1
	if repeated {
		if st.ticks, err = strconv.Atoi(count); err != nil || st.ticks < 1 {
			return step{}, fmt.Errorf("frame %q: the count after * must be 1 or more", s)
		}
	}
	text, flash := strings.CutSuffix(body, "!")
	if st.frame, err = display.Parse(strings.ReplaceAll(text, "?", "0")); err != nil {
		return step{}, fmt.Errorf("frame %q: %v", s, err)
	}
	if flash {
		st.frame.Flash = 1
	}
	shown := []rune(strings.ReplaceAll(strings.TrimSpace(text), ".", "")) // at most 3: Parse accepted it
	for i, r := range shown {
		st.roll[len(st.roll)-len(shown)+i] = r == '?'
	}
	return st, nil
}

// show returns the step's frame with fresh random digits.
func (st step) show() display.Frame {
	f := st.frame
	for i, roll := range st.roll {
		if roll {
			f.Digits[i] = byte(rand.Intn(10))
		}
	}
	return f
}

// script compiles the named animation, or all of them when name is empty.
func script(name string) ([]step, error) {
	var steps []step
	var names []string
	for _, a := range Animations {
		names = append(names, a.Name)
		if name != "" && name != a.Name {
			continue
		}
		for _, s := range strings.Fields(a.Frames) {
			st, err := compile(s)
			if err != nil {
				return nil, fmt.Errorf("animation %s: %v", a.Name, err)
			}
			steps = append(steps, st)
		}
	}
	if len(steps) == 0 {
		return nil, fmt.Errorf("no animation %q; there are: %s", name, strings.Join(names, " "))
	}
	return steps, nil
}

// Player returns a function that gives the frame for each tick, looping
// forever over the named animation, or over all of them when name is empty.
func Player(name string) (next func() display.Frame, err error) {
	steps, err := script(name)
	if err != nil {
		return nil, err
	}
	i, shown := 0, 0
	return func() display.Frame {
		if shown == steps[i].ticks {
			i, shown = (i+1)%len(steps), 0
		}
		shown++
		return steps[i].show()
	}, nil
}
