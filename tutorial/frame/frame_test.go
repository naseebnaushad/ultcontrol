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
