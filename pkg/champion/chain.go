// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2026 Stevie Bellis.

package champion

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
)

// The vor chain verifier (S-CHAMPION-HARDENING Steps 34-40, RT-14/16).
// A justification Reference names where the model's grounding came
// from; Digest ties it to the exact text. VerifyChain recomputes each
// Digest from the text the caller actually retrieved for that topic, so
// a reference whose text changed, was never retrieved, or carries a
// made-up digest does not count as trusted provenance.

// ReferenceDigest is the Digest a Reference carries for text: lowercase
// hex SHA-256 of its bytes.
func ReferenceDigest(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// VerifyReference checks one reference against lookup, which returns the
// text retrieved for a topic. A "local" reference must carry Topic and
// Digest; any reference that carries a Digest must match its topic's
// text. References that are neither ("canonical" or "direct-user" with
// no digest) pass: their trust is the label's, not a digest's.
func VerifyReference(r Reference, lookup func(topic string) (string, bool)) error {
	if r.SourceTrust == "local" {
		if r.Topic == "" {
			return errors.New("local reference has no topic")
		}
		if r.Digest == "" {
			return fmt.Errorf("local reference %q has no digest", r.Topic)
		}
	}
	if r.Digest == "" {
		return nil
	}
	if !isSHA256Hex(r.Digest) {
		return fmt.Errorf("reference %q: digest is not lowercase hex sha256", r.Topic)
	}
	if lookup == nil {
		return fmt.Errorf("reference %q: no retrieved text to verify against", r.Topic)
	}
	text, ok := lookup(r.Topic)
	if !ok {
		return fmt.Errorf("reference %q: topic was not retrieved", r.Topic)
	}
	if ReferenceDigest(text) != r.Digest {
		return fmt.Errorf("reference %q: digest does not match the retrieved text", r.Topic)
	}
	return nil
}

// VerifyChain verifies every reference in refs (see VerifyReference) and
// returns all failures joined, or nil when the whole chain verifies.
func VerifyChain(refs []Reference, lookup func(topic string) (string, bool)) error {
	var errs []error
	for i, r := range refs {
		if err := VerifyReference(r, lookup); err != nil {
			errs = append(errs, fmt.Errorf("justification[%d]: %w", i, err))
		}
	}
	return errors.Join(errs...)
}

func isSHA256Hex(s string) bool {
	if len(s) != sha256.Size*2 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
