package sony

import (
	"context"
	"errors"
	"os"
	"sync"
	"time"
)

// NcAsm is the noise-cancelling / ambient-sound state.
type NcAsm struct {
	Enabled bool
	Mode    Mode
	Level   int // ambient level, 0-20
}

type Mode string

const (
	ModeNC      Mode = "nc"
	ModeAmbient Mode = "ambient"
)

// Event is an unsolicited update from the headphones.
type Event struct {
	NcAsm *NcAsm
	Eq    *Eq
}

type Headphones struct {
	f      *os.File
	events chan Event

	mu      sync.Mutex // guards seq, waiters and writes
	seq     byte
	waiters map[byte]chan []byte
	done    chan struct{}
}

// Open connects to the headphones with the given Bluetooth address.
func Open(ctx context.Context, mac string) (*Headphones, error) {
	f, err := dial(ctx, mac)
	if err != nil {
		return nil, err
	}
	h := &Headphones{f: f, events: make(chan Event, 8), waiters: map[byte]chan []byte{}, done: make(chan struct{})}
	go h.readLoop()
	return h, nil
}

func (h *Headphones) Close() error { return h.f.Close() }

// Events delivers changes made on the headphones themselves; closed when the link drops.
func (h *Headphones) Events() <-chan Event { return h.events }

func (h *Headphones) write(dataType byte, payload []byte) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	err := h.writeLocked(dataType, h.seq, payload)
	if dataType == typeMDR {
		h.seq ^= 1
	}
	return err
}

func (h *Headphones) writeLocked(dataType, seq byte, payload []byte) error {
	_, err := h.f.Write(encode(dataType, seq, payload))
	return err
}

func (h *Headphones) readLoop() {
	defer close(h.events)
	defer close(h.done)
	var d decoder
	buf := make([]byte, 1024)
	for {
		n, err := h.f.Read(buf)
		if err != nil {
			return
		}
		for _, fr := range d.feed(buf[:n]) {
			if fr.dataType == typeAck || len(fr.payload) == 0 {
				continue
			}
			h.mu.Lock()
			_ = h.writeLocked(typeAck, fr.seq^1, nil)
			w := h.waiters[fr.payload[0]]
			h.mu.Unlock()
			if w != nil {
				select {
				case w <- fr.payload:
				default:
				}
			}
			if fr.payload[0] == 0x59 {
				if e, ok := parseEq(fr.payload); ok {
					select {
					case h.events <- Event{Eq: &e}:
					default:
					}
				}
			}
			if fr.payload[0] == 0x69 {
				if s, ok := parseNcAsm(fr.payload); ok {
					select {
					case h.events <- Event{NcAsm: &s}:
					default:
					}
				}
			}
		}
	}
}

func (h *Headphones) request(payload []byte, replyCmd byte) ([]byte, error) {
	w := make(chan []byte, 1)
	h.mu.Lock()
	h.waiters[replyCmd] = w
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.waiters, replyCmd)
		h.mu.Unlock()
	}()
	if err := h.write(typeMDR, payload); err != nil {
		return nil, err
	}
	select {
	case r := <-w:
		return r, nil
	case <-h.done:
		return nil, errors.New("headphones disconnected")
	case <-time.After(3 * time.Second):
		return nil, errors.New("timed out waiting for headphones")
	}
}

func (h *Headphones) text(param byte) (string, error) {
	r, err := h.request([]byte{0x04, param}, 0x05)
	if err != nil || len(r) < 4 {
		return "", errOr(err)
	}
	return string(r[3:]), nil
}

func errOr(err error) error {
	if err != nil {
		return err
	}
	return errors.New("short reply")
}

func (h *Headphones) Name() (string, error)     { return h.text(0x01) }
func (h *Headphones) Firmware() (string, error) { return h.text(0x02) }

func (h *Headphones) Battery() (level int, charging bool, err error) {
	r, err := h.request([]byte{0x22, 0x00}, 0x23)
	if err != nil || len(r) < 4 {
		return 0, false, errOr(err)
	}
	return int(r[2]), r[3] != 0, nil
}

func parseNcAsm(r []byte) (NcAsm, bool) {
	if len(r) < 7 {
		return NcAsm{}, false
	}
	m := ModeNC // 0x00 = NC, 0x01 = ambient sound
	if r[4] != 0 {
		m = ModeAmbient
	}
	return NcAsm{Enabled: r[3] != 0, Mode: m, Level: int(r[6])}, true
}

func (h *Headphones) NcAsm() (NcAsm, error) {
	r, err := h.request([]byte{0x66, 0x17}, 0x67)
	if err != nil {
		return NcAsm{}, err
	}
	s, ok := parseNcAsm(r)
	if !ok {
		return NcAsm{}, errors.New("short NC/ambient reply")
	}
	return s, nil
}

func b2i(b bool) byte {
	if b {
		return 1
	}
	return 0
}

func (h *Headphones) SetNcAsm(s NcAsm) (NcAsm, error) {
	if s.Level < 0 {
		s.Level = 0
	}
	if s.Level > 20 {
		s.Level = 20
	}
	_, err := h.request([]byte{0x68, 0x17, 0x01, b2i(s.Enabled), b2i(s.Mode == ModeAmbient), 0x00, byte(s.Level)}, 0x69)
	if err != nil {
		return NcAsm{}, err
	}
	return h.NcAsm()
}

// Raw sends an arbitrary payload and returns the first reply whose command byte is replyCmd.
// Intended for protocol exploration.
func (h *Headphones) Raw(payload []byte, replyCmd byte) ([]byte, error) {
	return h.request(payload, replyCmd)
}
