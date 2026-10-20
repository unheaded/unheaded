// SPDX-License-Identifier: GPL-3.0-or-later
package gjallarhorn

import (
	"bytes"
	"testing"
)

func TestMarshalUnmarshalRoundTrip(t *testing.T) {
	p := &Packet{
		Magic:       Magic,
		Kind:        BootstrapBroadcast,
		ClusterID:   42,
		ManifestPtr: 0xCAFEBABEDEADBEEF,
	}
	buf := p.Marshal()
	if len(buf) != PacketSize {
		t.Fatalf("expected %d bytes, got %d", PacketSize, len(buf))
	}
	if !bytes.Equal(buf[0:4], Magic[:]) {
		t.Fatalf("magic mismatch in marshal")
	}

	p2, err := Unmarshal(buf)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p2.Kind != p.Kind || p2.ClusterID != p.ClusterID || p2.ManifestPtr != p.ManifestPtr {
		t.Fatalf("round trip mismatch: got %+v want %+v", p2, p)
	}
}

func TestUnmarshalRejectsShortPacket(t *testing.T) {
	_, err := Unmarshal([]byte{1, 2, 3})
	if err != ErrShortPacket {
		t.Fatalf("expected ErrShortPacket, got %v", err)
	}
}

func TestUnmarshalRejectsBadMagic(t *testing.T) {
	buf := make([]byte, PacketSize)
	copy(buf, []byte("XXXX"))
	_, err := Unmarshal(buf)
	if err != ErrBadMagic {
		t.Fatalf("expected ErrBadMagic, got %v", err)
	}
}

func TestUnmarshalRejectsBadKind(t *testing.T) {
	buf := make([]byte, PacketSize)
	copy(buf, Magic[:])
	buf[4] = 0xFF // unknown kind
	_, err := Unmarshal(buf)
	if err != ErrBadKind {
		t.Fatalf("expected ErrBadKind, got %v", err)
	}
}

func TestPacketSize20Bytes(t *testing.T) {
	// Hard requirement from ADR-043: must fit in 20-byte Monad register
	if PacketSize != 20 {
		t.Fatalf("PacketSize must be 20 bytes (Monad register), got %d", PacketSize)
	}
}

// LICH-012 L12e: a trigger has exactly one encoding. A parser that accepts
// trailing bytes or set reserved bits lets two different datagrams mean the
// same trigger (and carry data past every check), so it must refuse both.
func TestUnmarshalRejectsTrailingBytes(t *testing.T) {
	buf := append((&Packet{Kind: ReverifyUnicast, ClusterID: 7}).Marshal(), 0xAA)
	if _, err := Unmarshal(buf); err != ErrLongPacket {
		t.Fatalf("21-byte datagram: err=%v, want ErrLongPacket", err)
	}
}

func TestUnmarshalRejectsReservedBits(t *testing.T) {
	for i := 17; i < PacketSize; i++ {
		buf := (&Packet{Kind: BootstrapBroadcast, ClusterID: 1}).Marshal()
		buf[i] = 0x01
		if _, err := Unmarshal(buf); err != ErrReservedSet {
			t.Fatalf("reserved byte %d set: err=%v, want ErrReservedSet", i, err)
		}
	}
}

// FuzzUnmarshal: any datagram Unmarshal accepts re-marshals to exactly the
// same bytes.
func FuzzUnmarshal(f *testing.F) {
	f.Add((&Packet{Kind: BootstrapBroadcast, ClusterID: 42, ManifestPtr: 0xCAFEBABEDEADBEEF}).Marshal())
	f.Add((&Packet{Kind: ReverifyUnicast}).Marshal())
	f.Add(append((&Packet{Kind: ReverifyUnicast}).Marshal(), 0))
	f.Add([]byte("GJLR"))
	f.Fuzz(func(t *testing.T, buf []byte) {
		p, err := Unmarshal(buf)
		if err != nil {
			return
		}
		if got := p.Marshal(); !bytes.Equal(got, buf) {
			t.Fatalf("accepted %x, which re-marshals as %x", buf, got)
		}
	})
}
