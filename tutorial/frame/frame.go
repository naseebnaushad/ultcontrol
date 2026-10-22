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
