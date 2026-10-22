package main

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"ultcontrol/internal/sony"
)

func main() {
	mac := flag.String("mac", os.Getenv("ULT_MAC"), "headphone Bluetooth address (default: auto-detect)")
	asJSON := flag.Bool("json", false, "print the full device state as JSON after running a command")
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: ultcontrol [--mac ADDR] [--json] [status | mode nc|ambient|off [level 0-20] | eq [preset NAME | ult 0-2 | custom G1..G6]]\nWith no command, starts the interactive UI.")
	}
	flag.Parse()

	if err := run(*mac, *asJSON, flag.Args()); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(mac string, asJSON bool, args []string) error {
	if mac == "" {
		var err error
		if mac, err = sony.FindDevice(); err != nil {
			return err
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	h, err := sony.Open(ctx, mac)
	if err != nil {
		return err
	}
	defer h.Close()

	if len(args) == 0 {
		_, err := tea.NewProgram(newModel(h), tea.WithAltScreen()).Run()
		return err
	}
	if asJSON {
		if args[0] != "status" {
			if err := runQuiet(h, args); err != nil {
				return err
			}
		}
		return printJSON(h)
	}
	switch args[0] {
	case "status":
		return status(h)
	case "mode":
		return setMode(h, args[1:])
	case "eq":
		return eqCmd(h, args[1:])
	case "raw": // debugging: send raw hex payloads
		return raw(h, args[1:])
	}
	return fmt.Errorf("unknown command %q", args[0])
}

func status(h *sony.Headphones) error {
	name, err := h.Name()
	if err != nil {
		return err
	}
	fw, _ := h.Firmware()
	lvl, chg, err := h.Battery()
	if err != nil {
		return err
	}
	n, err := h.NcAsm()
	if err != nil {
		return err
	}
	fmt.Printf("%s  fw %s\nbattery %d%%%s\nmode    %s  ambient level %d\n", name, fw, lvl, map[bool]string{true: " (charging)"}[chg], modeLabel(n), n.Level)
	return nil
}

func setMode(h *sony.Headphones, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: mode nc|ambient|off [level 0-20]")
	}
	cur, err := h.NcAsm()
	if err != nil {
		return err
	}
	switch args[0] {
	case "nc":
		cur.Enabled, cur.Mode = true, sony.ModeNC
	case "ambient":
		cur.Enabled, cur.Mode = true, sony.ModeAmbient
	case "off":
		cur.Enabled = false
	default:
		return fmt.Errorf("unknown mode %q", args[0])
	}
	if len(args) > 1 {
		if _, err := fmt.Sscan(args[1], &cur.Level); err != nil {
			return fmt.Errorf("bad level %q", args[1])
		}
	}
	n, err := h.SetNcAsm(cur)
	if err != nil {
		return err
	}
	fmt.Printf("mode %s  ambient level %d\n", modeLabel(n), n.Level)
	return nil
}

func modeLabel(n sony.NcAsm) string {
	if !n.Enabled {
		return "off"
	}
	return string(n.Mode)
}

func raw(h *sony.Headphones, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: raw HEXPAYLOAD... (reply command is request command+1)")
	}
	for _, a := range args {
		p, err := hex.DecodeString(a)
		if err != nil || len(p) == 0 {
			return fmt.Errorf("bad hex payload %q", a)
		}
		r, err := h.Raw(p, p[0]+1)
		if err != nil {
			fmt.Printf("%s -> %v\n", a, err)
			continue
		}
		fmt.Printf("%s -> %s\n", a, hex.EncodeToString(r))
	}
	return nil
}

func printEq(e sony.Eq) {
	fmt.Printf("preset %s  ult %d\n", sony.PresetName(e.Preset), e.Ult)
	for i, n := range sony.BandNames {
		fmt.Printf("  %-11s %+d\n", n, e.Bands[i])
	}
}

func eqCmd(h *sony.Headphones, args []string) error {
	cur, err := h.Eq()
	if err != nil {
		return err
	}
	if len(args) == 0 {
		printEq(cur)
		return nil
	}
	var e sony.Eq
	switch args[0] {
	case "preset":
		if len(args) < 2 {
			return fmt.Errorf("usage: eq preset NAME")
		}
		found := false
		for _, p := range sony.Presets {
			if strings.EqualFold(p.Name, args[1]) {
				found = true
				if p.ID == sony.PresetCustom {
					e, err = h.SetCustom(cur.Bands, cur.Ult)
				} else {
					e, err = h.SetPreset(p.ID, cur.Ult)
				}
			}
		}
		if !found {
			return fmt.Errorf("unknown preset %q", args[1])
		}
	case "ult":
		var u int
		if len(args) < 2 {
			return fmt.Errorf("usage: eq ult 0|1|2")
		}
		if _, err := fmt.Sscan(args[1], &u); err != nil || u < 0 || u > 2 {
			return fmt.Errorf("ult must be 0, 1 or 2")
		}
		if cur.Preset == sony.PresetCustom {
			e, err = h.SetCustom(cur.Bands, u)
		} else {
			e, err = h.SetPreset(cur.Preset, u)
		}
	case "custom":
		if len(args) != 1+sony.EqBands {
			return fmt.Errorf("usage: eq custom G1 G2 G3 G4 G5 G6   (gains -10..10: %s)", strings.Join(sony.BandNames[:], ", "))
		}
		var b [sony.EqBands]int
		for i := range b {
			if _, err := fmt.Sscan(args[1+i], &b[i]); err != nil || b[i] < -sony.EqMaxGain || b[i] > sony.EqMaxGain {
				return fmt.Errorf("bad gain %q", args[1+i])
			}
		}
		e, err = h.SetCustom(b, cur.Ult)
	default:
		return fmt.Errorf("unknown eq command %q", args[0])
	}
	if err != nil {
		return err
	}
	printEq(e)
	return nil
}

// runQuiet executes a command, discarding its human-readable output.
func runQuiet(h *sony.Headphones, args []string) error {
	stdout := os.Stdout
	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer devnull.Close()
	os.Stdout = devnull
	defer func() { os.Stdout = stdout }()
	switch args[0] {
	case "mode":
		return setMode(h, args[1:])
	case "eq":
		return eqCmd(h, args[1:])
	}
	return fmt.Errorf("unknown command %q", args[0])
}

type snapshot struct {
	Name     string `json:"name"`
	Firmware string `json:"firmware"`
	Battery  int    `json:"battery"`
	Charging bool   `json:"charging"`
	Mode     string `json:"mode"` // nc, ambient or off
	Ambient  int    `json:"ambient_level"`
	Preset   string `json:"eq_preset"`
	Ult      int    `json:"ult"`
	Bands    []int  `json:"eq_bands"`
}

func printJSON(h *sony.Headphones) error {
	var s snapshot
	var err error
	if s.Name, err = h.Name(); err != nil {
		return err
	}
	s.Firmware, _ = h.Firmware()
	if s.Battery, s.Charging, err = h.Battery(); err != nil {
		return err
	}
	n, err := h.NcAsm()
	if err != nil {
		return err
	}
	s.Mode, s.Ambient = modeLabel(n), n.Level
	e, err := h.Eq()
	if err != nil {
		return err
	}
	s.Preset, s.Ult, s.Bands = sony.PresetName(e.Preset), e.Ult, e.Bands[:]
	return json.NewEncoder(os.Stdout).Encode(s)
}
