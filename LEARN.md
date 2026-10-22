# Learn Go by building a headphone controller

**Who this is for:** you have never programmed in Go, and maybe not in any language. By the end you
will have written a program that talks to Bluetooth headphones over a private data channel, read the
battery, and switched noise cancelling on and off. Then you will see how the finished tool
(`ultcontrol`) and its desktop plugin are built from the same pieces.

Every code sample in this guide has been compiled and run. They live in the `tutorial/` folder next to
this file, so you can compare your typing with a working copy.

Estimated time: a weekend, going slowly. Do the lessons in order; each one uses the previous one.

---

## Contents

0. [What you are building](#0-what-you-are-building)
1. [Set up your computer](#1-set-up-your-computer)
2. [Lesson 1: Go in 30 minutes](#2-lesson-1-go-in-30-minutes)
3. [Lesson 2: How the headphones talk](#3-lesson-2-how-the-headphones-talk)
4. [Lesson 3: Building messages (and testing them)](#4-lesson-3-building-messages-and-testing-them)
5. [Lesson 4: Reading messages that arrive in pieces](#5-lesson-4-reading-messages-that-arrive-in-pieces)
6. [Lesson 5: First contact with the real headphones](#6-lesson-5-first-contact-with-the-real-headphones)
7. [Lesson 6: Asking questions: name, battery, noise cancelling](#7-lesson-6-asking-questions-name-battery-noise-cancelling)
8. [Lesson 7: Changing settings (carefully)](#8-lesson-7-changing-settings-carefully)
9. [Lesson 8: How the real project is organised](#9-lesson-8-how-the-real-project-is-organised)
10. [Lesson 9: The terminal UI](#10-lesson-9-the-terminal-ui)
11. [Lesson 10: The desktop plugin](#11-lesson-10-the-desktop-plugin)
12. [Troubleshooting](#12-troubleshooting)
13. [Exercises](#13-exercises)
14. [Glossary](#14-glossary)

---

## 0. What you are building

Sony's phone app changes headphone settings through a **second, private Bluetooth connection**, separate
from the one that plays music. Messages on that connection are short strings of numbers. For example,
"what is your battery level?" is just the two numbers `22 00`.

Your job is to:

1. open that private connection from Linux,
2. wrap numbers in the envelope the headphones expect,
3. send questions and understand the answers,
4. put a friendly face on it.

```
   you
    │  type a command, or click a button
    ▼
 your Go program ──────────────►  headphones
   (builds messages,               (answer with numbers)
    reads answers)
```

You do not need to understand Bluetooth radio. Linux already handles that; we only ask it to hand
us the finished connection.

---

## 1. Set up your computer

This guide is written for Linux (it was built on CachyOS/Arch).

You need:

| Thing | Why | Check it |
|---|---|---|
| **Go** 1.22 or newer | the language | `go version` |
| **BlueZ** | Linux's Bluetooth service (almost always installed) | `bluetoothctl --version` |
| Your headphones **paired and connected** | so there is something to talk to | `bluetoothctl devices` |
| A text editor | VS Code, Neovim, anything | |

Install Go on Arch: `sudo pacman -S go`. On Ubuntu/Debian: `sudo apt install golang`.

Find your headphones' address (the **MAC address**, six pairs of hex digits):

```sh
bluetoothctl devices
# Device AA:BB:CC:DD:EE:FF ULT WEAR
```

Write yours down. Wherever you see `AA:BB:CC:DD:EE:FF` below, use yours.

**Important rule for the whole guide:** only one program can use the headphones' private channel at a
time. Close the Sony phone app, and do not run two of your programs at once.

Make a folder for your work:

```sh
mkdir ~/learn-go && cd ~/learn-go
go mod init learn
```

`go mod init learn` creates a file called `go.mod`. It tells Go that this folder is one project
(a *module*) named `learn`.

---

## 2. Lesson 1: Go in 30 minutes

You only need a small part of Go for this project. Type this program yourself instead of pasting it;
typing is how it sticks.

Create `basics/main.go`:

```go
package main

import "fmt"

// add takes two numbers and returns their sum.
func add(a int, b int) int {
	return a + b
}

// checksum adds up all the bytes. A byte can only hold 0-255, so the total
// wraps around automatically (this is exactly what the headphones expect).
func checksum(data []byte) byte {
	var sum byte
	for _, b := range data {
		sum += b
	}
	return sum
}

type Battery struct {
	Level    int
	Charging bool
}

func (b Battery) Describe() string {
	if b.Charging {
		return fmt.Sprintf("%d%% (charging)", b.Level)
	}
	return fmt.Sprintf("%d%%", b.Level)
}

func main() {
	fmt.Println(add(2, 3))

	data := []byte{0x04, 0x01, 0xFF}
	fmt.Printf("% x\n", data)
	fmt.Println(checksum(data))

	b := Battery{Level: 81}
	fmt.Println(b.Describe())
}
```

Run it:

```sh
go run ./basics
```

You should see:

```
5
04 01 ff
4
81%
```

### What each part means

**`package main`**: every file begins by saying which *package* (group of code) it belongs to. A package
named `main` is a program you can run.

**`import "fmt"`**: bring in code written by others. `fmt` ("format") prints text.

**Functions.** 

```go
func add(a int, b int) int {
	return a + b
}
```

`func` starts a function, `add` is its name, `(a int, b int)` are the inputs and their types, and the
`int` after the brackets is the type of the result. Go always wants types written down.

**Variables.**

```go
var sum byte    // creates sum, starts at 0
data := []byte{0x04, 0x01, 0xFF}   // := creates and sets in one go
```

**Bytes.** A `byte` is a number from 0 to 255. Computers send everything as bytes. We write them in
**hex** (base 16) because two hex digits are exactly one byte: `0xFF` is 255. `0x` just means "hex follows".

**Slices.** `[]byte` is a list of bytes of any length. `append` adds to a list.

**Loops.** 

```go
for _, b := range data {
	sum += b
}
```

`range` walks through a list. It gives you the position and the value; the `_` means "I do not need the
position". Notice that `sum` is a `byte`, so adding past 255 wraps around to 0. That wrap-around is exactly
what the headphones' checksum needs. `4 + 1 + 255 = 260`, and `260 - 256 = 4`, which is what the program printed.

**Structs and methods.**

```go
type Battery struct {
	Level    int
	Charging bool
}

func (b Battery) Describe() string { ... }
```

A `struct` groups related values. A *method* is a function attached to a type, called as
`b.Describe()`.

**Errors.** Go has no exceptions. A function that can fail returns two things: the result and an `error`.
You always check it:

```go
f, err := Decode(wire)
if err != nil {
	return err      // stop and pass the problem upward
}
```

You will write `if err != nil` hundreds of times. That is normal Go.

**Capital letters.** A name that starts with a capital (`Encode`, `Frame`) can be used from other packages.
A lowercase name (`escape`) is private. This is the only access rule Go has.

### Check yourself

Change `checksum` to also work on an empty list. (It already does. Try it: `checksum(nil)`.) Then predict what
`checksum([]byte{200, 100})` prints, and run it to see if you were right. (Answer: 44, because 300 − 256.)

---

## 3. Lesson 2: How the headphones talk

Every message, in either direction, is wrapped in an **envelope** called a *frame*:

```
 3E | type | seq | length (4 bytes) | payload ... | checksum | 3C
 ^                                                              ^
 start marker                                          end marker
```

- **`3E`** and **`3C`** are markers that say "a message starts here" and "it ends here".
- **type**: `0C` means "a command or reply", `01` means "I got your message" (an *acknowledgement*, or ACK).
- **seq**: a counter that flips between 0 and 1, so each side can tell new messages from repeats.
- **length**: how many payload bytes follow (4 bytes, big-endian, which just means the biggest digit comes first).
- **payload**: the actual command. The first payload byte says what kind it is.
- **checksum**: all the bytes after the start marker, added together, wrapping at 256. It catches corruption.

### The escape rule

What if the payload happens to contain `3C`? The receiver would think the message ended. So inside a
frame, any `3C`, `3D` or `3E` is replaced by two bytes: `3D` followed by the original with one bit
cleared (`byte & 0xEF`). To undo it, you put that bit back (`byte | 0x10`).

### The acknowledgement rule

Every frame the headphones send you **must** be answered with an ACK frame whose `seq` is flipped
(0 becomes 1, 1 becomes 0). If you do not, they keep resending and eventually stop talking to you.

### A worked example

"What is your name?" is payload `04 01`. As a frame, with seq 0:

```
3E  0C  00  00 00 00 02  04 01  13  3C
```

where the checksum `13` is `0C + 00 + 00 + 00 + 00 + 02 + 04 + 01 = 0x13`. You will make your program produce exactly
these bytes.

---

## 4. Lesson 3: Building messages (and testing them)

A good habit: write the test first, then the code. Tests are how you know a piece works without
needing the headphones.

Make the folder `frame/`.

Create `frame/frame_test.go`:

```go
package frame

import (
	"bytes"
	"testing"
)

func TestEncodeKnownMessage(t *testing.T) {
	got := Encode(TypeMDR, 0, []byte{0x04, 0x01})
	want := []byte{0x3E, 0x0C, 0x00, 0x00, 0x00, 0x00, 0x02, 0x04, 0x01, 0x13, 0x3C}
	if !bytes.Equal(got, want) {
		t.Fatalf("got % x, want % x", got, want)
	}
}
```

Run `go test ./frame`. It fails to compile: `Encode` does not exist yet. Good, that is the first step.

Now create `frame/frame.go`. We build it in pieces. First the constants and the escaping:

```go
// Package frame builds and reads the messages the headphones understand.
package frame

import "errors"

const (
	Start byte = 0x3E
	End   byte = 0x3C
	Esc   byte = 0x3D

	TypeAck byte = 0x01
	TypeMDR byte = 0x0C
)

// escape replaces the three special bytes so they cannot be confused with markers.
func escape(in []byte) []byte {
	out := make([]byte, 0, len(in)+4)
	for _, b := range in {
		if b == Start || b == End || b == Esc {
			out = append(out, Esc, b&0xEF)
		} else {
			out = append(out, b)
		}
	}
	return out
}
```

Then `Encode`, which follows the envelope diagram from top to bottom:

```go
// Encode turns a payload into the bytes to send over the wire.
func Encode(dataType, seq byte, payload []byte) []byte {
	body := []byte{dataType, seq, 0, 0, 0, byte(len(payload))}
	body = append(body, payload...)

	var sum byte
	for _, b := range body {
		sum += b
	}
	body = append(body, sum)

	out := []byte{Start}
	out = append(out, escape(body)...)
	return append(out, End)
}
```

(`body` is type, seq, then the 4-byte length. We only support payloads under 256 bytes, so the first three length
bytes are 0. `payload...` means "append every item of this list".)

Run `go test ./frame`. It passes.

### Reading messages back

Now the reverse. Add to the test file:

```go
func TestRoundTrip(t *testing.T) {
	// 0x3C and 0x3E inside the payload must survive escaping.
	payload := []byte{0x3C, 0x3D, 0x3E, 0x01}
	f, err := Decode(Encode(TypeMDR, 1, payload))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(f.Payload, payload) || f.Seq != 1 {
		t.Fatalf("bad round trip: %+v", f)
	}
}

func TestBadChecksum(t *testing.T) {
	wire := Encode(TypeMDR, 0, []byte{0x04, 0x01})
	wire[len(wire)-2]++ // corrupt the checksum
	if _, err := Decode(wire); err == nil {
		t.Fatal("expected an error")
	}
}
```

And add `Decode` to `frame.go`:

```go
// Frame is one decoded message.
type Frame struct {
	DataType byte
	Seq      byte
	Payload  []byte
}

// Decode reads one complete wire message (including the Start and End markers).
func Decode(wire []byte) (Frame, error) {
	if len(wire) < 2 || wire[0] != Start || wire[len(wire)-1] != End {
		return Frame{}, errors.New("missing start or end marker")
	}
	inner := wire[1 : len(wire)-1]

	// undo the escaping
	raw := make([]byte, 0, len(inner))
	for i := 0; i < len(inner); i++ {
		if inner[i] == Esc && i+1 < len(inner) {
			i++
			raw = append(raw, inner[i]|0x10)
		} else {
			raw = append(raw, inner[i])
		}
	}
	if len(raw) < 7 {
		return Frame{}, errors.New("message too short")
	}

	var sum byte
	for _, b := range raw[:len(raw)-1] {
		sum += b
	}
	if sum != raw[len(raw)-1] {
		return Frame{}, errors.New("checksum mismatch")
	}

	n := int(raw[2])<<24 | int(raw[3])<<16 | int(raw[4])<<8 | int(raw[5])
	if 6+n > len(raw)-1 {
		return Frame{}, errors.New("length field is wrong")
	}
	return Frame{DataType: raw[0], Seq: raw[1], Payload: raw[6 : 6+n]}, nil
}
```

`raw[6 : 6+n]` is a **slice expression**: items 6 up to (not including) 6+n. `<<` shifts bits left, so
those four lines glue four bytes into one number.

`go test ./frame` should now pass all three tests.

---

## 5. Lesson 4: Reading messages that arrive in pieces

Bluetooth does not promise "one read equals one message". A read might return half a message, or two
messages stuck together (we will see this happen on the real device). So we need a **Decoder** that
collects bytes and returns whatever complete frames it has.

Add this test:

```go
func TestDecoderHandlesSplitAndGlued(t *testing.T) {
	a := Encode(TypeMDR, 0, []byte{1, 2, 3})
	b := Encode(TypeAck, 1, nil)
	all := append(append([]byte{}, a...), b...)

	var d Decoder
	var frames []Frame
	frames = append(frames, d.Feed(all[:5])...) // first half of a
	frames = append(frames, d.Feed(all[5:])...) // rest of a, plus b
	if len(frames) != 2 {
		t.Fatalf("got %d frames, want 2", len(frames))
	}
}
```

And the code, at the bottom of `frame.go`:

```go
// Decoder collects bytes as they arrive and hands back complete frames.
// Bluetooth may deliver half a message, or two messages glued together.
type Decoder struct {
	buf []byte
}

// Feed adds newly received bytes and returns every frame that is now complete.
func (d *Decoder) Feed(chunk []byte) []Frame {
	var out []Frame
	for _, b := range chunk {
		if b == Start {
			d.buf = d.buf[:0] // a new message begins
		}
		d.buf = append(d.buf, b)
		if b == End && len(d.buf) > 1 && d.buf[0] == Start {
			if f, err := Decode(d.buf); err == nil {
				f.Payload = append([]byte(nil), f.Payload...) // copy: buf is reused
				out = append(out, f)
			}
			d.buf = d.buf[:0]
		}
	}
	return out
}
```

Two Go ideas appear here:

- `func (d *Decoder) Feed` has a `*`: a **pointer**. It means "work on the real Decoder, not a copy", so
  `d.buf` keeps its contents between calls.
- The line with `append([]byte(nil), ...)` makes a private copy, because `d.buf` gets reused and would otherwise
  overwrite the payload we just returned. This kind of bug (two things sharing one list) is classic.

Run `go test ./frame`. All green? You now have the whole message layer, and you never needed the headphones.

---

## 6. Lesson 5: First contact with the real headphones

Now the hard part: getting a connection. Here is the honest story, because it explains the odd-looking code.

I first tried opening a raw Bluetooth socket and guessing channel numbers. Many channels accepted my
connection but none answered. I tried to ask the headphones which channel to use; that timed out.

What worked: let **BlueZ** do it. BlueZ is the Linux Bluetooth service, and programs talk to it through
**D-Bus**, a message bus between programs on your machine. The steps:

1. We tell BlueZ: "I handle the profile with this UUID" (a long ID, `956c7b26-…`, that Sony headphones advertise
   for their control channel).
2. We tell BlueZ: "connect that profile on this device".
3. BlueZ opens the connection and calls our `NewConnection` function, handing over a **file descriptor**:
   an open connection that Linux lets us treat exactly like a file. We then `Read` and `Write` it.

Install the D-Bus library in your project:

```sh
go get github.com/godbus/dbus/v5
```

Create `cmd/first/main.go`:

```go
// first contact: connect, ask for the name, print the reply.
package main

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/godbus/dbus/v5"

	"learn/frame"
)

const sonyUUID = "956c7b26-d49a-4ba8-b03f-b17d393cb6e2"

// profile is the object BlueZ calls when the connection is ready.
type profile struct{ conns chan *os.File }

func (p *profile) NewConnection(_ dbus.ObjectPath, fd dbus.UnixFD, _ map[string]dbus.Variant) *dbus.Error {
	syscall.SetNonblock(int(fd), true)
	p.conns <- os.NewFile(uintptr(fd), "rfcomm")
	return nil
}
func (p *profile) RequestDisconnection(dbus.ObjectPath) *dbus.Error { return nil }
func (p *profile) Release() *dbus.Error                             { return nil }

func main() {
	if len(os.Args) != 2 {
		fmt.Println("usage: first AA:BB:CC:DD:EE:FF")
		os.Exit(1)
	}
	mac := os.Args[1]

	bus, err := dbus.ConnectSystemBus()
	if err != nil {
		panic(err)
	}
	defer bus.Close()

	p := &profile{conns: make(chan *os.File, 1)}
	path := dbus.ObjectPath("/learn/profile")
	bus.Export(p, path, "org.bluez.Profile1")

	opts := map[string]dbus.Variant{
		"Role":                  dbus.MakeVariant("client"),
		"RequireAuthentication": dbus.MakeVariant(false),
		"RequireAuthorization":  dbus.MakeVariant(false),
		"AutoConnect":           dbus.MakeVariant(false),
	}
	mgr := bus.Object("org.bluez", "/org/bluez")
	if err := mgr.Call("org.bluez.ProfileManager1.RegisterProfile", 0, path, sonyUUID, opts).Err; err != nil {
		panic(err)
	}
	defer mgr.Call("org.bluez.ProfileManager1.UnregisterProfile", 0, path)

	dev := dbus.ObjectPath("/org/bluez/hci0/dev_" + strings.ReplaceAll(mac, ":", "_"))
	bus.Object("org.bluez", dev).Go("org.bluez.Device1.ConnectProfile", 0, nil, sonyUUID)

	var f *os.File
	select {
	case f = <-p.conns:
	case <-time.After(10 * time.Second):
		fmt.Println("no connection (is another app using the headphones?)")
		os.Exit(1)
	}
	defer f.Close()

	f.Write(frame.Encode(frame.TypeMDR, 0, []byte{0x04, 0x01})) // "what is your name?"

	var dec frame.Decoder
	buf := make([]byte, 256)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		n, err := f.Read(buf)
		if err != nil {
			time.Sleep(50 * time.Millisecond) // nothing yet, try again
			continue
		}
		for _, fr := range dec.Feed(buf[:n]) {
			if fr.DataType != frame.TypeMDR {
				continue // an acknowledgement; ignore it
			}
			fmt.Printf("name: %s\n", fr.Payload[3:])
			f.Write(frame.Encode(frame.TypeAck, fr.Seq^1, nil))
			return
		}
	}
}
```

Build and run it. Use your own address. The headphones must be connected and the Sony app closed:

```sh
go build -o first ./cmd/first
./first AA:BB:CC:DD:EE:FF
```

Expected output:

```
name: ULT WEAR
```

**You just talked to your headphones.**

### What to notice in the code

- `type profile struct{ conns chan *os.File }`: a **channel** is Go's pipe between pieces of code. BlueZ's
  callback puts the connection into the pipe; `main` takes it out with `<-p.conns`. That is how two parts that run
  at different times hand something over safely.
- `select { case f = <-p.conns: ... case <-time.After(10 * time.Second): ... }` waits for whichever happens
  first: a connection, or ten seconds passing.
- `defer f.Close()` means "run this when the function ends", whatever way it ends. `defer` is how Go cleans up.
- `fr.Seq^1` flips 0 and 1 (`^` is XOR). That is the acknowledgement rule from Lesson 2.
- `panic(err)` crashes immediately. Fine for a learning program; the real project returns errors instead.

---

## 7. Lesson 6: Asking questions: name, battery, noise cancelling

Asking one question works. Asking many needs a helper. The helper must:

1. send a command,
2. flip the sequence number,
3. read until the matching reply arrives,
4. **acknowledge every frame it receives** (not just the one it wants).

The reply to command `X` starts with the byte `X + 1`: name `04` gets `05`, battery `22` gets `23`, noise
cancelling `66` gets `67`.

Create `cmd/second/main.go`. It begins like `first` and ends with the helper and three questions. This is the
part that is new:

```go
	// ask sends one command and waits for the reply whose first byte is replyCmd.
	ask := func(payload []byte, replyCmd byte) []byte {
		f.Write(frame.Encode(frame.TypeMDR, seq, payload))
		seq ^= 1 // the sequence number alternates 0, 1, 0, 1 ...
		buf := make([]byte, 256)
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			n, err := f.Read(buf)
			if err != nil {
				time.Sleep(50 * time.Millisecond)
				continue
			}
			for _, fr := range dec.Feed(buf[:n]) {
				if fr.DataType != frame.TypeMDR {
					continue
				}
				f.Write(frame.Encode(frame.TypeAck, fr.Seq^1, nil)) // always acknowledge
				if len(fr.Payload) > 0 && fr.Payload[0] == replyCmd {
					return fr.Payload
				}
			}
		}
		return nil
	}

	if r := ask([]byte{0x04, 0x01}, 0x05); r != nil {
		fmt.Printf("name:    %s\n", r[3:])
	}
	if r := ask([]byte{0x22, 0x00}, 0x23); r != nil {
		fmt.Printf("battery: %d%%\n", r[2])
	}
	if r := ask([]byte{0x66, 0x17}, 0x67); r != nil {
		mode := "noise cancelling"
		if r[4] == 1 {
			mode = "ambient sound"
		}
		fmt.Printf("mode:    %s, ambient level %d\n", mode, r[6])
	}
}
```

Run it:

```sh
go build -o second ./cmd/second && ./second AA:BB:CC:DD:EE:FF
```

```
name:    ULT WEAR
battery: 81%
mode:    ambient sound, ambient level 20
```

### How to read the answers

| Question | Reply bytes | Meaning |
|---|---|---|
| `22 00` | `23 00 LEVEL CHARGING` | `r[2]` is battery %, `r[3]` is 1 if charging |
| `66 17` | `67 17 01 ENABLED MODE 00 LEVEL` | `r[3]` on/off, `r[4]` mode, `r[6]` ambient level |
| `04 01` | `05 01 LEN TEXT...` | the name, starting at byte 3 |

For `MODE`: **`0` = noise cancelling, `1` = ambient sound**. (This project originally had these swapped;
the fix was to read the protocol definition instead of guessing. See the end of Lesson 7.)

`closures`: `ask := func(...) {...}` defines a function inside a function that can use the variables around it
(`f`, `dec`, `seq`). Handy for small helpers.

---

## 8. Lesson 7: Changing settings (carefully)

Everything so far only *asks*. Changing settings *writes* to the headphones, so slow down.

**Rules for safe experiments**

1. Read and write down the current value before changing anything.
2. Change one thing at a time.
3. Read it back to confirm.
4. Put it back.
5. Use a documented format. Guessing the bytes can leave the device in a strange state. (During this
   project, a guessed EQ command did exactly that. It was recoverable only because the original bytes
   had been written down.)

To set noise cancelling or ambient sound, send `68 17 01 ENABLED MODE 00 LEVEL`; the headphones answer `69 17 ...`.
Add this to the end of `main` in `second`, and note that it uses the values you just read:

```go
// switch to noise cancelling (mode 0), keep it enabled, keep the same level
current := ask([]byte{0x66, 0x17}, 0x67)
if current != nil {
	level := current[6]
	reply := ask([]byte{0x68, 0x17, 0x01, 0x01, 0x00, 0x00, level}, 0x69)
	fmt.Printf("set reply: % x\n", reply)
}
```

Wait about 6 seconds between runs, because the headphones refuse reconnects right after you disconnect. Listen:
the world should go quiet. Put it back to ambient by changing the mode byte `0x00` to `0x01`.

### The equaliser (EQ)

Same idea, different commands:

| Command | Meaning |
|---|---|
| `56 03` | read EQ. Reply `57 03 PRESET ULT 06 b1 b2 b3 b4 b5 b6` |
| `58 03 PRESET ULT 00` | select a preset |
| `58 03 A0 ULT 06 b1..b6` | set custom bands |

The six bands are Clear Bass, 400 Hz, 1 kHz, 2.5 kHz, 6.3 kHz and 16 kHz. Each gain is sent as **dB + 10**, so
−10 dB is 0 and +10 dB is 20. Preset numbers: `00` Off, `10` Bright, `11` Excited, `12` Mellow, `13` Relaxed,
`14` Vocal, `15` Treble, `16` Bass, `17` Speech, `A0` Custom. `ULT` is 0 (off), 1 or 2.

### How I found these numbers

- I did not invent them. I found an open-source project, **SonyHeadphonesClient**, with a library
  (`libmdr`) that documents Sony's protocol.
- I read the definitions, then tried only the *read* commands on the real headphones and compared the
  answers with what the documentation predicted.
- Only after reads matched did I try writes, with the original value saved.
- When something is unclear, look at what an existing app does before you guess. Searching for "has someone
  already documented this?" is the most valuable first step in any reverse-engineering project.

---

## 9. Lesson 8: How the real project is organised

Your two programs repeat themselves. The real tool splits the work into layers, each with one job:

```
cmd/ultcontrol/main.go     the command line: status, mode, eq, raw, --json
cmd/ultcontrol/tui.go      the terminal UI
internal/sony/frame.go     Encode / Decode / Decoder     (your Lessons 3 and 4)
internal/sony/bluez.go     getting the connection         (your Lesson 5)
internal/sony/device.go    Name(), Battery(), NcAsm(), SetNcAsm()   (your Lessons 6 and 7)
internal/sony/eq.go        EQ read and write
```

`internal/` is a Go convention: packages in it can only be used by this project.

The biggest difference is `device.go`. Instead of reading and writing in the same place, it runs a
**reader goroutine**. A *goroutine* is a function running alongside the others, started with `go`:

```go
h := &Headphones{...}
go h.readLoop()   // runs in the background from now on
```

The loop does three things forever:

1. read bytes and decode frames,
2. **acknowledge every frame** immediately,
3. pass the frame to whoever asked (via a channel), or, if nobody asked, treat it as a **notification**. Notifications
   happen when you press the button on the headphones; the TUI uses them to update live.

A request then becomes simple:

```go
func (h *Headphones) request(payload []byte, replyCmd byte) ([]byte, error)
```

It registers "I am waiting for a reply starting with `replyCmd`", sends the command, and waits up to 3
seconds. This is why the TUI can show changes the moment you press the headphone button, which `second`
cannot.

Other things in `bluez.go` that you did not need yet:

- **retry on "busy"**: after you disconnect, the headphones refuse new connections for a few seconds, so `dial` waits 1.5 s
  and tries again until a deadline;
- `FindDevice()` looks through BlueZ's device list for a name starting with "ULT", so you do not have to type the MAC.

Read those files next. After this guide, none of the Go in them should be a mystery.

The CLI in `main.go` is a `switch` on the first word you type:

```go
switch args[0] {
case "status": return status(h)
case "mode":   return setMode(h, args[1:])
case "eq":     return eqCmd(h, args[1:])
}
```

Try it:

```sh
go build -o bin/ultcontrol ./cmd/ultcontrol
bin/ultcontrol status
bin/ultcontrol mode nc
bin/ultcontrol eq preset bass
bin/ultcontrol --json status
```

The `--json` flag matters later: it prints one line like `{"battery":81,"mode":"nc",...}` that other programs can read.

---

## 10. Lesson 9: The terminal UI

`cmd/ultcontrol/tui.go` uses **Bubble Tea**, a library for terminal apps. It is built on one idea, "the Elm
architecture", and once you see it, every screen is easy:

- a **model**: a struct holding everything the screen shows (selected row, battery, EQ values),
- an **Update** function: it receives an event (a key press, a notification from the headphones, a timer) and
  returns the new model,
- a **View** function: it turns the model into text.

```
 key press ──► Update(model, key) ──► new model ──► View(model) ──► text on screen
```

You never draw by hand. You change the model, and the screen follows. Keys: arrows or `j`/`k` pick a row, left/right
(or `h`/`l`) change it, `q` quits. Editing an EQ band switches the preset to Custom, because the numbers no longer match
a preset.

To learn it, read the [Bubble Tea tutorials](https://github.com/charmbracelet/bubbletea/tree/main/tutorials), then open `tui.go` and find
`Update` and `View`.

---

## 11. Lesson 10: The desktop plugin

The plugin is for the **Noctalia** shell (the panel and bar on this laptop). It is not Go. It uses **Luau**, a small safe
dialect of Lua. A plugin is a folder:

```
noctalia-plugin/
  plugin.toml          name, id, settings, which entries exist
  service.luau         runs ultcontrol, owns the state
  bar.luau             the little icon in the bar
  panel.luau           the pop-up with the controls
  translations/en.json text shown to the user
```

The design rule is the same as everywhere in this project: **one piece talks to the headphones, everyone else talks to that
piece.**

```
 bar.luau ──┐                         ┌── reads ult.status
            ├── shared state ◄────────┤
 panel.luau ┘   (noctalia.state)      └── writes ult.cmd
                     ▲   │
                     │   ▼
                 service.luau ── runs ──►  ultcontrol --json ...
```

- The **service** runs `ultcontrol --json status` every two minutes and after every command, then stores the JSON in
  `ult.status`.
- The **bar** watches `ult.status` and draws the battery percentage.
- The **panel** draws buttons. A button press writes something like `3|mode nc` into `ult.cmd`; the service sees it and
  runs the command.

Why not talk to Bluetooth from every piece? Because only one program may hold the channel, and the service
runs one command at a time so nothing collides.

Install steps (for this machine):

```sh
go build -o bin/ultcontrol ./cmd/ultcontrol
install bin/ultcontrol ~/.local/bin/
mkdir -p ~/.local/share/noctalia/plugins
cp -r noctalia-plugin ~/.local/share/noctalia/plugins/ultcontrol
noctalia msg plugins enable mavxd/ultcontrol
```

Then add the widget to the bar. Noctalia has two configuration files: `~/.config/noctalia/config.toml` is yours, and
`~/.local/state/noctalia/settings.toml` is edited by the settings app and **wins** when they disagree. Define the
widget in the first and put its name in a bar list in whichever file is in charge:

```toml
[widget.ultcontrol]
type = "mavxd/ultcontrol:bar"
```

Check the plugin with `noctalia plugins lint noctalia-plugin`.

---

## 12. Troubleshooting

| What you see | What it means | What to do |
|---|---|---|
| `could not open control channel` | The headphones are off, not connected, or another app has the channel | Check `bluetoothctl info <MAC>`; close the Sony app and any other `ultcontrol`; wait 10 seconds |
| `br-connection-busy` or it works only every other time | The headphones refuse reconnects right after a disconnect | Wait 6 seconds between runs |
| `Host is down` | The headphones dropped off Bluetooth (battery low or powered off) | Switch them off and on |
| The program hangs | You are waiting for a reply that never comes | Always use a timeout (every `ask` and `request` does) |
| Works, then the next command fails | Another instance (the plugin!) was polling | Plugin polls every 120 s; try again in a few seconds |
| `no such file or directory: /org/bluez/hci0/...` | Wrong MAC address, or your adapter is not `hci0` | Re-check `bluetoothctl devices` and `hciconfig` |
| Widget not in the bar | `settings.toml` overrides `config.toml` | Add it to the bar list in `~/.local/state/noctalia/settings.toml` |
| Buttons do the opposite thing | The two mode values are swapped | `0` is noise cancelling, `1` is ambient |

A useful debugging habit: print the **raw bytes** you send and receive, with `fmt.Printf("% x\n", data)`. Most
protocol bugs are obvious once you can see the bytes.

---

## 13. Exercises

Easy:

1. Print whether the headphones are charging (`r[3]` of the battery reply).
2. Print the firmware version. (`04 02`; reply starts with `05`; the text starts at byte 3.)
3. Make `second` take the number of times to ask for the battery, and print it once per second.

Medium:

4. Read the EQ with `56 03` and print each band name with its gain (remember to subtract 10).
5. Write a function `eqPreset(ask, preset byte)` that selects a preset, then reads the EQ back and prints it.
6. Add a unit test for `Decode` that feeds it a frame with a wrong length field.

Harder:

7. Replace the polling loop in `ask` with a goroutine that does all the reading, like `readLoop` in `device.go`.
8. Print a message whenever you press the noise-cancelling button on the headphones. (Look for reply byte `0x69`
   arriving without you asking.)
9. Look at what `06 xx` returns ("capability list") and write down what each entry might mean. Look it up in `libmdr`
   before you assume.

---

## 14. Glossary

- **Byte**: a number from 0 to 255.
- **Hex**: writing numbers in base 16 (`0xFF` = 255). Two hex digits = one byte.
- **MAC address**: the unique address of a Bluetooth device, like `AA:BB:CC:DD:EE:FF`.
- **BlueZ**: the Linux Bluetooth service.
- **D-Bus**: the message bus programs on Linux use to talk to services such as BlueZ.
- **RFCOMM**: a Bluetooth channel that behaves like a serial cable. The headphones' control channel is one.
- **UUID**: a long ID naming a kind of Bluetooth service.
- **File descriptor**: a number Linux uses for an open connection or file. You can read and write it.
- **Frame**: one complete message with markers, length and checksum.
- **Payload**: the useful part inside a frame (the actual command).
- **Checksum**: a small number computed from the data to detect corruption.
- **ACK**: "I received your message".
- **Goroutine**: a function running in the background (`go f()`).
- **Channel**: a pipe for sending values between goroutines.
- **Module**: a Go project (has a `go.mod` file).
- **Package**: a folder of Go files that belong together.
- **Slice**: a list of values that can grow (`[]byte`).
- **Struct**: a group of named fields.
- **Pointer**: a reference to a value, so you change the original rather than a copy.
- **TUI**: Text User Interface, a full-screen app inside the terminal.
- **Notification** (protocol): a message the headphones send by themselves when something changes.
- **NC**: noise cancelling. **ASM** / ambient sound: lets outside sound in.
- **EQ**: equaliser, which makes some pitches louder or quieter.

---

## Where to learn more Go

- [A Tour of Go](https://go.dev/tour/): the official interactive tutorial. Do it alongside Lesson 1.
- [Go by Example](https://gobyexample.com/): short runnable snippets for each feature.
- [Effective Go](https://go.dev/doc/effective_go): once you are comfortable.

The finished project in this folder is only about 1,100 lines of Go. After this guide you should be able to read all of it.
