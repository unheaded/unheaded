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
