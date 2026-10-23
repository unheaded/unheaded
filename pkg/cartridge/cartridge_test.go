// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package cartridge

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"testing"
)

// buildImage mirrors upc-cartridge's Image::encode (ADR-108 ABI v0).
func buildImage(stage uint8, maxInsn, entry, loopBound, flags uint16, rom, data []uint32) []byte {
	b := make([]byte, 0, HeaderLen+4*(len(rom)+len(data)))
	b = append(b, 'M', 'B', 'C', 'C', 0, stage)
	b = binary.LittleEndian.AppendUint16(b, maxInsn)
	b = binary.LittleEndian.AppendUint16(b, uint16(len(rom)))
	b = binary.LittleEndian.AppendUint16(b, uint16(len(data)))
	b = binary.LittleEndian.AppendUint16(b, entry)
	b = binary.LittleEndian.AppendUint16(b, loopBound)
	b = binary.LittleEndian.AppendUint16(b, flags)
	b = binary.LittleEndian.AppendUint16(b, 0)
	var body []byte
	for _, w := range append(append([]uint32{}, rom...), data...) {
		body = binary.LittleEndian.AppendUint32(body, w)
	}
	h := sha256.New()
	h.Write(b[:20])
	h.Write(body)
	b = append(b, h.Sum(nil)...)
	return append(b, body...)
}

func TestDecodeImage(t *testing.T) {
	t.Parallel()
	raw := buildImage(StageXDP, 64, 1, 0, FlagFailClosed, []uint32{0x0F100000, 0xFF000000}, []uint32{7})
	img, err := DecodeImage(raw)
	if err != nil {
		t.Fatal(err)
	}
	if img.Stage != StageXDP || img.MaxInsn != 64 || img.Entry != 1 || img.Flags != FlagFailClosed ||
		len(img.ROM) != 2 || img.ROM[1] != 0xFF000000 || len(img.Data) != 1 || img.Data[0] != 7 {
		t.Fatalf("decoded %+v", img)
	}
	if img.SHA256 != [32]byte(raw[20:52]) {
		t.Fatal("sha not carried")
	}
}

func TestDecodeImage_Hostile(t *testing.T) {
	t.Parallel()
	good := buildImage(StageXDP, 64, 0, 0, 0, []uint32{0xFF000000}, nil)
	mut := func(f func(b []byte) []byte) []byte { return f(append([]byte{}, good...)) }
	cases := map[string][]byte{
		"empty":     nil,
		"short":     good[:51],
		"magic":     mut(func(b []byte) []byte { b[0] = 'X'; return b }),
		"version":   mut(func(b []byte) []byte { b[4] = 1; return b }),
		"stage":     mut(func(b []byte) []byte { b[5] = 9; return b }),
		"rom 0":     mut(func(b []byte) []byte { b[8] = 0; return b }),
		"rom 1025":  mut(func(b []byte) []byte { binary.LittleEndian.PutUint16(b[8:], 1025); return b }),
		"data 1025": mut(func(b []byte) []byte { binary.LittleEndian.PutUint16(b[10:], 1025); return b }),
		"trailing":  mut(func(b []byte) []byte { return append(b, 0) }),
		"reserved":  mut(func(b []byte) []byte { b[18] = 1; return b }),
		"flag":      mut(func(b []byte) []byte { b[16] = 2; return b }),
		"entry":     mut(func(b []byte) []byte { b[12] = 1; return b }),
		"sha":       mut(func(b []byte) []byte { b[52] ^= 1; return b }),
	}
	for name, b := range cases {
		if _, err := DecodeImage(b); !errors.Is(err, ErrImage) {
			t.Errorf("%s: err = %v, want ErrImage", name, err)
		}
	}
	for i := range good {
		for bit := 0; bit < 8; bit++ {
			b := append([]byte{}, good...)
			b[i] ^= 1 << bit
			if _, err := DecodeImage(b); err == nil {
				t.Fatalf("flip byte %d bit %d accepted", i, bit)
			}
		}
	}
}

func FuzzDecodeImage(f *testing.F) {
	f.Add(buildImage(StageTick, 256, 0, 4, 0, []uint32{0, 0xFF000000}, []uint32{1, 2}))
	f.Fuzz(func(t *testing.T, b []byte) { _, _ = DecodeImage(b) })
}

func TestSlotCfgEncoding(t *testing.T) {
	t.Parallel()
	c := SlotCfg{Active: 1, Stage: StageTick, Flags: 1, MaxInsn: 200, ROMWords: 3, DataWords: 4, Entry: 2, LoopBound: 9, Gen: 0xA1B2C3D4}
	b := c.MarshalBinary()
	want := []byte{1, 1, 1, 0, 200, 0, 3, 0, 4, 0, 2, 0, 9, 0, 0, 0, 0xD4, 0xC3, 0xB2, 0xA1}
	if string(b) != string(want) {
		t.Fatalf("got % x\nwant % x", b, want)
	}
	if len(b) != SlotCfgSize {
		t.Fatal("size")
	}
}

// fakeMaps records the order of every map write.
type fakeMaps struct{ log []string }

func (f *fakeMaps) putROM(slot, i int, w uint32) error  { f.log = append(f.log, "rom"); return nil }
func (f *fakeMaps) putData(slot, i int, w uint32) error { f.log = append(f.log, "data"); return nil }
func (f *fakeMaps) putSlot(slot int, c SlotCfg) error {
	if c.Active == 0 {
		f.log = append(f.log, "slot-off")
	} else {
		f.log = append(f.log, "slot-on")
	}
	return nil
}

func TestWriteSlot_GenLast(t *testing.T) {
	t.Parallel()
	img, err := DecodeImage(buildImage(StageXDP, 64, 0, 0, 0, []uint32{1, 2}, []uint32{3}))
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeMaps{}
	if err := writeSlot(f, 2, img, 7); err != nil {
		t.Fatal(err)
	}
	want := []string{"slot-off", "rom", "rom", "data", "slot-on"}
	if len(f.log) != len(want) {
		t.Fatalf("log %v", f.log)
	}
	for i := range want {
		if f.log[i] != want[i] {
			t.Fatalf("order %v, want %v (config with the new gen must be written LAST)", f.log, want)
		}
	}
	if err := writeSlot(f, Slots, img, 1); err == nil {
		t.Fatal("slot out of range accepted")
	}
}

func TestFlowKeyEncoding(t *testing.T) {
	t.Parallel()
	k := FlowKey{Sport: 0x1234, Dport: 9, Proto: 17, L3: 4, Slot: 1}
	k.Src[0], k.Dst[3] = 10, 2
	b := k.MarshalBinary()
	if len(b) != FlowKeySize || b[0] != 10 || b[19] != 2 || b[32] != 0x34 || b[33] != 0x12 || b[34] != 9 || b[36] != 17 || b[37] != 4 || b[38] != 1 {
		t.Fatalf("% x", b)
	}
}

func TestDecodeState(t *testing.T) {
	t.Parallel()
	b := make([]byte, CartStateSize)
	binary.LittleEndian.PutUint32(b[0:], 5)        // r0
	binary.LittleEndian.PutUint32(b[64+4*3:], 77)  // mem[3]
	binary.LittleEndian.PutUint16(b[320:], 9)      // pc
	b[322] = 1                                     // flags
	binary.LittleEndian.PutUint32(b[332:], 0xBEEF) // gen
	s, err := DecodeState(b)
	if err != nil || s.Regs[0] != 5 || s.Mem[3] != 77 || s.PC != 9 || s.Flags != 1 || s.Gen != 0xBEEF {
		t.Fatalf("%+v %v", s, err)
	}
	// FNV-1a over regs||pc||flags||mem, the ADR-108 state hash.
	if s.Hash() == (CartState{}).Hash() {
		t.Fatal("hash ignores state")
	}
	// Cross-language vector shared with monad_common::cart's cart_state_hash_vector.
	var v CartState
	for i := range v.Regs {
		v.Regs[i] = uint32(i)
	}
	for i := range v.Mem {
		v.Mem[i] = uint32(3 * i)
	}
	v.PC, v.Flags = 7, 5
	if got := v.Hash(); got != 0x6da0d6f5 {
		t.Fatalf("hash vector %#x, want 0x6da0d6f5", got)
	}
	if _, err := DecodeState(b[:10]); err == nil {
		t.Fatal("short state accepted")
	}
}

func TestDecodeScoreJob(t *testing.T) {
	t.Parallel()
	b := make([]byte, ScoreJobSize)
	b[0], b[32], b[36], b[37], b[38] = 10, 0x35, 17, 4, 1 // src[0], sport lo, proto, l3, slot
	for i := 0; i < FeatWords; i++ {
		binary.LittleEndian.PutUint32(b[40+4*i:], uint32(100+i)) // #nosec G115 -- i < 8
	}
	b[72] = 1
	binary.LittleEndian.PutUint32(b[76:], 0xCAFE)
	j, err := DecodeScoreJob(b)
	if err != nil {
		t.Fatal(err)
	}
	if j.Flow.Src[0] != 10 || j.Flow.Sport != 0x35 || j.Flow.Proto != 17 || j.Flow.L3 != 4 || j.Flow.Slot != 1 {
		t.Fatalf("flow %+v", j.Flow)
	}
	if j.Feat[0] != 100 || j.Feat[7] != 107 || j.SrcSlot != 1 || j.EnqNSLo != 0xCAFE {
		t.Fatalf("job %+v", j)
	}
	if _, err := DecodeScoreJob(b[:79]); err == nil {
		t.Fatal("short job accepted")
	}
}
