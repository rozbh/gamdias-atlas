package display

import (
	"errors"
	"strings"
)

// Glyphs is what Parse accepts for one digit: character -> byte sent to the display.
// The display draws 0-9 and ignores 10-40. Values above 40 are untested: try them
// with `atlas raw` and add any that draw something here.
var Glyphs = map[rune]byte{
	'0': 0, '1': 1, '2': 2, '3': 3, '4': 4, '5': 5, '6': 6, '7': 7, '8': 8, '9': 9,
}

var (
	ErrGlyphs = errors.New("use up to 3 characters from the glyphs table")
	ErrDot    = errors.New("the dot only goes before the last digit")
)

// Parse turns text like "123", "12.3" or "7" (shown as 007) into a frame.
func Parse(s string) (f Frame, err error) {
	var g [3]byte
	n, afterDot := 0, -1 // glyphs seen, and glyphs seen since the dot
	for _, r := range strings.TrimSpace(s) {
		b, ok := Glyphs[r]
		switch {
		case r == '.' && afterDot < 0:
			afterDot = 0
		case ok && n < len(g):
			g[n] = b
			n++
			if afterDot >= 0 {
				afterDot++
			}
		default:
			return Frame{}, ErrGlyphs
		}
	}
	if afterDot == 0 || afterDot > 1 {
		return Frame{}, ErrDot
	}
	copy(f.Digits[len(g)-n:], g[:n]) // right-aligned: there is no blank digit, so short values start with zeros
	if afterDot == 1 {
		f.Dot = 1
	}
	return f, nil
}

// Temperature turns tenths of a degree into a frame like 48.7, capped at 99.9
// as ZEUS CAST does.
func Temperature(tenths int) Frame {
	t := max(0, min(tenths, 999))
	return Frame{Digits: [3]byte{byte(t / 100), byte(t / 10 % 10), byte(t % 10)}, Dot: 1}
}
