// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

// Package wotansig signs and verifies Wotan topic messages with ML-DSA-65
// (ADR-043 hard condition #2, ADR-105 signed drift events).
//
// It is the importable half of services/wotan/internal/signing: publishers
// (wotan-client, Heimdall) sign with TopicSigner, and any subscriber can
// re-verify a delivered event with VerifyEvent against the public keys it
// trusts. Wotan's own replay window and clock-skew check stay in
// services/wotan/internal/signing; a subscriber checking an event later has
// no use for either, since the signature proves who sent the message
// whenever it is checked.
//
// What a signature covers (canonical form "wotan/topic-sig/v2", unchanged
// from before the move and pinned by a golden vector):
//
//	SHA-256("wotan/topic-sig/v2" || 0 || topic || 0 || sender || 0 ||
//	        signed_at (int64, big-endian, Unix ms) || payload)
//
// The sender is the publish request's sender_id or, when the publisher sent
// none, the display_name Wotan authorized it under ("service" when it gave
// none). Wotan delivers it as TopicEvent.sender_id on signed messages, with
// signature, public_key, algorithm and signed_at_unix_ms beside it.
package wotansig

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"slices"
	"time"

	"unheaded/pkg/crypto/pqc"
)

// Algorithm is the only signature algorithm accepted (no agility).
const Algorithm = "ml-dsa-65"

// Domain is the canonical form's domain-separation prefix.
const Domain = "wotan/topic-sig/v2"

// The error texts are the ones services/wotan/internal/signing always used:
// Wotan returns them to publishers.
var (
	ErrInvalidSignature = errors.New("signing: signature verification failed")
	ErrWrongAlgorithm   = errors.New("signing: only ml-dsa-65 accepted (no algorithm agility)")
	ErrNoTrustedKeys    = errors.New("signing: no trusted signing keys configured (topics.signing_keys)")
	ErrUntrustedKey     = errors.New("signing: public key is not a trusted signing key")
	// ErrNoSigner refuses to sign without a key.
	ErrNoSigner = errors.New("signing: no signing key")
)

// Signed is a topic message's signature and what a verifier needs with it.
type Signed struct {
	Signature      []byte
	PublicKey      []byte
	Algorithm      string
	SignedAtUnixMs int64
}

// Canonicalize returns the digest a topic signature covers (see the package
// doc). The fields are NUL-separated and the time is fixed-width, so no two
// distinct (topic, sender, time, payload) tuples whose topic and sender hold
// no NUL share a digest.
func Canonicalize(topic, senderID string, signedAtMs int64, payload []byte) []byte {
	h := sha256.New()
	h.Write([]byte(Domain))
	h.Write([]byte{0})
	h.Write([]byte(topic))
	h.Write([]byte{0})
	h.Write([]byte(senderID))
	h.Write([]byte{0})
	_ = binary.Write(h, binary.BigEndian, signedAtMs) // a hash.Hash write never fails
	h.Write(payload)
	return h.Sum(nil)
}

// TopicSigner signs Wotan topic messages with one ML-DSA-65 key. It is safe
// for concurrent use.
type TopicSigner struct {
	signer *pqc.MLDSASigner
	pubKey []byte
}

// NewTopicSigner wraps an ML-DSA-65 signer (pqc.GenerateMLDSAKeyPair, or
// pqc.NewMLDSASignerFromPrivateKey for a key loaded from a seed file). It
// returns nil for a nil signer; signing with a signer of another parameter
// set fails with ErrWrongAlgorithm.
func NewTopicSigner(signer *pqc.MLDSASigner) *TopicSigner {
	if signer == nil {
		return nil
	}
	return &TopicSigner{signer: signer, pubKey: bytes.Clone(signer.PublicKey())}
}

// PublicKey returns a copy of the signer's packed public key, or nil.
func (ts *TopicSigner) PublicKey() []byte {
	if ts == nil {
		return nil
	}
	return bytes.Clone(ts.pubKey)
}

// Sign signs a topic message now.
func (ts *TopicSigner) Sign(topic, senderID string, payload []byte) (Signed, error) {
	return ts.SignAt(topic, senderID, payload, time.Now())
}

// SignAt signs a topic message as of at.
func (ts *TopicSigner) SignAt(topic, senderID string, payload []byte, at time.Time) (Signed, error) {
	if ts == nil || ts.signer == nil {
		return Signed{}, ErrNoSigner
	}
	if ts.signer.ParameterSetID() != pqc.MLDSA65 {
		return Signed{}, ErrWrongAlgorithm
	}
	ms := at.UnixMilli()
	sig, err := ts.signer.Sign(Canonicalize(topic, senderID, ms, payload))
	if err != nil {
		return Signed{}, err
	}
	return Signed{Signature: sig, PublicKey: bytes.Clone(ts.pubKey), Algorithm: Algorithm, SignedAtUnixMs: ms}, nil
}

// CheckKey is the cheap part of verification: the algorithm is ML-DSA-65
// and the attached public key is one of trusted. With no trusted keys every
// signature is refused (fail closed).
func CheckKey(sg Signed, trusted [][]byte) error {
	if sg.Algorithm != Algorithm {
		return ErrWrongAlgorithm
	}
	if len(trusted) == 0 {
		return ErrNoTrustedKeys
	}
	if len(sg.PublicKey) == 0 || !slices.ContainsFunc(trusted, func(k []byte) bool { return bytes.Equal(k, sg.PublicKey) }) {
		return ErrUntrustedKey
	}
	return nil
}

// VerifySignature checks only the ML-DSA-65 signature over the canonical
// form, with the attached public key. Call CheckKey first: on its own this
// proves only that the sender holds some key.
func VerifySignature(topic, senderID string, payload []byte, sg Signed) error {
	v, err := pqc.NewMLDSAVerifier(pqc.MLDSA65)
	if err != nil {
		return err
	}
	ok, err := v.Verify(Canonicalize(topic, senderID, sg.SignedAtUnixMs, payload), sg.Signature, sg.PublicKey)
	if err != nil || !ok {
		// A malformed signature or key is a failed verification, not an
		// internal error: the bytes came from the network.
		return ErrInvalidSignature
	}
	return nil
}

// VerifyEvent checks a delivered (or about to be published) topic message:
// ML-DSA-65, signed by one of trusted, over exactly this topic, sender,
// signing time and payload. It keeps no state and reads no clock: replay
// and freshness are Wotan's checks at publish time.
func VerifyEvent(topic, senderID string, payload []byte, sg Signed, trusted [][]byte) error {
	if err := CheckKey(sg, trusted); err != nil {
		return err
	}
	return VerifySignature(topic, senderID, payload, sg)
}
