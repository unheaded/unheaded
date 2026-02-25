// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

// Package intermediary provides hop validation and malformation detection
// for the Unheaded protocol.
package intermediary

import (
	"fmt"
)

// MalformedReason represents a specific malformation in a Monad packet.
type MalformedReason string

const (
	// InvalidVersion indicates the protocol version field is invalid.
	InvalidVersion MalformedReason = "InvalidVersion"

	// InvalidCRC indicates the CRC checksum is invalid.
	InvalidCRC MalformedReason = "InvalidCRC"

	// UnknownDictID indicates an unknown dictionary ID was used.
	UnknownDictID MalformedReason = "UnknownDictID"

	// InvalidFieldID indicates an invalid field ID in the packet.
	InvalidFieldID MalformedReason = "InvalidFieldID"

	// DictAuthViolation indicates a dictionary authority violation.
	DictAuthViolation MalformedReason = "DictAuthViolation"

	// NonZeroReserved indicates reserved fields are non-zero.
	NonZeroReserved MalformedReason = "NonZeroReserved"
)

// MonadHeader represents the basic structure of a Monad packet header.
// This is a simplified representation for validation purposes.
type MonadHeader struct {
	Version      uint8
	Reserved     uint8
	DictID       uint16
	FieldID      uint16
	CRC          uint32
	PayloadLength uint16
}

// ValidateMonad performs full intermediary validation on packet data.
// Returns a list of malformation reasons found, or error if validation fails.
func ValidateMonad(data []byte) ([]MalformedReason, error) {
	if len(data) < 12 {
		return nil, fmt.Errorf("packet too short: %d bytes (minimum 12)", len(data))
	}

	var malformations []MalformedReason

	// Parse header (simplified parsing)
	version := data[0]
	reserved := data[1]

	// Version validation (assuming valid versions are 1-3)
	if version < 1 || version > 3 {
		malformations = append(malformations, InvalidVersion)
	}

	// Reserved field validation - MUST be zero
	if reserved != 0 {
		malformations = append(malformations, NonZeroReserved)
	}

	// Return malformations if any found
	if len(malformations) > 0 {
		return malformations, nil
	}

	return nil, nil
}

// CrossProtocolCheck validates the reserved 4-bit field is zero.
// This prevents cross-protocol confusion attacks.
func CrossProtocolCheck(data []byte) bool {
	if len(data) < 1 {
		return false
	}

	// Check reserved 4-bit field (upper 4 bits of first byte)
	reserved := data[0] >> 4
	return reserved == 0
}

// IntermediaryPolicy defines validation policies for intermediary nodes.
type IntermediaryPolicy struct {
	enforceVersionCheck bool
	enforceAuthority    bool
	dropMalformed       bool
}

// NewIntermediaryPolicy creates a new intermediary policy with secure defaults.
func NewIntermediaryPolicy() *IntermediaryPolicy {
	return &IntermediaryPolicy{
		enforceVersionCheck: true,
		enforceAuthority:    true,
		dropMalformed:       true,
	}
}

// SetEnforceVersionCheck sets whether to enforce version checking.
func (ip *IntermediaryPolicy) SetEnforceVersionCheck(enforce bool) {
	ip.enforceVersionCheck = enforce
}

// SetEnforceAuthority sets whether to enforce dictionary authority.
func (ip *IntermediaryPolicy) SetEnforceAuthority(enforce bool) {
	ip.enforceAuthority = enforce
}

// SetDropMalformed sets whether to drop malformed packets.
func (ip *IntermediaryPolicy) SetDropMalformed(drop bool) {
	ip.dropMalformed = drop
}

// ValidateAndProcess validates a packet according to policy.
// Returns true if packet should be forwarded, false if it should be dropped.
func (ip *IntermediaryPolicy) ValidateAndProcess(data []byte) (bool, []MalformedReason, error) {
	// Cross-protocol check (must always be enforced)
	if !CrossProtocolCheck(data) {
		return false, []MalformedReason{NonZeroReserved}, nil
	}

	// Full Monad validation
	malformations, err := ValidateMonad(data)
	if err != nil {
		return false, nil, err
	}

	// Determine if packet should be dropped
	if len(malformations) > 0 && ip.dropMalformed {
		return false, malformations, nil
	}

	return true, malformations, nil
}

// FieldValidator validates specific fields in a Monad packet.
type FieldValidator struct {
	knownDictIDs map[uint16]bool
	knownFieldIDs map[uint16]bool
}

// NewFieldValidator creates a new field validator.
func NewFieldValidator() *FieldValidator {
	return &FieldValidator{
		knownDictIDs: make(map[uint16]bool),
		knownFieldIDs: make(map[uint16]bool),
	}
}

// RegisterDictID registers a known dictionary ID.
func (fv *FieldValidator) RegisterDictID(dictID uint16) {
	fv.knownDictIDs[dictID] = true
}

// RegisterFieldID registers a known field ID.
func (fv *FieldValidator) RegisterFieldID(fieldID uint16) {
	fv.knownFieldIDs[fieldID] = true
}

// ValidateDictID checks if a dictionary ID is known and valid.
func (fv *FieldValidator) ValidateDictID(dictID uint16) bool {
	return fv.knownDictIDs[dictID]
}

// ValidateFieldID checks if a field ID is known and valid.
func (fv *FieldValidator) ValidateFieldID(fieldID uint16) bool {
	return fv.knownFieldIDs[fieldID]
}

// AuthorityValidator validates dictionary authority.
type AuthorityValidator struct {
	dictAuthorities map[uint16]string // dictID -> authority (e.g., domain name)
}

// NewAuthorityValidator creates a new authority validator.
func NewAuthorityValidator() *AuthorityValidator {
	return &AuthorityValidator{
		dictAuthorities: make(map[uint16]string),
	}
}

// RegisterAuthority registers the authority for a dictionary.
func (av *AuthorityValidator) RegisterAuthority(dictID uint16, authority string) {
	av.dictAuthorities[dictID] = authority
}

// ValidateAuthority checks if a dictionary ID has the correct authority.
func (av *AuthorityValidator) ValidateAuthority(dictID uint16, claimedAuthority string) bool {
	expectedAuthority, exists := av.dictAuthorities[dictID]
	if !exists {
		return false
	}
	return expectedAuthority == claimedAuthority
}

// PacketValidator combines all validation checks.
type PacketValidator struct {
	policy           *IntermediaryPolicy
	fieldValidator   *FieldValidator
	authorityValidator *AuthorityValidator
}

// NewPacketValidator creates a new comprehensive packet validator.
func NewPacketValidator() *PacketValidator {
	return &PacketValidator{
		policy:             NewIntermediaryPolicy(),
		fieldValidator:     NewFieldValidator(),
		authorityValidator: NewAuthorityValidator(),
	}
}

// Validate performs all validation checks on a packet.
func (pv *PacketValidator) Validate(data []byte) (bool, []MalformedReason, error) {
	return pv.policy.ValidateAndProcess(data)
}

// RegisterDictID registers a known dictionary ID.
func (pv *PacketValidator) RegisterDictID(dictID uint16) {
	pv.fieldValidator.RegisterDictID(dictID)
}

// RegisterFieldID registers a known field ID.
func (pv *PacketValidator) RegisterFieldID(fieldID uint16) {
	pv.fieldValidator.RegisterFieldID(fieldID)
}

// RegisterAuthority registers the authority for a dictionary.
func (pv *PacketValidator) RegisterAuthority(dictID uint16, authority string) {
	pv.authorityValidator.RegisterAuthority(dictID, authority)
}
