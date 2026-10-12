package anim

import (
	"strings"
	"testing"

	"github.com/rozbh/gamdias-atlas/internal/display"
)

func frame(d1, d2, d3, dot, flash byte) display.Frame {
	return display.Frame{Digits: [3]byte{d1, d2, d3}, Dot: dot, Flash: flash}
}

func TestCompile(t *testing.T) {
	for in, want := range map[string]step{
		"123":     {frame: frame(1, 2, 3, 0, 0), ticks: 1},
		"12.3":    {frame: frame(1, 2, 3, 1, 0), ticks: 1},
		"777!":    {frame: frame(7, 7, 7, 0, 1), ticks: 1},
		"888*3":   {frame: frame(8, 8, 8, 0, 0), ticks: 3},
		"0.5!*16": {frame: frame(0, 0, 5, 1, 1), ticks: 16},
		"???":     {roll: [3]bool{true, true, true}, ticks: 1},
		"7??*8":   {frame: frame(7, 0, 0, 0, 0), roll: [3]bool{false, true, true}, ticks: 8},
		"?":       {roll: [3]bool{false, false, true}, ticks: 1},
		"?.?":     {frame: frame(0, 0, 0, 1, 0), roll: [3]bool{false, true, true}, ticks: 1},
		"5":       {frame: frame(0, 0, 5, 0, 0), ticks: 1},
		// a huge count is one step held for a long time, not a huge list
		"888*9000000000000000000": {frame: frame(8, 8, 8, 0, 0), ticks: 9000000000000000000},
	} {
		if got, err := compile(in); err != nil || got != want {
			t.Errorf("compile(%q) = %+v, %v; want %+v", in, got, err, want)
		}
	}
	for _, in := range []string{"1234", "abc", "1.23", "888*0", "888*-1", "888*x", "888*", "888*2*3",
		"888*99999999999999999999", "88!8", "!!", "1 2", "????"} {
		if got, err := compile(in); err == nil || got != (step{}) {
			t.Errorf("compile(%q) = %+v, %v; want an error", in, got, err)
		}
	}
}

func TestStepRolls(t *testing.T) {
	st, err := compile("7?.?!")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[display.Frame]bool{}
	for i := 0; i < 500; i++ {
		f := st.show()
		if f.Digits[0] != 7 || f.Digits[1] > 9 || f.Digits[2] > 9 || f.Dot != 1 || f.Flash != 1 {
			t.Fatalf("show() = %+v", f)
		}
		seen[f] = true
	}
	if len(seen) < 50 {
		t.Errorf("only %d different frames out of 100 possible", len(seen))
	}
	if st.frame != frame(7, 0, 0, 1, 1) {
		t.Errorf("show() changed the step: %+v", st.frame)
	}
}

// withAnimations swaps the table for one test.
func withAnimations(t *testing.T, as ...Animation) {
	saved := Animations
	t.Cleanup(func() { Animations = saved })
	Animations = as
}

func TestBuiltInAnimations(t *testing.T) {
	all, err := script("")
	if err != nil {
		t.Fatalf("the built-in animations do not compile: %v", err)
	}
	total := 0
	for _, a := range Animations {
		one, err := script(a.Name)
		if err != nil || len(one) != len(strings.Fields(a.Frames)) {
			t.Errorf("script(%q): %d steps, %v", a.Name, len(one), err)
		}
		total += len(one)
	}
	if len(all) != total {
		t.Errorf("all animations have %d steps, one by one they have %d", len(all), total)
	}
	if _, err := Player("nope"); err == nil || !strings.Contains(err.Error(), "fill ball wave") {
		t.Errorf("Player(nope) error should list the names, got %v", err)
	}
}

func TestBrokenAnimationsAreReported(t *testing.T) {
	withAnimations(t, Animation{"bad", "123 12x4"})
	if _, err := Player(""); err == nil || !strings.Contains(err.Error(), "bad") || !strings.Contains(err.Error(), "12x4") {
		t.Errorf("a broken animation should be named in the error, got %v", err)
	}
	withAnimations(t, Animation{"empty", ""})
	if _, err := Player(""); err == nil {
		t.Error("no frames at all should be an error, not an empty animation")
	}
	withAnimations(t)
	if _, err := Player(""); err == nil {
		t.Error("an empty table should be an error")
	}
}

func TestPlayer(t *testing.T) {
	withAnimations(t, Animation{"a", "111*2 222"}, Animation{"b", "333*3"})
	next, err := Player("")
	if err != nil {
		t.Fatal(err)
	}
	var got []byte
	for i := 0; i < 14; i++ {
		got = append(got, next().Digits[0])
	}
	if want := "11233311233311"; string(digitsText(got)) != want {
		t.Errorf("played %s, want %s", digitsText(got), want)
	}

	next, err = Player("b")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 7; i++ {
		if f := next(); f != frame(3, 3, 3, 0, 0) {
			t.Fatalf("a single step should repeat, got %+v", f)
		}
	}
}

func digitsText(ds []byte) []byte {
	out := make([]byte, len(ds))
	for i, d := range ds {
		out[i] = '0' + d
	}
	return out
}

func TestSlotMachine(t *testing.T) {
	next, err := Player("slot") // ???*12 7??*8 77?*8 777!*16
	if err != nil {
		t.Fatal(err)
	}
	for tick := 0; tick < 88; tick++ {
		f, i := next(), tick%44
		ok := f.Digits[0] <= 9 && f.Digits[1] <= 9 && f.Digits[2] <= 9 && f.Dot == 0
		switch {
		case i < 12:
			ok = ok && f.Flash == 0
		case i < 20:
			ok = ok && f.Digits[0] == 7 && f.Flash == 0
		case i < 28:
			ok = ok && f.Digits[0] == 7 && f.Digits[1] == 7 && f.Flash == 0
		default:
			ok = ok && f.Digits == [3]byte{7, 7, 7} && f.Flash == 1
		}
		if !ok {
			t.Errorf("tick %d: %+v", tick, f)
		}
	}
}

func TestPlayingDoesNotAllocate(t *testing.T) {
	next, err := Player("")
	if err != nil {
		t.Fatal(err)
	}
	var sink byte
	if n := testing.AllocsPerRun(2000, func() { sink += next().Digits[2] }); n != 0 {
		t.Errorf("each tick allocates %v times, want 0", n)
	}
}

func FuzzCompile(f *testing.F) {
	for _, s := range []string{"7??*8", "777!", "*", "1*0", "?.?", "12.3!*4", "????", "!", "1*-1", "?*9999999999999999999"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		st, err := compile(s)
		if err != nil {
			return
		}
		if st.ticks < 1 {
			t.Errorf("compile(%q) holds for %d ticks", s, st.ticks)
		}
		for i := 0; i < 3; i++ {
			if fr := st.show(); fr.Digits[0] > 9 || fr.Digits[1] > 9 || fr.Digits[2] > 9 || fr.Dot > 1 || fr.Flash > 1 {
				t.Errorf("compile(%q) plays %+v", s, fr)
			}
		}
	})
}
