// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package monadpkt

import (
	"encoding/binary"
	"errors"
	"net"
	"testing"

	"unheaded/pkg/protocol/encoding"
)

func TestBuild_Layout(t *testing.T) {
	f := Frame{FlowLabel: 0x5EE1, Dst: net.ParseIP("fd00:5ee1::2"), Src: net.ParseIP("fd00:5ee1::1"),
		SrcMAC: [6]byte{2, 0, 0, 0, 0, 1}, SrcSvc: 3, DstSvc: 4, QoS: 2, SrcPort: 4000, DstPort: 26600}
	p, err := Build(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(p) != HeaderLen {
		t.Fatalf("len = %d, want %d", len(p), HeaderLen)
	}
	if p[0] != 0x33 || p[1] != 0x33 || p[5] != 0x01 {
		t.Errorf("dst MAC = % x, want IPv6 all-nodes", p[0:6])
	}
	if binary.BigEndian.Uint16(p[12:]) != 0x86DD {
		t.Errorf("EtherType = %#x", binary.BigEndian.Uint16(p[12:]))
	}
	ip := p[14:]
	if ip[0]>>4 != 6 || binary.BigEndian.Uint32(ip[0:])&FlowMask != 0x5EE1 {
		t.Errorf("vtf = %#x", binary.BigEndian.Uint32(ip[0:]))
	}
	if binary.BigEndian.Uint16(ip[4:]) != uint16(HbHLen+UDPLen) || ip[6] != 0 || ip[7] != 64 {
		t.Errorf("payload len/next/hop = %d %d %d", binary.BigEndian.Uint16(ip[4:]), ip[6], ip[7])
	}
	if !net.IP(ip[8:24]).Equal(f.Src) || !net.IP(ip[24:40]).Equal(f.Dst) {
		t.Errorf("addresses = %v -> %v", net.IP(ip[8:24]), net.IP(ip[24:40]))
	}
	hbh := ip[40:]
	if hbh[0] != 17 || hbh[1] != 2 || hbh[2] != 0x3E || hbh[3] != 20 {
		t.Errorf("hbh header = % x", hbh[:4])
	}
	m := p[MonadOff : MonadOff+MonadLen]
	if m[0] != 1 || m[1] != 3 || m[2] != 4 || m[4] != 2 || m[7] != 0 {
		t.Errorf("monad = % x", m)
	}
	if got, want := binary.BigEndian.Uint16(m[18:]), encoding.CRC16CCITT(m[:18]); got != want {
		t.Errorf("crc = %#x, want %#x", got, want)
	}
	udp := hbh[24:]
	if binary.BigEndian.Uint16(udp[0:]) != 4000 || binary.BigEndian.Uint16(udp[2:]) != 26600 || binary.BigEndian.Uint16(udp[4:]) != 8 {
		t.Errorf("udp = % x", udp)
	}
}

func TestBuild_CRCMatchesTheSpecVector(t *testing.T) {
	// CRC-16/CCITT-FALSE check value: "123456789" -> 0x29B1. The Monad
	// checksum is this algorithm over bytes 0-17 (foundation-06 §CRC).
	if got := encoding.CRC16CCITT([]byte("123456789")); got != 0x29B1 {
		t.Fatalf("CRC16CCITT(123456789) = %#x, want 0x29B1", got)
	}
	// A version-only Monad (the eBPF Monad::new() + recompute_checksum shape).
	p, err := Build(Frame{FlowLabel: 1})
	if err != nil {
		t.Fatal(err)
	}
	m := p[MonadOff : MonadOff+MonadLen]
	var zero [18]byte
	zero[0] = 1
	if got, want := binary.BigEndian.Uint16(m[18:]), encoding.CRC16CCITT(zero[:]); got != want {
		t.Fatalf("crc = %#x, want %#x", got, want)
	}
}

func TestBuild_Refusals(t *testing.T) {
	cases := map[string]Frame{
		"flow label 0":    {FlowLabel: 0},
		"flow label >20b": {FlowLabel: 0x100000},
		"ipv4 dst":        {FlowLabel: 1, Dst: net.ParseIP("10.0.0.1")},
		"too long":        {FlowLabel: 1, Len: 9001},
	}
	for name, f := range cases {
		if _, err := Build(f); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	p, err := Build(Frame{FlowLabel: 1, Len: 200})
	if err != nil || len(p) != 200 {
		t.Fatalf("len 200: %v %d", err, len(p))
	}
	if binary.BigEndian.Uint16(p[14+4:]) != 200-54 || binary.BigEndian.Uint16(p[14+40+24+4:]) != 200-78 {
		t.Error("lengths do not follow the frame length")
	}
}

func TestParse_RoundTripAndChaosBit(t *testing.T) {
	p, _ := Build(Frame{FlowLabel: 0x5EE2, Flags: FlagChaos | 0x20})
	in, err := Parse(p)
	if err != nil {
		t.Fatal(err)
	}
	if in.FlowLabel != 0x5EE2 || !in.Chaos || in.Flags != 0xA0 || !in.CRCOK {
		t.Fatalf("info = %+v", in)
	}
	p[MonadOff+1] ^= 1 // src_service_id changes, CRC is stale
	if in, _ = Parse(p); in.CRCOK {
		t.Error("stale CRC reported as valid")
	}
	// Parse must not need the payload: headers alone decode.
	if _, err := Parse(p[:EthLen+IPv6Len+HbHLen]); err != nil {
		t.Errorf("headers only: %v", err)
	}
}

func TestParse_Refusals(t *testing.T) {
	if _, err := Parse(make([]byte, 50)); !errors.Is(err, ErrShort) {
		t.Errorf("short: %v", err)
	}
	p, _ := Build(Frame{FlowLabel: 7})
	ip4 := append([]byte{}, p...)
	binary.BigEndian.PutUint16(ip4[12:], 0x0800)
	if _, err := Parse(ip4); !errors.Is(err, ErrNotMonad) {
		t.Errorf("ipv4: %v", err)
	}
	tcp := append([]byte{}, p...)
	tcp[14+6] = 6
	if _, err := Parse(tcp); !errors.Is(err, ErrNotMonad) {
		t.Errorf("no hbh: %v", err)
	}
	other := append([]byte{}, p...)
	other[14+40+2] = 0x01 // a PadN-ish option, not ours
	if _, err := Parse(other); !errors.Is(err, ErrNotMonad) {
		t.Errorf("other option: %v", err)
	}
}

func TestParseFlowLabel(t *testing.T) {
	good := map[string]uint32{"0x5EE1": 0x5EE1, "0X5ee2": 0x5EE2, "24289": 24289, "1": 1, "0xFFFFF": 0xFFFFF}
	for s, want := range good {
		if got, err := ParseFlowLabel(s); err != nil || got != want {
			t.Errorf("%q = %#x %v, want %#x", s, got, err, want)
		}
	}
	for _, s := range []string{"", "0", "0x0", "0x100000", "5EE1", "0x5EE1zz", "-1", "lots"} {
		if _, err := ParseFlowLabel(s); err == nil {
			t.Errorf("%q accepted", s)
		}
	}
}
