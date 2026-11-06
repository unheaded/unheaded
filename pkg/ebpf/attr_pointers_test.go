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
		{reflect.TypeFor[bpfProgLoadAttr](), []string{"Insns", "License", "LogBuf", "FuncInfo", "LineInfo", "FDArray"}},
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
	for name, off := range map[string]struct{ got, want uintptr }{
		"FuncInfo": {unsafe.Offsetof(bpfProgLoadAttr{}.FuncInfo), 80},
		"LineInfo": {unsafe.Offsetof(bpfProgLoadAttr{}.LineInfo), 96},
		"FDArray":  {unsafe.Offsetof(bpfProgLoadAttr{}.FDArray), 120},
	} {
		if off.got != off.want {
			t.Errorf("bpfProgLoadAttr.%s is at %d, want %d", name, off.got, off.want)
		}
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

// Why the fields are typed. This shows the runtime's stack copier on local types,
// not the loader, so it compiles whatever the attr structs hold;
// TestAttrPointersAreTyped is the check on them. After the stack moves, a pointer
// field still names the buffer and an integer copy of the old address does not.
func TestTypedFieldFollowsAStackMoveAndAnIntegerDoesNot(t *testing.T) {
	done := make(chan string, 1)
	go func() {
		var key [8]byte
		before := uintptr(unsafe.Pointer(&key[0]))
		typed := struct{ Key unsafe.Pointer }{unsafe.Pointer(&key[0])}
		integer := struct{ Key uint64 }{uint64(before)}
		growStack(256)
		now := uintptr(unsafe.Pointer(&key[0]))
		switch {
		case now == before:
			done <- "the stack did not move, so this test proved nothing"
		case uintptr(typed.Key) != now:
			done <- "the pointer field was left behind by the stack move"
		case uintptr(integer.Key) == now:
			done <- "the integer followed the stack, which nothing should do"
		default:
			done <- ""
		}
	}()
	if msg := <-done; msg != "" {
		t.Error(msg)
	}
}
