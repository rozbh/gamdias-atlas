package display

import (
	"math"
	"strings"
	"testing"
)

func digits(d1, d2, d3, dot byte) Frame {
	return Frame{Digits: [3]byte{d1, d2, d3}, Dot: dot}
}

func TestParse(t *testing.T) {
	for in, want := range map[string]Frame{
		"123": digits(1, 2, 3, 0), "12.3": digits(1, 2, 3, 1), "000": digits(0, 0, 0, 0), "99.9": digits(9, 9, 9, 1),
		"7": digits(0, 0, 7, 0), "42": digits(0, 4, 2, 0), ".5": digits(0, 0, 5, 1), "4.2": digits(0, 4, 2, 1),
		"": {}, "   ": {}, " 42\n": digits(0, 4, 2, 0), "7.5\r\n": digits(0, 7, 5, 1),
	} {
		if got, err := Parse(in); err != nil || got != want {
			t.Errorf("Parse(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for in, want := range map[string]error{
		"1234": ErrGlyphs, "abc": ErrGlyphs, "-12": ErrGlyphs, "1 2": ErrGlyphs, "１２３": ErrGlyphs,
		"12\x00": ErrGlyphs, "\xff": ErrGlyphs, "1..2": ErrGlyphs, "1.2.3": ErrGlyphs,
		strings.Repeat("1", 1<<20): ErrGlyphs,
		"1.23":                     ErrDot, "12.": ErrDot, ".": ErrDot, ".123": ErrDot,
	} {
		if got, err := Parse(in); err != want || got != (Frame{}) {
			t.Errorf("Parse(%.20q) = %v, %v; want error %q", in, got, err, want)
		}
	}
}

func TestTemperature(t *testing.T) {
	for in, want := range map[int]Frame{
		math.MinInt: digits(0, 0, 0, 1), -1: digits(0, 0, 0, 1), 0: digits(0, 0, 0, 1), 1: digits(0, 0, 1, 1),
		9: digits(0, 0, 9, 1), 10: digits(0, 1, 0, 1), 99: digits(0, 9, 9, 1), 100: digits(1, 0, 0, 1),
		366: digits(3, 6, 6, 1), 998: digits(9, 9, 8, 1), 999: digits(9, 9, 9, 1), 1000: digits(9, 9, 9, 1),
		math.MaxInt: digits(9, 9, 9, 1),
	} {
		if got := Temperature(in); got != want {
			t.Errorf("Temperature(%d) = %v, want %v", in, got, want)
		}
	}
}

func FuzzParse(f *testing.F) {
	for _, s := range []string{"123", "12.3", "7", "", "1.23", "abc", "..", "１２３", "12.3\n", "\xff\xfe", "1\x002"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		fr, err := Parse(s)
		if err != nil {
			if fr != (Frame{}) {
				t.Errorf("Parse(%q) failed but returned %v", s, fr)
			}
			return
		}
		if fr.Digits[0] > 9 || fr.Digits[1] > 9 || fr.Digits[2] > 9 || fr.Dot > 1 || fr.Flash != 0 {
			t.Errorf("Parse(%q) = %v", s, fr)
		}
	})
}
