// Package display talks to the GAMDIAS ATLAS Digital Display (USB 1b80:b538):
// three seven-segment digits with a decimal point before the last one.
//
// The host sends a 64-byte HID report and the display answers with one. The
// README describes every byte.
package display

import "strconv"

// Mode is what the display shows.
type Mode byte

const (
	Off     Mode = 0
	CPU     Mode = 1
	GPU     Mode = 2
	Ambient Mode = 3 // the display's own sensor

	// Keep, sent with a frame, leaves the display in the mode it is in.
	Keep Mode = 0xFF
)

var modeNames = [...]string{Off: "off", CPU: "cpu", GPU: "gpu", Ambient: "amb"}

func (m Mode) String() string {
	if int(m) < len(modeNames) {
		return modeNames[m]
	}
	return strconv.Itoa(int(m))
}

// ParseMode turns "off", "cpu", "gpu" or "amb" into a Mode.
func ParseMode(s string) (Mode, bool) {
	for m, name := range modeNames {
		if s == name {
			return Mode(m), true
		}
	}
	return 0, false
}

// Frame is what the three digits show.
type Frame struct {
	Digits [3]byte // one value per digit: the firmware draws 0-9 and ignores 10-40
	Dot    byte    // 1 lights the decimal point before the last digit
	Flash  byte    // 1 makes the display blink
}

const (
	cmdSet   byte = 0x01
	cmdQuery byte = 0xFF
)

// report builds what is written to the device: the HID report number, always
// 0, then the 64-byte report.
func report(cmd byte, f Frame, m Mode) (p [65]byte) {
	b := p[1:]
	b[0], b[1], b[2] = 0x38, 0xB5, cmd
	b[3] = 1 // CPU slot
	copy(b[4:7], f.Digits[:])
	b[7] = f.Dot
	b[8] = 2 // GPU slot; ZEUS CAST sends the same value in both
	copy(b[9:12], f.Digits[:])
	b[12] = f.Dot
	b[13] = f.Flash
	if cmd == cmdSet {
		b[14] = 1 // degrees C (0 = F)
	}
	b[15] = byte(m)
	for _, x := range b[:16] {
		b[16] += x
	}
	return p
}

// Reply is a report from the display.
type Reply [64]byte

// valid reports whether the first n bytes are a reply: the header, then the
// sum of bytes 0-10 in byte 11.
func (r Reply) valid(n int) bool {
	if n < 12 || r[0] != 0x38 || r[1] != 0xB5 {
		return false
	}
	var sum byte
	for _, x := range r[:11] {
		sum += x
	}
	return sum == r[11]
}

// Mode is the mode the display is in.
func (r Reply) Mode() Mode { return Mode(r[5]) }

// Firmware is the display's firmware version.
func (r Reply) Firmware() (major, minor byte) { return r[3], r[4] }

// Ambient is the reading of the display's own sensor, in tenths of a degree C.
func (r Reply) Ambient() int {
	t := int(r[7])*100 + int(r[8])*10 + int(r[9])
	if r[10] != 1 { // no decimal point: whole degrees
		t *= 10
	}
	if r[6] == 0 { // reported in Fahrenheit
		t = (t - 320) * 5 / 9
	}
	return t
}
