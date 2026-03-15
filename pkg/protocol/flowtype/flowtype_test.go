// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package flowtype

import (
	"fmt"
	"testing"
)

func TestFlowTypeString(t *testing.T) {
	tests := []struct {
		ft       FlowType
		expected string
	}{
		{Control, "Control"},
		{Data, "Data"},
		{Prefetch, "Prefetch"},
		{Reserved, "Reserved"},
		{FlowType(255), "FlowType(255)"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.ft.String(); got != tt.expected {
				t.Errorf("String() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestFlowTypePredicates(t *testing.T) {
	tests := []struct {
		ft       FlowType
		isCtrl   bool
		isData   bool
		isPf     bool
		isRes    bool
	}{
		{Control, true, false, false, false},
		{Data, false, true, false, false},
		{Prefetch, false, false, true, false},
		{Reserved, false, false, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.ft.String(), func(t *testing.T) {
			if got := tt.ft.IsControl(); got != tt.isCtrl {
				t.Errorf("IsControl() = %v, want %v", got, tt.isCtrl)
			}
			if got := tt.ft.IsData(); got != tt.isData {
				t.Errorf("IsData() = %v, want %v", got, tt.isData)
			}
			if got := tt.ft.IsPrefetch(); got != tt.isPf {
				t.Errorf("IsPrefetch() = %v, want %v", got, tt.isPf)
			}
			if got := tt.ft.IsReserved(); got != tt.isRes {
				t.Errorf("IsReserved() = %v, want %v", got, tt.isRes)
			}
		})
	}
}

func TestFlowTypePriority(t *testing.T) {
	tests := []struct {
		ft       FlowType
		priority int
	}{
		{Control, 0},
		{Data, 1},
		{Prefetch, 2},
		{Reserved, 3},
	}

	for _, tt := range tests {
		t.Run(tt.ft.String(), func(t *testing.T) {
			if got := tt.ft.Priority(); got != tt.priority {
				t.Errorf("Priority() = %d, want %d", got, tt.priority)
			}
		})
	}
}

func TestFlowTypeIsHigherPriority(t *testing.T) {
	tests := []struct {
		ft       FlowType
		other    FlowType
		expected bool
	}{
		{Control, Data, true},
		{Control, Prefetch, true},
		{Data, Control, false},
		{Data, Prefetch, true},
		{Prefetch, Control, false},
		{Prefetch, Data, false},
		{Control, Control, false},
	}

	for _, tt := range tests {
		t.Run(tt.ft.String()+"_vs_"+tt.other.String(), func(t *testing.T) {
			if got := tt.ft.IsHigherPriority(tt.other); got != tt.expected {
				t.Errorf("IsHigherPriority(%s) = %v, want %v", tt.other, got, tt.expected)
			}
		})
	}
}

func TestFlowTypeFromFlags(t *testing.T) {
	tests := []struct {
		flags    uint8
		expected FlowType
	}{
		{0x00, Control},
		{0x01, Data},
		{0x02, Prefetch},
		{0x03, Reserved},
		{0x04, Control},       // Upper bits are masked out
		{0x05, Data},          // 0x05 = 0b101, lowest 2 bits = 0b01 = Data
		{0xFC, Control},       // 0xFC = 0b11111100, lowest 2 bits = 0b00 = Control
		{0xFF, Reserved},      // 0xFF = 0b11111111, lowest 2 bits = 0b11 = Reserved
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("flags_0x%02X", tt.flags), func(t *testing.T) {
			got := FlowTypeFromFlags(tt.flags)
			if got != tt.expected {
				t.Errorf("FlowTypeFromFlags(0x%02X) = %v, want %v", tt.flags, got, tt.expected)
			}
		})
	}
}

func TestFlowTypeToFlags(t *testing.T) {
	tests := []struct {
		flowType FlowType
		existing uint8
		expected uint8
		wantErr  bool
	}{
		{Control, 0x00, 0x00, false},
		{Data, 0x00, 0x01, false},
		{Prefetch, 0x00, 0x02, false},
		{Reserved, 0x00, 0x03, false},
		{Control, 0xFC, 0xFC, false},   // Preserve upper bits
		{Data, 0xFC, 0xFD, false},      // 0xFC | 0x01 with mask
		{Data, 0xFF, 0xFD, false},      // 0xFF & 0xFC | 0x01 = 0xFD
		{Prefetch, 0xAA, 0xAA, false},  // 0xAA = 0b10101010, set Prefetch (0x02 = 0b10)
	}

	for _, tt := range tests {
		t.Run(tt.flowType.String()+"_existing_0x"+fmt.Sprintf("%02X", tt.existing), func(t *testing.T) {
			got, err := FlowTypeToFlags(tt.flowType, tt.existing)

			if (err != nil) != tt.wantErr {
				t.Errorf("FlowTypeToFlags error = %v, wantErr %v", err, tt.wantErr)
			}

			if !tt.wantErr && got != tt.expected {
				t.Errorf("FlowTypeToFlags(%v, 0x%02X) = 0x%02X, want 0x%02X", tt.flowType, tt.existing, got, tt.expected)
			}
		})
	}
}

func TestFlowTypeToFlagsErrors(t *testing.T) {
	tests := []struct {
		name     string
		flowType FlowType
		wantErr  bool
	}{
		{"reserved", Reserved, false},
		{"valid control", Control, false},
		{"valid data", Data, false},
		{"valid prefetch", Prefetch, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := FlowTypeToFlags(tt.flowType, 0x00)
			if (err != nil) != tt.wantErr {
				t.Errorf("FlowTypeToFlags error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestFlagRoundtrip(t *testing.T) {
	tests := []struct {
		flowType FlowType
		existing uint8
	}{
		{Control, 0x00},
		{Data, 0x00},
		{Prefetch, 0x00},
		{Control, 0xFC},
		{Data, 0xFC},
		{Prefetch, 0xFC},
		{Control, 0xFF},
		{Data, 0xFF},
		{Prefetch, 0xFF},
	}

	for _, tt := range tests {
		t.Run(tt.flowType.String()+"_existing_0x"+fmt.Sprintf("%02X", tt.existing), func(t *testing.T) {
			// Encode
			flags, err := FlowTypeToFlags(tt.flowType, tt.existing)
			if err != nil {
				t.Fatalf("FlowTypeToFlags failed: %v", err)
			}

			// Decode
			decoded := FlowTypeFromFlags(flags)

			if decoded != tt.flowType {
				t.Errorf("roundtrip failed: encoded %v, decoded %v", tt.flowType, decoded)
			}
		})
	}
}

func TestFlowTypeValidate(t *testing.T) {
	tests := []struct {
		ft      FlowType
		wantErr bool
	}{
		{Control, false},
		{Data, false},
		{Prefetch, false},
		{Reserved, true},
	}

	for _, tt := range tests {
		t.Run(tt.ft.String(), func(t *testing.T) {
			err := tt.ft.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

