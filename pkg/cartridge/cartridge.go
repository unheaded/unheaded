// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package cartridge loads and drives ebpf/cartridge-ebpf (ADR-108, MBC
// cartridges): it decodes .cart images, writes slots (ROM, DATA, then the
// slot config with its new generation LAST, so the interpreter never runs a
// half-written slot), runs cart_xdp / cart_tick through BPF_PROG_TEST_RUN,
// attaches cart_xdp to an interface, and reads state, stats and events back.
//
// This file is the OS-independent half: the ABI v0 records and their
// little-endian encodings, kept byte-for-byte with monad_common::cart.
package cartridge

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
)

// ABI v0 constants (ADR-108).
const (
	HeaderLen      = 52
	StageXDP       = 0
	StageTick      = 1
	FlagFailClosed = 1
	Slots          = 4
	SlotWords      = 1024
	MemWords       = 64
	FeatWords      = 8
	FieldCount     = 13
	MetaSlot       = 0xFF

	SlotCfgSize   = 20
	FlowKeySize   = 40
	CartStateSize = 336
	ScoreJobSize  = 80
	CartEventSize = 64
)

// CART_STATS counters (monad_common::cart::stat).
const (
	StatRuns         = 0
	StatPass         = 1
	StatDrop         = 2
	StatFaults       = 3
	StatInsns        = 4
	StatEmits        = 5
	StatQueueOK      = 6
	StatQueueDrops   = 7
	StatXDPPackets   = 32
	StatTickCalls    = 33
	StatTickJobsDone = 34
	StatEventsDrop   = 35
	StatTickIdle     = 36
	StatBreakerTrips = 37
	StatTickStale    = 38
	StatCount        = 40
)

// CART_CFG keys.
const (
	CfgTrace   = 0
	CfgClockNS = 2
)

// ErrImage marks every reason bytes are not a .cart image.
var ErrImage = errors.New("cartridge: bad image")

// Image is a decoded .cart file.
type Image struct {
	Stage     uint8
	MaxInsn   uint16
	Entry     uint16
	LoopBound uint16
	Flags     uint16
	ROM       []uint32
	Data      []uint32
	SHA256    [32]byte
}

func imgErr(format string, a ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrImage}, a...)...)
}

// DecodeImage parses hostile bytes with the same rules as upc-cartridge's
// Image::decode: sizes from the header are range-checked before the length
// is trusted, and the embedded sha256 must match.
func DecodeImage(b []byte) (*Image, error) {
	if len(b) < HeaderLen {
		return nil, imgErr("shorter than the %d-byte header", HeaderLen)
	}
	if string(b[0:4]) != "MBCC" {
		return nil, imgErr("bad magic")
	}
	if b[4] != 0 {
		return nil, imgErr("unknown version %d", b[4])
	}
	stage := b[5]
	if stage != StageXDP && stage != StageTick {
		return nil, imgErr("unknown stage %d", stage)
	}
	le := binary.LittleEndian
	img := &Image{
		Stage:     stage,
		MaxInsn:   le.Uint16(b[6:]),
		Entry:     le.Uint16(b[12:]),
		LoopBound: le.Uint16(b[14:]),
		Flags:     le.Uint16(b[16:]),
	}
	rom, data := int(le.Uint16(b[8:])), int(le.Uint16(b[10:]))
	if rom == 0 || rom > SlotWords {
		return nil, imgErr("rom_words %d", rom)
	}
	if data > SlotWords {
		return nil, imgErr("data_words %d", data)
	}
	if want := HeaderLen + 4*(rom+data); len(b) != want {
		return nil, imgErr("length %d, header implies %d", len(b), want)
	}
	if le.Uint16(b[18:]) != 0 || img.Flags&^FlagFailClosed != 0 {
		return nil, imgErr("reserved field or unknown flag set")
	}
	if int(img.Entry) >= rom {
		return nil, imgErr("entry %d outside the ROM", img.Entry)
	}
	h := sha256.New()
	h.Write(b[:20])
	h.Write(b[HeaderLen:])
	copy(img.SHA256[:], b[20:HeaderLen])
	if [32]byte(h.Sum(nil)) != img.SHA256 {
		return nil, imgErr("embedded sha256 does not match")
	}
	img.ROM = make([]uint32, rom)
	for i := range img.ROM {
		img.ROM[i] = le.Uint32(b[HeaderLen+4*i:])
	}
	img.Data = make([]uint32, data)
	for i := range img.Data {
		img.Data[i] = le.Uint32(b[HeaderLen+4*(rom+i):])
	}
	return img, nil
}

// SlotCfg is the CART_SLOT value.
type SlotCfg struct {
	Active, Stage, Flags                           uint8
	MaxInsn, ROMWords, DataWords, Entry, LoopBound uint16
	Gen                                            uint32
}

// MarshalBinary encodes the 20-byte ABI layout.
func (c SlotCfg) MarshalBinary() []byte {
	b := make([]byte, SlotCfgSize)
	b[0], b[1], b[2] = c.Active, c.Stage, c.Flags
	le := binary.LittleEndian
	le.PutUint16(b[4:], c.MaxInsn)
	le.PutUint16(b[6:], c.ROMWords)
	le.PutUint16(b[8:], c.DataWords)
	le.PutUint16(b[10:], c.Entry)
	le.PutUint16(b[12:], c.LoopBound)
	le.PutUint32(b[16:], c.Gen)
	return b
}

// UnmarshalSlotCfg decodes a CART_SLOT value.
func UnmarshalSlotCfg(b []byte) (SlotCfg, error) {
	if len(b) != SlotCfgSize {
		return SlotCfg{}, fmt.Errorf("cartridge: slot cfg %d bytes", len(b))
	}
	le := binary.LittleEndian
	return SlotCfg{
		Active: b[0], Stage: b[1], Flags: b[2],
		MaxInsn: le.Uint16(b[4:]), ROMWords: le.Uint16(b[6:]), DataWords: le.Uint16(b[8:]),
		Entry: le.Uint16(b[10:]), LoopBound: le.Uint16(b[12:]), Gen: le.Uint32(b[16:]),
	}, nil
}

// SlotCfgFor is the configuration an image loads with.
func SlotCfgFor(img *Image, gen uint32) SlotCfg {
	return SlotCfg{
		Active: 1, Stage: img.Stage, Flags: uint8(img.Flags & FlagFailClosed),
		MaxInsn: img.MaxInsn, ROMWords: uint16(len(img.ROM)), DataWords: uint16(len(img.Data)), // #nosec G115 -- bounded by DecodeImage (<= 1024)
		Entry: img.Entry, LoopBound: img.LoopBound, Gen: gen,
	}
}

// FlowKey is the FLOW_STATE / event flow identity.
type FlowKey struct {
	Src, Dst     [16]byte
	Sport, Dport uint16
	Proto, L3    uint8
	Slot         uint8
}

// MarshalBinary encodes the 40-byte ABI layout.
func (k FlowKey) MarshalBinary() []byte {
	b := make([]byte, FlowKeySize)
	copy(b[0:16], k.Src[:])
	copy(b[16:32], k.Dst[:])
	binary.LittleEndian.PutUint16(b[32:], k.Sport)
	binary.LittleEndian.PutUint16(b[34:], k.Dport)
	b[36], b[37], b[38] = k.Proto, k.L3, k.Slot
	return b
}

func decodeFlowKey(b []byte) FlowKey {
	var k FlowKey
	copy(k.Src[:], b[0:16])
	copy(k.Dst[:], b[16:32])
	k.Sport = binary.LittleEndian.Uint16(b[32:])
	k.Dport = binary.LittleEndian.Uint16(b[34:])
	k.Proto, k.L3, k.Slot = b[36], b[37], b[38]
	return k
}

// CartState is the 336-byte machine state.
type CartState struct {
	Regs      [16]uint32
	Mem       [MemWords]uint32
	PC        uint16
	Flags     uint8
	Status    uint8
	Insns     uint32
	BackEdges uint32
	Gen       uint32
}

// DecodeState decodes a FLOW_STATE value.
func DecodeState(b []byte) (CartState, error) {
	var s CartState
	if len(b) != CartStateSize {
		return s, fmt.Errorf("cartridge: state %d bytes, want %d", len(b), CartStateSize)
	}
	le := binary.LittleEndian
	for i := range s.Regs {
		s.Regs[i] = le.Uint32(b[4*i:])
	}
	for i := range s.Mem {
		s.Mem[i] = le.Uint32(b[64+4*i:])
	}
	s.PC = le.Uint16(b[320:])
	s.Flags, s.Status = b[322], b[323]
	s.Insns = le.Uint32(b[324:])
	s.BackEdges = le.Uint32(b[328:])
	s.Gen = le.Uint32(b[332:])
	return s, nil
}

// Hash is the ADR-108 state_hash: 32-bit FNV-1a over regs || pc || flags ||
// mem, little-endian (monad_common::cart::state_hash).
func (s CartState) Hash() uint32 {
	h := uint32(0x811c9dc5)
	eat := func(b byte) {
		h ^= uint32(b)
		h *= 0x01000193
	}
	var w [4]byte
	for _, r := range s.Regs {
		binary.LittleEndian.PutUint32(w[:], r)
		for _, b := range w {
			eat(b)
		}
	}
	var pc [2]byte
	binary.LittleEndian.PutUint16(pc[:], s.PC)
	eat(pc[0])
	eat(pc[1])
	eat(s.Flags)
	for _, m := range s.Mem {
		binary.LittleEndian.PutUint32(w[:], m)
		for _, b := range w {
			eat(b)
		}
	}
	return h
}

// Event is one CART_EVENTS record.
type Event struct {
	TsNS        uint64
	Flow        FlowKey
	Kind, A, B  uint32
	Slot, Stage uint8
}

// DecodeEvent decodes a 64-byte CART_EVENTS record.
func DecodeEvent(b []byte) (Event, error) {
	if len(b) != CartEventSize {
		return Event{}, fmt.Errorf("cartridge: event %d bytes, want %d", len(b), CartEventSize)
	}
	le := binary.LittleEndian
	return Event{
		TsNS: le.Uint64(b[0:]), Flow: decodeFlowKey(b[8:48]),
		Kind: le.Uint32(b[48:]), A: le.Uint32(b[52:]), B: le.Uint32(b[56:]),
		Slot: b[60], Stage: b[61],
	}, nil
}

// slotWriter is the map surface writeSlot needs (fakes in tests).
type slotWriter interface {
	putROM(slot, i int, w uint32) error
	putData(slot, i int, w uint32) error
	putSlot(slot int, c SlotCfg) error
}

// writeSlot loads img into slot: deactivate, write ROM and DATA, then the
// configuration with the new generation LAST. A packet that runs between
// the steps sees an inactive slot, never a half-written one; flow state of
// an older generation resets on its next packet.
func writeSlot(w slotWriter, slot int, img *Image, gen uint32) error {
	if slot < 0 || slot >= Slots {
		return fmt.Errorf("cartridge: slot %d outside 0..%d", slot, Slots-1)
	}
	if err := w.putSlot(slot, SlotCfg{}); err != nil {
		return fmt.Errorf("deactivate slot %d: %w", slot, err)
	}
	for i, x := range img.ROM {
		if err := w.putROM(slot, i, x); err != nil {
			return fmt.Errorf("rom[%d]: %w", i, err)
		}
	}
	for i, x := range img.Data {
		if err := w.putData(slot, i, x); err != nil {
			return fmt.Errorf("data[%d]: %w", i, err)
		}
	}
	if err := w.putSlot(slot, SlotCfgFor(img, gen)); err != nil {
		return fmt.Errorf("activate slot %d: %w", slot, err)
	}
	return nil
}

// SlotStat is one per-slot counter from a CART_STATS snapshot.
func SlotStat(s [StatCount]uint64, slot, off int) uint64 { return s[slot*8+off] }
