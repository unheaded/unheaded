// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

package main

// Hash-chained chain_of_custody signatures (plan S-QUANTUM-ARMOR Step 213).
//
// Entry i's `signature` is the hex of a detached gungnir evidence signature
// (ML-DSA-65, context unheaded/gungnir/evidence/v1, purpose custody-entry) over
// custodyMessage(i):
//
//	lp("yggdrasil/custody/v1") lp(timestamp_utc) lp(actor) lp(action) lp(prev)
//
// where lp is a 4-byte big-endian length prefix and prev is SHA-256 of entry
// i-1's custodyMessage (32 zero bytes for entry 0). Removing, inserting or
// reordering an entry changes every later prev, so every later signature
// fails: append-only becomes checkable. The signature field itself is not in
// the chain, so signing entry i never changes entry i+1's message.

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/cloudflare/circl/sign/mldsa/mldsa65"

	"unheaded/pkg/gungnir"
)

const custodyDomain = "yggdrasil/custody/v1"

func custodyMessage(e CustodyEntry, prev []byte) []byte {
	var b bytes.Buffer
	field := func(p []byte) {
		_ = binary.Write(&b, binary.BigEndian, uint32(len(p))) // #nosec G115 -- manifest is capped at 1 MiB
		b.Write(p)
	}
	field([]byte(custodyDomain))
	field([]byte(e.TimestampUTC))
	field([]byte(e.Actor))
	field([]byte(e.Action))
	field(prev)
	return b.Bytes()
}

// custodyMessages returns every entry's signed message, chained.
func custodyMessages(entries []CustodyEntry) [][]byte {
	out := make([][]byte, len(entries))
	prev := make([]byte, sha256.Size)
	for i, e := range entries {
		out[i] = custodyMessage(e, prev)
		h := sha256.Sum256(out[i])
		prev = h[:]
	}
	return out
}

// signCustodyEntry signs entry index of a manifest (which must validate) and
// returns the hex signature for that entry's `signature` field.
func signCustodyEntry(s *gungnir.Signer, manifest []byte, index int) (string, error) {
	m, err := ValidateManifest(manifest)
	if err != nil {
		return "", fmt.Errorf("manifest does not validate:\n%v", err)
	}
	if index < 0 || index >= len(m.ChainOfCustody) {
		return "", fmt.Errorf("--custody-index %d out of range: the manifest has %d chain_of_custody entries", index, len(m.ChainOfCustody))
	}
	sig, err := s.SignEvidenceDetached(custodyMessages(m.ChainOfCustody)[index], purposeCustody)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(sig), nil
}

// checkCustody verifies the chain: timestamps never decrease, every entry is
// signed by pub over its chained message. With allowUnsigned, unsigned
// entries are listed but the chain still fails (legacy-pack inspection).
func checkCustody(entries []CustodyEntry, pub *mldsa65.PublicKey, allowUnsigned bool) []error {
	var errs []error
	if len(entries) == 0 {
		return []error{fmt.Errorf("chain_of_custody is empty")}
	}
	msgs := custodyMessages(entries)
	var last time.Time
	for i, e := range entries {
		ts, err := time.Parse(time.RFC3339Nano, e.TimestampUTC)
		if err != nil {
			errs = append(errs, fmt.Errorf("chain_of_custody[%d]: timestamp %q: %v", i, e.TimestampUTC, err))
		} else {
			if i > 0 && ts.Before(last) {
				errs = append(errs, fmt.Errorf("chain_of_custody[%d]: timestamp %s is before entry %d's %s (not append-only)", i, e.TimestampUTC, i-1, last.Format(time.RFC3339)))
			}
			last = ts
		}
		if e.Signature == "" {
			if allowUnsigned {
				errs = append(errs, fmt.Errorf("chain_of_custody[%d]: UNSIGNED (%s, %s)", i, e.TimestampUTC, e.Actor))
			} else {
				errs = append(errs, fmt.Errorf("chain_of_custody[%d]: unsigned entry (every entry must be signed)", i))
			}
			continue
		}
		sig, err := hex.DecodeString(e.Signature)
		if err != nil {
			errs = append(errs, fmt.Errorf("chain_of_custody[%d]: signature is not hex: %v", i, err))
			continue
		}
		if err := gungnir.VerifyEvidenceDetached(msgs[i], sig, pub, purposeCustody); err != nil {
			errs = append(errs, fmt.Errorf("chain_of_custody[%d]: %v (entry altered, removed, reordered, or signed by another key)", i, err))
		}
	}
	return errs
}
