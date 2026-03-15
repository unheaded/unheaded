// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Steven Bellis. All rights reserved.

package pqc

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
	"sync"
	"time"

	"golang.org/x/crypto/hkdf"
)

// TunnelKeySet holds the derived keys for a KEM tunnel.
type TunnelKeySet struct {
	SigningKey [32]byte // For authenticating tunnel messages
	AuthKey   [32]byte // For HMAC authentication
	IVKey     [16]byte // For initialization vectors
}

// Zero securely wipes all key material.
func (ks *TunnelKeySet) Zero() {
	for i := range ks.SigningKey {
		ks.SigningKey[i] = 0
	}
	for i := range ks.AuthKey {
		ks.AuthKey[i] = 0
	}
	for i := range ks.IVKey {
		ks.IVKey[i] = 0
	}
}

// DeriveSigningKey derives a 32-byte signing key from a shared secret using
// HKDF-SHA256. The derivation binds the key to the flow ID and epoch to
// prevent cross-flow key reuse.
//
// Parameters:
//   - sharedSecret: The KEM shared secret (32 bytes)
//   - salt: Optional salt for HKDF (can be nil)
//   - flowID: Flow identifier for domain separation
//   - epoch: Epoch counter for key freshness
func DeriveSigningKey(sharedSecret [32]byte, salt []byte, flowID []byte, epoch uint32) ([32]byte, error) {
	// Build HKDF info: "unheaded-pqc-signing" || flowID || epoch
	info := buildInfo("unheaded-pqc-signing", flowID, epoch)

	reader := hkdf.New(sha256.New, sharedSecret[:], salt, info)

	var key [32]byte
	if _, err := io.ReadFull(reader, key[:]); err != nil {
		return [32]byte{}, fmt.Errorf("pqc: HKDF derive signing key: %w", err)
	}
	return key, nil
}

// DeriveTunnelKeys derives a complete TunnelKeySet from a shared secret.
// Three keys are derived: signing, authentication, and IV initialization.
//
// Parameters:
//   - sharedSecret: The KEM shared secret (32 bytes)
//   - salt: Optional salt for HKDF (can be nil)
//   - context: Application-specific context string
func DeriveTunnelKeys(sharedSecret [32]byte, salt []byte, context string) (*TunnelKeySet, error) {
	info := []byte("unheaded-pqc-tunnel-" + context)
	reader := hkdf.New(sha256.New, sharedSecret[:], salt, info)

	ks := &TunnelKeySet{}

	// Derive signing key (32 bytes).
	if _, err := io.ReadFull(reader, ks.SigningKey[:]); err != nil {
		return nil, fmt.Errorf("pqc: HKDF derive signing key: %w", err)
	}

	// Derive auth key (32 bytes).
	if _, err := io.ReadFull(reader, ks.AuthKey[:]); err != nil {
		return nil, fmt.Errorf("pqc: HKDF derive auth key: %w", err)
	}

	// Derive IV key (16 bytes).
	if _, err := io.ReadFull(reader, ks.IVKey[:]); err != nil {
		return nil, fmt.Errorf("pqc: HKDF derive IV key: %w", err)
	}

	return ks, nil
}

// TunnelNegotiator handles the KEM tunnel handshake. The initiator
// encapsulates against the responder's public key; the responder
// decapsulates to recover the shared secret. Both sides then derive
// tunnel keys via HKDF.
type TunnelNegotiator struct {
	mu       sync.Mutex
	params   ParameterSet
	encap    *MLKEMEncapsulator
	decap    *MLKEMDecapsulator
}

// NewTunnelNegotiator creates a negotiator for the given ML-KEM parameter set.
func NewTunnelNegotiator(params ParameterSet) (*TunnelNegotiator, error) {
	encap, err := NewMLKEMEncapsulator(params)
	if err != nil {
		return nil, fmt.Errorf("pqc: tunnel negotiator encapsulator: %w", err)
	}
	decap, err := NewMLKEMDecapsulator(params)
	if err != nil {
		return nil, fmt.Errorf("pqc: tunnel negotiator decapsulator: %w", err)
	}
	return &TunnelNegotiator{
		params: params,
		encap:  encap,
		decap:  decap,
	}, nil
}

// HandshakeResult contains the output of a KEM handshake step.
type HandshakeResult struct {
	Ciphertext   []byte
	SharedSecret [32]byte
	Keys         *TunnelKeySet
}

// Zero securely wipes the handshake result.
func (hr *HandshakeResult) Zero() {
	for i := range hr.SharedSecret {
		hr.SharedSecret[i] = 0
	}
	if hr.Keys != nil {
		hr.Keys.Zero()
	}
}

// InitiatorHandshake performs the initiator side of the KEM handshake.
// It encapsulates against the responder's public key, producing a
// ciphertext to send and a shared secret to derive keys from.
func (tn *TunnelNegotiator) InitiatorHandshake(responderPubKey []byte, salt []byte, context string) (*HandshakeResult, error) {
	tn.mu.Lock()
	defer tn.mu.Unlock()

	ct, ss, err := tn.encap.Encapsulate(responderPubKey)
	if err != nil {
		return nil, fmt.Errorf("pqc: initiator encapsulate: %w", err)
	}

	var ss32 [32]byte
	copy(ss32[:], ss)

	keys, err := DeriveTunnelKeys(ss32, salt, context)
	if err != nil {
		return nil, fmt.Errorf("pqc: initiator derive keys: %w", err)
	}

	return &HandshakeResult{
		Ciphertext:   ct,
		SharedSecret: ss32,
		Keys:         keys,
	}, nil
}

// ResponderHandshake performs the responder side of the KEM handshake.
// It decapsulates the ciphertext using the secret key to recover the
// shared secret and derive tunnel keys.
func (tn *TunnelNegotiator) ResponderHandshake(ciphertext, secretKey, salt []byte, context string) (*HandshakeResult, error) {
	tn.mu.Lock()
	defer tn.mu.Unlock()

	ss, err := tn.decap.Decapsulate(ciphertext, secretKey)
	if err != nil {
		return nil, fmt.Errorf("pqc: responder decapsulate: %w", err)
	}

	var ss32 [32]byte
	copy(ss32[:], ss)

	keys, err := DeriveTunnelKeys(ss32, salt, context)
	if err != nil {
		return nil, fmt.Errorf("pqc: responder derive keys: %w", err)
	}

	return &HandshakeResult{
		Ciphertext:   ciphertext,
		SharedSecret: ss32,
		Keys:         keys,
	}, nil
}

// KEMTunnelRateLimiter limits the rate of KEM tunnel setup requests
// to prevent denial-of-service via expensive KEM operations.
type KEMTunnelRateLimiter struct {
	mu          sync.Mutex
	count       int64
	maxRate     int64
	windowStart time.Time
}

// NewKEMTunnelRateLimiter creates a rate limiter for KEM tunnel setup.
// maxRate specifies the maximum number of handshakes allowed per second.
func NewKEMTunnelRateLimiter(maxRate int64) *KEMTunnelRateLimiter {
	return &KEMTunnelRateLimiter{
		maxRate:     maxRate,
		windowStart: time.Now(),
	}
}

// Allow checks if a KEM handshake is permitted under the rate limit.
func (rl *KEMTunnelRateLimiter) Allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	if now.Sub(rl.windowStart) >= time.Second {
		rl.count = 0
		rl.windowStart = now
	}

	if rl.count >= rl.maxRate {
		return false
	}

	rl.count++
	return true
}

// Count returns the number of handshakes in the current window.
func (rl *KEMTunnelRateLimiter) Count() int64 {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	return rl.count
}

// buildInfo constructs the HKDF info parameter with domain separation.
func buildInfo(label string, flowID []byte, epoch uint32) []byte {
	var epochBytes [4]byte
	binary.BigEndian.PutUint32(epochBytes[:], epoch)

	info := make([]byte, 0, len(label)+len(flowID)+4)
	info = append(info, []byte(label)...)
	info = append(info, flowID...)
	info = append(info, epochBytes[:]...)
	return info
}
