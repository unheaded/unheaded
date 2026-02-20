// Package settings provides capability negotiation for the Unheaded protocol.
// It implements H4 finding: Settings negotiation using varint key-value pairs.
// Uses RFC 9000 variable-length integers via pkg/protocol/encoding.
package settings

import (
	"bytes"
	"fmt"

	"unheaded/pkg/logger"
	"unheaded/pkg/protocol/encoding"
)

// SettingID represents a unique setting identifier.
type SettingID uint16

// Defined settings identifiers.
const (
	SettingsMaxDictionarySize SettingID = 0x01
	SettingsMaxFlowCount      SettingID = 0x02
	SettingsMaxMonadSize      SettingID = 0x03
	SettingsSupportedExtensions SettingID = 0x04
	SettingsSophiaCompression SettingID = 0x05
	SettingsRingBufferSize    SettingID = 0x06
	SettingsWotanBufferSize   SettingID = 0x07
)

// String returns the string representation of a SettingID.
func (s SettingID) String() string {
	switch s {
	case SettingsMaxDictionarySize:
		return "SETTINGS_MAX_DICTIONARY_SIZE"
	case SettingsMaxFlowCount:
		return "SETTINGS_MAX_FLOW_COUNT"
	case SettingsMaxMonadSize:
		return "SETTINGS_MAX_MONAD_SIZE"
	case SettingsSupportedExtensions:
		return "SETTINGS_SUPPORTED_EXTENSIONS"
	case SettingsSophiaCompression:
		return "SETTINGS_SOPHIA_COMPRESSION"
	case SettingsRingBufferSize:
		return "SETTINGS_RING_BUFFER_SIZE"
	case SettingsWotanBufferSize:
		return "SETTINGS_WOTAN_BUFFER_SIZE"
	default:
		return fmt.Sprintf("UNKNOWN_SETTING(%d)", s)
	}
}

// Settings represents a collection of protocol settings.
type Settings struct {
	values                   map[SettingID]uint64
	UnknownSettingsIgnored   bool
}

// New creates a new Settings instance with the given key-value pairs.
func New(unknownIgnored bool, values ...map[SettingID]uint64) *Settings {
	s := &Settings{
		values:                 make(map[SettingID]uint64),
		UnknownSettingsIgnored: unknownIgnored,
	}
	if len(values) > 0 {
		for k, v := range values[0] {
			s.values[k] = v
		}
	}
	return s
}

// Set sets a setting value.
func (s *Settings) Set(id SettingID, value uint64) error {
	if s == nil {
		return fmt.Errorf("settings: nil Settings instance")
	}
	s.values[id] = value
	return nil
}

// Get retrieves a setting value. Returns 0 and false if not set.
func (s *Settings) Get(id SettingID) (uint64, bool) {
	if s == nil {
		return 0, false
	}
	v, ok := s.values[id]
	return v, ok
}

// All returns a copy of all settings values.
func (s *Settings) All() map[SettingID]uint64 {
	if s == nil {
		return make(map[SettingID]uint64)
	}
	result := make(map[SettingID]uint64)
	for k, v := range s.values {
		result[k] = v
	}
	return result
}

// encodeVarint encodes a uint64 as a QUIC variable-length integer using pkg/protocol/encoding.
func encodeVarint(v uint64) []byte {
	encoded, err := encoding.EncodeVarint(v)
	if err != nil {
		// For backward compatibility with existing code, fallback gracefully
		// (should never happen unless value exceeds MaxVarint)
		return []byte{}
	}
	return encoded
}

// decodeVarint decodes a QUIC variable-length integer from a buffer and returns the value and bytes consumed.
// Uses pkg/protocol/encoding.DecodeVarint per RFC 9000 §16.
func decodeVarint(buf []byte) (uint64, int, error) {
	if len(buf) == 0 {
		return 0, 0, fmt.Errorf("settings: empty buffer for varint decode")
	}
	v, n, err := encoding.DecodeVarint(buf)
	if err != nil {
		return 0, 0, fmt.Errorf("settings: invalid varint encoding: %w", err)
	}
	return v, n, nil
}

// Encode encodes the settings as varint key-value pairs.
// Format: [key_id(varint), value(varint), ...] repeated.
func (s *Settings) Encode() ([]byte, error) {
	if s == nil {
		return nil, fmt.Errorf("settings: nil Settings instance")
	}

	var buf bytes.Buffer

	for id, value := range s.values {
		// Encode setting ID as varint
		idBytes := encodeVarint(uint64(id))
		if _, err := buf.Write(idBytes); err != nil {
			return nil, fmt.Errorf("settings: failed to encode setting ID: %w", err)
		}

		// Encode value as varint
		valueBytes := encodeVarint(value)
		if _, err := buf.Write(valueBytes); err != nil {
			return nil, fmt.Errorf("settings: failed to encode value: %w", err)
		}
	}

	return buf.Bytes(), nil
}

// Decode decodes varint key-value pairs into settings.
func (s *Settings) Decode(data []byte) error {
	if s == nil {
		return fmt.Errorf("settings: nil Settings instance")
	}

	if len(data) == 0 {
		return nil // Empty is valid
	}

	offset := 0
	for offset < len(data) {
		// Decode setting ID
		id, n, err := decodeVarint(data[offset:])
		if err != nil {
			return fmt.Errorf("settings: failed to decode setting ID at offset %d: %w", offset, err)
		}
		offset += n

		if offset >= len(data) {
			return fmt.Errorf("settings: incomplete setting: ID without value")
		}

		// Decode value
		value, n, err := decodeVarint(data[offset:])
		if err != nil {
			return fmt.Errorf("settings: failed to decode value at offset %d: %w", offset, err)
		}
		offset += n

		settingID := SettingID(id)

		// Check if this is an unknown setting
		if settingID > SettingsWotanBufferSize && !s.UnknownSettingsIgnored {
			return fmt.Errorf("settings: unknown setting ID %d", settingID)
		}

		// Store the setting
		s.values[settingID] = value
	}

	return nil
}

// Negotiate computes the intersection of local and remote settings.
// For limit settings (numeric values), uses the minimum.
// For extension settings, computes the intersection.
func Negotiate(local, remote *Settings) (*Settings, error) {
	if local == nil {
		return nil, fmt.Errorf("settings: local settings cannot be nil")
	}
	if remote == nil {
		return nil, fmt.Errorf("settings: remote settings cannot be nil")
	}

	result := New(local.UnknownSettingsIgnored)

	// Iterate over all settings from both local and remote
	allIDs := make(map[SettingID]bool)
	for id := range local.values {
		allIDs[id] = true
	}
	for id := range remote.values {
		allIDs[id] = true
	}

	for id := range allIDs {
		localVal, localOk := local.Get(id)
		remoteVal, remoteOk := remote.Get(id)

		if !localOk && !remoteOk {
			continue
		}

		switch id {
		case SettingsSupportedExtensions:
			// For extensions, compute intersection (bitwise AND)
			if localOk && remoteOk {
				result.Set(id, localVal&remoteVal)
			} else if localOk {
				result.Set(id, localVal)
			} else {
				result.Set(id, remoteVal)
			}
		default:
			// For limit settings, use minimum (both must be present to negotiate)
			if localOk && remoteOk {
				if localVal < remoteVal {
					result.Set(id, localVal)
				} else {
					result.Set(id, remoteVal)
				}
			} else if localOk {
				result.Set(id, localVal)
			} else {
				result.Set(id, remoteVal)
			}
		}
	}

	logger.Debug().Msgf("negotiated settings: local=%v remote=%v result=%v",
		local.All(), remote.All(), result.All())

	return result, nil
}
