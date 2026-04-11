// SPDX-License-Identifier: GPL-3.0-or-later
// Package gjallarhorn implements the UPC trigger packet sender/receiver
// for Mímir's Law / Gleipnir Phase 0 PoC. Per ADR-043 §Decision: discrete
// trigger packets that fit within the frozen Monad v0x01 wire format.
//
// Wire layout (20-byte Monad register payload):
//   bytes [0:4]   : magic "GJLR" (0x474A4C52)
//   bytes [4:5]   : trigger_kind (0x01=BOOTSTRAP_BROADCAST, 0x02=REVERIFY_UNICAST)
//   bytes [5:9]   : cluster_id (uint32 BE)
//   bytes [9:17]  : Mjölnir manifest pointer (uint64 BE)
//   bytes [17:20] : reserved/padding
package gjallarhorn

import (
	"encoding/binary"
	"errors"
)

const (
	PacketSize = 20
)

var Magic = [4]byte{'G', 'J', 'L', 'R'}

type TriggerKind uint8

const (
	BootstrapBroadcast TriggerKind = 0x01
	ReverifyUnicast    TriggerKind = 0x02
)

// Packet is the 20-byte Monad register payload for a Gjallarhorn UPC trigger.
type Packet struct {
	Magic       [4]byte
	Kind        TriggerKind
	ClusterID   uint32
	ManifestPtr uint64
}

var (
	ErrShortPacket = errors.New("gjallarhorn: short packet")
	ErrBadMagic    = errors.New("gjallarhorn: magic mismatch (not GJLR)")
	ErrBadKind     = errors.New("gjallarhorn: unknown trigger kind")
)

// Marshal encodes a Packet into 20 bytes (the Monad register payload).
func (p *Packet) Marshal() []byte {
	buf := make([]byte, PacketSize)
	copy(buf[0:4], Magic[:])
	buf[4] = byte(p.Kind)
	binary.BigEndian.PutUint32(buf[5:9], p.ClusterID)
	binary.BigEndian.PutUint64(buf[9:17], p.ManifestPtr)
	// bytes 17:20 reserved (already zero)
	return buf
}

// Unmarshal decodes a 20-byte buffer into a Packet.
func Unmarshal(buf []byte) (*Packet, error) {
	if len(buf) < PacketSize {
		return nil, ErrShortPacket
	}
	if buf[0] != 'G' || buf[1] != 'J' || buf[2] != 'L' || buf[3] != 'R' {
		return nil, ErrBadMagic
	}
	kind := TriggerKind(buf[4])
	if kind != BootstrapBroadcast && kind != ReverifyUnicast {
		return nil, ErrBadKind
	}
	return &Packet{
		Magic:       Magic,
		Kind:        kind,
		ClusterID:   binary.BigEndian.Uint32(buf[5:9]),
		ManifestPtr: binary.BigEndian.Uint64(buf[9:17]),
	}, nil
}
