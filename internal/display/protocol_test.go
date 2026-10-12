package display

import (
	"bytes"
	"fmt"
	"testing"
)

// realReply is what the hardware answered: firmware 1.2, CPU mode, ambient 26.8 C.
var realReply = []byte{0x38, 0xb5, 0x01, 0x01, 0x02, 0x01, 0x01, 0x02, 0x06, 0x08, 0x01, 0x04}

func TestReport(t *testing.T) {
	// The expected bytes come from the ZEUS CAST logic (its class HIDB538II).
	for _, c := range []struct {
		name string
		got  [65]byte
		want string
	}{
		{"query", report(cmdQuery, Frame{}, Keep), "38 b5 ff 01 00 00 00 00 02 00 00 00 00 00 00 ff ee"},
		{"48.8", report(cmdSet, Temperature(488), Keep), "38 b5 01 01 04 08 08 01 02 04 08 08 01 00 01 ff 1b"},
		{"48.9 and GPU mode", report(cmdSet, Temperature(489), GPU), "38 b5 01 01 04 08 09 01 02 04 08 09 01 00 01 02 20"},
		{"flashing 777", report(cmdSet, Frame{Digits: [3]byte{7, 7, 7}, Flash: 1}, Keep), "38 b5 01 01 07 07 07 00 02 07 07 07 00 01 01 ff 1c"},
		{"checksum wraps", report(cmdSet, Frame{[3]byte{255, 255, 255}, 255, 255}, Keep), "38 b5 01 01 ff ff ff ff 02 ff ff ff ff ff 01 ff e8"},
	} {
		if c.got[0] != 0 {
			t.Errorf("%s: report number is %d, want 0", c.name, c.got[0])
		}
		if got := fmt.Sprintf("% x", c.got[1:18]); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", c.name, got, c.want)
		}
		if !bytes.Equal(c.got[18:], make([]byte, 47)) {
			t.Errorf("%s: padding is not zero", c.name)
		}
	}
}

func TestMode(t *testing.T) {
	for m, name := range map[Mode]string{Off: "off", CPU: "cpu", GPU: "gpu", Ambient: "amb"} {
		if m.String() != name {
			t.Errorf("Mode(%d).String() = %q, want %q", m, m, name)
		}
		if got, ok := ParseMode(name); !ok || got != m {
			t.Errorf("ParseMode(%q) = %d, %v; want %d", name, got, ok, m)
		}
	}
	for _, bad := range []string{"", "CPU", "ambient", "keep", "255", "cpu "} {
		if m, ok := ParseMode(bad); ok {
			t.Errorf("ParseMode(%q) = %v, want no mode", bad, m)
		}
	}
	if got := Mode(4).String() + " " + Keep.String(); got != "4 255" {
		t.Errorf("unknown modes print as %q, want their numbers", got)
	}
}

func TestReplyValid(t *testing.T) {
	var r Reply
	copy(r[:], realReply)
	if !r.valid(64) || !r.valid(12) {
		t.Error("the real reply should be valid")
	}
	if r.valid(11) || r.valid(0) || r.valid(-1) {
		t.Error("a reply shorter than 12 bytes should be invalid")
	}
	for i := 0; i < 12; i++ {
		bad := r
		bad[i] ^= 0x10
		if bad.valid(64) {
			t.Errorf("a reply with byte %d corrupted should be invalid", i)
		}
	}
	// Also from the hardware: every mode, with the ambient sensor at 26.4 C.
	for mode, checksum := range map[Mode]byte{CPU: 0x00, GPU: 0x01, Ambient: 0x02, Off: 0xff} {
		got := Reply{0x38, 0xb5, 0x01, 0x01, 0x02, byte(mode), 0x01, 0x02, 0x06, 0x04, 0x01, checksum}
		if !got.valid(64) || got.Mode() != mode || got.Ambient() != 264 {
			t.Errorf("the real reply in mode %v: valid %v, mode %v, ambient %d", mode, got.valid(64), got.Mode(), got.Ambient())
		}
	}
	if (Reply{}).valid(64) {
		t.Error("an all-zero reply should be invalid")
	}
}

func TestReplyFields(t *testing.T) {
	var real Reply
	copy(real[:], realReply)
	if major, minor := real.Firmware(); major != 1 || minor != 2 {
		t.Errorf("firmware = %d.%d, want 1.2", major, minor)
	}
	if real.Mode() != CPU {
		t.Errorf("mode = %v, want cpu", real.Mode())
	}
	mk := func(unit, d1, d2, d3, dot byte) Reply {
		return Reply{0x38, 0xb5, 1, 1, 2, 1, unit, d1, d2, d3, dot}
	}
	for _, c := range []struct {
		r    Reply
		want int
	}{
		{real, 268},                       // 26.8 C
		{mk(1, 0, 2, 6, 0), 260},          // 26 C, whole degrees
		{mk(1, 0, 0, 0, 1), 0},            // 0.0 C
		{mk(0, 8, 0, 2, 1), 267},          // 80.2 F
		{mk(0, 1, 0, 4, 0), 400},          // 104 F
		{mk(0, 0, 3, 2, 0), 0},            // 32 F
		{mk(1, 9, 9, 9, 0), 9990},         // largest whole number
		{mk(1, 255, 255, 255, 0), 283050}, // garbage digits must not panic or overflow
	} {
		if got := c.r.Ambient(); got != c.want {
			t.Errorf("ambient of % x = %d, want %d", c.r[:11], got, c.want)
		}
	}
}
