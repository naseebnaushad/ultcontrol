package main

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"ultcontrol/internal/sony"
)

var (
	accent  = lipgloss.Color("#7aa2f7")
	dim     = lipgloss.Color("#6b7089")
	warn    = lipgloss.Color("#e0af68")
	bad     = lipgloss.Color("#f7768e")
	panel   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(dim).Padding(0, 2).Width(64)
	title   = lipgloss.NewStyle().Bold(true).Foreground(accent)
	label   = lipgloss.NewStyle().Foreground(dim)
	cursor  = lipgloss.NewStyle().Bold(true).Foreground(accent)
	heading = lipgloss.NewStyle().Bold(true)
	selStyl = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#1a1b26")).Background(accent).Padding(0, 1)
	optStyl = lipgloss.NewStyle().Foreground(dim).Padding(0, 1)
)

var modes = []string{"Noise cancelling", "Ambient", "Off"}
var ultNames = []string{"Off", "ULT 1", "ULT 2"}

// Form rows, top to bottom.
const (
	rowMode = iota
	rowAmbient
	rowPreset
	rowUlt
	rowBand0 // rowBand0..rowBand0+EqBands-1
	numRows  = rowBand0 + sony.EqBands
)

type model struct {
	h        *sony.Headphones
	name, fw string
	battery  int
	charging bool
	nc       sony.NcAsm
	eq       sony.Eq
	loaded   bool
	row      int
	err      error
}

type loadedMsg struct {
	name, fw string
	battery  int
	charging bool
	nc       sony.NcAsm
	eq       sony.Eq
}
type batteryMsg struct {
	level    int
	charging bool
}
type ncMsg sony.NcAsm
type eqMsg sony.Eq
type errMsg struct{ err error }
type tickMsg struct{}
type eventMsg sony.Event
type closedMsg struct{}

func newModel(h *sony.Headphones) model { return model{h: h} }

func (m model) Init() tea.Cmd {
	return tea.Batch(m.load(), m.waitEvent(), tick())
}

func tick() tea.Cmd { return tea.Tick(30*time.Second, func(time.Time) tea.Msg { return tickMsg{} }) }

func (m model) load() tea.Cmd {
	return func() tea.Msg {
		var l loadedMsg
		var err error
		if l.name, err = m.h.Name(); err != nil {
			return errMsg{err}
		}
		l.fw, _ = m.h.Firmware()
		if l.battery, l.charging, err = m.h.Battery(); err != nil {
			return errMsg{err}
		}
		if l.nc, err = m.h.NcAsm(); err != nil {
			return errMsg{err}
		}
		if l.eq, err = m.h.Eq(); err != nil {
			return errMsg{err}
		}
		return l
	}
}

func (m model) waitEvent() tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-m.h.Events()
		if !ok {
			return closedMsg{}
		}
		return eventMsg(ev)
	}
}

func (m model) applyNc(s sony.NcAsm) tea.Cmd {
	return func() tea.Msg {
		n, err := m.h.SetNcAsm(s)
		if err != nil {
			return errMsg{err}
		}
		return ncMsg(n)
	}
}

func (m model) applyEq(f func() (sony.Eq, error)) tea.Cmd {
	return func() tea.Msg {
		e, err := f()
		if err != nil {
			return errMsg{err}
		}
		return eqMsg(e)
	}
}

func (m model) modeIndex() int {
	switch {
	case !m.nc.Enabled:
		return 2
	case m.nc.Mode == sony.ModeNC:
		return 0
	}
	return 1
}

func (m model) withMode(i int) sony.NcAsm {
	s := m.nc
	switch i {
	case 0:
		s.Enabled, s.Mode = true, sony.ModeNC
	case 1:
		s.Enabled, s.Mode = true, sony.ModeAmbient
	default:
		s.Enabled = false
	}
	return s
}

func (m model) presetIndex() int {
	for i, p := range sony.Presets {
		if p.ID == m.eq.Preset {
			return i
		}
	}
	return 0
}

// adjust changes the value of the focused row by dir (-1 or +1).
func (m model) adjust(dir int) tea.Cmd {
	switch {
	case m.row == rowMode:
		return m.applyNc(m.withMode((m.modeIndex() + dir + 3) % 3))
	case m.row == rowAmbient:
		s := m.nc
		s.Level = max(0, min(20, s.Level+dir))
		return m.applyNc(s)
	case m.row == rowPreset:
		n := len(sony.Presets)
		id := sony.Presets[(m.presetIndex()+dir+n)%n].ID
		ult := m.eq.Ult
		if id == sony.PresetCustom {
			bands := m.eq.Bands
			return m.applyEq(func() (sony.Eq, error) { return m.h.SetCustom(bands, ult) })
		}
		return m.applyEq(func() (sony.Eq, error) { return m.h.SetPreset(id, ult) })
	case m.row == rowUlt:
		ult := (m.eq.Ult + dir + 3) % 3
		if m.eq.Preset == sony.PresetCustom {
			bands := m.eq.Bands
			return m.applyEq(func() (sony.Eq, error) { return m.h.SetCustom(bands, ult) })
		}
		p := m.eq.Preset
		return m.applyEq(func() (sony.Eq, error) { return m.h.SetPreset(p, ult) })
	default:
		bands := m.eq.Bands
		i := m.row - rowBand0
		bands[i] = max(-sony.EqMaxGain, min(sony.EqMaxGain, bands[i]+dir))
		ult := m.eq.Ult
		return m.applyEq(func() (sony.Eq, error) { return m.h.SetCustom(bands, ult) })
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case loadedMsg:
		m.name, m.fw, m.battery, m.charging, m.nc, m.eq, m.loaded = msg.name, msg.fw, msg.battery, msg.charging, msg.nc, msg.eq, true
	case batteryMsg:
		m.battery, m.charging = msg.level, msg.charging
	case ncMsg:
		m.nc = sony.NcAsm(msg)
	case eqMsg:
		m.eq = sony.Eq(msg)
	case eventMsg:
		if msg.NcAsm != nil {
			m.nc = *msg.NcAsm
		}
		if msg.Eq != nil && (msg.Eq.Preset != sony.PresetCustom || msg.Eq.Bands != [sony.EqBands]int{} || m.eq.Preset == sony.PresetCustom) {
			m.eq = *msg.Eq
		}
		return m, m.waitEvent()
	case tickMsg:
		return m, tea.Batch(tick(), func() tea.Msg {
			l, c, err := m.h.Battery()
			if err != nil {
				return errMsg{err}
			}
			return batteryMsg{l, c}
		})
	case errMsg:
		m.err = msg.err
	case closedMsg:
		m.err = fmt.Errorf("headphones disconnected")
	case tea.KeyMsg:
		if m.err != nil {
			return m, tea.Quit
		}
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		}
		if !m.loaded {
			return m, nil
		}
		switch msg.String() {
		case "up", "k":
			m.row = (m.row + numRows - 1) % numRows
		case "down", "j", "tab":
			m.row = (m.row + 1) % numRows
		case "left", "h", "-":
			return m, m.adjust(-1)
		case "right", "l", "+", "=":
			return m, m.adjust(1)
		}
	}
	return m, nil
}

func bar(v, max, width int, c lipgloss.Color) string {
	filled := v * width / max
	return lipgloss.NewStyle().Foreground(c).Render(strings.Repeat("█", filled)) +
		lipgloss.NewStyle().Foreground(dim).Render(strings.Repeat("░", width-filled))
}

// gainBar draws a -10..+10 slider centred on zero.
func gainBar(v int) string {
	const half = sony.EqMaxGain
	cells := make([]string, 2*half+1)
	for i := range cells {
		g := i - half
		switch {
		case g == 0:
			cells[i] = label.Render("│")
		case (v > 0 && g > 0 && g <= v) || (v < 0 && g < 0 && g >= v):
			cells[i] = lipgloss.NewStyle().Foreground(accent).Render("█")
		default:
			cells[i] = label.Render("·")
		}
	}
	return strings.Join(cells, "")
}

func (m model) line(row int, name, value string, active bool) string {
	prefix := "  "
	if m.row == row {
		prefix = cursor.Render("▸ ")
	}
	st := label
	if active && m.row == row {
		st = lipgloss.NewStyle()
	}
	return prefix + st.Render(fmt.Sprintf("%-12s", name)) + value
}

func (m model) View() string {
	if m.err != nil {
		return panel.Render(lipgloss.NewStyle().Foreground(bad).Render("Error: "+m.err.Error())+"\n\n"+label.Render("press any key to exit")) + "\n"
	}
	if !m.loaded {
		return panel.Render(label.Render("Connecting to headphones…")) + "\n"
	}
	bc := accent
	if m.battery <= 20 {
		bc = warn
	}
	chg := ""
	if m.charging {
		chg = " ⚡"
	}
	var tabs []string
	for i, name := range modes {
		if i == m.modeIndex() {
			tabs = append(tabs, selStyl.Render(name))
		} else {
			tabs = append(tabs, optStyl.Render(name))
		}
	}
	ambient := bar(m.nc.Level, 20, 20, accent) + fmt.Sprintf(" %2d", m.nc.Level)
	if m.modeIndex() != 1 {
		ambient = label.Render(ambient)
	}
	choice := func(s string) string { return label.Render("‹ ") + selStyl.Render(s) + label.Render(" ›") }

	lines := []string{
		title.Render(m.name) + label.Render("  fw "+m.fw),
		label.Render("Battery      ") + bar(m.battery, 100, 20, bc) + fmt.Sprintf(" %3d%%%s", m.battery, chg),
		"",
		heading.Render("Sound"),
		m.line(rowMode, "Mode", strings.Join(tabs, " "), true),
		m.line(rowAmbient, "Ambient", ambient, true),
		"",
		heading.Render("Equalizer"),
		m.line(rowPreset, "Preset", choice(sony.PresetName(m.eq.Preset)), true),
		m.line(rowUlt, "ULT mode", choice(ultNames[m.eq.Ult%3]), true),
	}
	for i, name := range sony.BandNames {
		g := m.eq.Bands[i]
		lines = append(lines, m.line(rowBand0+i, name, gainBar(g)+fmt.Sprintf(" %+3d", g), true))
	}
	lines = append(lines, "", label.Render("↑/↓ select   ←/→ change   q quit"))
	return panel.Render(strings.Join(lines, "\n")) + "\n"
}
