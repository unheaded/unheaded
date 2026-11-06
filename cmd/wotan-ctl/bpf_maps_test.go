// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"bytes"
	"errors"
	"reflect"
	"testing"
	"unsafe"
)

// The kernel reads the map's value size through the buffer, so CpuState has to be
// MbcCpuState byte for byte (ebpf/monad-common/src/lib.rs, 144 bytes with
// ascend-linux). A short struct would hand the kernel whatever follows it.
func TestCpuStateMatchesTheKernelLayout(t *testing.T) {
	if got := unsafe.Sizeof(CpuState{}); got != 144 {
		t.Errorf("CpuState is %d bytes, want 144", got)
	}
	typ := reflect.TypeFor[CpuState]()
	for name, want := range map[string]uintptr{
		"PageDirBase": 124, "PrivLevel": 128, "ReservationAddress": 132, "Rv2mbcBase": 136,
	} {
		f, ok := typ.FieldByName(name)
		if !ok {
			t.Errorf("CpuState has no field %s", name)
			continue
		}
		if f.Offset != want {
			t.Errorf("CpuState.%s is at %d, want %d", name, f.Offset, want)
		}
	}
}

func TestCpuStateDefaultsToNoReservation(t *testing.T) {
	if got := defaultCpuState(0).ReservationAddress; got != noReservation {
		t.Errorf("ReservationAddress = %#x, want no reservation", got)
	}
}

type fakeElemMap struct {
	valueSize  uint32
	key, value []byte
	err        error
}

func (f *fakeElemMap) ValueSize() uint32 { return f.valueSize }
func (f *fakeElemMap) Close() error      { return nil }
func (f *fakeElemMap) Update(key, value []byte) error {
	f.key, f.value = key, value
	return f.err
}

// CpuState goes out at the size of the kernel object in front of it.
func TestUpdateSizesCpuStateToTheMap(t *testing.T) {
	for _, size := range []uint32{136, 144} {
		f := &fakeElemMap{valueSize: size}
		if err := (&BPFMap{m: f}).Update(uint32(0), defaultCpuState(0)); err != nil {
			t.Fatalf("value size %d: %v", size, err)
		}
		if uint32(len(f.value)) != size {
			t.Errorf("map of value size %d was handed %d bytes", size, len(f.value))
		}
	}
}

// Only CpuState is trimmed: anything else reaches the map as encoded, so the map's
// own size check sees it, and that check's refusal comes back to the caller.
func TestUpdateLeavesOtherValuesToTheMapsCheck(t *testing.T) {
	refused := errors.New("refused")
	f := &fakeElemMap{valueSize: 136, err: refused}
	val := make([]byte, 200)
	err := (&BPFMap{m: f}).Update(uint32(7), val)
	if !errors.Is(err, refused) {
		t.Fatalf("Update: %v, want the map's refusal", err)
	}
	if !bytes.Equal(f.key, []byte{7, 0, 0, 0}) || len(f.value) != 200 {
		t.Errorf("map saw key %v and %d value bytes", f.key, len(f.value))
	}
}

func TestUnopenedMapIsRefused(t *testing.T) {
	var none *BPFMap
	if err := none.Update(uint32(0), uint32(0)); err == nil {
		t.Error("Update on a nil map succeeded")
	}
	if err := (&BPFMap{}).Update(uint32(0), uint32(0)); err == nil {
		t.Error("Update on an unopened map succeeded")
	}
	if err := none.Close(); err != nil {
		t.Errorf("Close on a nil map: %v", err)
	}
}
