// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build linux

package ebpf

import (
	"context"
	"errors"
	"math"
	"testing"
)

func oneMapLoader(typ MapType) *NativeLoader {
	return &NativeLoader{programs: map[string]*loadedProgram{"p": {
		maps: map[string]*loadedMap{"m": {fd: -1, info: &MapInfo{Type: typ, KeySize: 4, ValueSize: 8}}},
	}}}
}

// The kernel reads and writes the map's own key and value sizes through these
// buffers, whatever the slices hold, so a mismatch stops before the syscall.
func TestNativeLoaderMapOpsHoldToTheMapsSizes(t *testing.T) {
	ctx := context.Background()
	l := oneMapLoader(MapTypeHash)
	good, val := make([]byte, 4), make([]byte, 8)

	for _, key := range [][]byte{nil, {}, make([]byte, 3), make([]byte, 5)} {
		if _, err := l.ReadMap(ctx, "p", "m", key); !errors.Is(err, ErrKeySize) {
			t.Errorf("ReadMap key of %d bytes: %v, want ErrKeySize", len(key), err)
		}
		if err := l.WriteMap(ctx, "p", "m", key, val); !errors.Is(err, ErrKeySize) {
			t.Errorf("WriteMap key of %d bytes: %v, want ErrKeySize", len(key), err)
		}
		if err := l.DeleteMapEntry(ctx, "p", "m", key); !errors.Is(err, ErrKeySize) {
			t.Errorf("DeleteMapEntry key of %d bytes: %v, want ErrKeySize", len(key), err)
		}
	}
	for _, v := range [][]byte{nil, make([]byte, 7), make([]byte, 9)} {
		if err := l.WriteMap(ctx, "p", "m", good, v); !errors.Is(err, ErrValueSize) {
			t.Errorf("WriteMap value of %d bytes: %v, want ErrValueSize", len(v), err)
		}
	}
}

// A per-CPU lookup writes one value per CPU, more than one value's buffer holds,
// and a ring buffer has no elements to read. PinnedMap refuses them too.
func TestNativeLoaderRefusesMapsItsBuffersDoNotFit(t *testing.T) {
	ctx := context.Background()
	for _, kernelType := range []uint32{
		BPF_MAP_TYPE_PERCPU_HASH, BPF_MAP_TYPE_PERCPU_ARRAY, BPF_MAP_TYPE_LRU_PERCPU_HASH,
		BPF_MAP_TYPE_PERCPU_CGROUP_STORAGE, BPF_MAP_TYPE_RINGBUF,
	} {
		typ := mapTypeToMapType(kernelType)
		l := oneMapLoader(typ)
		if _, err := l.ReadMap(ctx, "p", "m", make([]byte, 4)); !errors.Is(err, ErrUnsupportedMapType) {
			t.Errorf("ReadMap on %s: %v, want ErrUnsupportedMapType", typ, err)
		}
		if err := l.WriteMap(ctx, "p", "m", make([]byte, 4), make([]byte, 8)); !errors.Is(err, ErrUnsupportedMapType) {
			t.Errorf("WriteMap on %s: %v, want ErrUnsupportedMapType", typ, err)
		}
		err := l.IterateMap(ctx, "p", "m", func(_, _ []byte) error { return nil })
		if !errors.Is(err, ErrUnsupportedMapType) {
			t.Errorf("IterateMap on %s: %v, want ErrUnsupportedMapType", typ, err)
		}
	}
}

// A map with no key, such as a queue, has nothing to iterate by key.
func TestNativeLoaderIterateRefusesAKeylessMap(t *testing.T) {
	l := oneMapLoader(MapTypeQueue)
	l.programs["p"].maps["m"].info.KeySize = 0
	err := l.IterateMap(context.Background(), "p", "m", func(_, _ []byte) error { return nil })
	if !errors.Is(err, ErrUnsupportedMapType) {
		t.Fatalf("IterateMap: %v, want ErrUnsupportedMapType", err)
	}
}

// The map types a kernel object can hold that still fit one buffer stay usable.
func TestOneBufferMapTypesStayUsable(t *testing.T) {
	for _, kernelType := range []uint32{BPF_MAP_TYPE_HASH, BPF_MAP_TYPE_ARRAY, BPF_MAP_TYPE_LRU_HASH, BPF_MAP_TYPE_CGROUP_STORAGE} {
		if typ := mapTypeToMapType(kernelType); !typ.elemFitsOneBuffer() {
			t.Errorf("kernel map type %d (%s) was refused", kernelType, typ)
		}
	}
}

// A section the value size cannot name is refused, not narrowed.
func TestRodataValueSizeRefusesWhatItCannotHold(t *testing.T) {
	if got, err := rodataValueSize(math.MaxUint32); err != nil || got != math.MaxUint32 {
		t.Errorf("rodataValueSize(MaxUint32) = %d, %v", got, err)
	}
	if got, err := rodataValueSize(math.MaxUint32 + 1); !errors.Is(err, ErrInvalidProgram) {
		t.Errorf("rodataValueSize(MaxUint32+1) = %d, %v, want ErrInvalidProgram", got, err)
	}
}
