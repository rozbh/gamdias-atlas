# atlas

An open-source command-line tool for the GAMDIAS **ATLAS Digital Display** (USB
`1b80:b538`), the three-digit temperature readout on the ATLAS P1 case.

It does the job of GAMDIAS's ZEUS CAST without ZEUS CAST: it sends the CPU or GPU
temperature to the display. It also shows your own numbers and plays small
animations, which ZEUS CAST does not offer for this display.

- **The program** is a small Go program with no dependencies. It runs on Linux.
- **The protocol** is written down in full [below](#protocol), so the display can
  be driven from any system and any language.

> **Unofficial.** This project is not made or endorsed by GAMDIAS. It has been
> used on one display, with firmware 1.2.

## Install

1. Get the program. Either download it from the
   [latest release](https://github.com/rozbh/gamdias-atlas/releases/latest):

   ```sh
   mkdir -p ~/.local/bin
   curl -L -o ~/.local/bin/atlas \
     https://github.com/rozbh/gamdias-atlas/releases/latest/download/atlas-linux-amd64
   chmod +x ~/.local/bin/atlas
   ```

   or build it with Go 1.22 or newer:

   ```sh
   go install github.com/rozbh/gamdias-atlas/cmd/atlas@latest
   ```

2. Let your user open the display without `sudo`:

   ```sh
   sudo curl -L -o /etc/udev/rules.d/60-gamdias-atlas.rules \
     https://github.com/rozbh/gamdias-atlas/releases/latest/download/60-gamdias-atlas.rules
   sudo udevadm control --reload && sudo udevadm trigger
   ```

3. Check that the display answers:

   ```sh
   atlas status
   ```

The release also has `atlas-linux-arm64` and a `SHA256SUMS` file.

## Usage

```sh
atlas status       # firmware version, current mode, ambient sensor, raw reply
atlas              # feed the CPU or GPU temperature until Ctrl-C
atlas mode gpu     # what the display shows: cpu | gpu | amb | off
```

Leave `atlas` running: the display has no other way to learn the temperatures.
The CPU is read from `/sys/class/hwmon` (`k10temp`, `coretemp`, `zenpower`); the GPU
from `nvidia-smi` if present, otherwise `amdgpu` or `nouveau`. `amb` is a sensor
inside the display itself.

- The GPU reader only starts once the display is in GPU mode. Until it has a
  reading, or if there is no GPU sensor, the display shows the CPU.
- If the CPU sensor, the display or `nvidia-smi` goes away, `atlas` exits with a
  message instead of showing a stale number. The systemd unit below restarts it.
- It costs about 0.01 % of one CPU core and 6 MB of memory. In GPU mode,
  `nvidia-smi` adds about 28 MB.

To start it at login, save this as `~/.config/systemd/user/atlas.service` and run
`systemctl --user enable --now atlas`:

```ini
[Service]
ExecStart=%h/.local/bin/atlas
Restart=always
RestartSec=3

[Install]
WantedBy=default.target
```

## Showing your own values

```sh
atlas show 123     # three digits
atlas show 12.3    # with the decimal point
atlas show 7       # shorter values are padded with zeros: 007
```

With no argument, `show` reads lines from stdin and displays the latest one, so the
custom logic can live in a shell script:

```sh
while sleep 1; do date +%S; done | atlas show        # seconds
while sleep 2; do nvidia-smi --query-gpu=utilization.gpu \
    --format=csv,noheader,nounits; done | atlas show # GPU load in percent
```

`show` keeps running until Ctrl-C. Stop the background service first
(`systemctl --user stop atlas`), or the two will take turns on the display.
A line longer than 64 KB ends the input; the last value stays on the display.

## Fun mode

```sh
atlas fun          # play every animation in a loop
atlas fun slot     # play only one
```

The animations are made of digits, since that is all the display can draw:

| Name     | What it does                                              |
|----------|-----------------------------------------------------------|
| `fill`   | Boxes fill up and drain (`0` is an empty box, `8` a full one) |
| `ball`   | A full box bounces left and right                         |
| `wave`   | A wave of brightness moves across the digits              |
| `slot`   | Slot machine: random digits, then `777` flashing          |
| `launch` | Countdown `3 2 1`, then `000` flashing                    |
| `sos`    | `505` flashing                                            |
| `words`  | `800` `808` `816` `200` `704`: BOO BOB BIG ZOO TOY        |
| `eyes`   | `606` and `909` alternating                               |
| `pi`     | The digits of pi scrolling past                           |

They live in one table, [internal/anim/animations.go](internal/anim/animations.go),
one line each. To add your own, add a line and build again:

```go
{"mine", "123 12.3 ???*5 888!*10"},
```

A frame is three digits with an optional dot. `?` is a random digit, a trailing `!`
makes the display flash, and `*N` holds the frame for N ticks of 150 ms. A mistake
in the table is reported when you run `atlas fun`, naming the animation and frame.

Stopping `fun`, `show` or `raw` with Ctrl-C turns the flash off first, so the
display is not left blinking.

### What the display can draw

The protocol sends a *value* for each digit, not the seven segments, so the display
can only draw what its firmware has a shape for.

- **Works:** digits `0`-`9`, and the decimal point before the last digit. This is
  everything ZEUS CAST ever sends.
- **Does nothing:** values 10-40, tested on the hardware. That includes 32, which
  ZEUS CAST uses as a blank digit on the sibling BOREAS and CHIONE displays. So
  there is no blank, and `show` pads with zeros.
- **Untested:** values 41-255, and dot flags other than 0 and 1.

`raw` sends any byte values, so the untested ones can be tried:

```sh
timeout 1 atlas show 888   # put something known on the display first
for i in $(seq 41 255); do echo $i; timeout 1 atlas raw $i $i $i; done
atlas raw 1 2 3 2       # fourth byte is the dot flag
atlas raw 1 2 3 0 1     # fifth byte is the flash flag
```

If the display changes, the number last printed in the terminal is the value that
did it. Add it to the table in [internal/display/glyphs.go](internal/display/glyphs.go)
and `show` will accept that character. A pull request with what you find is welcome.

## Left out

Fahrenheit, and flashing the display when a temperature is too high. Fahrenheit is
byte 14 in `report()` in [internal/display/protocol.go](internal/display/protocol.go);
the flash flag is already there (`Frame.Flash`).

Other GAMDIAS displays are not supported. ZEUS CAST drives the BOREAS (`b53a`) and
CHIONE (`b53c`) digital displays with similar-looking reports that start with
`3A B5`, which may make them a small addition for someone who owns one.

## Building and testing

```sh
git clone https://github.com/rozbh/gamdias-atlas
cd gamdias-atlas
make build        # makes bin/atlas
make test         # all tests, with the race detector; a few seconds
make check        # formatting and go vet
make install      # copies bin/atlas to ~/.local/bin
sudo make udev    # installs the udev rule
make clean        # removes everything that was built
```

The tests need no hardware. A fake display on the other end of a pipe answers
like the real one, and temporary directories stand in for `/sys`. They cover the
report bytes (against the ZEUS CAST logic and replies captured from a real
display), every limit of the text and number parsers, corrupt and missing replies,
an unplugged display, the sensor readers, and every command.

The three parsers also have fuzz tests, for example
`go test -fuzz=FuzzParse -fuzztime=20s ./internal/display`.

Every push runs the tests on GitHub. Pushing a tag like `v1.2.3` also builds the
binaries and publishes a release ([the workflow](.github/workflows/build.yml)).

### Layout

| Path | What is in it |
|------|---------------|
| `cmd/atlas/` | The program: the commands, and the loops that keep the display fed |
| `internal/display/` | The device and its protocol: reports, replies, modes, what the digits can show |
| `internal/display/displaytest/` | The fake display the tests use |
| `internal/sensor/` | CPU and GPU temperature readers |
| `internal/anim/` | The animation table and its player |
| `udev/` | The rule that lets your user open the display |
| `tools/` | The script that unpacked the ZEUS CAST installer |

## Safety

- `atlas` opens one device, the display, and reads `/sys`. It writes no files, uses
  no network and runs no shell.
- It starts one other program, `nvidia-smi`, found through `PATH` and given fixed
  arguments. Run `atlas` as your own user, which the udev rule makes possible, and
  not with `sudo`.
- It can only send the display's "set" and "query" reports. `raw` and the
  animation table choose the digit, dot and flash bytes and nothing else.
- Replies are used only if the header and checksum are right.
- The udev rule gives access to the user logged in at the machine, not to everyone.

## Protocol

One HID interface, no report IDs, 64-byte reports each way (interrupt endpoints
`0x03` out, `0x81` in). The host sends a report and the display answers with one.
ZEUS CAST sends every 390 ms.

**Host to display**

| Byte  | Value                 | Meaning                                              |
|-------|-----------------------|------------------------------------------------------|
| 0, 1  | `38 B5`               | Header                                               |
| 2     | `01` set, `FF` query  | ZEUS CAST sends one query first, then only sets      |
| 3     | `01`                  | CPU slot tag                                         |
| 4-6   | digit, digit, digit   | CPU value as raw digits 0-9 (not ASCII)              |
| 7     | `00`/`01`             | CPU decimal point: `01` shows `d1 d2 . d3`           |
| 8     | `02`                  | GPU slot tag                                         |
| 9-11  | digit, digit, digit   | GPU value                                            |
| 12    | `00`/`01`             | GPU decimal point                                    |
| 13    | `00`/`01`             | Flash the display (over-temperature alarm)           |
| 14    | `01` C, `00` F        | Unit                                                 |
| 15    | `FF`, or a mode       | `FF` keeps the mode; `01` CPU, `02` GPU, `03` ambient, `00` off |
| 16    | sum of bytes 0-15     | Checksum, low 8 bits                                 |
| 17-63 | `00`                  | Padding                                              |

Digits: in Celsius the value is always tenths (`48.7` is `4 8 7`, dot `1`; capped at
`9 9 9`). In Fahrenheit, below 100 it is tenths with the dot; from 100 up it is whole
degrees without the dot.

**Display to host**

| Byte | Meaning                                                  |
|------|----------------------------------------------------------|
| 0, 1 | `38 B5` header                                           |
| 2    | `01`, also in reply to a query (not an echo of byte 2)   |
| 3, 4 | Firmware version, major and minor                        |
| 5    | Current mode (same codes as byte 15 above)               |
| 6    | Unit of the ambient reading: `01` C, `00` F              |
| 7-9  | Ambient temperature digits                               |
| 10   | Ambient decimal point (`01` means divide by 10)          |
| 11   | Checksum: sum of bytes 0-10, low 8 bits                  |

ZEUS CAST never checks the reply checksum; `atlas` does, and it held for every
reply seen from the real display, in all four modes.

A real reply (firmware 1.2, CPU mode, ambient 26.8 C):

```
38 b5 01 01 02 01 01 02 06 08 01 04
```

On Linux a write to `/dev/hidrawN` is prefixed with one `00` byte (the report number),
so 65 bytes are written and 64 are read back.

Example, 48.8 C in both slots, mode unchanged:

```
38 b5 01 01 04 08 08 01 02 04 08 08 01 00 01 ff 1b
```

### Unverified details

- ZEUS CAST puts the *same* value in both slots, chosen by the mode the display
  reports. `atlas` does the same. An unused newer class in the app (`HIDB538III`)
  fills the CPU and GPU slots separately, so the firmware probably accepts that too.
- Whether the display keeps the last value when nothing is sending. `atlas` and
  `show` therefore keep sending, as ZEUS CAST does.

## How the protocol was found

The protocol was worked out from ZEUS CAST 3.3.1.4, so that the display can be used
on Linux. None of GAMDIAS's files are in this repository.

1. The installer is an Inno Setup 6.5.2 package, newer than `innoextract`
   supported at the time. [tools/innounpack.py](tools/innounpack.py) unpacks it
   without running it: `python3 -I tools/innounpack.py <setup.exe> <outdir>`.
2. `ZEUS CAST.exe` is a .NET 4.8 program and is not obfuscated, so ILSpy turns it
   back into readable C#.
3. In that code, the class `HIDB538II` builds the report and runs the send loop,
   the `AtlasDigital` page holds the mode buttons and the alarm, and
   `DeviceRegistry` lists every device the program knows.
4. The result was then checked against a real display, which is where the reply
   checksum and the ignored digit values come from.

## License

[MIT](LICENSE)
