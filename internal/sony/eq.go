package sony

import "errors"

// EQ band count and gain range (the headphones store gain+10 per band).
const (
	EqBands   = 6
	EqMaxGain = 10
	eqType    = 0x03 // PRESET_EQ_AND_ULT_MODE: the EQ variant used by ULT WEAR
)

// BandNames labels the six bands in wire order.
var BandNames = [EqBands]string{"Clear Bass", "400 Hz", "1 kHz", "2.5 kHz", "6.3 kHz", "16 kHz"}

// Preset is an EQ preset id as used on the wire.
type Preset byte

const PresetCustom Preset = 0xA0

// Presets lists the presets this model reports, in display order.
var Presets = []struct {
	ID   Preset
	Name string
}{
	{0x00, "Off"}, {0x10, "Bright"}, {0x11, "Excited"}, {0x12, "Mellow"}, {0x13, "Relaxed"},
	{0x14, "Vocal"}, {0x15, "Treble"}, {0x16, "Bass"}, {0x17, "Speech"}, {0xA0, "Custom"},
}

// PresetName returns the display name for a preset id.
func PresetName(id Preset) string {
	for _, p := range Presets {
		if p.ID == id {
			return p.Name
		}
	}
	return "Unknown"
}

// Eq is the equalizer state. Ult is 0 (off), 1 (ULT 1) or 2 (ULT 2).
type Eq struct {
	Preset Preset
	Ult    int
	Bands  [EqBands]int // gain per band, -10..+10
}

func parseEq(r []byte) (Eq, bool) {
	// cmd, type, preset, ult, count, bands...
	if len(r) < 5 || r[1] != eqType {
		return Eq{}, false
	}
	e := Eq{Preset: Preset(r[2]), Ult: int(r[3])}
	n := int(r[4])
	if n != 0 && (n != EqBands || len(r) < 5+n) {
		return Eq{}, false
	}
	for i := 0; i < n; i++ {
		e.Bands[i] = int(r[5+i]) - EqMaxGain
	}
	return e, true
}

func (h *Headphones) Eq() (Eq, error) {
	r, err := h.request([]byte{0x56, eqType}, 0x57)
	if err != nil {
		return Eq{}, err
	}
	e, ok := parseEq(r)
	if !ok {
		return Eq{}, errors.New("unexpected EQ reply")
	}
	return e, nil
}

// SetPreset selects a preset; the headphones report back the preset's band values.
func (h *Headphones) SetPreset(p Preset, ult int) (Eq, error) {
	return h.setEq([]byte{0x58, eqType, byte(p), byte(ult), 0})
}

// SetCustom applies custom band gains (switching to the Custom preset).
func (h *Headphones) SetCustom(bands [EqBands]int, ult int) (Eq, error) {
	p := []byte{0x58, eqType, byte(PresetCustom), byte(ult), EqBands}
	for _, b := range bands {
		p = append(p, byte(max(-EqMaxGain, min(EqMaxGain, b))+EqMaxGain))
	}
	return h.setEq(p)
}

func (h *Headphones) setEq(p []byte) (Eq, error) {
	r, err := h.request(p, 0x59)
	if err != nil {
		return Eq{}, err
	}
	e, ok := parseEq(r)
	if !ok {
		return Eq{}, errors.New("unexpected EQ reply")
	}
	return e, nil
}
