// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"encoding/binary"
	"net"
	"testing"
)

func TestBuildPacket(t *testing.T) {
	srcMAC := [6]byte{0x02, 0x42, 0xac, 0x11, 0x00, 0x02}
	dstMAC := [6]byte{0x02, 0x42, 0xac, 0x11, 0x00, 0x03}
	flowLabel := uint32(0xDE)

	pkt := buildPacket(flowLabel, srcMAC, dstMAC)

	// Verify total packet size.
	if len(pkt) != PacketSize {
		t.Fatalf("packet size: got %d, want %d", len(pkt), PacketSize)
	}
	if len(pkt) != 78 {
		t.Fatalf("packet size: got %d, want 78", len(pkt))
	}

	// Verify Ethernet header.
	for i := 0; i < 6; i++ {
		if pkt[i] != dstMAC[i] {
			t.Errorf("dst MAC byte %d: got 0x%02X, want 0x%02X", i, pkt[i], dstMAC[i])
		}
		if pkt[6+i] != srcMAC[i] {
			t.Errorf("src MAC byte %d: got 0x%02X, want 0x%02X", i, pkt[6+i], srcMAC[i])
		}
	}

	// EtherType should be 0x86DD (IPv6).
	etherType := binary.BigEndian.Uint16(pkt[12:14])
	if etherType != 0x86DD {
		t.Errorf("EtherType: got 0x%04X, want 0x86DD", etherType)
	}

	// IPv6 version should be 6 (top 4 bits of byte 14).
	version := pkt[14] >> 4
	if version != 6 {
		t.Errorf("IPv6 version: got %d, want 6", version)
	}

	// Flow label (lower 20 bits of bytes 14-17).
	vtcfl := binary.BigEndian.Uint32(pkt[14:18])
	gotFL := vtcfl & 0xFFFFF
	if gotFL != flowLabel {
		t.Errorf("flow label: got 0x%X, want 0x%X", gotFL, flowLabel)
	}

	// Payload length should be 24 (HBH 4 + Monad 20).
	payloadLen := binary.BigEndian.Uint16(pkt[18:20])
	if payloadLen != 24 {
		t.Errorf("payload length: got %d, want 24", payloadLen)
	}

	// Next header should be 0 (Hop-by-Hop).
	if pkt[20] != 0x00 {
		t.Errorf("next header: got 0x%02X, want 0x00", pkt[20])
	}

	// Hop limit should be 255.
	if pkt[21] != 0xFF {
		t.Errorf("hop limit: got %d, want 255", pkt[21])
	}

	// Source address: fd00:3f:75::1
	wantSrc := [16]byte{
		0xFD, 0x00, 0x00, 0x3F, 0x00, 0x75, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01,
	}
	for i := 0; i < 16; i++ {
		if pkt[22+i] != wantSrc[i] {
			t.Errorf("src addr byte %d: got 0x%02X, want 0x%02X", i, pkt[22+i], wantSrc[i])
		}
	}

	// Destination address: fd00:dead::1
	wantDst := [16]byte{
		0xFD, 0x00, 0xDE, 0xAD, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01,
	}
	for i := 0; i < 16; i++ {
		if pkt[38+i] != wantDst[i] {
			t.Errorf("dst addr byte %d: got 0x%02X, want 0x%02X", i, pkt[38+i], wantDst[i])
		}
	}

	// HBH header.
	if pkt[54] != 0x3B {
		t.Errorf("HBH next header: got 0x%02X, want 0x3B (No Next)", pkt[54])
	}
	if pkt[55] != 0x02 {
		t.Errorf("HBH ext len: got %d, want 2", pkt[55])
	}
	if pkt[56] != 0x3E {
		t.Errorf("HBH opt type: got 0x%02X, want 0x3E (Monad)", pkt[56])
	}
	if pkt[57] != 0x14 {
		t.Errorf("HBH opt data len: got %d, want 20", pkt[57])
	}

	// Monad register: type=0x01 (tick), version=0x02.
	if pkt[58] != 0x01 {
		t.Errorf("monad type: got 0x%02X, want 0x01", pkt[58])
	}
	if pkt[65] != 0x02 {
		t.Errorf("monad version: got 0x%02X, want 0x02", pkt[65])
	}
}

func TestBuildPacketFlowLabelMask(t *testing.T) {
	srcMAC := [6]byte{0x02, 0x42, 0xac, 0x11, 0x00, 0x02}
	dstMAC := [6]byte{0x02, 0x42, 0xac, 0x11, 0x00, 0x03}

	// Flow label should be masked to 20 bits.
	pkt := buildPacket(0xFFFFFFFF, srcMAC, dstMAC)
	vtcfl := binary.BigEndian.Uint32(pkt[14:18])
	gotFL := vtcfl & 0xFFFFF
	if gotFL != 0xFFFFF {
		t.Errorf("flow label mask: got 0x%X, want 0xFFFFF", gotFL)
	}

	// Version should still be 6.
	version := pkt[14] >> 4
	if version != 6 {
		t.Errorf("IPv6 version after mask: got %d, want 6", version)
	}
}

func TestBuildPacketMatchesPython(t *testing.T) {
	// Verify the Go packet builder produces the same bytes as the Python inject.py.
	// The Python code builds:
	//   eth = dst_mac + src_mac + struct.pack(">H", 0x86DD)
	//   ipv6 = struct.pack(">IHBB", vtcfl, 24, 0, 255) + src_addr + dst_addr
	//   hbh = struct.pack("BBBB", 59, 2, 0x3E, 20)
	//   monad = struct.pack("BBBBBBBB", 0x01, 0, 0, 0, 0, 0, 0, 0x02) + ...zeros...
	srcMAC := [6]byte{0x02, 0x42, 0xac, 0x11, 0x00, 0x02}
	dstMAC := [6]byte{0x02, 0x42, 0xac, 0x11, 0x00, 0x03}
	pkt := buildPacket(0xDE, srcMAC, dstMAC)

	// Cross-check critical byte offsets against the Python implementation.

	// Python: struct.pack(">IHBB", vtcfl, 24, 0, 255) is 4+2+1+1 = 8 bytes
	// vtcfl = (6 << 28) | (0xDE & 0xFFFFF) = 0x600000DE
	wantVTCFL := uint32(0x600000DE)
	gotVTCFL := binary.BigEndian.Uint32(pkt[14:18])
	if gotVTCFL != wantVTCFL {
		t.Errorf("VTCFL: got 0x%08X, want 0x%08X", gotVTCFL, wantVTCFL)
	}

	// Python payload length field: 24
	gotPL := binary.BigEndian.Uint16(pkt[18:20])
	if gotPL != 24 {
		t.Errorf("payload length: got %d, want 24", gotPL)
	}

	// Total size must be 78 (matching Python assert).
	if len(pkt) != 78 {
		t.Errorf("packet size: got %d, want 78", len(pkt))
	}
}

func TestParseMAC(t *testing.T) {
	tests := []struct {
		input   string
		want    [6]byte
		wantErr bool
	}{
		{"02:42:ac:11:00:02", [6]byte{0x02, 0x42, 0xac, 0x11, 0x00, 0x02}, false},
		{"ff:ff:ff:ff:ff:ff", [6]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, false},
		{"00:00:00:00:00:00", [6]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00}, false},
		{"invalid", [6]byte{}, true},
		{"", [6]byte{}, true},
		{"02:42:ac:11:00", [6]byte{}, true}, // too short
	}

	for _, tt := range tests {
		got, err := parseMAC(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseMAC(%q): expected error, got nil", tt.input)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseMAC(%q): unexpected error: %v", tt.input, err)
			continue
		}
		if got != tt.want {
			t.Errorf("parseMAC(%q): got %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestHtons(t *testing.T) {
	// On little-endian systems, htons should swap bytes.
	// On big-endian systems, it should be a no-op.
	// We test by verifying the result matches the unsafe conversion used in main.
	val := uint16(0x86DD)
	got := htons(val)

	// The result should be the network byte order representation.
	// Verify by converting back: reading the two bytes as big-endian should give val.
	buf := make([]byte, 2)
	binary.LittleEndian.PutUint16(buf, got)
	roundtrip := binary.BigEndian.Uint16(buf)

	if roundtrip != val {
		t.Errorf("htons(0x%04X) roundtrip: got 0x%04X, want 0x%04X", val, roundtrip, val)
	}
}

func TestConstants(t *testing.T) {
	if PacketSize != 78 {
		t.Errorf("PacketSize: got %d, want 78", PacketSize)
	}
	if EthHdrLen != 14 {
		t.Errorf("EthHdrLen: got %d, want 14", EthHdrLen)
	}
	if IPv6HdrLen != 40 {
		t.Errorf("IPv6HdrLen: got %d, want 40", IPv6HdrLen)
	}
	if HBHHdrLen != 4 {
		t.Errorf("HBHHdrLen: got %d, want 4", HBHHdrLen)
	}
	if MonadRegLen != 20 {
		t.Errorf("MonadRegLen: got %d, want 20", MonadRegLen)
	}
	if InsnsPerPacket != 4080 {
		t.Errorf("InsnsPerPacket: got %d, want 4080", InsnsPerPacket)
	}
}

func TestParseMAC_InterfaceMAC(t *testing.T) {
	// Verify that we can parse MACs that net.ParseMAC accepts.
	ifaces, err := net.Interfaces()
	if err != nil {
		t.Skip("cannot list interfaces:", err)
	}

	for _, iface := range ifaces {
		if len(iface.HardwareAddr) != 6 {
			continue
		}
		macStr := iface.HardwareAddr.String()
		got, err := parseMAC(macStr)
		if err != nil {
			t.Errorf("parseMAC(%q) from interface %s: %v", macStr, iface.Name, err)
			continue
		}
		for i := 0; i < 6; i++ {
			if got[i] != iface.HardwareAddr[i] {
				t.Errorf("parseMAC(%q) byte %d: got 0x%02X, want 0x%02X",
					macStr, i, got[i], iface.HardwareAddr[i])
			}
		}
	}
}
