// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build linux

package ebpf

import (
	"reflect"
	"testing"
	"unsafe"
)

// A pointer the kernel follows has to live in a pointer-typed field. Held as an
// integer, the garbage collector cannot see it and a stack copy does not update
// it, so a buffer on the caller's stack moves away between building the attr and
// entering the kernel, and the kernel reads or writes the old stack.
func TestAttrPointersAreTyped(t *testing.T) {
	for _, tc := range []struct {
		attr   reflect.Type
		fields []string
	}{
		{reflect.TypeFor[bpfMapOpAttr](), []string{"Key", "Value"}},
		{reflect.TypeFor[bpfProgLoadAttr](), []string{"Insns", "License", "LogBuf"}},
		{reflect.TypeFor[bpfObjAttr](), []string{"Pathname"}},
		{reflect.TypeFor[bpfObjGetInfoAttr](), []string{"Info"}},
		{reflect.TypeFor[bpfRawTPLinkAttr](), []string{"RawTPName"}},
		{reflect.TypeFor[bpfBTFLoadAttr](), []string{"BTF"}},
		{reflect.TypeFor[perfEventAttr](), []string{"ExtConfig"}},
	} {
		for _, name := range tc.fields {
			f, ok := tc.attr.FieldByName(name)
			if !ok {
				t.Errorf("%s has no field %s", tc.attr.Name(), name)
				continue
			}
			if f.Type.Kind() != reflect.UnsafePointer {
				t.Errorf("%s.%s is %s, want unsafe.Pointer", tc.attr.Name(), name, f.Type)
			}
		}
	}
}

// The ABI gives each of those fields 8 bytes, and the structs keep their size.
func TestAttrPointerFieldsKeepTheABI(t *testing.T) {
	if got := unsafe.Sizeof(bpfMapOpAttr{}); got != 32 {
		t.Errorf("bpfMapOpAttr is %d bytes, want 32", got)
	}
	if got := unsafe.Sizeof(bpfObjAttr{}); got != 16 {
		t.Errorf("bpfObjAttr is %d bytes, want 16", got)
	}
	if got := unsafe.Sizeof(bpfObjGetInfoAttr{}); got != 16 {
		t.Errorf("bpfObjGetInfoAttr is %d bytes, want 16", got)
	}
	if got := unsafe.Offsetof(perfEventAttr{}.ExtConfig); got != 56 {
		t.Errorf("perfEventAttr.ExtConfig is at %d, want 56", got)
	}
}

//go:noinline
func growStack(n int) byte {
	var pad [4096]byte
	pad[n%len(pad)] = byte(n)
	if n == 0 {
		return pad[0]
	}
	return growStack(n-1) + pad[n%len(pad)]
}

// What the typed field buys: after the stack moves the field still names the
// buffer, where an integer copy of the old address would not.
func TestAttrPointerFollowsAStackMove(t *testing.T) {
	done := make(chan string, 1)
	go func() {
		var key [8]byte
		before := uintptr(unsafe.Pointer(&key[0]))
		attr := bpfMapOpAttr{Key: unsafe.Pointer(&key[0])}
		growStack(256)
		now := uintptr(unsafe.Pointer(&key[0]))
		switch {
		case now == before:
			done <- "the stack did not move, so this test proved nothing"
		case uintptr(attr.Key) != now:
			done <- "attr.Key was left behind by the stack move"
		default:
			done <- ""
		}
	}()
	if msg := <-done; msg != "" {
		t.Error(msg)
	}
}
