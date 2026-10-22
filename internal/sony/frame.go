// Package sony speaks the Sony MDR control protocol used by ULT WEAR headphones.
package sony

import "bytes"

const (
	typeAck byte = 0x01
	typeMDR byte = 0x0C

	startByte = 0x3E
	endByte   = 0x3C
	escByte   = 0x3D
)

func escape(in []byte) []byte {
	out := make([]byte, 0, len(in)+4)
	for _, b := range in {
		if b == startByte || b == endByte || b == escByte {
			out = append(out, escByte, b&0xEF)
		} else {
			out = append(out, b)
		}
	}
	return out
}

// encode builds a wire frame: START | escape(type seq len32 payload checksum) | END.
func encode(dataType, seq byte, payload []byte) []byte {
	body := []byte{dataType, seq, 0, 0, 0, byte(len(payload))}
	body = append(body, payload...)
	var sum byte
	for _, b := range body {
		sum += b
	}
	body = append(body, sum)
	return append(append([]byte{startByte}, escape(body)...), endByte)
}

type frame struct {
	dataType, seq byte
	payload       []byte
}

// decoder reassembles frames from a byte stream.
type decoder struct {
	buf    bytes.Buffer
	active bool
}

func (d *decoder) feed(chunk []byte) []frame {
	var out []frame
	for _, b := range chunk {
		switch {
		case b == startByte:
			d.buf.Reset()
			d.active = true
		case b == endByte && d.active:
			d.active = false
			if f, ok := parse(d.buf.Bytes()); ok {
				out = append(out, f)
			}
		case d.active:
			d.buf.WriteByte(b)
		}
	}
	return out
}

func parse(escaped []byte) (frame, bool) {
	raw := make([]byte, 0, len(escaped))
	for i := 0; i < len(escaped); i++ {
		if escaped[i] == escByte && i+1 < len(escaped) {
			i++
			raw = append(raw, escaped[i]|0x10)
		} else {
			raw = append(raw, escaped[i])
		}
	}
	if len(raw) < 7 {
		return frame{}, false
	}
	var sum byte
	for _, b := range raw[:len(raw)-1] {
		sum += b
	}
	if sum != raw[len(raw)-1] {
		return frame{}, false
	}
	n := int(raw[2])<<24 | int(raw[3])<<16 | int(raw[4])<<8 | int(raw[5])
	if 6+n > len(raw)-1 {
		return frame{}, false
	}
	return frame{raw[0], raw[1], append([]byte(nil), raw[6:6+n]...)}, true
}
