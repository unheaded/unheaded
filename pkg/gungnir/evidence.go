// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package gungnir

// Evidence seals (ADR-105 decision 5): ML-DSA-65 signatures over archival
// evidence such as a Yggdrasil evidence pack's manifest.yaml, its
// image.iso.sha256 line, or one chain-of-custody entry.
//
// Why they do not expire: a pack is verified at quarterly audits, months after
// the build. A config seal (Sign/SignFor) carries an expiry so a stale baseline
// cannot be replayed, and Verify keeps refusing anything past it. Evidence
// answers a different question ("did the build key sign exactly these bytes, for
// this purpose?"), which stays true after the key is retired, so an evidence
// seal has no expiry field at all.
//
// Why they cannot be confused with config seals: the signature uses its own
// ML-DSA context ("unheaded/gungnir/evidence/v1", FIPS 204 ctx) over its own
// message layout, and the Purpose is inside the signed message. A config seal
// never verifies as evidence, evidence never verifies as a config seal, and a
// manifest seal never verifies as an iso-sha256 or custody-entry seal.

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"time"

	"github.com/cloudflare/circl/sign/mldsa/mldsa65"
)

// EvidenceFormat is the only accepted EvidenceSeal.Format.
const EvidenceFormat = "gungnir-evidence/v1"

// evidenceContext is the ML-DSA context string for evidence seals. It differs
// from sealContext (config seals) and from Wotan's topic-signature context.
const evidenceContext = "unheaded/gungnir/evidence/v1"

// MaxEvidenceSealBytes bounds an encoded seal (a 3309-byte signature is about
// 4.4 KiB of base64; the rest is short fields).
const MaxEvidenceSealBytes = 16 << 10

var (
	// ErrKeyID: the seal's key ID is not the verifying key's, or the signer
	// has no real key ID (the "dev-spike" placeholder).
	ErrKeyID = errors.New("gungnir: evidence seal key id does not match the public key")
	// ErrFormat: the seal is not a gungnir-evidence/v1 envelope.
	ErrFormat = errors.New("gungnir: not a " + EvidenceFormat + " seal")
	// ErrPurpose: the seal was issued for a different purpose, or the purpose
	// is malformed.
	ErrPurpose = errors.New("gungnir: evidence seal purpose mismatch or malformed")
)

// purposeRe: short lowercase tokens such as manifest, iso-sha256, custody-entry.
var purposeRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// EvidenceSeal is a non-expiring, domain-separated ML-DSA-65 signature. Its
// JSON field names are part of the evidence-pack format.
type EvidenceSeal struct {
	Format        string `json:"format"`
	Purpose       string `json:"purpose"`
	PayloadSha256 []byte `json:"payload_sha256"`
	KeyId         string `json:"key_id"`
	IssuedAtUnix  int64  `json:"issued_at_unix"`
	Algorithm     string `json:"algorithm"`
	Mldsa65Sig    []byte `json:"mldsa65_sig"`
}

// evidenceMessage is what an evidence signature covers: every field the
// verifier acts on, each length-prefixed (same construction as signedMessage).
func evidenceMessage(e *EvidenceSeal) []byte {
	var b bytes.Buffer
	field := func(p []byte) {
		_ = binary.Write(&b, binary.BigEndian, uint32(len(p))) // #nosec G115 -- fields are short names, a key id and a hash
		b.Write(p)
	}
	field([]byte(e.Format))
	field([]byte(e.Purpose))
	field([]byte(e.Algorithm))
	field([]byte(e.KeyId))
	_ = binary.Write(&b, binary.BigEndian, e.IssuedAtUnix)
	field(e.PayloadSha256)
	return b.Bytes()
}

func sha256Of(p []byte) []byte {
	h := sha256.Sum256(p)
	return h[:]
}

// SignEvidence seals payload for purpose. The signer must carry a real key ID
// (NewSignerWithID), so a verifier can tell which build key signed it.
func (s *Signer) SignEvidence(payload []byte, purpose string) (*EvidenceSeal, error) {
	return s.signEvidenceAt(payload, purpose, time.Now())
}

func (s *Signer) signEvidenceAt(payload []byte, purpose string, now time.Time) (*EvidenceSeal, error) {
	if s == nil || s.priv == nil || s.pub == nil {
		return nil, errors.New("gungnir: nil signer")
	}
	if !purposeRe.MatchString(purpose) {
		return nil, fmt.Errorf("%w: %q", ErrPurpose, purpose)
	}
	if s.keyID != KeyID(s.pub) {
		return nil, fmt.Errorf("%w: signer key id %q is not KeyID(pub); use NewSignerWithID", ErrKeyID, s.keyID)
	}
	seal := &EvidenceSeal{
		Format:        EvidenceFormat,
		Purpose:       purpose,
		PayloadSha256: sha256Of(payload),
		KeyId:         s.keyID,
		IssuedAtUnix:  now.Unix(),
		Algorithm:     Algorithm,
	}
	sig := make([]byte, mldsa65.SignatureSize)
	if err := mldsa65.SignTo(s.priv, evidenceMessage(seal), []byte(evidenceContext), false, sig); err != nil {
		return nil, err
	}
	seal.Mldsa65Sig = sig
	return seal, nil
}

// VerifyEvidence checks that seal is pub's evidence signature over payload for
// purpose. There is deliberately no time check (see the file comment).
func VerifyEvidence(payload []byte, seal *EvidenceSeal, pub *mldsa65.PublicKey, purpose string) error {
	if seal == nil {
		return ErrNilSeal
	}
	if pub == nil {
		return errors.New("gungnir: nil public key")
	}
	if seal.Format != EvidenceFormat {
		return ErrFormat
	}
	if seal.Algorithm != Algorithm {
		return ErrWrongAlgo
	}
	if purpose == "" || seal.Purpose != purpose {
		return fmt.Errorf("%w: seal is for %q, want %q", ErrPurpose, seal.Purpose, purpose)
	}
	if seal.KeyId != KeyID(pub) {
		return fmt.Errorf("%w: seal %q, key %q", ErrKeyID, seal.KeyId, KeyID(pub))
	}
	if subtle.ConstantTimeCompare(sha256Of(payload), seal.PayloadSha256) != 1 {
		return ErrPayloadHash
	}
	if len(seal.Mldsa65Sig) != mldsa65.SignatureSize ||
		!mldsa65.Verify(pub, evidenceMessage(seal), []byte(evidenceContext), seal.Mldsa65Sig) {
		return ErrBadSignature
	}
	return nil
}

// SignEvidenceDetached returns only the ML-DSA-65 signature of an evidence
// seal over payload for purpose, for formats that store signature bytes and
// nothing else (a Yggdrasil chain_of_custody entry's hex `signature`). The
// implied seal has IssuedAtUnix 0 and KeyId = KeyID(pub): no time is claimed,
// so whatever time matters must be inside the payload.
func (s *Signer) SignEvidenceDetached(payload []byte, purpose string) ([]byte, error) {
	seal, err := s.signEvidenceAt(payload, purpose, time.Unix(0, 0))
	if err != nil {
		return nil, err
	}
	return seal.Mldsa65Sig, nil
}

// VerifyEvidenceDetached checks a SignEvidenceDetached signature.
func VerifyEvidenceDetached(payload, sig []byte, pub *mldsa65.PublicKey, purpose string) error {
	if pub == nil {
		return errors.New("gungnir: nil public key")
	}
	return VerifyEvidence(payload, &EvidenceSeal{
		Format:        EvidenceFormat,
		Purpose:       purpose,
		PayloadSha256: sha256Of(payload),
		KeyId:         KeyID(pub),
		IssuedAtUnix:  0,
		Algorithm:     Algorithm,
		Mldsa65Sig:    sig,
	}, pub, purpose)
}

// ParseEvidenceSeal decodes a JSON evidence seal strictly: bounded size, one
// object, no unknown or duplicate fields, no trailing data. A config seal's
// JSON (GungnirSeal field names) is refused here, before any crypto.
func ParseEvidenceSeal(b []byte) (*EvidenceSeal, error) {
	if len(b) == 0 {
		return nil, fmt.Errorf("%w: empty", ErrFormat)
	}
	if len(b) > MaxEvidenceSealBytes {
		return nil, fmt.Errorf("%w: %d bytes exceeds %d", ErrFormat, len(b), MaxEvidenceSealBytes)
	}
	if err := rejectDuplicateKeys(b); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFormat, err)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	var seal EvidenceSeal
	if err := dec.Decode(&seal); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFormat, err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w: trailing data after the seal", ErrFormat)
	}
	if seal.Format != EvidenceFormat {
		return nil, ErrFormat
	}
	return &seal, nil
}

// rejectDuplicateKeys refuses a top-level JSON object that names a key twice:
// encoding/json keeps the last one, other parsers keep the first, and a seal
// must mean the same thing to every verifier.
func rejectDuplicateKeys(b []byte) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return errors.New("not a JSON object")
	}
	seen := map[string]bool{}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		k, ok := tok.(string)
		if !ok {
			return errors.New("non-string key")
		}
		if seen[k] {
			return fmt.Errorf("duplicate key %q", k)
		}
		seen[k] = true
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			return err
		}
	}
	return nil
}

// PublicKeyFromSeed derives the public key from a private seed file, with the
// same 0600 check as LoadSeedFile. It generates nothing (ADR-043 #4).
func PublicKeyFromSeed(path string) (*mldsa65.PublicKey, error) {
	priv, err := LoadSeedFile(path)
	if err != nil {
		return nil, err
	}
	pub, ok := priv.Public().(*mldsa65.PublicKey)
	if !ok {
		return nil, fmt.Errorf("gungnir: private key Public() returned %T", priv.Public())
	}
	return pub, nil
}
