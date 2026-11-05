// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

import (
	"reflect"
	"testing"
)

// A pointer the kernel follows lives in a pointer-typed field, where the garbage
// collector and a stack copy can see it. See pkg/ebpf's attr structs.
func TestAttrPointersAreTyped(t *testing.T) {
	for _, tc := range []struct {
		attr   reflect.Type
		fields []string
	}{
		{reflect.TypeFor[bpfObjGetAttr](), []string{"pathname"}},
		{reflect.TypeFor[bpfMapUpdateAttr](), []string{"key", "value"}},
		{reflect.TypeFor[bpfMapLookupAttr](), []string{"key", "value"}},
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
