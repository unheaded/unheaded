// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package config

import (
	"reflect"
	"strings"
	"testing"
)

// Pin the negative-int → unsigned-field rejection added 2026-05-09 to close
// gosec G115 silent-wrap (int -> uint64 truncation that turned -1 into
// 0xFFFFFFFFFFFFFFFF). Without this gate, a user supplying -1 to a uint
// config field gets the maximum u64 value silently.

type uintHolder struct {
	V uint64
}

type uint32Holder struct {
	V uint32
}

func TestSetFieldValue_RejectsNegativeIntForUnsignedField(t *testing.T) {
	t.Parallel()
	h := &uintHolder{}
	field := reflect.ValueOf(h).Elem().FieldByName("V")

	err := setFieldValue(field, -1)
	if err == nil {
		t.Fatal("expected error for negative int → uint, got nil")
	}
	if !strings.Contains(err.Error(), "negative") {
		t.Errorf("expected 'negative' in error message, got: %v", err)
	}
	if h.V != 0 {
		t.Errorf("V should be untouched on error, got %d", h.V)
	}
}

func TestSetFieldValue_RejectsNegativeInt64ForUnsignedField(t *testing.T) {
	t.Parallel()
	h := &uintHolder{}
	field := reflect.ValueOf(h).Elem().FieldByName("V")

	err := setFieldValue(field, int64(-100))
	if err == nil {
		t.Fatal("expected error for negative int64 → uint, got nil")
	}
}

func TestSetFieldValue_RejectsNegativeFloat64ForUnsignedField(t *testing.T) {
	t.Parallel()
	h := &uintHolder{}
	field := reflect.ValueOf(h).Elem().FieldByName("V")

	err := setFieldValue(field, float64(-2.5))
	if err == nil {
		t.Fatal("expected error for negative float64 → uint, got nil")
	}
	if !strings.Contains(err.Error(), "negative") {
		t.Errorf("expected 'negative' in error: %v", err)
	}
}

func TestSetFieldValue_AcceptsValidPositiveInt(t *testing.T) {
	t.Parallel()
	h := &uintHolder{}
	field := reflect.ValueOf(h).Elem().FieldByName("V")
	if err := setFieldValue(field, 42); err != nil {
		t.Errorf("positive int should succeed, got: %v", err)
	}
	if h.V != 42 {
		t.Errorf("V = %d, want 42", h.V)
	}
}

func TestSetFieldValue_AcceptsZero(t *testing.T) {
	t.Parallel()
	h := &uintHolder{V: 99}
	field := reflect.ValueOf(h).Elem().FieldByName("V")
	if err := setFieldValue(field, 0); err != nil {
		t.Errorf("zero should succeed, got: %v", err)
	}
	if h.V != 0 {
		t.Errorf("V = %d, want 0 (zero should overwrite)", h.V)
	}
}

func TestSetFieldValue_AcceptsLargePositiveInt64(t *testing.T) {
	t.Parallel()
	h := &uintHolder{}
	field := reflect.ValueOf(h).Elem().FieldByName("V")
	want := int64(1 << 60)
	if err := setFieldValue(field, want); err != nil {
		t.Errorf("large positive int64 should succeed, got: %v", err)
	}
	if h.V != uint64(want) {
		t.Errorf("V = %d, want %d", h.V, want)
	}
}
