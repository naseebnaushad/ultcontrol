# Building a controller for Bluetooth headphones, from scratch


This guide explains how `ultcontrol` was built: a command-line tool, a terminal UI and a Noctalia
shell plugin that control Sony WH-ULT900N ("ULT WEAR") headphones on Linux. It assumes you know
basic programming but nothing about Bluetooth, reverse engineering or Go.

The same method works for most Sony headphones, and the ideas carry over to other brands.

---

## 1. The big picture

Sony's phone app does not change settings through the normal Bluetooth audio connection. It opens a
second, private data connection to the headphones and sends small binary messages such as
"turn on noise cancelling" or "set the EQ to Bass". We want to do the same from a laptop.

Three problems to solve:

1. **Open that private connection** from Linux.
2. **Speak the language** the headphones understand (the message format).
3. **Wrap it in something usable** (CLI, TUI, desktop plugin).

```
 you ──► TUI / CLI / Noctalia plugin
              │
              ▼
        Go package sony      (frames, commands, EQ)
              │
              ▼
        BlueZ over D-Bus     (gives us an RFCOMM socket)
              │
              ▼
        headphones
```

## 2. Why these tools

| Choice | Reason |
|---|---|
| **Go** | One static binary, good concurrency, a D-Bus library and a TUI library. |
| **TUI instead of a GUI app** | The goal was a "proper GUI, but in the terminal", with a single codebase. |
| **CLI with `--json`** | Scripts and the shell plugin get a stable, machine-readable interface. |
| **Plugin calls the CLI** | The plugin stays tiny: it runs `ultcontrol --json …` and shows the result. |

Layout:

```
cmd/ultcontrol/main.go     CLI commands, --json, starts the TUI
cmd/ultcontrol/tui.go      Bubble Tea terminal UI
internal/sony/frame.go     wire format: framing, escaping, checksum
internal/sony/bluez.go     getting the Bluetooth socket from BlueZ
internal/sony/device.go    commands: name, battery, noise cancelling
internal/sony/eq.go        equaliser
noctalia-plugin/           Luau plugin for the Noctalia shell
packaging/                 .desktop launcher and icon
```

## 3. Step 1: Find out what is possible

Before writing code, check the headphones with normal tools:

```sh
bluetoothctl devices                      # find the MAC address
bluetoothctl info AA:BB:CC:DD:EE:FF       # look at "UUID:" lines
```

Sony devices list a custom service UUID: `956c7b26-d49a-4ba8-b03f-b17d393cb6e2`. That UUID is the
control channel.

Do not reverse-engineer from zero if you do not have to. Search for existing open-source work first.
Here that was **`mos9527/SonyHeadphonesClient`** and its `libmdr` library, which documents the Sony
"MDR" protocol. The message formats in this project come from reading that source and then
confirming each one on the real headphones. Never trust a protocol document until you have seen the
device answer.

## 4. Step 2: Open the connection (the hard part)

Natural first attempts fail:

- **Scanning RFCOMM channels 1-30 with a raw socket:** several channels accepted connections, but
  none answered.
- **Reading the SDP service record to find the channel:** timed out.

What works is to let **BlueZ** (the Linux Bluetooth daemon) open the socket for us:

1. Register ourselves with BlueZ as a *profile* for the Sony UUID (`ProfileManager1.RegisterProfile`).
2. Ask BlueZ to connect that profile to the headphones (`Device1.ConnectProfile`).
3. BlueZ calls our `NewConnection` method over D-Bus and hands us a **file descriptor**, the open
   socket. From then on it is a plain file we can read and write.

This is `internal/sony/bluez.go`. Things learned the hard way:

- **Only one program can own the channel.** The Sony phone app, our TUI, the CLI and the plugin all
  compete. Close one before using another.
- **After disconnecting, the headphones refuse new connections for a few seconds** (`br-connection-busy`).
  `dial()` retries every 1.5 s until a deadline.
- `bluetoothctl` can say "Connected: no" while the control channel still works, and the headphones
  drop off completely when switched off or at about 12 % battery.

## 5. Step 3: The wire format

Every message is a **frame**:

```
0x3E | data-type | seq | length (4 bytes, big-endian) | payload | checksum | 0x3C
```

- `0x3E` starts a frame and `0x3C` ends it.
- If `0x3C`, `0x3D` or `0x3E` appears inside the frame, it is escaped as `0x3D` followed by
  `byte & 0xEF`.
- The checksum is the sum of all the bytes between the markers, modulo 256.
- Data type `0x0C` carries a command, `0x01` is an acknowledgement.
- **Every frame you receive must be acknowledged**, with the sequence number flipped (0 becomes 1,
  1 becomes 0). The headphones stop talking if you do not. Your own sequence number also alternates.

This is `internal/sony/frame.go`: `encode()` builds a frame, a `decoder` reassembles frames from the
stream (bytes may arrive in pieces).

## 6. Step 4: Commands

The first payload byte is the command, and a reply uses command + 1.

| Request | Reply | Meaning |
|---|---|---|
| `04 01` | `05 …` | device name (text from byte 3) |
| `04 02` | `05 …` | firmware version |
| `22 00` | `23 00 level charging` | battery % and charging flag |
| `66 17` | `67 17 01 enabled mode 00 level` | noise cancelling / ambient state |
| `68 17 01 enabled mode 00 level` | `69 17 …` | set noise cancelling / ambient |
| `56 03` | `57 03 preset ult 06 b1..b6` | read EQ |
| `58 03 preset ult 00` | reply carries the bands | choose an EQ preset |
| `58 03 a0 ult 06 b1..b6` | | set a custom 6-band EQ |

Noise cancelling / ambient:
- `enabled`: 0 or 1
- `mode`: **`0x00` = noise cancelling, `0x01` = ambient sound**
- `level`: ambient level 0-20

EQ:
- Bands, in order: Clear Bass, 400 Hz, 1 kHz, 2.5 kHz, 6.3 kHz, 16 kHz.
- Each gain is sent as the value **+10**, so -10..+10 dB maps to 0..20.
- Presets: `00` Off, `10` Bright, `11` Excited, `12` Mellow, `13` Relaxed, `14` Vocal, `15` Treble,
  `16` Bass, `17` Speech, `a0` Custom.
- `ult` is 0 (off), 1 or 2.

The headphones also send **notifications** unprompted when something changes (for example you press
the button on the ear cup). `readLoop` in `device.go` ACKs every frame, hands replies to whoever is
waiting for them, and turns notifications into events for the UI.

## 7. Step 5: Explore safely

To test a command, `ultcontrol raw 6617` sends a payload and prints the reply. Rules that would have
saved me trouble:

- **Read before you write.** A `GET` command cannot hurt anything.
- **Write down the original state** (`status`) before testing any `SET`.
- **Change one thing, read it back, compare.**
- **Restore the original value afterwards.** I once left the headphones in the wrong mode and once
  corrupted the EQ with malformed writes, because I guessed a format instead of reading the source.
  Both were recoverable, but only because I had the original bytes written down.

## 8. Step 6: Build the program in layers

Build and verify one layer at a time.

1. **`frame.go`:** encode and decode. Test by encoding a known message and checking the bytes.
2. **`bluez.go`:** get a socket. Test by sending `04 01` and printing the name.
3. **`device.go`:** wrap each command in a Go method (`Name`, `Battery`, `NcAsm`, `SetNcAsm`).
4. **`eq.go`:** EQ read and write.
5. **CLI (`main.go`):** `status`, `mode`, `eq`, `raw`. The `--json` flag prints one line of JSON.
6. **TUI (`tui.go`):** a [Bubble Tea](https://github.com/charmbracelet/bubbletea) form. Up and down
   select a row, left and right change a value, `q` quits. It refreshes the battery every 30 seconds
   and updates live from notifications.

CLI usage:

```sh
ultcontrol                       # open the TUI
ultcontrol status
ultcontrol mode nc               # or: ambient [level], off
ultcontrol eq preset bass
ultcontrol eq ult 1
ultcontrol eq custom 0 0 3 5 5 6
ultcontrol --json status         # one line of JSON for scripts
```

JSON shape:

```json
{"name":"ULT WEAR","firmware":"2.1.0","battery":81,"charging":false,
 "mode":"nc","ambient_level":20,"eq_preset":"Bright","ult":0,
 "eq_bands":[-1,0,5,7,7,9]}
```

## 9. Step 7: The Noctalia plugin

Noctalia plugins are folders with a `plugin.toml` and **Luau** scripts (a small, safe Lua dialect).
Mine has three entries that share state:

- **`service.luau`:** the only part that talks to the headphones. It runs `ultcontrol --json …` through
  `noctalia.runAsync`, one command at a time, and publishes results with `noctalia.state.set`. It
  polls status every 120 seconds by default.
- **`bar.luau`:** the bar widget. It watches `ult.status` and shows a headphones icon and the battery %.
- **`panel.luau`:** the control panel. Buttons write a command string into `ult.cmd`, which the
  service picks up.

The widget and panel never touch Bluetooth. They only read and write shared state. That keeps the
"one program on the channel" rule intact and makes each file simple.

Check a plugin with `noctalia plugins lint noctalia-plugin`.

## 10. Step 8: Install

```sh
# 1. the binary
go build -o bin/ultcontrol ./cmd/ultcontrol
install bin/ultcontrol ~/.local/bin/

# 2. the plugin
mkdir -p ~/.local/share/noctalia/plugins
cp -r noctalia-plugin ~/.local/share/noctalia/plugins/ultcontrol
noctalia msg plugins enable mavxd/ultcontrol

# 3. the bar widget: define it in ~/.config/noctalia/config.toml ...
#      [widget.ultcontrol]
#      type = "mavxd/ultcontrol:bar"
#    ... and add "ultcontrol" to a bar list. Noctalia also keeps its own layout in
#    ~/.local/state/noctalia/settings.toml, and that file wins, so add it there too
#    (or use the Settings app).

# 4. the app launcher entry
cp packaging/ultcontrol.desktop ~/.local/share/applications/
cp packaging/ultcontrol.svg ~/.local/share/icons/hicolor/scalable/apps/
```

## 11. Mistakes worth learning from

| What went wrong | Why | Lesson |
|---|---|---|
| Ambient and NC buttons were swapped | I guessed which value meant which | Read the protocol definition (`NC = 0x00`, `ASM = 0x01`) and check it against what you hear |
| EQ got corrupted | I sent write commands with a guessed format | Find the exact format first; keep the original bytes |
| Widget did not appear | Noctalia's `settings.toml` overrides `config.toml` | When an edit has no effect, look for another file that overrides it |
| Connection errors right after a command | The headphones refuse reconnects for several seconds | Retry with a delay |
| Plugin and TUI fight | One owner of the control channel | Run only one at a time |

## 12. Where to go next

- **More features.** The capability reply (`06 xx`) lists what the headphones support. Other
  commands I saw but did not decode: `26 04/05`, `a6 01`, `e6 xx`, `f6 xx`. Look them up in `libmdr`,
  try the `GET` side first, and write only what you understand. Command `36 01/02` contains the
  serial number, which you should not publish.
- **Tests.** The frame encoder and decoder are pure functions and easy to unit test.
- **Other devices.** Different Sony models use a different command table. The framing is the same.

## Tools you need

Go 1.22 or newer, BlueZ with D-Bus (standard on Linux), `bluetoothctl`, and for the plugin the
Noctalia shell. Read-only debugging needs nothing else.
