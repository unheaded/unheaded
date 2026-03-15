// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package intermediary

import (
	"testing"
)

func TestMalformedReasonValues(t *testing.T) {
	reasons := []MalformedReason{
		InvalidVersion,
		InvalidCRC,
		UnknownDictID,
		InvalidFieldID,
		DictAuthViolation,
		NonZeroReserved,
	}

	for _, reason := range reasons {
		if reason == "" {
			t.Errorf("Malformed reason should not be empty")
		}
	}
}

func TestValidateMonadTooShort(t *testing.T) {
	data := []byte{0x01, 0x02}

	_, err := ValidateMonad(data)
	if err == nil {
		t.Errorf("Expected error for short packet")
	}
}

func TestValidateMonadValidVersion(t *testing.T) {
	// Construct a minimal valid packet
	data := make([]byte, 12)
	data[0] = 0x01      // valid version
	data[1] = 0x00      // reserved = 0 (valid)

	malformations, err := ValidateMonad(data)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if len(malformations) != 0 {
		t.Errorf("Expected no malformations, got %d", len(malformations))
	}
}

func TestValidateMonadInvalidVersion(t *testing.T) {
	data := make([]byte, 12)
	data[0] = 0xFF      // invalid version (> 3)
	data[1] = 0x00      // reserved = 0

	malformations, err := ValidateMonad(data)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if len(malformations) != 1 {
		t.Errorf("Expected 1 malformation, got %d", len(malformations))
	}

	if malformations[0] != InvalidVersion {
		t.Errorf("Expected InvalidVersion, got %s", malformations[0])
	}
}

func TestValidateMonadNonZeroReserved(t *testing.T) {
	data := make([]byte, 12)
	data[0] = 0x01      // valid version
	data[1] = 0xFF      // reserved != 0 (invalid)

	malformations, err := ValidateMonad(data)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if len(malformations) != 1 {
		t.Errorf("Expected 1 malformation, got %d", len(malformations))
	}

	if malformations[0] != NonZeroReserved {
		t.Errorf("Expected NonZeroReserved, got %s", malformations[0])
	}
}

func TestValidateMonadMultipleMalformations(t *testing.T) {
	data := make([]byte, 12)
	data[0] = 0xFF      // invalid version
	data[1] = 0xFF      // reserved != 0

	malformations, err := ValidateMonad(data)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if len(malformations) != 2 {
		t.Errorf("Expected 2 malformations, got %d", len(malformations))
	}
}

func TestCrossProtocolCheckValid(t *testing.T) {
	// Reserved 4-bit field is upper 4 bits = 0x0X where X is lower 4 bits
	data := []byte{0x05, 0x01, 0x02} // 0000 0101 - upper 4 bits are 0

	if !CrossProtocolCheck(data) {
		t.Errorf("Expected cross-protocol check to pass")
	}
}

func TestCrossProtocolCheckInvalid(t *testing.T) {
	// Upper 4 bits non-zero
	data := []byte{0xF5, 0x01, 0x02} // 1111 0101 - upper 4 bits are 1111

	if CrossProtocolCheck(data) {
		t.Errorf("Expected cross-protocol check to fail")
	}
}

func TestCrossProtocolCheckEmpty(t *testing.T) {
	data := []byte{}

	if CrossProtocolCheck(data) {
		t.Errorf("Expected cross-protocol check to fail for empty data")
	}
}

func TestIntermediaryPolicyDefaults(t *testing.T) {
	policy := NewIntermediaryPolicy()

	if !policy.enforceVersionCheck {
		t.Errorf("Expected enforceVersionCheck to be true by default")
	}

	if !policy.enforceAuthority {
		t.Errorf("Expected enforceAuthority to be true by default")
	}

	if !policy.dropMalformed {
		t.Errorf("Expected dropMalformed to be true by default")
	}
}

func TestIntermediaryPolicyValidateAndProcess(t *testing.T) {
	policy := NewIntermediaryPolicy()

	// Valid packet
	data := make([]byte, 12)
	data[0] = 0x01      // valid version (0000 0001 - upper 4 bits are 0)
	data[1] = 0x00      // reserved = 0

	shouldForward, malformations, err := policy.ValidateAndProcess(data)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if !shouldForward {
		t.Errorf("Expected packet to be forwarded")
	}

	if len(malformations) != 0 {
		t.Errorf("Expected no malformations")
	}
}

func TestIntermediaryPolicyCrossProtocolFail(t *testing.T) {
	policy := NewIntermediaryPolicy()

	// Invalid cross-protocol check
	data := make([]byte, 12)
	data[0] = 0xF1      // upper 4 bits non-zero
	data[1] = 0x00

	shouldForward, malformations, _ := policy.ValidateAndProcess(data)
	if shouldForward {
		t.Errorf("Expected packet to be dropped due to cross-protocol violation")
	}

	if len(malformations) != 1 || malformations[0] != NonZeroReserved {
		t.Errorf("Expected NonZeroReserved malformation")
	}
}

func TestFieldValidatorRegisterAndValidate(t *testing.T) {
	validator := NewFieldValidator()

	dictID := uint16(0x1234)
	validator.RegisterDictID(dictID)

	if !validator.ValidateDictID(dictID) {
		t.Errorf("Expected dictID to be valid after registration")
	}

	if validator.ValidateDictID(0x5678) {
		t.Errorf("Expected unregistered dictID to be invalid")
	}
}

func TestFieldValidatorFieldID(t *testing.T) {
	validator := NewFieldValidator()

	fieldID := uint16(0x0001)
	validator.RegisterFieldID(fieldID)

	if !validator.ValidateFieldID(fieldID) {
		t.Errorf("Expected fieldID to be valid after registration")
	}

	if validator.ValidateFieldID(0x0002) {
		t.Errorf("Expected unregistered fieldID to be invalid")
	}
}

func TestAuthorityValidatorRegisterAndValidate(t *testing.T) {
	validator := NewAuthorityValidator()

	dictID := uint16(0x1234)
	authority := "example.com"

	validator.RegisterAuthority(dictID, authority)

	if !validator.ValidateAuthority(dictID, authority) {
		t.Errorf("Expected authority to be valid")
	}

	if validator.ValidateAuthority(dictID, "wrong.com") {
		t.Errorf("Expected wrong authority to be invalid")
	}

	if validator.ValidateAuthority(0x5678, authority) {
		t.Errorf("Expected unregistered dictID to be invalid")
	}
}

func TestPacketValidatorValidate(t *testing.T) {
	validator := NewPacketValidator()

	// Valid packet
	data := make([]byte, 12)
	data[0] = 0x01      // valid version and cross-protocol check
	data[1] = 0x00      // reserved = 0

	shouldForward, malformations, err := validator.Validate(data)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if !shouldForward {
		t.Errorf("Expected packet to be forwarded")
	}

	if len(malformations) != 0 {
		t.Errorf("Expected no malformations")
	}
}

func TestPacketValidatorRegisterDictID(t *testing.T) {
	validator := NewPacketValidator()

	dictID := uint16(0x1234)
	validator.RegisterDictID(dictID)

	if !validator.fieldValidator.ValidateDictID(dictID) {
		t.Errorf("Expected dictID to be registered")
	}
}

func TestPacketValidatorRegisterFieldID(t *testing.T) {
	validator := NewPacketValidator()

	fieldID := uint16(0x0001)
	validator.RegisterFieldID(fieldID)

	if !validator.fieldValidator.ValidateFieldID(fieldID) {
		t.Errorf("Expected fieldID to be registered")
	}
}

func TestPacketValidatorRegisterAuthority(t *testing.T) {
	validator := NewPacketValidator()

	dictID := uint16(0x1234)
	authority := "example.com"

	validator.RegisterAuthority(dictID, authority)

	if !validator.authorityValidator.ValidateAuthority(dictID, authority) {
		t.Errorf("Expected authority to be registered")
	}
}

func TestValidateMonadAllVersions(t *testing.T) {
	validVersions := []uint8{0x01, 0x02, 0x03}

	for _, version := range validVersions {
		data := make([]byte, 12)
		data[0] = version
		data[1] = 0x00

		malformations, err := ValidateMonad(data)
		if err != nil {
			t.Errorf("Unexpected error for version %d: %v", version, err)
		}

		if len(malformations) != 0 {
			t.Errorf("Expected no malformations for version %d", version)
		}
	}
}

func TestCrossProtocolCheckBoundary(t *testing.T) {
	testCases := []struct {
		value    byte
		expected bool
		name     string
	}{
		{0x00, true, "all zeros"},
		{0x0F, true, "lower 4 bits only"},
		{0x10, false, "bit 4 set"},
		{0x80, false, "bit 7 set"},
		{0xFF, false, "all bits set"},
	}

	for _, tc := range testCases {
		data := []byte{tc.value}
		result := CrossProtocolCheck(data)
		if result != tc.expected {
			t.Errorf("%s (0x%02x): expected %v, got %v", tc.name, tc.value, tc.expected, result)
		}
	}
}
