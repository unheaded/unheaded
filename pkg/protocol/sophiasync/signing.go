// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package sophiasync

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"time"

	"unheaded/pkg/crypto/pqc"
	"unheaded/pkg/gungnir"
	"unheaded/pkg/metrics/auto"
	"unheaded/pkg/metrics/prom"
)

// Dictionary delta signatures (M-149, sophia-dictionary-03 Security
// Considerations 1 and 2): ML-DSA-65 over the delta's canonical bytes and
// its signing time, by a key the receiver trusts. A signing time more than
// MaxSkew from the receiver's clock is counted and logged, not refused:
// replay is stopped by the version check (a delta is applied only as the
// next version), and refusing a late delta wedged the receiver on the
// version before it. It replaces a "signature" that was SHA-256 of the
// delta, which anyone could recompute; that digest survives as Digest, the
// delta's identity, not its authentication.
//
// What a signature covers:
//
//	SHA-256("sophia/delta-sig/v1" || 0 || algorithm || 0 ||
//	        signed_at (int64, big-endian, Unix ms) || Canonical(delta))
//
// Canonical is the delta's wire encoding up to, not including, the
// signature fields: version, the length-prefixed additions, the removals.

// DeltaAlgorithm is the only delta signature algorithm accepted (no agility).
const DeltaAlgorithm = "ml-dsa-65"

// deltaDomain separates delta signatures from every other ML-DSA-65 use.
const deltaDomain = "sophia/delta-sig/v1"

// MaxSkew is how far a delta's signing time may be from the receiver's
// clock, either way, before the delta counts as stale
// (unheaded_sophia_delta_stale_total). sophia-dictionary-03 Security
// Considerations 2 says "reject updates with timestamps > 5 minutes in
// future/past"; the reference implementation applies them (see the package
// comment) and the wording is with the RFC desk (S-TRUE-WIRE Phase 8
// follow-up).
const MaxSkew = 5 * time.Minute

// Environment variables a service that distributes or applies deltas reads.
// No first-party service does today (sophiasync is a G8 reference package);
// these are the names it must use when one does.
const (
	// SigningKeyEnv names the delta-signing key: an ML-DSA-65 gungnir seed
	// (32 bytes, `unheaded-keygen -algo mldsa65`), mode 0400, owned by the
	// signing service's own account (the ADR-103 per-service key layout,
	// /etc/unheaded/sophia/keys/delta-signing.key). Unset: signing off.
	SigningKeyEnv = "SOPHIA_DELTA_SIGNING_KEY"
	// RequireSignatureEnv: "true" refuses unsigned deltas; unset or
	// "false" is observe mode (applied, counted, logged). When it flips:
	// docs/security/severity-ladder.md.
	RequireSignatureEnv = "SOPHIA_REQUIRE_DELTA_SIGNATURE"
)

var (
	ErrInvalidSignature = errors.New("sophiasync: delta signature verification failed")
	ErrWrongAlgorithm   = errors.New("sophiasync: only ml-dsa-65 delta signatures accepted")
	ErrNoTrustedKeys    = errors.New("sophiasync: no trusted delta-signing keys configured")
	ErrUntrustedKey     = errors.New("sophiasync: delta signed by a key that is not trusted")
	ErrNoSigner         = errors.New("sophiasync: no delta-signing key")
	ErrUnsignedDelta    = errors.New("sophiasync: unsigned delta refused (signatures required)")
)

var (
	deltaUnsigned = auto.NewCounter(prom.CounterOpts{
		Name: "unheaded_sophia_delta_unsigned_total",
		Help: "Sophia dictionary deltas received without a signature (applied in observe mode, refused when signatures are required)",
	})
	// deltaStale counts verified deltas whose signing time was more than
	// MaxSkew from the receiver's clock. They are applied: the next version
	// is the next version, and a replayed delta cannot pass the version
	// check, so refusing a late one only wedged the receiver on the version
	// before it (PR #2 review). The count is the evidence for clock drift or
	// a slow path between signer and receiver.
	deltaStale = auto.NewCounter(prom.CounterOpts{
		Name: "unheaded_sophia_delta_stale_total",
		Help: "Signed Sophia dictionary deltas applied although signed more than MaxSkew from the receiver's clock",
	})
	deltaSignatureFailures = auto.NewCounter(prom.CounterOpts{
		Name: "unheaded_sophia_delta_signature_failures_total",
		Help: "Signed Sophia dictionary deltas refused: bad signature, untrusted key or wrong algorithm",
	})
)

// Canonical returns the bytes a delta signature covers besides the
// algorithm and signing time: the delta's encoding without its signature
// fields. Every variable-length part is length-prefixed, so no two deltas
// share it.
func (d *DictionaryDelta) Canonical() []byte {
	return appendBody(make([]byte, 0, 64), d)
}

// Digest is SHA-256 of Canonical: the delta's identity (what the package
// once called its signature). It authenticates nothing.
func (d *DictionaryDelta) Digest() [sha256.Size]byte {
	return sha256.Sum256(d.Canonical())
}

// signingDigest is what the ML-DSA-65 signature is made over.
func signingDigest(d *DictionaryDelta) []byte {
	h := sha256.New()
	h.Write([]byte(deltaDomain))
	h.Write([]byte{0})
	h.Write([]byte(d.Algorithm))
	h.Write([]byte{0})
	_ = binary.Write(h, binary.BigEndian, d.SignedAtUnixMs) // a hash.Hash write never fails
	h.Write(d.Canonical())
	return h.Sum(nil)
}

// Signer signs dictionary deltas with one ML-DSA-65 key.
type Signer struct {
	signer *pqc.MLDSASigner
	pubKey []byte
}

// NewSigner wraps an ML-DSA-65 signer (pqc.GenerateMLDSAKeyPair, or
// LoadSignerFile for a key on disk).
func NewSigner(s *pqc.MLDSASigner) (*Signer, error) {
	if s == nil {
		return nil, ErrNoSigner
	}
	if s.ParameterSetID() != pqc.MLDSA65 {
		return nil, ErrWrongAlgorithm
	}
	return &Signer{signer: s, pubKey: bytes.Clone(s.PublicKey())}, nil
}

// PublicKey returns a copy of the signer's packed public key.
func (s *Signer) PublicKey() []byte {
	if s == nil {
		return nil
	}
	return bytes.Clone(s.pubKey)
}

// Sign signs d now, setting its Signature, PublicKey, Algorithm and
// SignedAtUnixMs.
func (s *Signer) Sign(d *DictionaryDelta) error {
	return s.SignAt(d, time.Now())
}

// SignAt signs d as of at.
func (s *Signer) SignAt(d *DictionaryDelta, at time.Time) error {
	if s == nil || s.signer == nil {
		return ErrNoSigner
	}
	if d == nil {
		return errors.New("sophiasync: nil delta")
	}
	d.Algorithm = DeltaAlgorithm
	d.SignedAtUnixMs = at.UnixMilli()
	d.PublicKey = bytes.Clone(s.pubKey)
	sig, err := s.signer.Sign(signingDigest(d))
	if err != nil {
		return fmt.Errorf("sophiasync: sign delta: %w", err)
	}
	d.Signature = sig
	return nil
}

// Verify checks a signed delta: ML-DSA-65, the attached public key one of
// trusted, and the signature over the delta's algorithm, signing time and
// canonical bytes. With no trusted keys it refuses everything (fail closed).
// It reads no clock: SyncManager.ApplyDelta checks the signing time.
func Verify(d *DictionaryDelta, trusted [][]byte) error {
	if d == nil {
		return ErrInvalidSignature
	}
	if d.Algorithm != DeltaAlgorithm {
		return ErrWrongAlgorithm
	}
	if len(trusted) == 0 {
		return ErrNoTrustedKeys
	}
	if len(d.PublicKey) == 0 || !slices.ContainsFunc(trusted, func(k []byte) bool { return bytes.Equal(k, d.PublicKey) }) {
		return ErrUntrustedKey
	}
	v, err := pqc.NewMLDSAVerifier(pqc.MLDSA65)
	if err != nil {
		return fmt.Errorf("sophiasync: delta verifier: %w", err)
	}
	ok, err := v.Verify(signingDigest(d), d.Signature, d.PublicKey)
	if err != nil || !ok {
		// Malformed bytes from the network are a failed verification.
		return ErrInvalidSignature
	}
	return nil
}

// LoadSignerFile loads a delta signer from an ML-DSA-65 gungnir seed file,
// refusing one that group or others can read.
func LoadSignerFile(path string) (*Signer, error) {
	if path == "" {
		return nil, errors.New("sophiasync: empty key path")
	}
	priv, err := gungnir.LoadSeedFile(path)
	if err != nil {
		return nil, fmt.Errorf("sophiasync: delta-signing key: %w", err)
	}
	s, err := pqc.NewMLDSASignerFromPrivateKey(priv)
	if err != nil {
		return nil, fmt.Errorf("sophiasync: delta-signing key %s: %w", path, err)
	}
	return NewSigner(s)
}

// SignerFromEnv loads the signer SigningKeyEnv names. Unset or empty is
// (nil, nil): signing is off, and the caller says so in its log. A key file
// that is named but cannot be used is an error, so a misconfigured signer
// fails loudly instead of distributing unsigned deltas.
func SignerFromEnv() (*Signer, error) {
	path := strings.TrimSpace(os.Getenv(SigningKeyEnv))
	if path == "" {
		return nil, nil
	}
	s, err := LoadSignerFile(path)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", SigningKeyEnv, err)
	}
	return s, nil
}

// RequireSignatureFromEnv reads RequireSignatureEnv: unset or empty is
// false (observe mode); "true"/"1" and "false"/"0" are accepted; anything
// else is an error, so a typo cannot silently mean "observe".
func RequireSignatureFromEnv() (bool, error) {
	switch v := strings.ToLower(strings.TrimSpace(os.Getenv(RequireSignatureEnv))); v {
	case "", "false", "0":
		return false, nil
	case "true", "1":
		return true, nil
	default:
		return false, fmt.Errorf("%s=%q: want true or false", RequireSignatureEnv, v)
	}
}
