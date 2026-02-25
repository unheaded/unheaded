// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package settings

import (
	"testing"
)

func TestSettingIDString(t *testing.T) {
	tests := []struct {
		id       SettingID
		expected string
	}{
		{SettingsMaxDictionarySize, "SETTINGS_MAX_DICTIONARY_SIZE"},
		{SettingsMaxFlowCount, "SETTINGS_MAX_FLOW_COUNT"},
		{SettingsMaxMonadSize, "SETTINGS_MAX_MONAD_SIZE"},
		{SettingsSupportedExtensions, "SETTINGS_SUPPORTED_EXTENSIONS"},
		{SettingsSophiaCompression, "SETTINGS_SOPHIA_COMPRESSION"},
		{SettingsRingBufferSize, "SETTINGS_RING_BUFFER_SIZE"},
		{SettingsWotanBufferSize, "SETTINGS_WOTAN_BUFFER_SIZE"},
		{SettingID(999), "UNKNOWN_SETTING(999)"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.id.String(); got != tt.expected {
				t.Errorf("SettingID.String() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestSettingsNew(t *testing.T) {
	tests := []struct {
		name           string
		unknownIgnored bool
		values         map[SettingID]uint64
		expectedCount  int
	}{
		{
			name:           "empty",
			unknownIgnored: false,
			expectedCount:  0,
		},
		{
			name:           "with values",
			unknownIgnored: true,
			values:         map[SettingID]uint64{SettingsMaxDictionarySize: 1024},
			expectedCount:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s *Settings
			if tt.values != nil {
				s = New(tt.unknownIgnored, tt.values)
			} else {
				s = New(tt.unknownIgnored)
			}

			if s.UnknownSettingsIgnored != tt.unknownIgnored {
				t.Errorf("UnknownSettingsIgnored = %v, want %v", s.UnknownSettingsIgnored, tt.unknownIgnored)
			}
			if len(s.All()) != tt.expectedCount {
				t.Errorf("All() count = %d, want %d", len(s.All()), tt.expectedCount)
			}
		})
	}
}

func TestSettingsSetGet(t *testing.T) {
	s := New(false)

	err := s.Set(SettingsMaxDictionarySize, 2048)
	if err != nil {
		t.Fatalf("Set failed: %v", err)
	}

	val, ok := s.Get(SettingsMaxDictionarySize)
	if !ok || val != 2048 {
		t.Errorf("Get returned (%d, %v), want (2048, true)", val, ok)
	}

	_, ok = s.Get(SettingsMaxFlowCount)
	if ok {
		t.Errorf("Get for non-existent setting returned ok=true, want false")
	}
}

func TestSettingsEncodeDecodeRoundtrip(t *testing.T) {
	tests := []struct {
		name   string
		values map[SettingID]uint64
	}{
		{
			name:   "empty",
			values: map[SettingID]uint64{},
		},
		{
			name: "single setting",
			values: map[SettingID]uint64{
				SettingsMaxDictionarySize: 4096,
			},
		},
		{
			name: "multiple settings",
			values: map[SettingID]uint64{
				SettingsMaxDictionarySize:   8192,
				SettingsMaxFlowCount:        256,
				SettingsMaxMonadSize:        65536,
				SettingsSophiaCompression:   1,
				SettingsRingBufferSize:      1024,
				SettingsWotanBufferSize:     512,
				SettingsSupportedExtensions: 0x0F,
			},
		},
		{
			name: "large values",
			values: map[SettingID]uint64{
				SettingsMaxDictionarySize: 0x3FFFFFFFFFFFFFFF, // MaxVarint (2^62-1)
				SettingsMaxFlowCount:      0x0000000000000001,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Encode
			original := New(false, tt.values)
			encoded, err := original.Encode()
			if err != nil {
				t.Fatalf("Encode failed: %v", err)
			}

			// Decode
			decoded := New(false)
			err = decoded.Decode(encoded)
			if err != nil {
				t.Fatalf("Decode failed: %v", err)
			}

			// Verify
			originalAll := original.All()
			decodedAll := decoded.All()

			if len(originalAll) != len(decodedAll) {
				t.Errorf("decoded settings count %d, want %d", len(decodedAll), len(originalAll))
			}

			for id, expectedVal := range originalAll {
				gotVal, ok := decoded.Get(id)
				if !ok {
					t.Errorf("decoded missing setting %s", id)
					continue
				}
				if gotVal != expectedVal {
					t.Errorf("setting %s: got %d, want %d", id, gotVal, expectedVal)
				}
			}
		})
	}
}

func TestSettingsDecodeErrors(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		ignored bool
		wantErr bool
	}{
		{
			name:    "empty data",
			data:    []byte{},
			wantErr: false,
		},
		{
			name:    "incomplete setting",
			data:    []byte{0x01}, // ID without value
			wantErr: true,
		},
		{
			name:    "unknown setting not ignored",
			data:    []byte{0x20, 0x01}, // ID=0x20 (32, unknown), Value=1
			ignored: false,
			wantErr: true,
		},
		{
			name:    "unknown setting ignored",
			data:    []byte{0x20, 0x01}, // ID=0x20 (32, unknown), Value=1
			ignored: true,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := New(tt.ignored)
			err := s.Decode(tt.data)

			if (err != nil) != tt.wantErr {
				t.Errorf("Decode error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNegotiate(t *testing.T) {
	tests := []struct {
		name           string
		local          map[SettingID]uint64
		remote         map[SettingID]uint64
		expectedResult map[SettingID]uint64
	}{
		{
			name:           "empty settings",
			local:          map[SettingID]uint64{},
			remote:         map[SettingID]uint64{},
			expectedResult: map[SettingID]uint64{},
		},
		{
			name: "limit: take minimum",
			local: map[SettingID]uint64{
				SettingsMaxDictionarySize: 1024,
			},
			remote: map[SettingID]uint64{
				SettingsMaxDictionarySize: 2048,
			},
			expectedResult: map[SettingID]uint64{
				SettingsMaxDictionarySize: 1024,
			},
		},
		{
			name: "extensions: intersection",
			local: map[SettingID]uint64{
				SettingsSupportedExtensions: 0x0F, // 1111 in binary
			},
			remote: map[SettingID]uint64{
				SettingsSupportedExtensions: 0x0C, // 1100 in binary
			},
			expectedResult: map[SettingID]uint64{
				SettingsSupportedExtensions: 0x0C, // 1100 in binary (AND result)
			},
		},
		{
			name: "mixed settings",
			local: map[SettingID]uint64{
				SettingsMaxDictionarySize:   8192,
				SettingsMaxFlowCount:        512,
				SettingsSupportedExtensions: 0xFF,
			},
			remote: map[SettingID]uint64{
				SettingsMaxDictionarySize:   4096,
				SettingsMaxFlowCount:        256,
				SettingsSupportedExtensions: 0x0F,
			},
			expectedResult: map[SettingID]uint64{
				SettingsMaxDictionarySize:   4096,
				SettingsMaxFlowCount:        256,
				SettingsSupportedExtensions: 0x0F,
			},
		},
		{
			name: "only local has setting",
			local: map[SettingID]uint64{
				SettingsMaxDictionarySize: 1024,
				SettingsMaxFlowCount:      512,
			},
			remote: map[SettingID]uint64{
				SettingsMaxDictionarySize: 2048,
			},
			expectedResult: map[SettingID]uint64{
				SettingsMaxDictionarySize: 1024,
				SettingsMaxFlowCount:      512,
			},
		},
		{
			name: "only remote has setting",
			local: map[SettingID]uint64{
				SettingsMaxDictionarySize: 2048,
			},
			remote: map[SettingID]uint64{
				SettingsMaxDictionarySize: 1024,
				SettingsMaxFlowCount:      256,
			},
			expectedResult: map[SettingID]uint64{
				SettingsMaxDictionarySize: 1024,
				SettingsMaxFlowCount:      256,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			local := New(false, tt.local)
			remote := New(false, tt.remote)

			result, err := Negotiate(local, remote)
			if err != nil {
				t.Fatalf("Negotiate failed: %v", err)
			}

			resultAll := result.All()
			if len(resultAll) != len(tt.expectedResult) {
				t.Errorf("result count = %d, want %d", len(resultAll), len(tt.expectedResult))
			}

			for id, expectedVal := range tt.expectedResult {
				gotVal, ok := result.Get(id)
				if !ok {
					t.Errorf("result missing setting %s", id)
					continue
				}
				if gotVal != expectedVal {
					t.Errorf("setting %s: got %d, want %d", id, gotVal, expectedVal)
				}
			}
		})
	}
}

func TestNegotiateErrors(t *testing.T) {
	tests := []struct {
		name    string
		local   *Settings
		remote  *Settings
		wantErr bool
	}{
		{
			name:    "nil local",
			local:   nil,
			remote:  New(false),
			wantErr: true,
		},
		{
			name:    "nil remote",
			local:   New(false),
			remote:  nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Negotiate(tt.local, tt.remote)
			if (err != nil) != tt.wantErr {
				t.Errorf("Negotiate error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
