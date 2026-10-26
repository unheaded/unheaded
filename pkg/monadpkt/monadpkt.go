// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package monadpkt builds and reads the one frame shape the Kingdom's eBPF
// programs expect: Ethernet / IPv6 / Hop-by-Hop with a single Monad option
// (type 0x3E, 20 bytes, version 0x01, CRC-16/CCITT over bytes 0-17) / UDP.
//
// It is the lab traffic source and counter for S-REAL-STEEL Phase 13
// (D-25): `steel-echo monad-send` builds frames, `steel-echo monad-count`
// reads only their headers, and pkg/cartridge's benchmarks use the same
// bytes. Nothing here changes the wire format: the layout is foundation-06's
// frozen v0x01 Monad inside a standard RFC 8200 HbH option.
package monadpkt

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"strconv"

	"unheaded/pkg/protocol/encoding"
)

// Frame geometry.
const (
	EthLen   = 14
	IPv6Len  = 40
	HbHLen   = 24 // next(1) + ext len(1) + opt type(1) + opt len(1) + Monad(20)
	UDPLen   = 8
	MonadLen = 20
	// HeaderLen is everything up to and including the UDP header: the bytes
	// Parse reads. A counter never looks past it.
	HeaderLen = EthLen + IPv6Len + HbHLen + UDPLen // 86

	MonadOff = EthLen + IPv6Len + 4 // 58: first Monad byte in the frame

	OptType    = 0x3E // Monad HbH option type (foundation-06, requested code point)
	OptDataLen = 20
	NextHbH    = 0  // IPv6 Next Header: Hop-by-Hop Options
	NextUDP    = 17 // HbH Next Header: UDP
	EtherIPv6  = 0x86DD
	FlowMask   = 0x000FFFFF

	// Monad byte offsets used here (foundation-06 register file).
	offVersion  = 0x00
	offSrcSvc   = 0x01
	offDstSvc   = 0x02
	offQoS      = 0x04
	offFlags    = 0x07
	offChecksum = 0x12

	// FlagChaos is bit 7 of the Monad flags byte: Yaldabaoth modified the packet.
	FlagChaos = 0x80
)

// Frame is what Build encodes.
type Frame struct {
	FlowLabel uint32 // 20 bits; 0 is refused (no correlation key)
	Src, Dst  net.IP // IPv6; nil defaults to fd00::1 / fd00::2
	SrcMAC    [6]byte
	DstMAC    [6]byte // all zero defaults to 33:33:00:00:00:01 (IPv6 all-nodes)
	SrcSvc    uint8
	DstSvc    uint8
	QoS       uint8
	Flags     uint8
	SrcPort   uint16
	DstPort   uint16
	// Len is the total frame length; below HeaderLen it is raised to HeaderLen.
	Len int
}

var (
	ErrFlowLabel = errors.New("monadpkt: flow label must be 1..0xFFFFF")
	ErrAddr      = errors.New("monadpkt: src and dst must be IPv6 addresses")
	ErrShort     = errors.New("monadpkt: frame shorter than its headers")
	ErrNotMonad  = errors.New("monadpkt: not an IPv6 frame with a Monad Hop-by-Hop option")
	ErrTooLong   = errors.New("monadpkt: frame longer than 9000 bytes")
)

// DefaultDstMAC is the IPv6 all-nodes group address: a frame nobody has to
// answer, which a counter on the peer still sees.
var DefaultDstMAC = [6]byte{0x33, 0x33, 0x00, 0x00, 0x00, 0x01}

// Build encodes f. The Monad carries version 0x01, the service ids, qos,
// flags and a valid CRC; every other register is zero. The UDP payload is
// zero bytes: a lab frame has nothing to say.
func Build(f Frame) ([]byte, error) {
	if f.FlowLabel == 0 || f.FlowLabel > FlowMask {
		return nil, ErrFlowLabel
	}
	src, dst := f.Src, f.Dst
	if src == nil {
		src = net.ParseIP("fd00::1")
	}
	if dst == nil {
		dst = net.ParseIP("fd00::2")
	}
	src16, dst16 := src.To16(), dst.To16()
	if src16 == nil || dst16 == nil || src.To4() != nil || dst.To4() != nil {
		return nil, ErrAddr
	}
	n := f.Len
	if n < HeaderLen {
		n = HeaderLen
	}
	if n > 9000 {
		return nil, ErrTooLong
	}
	dmac := f.DstMAC
	if dmac == [6]byte{} {
		dmac = DefaultDstMAC
	}
	p := make([]byte, n)
	copy(p[0:6], dmac[:])
	copy(p[6:12], f.SrcMAC[:])
	binary.BigEndian.PutUint16(p[12:], EtherIPv6)

	ip := p[EthLen:]
	binary.BigEndian.PutUint32(ip[0:], 6<<28|f.FlowLabel)
	binary.BigEndian.PutUint16(ip[4:], uint16(n-EthLen-IPv6Len)) // #nosec G115 -- n <= 9000
	ip[6], ip[7] = NextHbH, 64
	copy(ip[8:24], src16)
	copy(ip[24:40], dst16)

	hbh := ip[IPv6Len:]
	hbh[0], hbh[1], hbh[2], hbh[3] = NextUDP, (HbHLen/8)-1, OptType, OptDataLen
	m := hbh[4 : 4+MonadLen]
	m[offVersion] = 0x01
	m[offSrcSvc], m[offDstSvc] = f.SrcSvc, f.DstSvc
	m[offQoS], m[offFlags] = f.QoS, f.Flags
	binary.BigEndian.PutUint16(m[offChecksum:], encoding.CRC16CCITT(m[:offChecksum]))

	udp := hbh[HbHLen:]
	binary.BigEndian.PutUint16(udp[0:], f.SrcPort)
	binary.BigEndian.PutUint16(udp[2:], f.DstPort)
	binary.BigEndian.PutUint16(udp[4:], uint16(n-EthLen-IPv6Len-HbHLen)) // #nosec G115 -- n <= 9000
	return p, nil
}

// Info is what Parse reads from a frame's headers.
type Info struct {
	FlowLabel uint32
	Flags     uint8
	Chaos     bool // Flags & FlagChaos
	CRCOK     bool // Monad CRC over bytes 0-17 matches bytes 18-19
	Monad     [MonadLen]byte
}

// Parse reads the first HeaderLen bytes of a frame: EtherType, IPv6 next
// header, the HbH option type and length, the Monad. It never touches the
// payload. A frame that is not IPv6+HbH+Monad is ErrNotMonad; a frame too
// short to hold the headers is ErrShort.
func Parse(b []byte) (Info, error) {
	if len(b) < EthLen+IPv6Len+HbHLen {
		return Info{}, ErrShort
	}
	if binary.BigEndian.Uint16(b[12:]) != EtherIPv6 {
		return Info{}, ErrNotMonad
	}
	ip := b[EthLen:]
	if ip[0]>>4 != 6 || ip[6] != NextHbH {
		return Info{}, ErrNotMonad
	}
	hbh := ip[IPv6Len:]
	if hbh[2] != OptType || hbh[3] != OptDataLen {
		return Info{}, ErrNotMonad
	}
	var in Info
	in.FlowLabel = binary.BigEndian.Uint32(ip[0:]) & FlowMask
	copy(in.Monad[:], hbh[4:4+MonadLen])
	in.Flags = in.Monad[offFlags]
	in.Chaos = in.Flags&FlagChaos != 0
	in.CRCOK = binary.BigEndian.Uint16(in.Monad[offChecksum:]) == encoding.CRC16CCITT(in.Monad[:offChecksum])
	return in, nil
}

// ParseFlowLabel reads a flow label as decimal ("24289") or 0x-prefixed hex
// ("0x5EE1"), 1..0xFFFFF; anything else, trailing characters included, is
// an error.
func ParseFlowLabel(s string) (uint32, error) {
	v, err := strconv.ParseUint(s, 0, 32)
	if err != nil || v == 0 || v > FlowMask {
		return 0, fmt.Errorf("flow label %q: want 1..0xFFFFF", s)
	}
	return uint32(v), nil
}
