// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package pqc

import "testing"

// Parameter ids do not move until one table owns them (RFC desk item 8). The
// SHAKE sets take PQC-00's own 0x07-0x0C; the SHA2 sets keep 0x10-0x15, which
// is not PQC-00's numbering; ML-DSA keeps 0x01-0x03.
func TestSLHDSAParameterIDs(t *testing.T) {
	tests := []struct {
		ps      ParameterSet
		id      uint8
		name    string
		sigSize int
		pkSize  int
		skSize  int
	}{
		{SLHDSA_SHAKE_128s, 0x07, "SLH-DSA-SHAKE-128s", 7856, 32, 64},
		{SLHDSA_SHAKE_128f, 0x08, "SLH-DSA-SHAKE-128f", 17088, 32, 64},
		{SLHDSA_SHAKE_192s, 0x09, "SLH-DSA-SHAKE-192s", 16224, 48, 96},
		{SLHDSA_SHAKE_192f, 0x0A, "SLH-DSA-SHAKE-192f", 35664, 48, 96},
		{SLHDSA_SHAKE_256s, 0x0B, "SLH-DSA-SHAKE-256s", 29792, 64, 128},
		{SLHDSA_SHAKE_256f, 0x0C, "SLH-DSA-SHAKE-256f", 49856, 64, 128},
		{SLHDSA_SHA2_128s, 0x10, "SLH-DSA-SHA2-128s", 7856, 32, 64},
		{SLHDSA_SHA2_128f, 0x11, "SLH-DSA-SHA2-128f", 17088, 32, 64},
		{SLHDSA_SHA2_192s, 0x12, "SLH-DSA-SHA2-192s", 16224, 48, 96},
		{SLHDSA_SHA2_192f, 0x13, "SLH-DSA-SHA2-192f", 35664, 48, 96},
		{SLHDSA_SHA2_256s, 0x14, "SLH-DSA-SHA2-256s", 29792, 64, 128},
		{SLHDSA_SHA2_256f, 0x15, "SLH-DSA-SHA2-256f", 49856, 64, 128},
	}
	if len(slhdsaParamToID) != len(tests) || len(slhdsaParamInfo) != len(tests) {
		t.Fatalf("tables hold %d ids and %d info rows, want %d of each",
			len(slhdsaParamToID), len(slhdsaParamInfo), len(tests))
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if uint8(tt.ps) != tt.id {
				t.Errorf("id = %#02x, want %#02x", uint8(tt.ps), tt.id)
			}
			name, sigSize, pkSize, skSize, ok := SLHDSAParamInfo(tt.ps)
			if !ok {
				t.Fatalf("SLHDSAParamInfo(%#02x) not found", tt.id)
			}
			if name != tt.name || sigSize != tt.sigSize || pkSize != tt.pkSize || skSize != tt.skSize {
				t.Errorf("info = %q sig %d pk %d sk %d, want %q sig %d pk %d sk %d",
					name, sigSize, pkSize, skSize, tt.name, tt.sigSize, tt.pkSize, tt.skSize)
			}

			// The row must name the circl set it claims to: sizes alone cannot
			// tell SHA2 from SHAKE, or s from f at the wrong level.
			id, ok := slhdsaParamToID[tt.ps]
			if !ok {
				t.Fatalf("slhdsaParamToID[%#02x] missing", tt.id)
			}
			if id.String() != tt.name {
				t.Errorf("circl id = %q, want %q", id.String(), tt.name)
			}
			scheme := id.Scheme()
			if scheme.SignatureSize() != tt.sigSize || scheme.PublicKeySize() != tt.pkSize || scheme.PrivateKeySize() != tt.skSize {
				t.Errorf("circl sizes sig %d pk %d sk %d, want sig %d pk %d sk %d",
					scheme.SignatureSize(), scheme.PublicKeySize(), scheme.PrivateKeySize(),
					tt.sigSize, tt.pkSize, tt.skSize)
			}
		})
	}
}

func TestMLDSAParameterIDs(t *testing.T) {
	for _, tt := range []struct {
		ps ParameterSet
		id uint8
	}{{MLDSA44, 0x01}, {MLDSA65, 0x02}, {MLDSA87, 0x03}} {
		if uint8(tt.ps) != tt.id {
			t.Errorf("ML-DSA id = %#02x, want %#02x", uint8(tt.ps), tt.id)
		}
	}
}
