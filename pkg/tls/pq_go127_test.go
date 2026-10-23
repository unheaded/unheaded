// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//go:build go1.27

package tls

import (
	"crypto/tls"
	"testing"
)

// TestMLKEM1024Constant pins pq_test.go's literal to crypto/tls. It lives in
// its own file because go.mod says go 1.26 and vet rejects the go1.27 name.
func TestMLKEM1024Constant(t *testing.T) {
	if tls.MLKEM1024 != mlkem1024 {
		t.Fatalf("tls.MLKEM1024 = %#x, test literal %#x", uint16(tls.MLKEM1024), uint16(mlkem1024))
	}
}
