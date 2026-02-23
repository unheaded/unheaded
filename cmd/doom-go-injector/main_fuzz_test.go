package main

import (
	"encoding/binary"
	"testing"
)

// FuzzBuildPacket feeds random flow labels and MAC addresses to buildPacket.
// The function must never panic and must always produce a valid 78-byte packet.
//
// Go fuzz only supports primitive types and []byte/string, so we encode the
// flow label as uint32 and the two 6-byte MACs as a single 12-byte []byte.
func FuzzBuildPacket(f *testing.F) {
	// Seed corpus: typical inputs. Each seed is (flowLabel, macData[12]).
	f.Add(
		uint32(0xDE),
		[]byte{0x02, 0x42, 0xac, 0x11, 0x00, 0x02, 0x02, 0x42, 0xac, 0x11, 0x00, 0x03},
	)
	f.Add(
		uint32(0),
		[]byte{0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
	)
	f.Add(
		uint32(0xFFFFF),
		[]byte{0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF},
	)
	f.Add(
		uint32(0xFFFFFFFF),
		[]byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x0A, 0x0B, 0x0C, 0x0D, 0x0E, 0x0F},
	)

	f.Fuzz(func(t *testing.T, flowLabel uint32, macData []byte) {
		// We need exactly 12 bytes for two MAC addresses.
		if len(macData) < 12 {
			return
		}

		var srcMAC, dstMAC [6]byte
		copy(srcMAC[:], macData[0:6])
		copy(dstMAC[:], macData[6:12])

		pkt := buildPacket(flowLabel, srcMAC, dstMAC)

		// Invariant: packet is always exactly PacketSize (78) bytes.
		if len(pkt) != PacketSize {
			t.Fatalf("packet size = %d, want %d", len(pkt), PacketSize)
		}

		// Invariant: EtherType is always IPv6 (0x86DD).
		etherType := binary.BigEndian.Uint16(pkt[12:14])
		if etherType != EthTypeIPv6 {
			t.Errorf("EtherType = 0x%04X, want 0x%04X", etherType, EthTypeIPv6)
		}

		// Invariant: IPv6 version is always 6.
		version := pkt[14] >> 4
		if version != 6 {
			t.Errorf("IPv6 version = %d, want 6", version)
		}

		// Invariant: flow label is masked to 20 bits.
		vtcfl := binary.BigEndian.Uint32(pkt[14:18])
		gotFL := vtcfl & 0xFFFFF
		wantFL := flowLabel & 0xFFFFF
		if gotFL != wantFL {
			t.Errorf("flow label = 0x%X, want 0x%X", gotFL, wantFL)
		}

		// Invariant: payload length is always 24.
		payloadLen := binary.BigEndian.Uint16(pkt[18:20])
		if payloadLen != 24 {
			t.Errorf("payload length = %d, want 24", payloadLen)
		}

		// Invariant: next header is always Hop-by-Hop (0x00).
		if pkt[20] != 0x00 {
			t.Errorf("next header = 0x%02X, want 0x00", pkt[20])
		}

		// Invariant: hop limit is always 255.
		if pkt[21] != 0xFF {
			t.Errorf("hop limit = %d, want 255", pkt[21])
		}

		// Invariant: dst MAC is at bytes 0-5.
		for i := 0; i < 6; i++ {
			if pkt[i] != dstMAC[i] {
				t.Errorf("dst MAC[%d] = 0x%02X, want 0x%02X", i, pkt[i], dstMAC[i])
			}
		}

		// Invariant: src MAC is at bytes 6-11.
		for i := 0; i < 6; i++ {
			if pkt[6+i] != srcMAC[i] {
				t.Errorf("src MAC[%d] = 0x%02X, want 0x%02X", i, pkt[6+i], srcMAC[i])
			}
		}

		// Invariant: HBH option type is Monad (0x3E).
		if pkt[56] != 0x3E {
			t.Errorf("HBH opt type = 0x%02X, want 0x3E", pkt[56])
		}

		// Invariant: Monad type is tick (0x01).
		if pkt[58] != 0x01 {
			t.Errorf("monad type = 0x%02X, want 0x01", pkt[58])
		}

		// Invariant: Monad version is 0x02.
		if pkt[65] != 0x02 {
			t.Errorf("monad version = 0x%02X, want 0x02", pkt[65])
		}
	})
}

// FuzzParseMAC feeds random strings to parseMAC.
func FuzzParseMAC(f *testing.F) {
	f.Add("02:42:ac:11:00:02")
	f.Add("ff:ff:ff:ff:ff:ff")
	f.Add("00:00:00:00:00:00")
	f.Add("")
	f.Add("invalid")
	f.Add("02:42:ac:11:00")
	f.Add("02:42:ac:11:00:02:03")
	f.Add("zz:zz:zz:zz:zz:zz")
	f.Add("02-42-ac-11-00-02") // dash-separated format

	f.Fuzz(func(t *testing.T, input string) {
		mac, err := parseMAC(input)
		if err != nil {
			return // parse errors are expected
		}

		// Invariant: successful parse produces a 6-byte MAC.
		_ = mac[5] // accessing index 5 proves it has 6 bytes (compile-time check via array)

		// Invariant: round-trip through net.ParseMAC should succeed.
		formatted := formatMAC(mac)
		mac2, err := parseMAC(formatted)
		if err != nil {
			t.Fatalf("parseMAC(formatMAC(%v)) error: %v", mac, err)
		}
		if mac != mac2 {
			t.Errorf("roundtrip mismatch: %v != %v", mac, mac2)
		}
	})
}

// formatMAC formats a 6-byte MAC as colon-separated hex (for roundtrip testing).
func formatMAC(mac [6]byte) string {
	const hex = "0123456789abcdef"
	buf := make([]byte, 17)
	for i := 0; i < 6; i++ {
		if i > 0 {
			buf[i*3-1] = ':'
		}
		buf[i*3] = hex[mac[i]>>4]
		buf[i*3+1] = hex[mac[i]&0x0F]
	}
	return string(buf)
}
